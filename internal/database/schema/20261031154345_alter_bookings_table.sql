-- +goose Up
ALTER TABLE bookings ADD COLUMN phone_no VARCHAR(20) NOT NULL;

CREATE INDEX IF NOT EXISTS idx_phone_number ON bookings(phone_no);

-- +goose Down
DROP INDEX IF EXISTS idx_phone_number;
ALTER TABLE bookings DROP COLUMN IF EXISTS phone_no;
