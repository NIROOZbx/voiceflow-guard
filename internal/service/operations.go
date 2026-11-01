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
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	StatusPending   = "PENDING"
	StatusCompleted = "COMPLETED"
	StatusFailed    = "FAILED"
)

// IsValidStatus validates if the provided status is one of the allowed operation statuses.
func IsValidStatus(status string) bool {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case StatusPending, StatusCompleted, StatusFailed:
		return true
	default:
		return false
	}
}

type BookingService struct {
	operationsRepo repository.OperationsRepository
	TxManager      repository.TransactionManager
	bookingsRepo   repository.BookingsRepository
}
type CreateBookingInput struct {
	IdempotencyKey            string
	CustomerName              string
	PartySize                 int
	BookingDate               time.Time
	BookingTime               time.Time
	PhoneNumber               string
	ConfirmPotentialDuplicate bool
}

type BookingHashPayload struct {
	CustomerName              string `json:"customer_name"`
	BookingDate               string `json:"booking_date"`
	BookingTime               string `json:"booking_time"`
	PartySize                 int    `json:"party_size"`
	PhoneNo                   string `json:"phone_no"`
	ConfirmPotentialDuplicate bool   `json:"confirm_potential_duplicate"`
}

type CreateBookingResult struct {
	OperationID  string `json:"operation_id"`
	BookingID    string `json:"booking_id"`
	Status       string `json:"status"`
	CustomerName string `json:"customer_name"`
	BookingDate  string `json:"booking_date"`
	BookingTime  string `json:"booking_time"`
	PartySize    int    `json:"party_size"`
	PhoneNumber  string `json:"phone_no"`
}

func NewOpsService(opsRepo repository.OperationsRepository, txManager repository.TransactionManager, bookingsRepo repository.BookingsRepository) *BookingService {
	return &BookingService{
		operationsRepo: opsRepo,
		bookingsRepo:   bookingsRepo,
		TxManager:      txManager,
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
		CustomerName:              customerName,
		BookingDate:               input.BookingDate.Format("2006-01-02"),
		BookingTime:               input.BookingTime.Format("15:04:05"),
		PartySize:                 input.PartySize,
		PhoneNo:                   input.PhoneNumber,
		ConfirmPotentialDuplicate: input.ConfirmPotentialDuplicate,
	}

	requestHash, err := hasher.JSON(payload)
	if err != nil {
		return nil, apperror.ErrOperationFailed
	}

	existingOperation, err := s.operationsRepo.GetOperationByIdempotencyKey(ctx, key)
	if err == nil {
		if existingOperation.RequestHash.String != requestHash {
			return nil, apperror.ErrIdempotencyConflict
		}

		return s.handleExistingOperation(existingOperation)
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, apperror.ErrOperationFailed
	}

	var result CreateBookingResult

	err = s.TxManager.WithinTransaction(ctx, func(r *repository.Repositories) error {

		// Create operation
		operation, err := r.Operations.CreateOperation(ctx, key, requestHash)
		if err != nil {
			return fmt.Errorf("create operation: %w", err)
		}

		slotKey := fmt.Sprintf(
			"%s|%s|%s",
			input.PhoneNumber,
			input.BookingDate.Format("2006-01-02"),
			input.BookingTime.Format("15:04:05"),
		)

		if err := r.Bookings.LockBookingSlot(ctx, slotKey); err != nil {
			return fmt.Errorf("lock booking slot: %w", err)
		}

		if err := DuplicateCheck(ctx, input, r.Bookings); err != nil {
			return err
		}

		// Create booking

		booking, err := r.Bookings.CreateBooking(ctx, repository.CreateBookingParams{
			OperationID:  operation.ID,
			CustomerName: customerName,
			PhoneNo:      input.PhoneNumber,
			BookingDate:  toPGDate(input.BookingDate),
			BookingTime:  toPGTime(input.BookingTime),
			PartySize:    int32(input.PartySize),
		})

		if err != nil {
			return fmt.Errorf("create booking: %w", err)
		}

		// Build result
		result = mapBookingToResult(booking)

		// Validate status check
		if !IsValidStatus(result.Status) {
			return fmt.Errorf("invalid operation status: %s", result.Status)
		}

		// Serialize result
		bytes, err := json.Marshal(result)

		if err != nil {
			return fmt.Errorf("marshal booking result: %w", err)
		}
		// Mark operation completed
		_, err = r.Operations.UpdateOperationStatus(ctx, operation.ID, result.Status, bytes)

		if err != nil {
			return fmt.Errorf("save completed operation: %w", err)
		}
		return nil
	})

	if err != nil {
		var duplicateErr *apperror.PotentialDuplicateBookingError

		if errors.As(err, &duplicateErr) {
			return nil, duplicateErr
		}
		if isIdempotencyKeyConflict(err) {
			// Use the normal repository, NOT the failed transaction.
			existing, lookupErr := s.operationsRepo.GetOperationByIdempotencyKey(
				ctx,
				key,
			)
			if lookupErr != nil {
				return nil, apperror.ErrOperationFailed
			}

			// Check whether this is actually the same request.
			if !existing.RequestHash.Valid ||
				existing.RequestHash.String != requestHash {
				return nil, apperror.ErrIdempotencyConflict
			}

			// Same request: return the original result.
			return s.handleExistingOperation(existing)
		}

		return nil, apperror.ErrOperationFailed
	}

	return &result, nil

}

