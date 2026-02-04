package limiter

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestNewLimiter(t *testing.T) {
	logger := testLogger()
	l := NewLimiter(150, "", logger)

	if l.tokenBucket == nil {
		t.Error("tokenBucket should not be nil")
	}
	if l.concurrent == nil {
		t.Error("concurrent should not be nil")
	}
	if l.tokenBucket.rate != 150 {
		t.Errorf("expected rate 150, got %d", l.tokenBucket.rate)
	}
}

func TestTokenBucketLimiter_Wait(t *testing.T) {
	logger := testLogger()
	// Use high rate so we don't wait
	tbl := NewTokenBucketLimiter(1000, logger)

	ctx := context.Background()
	err := tbl.Wait(ctx)
	if err != nil {
		t.Errorf("Wait failed: %v", err)
	}

	// Tokens should have decreased
	if tbl.tokens >= 1000 {
		t.Error("tokens should have decreased after Wait")
	}
}

func TestTokenBucketLimiter_WaitContextCancelled(t *testing.T) {
	logger := testLogger()
	// Use very low rate to force waiting
	tbl := NewTokenBucketLimiter(1, logger)
	tbl.tokens = 0 // Force empty bucket

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := tbl.Wait(ctx)
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestConcurrentLimiter_GetSlot(t *testing.T) {
	logger := testLogger()
	cl := NewConcurrentLimiter(2, 1, logger)

	ctx := context.Background()

	// Acquire first slot
	err := cl.WaitForGetSlot(ctx)
	if err != nil {
		t.Fatalf("WaitForGetSlot failed: %v", err)
	}
	if cl.activeGETs != 1 {
		t.Errorf("expected activeGETs=1, got %d", cl.activeGETs)
	}

	// Acquire second slot
	err = cl.WaitForGetSlot(ctx)
	if err != nil {
		t.Fatalf("WaitForGetSlot failed: %v", err)
	}
	if cl.activeGETs != 2 {
		t.Errorf("expected activeGETs=2, got %d", cl.activeGETs)
	}

	// Release one slot
	cl.ReleaseGetSlot()
	if cl.activeGETs != 1 {
		t.Errorf("expected activeGETs=1 after release, got %d", cl.activeGETs)
	}

	// Release second slot
	cl.ReleaseGetSlot()
	if cl.activeGETs != 0 {
		t.Errorf("expected activeGETs=0 after release, got %d", cl.activeGETs)
	}
}

func TestConcurrentLimiter_WriteSlot(t *testing.T) {
	logger := testLogger()
	cl := NewConcurrentLimiter(50, 2, logger)

	ctx := context.Background()

	// Acquire write slots
	err := cl.WaitForWriteSlot(ctx)
	if err != nil {
		t.Fatalf("WaitForWriteSlot failed: %v", err)
	}
	if cl.activeWrites != 1 {
		t.Errorf("expected activeWrites=1, got %d", cl.activeWrites)
	}

	cl.ReleaseWriteSlot()
	if cl.activeWrites != 0 {
		t.Errorf("expected activeWrites=0 after release, got %d", cl.activeWrites)
	}
}

func TestLimiter_WaitForGet(t *testing.T) {
	logger := testLogger()
	l := NewLimiter(1000, "", logger)

	ctx := context.Background()
	err := l.WaitForGet(ctx)
	if err != nil {
		t.Fatalf("WaitForGet failed: %v", err)
	}

	// Should have acquired a concurrent slot
	if l.concurrent.activeGETs != 1 {
		t.Errorf("expected activeGETs=1, got %d", l.concurrent.activeGETs)
	}

	l.ReleaseGet()
	if l.concurrent.activeGETs != 0 {
		t.Errorf("expected activeGETs=0 after release, got %d", l.concurrent.activeGETs)
	}
}

func TestLimiter_SaveAndLoadState(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "rate_limiter_state.json")
	logger := testLogger()

	// Create limiter and consume some tokens
	l := NewLimiter(150, statePath, logger)
	l.tokenBucket.tokens = 100.5

	// Save state
	err := l.SaveState()
	if err != nil {
		t.Fatalf("SaveState failed: %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(statePath); os.IsNotExist(err) {
		t.Fatal("state file was not created")
	}

	// Create new limiter and load state
	l2 := NewLimiter(150, statePath, logger)

	// Tokens should be restored (with some refill from elapsed time)
	if l2.tokenBucket.tokens < 100 {
		t.Errorf("expected tokens >= 100, got %f", l2.tokenBucket.tokens)
	}
}

func TestLimiter_LoadStateRateMismatch(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "rate_limiter_state.json")
	logger := testLogger()

	// Create limiter with rate 150
	l := NewLimiter(150, statePath, logger)
	l.tokenBucket.tokens = 50

	err := l.SaveState()
	if err != nil {
		t.Fatalf("SaveState failed: %v", err)
	}

	// Create new limiter with different rate - should ignore saved state
	l2 := NewLimiter(1500, statePath, logger)

	// Should have full tokens (1500) because rate changed
	if l2.tokenBucket.tokens != 1500 {
		t.Errorf("expected tokens=1500 (rate mismatch), got %f", l2.tokenBucket.tokens)
	}
}

