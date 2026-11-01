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

-- name: GetPotentialDuplicateBooking :one
SELECT *
FROM bookings
WHERE phone_no = $1
  AND booking_date = $2
  AND booking_time = $3
ORDER BY created_at DESC
LIMIT 1;

-- name: LockBookingSlot :exec
SELECT pg_advisory_xact_lock(hashtextextended($1, 0));