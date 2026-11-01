package apperror

import (
	"errors"
	"net/http"
)

var (
	ErrMissingIdempotencyKey = errors.New("idempotency key is required")
	ErrInvalidBooking        = errors.New("invalid booking details")
	ErrInvalidInput          = errors.New("invalid input")
	ErrIdempotencyConflict   = errors.New("idempotency key reused with different request")
	ErrOperationInProgress   = errors.New("operation is still in progress")
	ErrOperationFailed       = errors.New("operation failed")
)

// HTTPStatus maps known domain errors to their appropriate HTTP status codes.
func HTTPStatus(err error) int {
	var dupErr *PotentialDuplicateBookingError
	switch {
	case errors.Is(err, ErrMissingIdempotencyKey):
		return http.StatusBadRequest // 400
	case errors.Is(err, ErrInvalidBooking):
		return http.StatusBadRequest // 400
	case errors.Is(err, ErrInvalidInput):
		return http.StatusBadRequest // 400
	case errors.Is(err, ErrIdempotencyConflict):
		return http.StatusConflict // 409
	case errors.Is(err, ErrOperationInProgress):
		return http.StatusConflict // 409
	case errors.As(err, &dupErr):
		return http.StatusConflict // 409
	case errors.Is(err, ErrOperationFailed):
		return http.StatusInternalServerError // 500
	default:
		return http.StatusInternalServerError // 500
	}
}

type PotentialDuplicateBookingError struct {
	BookingDate string
	BookingTime string
	PartySize   int
}

func (e *PotentialDuplicateBookingError) Error() string {
	return "potential duplicate booking detected"
}
