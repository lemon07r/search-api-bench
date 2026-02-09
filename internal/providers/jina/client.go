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
	"time"

	"github.com/lamim/search-api-bench/internal/providers"
)

const (
	readerBaseURL  = "https://r.jina.ai"
	searchBaseURL  = "https://s.jina.ai"
	apiBaseURL     = "https://api.jina.ai"
	defaultTimeout = 60 * time.Second
)

// Client represents a Jina AI API client
type Client struct {
	apiKey     string
	httpClient *http.Client
}

// NewClient creates a new Jina AI client
func NewClient() (*Client, error) {
	apiKey := os.Getenv("JINA_API_KEY")
	// Jina works without API key but with lower rate limits
	return &Client{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
	}, nil
}

// Name returns the provider name
func (c *Client) Name() string {
	return "jina"
}

// Search performs a web search using Jina AI Search API
// Endpoint: GET https://s.jina.ai/?q={query}
// Returns top 5 results with full content in LLM-friendly format
func (c *Client) Search(ctx context.Context, query string, _ providers.SearchOptions) (*providers.SearchResult, error) {
	start := time.Now()

	// Build search URL with query parameter
	searchURL := fmt.Sprintf("%s/?q=%s", searchBaseURL, url.QueryEscape(query))

	// Add JSON format for structured response
	searchURL += "&format=json"

	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
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

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result searchResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	latency := time.Since(start)

	// Convert Jina results to provider format
	items := make([]providers.SearchItem, 0, len(result.Data))
	for _, r := range result.Data {
		items = append(items, providers.SearchItem{
			Title:   r.Title,
			URL:     r.URL,
			Content: r.Content,
		})
	}

	// Jina Search costs 10,000 tokens per request (fixed)
	creditsUsed := 10000

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

	req, err := http.NewRequestWithContext(ctx, "GET", readerURL, nil)
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

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	latency := time.Since(start)

	var content, title string
	var metadata map[string]interface{}

	if opts.IncludeMetadata {
		var result readerResponse
		if err := json.Unmarshal(respBody, &result); err != nil {
			return nil, fmt.Errorf("failed to unmarshal response: %w", err)
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
		// Try to extract title from first line if it's markdown
		title = extractTitleFromMarkdown(content)
		metadata = map[string]interface{}{}
	}

	// Credits based on output token count (approximate)
	creditsUsed := len(content) / 4 // Rough token estimation

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
// Since Jina doesn't have native crawl, we simulate it via:
// 1. Search for site:domain.com to discover URLs
// 2. Extract each discovered URL
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
	searchOpts.MaxResults = opts.MaxPages

	searchResult, err := c.Search(ctx, searchQuery, searchOpts)
	if err != nil {
		// Fallback: just extract the starting URL
		return c.crawlSinglePage(ctx, startURL, opts, start)
	}

	// Step 2: Extract each discovered URL
	pages := make([]providers.CrawledPage, 0, opts.MaxPages)
	creditsUsed := searchResult.CreditsUsed
	extractOpts := providers.DefaultExtractOptions()

	for _, item := range searchResult.Results {
		if len(pages) >= opts.MaxPages {
			break
		}

		// Skip if not same domain
		itemURL, err := url.Parse(item.URL)
		if err != nil {
			continue
		}
		if itemURL.Host != domain {
			continue
		}

		extractResult, err := c.Extract(ctx, item.URL, extractOpts)
		if err != nil {
			continue // Skip failed extractions
		}

		pages = append(pages, providers.CrawledPage{
			URL:      item.URL,
			Title:    extractResult.Title,
			Content:  extractResult.Content,
			Markdown: extractResult.Markdown,
		})

		creditsUsed += extractResult.CreditsUsed
	}

	// If no pages were extracted, fallback to single page
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
