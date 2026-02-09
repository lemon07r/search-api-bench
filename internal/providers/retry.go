package providers

import (
	"bytes"
	"context"
	"fmt"
	"io"
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

// HTTPResult contains detailed information about a completed HTTP request.
type HTTPResult struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
	Attempts   int
}

// DoHTTPRequestDetailed executes an HTTP request with retry logic and returns full response details.
func (rc *RetryConfig) DoHTTPRequestDetailed(ctx context.Context, client *http.Client, req *http.Request) (*HTTPResult, error) {
	var requestBody []byte
	if req.Body != nil {
		bodyData, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read request body: %w", err)
		}
		requestBody = bodyData
		req.Body = io.NopCloser(bytes.NewReader(requestBody))
	}

	var lastErr error

	for attempt := 0; attempt <= rc.MaxRetries; attempt++ {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("context cancelled: %w", ctx.Err())
		default:
		}

		// Clone request for retry.
		reqClone := req.Clone(ctx)
		if requestBody != nil {
			reqClone.Body = io.NopCloser(bytes.NewReader(requestBody))
		}

		resp, err := client.Do(reqClone)
		if err != nil {
			lastErr = fmt.Errorf("request failed: %w", err)
			if attempt == rc.MaxRetries {
				break
			}
			if !rc.isRetryable(lastErr) {
				return nil, lastErr
			}
			backoff := rc.CalculateBackoff(attempt)
			if err := SleepWithContext(ctx, backoff); err != nil {
				return nil, err
			}
			continue
		}

		body, err := io.ReadAll(resp.Body)
		headers := resp.Header.Clone()
		_ = resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to read response: %w", err)
		}

		// Check for retryable status codes
		if rc.RetryableError(nil, resp.StatusCode) {
			lastErr = fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
			if attempt == rc.MaxRetries {
				break
			}
			backoff := rc.CalculateBackoff(attempt)
			if err := SleepWithContext(ctx, backoff); err != nil {
				return nil, err
			}
			continue
		}

		// Non-retryable error status
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
		}

		// Success
		return &HTTPResult{
			StatusCode: resp.StatusCode,
			Headers:    headers,
			Body:       body,
			Attempts:   attempt + 1,
		}, nil
	}

	return nil, fmt.Errorf("max retries (%d) exceeded: %w", rc.MaxRetries, lastErr)
}

// DoHTTPRequest executes an HTTP request with retry logic and returns only the response body.
func (rc *RetryConfig) DoHTTPRequest(ctx context.Context, client *http.Client, req *http.Request) ([]byte, error) {
	result, err := rc.DoHTTPRequestDetailed(ctx, client, req)
	if err != nil {
		return nil, err
	}
	return result.Body, nil
}
