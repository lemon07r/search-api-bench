// Package tavily provides a client for the Tavily API.
// It implements the providers.Provider interface for benchmarking search, extract, and crawl operations.
package tavily

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
	defaultBaseURL = "https://api.tavily.com"
)

// Client represents a Tavily API client
type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
	retryCfg   providers.RetryConfig
}

// NewClient creates a new Tavily client
func NewClient() (*Client, error) {
	apiKey := os.Getenv("TAVILY_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("TAVILY_API_KEY environment variable not set")
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
	return "tavily"
}

// Capabilities returns Tavily operation support levels.
// Crawl uses map+extract pattern (emulated), not native /crawl endpoint.
func (c *Client) Capabilities() providers.CapabilitySet {
	return providers.CapabilitySet{
		Search:  providers.SupportNative,
		Extract: providers.SupportNative,
		Crawl:   providers.SupportEmulated,
	}
}

// SupportsOperation returns whether Tavily supports the given operation type
func (c *Client) SupportsOperation(opType string) bool {
	return c.Capabilities().SupportsOperation(opType)
}

// Search performs a web search using Tavily
func (c *Client) Search(ctx context.Context, query string, opts providers.SearchOptions) (*providers.SearchResult, error) {
	start := time.Now()

	searchDepth := "basic"
	if opts.SearchDepth == "advanced" {
		searchDepth = "advanced"
	}

	payload := map[string]interface{}{
		"query":          query,
		"search_depth":   searchDepth,
		"max_results":    opts.MaxResults,
		"include_answer": opts.IncludeAnswer,
		"include_images": opts.IncludeImages,
	}

	if opts.TimeRange != "" {
		payload["time_range"] = opts.TimeRange
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

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

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

	items := make([]providers.SearchItem, 0, len(result.Results))
	for _, r := range result.Results {
		item := providers.SearchItem{
			Title:   r.Title,
			URL:     r.URL,
			Content: r.Content,
			Score:   r.Score,
		}
		if r.PublishedDate != "" {
			if t, err := time.Parse("2006-01-02", r.PublishedDate); err == nil {
				item.PublishedAt = &t
			}
		}
		items = append(items, item)
	}

	creditsUsed := 1
	if searchDepth == "advanced" {
		creditsUsed = 2
	}

	return &providers.SearchResult{
		Query:        query,
		Results:      items,
		TotalResults: len(items),
		Latency:      latency,
		CreditsUsed:  creditsUsed,
		RequestCount: 1,
		RawResponse:  resp.Body,
	}, nil
}

// Extract extracts content from a URL using Tavily Extract API
// Leverages native capabilities: extract_depth for basic vs comprehensive extraction
func (c *Client) Extract(ctx context.Context, url string, opts providers.ExtractOptions) (*providers.ExtractResult, error) {
	start := time.Now()

	// Map SearchDepth to extract_depth: basic -> basic, advanced -> advanced
	// Tavily API only accepts 'basic' or 'advanced' (not 'comprehensive')
	extractDepth := "basic"
	if opts.Format == "advanced" || opts.IncludeMetadata {
		extractDepth = "advanced"
	}

	payload := map[string]interface{}{
		"urls":          []string{url},
		"extract_depth": extractDepth,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	reqURL := c.baseURL + "/extract"
	providers.LogRequest(ctx, "POST", reqURL, map[string]string{
		"Content-Type":  "application/json",
		"Authorization": "Bearer [REDACTED]",
	}, string(body))

	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.retryCfg.DoHTTPRequestDetailed(ctx, c.httpClient, req)
	if err != nil {
		providers.LogError(ctx, err.Error(), "http", "extract request failed")
		return nil, err
	}

	var result extractResponse
	if err := json.Unmarshal(resp.Body, &result); err != nil {
		providers.LogError(ctx, err.Error(), "parse", "failed to unmarshal extract response")
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	latency := time.Since(start)
	providers.LogResponse(ctx, resp.StatusCode, providers.HeadersToMap(resp.Headers), string(resp.Body), len(resp.Body), latency)

	if len(result.Results) == 0 {
		return nil, fmt.Errorf("no extraction results returned")
	}

	r := result.Results[0]

	// Tavily Extract credits: basic = 1 per 5 URLs, advanced = 2 per 5 URLs.
	// For a single URL we round up to 1 credit (basic) or 2 credits (advanced).
	creditsUsed := 1
	if extractDepth == "advanced" {
		creditsUsed = 2
	}

	return &providers.ExtractResult{
		URL:          url,
		Title:        r.Title,
		Content:      r.RawContent,
		Markdown:     r.RawContent,
		Metadata:     map[string]interface{}{"images": r.Images},
		Latency:      latency,
		CreditsUsed:  creditsUsed,
		RequestCount: 1,
	}, nil
}

// Crawl crawls a website using Tavily (uses map + extract pattern).
// This is an emulated crawl - Tavily has a native /crawl endpoint but
// this implementation uses map to discover URLs then extract to fetch content.
func (c *Client) Crawl(ctx context.Context, url string, opts providers.CrawlOptions) (*providers.CrawlResult, error) {
	start := time.Now()

	// First, use Tavily Map to get URLs
	payload := map[string]interface{}{
		"url":   url,
		"limit": opts.MaxPages,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	reqURL := c.baseURL + "/map"
	providers.LogRequest(ctx, "POST", reqURL, map[string]string{
		"Content-Type":  "application/json",
		"Authorization": "Bearer [REDACTED]",
	}, string(body))

	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.retryCfg.DoHTTPRequestDetailed(ctx, c.httpClient, req)
	if err != nil {
		providers.LogError(ctx, err.Error(), "http", "map request failed")
		return nil, err
	}

	var mapResult mapResponse
	if err := json.Unmarshal(resp.Body, &mapResult); err != nil {
		providers.LogError(ctx, err.Error(), "parse", "failed to unmarshal map response")
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	providers.LogResponse(ctx, resp.StatusCode, providers.HeadersToMap(resp.Headers), string(resp.Body), len(resp.Body), time.Since(start))

	pages := make([]providers.CrawledPage, 0, len(mapResult.Results))
	creditsUsed := 1 // Map API cost
	extractErrors := 0

	// Handle empty map results
	if len(mapResult.Results) == 0 {
		// Try to extract the starting URL directly as a fallback
		extractOpts := providers.ExtractOptions{
			Format:          "comprehensive",
			IncludeMetadata: true,
		}

		extractResult, err := c.Extract(ctx, url, extractOpts)
		if err != nil {
			return nil, fmt.Errorf("map returned no URLs and extract failed: %w", err)
		}

		pages = append(pages, providers.CrawledPage{
			URL:      url,
			Title:    extractResult.Title,
			Content:  extractResult.Content,
			Markdown: extractResult.Markdown,
		})
		creditsUsed += extractResult.CreditsUsed
	} else {
		// For each URL, extract content
		for _, mappedURL := range mapResult.Results {
			if len(pages) >= opts.MaxPages {
				break
			}

			extractOpts := providers.ExtractOptions{
				Format:          "comprehensive",
				IncludeMetadata: true,
			}

			extractResult, err := c.Extract(ctx, mappedURL, extractOpts)
			if err != nil {
				extractErrors++
				continue // Skip failed extractions but count them
			}

			pages = append(pages, providers.CrawledPage{
				URL:      mappedURL,
				Title:    extractResult.Title,
				Content:  extractResult.Content,
				Markdown: extractResult.Markdown,
			})

			creditsUsed += extractResult.CreditsUsed
		}
	}

	// If we have no pages but had errors, report the failure
	if len(pages) == 0 {
		if extractErrors > 0 {
			return nil, fmt.Errorf("crawl failed: all %d extractions failed", extractErrors)
		}
		return nil, fmt.Errorf("crawl failed: no pages extracted")
	}

	latency := time.Since(start)

	return &providers.CrawlResult{
		URL:          url,
		Pages:        pages,
		TotalPages:   len(pages),
		Latency:      latency,
		CreditsUsed:  creditsUsed,
		RequestCount: 1 + len(pages),
	}, nil
}

// Response types
type searchResponse struct {
	Answer       string  `json:"answer"`
	Query        string  `json:"query"`
	ResponseTime float64 `json:"response_time"`
	Results      []struct {
		Title         string  `json:"title"`
		URL           string  `json:"url"`
		Content       string  `json:"content"`
		Score         float64 `json:"score"`
		PublishedDate string  `json:"published_date"`
	} `json:"results"`
}

type extractResponse struct {
	Results []struct {
		URL        string   `json:"url"`
		Title      string   `json:"title"`
		RawContent string   `json:"raw_content"`
		Images     []string `json:"images"`
	} `json:"results"`
	FailedResults []struct {
		URL   string `json:"url"`
		Error string `json:"error"`
	} `json:"failed_results"`
}

type mapResponse struct {
	Results []string `json:"results"`
}
