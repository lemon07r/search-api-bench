package quality

import (
	"net/http"
	"strings"
)

// isRetryableQualityStatus determines whether a quality-model HTTP response
// should be retried.
func isRetryableQualityStatus(statusCode int, body string) bool {
	if statusCode == http.StatusTooManyRequests || statusCode >= 500 {
		return true
	}

	if statusCode != http.StatusBadRequest {
		return false
	}

	msg := strings.ToLower(body)
	transientIndicators := []string{
		"repeat the request later",
		"unable to process request",
		"temporarily unavailable",
		"try again later",
		"please retry",
		"service unavailable",
		"overloaded",
		"busy",
	}

	for _, indicator := range transientIndicators {
		if strings.Contains(msg, indicator) {
			return true
		}
	}

	return false
}
