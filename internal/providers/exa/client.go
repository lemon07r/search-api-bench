// Package exa provides a client for the Exa AI Search API.
// It implements the providers.Provider interface for benchmarking search, extract, and crawl operations.
//
// Exa offers AI-native search with multiple modes:
// - Fast: <500ms latency, optimized for speed
// - Auto: Balanced quality and speed (default)
// - Deep: Agentic search with query expansion for highest quality
//
// API Documentation: https://docs.exa.ai/reference/quickstart
package exa

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
	defaultBaseURL = "https://api.exa.ai"
	defaultTimeout = 60 * time.Second
)

// Client represents an Exa AI API client
type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
	retryCfg   providers.RetryConfig
}

// NewClient creates a new Exa AI client
func NewClient() (*Client, error) {
	apiKey := os.Getenv("EXA_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("EXA_API_KEY environment variable not set")
	}

	return &Client{
		apiKey:  apiKey,
		baseURL: defaultBaseURL,
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
		retryCfg: providers.DefaultRetryConfig(),
	}, nil
}

// Name returns the provider name
func (c *Client) Name() string {
	return "exa"
}

// Capabilities returns Exa operation support levels.
func (c *Client) Capabilities() providers.CapabilitySet {
	return providers.CapabilitySet{
		Search:  providers.SupportNative,
		Extract: providers.SupportNative,
		Crawl:   providers.SupportEmulated, // Uses search + contents, not native subpages
	}
}

// SupportsOperation returns whether Exa supports the given operation type
func (c *Client) SupportsOperation(opType string) bool {
	return c.Capabilities().SupportsOperation(opType)
}

