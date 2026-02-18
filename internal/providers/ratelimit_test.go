package providers

import (
	"context"
	"testing"
	"time"
)

func TestRateLimiter_FirstRequestImmediate(t *testing.T) {
	rl := NewRateLimiter(1.0) // 1 req/s
	start := time.Now()
	if err := rl.Wait(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 10*time.Millisecond {
		t.Fatalf("first request should be immediate, took %v", elapsed)
	}
}

func TestRateLimiter_SecondRequestDelayed(t *testing.T) {
	rl := NewRateLimiter(10.0) // 10 req/s → 100ms interval
	ctx := context.Background()

	if err := rl.Wait(ctx); err != nil {
		t.Fatalf("unexpected error on first wait: %v", err)
	}

	start := time.Now()
	if err := rl.Wait(ctx); err != nil {
		t.Fatalf("unexpected error on second wait: %v", err)
	}
	elapsed := time.Since(start)

	// Should wait ~100ms (allow 50ms–200ms for scheduling variance)
	if elapsed < 50*time.Millisecond || elapsed > 200*time.Millisecond {
		t.Fatalf("expected ~100ms delay, got %v", elapsed)
	}
}

func TestRateLimiter_RespectsContextCancellation(t *testing.T) {
	rl := NewRateLimiter(0.5) // 0.5 req/s → 2s interval
	ctx := context.Background()

	// First request is immediate.
	if err := rl.Wait(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Cancel context before the next token is available.
	shortCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()

	err := rl.Wait(shortCtx)
	if err == nil {
		t.Fatal("expected context cancellation error, got nil")
	}
}

func TestRateLimiter_NilLimiterNoOp(t *testing.T) {
	// A nil limiter in RetryConfig should not panic or block.
	rc := DefaultRetryConfig()
	if rc.Limiter != nil {
		t.Fatal("default RetryConfig should have nil Limiter")
	}
}

func TestRateLimiter_BurstPacing(t *testing.T) {
	rl := NewRateLimiter(20.0) // 20 req/s → 50ms interval
	ctx := context.Background()

	start := time.Now()
	for i := 0; i < 5; i++ {
		if err := rl.Wait(ctx); err != nil {
			t.Fatalf("unexpected error on wait %d: %v", i, err)
		}
	}
	elapsed := time.Since(start)

	// 5 requests at 20 req/s: first immediate, then 4 × 50ms = 200ms total
	// Allow 150ms–350ms for scheduling variance.
	if elapsed < 150*time.Millisecond || elapsed > 350*time.Millisecond {
		t.Fatalf("expected ~200ms for 5 requests at 20 req/s, got %v", elapsed)
	}
}

func TestNewRateLimiter_ZeroRate(t *testing.T) {
	// Zero or negative rate should default to 1 req/s, not panic.
	rl := NewRateLimiter(0)
	if rl.interval != time.Second {
		t.Fatalf("expected 1s interval for zero rate, got %v", rl.interval)
	}

	rl2 := NewRateLimiter(-5)
	if rl2.interval != time.Second {
		t.Fatalf("expected 1s interval for negative rate, got %v", rl2.interval)
	}
}
