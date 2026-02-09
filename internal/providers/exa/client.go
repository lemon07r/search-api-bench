// Package exa provides a client for the Exa AI Search API.
// It implements the providers.Provider interface for benchmarking search, extract, and crawl operations.
//
// Exa offers AI-native search with three modes:
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

// Search performs a web search using Exa AI API
// Endpoint: POST /search
// Native features leveraged:
//   - Three search modes: fast, auto, deep
//   - Query expansion for better results (deep mode)
//   - Domain filtering
//   - Full text content retrieval
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

	// Build request payload
	payload := map[string]interface{}{
		"query":         query,
		"type":          searchType,
		"numResults":    min(opts.MaxResults, 100), // Max 100
		"useAutoprompt": false,                     // Use exact query
		"text":          true,                      // Get full text content
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/search", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	respBody, err := c.retryCfg.DoHTTPRequest(ctx, c.httpClient, req)
	if err != nil {
		return nil, err
	}

	var result searchResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	latency := time.Since(start)

	// Convert Exa results to provider format
	items := make([]providers.SearchItem, 0, len(result.Results))
	for _, r := range result.Results {
		item := providers.SearchItem{
			Title:   r.Title,
			URL:     r.URL,
			Content: r.Text,
			Score:   r.Score,
		}

		// Parse published date if available
		if r.PublishedDate != "" {
			if t, err := time.Parse("2006-01-02", r.PublishedDate); err == nil {
				item.PublishedAt = &t
			}
		}

		items = append(items, item)
	}

	// Credits: 1 per request (estimated from docs)
	creditsUsed := 1

	return &providers.SearchResult{
		Query:        query,
		Results:      items,
		TotalResults: len(items),
		Latency:      latency,
		CreditsUsed:  creditsUsed,
		RawResponse:  respBody,
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

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/contents", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	respBody, err := c.retryCfg.DoHTTPRequest(ctx, c.httpClient, req)
	if err != nil {
		return nil, err
	}

	var result contentsResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	latency := time.Since(start)

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

	// Credits: 1 per request (estimated)
	creditsUsed := 1

	return &providers.ExtractResult{
		URL:         pageURL,
		Title:       r.Title,
		Content:     r.Text,
		Markdown:    r.Text, // Exa returns clean text, treat as markdown
		Metadata:    metadata,
		Latency:     latency,
		CreditsUsed: creditsUsed,
	}, nil
}

// Crawl crawls a website using Exa AI
// Strategy:
// 1. Search for site:domain.com with higher numResults to discover URLs
// 2. Use /contents to extract each discovered URL
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

	// Step 1: Search for pages on this domain
	searchQuery := fmt.Sprintf("site:%s", domain)
	searchOpts := providers.DefaultSearchOptions()
	searchOpts.MaxResults = opts.MaxPages * 3 // Request more to account for filtering
	searchOpts.SearchDepth = "basic"          // Use fast search for URL discovery

	searchResult, err := c.Search(ctx, searchQuery, searchOpts)
	if err != nil {
		// Fallback: just extract the starting URL
		return c.crawlSinglePage(ctx, startURL, opts, start)
	}

	// Step 2: Collect URLs to extract
	urls := make([]string, 0, opts.MaxPages)
	visited := make(map[string]bool)

	for _, item := range searchResult.Results {
		if len(urls) >= opts.MaxPages {
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

		urls = append(urls, item.URL)
	}

	// If no URLs found, fallback to single page
	if len(urls) == 0 {
		return c.crawlSinglePage(ctx, startURL, opts, start)
	}

	// Step 3: Batch extract using /contents endpoint
	creditsUsed := searchResult.CreditsUsed
	pages := make([]providers.CrawledPage, 0, len(urls))

	// Exa supports batch extraction - process in batches
	batchSize := 10
	for i := 0; i < len(urls); i += batchSize {
		end := i + batchSize
		if end > len(urls) {
			end = len(urls)
		}
		batch := urls[i:end]

		batchPages, batchCredits, err := c.extractBatch(ctx, batch)
		if err != nil {
			continue // Skip failed batches
		}

		pages = append(pages, batchPages...)
		creditsUsed += batchCredits
	}

	if len(pages) == 0 {
		return c.crawlSinglePage(ctx, startURL, opts, start)
	}

	latency := time.Since(start)

	return &providers.CrawlResult{
		URL:         startURL,
		Pages:       pages,
		TotalPages:  len(pages),
		Latency:     latency,
		CreditsUsed: creditsUsed,
	}, nil
}

// extractBatch extracts content from multiple URLs using a single /contents call
func (c *Client) extractBatch(ctx context.Context, urls []string) ([]providers.CrawledPage, int, error) {
	payload := map[string]interface{}{
		"urls": urls,
		"text": true,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/contents", bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	respBody, err := c.retryCfg.DoHTTPRequest(ctx, c.httpClient, req)
	if err != nil {
		return nil, 0, err
	}

	var result contentsResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, 0, err
	}

	pages := make([]providers.CrawledPage, 0, len(result.Results))
	for _, r := range result.Results {
		pages = append(pages, providers.CrawledPage{
			URL:      r.URL,
			Title:    r.Title,
			Content:  r.Text,
			Markdown: r.Text,
		})
	}

	// Credits: 1 per batch
	creditsUsed := 1

	return pages, creditsUsed, nil
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

// Response types

type searchResponse struct {
	RequestID string `json:"requestId"`
	Results   []struct {
		Title         string  `json:"title"`
		URL           string  `json:"url"`
		Text          string  `json:"text"`
		Score         float64 `json:"score"`
		PublishedDate string  `json:"publishedDate,omitempty"`
		Author        string  `json:"author,omitempty"`
	} `json:"results"`
	AutopromptString string `json:"autopromptString,omitempty"`
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
}