// Search performs a web search using Exa AI API
// Endpoint: POST /search
// Native features leveraged:
//   - Multiple search modes: fast, auto, deep
//   - Content retrieval via contents object
//   - Domain filtering via includeDomains
func (c *Client) Search(ctx context.Context, query string, opts providers.SearchOptions) (*providers.SearchResult, error) {
	start := time.Now()

	// Map search depth to Exa search type
	var searchType string
	switch opts.SearchDepth {
	case "basic":
		searchType = "fast"
	case "advanced":
		searchType = "deep"
	default:
		searchType = "auto"
	}

	// Build request payload per Exa API spec
	payload := map[string]interface{}{
		"query":      query,
		"type":       searchType,
		"numResults": min(opts.MaxResults, 100),
		// Content retrieval must be nested in "contents" object
		"contents": map[string]interface{}{
			"text": true,
		},
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

	// Convert Exa results to provider format
	items := make([]providers.SearchItem, 0, len(result.Results))
	for _, r := range result.Results {
		content := r.Text
		if content == "" {
			content = r.Summary
		}

		item := providers.SearchItem{
			Title:   r.Title,
			URL:     r.URL,
			Content: content,
			Score:   r.Score,
		}

		// Parse published date if available
		if publishedAt, ok := parseExaPublishedAt(r.PublishedDate); ok {
			item.PublishedAt = publishedAt
		}

		items = append(items, item)
	}

	// Use actual cost from API response when available, otherwise estimate
	costUSD := result.CostDollars.Total
	creditsUsed := 1 // Fallback for cost calculator
	usageReported := costUSD > 0

	return &providers.SearchResult{
		Query:         query,
		Results:       items,
		TotalResults:  len(items),
		Latency:       latency,
		CreditsUsed:   creditsUsed,
		RequestCount:  1,
		UsageReported: usageReported,
		RawResponse:   resp.Body,
	}, nil
}

// Extract extracts content from URLs using Exa AI /contents endpoint
// Endpoint: POST /contents
// Native content extraction - one of Exa's key features
func (c *Client) Extract(ctx context.Context, pageURL string, opts providers.ExtractOptions) (*providers.ExtractResult, error) {
	start := time.Now()

	// Build request payload
	payload := map[string]interface{}{
		"urls": []string{pageURL},
		"text": true,
	}

	if opts.IncludeMetadata {
		payload["highlights"] = true
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	reqURL := c.baseURL + "/contents"
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

	var result contentsResponse
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

	// Build metadata
	metadata := map[string]interface{}{}
	if opts.IncludeMetadata {
		metadata["author"] = r.Author
		metadata["image"] = r.Image
		if len(r.Highlights) > 0 {
			metadata["highlights"] = r.Highlights
		}
	}

	creditsUsed := 1
	usageReported := result.CostDollars.Total > 0

	return &providers.ExtractResult{
		URL:           pageURL,
		Title:         r.Title,
		Content:       r.Text,
		Markdown:      r.Text, // Exa returns clean text, treat as markdown
		Metadata:      metadata,
		Latency:       latency,
		CreditsUsed:   creditsUsed,
		RequestCount:  1,
		UsageReported: usageReported,
	}, nil
}

// Crawl crawls a website using Exa AI
// Strategy: Use includeDomains to search for pages on the target domain,
// then batch extract content via /contents endpoint.
func (c *Client) Crawl(ctx context.Context, startURL string, opts providers.CrawlOptions) (*providers.CrawlResult, error) {
	start := time.Now()

	// Parse URL to get domain
	parsedURL, err := url.Parse(startURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	if parsedURL.Scheme == "" {
		parsedURL.Scheme = "https"
	}

	domain := parsedURL.Host

	// Step 1: Search for pages on this domain using includeDomains filter
	searchPayload := map[string]interface{}{
		"query":          parsedURL.Path, // Use path as query hint
		"type":           "auto",
		"numResults":     opts.MaxPages * 3, // Request more to account for filtering
		"includeDomains": []string{domain},
		"contents": map[string]interface{}{
			"text": true,
		},
	}

	// If the path is just "/" or empty, use the domain as query
	if parsedURL.Path == "" || parsedURL.Path == "/" {
		searchPayload["query"] = domain
	}

	searchBody, err := json.Marshal(searchPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal search request: %w", err)
	}

	searchReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/search", bytes.NewReader(searchBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create search request: %w", err)
	}

	searchReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	searchReq.Header.Set("Content-Type", "application/json")

	searchResp, err := c.retryCfg.DoHTTPRequestDetailed(ctx, c.httpClient, searchReq)
	if err != nil {
		// Fallback: just extract the starting URL
		return c.crawlSinglePage(ctx, startURL, opts, start)
	}

	var searchResult searchResponse
	if err := json.Unmarshal(searchResp.Body, &searchResult); err != nil {
		return c.crawlSinglePage(ctx, startURL, opts, start)
	}

	// Step 2: Collect pages (search already includes text content)
	pages := make([]providers.CrawledPage, 0, opts.MaxPages)
	visited := make(map[string]bool)

	for _, item := range searchResult.Results {
		if len(pages) >= opts.MaxPages {
			break
		}

		// Skip duplicates
		if visited[item.URL] {
			continue
		}
		visited[item.URL] = true

		// Skip if not same domain
		itemURL, err := url.Parse(item.URL)
		if err != nil {
			continue
		}
		if itemURL.Host != domain {
			continue
		}

		content := item.Text
		if content == "" {
			content = item.Summary
		}

		pages = append(pages, providers.CrawledPage{
			URL:      item.URL,
			Title:    item.Title,
			Content:  content,
			Markdown: content,
		})
	}

	// If no pages found, fallback to single page
	if len(pages) == 0 {
		return c.crawlSinglePage(ctx, startURL, opts, start)
	}

	latency := time.Since(start)

	creditsUsed := 1 // Search request
	usageReported := searchResult.CostDollars.Total > 0

	return &providers.CrawlResult{
		URL:           startURL,
		Pages:         pages,
		TotalPages:    len(pages),
		Latency:       latency,
		CreditsUsed:   creditsUsed,
		RequestCount:  1,
		UsageReported: usageReported,
	}, nil
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
		URL:          pageURL,
		Pages:        pages,
		TotalPages:   1,
		Latency:      latency,
		CreditsUsed:  extractResult.CreditsUsed,
		RequestCount: 1,
	}, nil
}

// Response types

type costDollars struct {
	Total float64 `json:"total,omitempty"`
}

type searchResponse struct {
	RequestID   string         `json:"requestId"`
	SearchType  string         `json:"searchType,omitempty"`
	Results     []exaSearchHit `json:"results"`
	CostDollars costDollars    `json:"costDollars,omitempty"`
}

type exaSearchHit struct {
	Title         string  `json:"title"`
	URL           string  `json:"url"`
	Text          string  `json:"text,omitempty"`
	Summary       string  `json:"summary,omitempty"`
	Score         float64 `json:"score,omitempty"`
	PublishedDate string  `json:"publishedDate,omitempty"`
	Author        string  `json:"author,omitempty"`
}

type contentsResponse struct {
	Results []struct {
		URL        string   `json:"url"`
		Title      string   `json:"title"`
		Text       string   `json:"text"`
		Author     string   `json:"author,omitempty"`
		Image      string   `json:"image,omitempty"`
		Highlights []string `json:"highlights,omitempty"`
	} `json:"results"`
	CostDollars costDollars `json:"costDollars,omitempty"`
}

func parseExaPublishedAt(value string) (*time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, false
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02",
	}
	for _, layout := range layouts {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return &parsed, true
		}
	}
	return nil, false
}
