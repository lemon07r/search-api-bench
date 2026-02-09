// Package jina provides a client for the Jina AI Reader and Search APIs.
// It implements the providers.Provider interface for benchmarking search, extract, and crawl operations.
//
// Jina AI offers two main endpoints:
// - Reader API (r.jina.ai): Converts URLs to LLM-friendly markdown
// - Search API (s.jina.ai): Searches web and returns top 5 results with content
//
// API Documentation: https://jina.ai/reader/
package jina

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/lamim/search-api-bench/internal/debug"
	"github.com/lamim/search-api-bench/internal/providers"
)

const (
	readerBaseURL  = "https://r.jina.ai"
	searchBaseURL  = "https://s.jina.ai"
	apiBaseURL     = "https://api.jina.ai"
	defaultTimeout = 30 * time.Second
	// MaxExtractCredits caps the credits for extract to prevent inflated numbers
	// Jina bills by tokens (chars/4), but we cap at 100 for fair comparison
	MaxExtractCredits = 100
	// BaseSearchCredits is the base cost per search request
	BaseSearchCredits = 1000
)

// Client represents a Jina AI API client
type Client struct {
	apiKey     string
	httpClient *http.Client
	retryCfg   providers.RetryConfig
}

// NewClient creates a new Jina AI client
func NewClient() (*Client, error) {
	apiKey := os.Getenv("JINA_API_KEY")
	// Jina works without API key but with lower rate limits

	// Allow timeout override via environment variable
	timeout := defaultTimeout
	if t := os.Getenv("JINA_TIMEOUT"); t != "" {
		if d, err := time.ParseDuration(t); err == nil {
			timeout = d
		}
	}

	// Use conservative retry config to avoid burning credits on retries
	retryCfg := providers.RetryConfig{
		MaxRetries:     1, // Reduce from 3 to 1 to limit credit usage on failures
		InitialBackoff: 500 * time.Millisecond,
		MaxBackoff:     5 * time.Second,
		BackoffFactor:  2.0,
		RetryableErrors: []int{
			http.StatusTooManyRequests,    // 429
			http.StatusServiceUnavailable, // 503
		},
	}

	return &Client{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		retryCfg: retryCfg,
	}, nil
}

// Name returns the provider name
func (c *Client) Name() string {
	return "jina"
}

// Search performs a web search using Jina AI Search API
// Endpoint: GET https://s.jina.ai/?q={query}
// Returns top 5 results with full content in LLM-friendly format
func (c *Client) Search(ctx context.Context, query string, opts providers.SearchOptions) (*providers.SearchResult, error) {
	// Limit max results to prevent timeouts - Jina Search API can be slow with many results
	if opts.MaxResults > 3 {
		opts.MaxResults = 3
	}

	var result *providers.SearchResult
	var searchErr error

	err := c.retryCfg.DoWithRetry(ctx, func() error {
		result, searchErr = c.searchInternal(ctx, query, opts)
		return searchErr
	})

	if err != nil {
		return nil, err
	}
	return result, nil
}

