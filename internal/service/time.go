package service

import (
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// toPGDate converts a Go time.Time to pgtype.Date.
func toPGDate(value time.Time) pgtype.Date {
	if value.IsZero() {
		return pgtype.Date{Valid: false}
	}
	return pgtype.Date{
		Time:  value,
		Valid: true,
	}
}

// toPGTime converts a Go time.Time to pgtype.Time (microseconds since midnight).
func toPGTime(value time.Time) pgtype.Time {
	if value.IsZero() {
		return pgtype.Time{Valid: false}
	}

	hour, min, sec := value.Clock()
	nsec := value.Nanosecond()
	microseconds := int64(hour*3600+min*60+sec)*1_000_000 + int64(nsec/1_000)

	return pgtype.Time{
		Microseconds: microseconds,
		Valid:        true,
	}
}

// fromPGDate formats a pgtype.Date into "YYYY-MM-DD" string.
func fromPGDate(value pgtype.Date) string {
	if !value.Valid {
		return ""
	}
	return value.Time.Format("2006-01-02")
}

// fromPGTime formats a pgtype.Time into "HH:MM:SS" string.
func fromPGTime(value pgtype.Time) string {
	if !value.Valid {
		return ""
	}

	totalSeconds := value.Microseconds / 1_000_000
	hours := totalSeconds / 3600
	minutes := (totalSeconds % 3600) / 60
	seconds := totalSeconds % 60

	return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
}
