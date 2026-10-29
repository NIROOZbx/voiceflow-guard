package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/NIROOZbx/voiceflow-guard/internal/database/sqlc"
	"github.com/NIROOZbx/voiceflow-guard/internal/repository"
	"github.com/NIROOZbx/voiceflow-guard/pkg/apperror"
	"github.com/NIROOZbx/voiceflow-guard/pkg/hasher"
	"github.com/jackc/pgx/v5"
)

type BookingService	 struct {
	operationsRepo     repository.OperationsRepository
	bookingsRepo repository.BookingsRepository
}
type CreateBookingInput struct {
	IdempotencyKey string
	CustomerName   string
	PartySize      int
	BookingDate    time.Time
	BookingTime    time.Time
}

type BookingHashPayload struct {
	CustomerName string `json:"customer_name"`
	BookingDate  string `json:"booking_date"`
	BookingTime  string `json:"booking_time"`
	PartySize    int    `json:"party_size"`
}

type CreateBookingResult struct {
	OperationID  string `json:"operation_id"`
	BookingID    string `json:"booking_id"`
	Status       string `json:"status"`
	CustomerName string `json:"customer_name"`
	BookingDate  string `json:"booking_date"`
	BookingTime  string `json:"booking_time"`
	PartySize    int    `json:"party_size"`
}

func NewOpsService(opsRepo repository.OperationsRepository, bookingRepo repository.BookingsRepository) *BookingService {
	return &BookingService{
		operationsRepo:     opsRepo,
		bookingsRepo: bookingRepo,
	}
}

func (s *BookingService) CreateBooking(ctx context.Context, input CreateBookingInput) (*CreateBookingResult, error) {
	key := strings.TrimSpace(input.IdempotencyKey)
	if key == "" {
		return nil, apperror.ErrMissingIdempotencyKey
	}

	customerName := strings.TrimSpace(input.CustomerName)
	if customerName == "" || input.PartySize <= 0 {
		return nil, apperror.ErrInvalidBooking
	}

	payload := BookingHashPayload{
		CustomerName: customerName,
		BookingDate:  input.BookingDate.Format("2006-01-02"),
		BookingTime:  input.BookingTime.Format("15:04:05"),
		PartySize:    input.PartySize,
	}

	requestHash, err := hasher.JSON(payload)
	if err != nil {
		return nil, fmt.Errorf("hash booking request: %w", err)
	}

	existingOperation, err := s.operationsRepo.GetOperationByIdempotencyKey(ctx, key)
	if err == nil {
		if existingOperation.RequestHash.String != requestHash {
			return nil, apperror.ErrIdempotencyConflict
		}

		return s.handleExistingOperation(existingOperation)
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("check existing operation: %w", err)
	}

	operation, err := s.operationsRepo.CreateOperation(ctx, key, requestHash)
	if err != nil {
		return nil, fmt.Errorf("create operation: %w", err)
	}

	booking, err := s.bookingsRepo.CreateBooking(ctx, repository.CreateBookingParams{
		OperationID:  operation.ID,
		CustomerName: input.CustomerName,
		BookingDate:  toPGDate(input.BookingDate),
		BookingTime:  toPGTime(input.BookingTime),
		PartySize:    int32(input.PartySize),
	})

	if err != nil {
		return nil, fmt.Errorf("create booking: %w", err)
	}
	result := mapBookingToResult(operation, booking)

	bytes, err := json.Marshal(result)

	if err != nil {
		return nil, fmt.Errorf("marshal booking result: %w", err)
	}

	_, err = s.operationsRepo.UpdateOperationStatus(ctx, operation.ID, result.Status, bytes)

	if err != nil {
		return nil, fmt.Errorf("save completed operation: %w", err)
	}

	return result, nil

}

func (s *BookingService) handleExistingOperation(existingOperation sqlc.Operation) (*CreateBookingResult, error) {
	switch strings.ToUpper(existingOperation.Status) {
	case "COMPLETED":
		return decodeOperationResult(existingOperation)

	case "PENDING":
		return nil, apperror.ErrOperationInProgress

	case "FAILED":
		return nil, apperror.ErrOperationFailed

	default:
		return nil, fmt.Errorf("unknown operation status %q", existingOperation.Status)
	}
}

func mapBookingToResult(operation sqlc.Operation, booking sqlc.Booking) *CreateBookingResult {
	return &CreateBookingResult{
		OperationID:  operation.ID.String(),
		BookingID:    booking.ID.String(),
		Status:       "COMPLETED",
		CustomerName: booking.CustomerName,
		BookingDate:  fromPGDate(booking.BookingDate),
		BookingTime:  fromPGTime(booking.BookingTime),
		PartySize:    int(booking.PartySize),
	}
}


func decodeOperationResult(operation sqlc.Operation) (*CreateBookingResult, error) {
	var result CreateBookingResult

	if err := json.Unmarshal(operation.Result, &result); err != nil {
		return nil, fmt.Errorf("decode operation result: %w", err)
	}

	return &result, nil
}