func (s *BookingService) handleExistingOperation(existingOperation sqlc.Operation) (*CreateBookingResult, error) {
	status := strings.ToUpper(strings.TrimSpace(existingOperation.Status))
	if !IsValidStatus(status) {
		return nil, apperror.ErrOperationFailed
	}

	switch status {
	case StatusCompleted:
		return decodeOperationResult(existingOperation)

	case StatusPending:
		return nil, apperror.ErrOperationInProgress

	case StatusFailed:
		return nil, apperror.ErrOperationFailed

	default:
		return nil, apperror.ErrOperationFailed
	}
}

func (s *BookingService) ListBooking(ctx context.Context, phoneNumber string, bookingDate *time.Time) ([]CreateBookingResult, error) {
	phone := strings.TrimSpace(phoneNumber)
	if phone == "" {
		return nil, apperror.ErrInvalidInput
	}
	var pgDate pgtype.Date
	if bookingDate != nil {
		pgDate = toPGDate(*bookingDate)
	}
	bookings, err := s.bookingsRepo.ListBooking(ctx, phone, pgDate)
	if err != nil {
		return nil, apperror.ErrOperationFailed
	}
	results := make([]CreateBookingResult, len(bookings))
	for i, b := range bookings {
		results[i] = mapBookingToResult(b)
	}
	return results, nil
}

// Helpers
func mapBookingToResult(booking sqlc.Booking) CreateBookingResult {
	return CreateBookingResult{
		OperationID:  booking.OperationID.String(),
		BookingID:    booking.ID.String(),
		Status:       StatusCompleted,
		CustomerName: booking.CustomerName,
		PhoneNumber:  booking.PhoneNo,
		BookingDate:  fromPGDate(booking.BookingDate),
		BookingTime:  fromPGTime(booking.BookingTime),
		PartySize:    int(booking.PartySize),
	}
}

func decodeOperationResult(operation sqlc.Operation) (*CreateBookingResult, error) {
	var result CreateBookingResult

	if err := json.Unmarshal(operation.Result, &result); err != nil {
		return nil, apperror.ErrOperationFailed
	}

	return &result, nil
}

func isIdempotencyKeyConflict(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return pgErr.ConstraintName == "" || strings.Contains(pgErr.ConstraintName, "idempotency_key")
	}
	return false
}

func DuplicateCheck(ctx context.Context, input CreateBookingInput, r repository.BookingsRepository) error {

	if input.ConfirmPotentialDuplicate {
		return nil
	}

	existingBooking, err := r.GetPotentialDuplicate(
		ctx,
		repository.DuplicateParams{
			PhoneNo:     input.PhoneNumber,
			BookingDate: toPGDate(input.BookingDate),
			BookingTime: toPGTime(input.BookingTime),
		},
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}

	if err != nil {
		return fmt.Errorf("check potential duplicate: %w", err)
	}

	return &apperror.PotentialDuplicateBookingError{
		BookingDate: fromPGDate(existingBooking.BookingDate),
		BookingTime: fromPGTime(existingBooking.BookingTime),
		PartySize:   int(existingBooking.PartySize),
	}

}
