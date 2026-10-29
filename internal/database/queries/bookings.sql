-- name: CreateBooking :one
INSERT INTO bookings (
    operation_id,
    customer_name,
    booking_date,
    booking_time,
    party_size
) VALUES (
    $1,
    $2,
    $3,
    $4,
    $5
)
RETURNING *;

-- name: GetBookingByOperationID :one
SELECT * FROM bookings
WHERE operation_id = $1;
