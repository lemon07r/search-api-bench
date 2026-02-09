package providers

import (
	"context"
	"errors"
	"testing"
)

func TestRetryConfigIsRetryable_ContextDeadlineExceeded(t *testing.T) {
	rc := DefaultRetryConfig()
	if !rc.isRetryable(errors.New("request failed: context deadline exceeded")) {
		t.Fatal("expected context deadline exceeded to be retryable")
	}
}

func TestRetryConfigDoWithRetry_RetriesOnContextDeadlineExceeded(t *testing.T) {
	rc := RetryConfig{
		MaxRetries:     1,
		InitialBackoff: 0,
		MaxBackoff:     0,
		BackoffFactor:  1,
	}

	attempts := 0
	err := rc.DoWithRetry(context.Background(), func() error {
		attempts++
		if attempts == 1 {
			return errors.New("request failed: context deadline exceeded")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("expected retry to recover from deadline exceeded, got error: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts)
	}
}
