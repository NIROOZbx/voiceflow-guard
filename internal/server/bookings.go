package server

import (
	"context"
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
	CustomerName string `json:"customer_name" binding:"required"`
	PartySize    int    `json:"party_size" binding:"required,gt=0"`
	BookingDate  string `json:"booking_date" binding:"required"`
	BookingTime  string `json:"booking_time" binding:"required"`
	PhoneNumber string `json:"phone_no" binding:"required"`
}

func (h *BookingHandler) CreateBooking(c *gin.Context) {
	var req CreateBookingRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	idempotencyKey := strings.TrimSpace(
		c.GetHeader("Idempotency-Key"),
	)

	if idempotencyKey == "" || len(idempotencyKey) > 255 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Idempotency-Key header is required and must not exceed 255 characters",
		})
		return
	}

	// 2. Parse Date (format: YYYY-MM-DD)
	bookingDate, err := time.Parse("2006-01-02", req.BookingDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid booking_date format, expected YYYY-MM-DD"})
		return
	}

	// 3. Parse Time (supports HH:MM or HH:MM:SS)
	bookingTime, err := time.Parse("15:04:05", req.BookingTime)
	if err != nil {
		bookingTime, err = time.Parse("15:04", req.BookingTime)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid booking_time format, expected HH:MM or HH:MM:SS"})
			return
		}
	}

	// 4. Delegate to Service
	result, err := h.bookingService.CreateBooking(c.Request.Context(), service.CreateBookingInput{
		IdempotencyKey: idempotencyKey,
		CustomerName:   req.CustomerName,
		PartySize:      req.PartySize,
		BookingDate:    bookingDate,
		BookingTime:    bookingTime,
		PhoneNumber: req.PhoneNumber,

	})
	if err != nil {
		status := apperror.HTTPStatus(err)
		if status == http.StatusInternalServerError {
			c.JSON(status, gin.H{"error": "operation failed"})
			return
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, result)
}

func (h *BookingHandler) GetBooking(c *gin.Context) {
	phoneNumber := strings.TrimSpace(c.Query("phone_no"))
	if phoneNumber == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "phone_no query parameter is required"})
		return
	}

	var parsedDate *time.Time
	if bookingDate := strings.TrimSpace(c.Query("date")); bookingDate != "" {
		date, err := time.Parse("2006-01-02", bookingDate)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid booking_date format, expected YYYY-MM-DD"})
			return
		}
		parsedDate = &date
	}

	bookings, err := h.bookingService.ListBooking(c.Request.Context(), phoneNumber, parsedDate)
	if err != nil {
		status := apperror.HTTPStatus(err)
		if status == http.StatusInternalServerError {
			c.JSON(status, gin.H{"error": "operation failed"})
			return
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, bookings)
}