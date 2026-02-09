package robustness

import (
	"fmt"
	"strings"
	"sync"
)

// ErrorCategory classifies different types of errors
type ErrorCategory int

const (
	// ErrUnknown represents an unknown error category
	ErrUnknown ErrorCategory = iota
	// ErrTimeout represents a timeout error
	ErrTimeout
	// ErrRateLimit represents a rate limit error
	ErrRateLimit
	// ErrAuth represents an authentication error
	ErrAuth
	// ErrServer5xx represents a server 5xx error
	ErrServer5xx
	// ErrClient4xx represents a client 4xx error
	ErrClient4xx
	// ErrNetwork represents a network error
	ErrNetwork
	// ErrParse represents a parse error
	ErrParse
	// ErrContextCanceled represents a context canceled error
	ErrContextCanceled
	// ErrValidation represents a validation error
	ErrValidation
	// ErrNotFound represents a not found error
	ErrNotFound
	// ErrRedirect represents a redirect error
	ErrRedirect
	// ErrContentType represents a content type error
	ErrContentType
	// ErrSizeLimit represents a size limit error
	ErrSizeLimit
)

// String returns the string representation of an error category
func (e ErrorCategory) String() string {
	switch e {
	case ErrTimeout:
		return "timeout"
	case ErrRateLimit:
		return "rate_limit"
	case ErrAuth:
		return "authentication"
	case ErrServer5xx:
		return "server_error"
	case ErrClient4xx:
		return "client_error"
	case ErrNetwork:
		return "network"
	case ErrParse:
		return "parse"
	case ErrContextCanceled:
		return "canceled"
	case ErrValidation:
		return "validation"
	case ErrNotFound:
		return "not_found"
	case ErrRedirect:
		return "redirect"
	case ErrContentType:
		return "content_type"
	case ErrSizeLimit:
		return "size_limit"
	default:
		return "unknown"
	}
}

// CategorizeError analyzes an error and returns its category and a normalized message
func CategorizeError(err error) (ErrorCategory, string) {
	if err == nil {
		return ErrUnknown, ""
	}

	errStr := err.Error()

	// Check error categories in order of specificity
	if cat, msg := checkContextErrors(errStr); cat != ErrUnknown {
		return cat, msg
	}
	if cat, msg := checkTimeoutErrors(errStr); cat != ErrUnknown {
		return cat, msg
	}
	if cat, msg := checkAuthErrors(errStr); cat != ErrUnknown {
		return cat, msg
	}
	if cat, msg := checkNotFoundErrors(errStr); cat != ErrUnknown {
		return cat, msg
	}
	if cat, msg := checkServerErrors(errStr); cat != ErrUnknown {
		return cat, msg
	}
	if cat, msg := checkClientErrors(errStr); cat != ErrUnknown {
		return cat, msg
	}
	if cat, msg := checkRedirectErrors(errStr); cat != ErrUnknown {
		return cat, msg
	}
	if cat, msg := checkNetworkErrors(errStr); cat != ErrUnknown {
		return cat, msg
	}
	if cat, msg := checkParseErrors(errStr); cat != ErrUnknown {
		return cat, msg
	}
	if cat, msg := checkContentErrors(errStr); cat != ErrUnknown {
		return cat, msg
	}

	return ErrUnknown, errStr
}

func checkContextErrors(errStr string) (ErrorCategory, string) {
	if contains(errStr, "context canceled") || contains(errStr, "context deadline exceeded") {
		return ErrContextCanceled, "Request was canceled"
	}
	return ErrUnknown, ""
}

func checkTimeoutErrors(errStr string) (ErrorCategory, string) {
	if contains(errStr, "timeout") || contains(errStr, "deadline exceeded") || contains(errStr, "i/o timeout") {
		return ErrTimeout, "Request timed out"
	}
	return ErrUnknown, ""
}

func checkAuthErrors(errStr string) (ErrorCategory, string) {
	if contains(errStr, "rate limit") || contains(errStr, "too many requests") || contains(errStr, "429") {
		return ErrRateLimit, "Rate limit exceeded"
	}
	if contains(errStr, "unauthorized") || contains(errStr, "authentication") ||
		contains(errStr, "api key") || contains(errStr, "401") || contains(errStr, "403") {
		return ErrAuth, "Authentication failed"
	}
	return ErrUnknown, ""
}

func checkNotFoundErrors(errStr string) (ErrorCategory, string) {
	if contains(errStr, "not found") || contains(errStr, "404") ||
		contains(errStr, "no such host") || contains(errStr, "no extraction results") {
		return ErrNotFound, "Resource not found"
	}
	return ErrUnknown, ""
}

func checkServerErrors(errStr string) (ErrorCategory, string) {
	if contains(errStr, "500") || contains(errStr, "502") ||
		contains(errStr, "503") || contains(errStr, "504") ||
		contains(errStr, "internal server error") {
		return ErrServer5xx, "Server error"
	}
	return ErrUnknown, ""
}

func checkClientErrors(errStr string) (ErrorCategory, string) {
	if contains(errStr, "400") || contains(errStr, "405") ||
		contains(errStr, "422") || contains(errStr, "bad request") {
		return ErrClient4xx, "Client error"
	}
	return ErrUnknown, ""
}

