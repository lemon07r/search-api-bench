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
	// defaultSearchTimeout bounds each individual search attempt.
	defaultSearchTimeout = 12 * time.Second
	// MaxExtractCredits caps the credits for extract to prevent inflated numbers
	// Jina bills by tokens (chars/4), but we cap at 100 for fair comparison
	MaxExtractCredits = 100
	// BaseSearchCredits is the base cost per search request
	BaseSearchCredits = 1000
)

// Client represents a Jina AI API client
type Client struct {
	apiKey          string
	httpClient      *http.Client
	searchRetryCfg  providers.RetryConfig
	extractRetryCfg providers.RetryConfig
	searchTimeout   time.Duration
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

	searchTimeout := defaultSearchTimeout
	if t := os.Getenv("JINA_SEARCH_TIMEOUT"); t != "" {
		if d, err := time.ParseDuration(t); err == nil {
			searchTimeout = d
		}
	}

	// Search uses balanced retry policy to improve reliability without excessive cost.
	searchRetryCfg := providers.RetryConfig{
		MaxRetries:     2,
		InitialBackoff: 500 * time.Millisecond,
		MaxBackoff:     8 * time.Second,
		BackoffFactor:  2.0,
		RetryableErrors: []int{
			http.StatusTooManyRequests,     // 429
			http.StatusInternalServerError, // 500
			http.StatusBadGateway,          // 502
			http.StatusServiceUnavailable,  // 503
			http.StatusGatewayTimeout,      // 504
		},
	}

	// Extract/crawl keep conservative retries to limit token usage.
	extractRetryCfg := providers.RetryConfig{
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
		searchRetryCfg:  searchRetryCfg,
		extractRetryCfg: extractRetryCfg,
		searchTimeout:   searchTimeout,
	}, nil
}

// Name returns the provider name
func (c *Client) Name() string {
	return "jina"
}

// SupportsOperation returns whether Jina supports the given operation type
func (c *Client) SupportsOperation(opType string) bool {
	switch opType {
	case "search", "extract", "crawl":
		return true
	default:
		return false
	}
}

// Search performs a web search using Jina AI Search API
// Endpoint: GET https://s.jina.ai/?q={query}
// Returns top 5 results with full content in LLM-friendly format
func (c *Client) Search(ctx context.Context, query string, opts providers.SearchOptions) (*providers.SearchResult, error) {
	searchStart := time.Now()

	// Limit max results to prevent timeouts - Jina Search API can be slow with many results
	if opts.MaxResults > 3 {
		opts.MaxResults = 3
	}

	var result *providers.SearchResult
	var searchErr error

	err := c.searchRetryCfg.DoWithRetry(ctx, func() error {
		result, searchErr = c.searchInternal(ctx, query, opts)
		return searchErr
	})

	if err != nil {
		return nil, fmt.Errorf(
			"jina search failed after %d attempts in %s: %w",
			c.searchRetryCfg.MaxRetries+1,
			time.Since(searchStart).Round(time.Millisecond),
			err,
		)
	}
	if result != nil && opts.MaxResults > 0 && len(result.Results) > opts.MaxResults {
		result.Results = result.Results[:opts.MaxResults]
		result.TotalResults = len(result.Results)
	}
	return result, nil
}

// searchInternal performs the actual search request
func (c *Client) searchInternal(ctx context.Context, query string, _ providers.SearchOptions) (*providers.SearchResult, error) {
	start := time.Now()
	reqCtx := ctx
	cancel := func() {}
	if c.searchTimeout > 0 {
		reqCtx, cancel = context.WithTimeout(ctx, c.searchTimeout)
	}
	defer cancel()

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
	traceCtx, timing, finalize := newTraceContext(reqCtx)
	req, err := http.NewRequestWithContext(traceCtx, "GET", searchURL, nil)
	if err != nil {
		providers.LogError(ctx, err.Error(), "request_build", "search request")
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add authorization if API key is available
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		providers.LogError(ctx, err.Error(), "http", "search request failed")
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		providers.LogError(ctx, err.Error(), "http", "search response read failed")
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
			providers.LogError(ctx, string(respBody), "http_status", "search non-200 response")
			return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(respBody))
		}
		// Plain text error (likely rate limit)
		errMsg := strings.TrimSpace(string(respBody))
		if resp.StatusCode == http.StatusTooManyRequests || isRateLimitError(errMsg) {
			providers.LogError(ctx, errMsg, "rate_limit", "search non-200 response")
			return nil, fmt.Errorf("rate limit exceeded: %s", errMsg)
		}
		providers.LogError(ctx, errMsg, "http_status", "search non-200 response")
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, errMsg)
	}

	// Check if response is actually JSON
	contentType := resp.Header.Get("Content-Type")
	if !isJSONContent(contentType) {
		// Handle plain text response (rate limiting without proper status code)
		textResp := strings.TrimSpace(string(respBody))
		if isRateLimitError(textResp) {
			providers.LogError(ctx, textResp, "rate_limit", "search text response")
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
			providers.LogError(ctx, textBody, "rate_limit", "search parse response")
			return nil, fmt.Errorf("rate limit exceeded: %s", textBody)
		}
		providers.LogError(ctx, err.Error(), "parse", "search unmarshal failed")
		return nil, fmt.Errorf("failed to unmarshal response: %w (body: %s)", err, truncateString(textBody, 200))
	}

	// Check for API-level errors in JSON response
	if result.Code != 0 && result.Code != 200 {
		providers.LogError(ctx, fmt.Sprintf("code=%d", result.Code), "api_error", "search json response")
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

	err := c.extractRetryCfg.DoWithRetry(ctx, func() error {
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

	// Build reader URL - Jina expects the full URL appended after the base
	// Example: https://r.jina.ai/https://docs.python.org/3/tutorial/
	readerURL := readerBaseURL + "/" + pageURL

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
		providers.LogError(ctx, err.Error(), "request_build", "extract request")
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add authorization if API key is available
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		providers.LogError(ctx, err.Error(), "http", "extract request failed")
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		providers.LogError(ctx, err.Error(), "http", "extract response read failed")
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
			providers.LogError(ctx, errMsg, "rate_limit", "extract non-200 response")
			return nil, fmt.Errorf("rate limit exceeded: %s", errMsg)
		}
		providers.LogError(ctx, errMsg, "http_status", "extract non-200 response")
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
					providers.LogError(ctx, textBody, "rate_limit", "extract parse response")
					return nil, fmt.Errorf("rate limit exceeded: %s", textBody)
				}
				providers.LogError(ctx, err.Error(), "parse", "extract unmarshal failed")
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
	providers.LogRequest(ctx, method, url, headers, body)
}

