// Package firecrawl provides a client for the Firecrawl API.
// It implements the providers.Provider interface for benchmarking search, extract, and crawl operations.
package firecrawl

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/lamim/search-api-bench/internal/providers"
)

const (
	defaultBaseURL = "https://api.firecrawl.dev/v1"
)

// Client represents a Firecrawl API client
type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
	retryCfg   providers.RetryConfig
}

// NewClient creates a new Firecrawl client
func NewClient() (*Client, error) {
	apiKey := os.Getenv("FIRECRAWL_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("FIRECRAWL_API_KEY environment variable not set")
	}

	return &Client{
		apiKey:  apiKey,
		baseURL: defaultBaseURL,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
		retryCfg: providers.DefaultRetryConfig(),
	}, nil
}

// Name returns the provider name
func (c *Client) Name() string {
	return "firecrawl"
}

// Capabilities returns Firecrawl operation support levels.
func (c *Client) Capabilities() providers.CapabilitySet {
	return providers.CapabilitySet{
		Search:  providers.SupportNative,
		Extract: providers.SupportNative,
		Crawl:   providers.SupportNative,
	}
}

// SupportsOperation returns whether Firecrawl supports the given operation type
func (c *Client) SupportsOperation(opType string) bool {
	return c.Capabilities().SupportsOperation(opType)
}