// searchInternal performs the actual search request
func (c *Client) searchInternal(ctx context.Context, query string, _ providers.SearchOptions) (*providers.SearchResult, error) {
	start := time.Now()

	// Build search URL with query parameter
	searchURL := fmt.Sprintf("%s/?q=%s", searchBaseURL, url.QueryEscape(query))

	// Add JSON format for structured response
	searchURL += "&format=json"

	// Prepare headers
	headers := make(map[string]string)
	if c.apiKey != "" {
		headers["Authorization"] = "Bearer " + c.apiKey
	}

	// Log the request
	logRequest(ctx, "GET", searchURL, headers, "")

	// Create request with trace context for timing
	traceCtx, timing, finalize := newTraceContext(ctx)
	req, err := http.NewRequestWithContext(traceCtx, "GET", searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add authorization if API key is available
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Finalize timing
	finalize()
	if timing != nil {
		timing.TotalDuration = time.Since(start)
	}

	// Log the response
	respHeaders := make(map[string]string)
	respHeaders["Content-Type"] = resp.Header.Get("Content-Type")
	logResponse(ctx, resp.StatusCode, respHeaders, string(respBody), len(respBody), time.Since(start))

	// Check for non-OK status codes first
	if resp.StatusCode != http.StatusOK {
		// Check if response is JSON error or plain text
		contentType := resp.Header.Get("Content-Type")
		if isJSONContent(contentType) {
			return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(respBody))
		}
		// Plain text error (likely rate limit)
		errMsg := strings.TrimSpace(string(respBody))
		if resp.StatusCode == http.StatusTooManyRequests || isRateLimitError(errMsg) {
			return nil, fmt.Errorf("rate limit exceeded: %s", errMsg)
		}
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, errMsg)
	}

	// Check if response is actually JSON
	contentType := resp.Header.Get("Content-Type")
	if !isJSONContent(contentType) {
		// Handle plain text response (rate limiting without proper status code)
		textResp := strings.TrimSpace(string(respBody))
		if isRateLimitError(textResp) {
			return nil, fmt.Errorf("rate limit exceeded: %s", textResp)
		}
		// Try to parse as plain text search results
		return c.parsePlainTextSearchResponse(query, textResp, start)
	}

	var result searchResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		// Check if the body is a plain text error message
		textBody := strings.TrimSpace(string(respBody))
		if isRateLimitError(textBody) {
			return nil, fmt.Errorf("rate limit exceeded: %s", textBody)
		}
		return nil, fmt.Errorf("failed to unmarshal response: %w (body: %s)", err, truncateString(textBody, 200))
	}

	// Check for API-level errors in JSON response
	if result.Code != 0 && result.Code != 200 {
		return nil, fmt.Errorf("API returned error code %d", result.Code)
	}

	latency := time.Since(start)

	// Convert Jina results to provider format
	items := make([]providers.SearchItem, 0, len(result.Data))
	totalContentLen := 0
	for _, r := range result.Data {
		items = append(items, providers.SearchItem{
			Title:   r.Title,
			URL:     r.URL,
			Content: r.Content,
		})
		totalContentLen += len(r.Title) + len(r.Content)
	}

	// Jina Search bills by tokens (~4 chars per token) + base request cost
	// Use actual content length for more accurate credit estimation
	creditsUsed := BaseSearchCredits + (totalContentLen / 4)
	if creditsUsed > 10000 {
		creditsUsed = 10000 // Cap at 10k to match Jina's typical max
	}

	return &providers.SearchResult{
		Query:        query,
		Results:      items,
		TotalResults: len(items),
		Latency:      latency,
		CreditsUsed:  creditsUsed,
		RawResponse:  respBody,
	}, nil
}

// Extract extracts content from a URL using Jina AI Reader API
// Endpoint: GET https://r.jina.ai/http://{url}
// Converts any URL to clean, LLM-friendly markdown
func (c *Client) Extract(ctx context.Context, pageURL string, opts providers.ExtractOptions) (*providers.ExtractResult, error) {
	var result *providers.ExtractResult
	var extractErr error

	err := c.retryCfg.DoWithRetry(ctx, func() error {
		result, extractErr = c.extractInternal(ctx, pageURL, opts)
		return extractErr
	})

	if err != nil {
		return nil, err
	}
	return result, nil
}

