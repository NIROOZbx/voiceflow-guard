//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/NIROOZbx/voiceflow-guard/internal/service"
)

// Scenario B: Retry with the same key and payload
func TestRetryWithSameKeyAndPayload(t *testing.T) {
	app := newTestApp(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	key := newIdempotencyKey()
	payload := validBookingPayload()

	// 1. Initial Request
	resp1, body1 := app.PostBooking(t, key, payload)
	if resp1.StatusCode != http.StatusCreated {
		t.Fatalf("first request expected status %d, got %d. Body: %s", http.StatusCreated, resp1.StatusCode, string(body1))
	}

	var result1 service.CreateBookingResult
	if err := json.Unmarshal(body1, &result1); err != nil {
		t.Fatalf("unmarshal first response: %v", err)
	}

	// 2. Retry Request (exact same key and payload)
	resp2, body2 := app.PostBooking(t, key, payload)
	if resp2.StatusCode != http.StatusCreated {
		t.Fatalf("retry request expected status %d, got %d. Body: %s", http.StatusCreated, resp2.StatusCode, string(body2))
	}

	var result2 service.CreateBookingResult
	if err := json.Unmarshal(body2, &result2); err != nil {
		t.Fatalf("unmarshal retry response: %v", err)
	}

	// 3. Verifications
	if result2.BookingID != result1.BookingID {
		t.Errorf("expected booking ID to remain identical: original %q, retry %q", result1.BookingID, result2.BookingID)
	}
	if result2.OperationID != result1.OperationID {
		t.Errorf("expected operation ID to remain identical: original %q, retry %q", result1.OperationID, result2.OperationID)
	}
	if result2.Status != "COMPLETED" {
		t.Errorf("expected status 'COMPLETED', got %q", result2.Status)
	}

	// Verify no additional records created
	opCount, err := app.TotalOperations(ctx)
	if err != nil || opCount != 1 {
		t.Errorf("expected exactly 1 operation in DB, got %d (err: %v)", opCount, err)
	}
	bookingCount, err := app.TotalBookings(ctx)
	if err != nil || bookingCount != 1 {
		t.Errorf("expected exactly 1 booking in DB, got %d (err: %v)", bookingCount, err)
	}
}

// Scenario C: Same key with a different payload
func TestSameKeyWithDifferentPayload(t *testing.T) {
	app := newTestApp(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	key := newIdempotencyKey()
	originalPayload := validBookingPayload()

	// 1. Initial Request
	resp1, body1 := app.PostBooking(t, key, originalPayload)
	if resp1.StatusCode != http.StatusCreated {
		t.Fatalf("first request expected %d, got %d. Body: %s", http.StatusCreated, resp1.StatusCode, string(body1))
	}

	// 2. Modified Payload with same key
	modifiedPayload := validBookingPayload()
	modifiedPayload["party_size"] = 10 // Change party size from 4 to 10

	resp2, body2 := app.PostBooking(t, key, modifiedPayload)
	if resp2.StatusCode != http.StatusConflict {
		t.Fatalf("expected status %d (Conflict), got %d. Body: %s", http.StatusConflict, resp2.StatusCode, string(body2))
	}

	var errResp map[string]string
	if err := json.Unmarshal(body2, &errResp); err != nil {
		t.Fatalf("unmarshal error response: %v", err)
	}
	if !strings.Contains(errResp["error"], "idempotency key reused with different request") {
		t.Errorf("expected conflict error message, got: %v", errResp["error"])
	}

	// 3. Verify no additional operation or booking was created
	opCount, err := app.TotalOperations(ctx)
	if err != nil || opCount != 1 {
		t.Errorf("expected exactly 1 operation in DB, got %d (err: %v)", opCount, err)
	}
	bookingCount, err := app.TotalBookings(ctx)
	if err != nil || bookingCount != 1 {
		t.Errorf("expected exactly 1 booking in DB, got %d (err: %v)", bookingCount, err)
	}
}

// Scenario D: Different key with confirmed duplicate payload
func TestDifferentKeyWithConfirmedDuplicate(t *testing.T) {
	app := newTestApp(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	key1 := newIdempotencyKey()
	key2 := newIdempotencyKey()
	payload := validBookingPayload()

	// 1. First Request
	resp1, body1 := app.PostBooking(t, key1, payload)
	if resp1.StatusCode != http.StatusCreated {
		t.Fatalf("request 1 expected %d, got %d. Body: %s", http.StatusCreated, resp1.StatusCode, string(body1))
	}
	var result1 service.CreateBookingResult
	if err := json.Unmarshal(body1, &result1); err != nil {
		t.Fatalf("unmarshal response 1: %v", err)
	}

	// 2. Second Request with different key and confirmed duplicate flag
	payload2 := validBookingPayload()
	payload2["confirm_potential_duplicate"] = true

	resp2, body2 := app.PostBooking(t, key2, payload2)
	if resp2.StatusCode != http.StatusCreated {
		t.Fatalf("request 2 expected %d, got %d. Body: %s", http.StatusCreated, resp2.StatusCode, string(body2))
	}
	var result2 service.CreateBookingResult
	if err := json.Unmarshal(body2, &result2); err != nil {
		t.Fatalf("unmarshal response 2: %v", err)
	}

	// 3. Verifications
	if result1.BookingID == result2.BookingID {
		t.Errorf("expected distinct booking IDs, got identical: %q", result1.BookingID)
	}
	if result1.OperationID == result2.OperationID {
		t.Errorf("expected distinct operation IDs, got identical: %q", result1.OperationID)
	}

	// Verify database contains 2 operations and 2 bookings
	opCount, err := app.TotalOperations(ctx)
	if err != nil || opCount != 2 {
		t.Errorf("expected 2 operations in DB, got %d (err: %v)", opCount, err)
	}
	bookingCount, err := app.TotalBookings(ctx)
	if err != nil || bookingCount != 2 {
		t.Errorf("expected 2 bookings in DB, got %d (err: %v)", bookingCount, err)
	}
}

// Scenario D.2: Unconfirmed duplicate booking is rejected with 409 Conflict
func TestDuplicateBookingDetectionAndConfirmation(t *testing.T) {
	app := newTestApp(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	key1 := newIdempotencyKey()
	payload := validBookingPayload()

	// 1. First booking succeeds
	resp1, body1 := app.PostBooking(t, key1, payload)
	if resp1.StatusCode != http.StatusCreated {
		t.Fatalf("first booking failed: %d. Body: %s", resp1.StatusCode, string(body1))
	}

	// 2. Second booking with new key for same slot without confirmation is rejected
	key2 := newIdempotencyKey()
	resp2, body2 := app.PostBooking(t, key2, payload)
	if resp2.StatusCode != http.StatusConflict {
		t.Fatalf("expected status 409 Conflict for duplicate booking, got %d. Body: %s", resp2.StatusCode, string(body2))
	}

	var conflictResp struct {
		Error              string `json:"error"`
		PotentialDuplicate struct {
			BookingDate string `json:"booking_date"`
			BookingTime string `json:"booking_time"`
			PartySize   int    `json:"party_size"`
		} `json:"potential_duplicate"`
	}
	if err := json.Unmarshal(body2, &conflictResp); err != nil {
		t.Fatalf("failed to decode duplicate 409 response: %v", err)
	}
	if !strings.Contains(conflictResp.Error, "potential duplicate booking detected") {
		t.Errorf("unexpected error message: %q", conflictResp.Error)
	}
	if conflictResp.PotentialDuplicate.PartySize != payload["party_size"].(int) {
		t.Errorf("expected duplicate party_size %v, got %v", payload["party_size"], conflictResp.PotentialDuplicate.PartySize)
	}

	// 3. Third booking with confirmed duplicate flag succeeds
	key3 := newIdempotencyKey()
	confirmedPayload := validBookingPayload()
	confirmedPayload["confirm_potential_duplicate"] = true

	resp3, body3 := app.PostBooking(t, key3, confirmedPayload)
	if resp3.StatusCode != http.StatusCreated {
		t.Fatalf("confirmed duplicate expected 201 Created, got %d. Body: %s", resp3.StatusCode, string(body3))
	}

	// Database should contain 2 operations and 2 bookings (the rejected duplicate does not persist)
	totalBookings, err := app.TotalBookings(ctx)
	if err != nil || totalBookings != 2 {
		t.Errorf("expected 2 bookings in DB, got %d (err: %v)", totalBookings, err)
	}
}

// Scenario E: Missing idempotency key
func TestMissingIdempotencyKey(t *testing.T) {
	app := newTestApp(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	payload := validBookingPayload()

	// Send without Idempotency-Key header
	resp, body := app.PostBooking(t, "", payload)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected status %d (Bad Request), got %d. Body: %s", http.StatusBadRequest, resp.StatusCode, string(body))
	}

	var errResp map[string]string
	if err := json.Unmarshal(body, &errResp); err != nil {
		t.Fatalf("unmarshal error response: %v", err)
	}
	if !strings.Contains(strings.ToLower(errResp["error"]), "idempotency-key") {
		t.Errorf("expected error to mention Idempotency-Key, got %q", errResp["error"])
	}

	// Verify nothing was persisted
	opCount, err := app.TotalOperations(ctx)
	if err != nil || opCount != 0 {
		t.Errorf("expected 0 operations in DB, got %d (err: %v)", opCount, err)
	}
	bookingCount, err := app.TotalBookings(ctx)
	if err != nil || bookingCount != 0 {
		t.Errorf("expected 0 bookings in DB, got %d (err: %v)", bookingCount, err)
	}
}

// Scenario F: Invalid idempotency key
func TestInvalidIdempotencyKey(t *testing.T) {
	tests := []struct {
		name string
		key  string
	}{
		{
			name: "empty string key",
			key:  "",
		},
		{
			name: "whitespace only key",
			key:  "     ",
		},
		{
			name: "key exceeding 255 characters",
			key:  strings.Repeat("k", 256),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			app := newTestApp(t)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			payload := validBookingPayload()

			resp, body := app.PostBooking(t, tc.key, payload)
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("[%s] expected status %d, got %d. Body: %s", tc.name, http.StatusBadRequest, resp.StatusCode, string(body))
			}

			// Verify nothing persisted
			opCount, err := app.TotalOperations(ctx)
			if err != nil || opCount != 0 {
				t.Errorf("[%s] expected 0 operations in DB, got %d", tc.name, opCount)
			}
			bookingCount, err := app.TotalBookings(ctx)
			if err != nil || bookingCount != 0 {
				t.Errorf("[%s] expected 0 bookings in DB, got %d", tc.name, bookingCount)
			}
		})
	}
}