func checkRedirectErrors(errStr string) (ErrorCategory, string) {
	if contains(errStr, "redirect") || contains(errStr, "301") ||
		contains(errStr, "302") || contains(errStr, "too many redirects") {
		return ErrRedirect, "Redirect error"
	}
	return ErrUnknown, ""
}

func checkNetworkErrors(errStr string) (ErrorCategory, string) {
	if contains(errStr, "connection refused") || contains(errStr, "connection reset") ||
		contains(errStr, "no such host") || contains(errStr, "temporary failure") ||
		contains(errStr, "network") || contains(errStr, "dial tcp") {
		return ErrNetwork, "Network error"
	}
	return ErrUnknown, ""
}

func checkParseErrors(errStr string) (ErrorCategory, string) {
	if contains(errStr, "unmarshal") || contains(errStr, "parse") ||
		contains(errStr, "invalid character") || contains(errStr, "json") {
		return ErrParse, "Parse error"
	}
	return ErrUnknown, ""
}

func checkContentErrors(errStr string) (ErrorCategory, string) {
	if contains(errStr, "content type") || contains(errStr, "mime") {
		return ErrContentType, "Content type error"
	}
	if contains(errStr, "size") || contains(errStr, "too large") ||
		contains(errStr, "content too long") {
		return ErrSizeLimit, "Size limit exceeded"
	}
	if contains(errStr, "validation") || contains(errStr, "invalid") ||
		contains(errStr, "missing") {
		return ErrValidation, "Validation error"
	}
	return ErrUnknown, ""
}

// ErrorStats tracks error statistics
type ErrorStats struct {
	mu       sync.Mutex
	counts   map[ErrorCategory]int
	examples map[ErrorCategory][]string
}

// NewErrorStats creates a new error statistics tracker
func NewErrorStats() *ErrorStats {
	return &ErrorStats{
		counts:   make(map[ErrorCategory]int),
		examples: make(map[ErrorCategory][]string),
	}
}

// Record records an error
func (s *ErrorStats) Record(err error) {
	if err == nil {
		return
	}

	category, normalized := CategorizeError(err)

	s.mu.Lock()
	defer s.mu.Unlock()

	s.counts[category]++

	// Keep up to 3 examples per category
	if len(s.examples[category]) < 3 {
		s.examples[category] = append(s.examples[category], normalized)
	}
}

// GetCounts returns error counts by category
func (s *ErrorStats) GetCounts() map[ErrorCategory]int {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := make(map[ErrorCategory]int)
	for k, v := range s.counts {
		result[k] = v
	}
	return result
}

// GetExamples returns example messages for each category
func (s *ErrorStats) GetExamples() map[ErrorCategory][]string {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := make(map[ErrorCategory][]string)
	for k, v := range s.examples {
		result[k] = append([]string(nil), v...)
	}
	return result
}

// TotalErrors returns the total error count
func (s *ErrorStats) TotalErrors() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	total := 0
	for _, count := range s.counts {
		total += count
	}
	return total
}

// MostCommon returns the most common error category
func (s *ErrorStats) MostCommon() (ErrorCategory, int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var mostCommon ErrorCategory
	maxCount := 0

	for category, count := range s.counts {
		if count > maxCount {
			maxCount = count
			mostCommon = category
		}
	}

	return mostCommon, maxCount
}

// FormatReport returns a formatted error report
func (s *ErrorStats) FormatReport() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	var sb strings.Builder
	sb.WriteString("Error Statistics:\n")

	total := 0
	for _, count := range s.counts {
		total += count
	}

	if total == 0 {
		sb.WriteString("  No errors recorded\n")
		return sb.String()
	}

	sb.WriteString(fmt.Sprintf("  Total Errors: %d\n", total))

	// Sort by count (simple bubble sort for small maps)
	categories := make([]ErrorCategory, 0, len(s.counts))
	for cat := range s.counts {
		categories = append(categories, cat)
	}

	for i := 0; i < len(categories); i++ {
		for j := i + 1; j < len(categories); j++ {
			if s.counts[categories[i]] < s.counts[categories[j]] {
				categories[i], categories[j] = categories[j], categories[i]
			}
		}
	}

	for _, category := range categories {
		count := s.counts[category]
		percentage := float64(count) / float64(total) * 100
		sb.WriteString(fmt.Sprintf("  %s: %d (%.1f%%)\n", category.String(), count, percentage))
	}

	return sb.String()
}

// ErrorReport is a serializable error report
type ErrorReport struct {
	TotalErrors int                 `json:"total_errors"`
	ByCategory  map[string]int      `json:"by_category"`
	Examples    map[string][]string `json:"examples"`
}

// GenerateReport creates a serializable error report
func (s *ErrorStats) GenerateReport() ErrorReport {
	s.mu.Lock()
	defer s.mu.Unlock()

	report := ErrorReport{
		ByCategory: make(map[string]int),
		Examples:   make(map[string][]string),
	}

	for category, count := range s.counts {
		report.TotalErrors += count
		report.ByCategory[category.String()] = count
	}

	for category, examples := range s.examples {
		report.Examples[category.String()] = append([]string(nil), examples...)
	}

	return report
}

func contains(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