// extractInternal performs the actual extract request
func (c *Client) extractInternal(ctx context.Context, pageURL string, opts providers.ExtractOptions) (*providers.ExtractResult, error) {
	start := time.Now()

	// Ensure URL has scheme
	if !hasScheme(pageURL) {
		pageURL = "https://" + pageURL
	}

	// Build reader URL - Jina expects the target URL after /http:// or /https://
	readerURL := fmt.Sprintf("%s/http://%s", readerBaseURL, pageURL)

	// Add JSON format if metadata is requested
	if opts.IncludeMetadata {
		readerURL += "?format=json"
	}

	// Prepare headers
	headers := make(map[string]string)
	if c.apiKey != "" {
		headers["Authorization"] = "Bearer " + c.apiKey
	}

	// Log the request
	logRequest(ctx, "GET", readerURL, headers, "")

	// Create request with trace context for timing
	traceCtx, timing, finalize := newTraceContext(ctx)
	req, err := http.NewRequestWithContext(traceCtx, "GET", readerURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add authorization if API key is available
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Finalize timing
	finalize()
	if timing != nil {
		timing.TotalDuration = time.Since(start)
	}

	// Log the response
	respHeaders := make(map[string]string)
	respHeaders["Content-Type"] = resp.Header.Get("Content-Type")
	logResponse(ctx, resp.StatusCode, respHeaders, string(respBody), len(respBody), time.Since(start))

	// Check for non-OK status codes with proper error handling
	if resp.StatusCode != http.StatusOK {
		contentType := resp.Header.Get("Content-Type")
		errMsg := strings.TrimSpace(string(respBody))
		if !isJSONContent(contentType) && isRateLimitError(errMsg) {
			return nil, fmt.Errorf("rate limit exceeded: %s", errMsg)
		}
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, errMsg)
	}

	latency := time.Since(start)

	var content, title string
	var metadata map[string]interface{}

	if opts.IncludeMetadata {
		// Check if response is actually JSON
		contentType := resp.Header.Get("Content-Type")
		if !isJSONContent(contentType) {
			// Handle plain text response (likely rate limit message)
			textResp := strings.TrimSpace(string(respBody))
			if isRateLimitError(textResp) {
				return nil, fmt.Errorf("rate limit exceeded: %s", textResp)
			}
			// Use plain text as content
			content = textResp
			title = extractTitleFromMarkdown(content)
			metadata = map[string]interface{}{}
		} else {
			var result readerResponse
			if err := json.Unmarshal(respBody, &result); err != nil {
				// Check if body is plain text error
				textBody := strings.TrimSpace(string(respBody))
				if isRateLimitError(textBody) {
					return nil, fmt.Errorf("rate limit exceeded: %s", textBody)
				}
				return nil, fmt.Errorf("failed to unmarshal response: %w (body: %s)", err, truncateString(textBody, 200))
			}
			content = result.Content
			title = result.Title
			metadata = map[string]interface{}{
				"url":       result.URL,
				"timestamp": result.Timestamp,
			}
		}
	} else {
		// Plain text response
		content = string(respBody)
		// Try to extract title from first line if it's markdown
		title = extractTitleFromMarkdown(content)
		metadata = map[string]interface{}{}
	}

	// Credits based on output token count (approximate)
	// Jina bills by tokens where 1 token ≈ 4 characters
	// We cap at MaxExtractCredits for fair comparison with other providers
	creditsUsed := len(content) / 4 // Rough token estimation
	if creditsUsed > MaxExtractCredits {
		creditsUsed = MaxExtractCredits
	}

	return &providers.ExtractResult{
		URL:         pageURL,
		Title:       title,
		Content:     content,
		Markdown:    content,
		Metadata:    metadata,
		Latency:     latency,
		CreditsUsed: creditsUsed,
	}, nil
}

// Crawl crawls a website using Jina AI
// Note: Jina doesn't have a native crawl API. We only extract the single starting URL
// to avoid burning through API quota with N+1 search+extract calls.
func (c *Client) Crawl(ctx context.Context, startURL string, opts providers.CrawlOptions) (*providers.CrawlResult, error) {
	start := time.Now()

	// Jina doesn't support native crawling - only extract the starting URL
	// This prevents excessive API usage from the N+1 search+extract pattern
	return c.crawlSinglePage(ctx, startURL, opts, start)
}

// crawlSinglePage is a fallback that just extracts the starting URL
func (c *Client) crawlSinglePage(ctx context.Context, pageURL string, _ providers.CrawlOptions, startTime time.Time) (*providers.CrawlResult, error) {
	extractOpts := providers.DefaultExtractOptions()
	extractResult, err := c.Extract(ctx, pageURL, extractOpts)
	if err != nil {
		return nil, fmt.Errorf("crawl failed: %w", err)
	}

	pages := []providers.CrawledPage{
		{
			URL:      pageURL,
			Title:    extractResult.Title,
			Content:  extractResult.Content,
			Markdown: extractResult.Markdown,
		},
	}

	latency := time.Since(startTime)

	return &providers.CrawlResult{
		URL:         pageURL,
		Pages:       pages,
		TotalPages:  1,
		Latency:     latency,
		CreditsUsed: extractResult.CreditsUsed,
	}, nil
}

