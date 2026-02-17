// Package jina provides a client for the Jina AI Reader and Search APIs.
// It implements the providers.Provider interface for benchmarking search, extract, and crawl operations.
//
// Jina AI offers two main endpoints:
// - Reader API (r.jina.ai): Converts URLs to LLM-friendly markdown
// - Search API (s.jina.ai): Searches web and returns web results in LLM-friendly formats
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
	"strconv"
	"strings"
	"time"

	"github.com/lamim/search-api-bench/internal/debug"
	"github.com/lamim/search-api-bench/internal/providers"
)

const (
	readerBaseURL  = "https://r.jina.ai"
	searchBaseURL  = "https://s.jina.ai"
	defaultTimeout = 30 * time.Second
	// defaultSearchTimeout bounds each individual search attempt.
	// Kept lower to reduce spend on slow responses by default.
	defaultSearchTimeout = 12 * time.Second

	defaultSearchMaxResults      = 10
	defaultSearchMaxRetries      = 0
	defaultExtractMaxRetries     = 0
	defaultExtractTokenBudget    = 6000
	defaultCrawlTokenBudget      = 4000
	defaultSearchNoContent       = true
	defaultEnableImageCaptioning = false
)

// Client represents a Jina AI API client
type Client struct {
	apiKey           string
	httpClient       *http.Client
	readerBaseURL    string
	searchBaseURL    string
	searchRetryCfg   providers.RetryConfig
	extractRetryCfg  providers.RetryConfig
	searchTimeout    time.Duration
	searchMaxResult  int
	searchNoContent  bool
	extractBudget    int
	crawlBudget      int
	withGeneratedAlt bool
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

	searchMaxRetries := parseNonNegativeIntEnv("JINA_SEARCH_MAX_RETRIES", defaultSearchMaxRetries)
	extractMaxRetries := parseNonNegativeIntEnv("JINA_EXTRACT_MAX_RETRIES", defaultExtractMaxRetries)
	searchMaxResults := parsePositiveIntEnv("JINA_SEARCH_MAX_RESULTS", defaultSearchMaxResults)
	extractBudget := parsePositiveIntEnv("JINA_EXTRACT_TOKEN_BUDGET", defaultExtractTokenBudget)
	crawlBudget := parsePositiveIntEnv("JINA_CRAWL_TOKEN_BUDGET", defaultCrawlTokenBudget)
	searchNoContent := parseBoolEnv("JINA_SEARCH_NO_CONTENT", defaultSearchNoContent)
	withGeneratedAlt := parseBoolEnv("JINA_WITH_GENERATED_ALT", defaultEnableImageCaptioning)

	// Search uses balanced retry policy to improve reliability without excessive cost.
	searchRetryCfg := providers.RetryConfig{
		MaxRetries:     searchMaxRetries,
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
		MaxRetries:     extractMaxRetries,
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
		readerBaseURL:    readerBaseURL,
		searchBaseURL:    searchBaseURL,
		searchRetryCfg:   searchRetryCfg,
		extractRetryCfg:  extractRetryCfg,
		searchTimeout:    searchTimeout,
		searchMaxResult:  searchMaxResults,
		searchNoContent:  searchNoContent,
		extractBudget:    extractBudget,
		crawlBudget:      crawlBudget,
		withGeneratedAlt: withGeneratedAlt,
	}, nil
}

// Name returns the provider name
func (c *Client) Name() string {
	return "jina"
}

// Capabilities returns Jina operation support levels.
func (c *Client) Capabilities() providers.CapabilitySet {
	return providers.CapabilitySet{
		Search:  providers.SupportNative,
		Extract: providers.SupportNative,
		Crawl:   providers.SupportEmulated,
	}
}

// SupportsOperation returns whether Jina supports the given operation type
func (c *Client) SupportsOperation(opType string) bool {
	return c.Capabilities().SupportsOperation(opType)
}