// Search performs a web search using Firecrawl
// Leverages native capabilities: scrape options, location settings
func (c *Client) Search(ctx context.Context, query string, opts providers.SearchOptions) (*providers.SearchResult, error) {
	start := time.Now()

	payload := map[string]interface{}{
		"query": query,
		"limit": opts.MaxResults,
		"lang":  "en", // Default to English for consistent results
	}

	// Advanced search includes full scraping with markdown format
	if opts.SearchDepth == "advanced" {
		payload["scrapeOptions"] = map[string]interface{}{
			"formats": []string{"markdown"},
		}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	reqURL := c.baseURL + "/search"
	providers.LogRequest(ctx, "POST", reqURL, map[string]string{
		"Content-Type":  "application/json",
		"Authorization": "Bearer [REDACTED]",
	}, string(body))

	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.retryCfg.DoHTTPRequestDetailed(ctx, c.httpClient, req)
	if err != nil {
		providers.LogError(ctx, err.Error(), "http", "search request failed")
		return nil, err
	}

	var result searchResponse
	if err := json.Unmarshal(resp.Body, &result); err != nil {
		providers.LogError(ctx, err.Error(), "parse", "failed to unmarshal search response")
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	latency := time.Since(start)
	providers.LogResponse(ctx, resp.StatusCode, providers.HeadersToMap(resp.Headers), string(resp.Body), len(resp.Body), latency)

	items := make([]providers.SearchItem, 0, len(result.Data))
	for _, d := range result.Data {
		items = append(items, providers.SearchItem{
			Title:   d.Metadata.Title,
			URL:     d.Metadata.SourceURL,
			Content: d.Markdown,
		})
	}

	return &providers.SearchResult{
		Query:        query,
		Results:      items,
		TotalResults: len(items),
		Latency:      latency,
		CreditsUsed:  1, // Firecrawl search uses 1 credit per request
		RequestCount: 1,
		RawResponse:  resp.Body,
	}, nil
}

// Extract extracts content from a URL using Firecrawl
func (c *Client) Extract(ctx context.Context, url string, opts providers.ExtractOptions) (*providers.ExtractResult, error) {
	start := time.Now()

	payload := map[string]interface{}{
		"url":     url,
		"formats": []string{"markdown"},
	}

	if opts.IncludeMetadata {
		payload["onlyMainContent"] = false
	} else {
		payload["onlyMainContent"] = true
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	reqURL := c.baseURL + "/scrape"
	providers.LogRequest(ctx, "POST", reqURL, map[string]string{
		"Content-Type":  "application/json",
		"Authorization": "Bearer [REDACTED]",
	}, string(body))

	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.retryCfg.DoHTTPRequestDetailed(ctx, c.httpClient, req)
	if err != nil {
		providers.LogError(ctx, err.Error(), "http", "extract request failed")
		return nil, err
	}

	var result scrapeResponse
	if err := json.Unmarshal(resp.Body, &result); err != nil {
		providers.LogError(ctx, err.Error(), "parse", "failed to unmarshal extract response")
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	latency := time.Since(start)
	providers.LogResponse(ctx, resp.StatusCode, providers.HeadersToMap(resp.Headers), string(resp.Body), len(resp.Body), latency)

	return &providers.ExtractResult{
		URL:          url,
		Title:        result.Data.Metadata.Title,
		Content:      result.Data.Markdown,
		Markdown:     result.Data.Markdown,
		Metadata:     result.Data.Metadata.Raw,
		Latency:      latency,
		CreditsUsed:  1, // Firecrawl scrape uses 1 credit
		RequestCount: 1,
	}, nil
}

// Crawl crawls a website using Firecrawl
// Leverages native capabilities: maxDepth, scrape options, exclude paths
func (c *Client) Crawl(ctx context.Context, url string, opts providers.CrawlOptions) (*providers.CrawlResult, error) {
	start := time.Now()
	maxPages, maxDepth := normalizeCrawlOptions(url, opts)

	payload := map[string]interface{}{
		"url":      url,
		"limit":    maxPages,
		"maxDepth": maxDepth, // Firecrawl supports max depth natively
		"scrapeOptions": map[string]interface{}{
			"formats": []string{"markdown"},
		},
	}

	// Add exclude paths if provided
	if len(opts.ExcludePaths) > 0 {
		payload["excludePaths"] = opts.ExcludePaths
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	reqURL := c.baseURL + "/crawl"
	providers.LogRequest(ctx, "POST", reqURL, map[string]string{
		"Content-Type":  "application/json",
		"Authorization": "Bearer [REDACTED]",
	}, string(body))

	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.retryCfg.DoHTTPRequestDetailed(ctx, c.httpClient, req)
	if err != nil {
		providers.LogError(ctx, err.Error(), "http", "crawl request failed")
		return nil, err
	}

	var result crawlResponse
	if err := json.Unmarshal(resp.Body, &result); err != nil {
		providers.LogError(ctx, err.Error(), "parse", "failed to unmarshal crawl response")
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// Wait for crawl to complete (poll for status)
	if result.ID != "" {
		return c.waitForCrawl(ctx, result.ID, start)
	}

	latency := time.Since(start)
	providers.LogResponse(ctx, resp.StatusCode, providers.HeadersToMap(resp.Headers), string(resp.Body), len(resp.Body), latency)

	pages := make([]providers.CrawledPage, 0, len(result.Data))
	for _, d := range result.Data {
		pages = append(pages, providers.CrawledPage{
			URL:      d.Metadata.SourceURL,
			Title:    d.Metadata.Title,
			Content:  d.Markdown,
			Markdown: d.Markdown,
		})
	}

	return &providers.CrawlResult{
		URL:           url,
		Pages:         pages,
		TotalPages:    len(pages),
		Latency:       latency,
		CreditsUsed:   len(pages), // Each page costs 1 credit
		RequestCount:  1,
		UsageReported: true,
	}, nil
}

func normalizeCrawlOptions(_ string, opts providers.CrawlOptions) (maxPages int, maxDepth int) {
	maxPages = opts.MaxPages
	if maxPages <= 0 {
		maxPages = 1
	}

	maxDepth = opts.MaxDepth
	if maxDepth < 0 {
		maxDepth = 0
	}

	return maxPages, maxDepth
}

func (c *Client) waitForCrawl(ctx context.Context, crawlID string, start time.Time) (*providers.CrawlResult, error) {
	checkURL := c.baseURL + "/crawl/" + crawlID

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}

		req, err := http.NewRequestWithContext(ctx, "GET", checkURL, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create status request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)

		resp, err := c.retryCfg.DoHTTPRequestDetailed(ctx, c.httpClient, req)
		if err != nil {
			return nil, fmt.Errorf("status request failed: %w", err)
		}

		var status crawlStatusResponse
		if err := json.Unmarshal(resp.Body, &status); err != nil {
			return nil, fmt.Errorf("failed to unmarshal status response: %w", err)
		}

		// Build result from available data (even partial)
		pages := make([]providers.CrawledPage, 0, len(status.Data))
		for _, d := range status.Data {
			pages = append(pages, providers.CrawledPage{
				URL:      d.Metadata.SourceURL,
				Title:    d.Metadata.Title,
				Content:  d.Markdown,
				Markdown: d.Markdown,
			})
		}

		switch status.Status {
		case "completed":
			latency := time.Since(start)
			return &providers.CrawlResult{
				URL:           status.URL,
				Pages:         pages,
				TotalPages:    len(pages),
				Latency:       latency,
				CreditsUsed:   len(pages),
				RequestCount:  2,
				UsageReported: true,
			}, nil

		case "failed", "scraping_failed":
			// Return partial results if we have any data, otherwise error
			if len(pages) > 0 {
				latency := time.Since(start)
				return &providers.CrawlResult{
					URL:           status.URL,
					Pages:         pages,
					TotalPages:    len(pages),
					Latency:       latency,
					CreditsUsed:   len(pages),
					RequestCount:  2,
					UsageReported: true,
				}, nil
			}
			errMsg := status.Error
			if errMsg == "" {
				errMsg = "crawl failed with no error details"
			}
			return nil, fmt.Errorf("crawl %s: %s", status.Status, errMsg)

		case "cancelled":
			return nil, fmt.Errorf("crawl was cancelled")

		default:
			// Continue polling for: scraping, scheduled, etc.
			continue
		}
	}
}

// Response types
type searchResponse struct {
	Success bool `json:"success"`
	Data    []struct {
		Markdown string `json:"markdown"`
		Metadata struct {
			Title     string `json:"title"`
			SourceURL string `json:"sourceURL"`
		} `json:"metadata"`
	} `json:"data"`
}

type scrapeResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Markdown string `json:"markdown"`
		Metadata struct {
			Title     string                 `json:"title"`
			SourceURL string                 `json:"sourceURL"`
			Raw       map[string]interface{} `json:"-"`
		} `json:"metadata"`
	} `json:"data"`
}

type crawlResponse struct {
	Success bool   `json:"success"`
	ID      string `json:"id"`
	URL     string `json:"url"`
	Data    []struct {
		Markdown string `json:"markdown"`
		Metadata struct {
			Title     string `json:"title"`
			SourceURL string `json:"sourceURL"`
		} `json:"metadata"`
	} `json:"data"`
}

type crawlStatusResponse struct {
	Status string `json:"status"`
	URL    string `json:"url"`
	Error  string `json:"error,omitempty"`
	Data   []struct {
		Markdown string `json:"markdown"`
		Metadata struct {
			Title     string `json:"title"`
			SourceURL string `json:"sourceURL"`
		} `json:"metadata"`
	} `json:"data"`
}
