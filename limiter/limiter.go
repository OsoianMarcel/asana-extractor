package limiter

import (
	"context"
	"encoding/json"
	"log/slog"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// TokenBucketLimiter implements a token bucket rate limiter.
// It limits requests based on a rate of tokens per minute.
type TokenBucketLimiter struct {
	rate           int       // tokens per minute
	tokens         float64   // current tokens
	maxTokens      float64   // maximum tokens in bucket
	lastRefillTime time.Time // last time we refilled tokens
	mu             sync.Mutex
	logger         *slog.Logger
}

// ConcurrentLimiter tracks concurrent requests by type.
type ConcurrentLimiter struct {
	getLimit       int // max concurrent GET requests
	writeLimit     int // max concurrent write requests (POST, PUT, PATCH, DELETE)
	activeGETs     int
	activeWrites   int
	mu             sync.Mutex
	getAvailable   *sync.Cond
	writeAvailable *sync.Cond
	logger         *slog.Logger
}

// Limiter combines both rate limiting strategies.
type Limiter struct {
	tokenBucket *TokenBucketLimiter
	concurrent  *ConcurrentLimiter
	statePath   string
	logger      *slog.Logger
}

// LimiterState represents the persisted state of the rate limiter.
type LimiterState struct {
	Tokens         float64   `json:"tokens"`
	LastRefillTime time.Time `json:"last_refill_time"`
	Rate           int       `json:"rate"`
}

// NewTokenBucketLimiter creates a new token bucket limiter.
func NewTokenBucketLimiter(requestsPerMinute int, logger *slog.Logger) *TokenBucketLimiter {
	return &TokenBucketLimiter{
		rate:           requestsPerMinute,
		tokens:         float64(requestsPerMinute),
		maxTokens:      float64(requestsPerMinute),
		lastRefillTime: time.Now(),
		logger:         logger,
	}
}

// NewConcurrentLimiter creates a new concurrent request limiter.
func NewConcurrentLimiter(getLimit, writeLimit int, logger *slog.Logger) *ConcurrentLimiter {
	cl := &ConcurrentLimiter{
		getLimit:   getLimit,
		writeLimit: writeLimit,
		logger:     logger,
	}
	cl.getAvailable = sync.NewCond(&cl.mu)
	cl.writeAvailable = sync.NewCond(&cl.mu)
	return cl
}

// NewLimiter creates a new combined rate limiter.
// rateLimitPerMin can be 150 (free tier) or 1500 (paid tier).
// statePath is the path to persist the limiter state (e.g., "./data/rate_limiter.json").
func NewLimiter(rateLimitPerMin int, statePath string, logger *slog.Logger) *Limiter {
	l := &Limiter{
		tokenBucket: NewTokenBucketLimiter(rateLimitPerMin, logger),
		concurrent:  NewConcurrentLimiter(50, 15, logger),
		statePath:   statePath,
		logger:      logger,
	}

	// Try to load existing state
	if err := l.LoadState(); err != nil {
		logger.Info("no existing rate limiter state found, starting fresh", "error", err)
	} else {
		logger.Info("loaded rate limiter state from disk",
			"tokens", l.tokenBucket.tokens,
			"last_refill", l.tokenBucket.lastRefillTime)
	}

	return l
}

// SaveState persists the current limiter state to disk.
func (l *Limiter) SaveState() error {
	if l.statePath == "" {
		return nil
	}

	l.tokenBucket.mu.Lock()
	state := LimiterState{
		Tokens:         l.tokenBucket.tokens,
		LastRefillTime: l.tokenBucket.lastRefillTime,
		Rate:           l.tokenBucket.rate,
	}
	l.tokenBucket.mu.Unlock()

	// Ensure directory exists
	dir := filepath.Dir(l.statePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	// Write to temp file first, then rename for atomicity
	tmpPath := l.statePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return err
	}

	return os.Rename(tmpPath, l.statePath)
}

// LoadState loads the limiter state from disk.
func (l *Limiter) LoadState() error {
	if l.statePath == "" {
		return nil
	}

	data, err := os.ReadFile(l.statePath)
	if err != nil {
		return err
	}

	var state LimiterState
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}

	l.tokenBucket.mu.Lock()
	defer l.tokenBucket.mu.Unlock()

	// Only restore if the rate matches (config might have changed)
	if state.Rate == l.tokenBucket.rate {
		// Calculate how many tokens should have been refilled since last save
		elapsed := time.Since(state.LastRefillTime)
		tokensToAdd := (elapsed.Minutes()) * float64(state.Rate)
		restoredTokens := math.Min(l.tokenBucket.maxTokens, state.Tokens+tokensToAdd)

		l.tokenBucket.tokens = restoredTokens
		l.tokenBucket.lastRefillTime = time.Now()

		l.logger.Info("rate limiter state restored",
			"saved_tokens", state.Tokens,
			"elapsed_since_save", elapsed,
			"restored_tokens", restoredTokens)
	} else {
		l.logger.Warn("rate limit config changed, ignoring saved state",
			"saved_rate", state.Rate,
			"current_rate", l.tokenBucket.rate)
	}

	return nil
}

