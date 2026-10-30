//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/NIROOZbx/voiceflow-guard/internal/service"
	"github.com/NIROOZbx/voiceflow-guard/pkg/hasher"
	"github.com/google/uuid"
)

func TestNormalBookingCreation(t *testing.T) {
	app := newTestApp(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	key := newIdempotencyKey()
	payload := validBookingPayload()

	resp, respBody := app.PostBooking(t, key, payload)

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected status %d, got %d. Body: %s", http.StatusCreated, resp.StatusCode, string(respBody))
	}

	var result service.CreateBookingResult
	if err := json.Unmarshal(respBody, &result); err != nil {
		t.Fatalf("failed to decode response JSON: %v", err)
	}

	if result.Status != "COMPLETED" {
		t.Errorf("expected status 'COMPLETED', got %q", result.Status)
	}
	if _, err := uuid.Parse(result.OperationID); err != nil {
		t.Errorf("invalid operation_id UUID: %q", result.OperationID)
	}
	if _, err := uuid.Parse(result.BookingID); err != nil {
		t.Errorf("invalid booking_id UUID: %q", result.BookingID)
	}
	if result.CustomerName != payload["customer_name"] {
		t.Errorf("expected customer_name %v, got %v", payload["customer_name"], result.CustomerName)
	}
	if result.PartySize != payload["party_size"] {
		t.Errorf("expected party_size %v, got %v", payload["party_size"], result.PartySize)
	}
	if result.BookingDate != payload["booking_date"] {
		t.Errorf("expected booking_date %v, got %v", payload["booking_date"], result.BookingDate)
	}
	if result.BookingTime != payload["booking_time"] {
		t.Errorf("expected booking_time %v, got %v", payload["booking_time"], result.BookingTime)
	}

	// Database assertions
	opCount, err := app.CountOperations(ctx, key)
	if err != nil {
		t.Fatalf("count operations: %v", err)
	}
	if opCount != 1 {
		t.Errorf("expected exactly 1 operation for key, got %d", opCount)
	}

	op, err := app.Queries.GetOperationByIdempotencyKey(ctx, key)
	if err != nil {
		t.Fatalf("get operation by key: %v", err)
	}
	if op.Status != "COMPLETED" {
		t.Errorf("expected operation status 'COMPLETED', got %q", op.Status)
	}

	expectedHash, err := hasher.JSON(service.BookingHashPayload{
		CustomerName: payload["customer_name"].(string),
		BookingDate:  payload["booking_date"].(string),
		BookingTime:  payload["booking_time"].(string),
		PartySize:    payload["party_size"].(int),
	})
	if err != nil {
		t.Fatalf("compute expected hash: %v", err)
	}
	if op.RequestHash.String != expectedHash {
		t.Errorf("expected hash %q, got %q", expectedHash, op.RequestHash.String)
	}

	bookingCount, err := app.CountBookingsByOperationID(ctx, op.ID)
	if err != nil {
		t.Fatalf("count bookings: %v", err)
	}
	if bookingCount != 1 {
		t.Errorf("expected exactly 1 booking for operation, got %d", bookingCount)
	}

	booking, err := app.Queries.GetBookingByOperationID(ctx, op.ID)
	if err != nil {
		t.Fatalf("get booking by op ID: %v", err)
	}
	if booking.CustomerName != payload["customer_name"] {
		t.Errorf("expected booking customer_name %v, got %v", payload["customer_name"], booking.CustomerName)
	}
	if int(booking.PartySize) != payload["party_size"] {
		t.Errorf("expected booking party_size %v, got %v", payload["party_size"], booking.PartySize)
	}

	totalOps, err := app.TotalOperations(ctx)
	if err != nil || totalOps != 1 {
		t.Errorf("expected total operations in db to be 1, got %d (err: %v)", totalOps, err)
	}
	totalBookings, err := app.TotalBookings(ctx)
	if err != nil || totalBookings != 1 {
		t.Errorf("expected total bookings in db to be 1, got %d (err: %v)", totalBookings, err)
	}
}
