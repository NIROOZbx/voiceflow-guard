package repository

import (
	"context"

	"github.com/NIROOZbx/voiceflow-guard/internal/database/sqlc"
	"github.com/jackc/pgx/v5/pgtype"
)

type BookingsRepo struct {
	db *sqlc.Queries
}

type CreateBookingParams struct {
	OperationID  pgtype.UUID
	CustomerName string
	PhoneNo      string
	BookingDate  pgtype.Date
	BookingTime  pgtype.Time
	PartySize    int32
}

type BookingsRepository interface {
	CreateBooking(ctx context.Context, params CreateBookingParams) (sqlc.Booking, error)
	GetBookingByOperationID(ctx context.Context, operationID pgtype.UUID) (sqlc.Booking, error)
	ListBooking(ctx context.Context, phoneNumber string, bookingDate pgtype.Date) ([]sqlc.Booking, error)
}

func NewBookingsRepo(db *sqlc.Queries) *BookingsRepo {
	return &BookingsRepo{
		db: db,
	}
}

func (b *BookingsRepo) CreateBooking(ctx context.Context,params CreateBookingParams,) (sqlc.Booking, error) {
	booking, err := b.db.CreateBooking(ctx, sqlc.CreateBookingParams{
		OperationID:  params.OperationID,
		CustomerName: params.CustomerName,
		PhoneNo:      params.PhoneNo,
		BookingDate:  params.BookingDate,
		BookingTime:  params.BookingTime,
		PartySize:    params.PartySize,
	})
	if err != nil {
		return sqlc.Booking{}, err
	}

	return booking, nil
}

func (b *BookingsRepo) GetBookingByOperationID(ctx context.Context,operationID pgtype.UUID) (sqlc.Booking, error) {
	booking, err := b.db.GetBookingByOperationID(ctx, operationID)
	if err != nil {
		return sqlc.Booking{}, err
	}

	return booking, nil
}

func (b *BookingsRepo) ListBooking(ctx context.Context, phoneNumber string, bookingDate pgtype.Date) ([]sqlc.Booking, error) {
	if bookingDate.Valid {
		return b.db.ListBookingsByPhoneAndDate(ctx, sqlc.ListBookingsByPhoneAndDateParams{
			PhoneNo:     phoneNumber,
			BookingDate: bookingDate,
		})
	}

	return b.db.ListBookingsByPhone(ctx, phoneNumber)
}