// WaitForGetSlot waits until a GET request slot is available and reserves it.
func (cl *ConcurrentLimiter) WaitForGetSlot(ctx context.Context) error {
	cl.mu.Lock()
	defer cl.mu.Unlock()

	for {
		if cl.activeGETs < cl.getLimit {
			cl.activeGETs++
			cl.logger.Debug("get slot acquired", "active_gets", cl.activeGETs)
			return nil
		}

		// Wait for a slot to become available
		if err := waitWithContext(ctx, cl.getAvailable); err != nil {
			return err
		}
	}
}

// ReleaseGetSlot releases a GET request slot.
func (cl *ConcurrentLimiter) ReleaseGetSlot() {
	cl.mu.Lock()
	defer cl.mu.Unlock()

	if cl.activeGETs > 0 {
		cl.activeGETs--
		cl.logger.Debug("get slot released", "active_gets", cl.activeGETs)
		cl.getAvailable.Signal()
	}
}

// WaitForWriteSlot waits until a write request slot is available and reserves it.
func (cl *ConcurrentLimiter) WaitForWriteSlot(ctx context.Context) error {
	cl.mu.Lock()
	defer cl.mu.Unlock()

	for {
		if cl.activeWrites < cl.writeLimit {
			cl.activeWrites++
			cl.logger.Debug("write slot acquired", "active_writes", cl.activeWrites)
			return nil
		}

		// Wait for a slot to become available
		if err := waitWithContext(ctx, cl.writeAvailable); err != nil {
			return err
		}
	}
}

// ReleaseWriteSlot releases a write request slot.
func (cl *ConcurrentLimiter) ReleaseWriteSlot() {
	cl.mu.Lock()
	defer cl.mu.Unlock()

	if cl.activeWrites > 0 {
		cl.activeWrites--
		cl.logger.Debug("write slot released", "active_writes", cl.activeWrites)
		cl.writeAvailable.Signal()
	}
}

// Wait waits until tokens are available and consumes them.
func (tbl *TokenBucketLimiter) Wait(ctx context.Context) error {
	for {
		tbl.mu.Lock()
		tbl.refillTokens()

		if tbl.tokens >= 1.0 {
			tbl.tokens--
			tbl.mu.Unlock()
			tbl.logger.Debug("token consumed", "tokens_remaining", math.Floor(tbl.tokens))
			return nil
		}

		timeToWait := tbl.calculateWaitTime()
		tbl.mu.Unlock()

		// Wait or exit if context is done
		select {
		case <-time.After(timeToWait):
			// Try again
			continue
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// refillTokens refills the token bucket based on elapsed time.
func (tbl *TokenBucketLimiter) refillTokens() {
	now := time.Now()
	elapsed := now.Sub(tbl.lastRefillTime)
	tokensToAdd := (elapsed.Minutes()) * float64(tbl.rate)
	tbl.tokens = math.Min(tbl.maxTokens, tbl.tokens+tokensToAdd)
	tbl.lastRefillTime = now
}

// calculateWaitTime calculates how long to wait for the next token.
func (tbl *TokenBucketLimiter) calculateWaitTime() time.Duration {
	if tbl.tokens >= 1.0 {
		return 0
	}

	// How many tokens do we need?
	tokensNeeded := 1.0 - tbl.tokens
	// How many tokens are added per second?
	tokensPerSecond := float64(tbl.rate) / 60.0
	// How long to wait?
	secondsToWait := tokensNeeded / tokensPerSecond

	return time.Duration(secondsToWait*1000) * time.Millisecond
}

// Wait waits for a request slot to be available, respecting rate limits.
func (l *Limiter) WaitForGet(ctx context.Context) error {
	// First acquire concurrent slot
	if err := l.concurrent.WaitForGetSlot(ctx); err != nil {
		return err
	}

	// Then respect rate limit
	if err := l.tokenBucket.Wait(ctx); err != nil {
		l.concurrent.ReleaseGetSlot()
		return err
	}

	return nil
}

// ReleaseGet releases a GET request slot.
func (l *Limiter) ReleaseGet() {
	l.concurrent.ReleaseGetSlot()
}

// WaitForWrite waits for a write request slot to be available, respecting rate limits.
func (l *Limiter) WaitForWrite(ctx context.Context) error {
	// First acquire concurrent slot
	if err := l.concurrent.WaitForWriteSlot(ctx); err != nil {
		return err
	}

	// Then respect rate limit
	if err := l.tokenBucket.Wait(ctx); err != nil {
		l.concurrent.ReleaseWriteSlot()
		return err
	}

	return nil
}

// ReleaseWrite releases a write request slot.
func (l *Limiter) ReleaseWrite() {
	l.concurrent.ReleaseWriteSlot()
}

// ExponentialBackoff implements exponential backoff with jitter.
// It returns the duration to wait before retrying.
func ExponentialBackoff(attempt int, maxWait time.Duration) time.Duration {
	if attempt <= 0 {
		return 0
	}

	// Calculate exponential backoff: 2^(attempt-1) seconds
	baseWait := time.Duration(math.Pow(2, float64(attempt-1))) * time.Second

	// Cap at maxWait
	if baseWait > maxWait {
		baseWait = maxWait
	}

	// Add jitter: random value between 0 and baseWait
	jitter := time.Duration(rand.Int63n(int64(baseWait)))

	return baseWait + jitter
}

// waitWithContext waits on a condition variable until context is done.
func waitWithContext(ctx context.Context, c *sync.Cond) error {
	done := make(chan struct{})
	go func() {
		c.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
