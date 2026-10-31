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
	case errors.Is(err, ErrOperationFailed):
		return http.StatusInternalServerError // 500
	default:
		return http.StatusInternalServerError // 500
	}
}
