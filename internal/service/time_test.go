package service

import (
	"testing"
	"time"
)

func TestDateConversions(t *testing.T) {
	parsedTime, err := time.Parse("2006-01-02", "2026-10-30")
	if err != nil {
		t.Fatalf("failed to parse test date: %v", err)
	}

	pgDate := toPGDate(parsedTime)
	if !pgDate.Valid {
		t.Fatalf("expected pgDate to be valid")
	}

	dateStr := fromPGDate(pgDate)
	if dateStr != "2026-10-30" {
		t.Fatalf("expected 2026-10-30, got %s", dateStr)
	}

	// Test zero / invalid
	invalidDate := toPGDate(time.Time{})
	if invalidDate.Valid {
		t.Fatalf("expected zero time to produce invalid pgtype.Date")
	}
	if fromPGDate(invalidDate) != "" {
		t.Fatalf("expected empty string for invalid pgtype.Date")
	}
}

func TestTimeConversions(t *testing.T) {
	parsedTime, err := time.Parse("15:04:05", "18:30:45")
	if err != nil {
		t.Fatalf("failed to parse test time: %v", err)
	}

	pgTime := toPGTime(parsedTime)
	if !pgTime.Valid {
		t.Fatalf("expected pgTime to be valid")
	}

	timeStr := fromPGTime(pgTime)
	if timeStr != "18:30:45" {
		t.Fatalf("expected 18:30:45, got %s", timeStr)
	}

	// Test zero / invalid
	invalidTime := toPGTime(time.Time{})
	if invalidTime.Valid {
		t.Fatalf("expected zero time to produce invalid pgtype.Time")
	}
	if fromPGTime(invalidTime) != "" {
		t.Fatalf("expected empty string for invalid pgtype.Time")
	}
}