// logResponse logs an HTTP response via the debug logger if available
func logResponse(ctx context.Context, statusCode int, headers map[string]string, body string, bodySize int, duration time.Duration) {
	providers.LogResponse(ctx, statusCode, headers, body, bodySize, duration)
}

// newTraceContext creates an httptrace context for timing breakdown if debug is enabled
func newTraceContext(ctx context.Context) (context.Context, *debug.TimingBreakdown, func()) {
	return providers.NewTraceContext(ctx)
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

// parsePlainTextSearchResponse attempts to parse plain text as search results.
func (c *Client) parsePlainTextSearchResponse(query, text string, startTime time.Time) (*providers.SearchResult, error) {
	latency := time.Since(startTime)
	text = strings.TrimSpace(text)

	if text == "" || isLikelyPlainTextError(text) {
		return nil, fmt.Errorf("API returned plain text error: %s", text)
	}

	items := make([]providers.SearchItem, 0, 5)
	fallbackLines := make([]string, 0, 8)

	type plainTextSearchItem struct {
		title        string
		url          string
		contentLines []string
	}

	var current *plainTextSearchItem
	flushCurrent := func() {
		if current == nil {
			return
		}
		title := strings.TrimSpace(current.title)
		content := strings.TrimSpace(strings.Join(current.contentLines, "\n"))
		if title == "" && content == "" && current.url == "" {
			current = nil
			return
		}
		if title == "" {
			title = "Search Result"
		}
		if content == "" {
			content = title
		}
		items = append(items, providers.SearchItem{
			Title:   title,
			URL:     strings.TrimSpace(current.url),
			Content: content,
		})
		current = nil
	}

	for _, rawLine := range strings.Split(text, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}

		field, value, ok := parseNumberedMetadataLine(line)
		if ok {
			switch strings.ToLower(field) {
			case "title":
				flushCurrent()
				current = &plainTextSearchItem{title: value}
			case "url source":
				if current == nil {
					current = &plainTextSearchItem{}
				}
				current.url = value
			case "description":
				if current == nil {
					current = &plainTextSearchItem{}
				}
				if value != "" {
					current.contentLines = append(current.contentLines, value)
				}
			default:
				if current != nil && value != "" {
					current.contentLines = append(current.contentLines, value)
				}
			}
			continue
		}

		if current != nil {
			current.contentLines = append(current.contentLines, line)
			continue
		}
		fallbackLines = append(fallbackLines, line)
	}
	flushCurrent()

	if len(items) == 0 {
		fallbackText := strings.TrimSpace(strings.Join(fallbackLines, "\n"))
		if fallbackText == "" {
			fallbackText = text
		}
		if isLikelyPlainTextError(fallbackText) {
			return nil, fmt.Errorf("API returned plain text error: %s", fallbackText)
		}
		items = []providers.SearchItem{
			{
				Title:   "Search Result",
				URL:     "",
				Content: fallbackText,
			},
		}
	}

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

func parseNumberedMetadataLine(line string) (field, value string, ok bool) {
	if !strings.HasPrefix(line, "[") {
		return "", "", false
	}
	closing := strings.Index(line, "]")
	if closing <= 1 || closing+1 >= len(line) {
		return "", "", false
	}

	rest := strings.TrimSpace(line[closing+1:])
	parts := strings.SplitN(rest, ":", 2)
	if len(parts) != 2 {
		return "", "", false
	}

	field = strings.TrimSpace(parts[0])
	value = strings.TrimSpace(parts[1])
	if field == "" {
		return "", "", false
	}
	return field, value, true
}

func isLikelyPlainTextError(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return true
	}
	lower := strings.ToLower(trimmed)
	if isRateLimitError(lower) {
		return true
	}

	errorPrefixes := []string{
		"error:",
		"error -",
		"request failed",
		"failed:",
		"{\"error\":",
		"{\"detail\":",
		"service unavailable",
	}
	for _, prefix := range errorPrefixes {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}

	// Short plain-text bodies that contain these patterns are usually API errors.
	if len(lower) < 200 {
		transientHints := []string{
			"unable to process request",
			"try again later",
			"temporarily unavailable",
		}
		for _, hint := range transientHints {
			if strings.Contains(lower, hint) {
				return true
			}
		}
	}

	return false
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
