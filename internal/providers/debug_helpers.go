// Package providers provides shared debug logging helpers for all provider implementations.
package providers

import (
	"context"
	"net/http/httptrace"
	"time"

	"github.com/lamim/sanity-web-eval/internal/debug"
)

// LogRequest logs an HTTP request via the debug logger if available in context.
// This is a shared helper that all providers can use for consistent debug logging.
func LogRequest(ctx context.Context, method, url string, headers map[string]string, body string) {
	testLog := TestLogFromContext(ctx)
	logger := DebugLoggerFromContext(ctx)
	if logger == nil || testLog == nil {
		return
	}
	logger.LogRequest(testLog, method, url, headers, body)
}

// LogResponse logs an HTTP response via the debug logger if available in context.
// This is a shared helper that all providers can use for consistent debug logging.
func LogResponse(ctx context.Context, statusCode int, headers map[string]string, body string, bodySize int, duration time.Duration) {
	testLog := TestLogFromContext(ctx)
	logger := DebugLoggerFromContext(ctx)
	if logger == nil || testLog == nil {
		return
	}
	logger.LogResponse(testLog, statusCode, headers, body, bodySize, duration)
}

// LogError logs an error via the debug logger if available in context.
// This is a shared helper that all providers can use for consistent debug logging.
func LogError(ctx context.Context, message, category, errContext string) {
	testLog := TestLogFromContext(ctx)
	logger := DebugLoggerFromContext(ctx)
	if logger == nil || testLog == nil {
		return
	}
	logger.LogError(testLog, message, category, errContext)
}

// NewTraceContext creates an httptrace context for timing breakdown if debug is enabled.
// Returns the enhanced context, timing breakdown pointer, and a finalize function.
// This is a shared helper that all providers can use for consistent timing capture.
func NewTraceContext(ctx context.Context) (context.Context, *debug.TimingBreakdown, func()) {
	testLog := TestLogFromContext(ctx)
	logger := DebugLoggerFromContext(ctx)
	if logger == nil || testLog == nil {
		return ctx, nil, func() {}
	}

	timing, finalize := logger.NewTraceContext(testLog)
	trace := logger.GetTraceFromTest(testLog)
	if trace != nil {
		return httptrace.WithClientTrace(ctx, trace), timing, finalize
	}
	return ctx, timing, finalize
}

// HeadersToMap converts http.Header to a map for logging.
// This is a shared helper that all providers can use for consistent header logging.
func HeadersToMap(headers map[string][]string) map[string]string {
	if headers == nil {
		return nil
	}
	result := make(map[string]string, len(headers))
	for k, v := range headers {
		if len(v) > 0 {
			result[k] = v[0]
		}
	}
	return result
}
