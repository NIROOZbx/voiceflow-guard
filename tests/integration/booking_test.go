//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	if result.PhoneNumber != payload["phone_no"] {
		t.Errorf("expected phone_no %v, got %v", payload["phone_no"], result.PhoneNumber)
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
		PhoneNo:      payload["phone_no"].(string),
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
	if booking.PhoneNo != payload["phone_no"] {
		t.Errorf("expected booking phone_no %v, got %v", payload["phone_no"], booking.PhoneNo)
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

func TestListBookings(t *testing.T) {
	app := newTestApp(t)

	// Create a booking first
	key := newIdempotencyKey()
	payload := validBookingPayload()
	resp, body := app.PostBooking(t, key, payload)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create booking failed: %d, body: %s", resp.StatusCode, string(body))
	}

	// 1. Query by phone number
	phone := payload["phone_no"].(string)
	reqURL := fmt.Sprintf("%s/bookings?phone_no=%s", app.Server.URL, phone)
	getResp, err := app.Client.Get(reqURL)
	if err != nil {
		t.Fatalf("GET /bookings request failed: %v", err)
	}
	defer getResp.Body.Close()

	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", getResp.StatusCode)
	}

	var bookings []service.CreateBookingResult
	getBody, _ := io.ReadAll(getResp.Body)
	if err := json.Unmarshal(getBody, &bookings); err != nil {
		t.Fatalf("failed to decode GET response: %v", err)
	}
	if len(bookings) != 1 {
		t.Fatalf("expected 1 booking for phone, got %d", len(bookings))
	}
	if bookings[0].PhoneNumber != phone {
		t.Errorf("expected phone %s, got %s", phone, bookings[0].PhoneNumber)
	}

	// 2. Query by phone and date filter
	date := payload["booking_date"].(string)
	dateURL := fmt.Sprintf("%s/bookings?phone_no=%s&date=%s", app.Server.URL, phone, date)
	dateResp, err := app.Client.Get(dateURL)
	if err != nil {
		t.Fatalf("GET /bookings with date failed: %v", err)
	}
	defer dateResp.Body.Close()

	if dateResp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", dateResp.StatusCode)
	}
	var dateBookings []service.CreateBookingResult
	dateBody, _ := io.ReadAll(dateResp.Body)
	if err := json.Unmarshal(dateBody, &dateBookings); err != nil {
		t.Fatalf("failed to decode GET response: %v", err)
	}
	if len(dateBookings) != 1 {
		t.Fatalf("expected 1 booking for phone and date, got %d", len(dateBookings))
	}

	// 3. Query with non-existent date should return empty array
	emptyURL := fmt.Sprintf("%s/bookings?phone_no=%s&date=2099-01-01", app.Server.URL, phone)
	emptyResp, err := app.Client.Get(emptyURL)
	if err != nil {
		t.Fatalf("GET /bookings empty failed: %v", err)
	}
	defer emptyResp.Body.Close()
	var emptyBookings []service.CreateBookingResult
	emptyBody, _ := io.ReadAll(emptyResp.Body)
	if err := json.Unmarshal(emptyBody, &emptyBookings); err != nil {
		t.Fatalf("failed to decode empty response: %v", err)
	}
	if len(emptyBookings) != 0 {
		t.Errorf("expected 0 bookings for non-existent date, got %d", len(emptyBookings))
	}
}
