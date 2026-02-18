package providers

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
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

func TestParseRetryAfter_HeaderSeconds(t *testing.T) {
	h := http.Header{}
	h.Set("Retry-After", "30")
	got := parseRetryAfter(h, nil)
	if got != 30*time.Second {
		t.Fatalf("expected 30s, got %v", got)
	}
}

func TestParseRetryAfter_BodyPattern(t *testing.T) {
	body := []byte(`{"error":"Rate limit exceeded. please retry after 45s, resets at ..."}`)
	got := parseRetryAfter(http.Header{}, body)
	if got != 45*time.Second {
		t.Fatalf("expected 45s, got %v", got)
	}
}

func TestParseRetryAfter_HeaderTakesPrecedenceOverBody(t *testing.T) {
	h := http.Header{}
	h.Set("Retry-After", "10")
	body := []byte(`retry after 45s`)
	got := parseRetryAfter(h, body)
	if got != 10*time.Second {
		t.Fatalf("expected header value 10s, got %v", got)
	}
}

func TestParseRetryAfter_NoHint(t *testing.T) {
	got := parseRetryAfter(http.Header{}, []byte(`some other error`))
	if got != 0 {
		t.Fatalf("expected 0, got %v", got)
	}
}

func TestDefaultRetryConfig_MaxBackoff60s(t *testing.T) {
	rc := DefaultRetryConfig()
	if rc.MaxBackoff != 60*time.Second {
		t.Fatalf("expected MaxBackoff 60s, got %v", rc.MaxBackoff)
	}
}
