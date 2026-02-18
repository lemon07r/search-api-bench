package providers

import (
	"context"
	"sync"
	"time"
)

// RateLimiter implements a token-bucket rate limiter that proactively spaces
// outgoing requests to stay within a provider's rate limit, avoiding 429s.
//
// Each call to Wait blocks until a token is available, or the context expires.
// Tokens are refilled at a steady rate (one per interval).
type RateLimiter struct {
	mu       sync.Mutex
	interval time.Duration // minimum time between requests
	last     time.Time     // timestamp of last granted token
}

// NewRateLimiter creates a rate limiter that allows maxPerSecond requests per second.
// For sub-second rates, pass a fractional value (e.g. 0.1 for 6 req/min).
func NewRateLimiter(maxPerSecond float64) *RateLimiter {
	if maxPerSecond <= 0 {
		maxPerSecond = 1
	}
	return &RateLimiter{
		interval: time.Duration(float64(time.Second) / maxPerSecond),
	}
}

// Wait blocks until a request token is available or the context is cancelled.
// Returns nil when a token is granted, or the context error if cancelled.
func (rl *RateLimiter) Wait(ctx context.Context) error {
	rl.mu.Lock()
	now := time.Now()

	// First request or enough time has passed — grant immediately.
	if rl.last.IsZero() || now.Sub(rl.last) >= rl.interval {
		rl.last = now
		rl.mu.Unlock()
		return nil
	}

	// Calculate how long to wait for the next slot.
	waitUntil := rl.last.Add(rl.interval)
	rl.last = waitUntil
	rl.mu.Unlock()

	delay := time.Until(waitUntil)
	if delay <= 0 {
		return nil
	}

	return SleepWithContext(ctx, delay)
}