// Search performs a web search using Jina AI Search API.
// Endpoint: GET https://s.jina.ai/?q={query}
// Default mode uses X-Respond-With:no-content for lower token spend.
func (c *Client) Search(ctx context.Context, query string, opts providers.SearchOptions) (*providers.SearchResult, error) {
	searchStart := time.Now()

	// Use caller max results when provided. Fall back to provider default.
	if opts.MaxResults <= 0 {
		opts.MaxResults = c.searchMaxResult
	}

	// Use content-rich mode when caller requests advanced search.
	noContent := c.searchNoContent
	if opts.SearchDepth == "advanced" {
		noContent = false
	}

	var result *providers.SearchResult
	var searchErr error

	err := c.searchRetryCfg.DoWithRetry(ctx, func() error {
		result, searchErr = c.searchInternal(ctx, query, opts, noContent)
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
func (c *Client) searchInternal(ctx context.Context, query string, opts providers.SearchOptions, noContent bool) (*providers.SearchResult, error) {
	start := time.Now()
	reqCtx := ctx
	cancel := func() {}
	if c.searchTimeout > 0 {
		reqCtx, cancel = context.WithTimeout(ctx, c.searchTimeout)
	}
	defer cancel()

	topN := opts.MaxResults
	if topN <= 0 {
		topN = c.searchMaxResult
	}
	if topN <= 0 {
		topN = defaultSearchMaxResults
	}
	// Build search URL with query parameter.
	searchURL := fmt.Sprintf("%s/?q=%s&top_n=%d", c.searchBaseURL, url.QueryEscape(query), topN)

	// Prepare headers
	headers := make(map[string]string)
	if c.apiKey != "" {
		headers["Authorization"] = "Bearer " + c.apiKey
	}
	if noContent {
		headers["X-Respond-With"] = "no-content"
	}
	if !c.withGeneratedAlt {
		headers["X-Retain-Images"] = "none"
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
	if noContent {
		req.Header.Set("X-Respond-With", "no-content")
	}
	if !c.withGeneratedAlt {
		req.Header.Set("X-Retain-Images", "none")
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

	// Jina bills by processed tokens; when usage isn't returned, estimate from body length.
	creditsUsed := tokenEstimateFromBytes(respBody)
	if creditsUsed == 0 {
		creditsUsed = tokenEstimateFromChars(totalContentLen)
	}

	return &providers.SearchResult{
		Query:         query,
		Results:       items,
		TotalResults:  len(items),
		Latency:       latency,
		CreditsUsed:   creditsUsed,
		RequestCount:  1,
		UsageReported: false,
		RawResponse:   respBody,
	}, nil
}

// Extract extracts content from a URL using Jina AI Reader API
// Endpoint: GET https://r.jina.ai/http://{url}
// Converts any URL to clean, LLM-friendly markdown
func (c *Client) Extract(ctx context.Context, pageURL string, opts providers.ExtractOptions) (*providers.ExtractResult, error) {
	return c.executeExtractWithRetry(ctx, pageURL, opts, c.extractBudget)
}

func (c *Client) executeExtractWithRetry(ctx context.Context, pageURL string, opts providers.ExtractOptions, tokenBudget int) (*providers.ExtractResult, error) {
	var result *providers.ExtractResult
	var extractErr error

	err := c.extractRetryCfg.DoWithRetry(ctx, func() error {
		result, extractErr = c.extractInternal(ctx, pageURL, opts, tokenBudget)
		return extractErr
	})

	if err != nil {
		return nil, err
	}
	return result, nil
}

// extractInternal performs the actual extract request
func (c *Client) extractInternal(ctx context.Context, pageURL string, _ providers.ExtractOptions, tokenBudget int) (*providers.ExtractResult, error) {
	start := time.Now()

	// Ensure URL has scheme
	if !hasScheme(pageURL) {
		pageURL = "https://" + pageURL
	}

	// Build reader URL - Jina expects the full URL appended after the base
	// Example: https://r.jina.ai/https://docs.python.org/3/tutorial/
	readerURL := c.readerBaseURL + "/" + pageURL

	// Prepare headers
	headers := make(map[string]string)
	if c.apiKey != "" {
		headers["Authorization"] = "Bearer " + c.apiKey
	}
	if tokenBudget > 0 {
		headers["X-Token-Budget"] = fmt.Sprintf("%d", tokenBudget)
	}
	if !c.withGeneratedAlt {
		headers["X-Retain-Images"] = "none"
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
	if tokenBudget > 0 {
		req.Header.Set("X-Token-Budget", fmt.Sprintf("%d", tokenBudget))
	}
	if !c.withGeneratedAlt {
		req.Header.Set("X-Retain-Images", "none")
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

	// Check if response is actually JSON
	contentType := resp.Header.Get("Content-Type")
	if isJSONContent(contentType) {
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
	} else {
		// Plain text response
		content = string(respBody)
		title = extractTitle(content)
		metadata = extractMetadata(content)
	}

	// Credits based on output token count (approximate).
	creditsUsed := tokenEstimateFromString(content)

	return &providers.ExtractResult{
		URL:           pageURL,
		Title:         title,
		Content:       content,
		Markdown:      content,
		Metadata:      metadata,
		Latency:       latency,
		CreditsUsed:   creditsUsed,
		RequestCount:  1,
		UsageReported: false,
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
	extractResult, err := c.executeExtractWithRetry(ctx, pageURL, extractOpts, c.crawlBudget)
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
		URL:          pageURL,
		Pages:        pages,
		TotalPages:   1,
		Latency:      latency,
		CreditsUsed:  extractResult.CreditsUsed,
		RequestCount: 1,
	}, nil
}

// Helper functions

func hasScheme(s string) bool {
	return len(s) > 7 && (s[:7] == "http://" || s[:8] == "https://")
}

func parseBoolEnv(name string, defaultValue bool) bool {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return defaultValue
	}
	switch strings.ToLower(raw) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return defaultValue
	}
}

func parsePositiveIntEnv(name string, defaultValue int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return defaultValue
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return defaultValue
	}
	return v
}

func parseNonNegativeIntEnv(name string, defaultValue int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return defaultValue
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < 0 {
		return defaultValue
	}
	return v
}

func tokenEstimateFromString(text string) int {
	return tokenEstimateFromChars(len(text))
}

func tokenEstimateFromBytes(body []byte) int {
	return tokenEstimateFromChars(len(body))
}

func tokenEstimateFromChars(chars int) int {
	if chars <= 0 {
		return 0
	}
	tokens := chars / 4
	if tokens == 0 {
		return 1
	}
	return tokens
}

func extractTitle(content string) string {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(strings.ToLower(line), "title:") {
			return strings.TrimSpace(line[len("title:"):])
		}
		break
	}
	return extractTitleFromMarkdown(content)
}

func extractMetadata(content string) map[string]interface{} {
	meta := map[string]interface{}{}
	for _, line := range strings.Split(content, "\n") {
		field, value, ok := parseMetadataLine(line)
		if !ok {
			continue
		}
		switch strings.ToLower(field) {
		case "url source":
			meta["url"] = value
		case "published time", "date":
			meta["published"] = value
		}
	}
	return meta
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

	creditsUsed := tokenEstimateFromString(text)

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

func parseMetadataLine(line string) (field, value string, ok bool) {
	parts := strings.SplitN(strings.TrimSpace(line), ":", 2)
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
