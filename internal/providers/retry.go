package providers

import (
	"context"
	"fmt"
	"math"
	"math/rand/v2"
	"net/http"
	"strings"
	"time"
)

// RetryConfig configures retry behavior for HTTP requests
type RetryConfig struct {
	MaxRetries      int
	InitialBackoff  time.Duration
	MaxBackoff      time.Duration
	BackoffFactor   float64
	RetryableErrors []int // HTTP status codes to retry
}

// DefaultRetryConfig returns a default retry configuration
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:     3,
		InitialBackoff: 500 * time.Millisecond,
		MaxBackoff:     10 * time.Second,
		BackoffFactor:  2.0,
		RetryableErrors: []int{
			http.StatusTooManyRequests,     // 429
			http.StatusInternalServerError, // 500
			http.StatusBadGateway,          // 502
			http.StatusServiceUnavailable,  // 503
			http.StatusGatewayTimeout,      // 504
		},
	}
}

// RetryableError checks if an error or status code should trigger a retry
func (rc *RetryConfig) RetryableError(err error, statusCode int) bool {
	// Check status code
	for _, code := range rc.RetryableErrors {
		if statusCode == code {
			return true
		}
	}

	// Check error message for rate limiting indicators
	if err != nil {
		errStr := strings.ToLower(err.Error())
		rateLimitIndicators := []string{
			"rate limit",
			"ratelimit",
			"too many requests",
			"quota exceeded",
			"limit exceeded",
			"429",
		}
		for _, indicator := range rateLimitIndicators {
			if strings.Contains(errStr, indicator) {
				return true
			}
		}
	}

	return false
}

// CalculateBackoff calculates the backoff duration for a given attempt
func (rc *RetryConfig) CalculateBackoff(attempt int) time.Duration {
	if attempt < 0 {
		attempt = 0
	}

	// Exponential backoff: initial * factor^attempt
	backoff := float64(rc.InitialBackoff) * math.Pow(rc.BackoffFactor, float64(attempt))

	// Add jitter (±25%) to avoid thundering herd
	//nolint:gosec // Math/rand/v2 is sufficient for jitter; crypto/rand is unnecessary for this use case
	jitter := backoff * 0.25 * (2*rand.Float64() - 1)
	backoff += jitter

	// Cap at max backoff
	if backoff > float64(rc.MaxBackoff) {
		backoff = float64(rc.MaxBackoff)
	}

	return time.Duration(backoff)
}

// DoWithRetry executes the given function with retry logic
func (rc *RetryConfig) DoWithRetry(ctx context.Context, operation func() error) error {
	var lastErr error

	for attempt := 0; attempt <= rc.MaxRetries; attempt++ {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled: %w", ctx.Err())
		default:
		}

		lastErr = operation()
		if lastErr == nil {
			return nil // Success
		}

		// Don't retry on last attempt
		if attempt == rc.MaxRetries {
			break
		}

		// Check if error is retryable
		// We need to determine status code from error if possible
		// This is a simplified check - providers should integrate more specific logic
		if !rc.isRetryable(lastErr) {
			return lastErr // Non-retryable error, return immediately
		}

		// Calculate and apply backoff
		backoff := rc.CalculateBackoff(attempt)

		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled during retry: %w", ctx.Err())
		case <-time.After(backoff):
			// Continue to next attempt
		}
	}

	return fmt.Errorf("max retries (%d) exceeded: %w", rc.MaxRetries, lastErr)
}

// isRetryable determines if an error should be retried
func (rc *RetryConfig) isRetryable(err error) bool {
	if err == nil {
		return false
	}

	errStr := strings.ToLower(err.Error())

	// Rate limiting indicators
	rateLimitIndicators := []string{
		"rate limit",
		"ratelimit",
		"too many requests",
		"quota exceeded",
		"limit exceeded",
		"429",
	}

	for _, indicator := range rateLimitIndicators {
		if strings.Contains(errStr, indicator) {
			return true
		}
	}

	// Server error indicators
	serverErrorIndicators := []string{
		"500",
		"502",
		"503",
		"504",
		"internal server error",
		"bad gateway",
		"service unavailable",
		"gateway timeout",
	}

	for _, indicator := range serverErrorIndicators {
		if strings.Contains(errStr, indicator) {
			return true
		}
	}

	// Network/transient error indicators
	transientIndicators := []string{
		"timeout",
		"connection refused",
		"no such host",
		"temporary",
		"eof",
	}

	for _, indicator := range transientIndicators {
		if strings.Contains(errStr, indicator) {
			return true
		}
	}

	return false
}

// SleepWithContext sleeps for the given duration or until context is cancelled
func SleepWithContext(ctx context.Context, duration time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(duration):
		return nil
	}
}
