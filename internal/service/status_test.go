package service

import (
	"testing"
)

func TestIsValidStatus(t *testing.T) {
	tests := []struct {
		name     string
		status   string
		expected bool
	}{
		{
			name:     "valid uppercase pending",
			status:   StatusPending,
			expected: true,
		},
		{
			name:     "valid uppercase completed",
			status:   StatusCompleted,
			expected: true,
		},
		{
			name:     "valid uppercase failed",
			status:   StatusFailed,
			expected: true,
		},
		{
			name:     "valid lowercase pending with spaces",
			status:   "  pending  ",
			expected: true,
		},
		{
			name:     "valid mixed case completed",
			status:   "Completed",
			expected: true,
		},
		{
			name:     "invalid unknown status",
			status:   "PROCESSING",
			expected: false,
		},
		{
			name:     "invalid random string",
			status:   "CANCELLED",
			expected: false,
		},
		{
			name:     "invalid empty string",
			status:   "",
			expected: false,
		},
		{
			name:     "invalid whitespace string",
			status:   "   ",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsValidStatus(tt.status)
			if got != tt.expected {
				t.Errorf("IsValidStatus(%q) = %v; want %v", tt.status, got, tt.expected)
			}
		})
	}
}