func TestLimiter_EmptyStatePath(t *testing.T) {
	logger := testLogger()
	l := NewLimiter(150, "", logger)

	// Should not error with empty state path
	err := l.SaveState()
	if err != nil {
		t.Errorf("SaveState with empty path should not error: %v", err)
	}

	err = l.LoadState()
	if err != nil {
		t.Errorf("LoadState with empty path should not error: %v", err)
	}
}

func TestExponentialBackoff(t *testing.T) {
	tests := []struct {
		attempt int
		maxWait time.Duration
		minWait time.Duration
	}{
		{attempt: 0, maxWait: 30 * time.Second, minWait: 0},
		{attempt: 1, maxWait: 30 * time.Second, minWait: 1 * time.Second},
		{attempt: 2, maxWait: 30 * time.Second, minWait: 2 * time.Second},
		{attempt: 3, maxWait: 30 * time.Second, minWait: 4 * time.Second},
		{attempt: 10, maxWait: 30 * time.Second, minWait: 30 * time.Second}, // capped at maxWait
	}

	for _, tt := range tests {
		result := ExponentialBackoff(tt.attempt, tt.maxWait)
		if result < tt.minWait {
			t.Errorf("ExponentialBackoff(%d, %v) = %v, want >= %v", tt.attempt, tt.maxWait, result, tt.minWait)
		}
		// With jitter, max is 2x the base (capped at maxWait)
		maxExpected := 2 * tt.maxWait
		if result > maxExpected {
			t.Errorf("ExponentialBackoff(%d, %v) = %v, want <= %v", tt.attempt, tt.maxWait, result, maxExpected)
		}
	}
}

func TestTokenBucketLimiter_Refill(t *testing.T) {
	logger := testLogger()
	tbl := NewTokenBucketLimiter(60, logger) // 60 per minute = 1 per second
	tbl.tokens = 0
	tbl.lastRefillTime = time.Now().Add(-2 * time.Second)

	tbl.mu.Lock()
	tbl.refillTokens()
	tbl.mu.Unlock()

	// Should have refilled ~2 tokens (60/minute = 1/second, 2 seconds elapsed)
	if tbl.tokens < 1.5 || tbl.tokens > 2.5 {
		t.Errorf("expected ~2 tokens after 2 second refill, got %f", tbl.tokens)
	}
}

func TestTokenBucketLimiter_MaxTokens(t *testing.T) {
	logger := testLogger()
	tbl := NewTokenBucketLimiter(100, logger)
	tbl.tokens = 100
	tbl.lastRefillTime = time.Now().Add(-10 * time.Minute) // Long time ago

	tbl.mu.Lock()
	tbl.refillTokens()
	tbl.mu.Unlock()

	// Should not exceed maxTokens
	if tbl.tokens > tbl.maxTokens {
		t.Errorf("tokens %f exceeded maxTokens %f", tbl.tokens, tbl.maxTokens)
	}
}
