// Package tavily provides a client for the Tavily API.
// It implements the providers.Provider interface for benchmarking search, extract, and crawl operations.
package tavily

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	}, nil
}

// Name returns the provider name
func (c *Client) Name() string {
	return "tavily"
}

// Search performs a web search using Tavily
func (c *Client) Search(ctx context.Context, query string, opts providers.SearchOptions) (*providers.SearchResult, error) {
	start := time.Now()

	searchDepth := "basic"
	if opts.SearchDepth == "advanced" {
		searchDepth = "advanced"
	}

	payload := map[string]interface{}{
		"api_key":        c.apiKey,
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

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/search", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

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

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result searchResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	latency := time.Since(start)

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
		RawResponse:  respBody,
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
		"api_key":       c.apiKey,
		"urls":          []string{url},
		"extract_depth": extractDepth,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/extract", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

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

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result extractResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	latency := time.Since(start)

	if len(result.Results) == 0 {
		return nil, fmt.Errorf("no extraction results returned")
	}

	r := result.Results[0]

	// Tavily Extract: 1 credit per 5 successful extractions (basic)
	creditsUsed := 1

	return &providers.ExtractResult{
		URL:         url,
		Title:       r.Title,
		Content:     r.RawContent,
		Markdown:    r.RawContent,
		Metadata:    map[string]interface{}{"images": r.Images},
		Latency:     latency,
		CreditsUsed: creditsUsed,
	}, nil
}

// Crawl crawls a website using Tavily (uses map + extract)
func (c *Client) Crawl(ctx context.Context, url string, opts providers.CrawlOptions) (*providers.CrawlResult, error) {
	start := time.Now()

	// First, use Tavily Map to get URLs
	payload := map[string]interface{}{
		"api_key": c.apiKey,
		"url":     url,
		"limit":   opts.MaxPages,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/map", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

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

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var mapResult mapResponse
	if err := json.Unmarshal(respBody, &mapResult); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

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
		URL:         url,
		Pages:       pages,
		TotalPages:  len(pages),
		Latency:     latency,
		CreditsUsed: creditsUsed,
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
	Links   []string `json:"links"`
	Results []string `json:"results"`
}
