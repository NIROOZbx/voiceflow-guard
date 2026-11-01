//go:build integration

package integration

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/NIROOZbx/voiceflow-guard/internal/database/sqlc"
	"github.com/NIROOZbx/voiceflow-guard/internal/repository"
	"github.com/jackc/pgx/v5/pgtype"
)

// failingTxManager intercepts transaction callbacks to inject controlled failures.
type failingTxManager struct {
	base                  repository.TransactionManager
	failOnBookingCreation bool
	failOnStatusUpdate    bool
}

func (m *failingTxManager) WithinTransaction(ctx context.Context, fn func(*repository.Repositories) error) error {
	return m.base.WithinTransaction(ctx, func(r *repository.Repositories) error {
		if m.failOnBookingCreation {
			r.Bookings = &mockFailingBookingsRepo{BookingsRepository: r.Bookings}
		}
		if m.failOnStatusUpdate {
			r.Operations = &mockFailingOperationsRepo{OperationsRepository: r.Operations}
		}
		return fn(r)
	})
}

type mockFailingBookingsRepo struct {
	repository.BookingsRepository
}

func (m *mockFailingBookingsRepo) CreateBooking(ctx context.Context, params repository.CreateBookingParams) (sqlc.Booking, error) {
	return sqlc.Booking{}, errors.New("simulated booking creation database failure")
}

type mockFailingOperationsRepo struct {
	repository.OperationsRepository
}

func (m *mockFailingOperationsRepo) UpdateOperationStatus(ctx context.Context, id pgtype.UUID, status string, result []byte) (sqlc.Operation, error) {
	// Simulate failure when updating the operation status after booking creation
	return sqlc.Operation{}, errors.New("simulated operation status update failure")
}

// Scenario G.1: Failure during booking creation after operation has been inserted
func TestRollbackOnBookingCreationFailure(t *testing.T) {
	baseTxManager := repository.NewTransactionManager(globalDBPool)
	injectedTxManager := &failingTxManager{
		base:                  baseTxManager,
		failOnBookingCreation: true,
	}

	app := newTestAppWithTxManager(t, injectedTxManager)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	key := newIdempotencyKey()
	payload := validBookingPayload()

	resp, body := app.PostBooking(t, key, payload)

	// API must return 500 Internal Server Error
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d. Body: %s", http.StatusInternalServerError, resp.StatusCode, string(body))
	}

	// Verify the operation insert was rolled back completely
	opCount, err := app.CountOperations(ctx, key)
	if err != nil {
		t.Fatalf("count operations: %v", err)
	}
	if opCount != 0 {
		t.Errorf("expected operation to be rolled back (count 0), got %d", opCount)
	}

	// Verify no booking was persisted
	totalBookings, err := app.TotalBookings(ctx)
	if err != nil {
		t.Fatalf("count bookings: %v", err)
	}
	if totalBookings != 0 {
		t.Errorf("expected no bookings in DB, got %d", totalBookings)
	}

	// Verify no incomplete or pending operations remain in the database
	totalOps, err := app.TotalOperations(ctx)
	if err != nil {
		t.Fatalf("total operations: %v", err)
	}
	if totalOps != 0 {
		t.Errorf("expected 0 total operations in DB, got %d", totalOps)
	}
}

// Scenario G.2: Failure when updating operation status after booking creation
func TestRollbackOnUpdateOperationStatusFailure(t *testing.T) {
	baseTxManager := repository.NewTransactionManager(globalDBPool)
	injectedTxManager := &failingTxManager{
		base:               baseTxManager,
		failOnStatusUpdate: true,
	}

	app := newTestAppWithTxManager(t, injectedTxManager)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	key := newIdempotencyKey()
	payload := validBookingPayload()

	resp, body := app.PostBooking(t, key, payload)

	// API must return 500 Internal Server Error
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d. Body: %s", http.StatusInternalServerError, resp.StatusCode, string(body))
	}

	// Verify BOTH the operation and booking were rolled back atomically
	opCount, err := app.TotalOperations(ctx)
	if err != nil {
		t.Fatalf("count operations: %v", err)
	}
	if opCount != 0 {
		t.Errorf("expected 0 operations after rollback, got %d", opCount)
	}

	bookingCount, err := app.TotalBookings(ctx)
	if err != nil {
		t.Fatalf("count bookings: %v", err)
	}
	if bookingCount != 0 {
		t.Errorf("expected 0 bookings after rollback, got %d", bookingCount)
	}
}

// Scenario G.3: Real PostgreSQL database constraint violation rollback
func TestRollbackOnDBConstraintViolation(t *testing.T) {
	app := newTestApp(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	key := newIdempotencyKey()
	payload := validBookingPayload()
	// Exceeds VARCHAR(255) for customer_name in bookings table
	payload["customer_name"] = strings.Repeat("A", 300)

	resp, body := app.PostBooking(t, key, payload)

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d. Body: %s", http.StatusInternalServerError, resp.StatusCode, string(body))
	}

	// Verify that the earlier operation insert was rolled back by PostgreSQL
	opCount, err := app.CountOperations(ctx, key)
	if err != nil {
		t.Fatalf("count operations: %v", err)
	}
	if opCount != 0 {
		t.Errorf("expected operation to be rolled back on DB constraint failure, got %d", opCount)
	}

	bookingCount, err := app.TotalBookings(ctx)
	if err != nil {
		t.Fatalf("count bookings: %v", err)
	}
	if bookingCount != 0 {
		t.Errorf("expected 0 bookings, got %d", bookingCount)
	}
}
