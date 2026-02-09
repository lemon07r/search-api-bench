// Package firecrawl provides a client for the Firecrawl API.
// It implements the providers.Provider interface for benchmarking search, extract, and crawl operations.
package firecrawl

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
	defaultBaseURL = "https://api.firecrawl.dev/v1"
)

// Client represents a Firecrawl API client
type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
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
	}, nil
}

// Name returns the provider name
func (c *Client) Name() string {
	return "firecrawl"
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

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/search", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
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
		RawResponse:  respBody,
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

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/scrape", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
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

	var result scrapeResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	latency := time.Since(start)

	return &providers.ExtractResult{
		URL:         url,
		Title:       result.Data.Metadata.Title,
		Content:     result.Data.Markdown,
		Markdown:    result.Data.Markdown,
		Metadata:    result.Data.Metadata.Raw,
		Latency:     latency,
		CreditsUsed: 1, // Firecrawl scrape uses 1 credit
	}, nil
}

// Crawl crawls a website using Firecrawl
// Leverages native capabilities: maxDepth, scrape options, exclude paths
func (c *Client) Crawl(ctx context.Context, url string, opts providers.CrawlOptions) (*providers.CrawlResult, error) {
	start := time.Now()

	payload := map[string]interface{}{
		"url":      url,
		"limit":    opts.MaxPages,
		"maxDepth": opts.MaxDepth, // Firecrawl supports max depth natively
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

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/crawl", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
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

	var result crawlResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// Wait for crawl to complete (poll for status)
	if result.ID != "" {
		return c.waitForCrawl(ctx, result.ID, start)
	}

	latency := time.Since(start)

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
		URL:         url,
		Pages:       pages,
		TotalPages:  len(pages),
		Latency:     latency,
		CreditsUsed: len(pages), // Each page costs 1 credit
	}, nil
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

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("status request failed: %w", err)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("failed to read status response: %w", err)
		}
		if err := resp.Body.Close(); err != nil {
			return nil, err
		}

		var status crawlStatusResponse
		if err := json.Unmarshal(body, &status); err != nil {
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
				URL:         status.URL,
				Pages:       pages,
				TotalPages:  len(pages),
				Latency:     latency,
				CreditsUsed: len(pages),
			}, nil

		case "failed", "scraping_failed":
			// Return partial results if we have any data, otherwise error
			if len(pages) > 0 {
				latency := time.Since(start)
				return &providers.CrawlResult{
					URL:         status.URL,
					Pages:       pages,
					TotalPages:  len(pages),
					Latency:     latency,
					CreditsUsed: len(pages),
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
