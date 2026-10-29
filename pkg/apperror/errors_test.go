package apperror_test

import (
	"errors"
	"net/http"
	"testing"

	"github.com/NIROOZbx/voiceflow-guard/pkg/apperror"
)

func TestHTTPStatus(t *testing.T) {
	tests := []struct {
		err            error
		expectedStatus int
	}{
		{apperror.ErrMissingIdempotencyKey, http.StatusBadRequest},
		{apperror.ErrInvalidBooking, http.StatusBadRequest},
		{apperror.ErrIdempotencyConflict, http.StatusConflict},
		{apperror.ErrOperationInProgress, http.StatusConflict},
		{apperror.ErrOperationFailed, http.StatusInternalServerError},
		{errors.New("unknown error"), http.StatusInternalServerError},
	}

	for _, tt := range tests {
		status := apperror.HTTPStatus(tt.err)
		if status != tt.expectedStatus {
			t.Errorf("for error %v, expected status %d, got %d", tt.err, tt.expectedStatus, status)
		}
	}
}
