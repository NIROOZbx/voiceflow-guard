-- name: CreateBooking :one
INSERT INTO bookings (
    operation_id,
    customer_name,
    phone_no,
    booking_date,
    booking_time,
    party_size
) VALUES (
    $1,
    $2,
    $3,
    $4,
    $5,
    $6
)
RETURNING *;


-- name: GetBookingByOperationID :one
SELECT * FROM bookings
WHERE operation_id = $1;


-- name: ListBookingsByPhone :many
SELECT *
FROM bookings
WHERE phone_no = $1
ORDER BY booking_date ASC, booking_time ASC;

-- name: ListBookingsByPhoneAndDate :many
SELECT *
FROM bookings
WHERE phone_no = $1
  AND booking_date = $2
ORDER BY booking_time ASC;