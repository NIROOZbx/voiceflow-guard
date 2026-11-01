//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/NIROOZbx/voiceflow-guard/internal/service"
)

type httpResult struct {
	StatusCode int
	Body       []byte
	Err        error
}

/*
Expected Concurrency Behavior:
------------------------------
When multiple concurrent HTTP requests arrive with the same Idempotency-Key and payload:
 1. The PostgreSQL unique constraint on operations(idempotency_key) guarantees that only
    one transaction can insert the initial operation record.
 2. Concurrent transactions attempting to insert with the same key wait for the first transaction's lock.
 3. When the first transaction commits:
    - The subsequent transactions encounter a unique constraint violation (SQLSTATE 23505).
    - The application detects this unique violation, rolls back the sub-transaction cleanly,
      and queries the committed operation record.
 4. Responses:
    - If the operation is COMPLETED, the caller receives HTTP 201 Created with the exact same
      booking_id and operation_id.
    - If another request arrives while the operation is still PENDING, the API returns
      HTTP 409 Conflict with error "operation is still in progress".
 5. In all cases:
    - No 500 Internal Server Errors occur.
    - Exactly ONE operation and ONE booking are persisted in PostgreSQL.
    - No duplicate bookings are ever created.
*/

// Scenario H: Concurrent requests with the same key and payload
func TestConcurrentRequestsSameKey(t *testing.T) {
	app := newTestApp(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	key := newIdempotencyKey()
	payload := validBookingPayload()
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	const numWorkers = 20
	results := make([]httpResult, numWorkers)

	var readyWg sync.WaitGroup
	var doneWg sync.WaitGroup
	startGate := make(chan struct{})

	readyWg.Add(numWorkers)
	doneWg.Add(numWorkers)

	for i := 0; i < numWorkers; i++ {
		workerID := i
		go func() {
			defer doneWg.Done()

			// Prepare request
			req, err := http.NewRequest(http.MethodPost, app.Server.URL+"/bookings", bytes.NewReader(bodyBytes))
			if err != nil {
				results[workerID] = httpResult{Err: err}
				readyWg.Done()
				return
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Idempotency-Key", key)

			// Signal ready
			readyWg.Done()

			// Synchronize all workers to release simultaneously
			<-startGate

			resp, err := app.Client.Do(req)
			if err != nil {
				results[workerID] = httpResult{Err: err}
				return
			}
			defer resp.Body.Close()

			respBody, err := io.ReadAll(resp.Body)
			results[workerID] = httpResult{
				StatusCode: resp.StatusCode,
				Body:       respBody,
				Err:        err,
			}
		}()
	}

	// Wait until all workers are primed
	readyWg.Wait()
	// Open gate: all 20 requests fire concurrently
	close(startGate)
	// Wait for all requests to finish
	doneWg.Wait()

	// Analyze responses
	var (
		createdCount  int
		conflictCount int
		firstBookingID string
		firstOperationID string
	)

	for i, r := range results {
		if r.Err != nil {
			t.Fatalf("worker %d network error: %v", i, r.Err)
		}

		switch r.StatusCode {
		case http.StatusCreated:
			createdCount++
			var res service.CreateBookingResult
			if err := json.Unmarshal(r.Body, &res); err != nil {
				t.Fatalf("worker %d failed to unmarshal 201 response: %v. Body: %s", i, err, string(r.Body))
			}

			if firstBookingID == "" {
				firstBookingID = res.BookingID
				firstOperationID = res.OperationID
			} else {
				// All successful responses must reference the SAME booking ID and operation ID
				if res.BookingID != firstBookingID {
					t.Errorf("worker %d got booking ID %q, expected %q", i, res.BookingID, firstBookingID)
				}
				if res.OperationID != firstOperationID {
					t.Errorf("worker %d got operation ID %q, expected %q", i, res.OperationID, firstOperationID)
				}
			}

		case http.StatusConflict:
			conflictCount++
			var errResp map[string]string
			if err := json.Unmarshal(r.Body, &errResp); err != nil {
				t.Fatalf("worker %d failed to unmarshal 409 response: %v", i, err)
			}
			errMsg := errResp["error"]
			if errMsg != "operation is still in progress" && errMsg != "idempotency key reused with different request" {
				t.Errorf("worker %d unexpected 409 message: %q", i, errMsg)
			}

		default:
			t.Errorf("worker %d returned unexpected status %d (body: %s)", i, r.StatusCode, string(r.Body))
		}
	}

	t.Logf("Concurrent run results: %d Created (201), %d Conflict (409)", createdCount, conflictCount)

	// At least one request must succeed with 201 Created
	if createdCount < 1 {
		t.Fatalf("expected at least 1 request to succeed with 201 Created, got %d", createdCount)
	}

	// Verify database state: exactly ONE operation and ONE booking persisted
	opCount, err := app.CountOperations(ctx, key)
	if err != nil {
		t.Fatalf("count operations: %v", err)
	}
	if opCount != 1 {
		t.Errorf("expected exactly 1 operation in DB for key, got %d", opCount)
	}

	totalOps, err := app.TotalOperations(ctx)
	if err != nil {
		t.Fatalf("total operations: %v", err)
	}
	if totalOps != 1 {
		t.Errorf("expected exactly 1 total operation in DB, got %d", totalOps)
	}

	totalBookings, err := app.TotalBookings(ctx)
	if err != nil {
		t.Fatalf("total bookings: %v", err)
	}
	if totalBookings != 1 {
		t.Errorf("expected exactly 1 total booking in DB, got %d", totalBookings)
	}

	// Verify the single persisted booking matches the booking ID returned to clients
	op, err := app.Queries.GetOperationByIdempotencyKey(ctx, key)
	if err != nil {
		t.Fatalf("get operation by key: %v", err)
	}
	if op.Status != "COMPLETED" {
		t.Errorf("expected operation status 'COMPLETED', got %q", op.Status)
	}

	booking, err := app.Queries.GetBookingByOperationID(ctx, op.ID)
	if err != nil {
		t.Fatalf("get booking: %v", err)
	}
	if booking.ID.String() != firstBookingID {
		t.Errorf("persisted booking ID %q does not match returned booking ID %q", booking.ID.String(), firstBookingID)
	}
}

// TestConcurrentRequestsSameKeyWithConflictingPayloads verifies that when concurrent requests
// race with different payloads for the same idempotency key, only one payload is committed,
// all other requests are safely rejected with 409 Conflict, and no duplicates are created.
func TestConcurrentRequestsSameKeyWithConflictingPayloads(t *testing.T) {
	app := newTestApp(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	key := newIdempotencyKey()

	payloadA := validBookingPayload()
	payloadA["party_size"] = 2
	bytesA, _ := json.Marshal(payloadA)

	payloadB := validBookingPayload()
	payloadB["party_size"] = 8
	bytesB, _ := json.Marshal(payloadB)

	const numWorkers = 20
	results := make([]httpResult, numWorkers)

	var readyWg sync.WaitGroup
	var doneWg sync.WaitGroup
	startGate := make(chan struct{})

	readyWg.Add(numWorkers)
	doneWg.Add(numWorkers)

	for i := 0; i < numWorkers; i++ {
		workerID := i
		var body []byte
		if workerID%2 == 0 {
			body = bytesA
		} else {
			body = bytesB
		}

		go func() {
			defer doneWg.Done()

			req, _ := http.NewRequest(http.MethodPost, app.Server.URL+"/bookings", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Idempotency-Key", key)

			readyWg.Done()
			<-startGate

			resp, err := app.Client.Do(req)
			if err != nil {
				results[workerID] = httpResult{Err: err}
				return
			}
			defer resp.Body.Close()

			respBody, err := io.ReadAll(resp.Body)
			results[workerID] = httpResult{
				StatusCode: resp.StatusCode,
				Body:       respBody,
				Err:        err,
			}
		}()
	}

	readyWg.Wait()
	close(startGate)
	doneWg.Wait()

	for i, r := range results {
		if r.Err != nil {
			t.Fatalf("worker %d error: %v", i, r.Err)
		}
		if r.StatusCode != http.StatusCreated && r.StatusCode != http.StatusConflict {
			t.Errorf("worker %d unexpected status: %d (body: %s)", i, r.StatusCode, string(r.Body))
		}
	}

	// Exactly 1 operation and 1 booking in DB
	totalOps, err := app.TotalOperations(ctx)
	if err != nil || totalOps != 1 {
		t.Errorf("expected 1 operation in DB, got %d", totalOps)
	}
	totalBookings, err := app.TotalBookings(ctx)
	if err != nil || totalBookings != 1 {
		t.Errorf("expected 1 booking in DB, got %d", totalBookings)
	}
}

// TestConcurrentRequestsDifferentKeysSameSlot verifies that when concurrent requests race
// for the exact same booking slot (phone, date, time) with different idempotency keys,
// PostgreSQL advisory locking serializes them, duplicate detection catches the subsequent ones,
// and exactly ONE booking is persisted in the database.
func TestConcurrentRequestsDifferentKeysSameSlot(t *testing.T) {
	app := newTestApp(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	payload := validBookingPayload()
	bodyBytes, _ := json.Marshal(payload)

	const numWorkers = 10
	results := make([]httpResult, numWorkers)

	var readyWg sync.WaitGroup
	var doneWg sync.WaitGroup
	startGate := make(chan struct{})

	readyWg.Add(numWorkers)
	doneWg.Add(numWorkers)

	for i := 0; i < numWorkers; i++ {
		workerID := i
		// Each worker gets a UNIQUE idempotency key
		workerKey := newIdempotencyKey()

		go func() {
			defer doneWg.Done()

			req, _ := http.NewRequest(http.MethodPost, app.Server.URL+"/bookings", bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Idempotency-Key", workerKey)

			readyWg.Done()
			<-startGate

			resp, err := app.Client.Do(req)
			if err != nil {
				results[workerID] = httpResult{Err: err}
				return
			}
			defer resp.Body.Close()

			respBody, err := io.ReadAll(resp.Body)
			results[workerID] = httpResult{
				StatusCode: resp.StatusCode,
				Body:       respBody,
				Err:        err,
			}
		}()
	}

	readyWg.Wait()
	close(startGate)
	doneWg.Wait()

	var createdCount int
	var duplicateConflictCount int

	for i, r := range results {
		if r.Err != nil {
			t.Fatalf("worker %d error: %v", i, r.Err)
		}
		switch r.StatusCode {
		case http.StatusCreated:
			createdCount++
		case http.StatusConflict:
			duplicateConflictCount++
		default:
			t.Errorf("worker %d unexpected status: %d (body: %s)", i, r.StatusCode, string(r.Body))
		}
	}

	// Exactly 1 worker must succeed, all other concurrent slot claimants are rejected with 409
	if createdCount != 1 {
		t.Errorf("expected exactly 1 created booking, got %d", createdCount)
	}
	if duplicateConflictCount != numWorkers-1 {
		t.Errorf("expected %d duplicate conflict rejections, got %d", numWorkers-1, duplicateConflictCount)
	}

	// Database state: exactly 1 booking persisted
	totalBookings, err := app.TotalBookings(ctx)
	if err != nil || totalBookings != 1 {
		t.Errorf("expected exactly 1 booking in DB, got %d", totalBookings)
	}
}