// Helper functions

func hasScheme(s string) bool {
	return len(s) > 7 && (s[:7] == "http://" || s[:8] == "https://")
}

// logRequest logs an HTTP request via the debug logger if available
func logRequest(ctx context.Context, method, url string, headers map[string]string, body string) {
	testLog := providers.TestLogFromContext(ctx)
	logger := providers.DebugLoggerFromContext(ctx)
	if logger == nil || testLog == nil {
		return
	}
	logger.LogRequest(testLog, method, url, headers, body)
}

// logResponse logs an HTTP response via the debug logger if available
func logResponse(ctx context.Context, statusCode int, headers map[string]string, body string, bodySize int, duration time.Duration) {
	testLog := providers.TestLogFromContext(ctx)
	logger := providers.DebugLoggerFromContext(ctx)
	if logger == nil || testLog == nil {
		return
	}
	logger.LogResponse(testLog, statusCode, headers, body, bodySize, duration)
}

// newTraceContext creates an httptrace context for timing breakdown if debug is enabled
func newTraceContext(ctx context.Context) (context.Context, *debug.TimingBreakdown, func()) {
	testLog := providers.TestLogFromContext(ctx)
	logger := providers.DebugLoggerFromContext(ctx)
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

// isJSONContent checks if content type indicates JSON
func isJSONContent(contentType string) bool {
	return strings.Contains(contentType, "application/json") || strings.Contains(contentType, "text/json")
}

// isRateLimitError checks if error message indicates rate limiting
func isRateLimitError(msg string) bool {
	msgLower := strings.ToLower(msg)
	return strings.Contains(msgLower, "too many requests") ||
		strings.Contains(msgLower, "rate limit") ||
		strings.Contains(msgLower, "ratelimit") ||
		strings.Contains(msgLower, "quota exceeded") ||
		strings.Contains(msgLower, "limit exceeded")
}

// truncateString limits string length with ellipsis
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// parsePlainTextSearchResponse attempts to parse plain text as search results
func (c *Client) parsePlainTextSearchResponse(query, text string, startTime time.Time) (*providers.SearchResult, error) {
	latency := time.Since(startTime)

	// If text is empty or looks like an error, return it as error
	if text == "" || strings.Contains(strings.ToLower(text), "error") {
		return nil, fmt.Errorf("API returned plain text error: %s", text)
	}

	// Create a single search result from the text
	// This is a fallback when JSON format is not available
	items := []providers.SearchItem{
		{
			Title:   "Search Result",
			URL:     "",
			Content: text,
		},
	}

	// Calculate credits based on actual content length
	creditsUsed := BaseSearchCredits + (len(text) / 4)
	if creditsUsed > 10000 {
		creditsUsed = 10000
	}

	return &providers.SearchResult{
		Query:        query,
		Results:      items,
		TotalResults: len(items),
		Latency:      latency,
		CreditsUsed:  creditsUsed,
	}, nil
}

func extractTitleFromMarkdown(content string) string {
	// Try to extract title from first # heading
	if len(content) > 2 && content[0] == '#' {
		end := bytes.IndexByte([]byte(content[1:]), '\n')
		if end == -1 {
			end = len(content) - 1
		}
		return string(bytes.TrimSpace([]byte(content[1 : end+1])))
	}
	return ""
}

// Response types

type searchResponse struct {
	Code int `json:"code"`
	Data []struct {
		Title   string `json:"title"`
		URL     string `json:"url"`
		Content string `json:"content"`
	} `json:"data"`
}

type readerResponse struct {
	URL       string `json:"url"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	Timestamp string `json:"timestamp,omitempty"`
}
