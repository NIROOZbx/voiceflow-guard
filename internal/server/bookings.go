package server

import (
	"context"
	"errors"
	"log"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/NIROOZbx/voiceflow-guard/internal/service"
	"github.com/NIROOZbx/voiceflow-guard/pkg/apperror"
	"github.com/gin-gonic/gin"
)

type BookingService interface {
	CreateBooking(ctx context.Context, input service.CreateBookingInput) (*service.CreateBookingResult, error)
	ListBooking(ctx context.Context, phoneNumber string, bookingDate *time.Time) ([]service.CreateBookingResult, error)
}

type BookingHandler struct {
	bookingService BookingService
}

func NewBookingHandler(bookingService BookingService) *BookingHandler {
	return &BookingHandler{
		bookingService: bookingService,
	}
}

type CreateBookingRequest struct {
	CustomerName              string  `json:"customer_name" binding:"required"`
	PartySize                 float64 `json:"party_size" binding:"required,gt=0"`
	BookingDate               string  `json:"booking_date" binding:"required"`
	BookingTime               string  `json:"booking_time" binding:"required"`
	PhoneNumber               string  `json:"phone_no" binding:"required"`
	ConfirmPotentialDuplicate bool    `json:"confirm_potential_duplicate"`
	IdempotencyKey            string  `json:"idempotency_key"`
}

func (h *BookingHandler) CreateBooking(c *gin.Context) {
	var req CreateBookingRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("[Handler CreateBooking] ERROR: Failed to bind JSON: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	idempotencyKey := strings.TrimSpace(
		c.GetHeader("Idempotency-Key"),
	)

	if idempotencyKey == "" {
		idempotencyKey = strings.TrimSpace(req.IdempotencyKey)
	}
	if idempotencyKey == "" || len(idempotencyKey) > 255 {
		log.Printf("[Handler CreateBooking] ERROR: Invalid or missing Idempotency-Key (len=%d)", len(idempotencyKey))
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Idempotency-Key is required and must not exceed 255 characters",
		})
		return
	}

	// 1. Validate: Is party_size a whole number?
	if req.PartySize != math.Trunc(req.PartySize) || req.PartySize <= 0 {
		log.Printf("[Handler CreateBooking] ERROR: party_size must be a positive whole number, got: %v", req.PartySize)
		c.JSON(http.StatusBadRequest, gin.H{"error": "party_size must be a positive whole number"})
		return
	}
	partySize := int(req.PartySize)

	// 2. Parse Date (format: YYYY-MM-DD)
	bookingDate, err := time.Parse("2006-01-02", req.BookingDate)
	if err != nil {
		log.Printf("[Handler CreateBooking] ERROR: Invalid booking_date format %q: %v", req.BookingDate, err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid booking_date format, expected YYYY-MM-DD"})
		return
	}

	// 3. Parse Time (supports HH:MM or HH:MM:SS)
	bookingTime, err := time.Parse("15:04:05", req.BookingTime)
	if err != nil {
		bookingTime, err = time.Parse("15:04", req.BookingTime)
		if err != nil {
			log.Printf("[Handler CreateBooking] ERROR: Invalid booking_time format %q: %v", req.BookingTime, err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid booking_time format, expected HH:MM or HH:MM:SS"})
			return
		}
	}

	log.Printf("[Handler CreateBooking] Processing booking for %q (party_size: %d, phone: %s, date: %s, time: %s, key: %s)",
		req.CustomerName, partySize, req.PhoneNumber, req.BookingDate, req.BookingTime, idempotencyKey)

	// 4. Delegate to Service
	result, err := h.bookingService.CreateBooking(c.Request.Context(), service.CreateBookingInput{
		IdempotencyKey:            idempotencyKey,
		CustomerName:              req.CustomerName,
		PartySize:                 partySize,
		BookingDate:               bookingDate,
		BookingTime:               bookingTime,
		PhoneNumber:               req.PhoneNumber,
		ConfirmPotentialDuplicate: req.ConfirmPotentialDuplicate,
	})
	if err != nil {
		log.Printf("[Handler CreateBooking] Service returned error: %v", err)
		var dupErr *apperror.PotentialDuplicateBookingError
		if errors.As(err, &dupErr) {
			log.Printf("[Handler CreateBooking] Potential duplicate booking detected for %s", req.PhoneNumber)
			c.JSON(http.StatusConflict, gin.H{
				"success":               false,
				"code":                  "POTENTIAL_DUPLICATE",
				"message":               dupErr.Error(),
				"requires_confirmation": true,
				"existing_booking": gin.H{
					"booking_date": dupErr.BookingDate,
					"booking_time": dupErr.BookingTime,
					"party_size":   dupErr.PartySize,
				},
			})
			return
		}

		status := apperror.HTTPStatus(err)
		if status == http.StatusInternalServerError {
			log.Printf("[Handler CreateBooking] Internal server error (status %d): %v", status, err)
			c.JSON(status, gin.H{"error": "operation failed"})
			return
		}
		log.Printf("[Handler CreateBooking] Client error (status %d): %v", status, err)
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	log.Printf("[Handler CreateBooking] SUCCESS: Booking created/resolved (booking_id: %s, op_id: %s, status: %s)",
		result.BookingID, result.OperationID, result.Status)
	c.JSON(http.StatusCreated, result)
}

func (h *BookingHandler) GetBooking(c *gin.Context) {
	phoneNumber := strings.TrimSpace(c.Query("phone_no"))
	if phoneNumber == "" {
		log.Println("[Handler GetBooking] ERROR: Missing phone_no query param")
		c.JSON(http.StatusBadRequest, gin.H{"error": "phone_no query parameter is required"})
		return
	}

	var parsedDate *time.Time
	if bookingDate := strings.TrimSpace(c.Query("date")); bookingDate != "" {
		date, err := time.Parse("2006-01-02", bookingDate)
		if err != nil {
			log.Printf("[Handler GetBooking] ERROR: Invalid date query param %q: %v", bookingDate, err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid booking_date format, expected YYYY-MM-DD"})
			return
		}
		parsedDate = &date
	}

	log.Printf("[Handler GetBooking] Listing bookings for phone: %s (date filter: %v)", phoneNumber, parsedDate)

	bookings, err := h.bookingService.ListBooking(c.Request.Context(), phoneNumber, parsedDate)
	if err != nil {
		log.Printf("[Handler GetBooking] Service error: %v", err)
		status := apperror.HTTPStatus(err)
		if status == http.StatusInternalServerError {
			c.JSON(status, gin.H{"error": "operation failed"})
			return
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	log.Printf("[Handler GetBooking] SUCCESS: Found %d bookings for phone %s", len(bookings), phoneNumber)
	c.JSON(http.StatusOK, bookings)
}
