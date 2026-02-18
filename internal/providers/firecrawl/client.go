// Package firecrawl provides a client for the Firecrawl API (v2).
// It implements the providers.Provider interface for benchmarking search, extract, and crawl operations.
//
// API Documentation: https://docs.firecrawl.dev/api-reference
package firecrawl

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/lamim/search-api-bench/internal/providers"
)

const (
	defaultBaseURL = "https://api.firecrawl.dev/v2"
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

// Search performs a web search using Firecrawl v2
// Endpoint: POST /v2/search
func (c *Client) Search(ctx context.Context, query string, opts providers.SearchOptions) (*providers.SearchResult, error) {
	start := time.Now()

	payload := map[string]interface{}{
		"query": query,
		"limit": opts.MaxResults,
	}

	// Advanced search includes full scraping with markdown format
	if opts.SearchDepth == "advanced" {
		payload["scrapeOptions"] = map[string]interface{}{
			"formats": []interface{}{
				map[string]interface{}{"type": "markdown"},
			},
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

	items := make([]providers.SearchItem, 0, len(result.Data.Web))
	for _, d := range result.Data.Web {
		items = append(items, providers.SearchItem{
			Title:   d.Metadata.Title,
			URL:     d.Metadata.SourceURL,
			Content: d.Markdown,
		})
	}

	// Use actual creditsUsed from API response when available, otherwise estimate.
	// Firecrawl search: 2 credits per 10 results + 1 credit per scraped page if scrapeOptions used.
	creditsUsed := result.CreditsUsed
	if creditsUsed <= 0 {
		creditsUsed = 2
		if opts.SearchDepth == "advanced" {
			creditsUsed += len(items)
		}
	}

	return &providers.SearchResult{
		Query:         query,
		Results:       items,
		TotalResults:  len(items),
		Latency:       latency,
		CreditsUsed:   creditsUsed,
		RequestCount:  1,
		UsageReported: result.CreditsUsed > 0,
		RawResponse:   resp.Body,
	}, nil
}

// Extract extracts content from a URL using Firecrawl v2
// Endpoint: POST /v2/scrape
func (c *Client) Extract(ctx context.Context, url string, opts providers.ExtractOptions) (*providers.ExtractResult, error) {
	start := time.Now()

	payload := map[string]interface{}{
		"url": url,
		"formats": []interface{}{
			map[string]interface{}{"type": "markdown"},
		},
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

// Crawl crawls a website using Firecrawl v2
// Endpoint: POST /v2/crawl (async with polling)
func (c *Client) Crawl(ctx context.Context, url string, opts providers.CrawlOptions) (*providers.CrawlResult, error) {
	start := time.Now()
	maxPages, maxDepth := normalizeCrawlOptions(url, opts)

	payload := map[string]interface{}{
		"url":               url,
		"limit":             maxPages,
		"maxDiscoveryDepth": maxDepth, // v2 field name
		"scrapeOptions": map[string]interface{}{
			"formats": []interface{}{
				map[string]interface{}{"type": "markdown"},
			},
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

// normalizeCrawlOptions adjusts crawl parameters for Firecrawl.
// When MaxDepth is 0, it computes a reasonable depth from the seed URL's
// path segments so the crawl covers the subtree under that path.
func normalizeCrawlOptions(seedURL string, opts providers.CrawlOptions) (maxPages int, maxDepth int) {
	maxPages = opts.MaxPages
	if maxPages <= 0 {
		maxPages = 1
	}

	maxDepth = opts.MaxDepth
	if maxDepth < 0 {
		maxDepth = 0
	}

	// When depth is 0, auto-calculate from the seed URL path depth.
	// A deeper seed URL needs a higher maxDepth to discover sub-pages.
	if maxDepth == 0 {
		maxDepth = seedURLPathDepth(seedURL)
		if maxDepth < 1 {
			maxDepth = 1
		}
	}

	return maxPages, maxDepth
}

// seedURLPathDepth counts the number of non-empty path segments in a URL.
func seedURLPathDepth(rawURL string) int {
	u, err := url.Parse(rawURL)
	if err != nil {
		return 1
	}
	path := strings.Trim(u.Path, "/")
	if path == "" {
		return 1
	}
	return len(strings.Split(path, "/"))
}

func (c *Client) waitForCrawl(ctx context.Context, crawlID string, start time.Time) (*providers.CrawlResult, error) {
	checkURL := c.baseURL + "/crawl/" + crawlID
	var allPages []providers.CrawledPage
	requestCount := 1 // Initial crawl request

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
		requestCount++

		var status crawlStatusResponse
		if err := json.Unmarshal(resp.Body, &status); err != nil {
			return nil, fmt.Errorf("failed to unmarshal status response: %w", err)
		}

		// Collect pages from this response
		for _, d := range status.Data {
			allPages = append(allPages, providers.CrawledPage{
				URL:      d.Metadata.SourceURL,
				Title:    d.Metadata.Title,
				Content:  d.Markdown,
				Markdown: d.Markdown,
			})
		}

		switch status.Status {
		case "completed":
			latency := time.Since(start)

			// Use actual creditsUsed from API response when available
			creditsUsed := status.CreditsUsed
			if creditsUsed <= 0 {
				creditsUsed = len(allPages)
			}

			return &providers.CrawlResult{
				URL:           status.URL,
				Pages:         allPages,
				TotalPages:    len(allPages),
				Latency:       latency,
				CreditsUsed:   creditsUsed,
				RequestCount:  requestCount,
				UsageReported: status.CreditsUsed > 0,
			}, nil

		case "failed", "scraping_failed":
			// Return partial results if we have any data, otherwise error
			if len(allPages) > 0 {
				latency := time.Since(start)
				creditsUsed := status.CreditsUsed
				if creditsUsed <= 0 {
					creditsUsed = len(allPages)
				}
				return &providers.CrawlResult{
					URL:           status.URL,
					Pages:         allPages,
					TotalPages:    len(allPages),
					Latency:       latency,
					CreditsUsed:   creditsUsed,
					RequestCount:  requestCount,
					UsageReported: status.CreditsUsed > 0,
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
			// Handle v2 pagination: if there's a next URL, follow it
			if status.Next != "" {
				checkURL = status.Next
			}
			// Continue polling for: scraping, scheduled, etc.
			continue
		}
	}
}

// Response types for Firecrawl v2 API

type searchResponse struct {
	Success     bool `json:"success"`
	CreditsUsed int  `json:"creditsUsed,omitempty"`
	Data        struct {
		Web []struct {
			Markdown string `json:"markdown"`
			Metadata struct {
				Title     string `json:"title"`
				SourceURL string `json:"sourceURL"`
			} `json:"metadata"`
		} `json:"web"`
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
	Status      string `json:"status"`
	URL         string `json:"url"`
	Error       string `json:"error,omitempty"`
	Next        string `json:"next,omitempty"`        // v2 pagination URL
	CreditsUsed int    `json:"creditsUsed,omitempty"` // v2 actual credit usage
	Data        []struct {
		Markdown string `json:"markdown"`
		Metadata struct {
			Title     string `json:"title"`
			SourceURL string `json:"sourceURL"`
		} `json:"metadata"`
	} `json:"data"`
}
