// Package providers defines the interface for search/crawl providers
// and common types used across different provider implementations.
package providers

import (
	"context"
	"time"

	"github.com/lamim/sanity-web-eval/internal/debug"
)

// SupportLevel describes the implementation quality for an operation.
type SupportLevel string

const (
	// SupportNative indicates first-class native provider support.
	SupportNative SupportLevel = "native"
	// SupportEmulated indicates a fallback strategy (not first-class).
	SupportEmulated SupportLevel = "emulated"
	// SupportUnsupported indicates the operation is not available.
	SupportUnsupported SupportLevel = "unsupported"
)

// RunMode defines benchmark execution mode.
type RunMode string

const (
	// ModeNormalized enforces mostly comparable settings across providers.
	ModeNormalized RunMode = "normalized"
	// ModeNative allows provider-optimized native settings.
	ModeNative RunMode = "native"
)

// CapabilitySet describes provider support by operation.
type CapabilitySet struct {
	Search  SupportLevel
	Extract SupportLevel
	Crawl   SupportLevel
}

// ForOperation returns the support level for the given operation.
func (c CapabilitySet) ForOperation(opType string) SupportLevel {
	switch opType {
	case "search":
		if c.Search == "" {
			return SupportUnsupported
		}
		return c.Search
	case "extract":
		if c.Extract == "" {
			return SupportUnsupported
		}
		return c.Extract
	case "crawl":
		if c.Crawl == "" {
			return SupportUnsupported
		}
		return c.Crawl
	default:
		return SupportUnsupported
	}
}

// SupportsOperation checks support from the capability set.
func (c CapabilitySet) SupportsOperation(opType string) bool {
	return c.ForOperation(opType) != SupportUnsupported
}

// contextKey is a private type for context keys to avoid collisions
type contextKey int

const (
	// debugLoggerKey is the context key for the debug logger
	debugLoggerKey contextKey = iota
	// testLogKey is the context key for the current test log
	testLogKey
)

// WithDebugLogger returns a context with the debug logger attached
func WithDebugLogger(ctx context.Context, logger *debug.Logger) context.Context {
	return context.WithValue(ctx, debugLoggerKey, logger)
}

// DebugLoggerFromContext retrieves the debug logger from context
func DebugLoggerFromContext(ctx context.Context) *debug.Logger {
	if logger, ok := ctx.Value(debugLoggerKey).(*debug.Logger); ok {
		return logger
	}
	return nil
}

// WithTestLog returns a context with the test log attached
func WithTestLog(ctx context.Context, testLog *debug.TestLog) context.Context {
	return context.WithValue(ctx, testLogKey, testLog)
}

// TestLogFromContext retrieves the test log from context
func TestLogFromContext(ctx context.Context) *debug.TestLog {
	if testLog, ok := ctx.Value(testLogKey).(*debug.TestLog); ok {
		return testLog
	}
	return nil
}

// SearchResult represents the result of a search operation
type SearchResult struct {
	Query        string
	Results      []SearchItem
	TotalResults int
	Latency      time.Duration
	CreditsUsed  int
	RequestCount int
	// UsageReported indicates credits/tokens came from provider usage metadata.
	UsageReported bool
	RawResponse   []byte
}

// SearchItem represents a single search result
type SearchItem struct {
	Title       string
	URL         string
	Content     string
	Score       float64
	PublishedAt *time.Time
}

// ExtractResult represents the result of a content extraction operation
type ExtractResult struct {
	URL           string
	Title         string
	Content       string
	Markdown      string
	Metadata      map[string]interface{}
	Latency       time.Duration
	CreditsUsed   int
	RequestCount  int
	UsageReported bool
}

// CrawlResult represents the result of a crawl operation
type CrawlResult struct {
	URL           string
	Pages         []CrawledPage
	TotalPages    int
	Latency       time.Duration
	CreditsUsed   int
	RequestCount  int
	UsageReported bool
}

// CrawledPage represents a single page from a crawl
type CrawledPage struct {
	URL      string
	Title    string
	Content  string
	Markdown string
}

// Provider defines the interface for search/crawl providers
// Provider defines the interface for search/crawl providers
type Provider interface {
	Name() string
	// Capabilities returns support level by operation.
	Capabilities() CapabilitySet
	// SupportsOperation returns whether the provider supports the given operation type
	// Valid operation types: "search", "extract", "crawl"
	SupportsOperation(opType string) bool
	Search(ctx context.Context, query string, opts SearchOptions) (*SearchResult, error)
	Extract(ctx context.Context, url string, opts ExtractOptions) (*ExtractResult, error)
	Crawl(ctx context.Context, url string, opts CrawlOptions) (*CrawlResult, error)
}

// SearchOptions contains options for search operations
type SearchOptions struct {
	MaxResults    int
	SearchDepth   string // basic, advanced
	IncludeImages bool
	IncludeAnswer bool
	TimeRange     string // day, week, month, year
}

// ExtractOptions contains options for extract operations
type ExtractOptions struct {
	Format          string // markdown, html, text
	IncludeMetadata bool
}

// CrawlOptions contains options for crawl operations
type CrawlOptions struct {
	MaxPages     int
	MaxDepth     int
	ExcludePaths []string
}

// DefaultSearchOptions returns default search options
func DefaultSearchOptions() SearchOptions {
	return SearchOptions{
		MaxResults:    5,
		SearchDepth:   "advanced", // Use advanced to get full content for quality scoring
		IncludeAnswer: true,
	}
}

// DefaultExtractOptions returns default extract options
func DefaultExtractOptions() ExtractOptions {
	return ExtractOptions{
		Format:          "markdown",
		IncludeMetadata: true,
	}
}

// DefaultCrawlOptions returns default crawl options
func DefaultCrawlOptions() CrawlOptions {
	return CrawlOptions{
		MaxPages: 10,
		MaxDepth: 2,
	}
}
