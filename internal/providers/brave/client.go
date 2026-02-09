// Package brave provides a client for the Brave Search API.
// It implements the providers.Provider interface for benchmarking search, extract, and crawl operations.
//
// Brave Search API offers:
// - Web Search: Billions of indexed pages with freshness filtering
// - Local Search: Points of interest data (Pro subscription)
// - Rich Search: Real-time data like weather, stocks (Pro subscription)
//
// API Documentation: https://api-dashboard.search.brave.com/app/documentation/web-search/get-started
package brave

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	md "github.com/JohannesKaufmann/html-to-markdown/v2"
	"github.com/lamim/search-api-bench/internal/providers"
)

const (
	defaultBaseURL = "https://api.search.brave.com/res/v1"
	defaultTimeout = 60 * time.Second
)

// Client represents a Brave Search API client
type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
	retryCfg   providers.RetryConfig
}

// NewClient creates a new Brave Search client
func NewClient() (*Client, error) {
	apiKey := os.Getenv("BRAVE_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("BRAVE_API_KEY environment variable not set")
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
	return "brave"
}

// SupportsOperation returns whether Brave supports the given operation type
func (c *Client) SupportsOperation(opType string) bool {
	switch opType {
	case "search", "extract", "crawl":
		return true
	default:
		return false
	}
}

// Search performs a web search using Brave Search API
// Endpoint: GET /res/v1/web/search
// Features leveraged:
//   - Freshness filtering (pd, pw, pm, py)
//   - Extra snippets for more content
//   - Country/language targeting
//   - Pagination support
func (c *Client) Search(ctx context.Context, query string, opts providers.SearchOptions) (*providers.SearchResult, error) {
	start := time.Now()

	// Build query parameters
	params := url.Values{}
	params.Set("q", query)
	params.Set("count", fmt.Sprintf("%d", min(opts.MaxResults, 20))) // Max 20 per request
	params.Set("extra_snippets", "true")                             // Get more content per result

	// Map TimeRange to freshness parameter
	if opts.TimeRange != "" {
		freshness := mapTimeRangeToFreshness(opts.TimeRange)
		if freshness != "" {
			params.Set("freshness", freshness)
		}
	}

	// Add offset for pagination if needed
	params.Set("offset", "0")

	// Safe search (default moderate)
	params.Set("safesearch", "moderate")

	searchURL := fmt.Sprintf("%s/web/search?%s", c.baseURL, params.Encode())

	providers.LogRequest(ctx, "GET", searchURL, map[string]string{
		"X-Subscription-Token": "[REDACTED]",
		"Accept":               "application/json",
	}, "")

	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("X-Subscription-Token", c.apiKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Search-API-Bench/1.0")

	resp, err := c.retryCfg.DoHTTPRequestDetailed(ctx, c.httpClient, req)
	if err != nil {
		providers.LogError(ctx, err.Error(), "http", "search request failed")
		return nil, err
	}

	var result webSearchResponse
	if err := json.Unmarshal(resp.Body, &result); err != nil {
		providers.LogError(ctx, err.Error(), "parse", "failed to unmarshal search response")
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	latency := time.Since(start)
	providers.LogResponse(ctx, resp.StatusCode, providers.HeadersToMap(resp.Headers), string(resp.Body), len(resp.Body), latency)

	// Convert Brave results to provider format
	items := make([]providers.SearchItem, 0, len(result.Web.Results))
	for _, r := range result.Web.Results {
		// Combine description and extra snippets for content
		content := r.Description
		if len(r.ExtraSnippets) > 0 {
			content += "\n\n" + strings.Join(r.ExtraSnippets, "\n")
		}

		item := providers.SearchItem{
			Title:   r.Title,
			URL:     r.URL,
			Content: content,
		}

		// Parse age if available
		if r.Age != "" {
			item.PublishedAt = parseBraveAge(r.Age)
		}

		items = append(items, item)
	}

	// Credits: 1 per request (based on pricing page)
	creditsUsed := 1

	return &providers.SearchResult{
		Query:        query,
		Results:      items,
		TotalResults: len(items),
		Latency:      latency,
		CreditsUsed:  creditsUsed,
		RawResponse:  resp.Body,
	}, nil
}

// Extract extracts content from a URL using direct HTTP fetch
// Brave Search doesn't have a native extract endpoint, so we fetch directly
// and convert HTML to markdown.
func (c *Client) Extract(ctx context.Context, pageURL string, opts providers.ExtractOptions) (*providers.ExtractResult, error) {
	start := time.Now()

	// Ensure URL has scheme
	if !strings.HasPrefix(pageURL, "http://") && !strings.HasPrefix(pageURL, "https://") {
		pageURL = "https://" + pageURL
	}

	req, err := http.NewRequestWithContext(ctx, "GET", pageURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers to mimic browser
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")

	resp, err := c.retryCfg.DoHTTPRequestDetailed(ctx, c.httpClient, req)
	if err != nil {
		return nil, err
	}
	providers.LogResponse(ctx, resp.StatusCode, providers.HeadersToMap(resp.Headers), string(resp.Body), len(resp.Body), time.Since(start))

	// Convert HTML to markdown
	content, err := md.ConvertString(string(resp.Body))
	if err != nil {
		// Fallback to plain text
		content = string(resp.Body)
	}

	// Clean up content
	content = cleanContent(content)

	// Extract title from content or use URL
	title := extractTitle(string(resp.Body))
	if title == "" {
		title = pageURL
	}

	latency := time.Since(start)

	metadata := map[string]interface{}{}
	if opts.IncludeMetadata {
		metadata["source"] = "direct-fetch"
	}

	return &providers.ExtractResult{
		URL:         pageURL,
		Title:       title,
		Content:     content,
		Markdown:    content,
		Metadata:    metadata,
		Latency:     latency,
		CreditsUsed: 0, // Direct fetch - no API credits
	}, nil
}

// Crawl crawls a website using Brave Search
// Strategy:
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

	// Step 1: Search for pages on this domain using site: operator
	searchQuery := fmt.Sprintf("site:%s", domain)
	searchOpts := providers.DefaultSearchOptions()
	searchOpts.MaxResults = opts.MaxPages * 2 // Request more to account for filtering

	searchResult, err := c.Search(ctx, searchQuery, searchOpts)
	if err != nil {
		// Fallback: just extract the starting URL
		return c.crawlSinglePage(ctx, startURL, opts, start)
	}

	// Step 2: Extract each discovered URL
	pages := make([]providers.CrawledPage, 0, opts.MaxPages)
	creditsUsed := searchResult.CreditsUsed
	extractOpts := providers.DefaultExtractOptions()
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

		// Direct fetch doesn't use credits
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
		CreditsUsed: 0,
	}, nil
}

// Helper functions

func mapTimeRangeToFreshness(timeRange string) string {
	switch timeRange {
	case "day":
		return "pd" // Past day
	case "week":
		return "pw" // Past week
	case "month":
		return "pm" // Past month
	case "year":
		return "py" // Past year
	default:
		return ""
	}
}

func parseBraveAge(_ string) *time.Time {
	// Brave age format examples: "1 day ago", "2 hours ago", "1 week ago"
	// For simplicity, return nil - we could parse this more precisely if needed
	return nil
}

func extractTitle(html string) string {
	// Simple extraction of <title> tag
	start := strings.Index(html, "<title>")
	end := strings.Index(html, "</title>")
	if start != -1 && end != -1 && end > start+7 {
		return strings.TrimSpace(html[start+7 : end])
	}
	return ""
}

func cleanContent(content string) string {
	// Remove excessive whitespace
	for strings.Contains(content, "\n\n\n") {
		content = strings.ReplaceAll(content, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(content)
}

// Response types

type webSearchResponse struct {
	Query struct {
		Original             string `json:"original"`
		MoreResultsAvailable bool   `json:"more_results_available"`
	} `json:"query"`
	Web struct {
		Results []struct {
			Title         string   `json:"title"`
			URL           string   `json:"url"`
			Description   string   `json:"description"`
			ExtraSnippets []string `json:"extra_snippets,omitempty"`
			Age           string   `json:"age,omitempty"`
		} `json:"results"`
	} `json:"web"`
}
