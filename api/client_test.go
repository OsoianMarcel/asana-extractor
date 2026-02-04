package api

import (
	"io"
	"log/slog"
	"testing"
	"time"
)

func TestParseRetryAfter(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		minTime time.Duration
		maxTime time.Duration
	}{
		{
			name:    "seconds format",
			input:   "30",
			minTime: 29 * time.Second,
			maxTime: 31 * time.Second,
		},
		{
			name:    "empty string defaults to 60",
			input:   "",
			minTime: 59 * time.Second,
			maxTime: 61 * time.Second,
		},
		{
			name:    "invalid format defaults to 60",
			input:   "invalid",
			minTime: 59 * time.Second,
			maxTime: 61 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseRetryAfter(tt.input)
			if result < tt.minTime || result > tt.maxTime {
				t.Errorf("parseRetryAfter(%q) = %v, want between %v and %v", tt.input, result, tt.minTime, tt.maxTime)
			}
		})
	}
}

func TestIsRateLimitError(t *testing.T) {
	rle := &RateLimitError{
		RetryAfter: 30 * time.Second,
		Message:    "rate limited",
	}

	rateLimitErr, ok := IsRateLimitError(rle)
	if !ok {
		t.Errorf("IsRateLimitError should return true for RateLimitError")
	}
	if rateLimitErr.RetryAfter != 30*time.Second {
		t.Errorf("IsRateLimitError returned wrong RetryAfter: %v", rateLimitErr.RetryAfter)
	}

	_, ok = IsRateLimitError(nil)
	if ok {
		t.Errorf("IsRateLimitError should return false for nil")
	}
}

func TestNewClient(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	client := NewClient("test-token", nil, logger)

	if client.token != "test-token" {
		t.Errorf("NewClient didn't set token correctly")
	}
	if client.httpClient == nil {
		t.Errorf("NewClient didn't initialize httpClient")
	}
	if client.logger == nil {
		t.Errorf("NewClient didn't set logger")
	}
}
