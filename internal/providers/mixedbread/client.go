// Package mixedbread provides a client for the Mixedbread AI Search API.
// It implements the providers.Provider interface for benchmarking search, extract, and crawl operations.
//
// Mixedbread offers:
// - Search API: Multimodal, multilingual search (launched Oct 2025)
// - Reranking API: State-of-the-art reranking models (mxbai-rerank-v2 series)
// - Embeddings API: High-quality embeddings for various use cases
// - Parsing API: Document processing for complex formats
//
// API Documentation: https://www.mixedbread.com/docs
package mixedbread

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

	md "github.com/JohannesKaufmann/html-to-markdown/v2"
	"github.com/lamim/search-api-bench/internal/providers"
)

const (
	defaultBaseURL = "https://api.mixedbread.com"
	defaultTimeout = 60 * time.Second
)

// Client represents a Mixedbread AI API client
type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
	retryCfg   providers.RetryConfig
}

// NewClient creates a new Mixedbread AI client
func NewClient() (*Client, error) {
	// Support both MXB_API_KEY (short form) and MIXEDBREAD_API_KEY (verbose form)
	apiKey := os.Getenv("MXB_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("MIXEDBREAD_API_KEY")
	}
	if apiKey == "" {
		return nil, fmt.Errorf("MXB_API_KEY (or MIXEDBREAD_API_KEY) environment variable not set")
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
	return "mixedbread"
}

// Search performs a web search using Mixedbread AI Search API
// Endpoint: POST /v1/stores/search
// Uses the public "mixedbread/web" store for real-time web search
// Documentation: https://www.mixedbread.com/docs/stores/search
func (c *Client) Search(ctx context.Context, query string, opts providers.SearchOptions) (*providers.SearchResult, error) {
	start := time.Now()

	// Mixedbread Stores Search API with web store
	// The "mixedbread/web" store provides real-time web search capabilities
	payload := map[string]interface{}{
		"query":             query,
		"store_identifiers": []string{"mixedbread/web"},
		"top_k":             opts.MaxResults,
		"search_options": map[string]interface{}{
			"rerank": true, // Web search results are always reranked for optimal relevance
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Correct endpoint for Mixedbread Stores Search API
	searchURL := c.baseURL + "/v1/stores/search"

	req, err := http.NewRequestWithContext(ctx, "POST", searchURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Search-API-Bench/1.0")

	respBody, err := c.retryCfg.DoHTTPRequest(ctx, c.httpClient, req)
	if err != nil {
		return nil, err
	}

	var result searchResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	latency := time.Since(start)

	// Convert Mixedbread results to provider format
	// Response structure: result.Data[].{Text, Score, Metadata.{Title, URL}}
	items := make([]providers.SearchItem, 0, len(result.Data))
	for _, r := range result.Data {
		items = append(items, providers.SearchItem{
			Title:   r.Metadata.Title,
			URL:     r.Metadata.URL,
			Content: r.Text,
			Score:   r.Score,
		})
	}

	// Credits: Web search queries are billed as "search with rerank query"
	// Rough estimate based on query complexity and result count
	creditsUsed := 10 + len(query)*2 + len(items)*5

	return &providers.SearchResult{
		Query:        query,
		Results:      items,
		TotalResults: len(items),
		Latency:      latency,
		CreditsUsed:  creditsUsed,
		RawResponse:  respBody,
	}, nil
}

// Extract extracts content from a URL using direct HTTP fetch
// Mixedbread doesn't have a native extract endpoint, so we fetch directly
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
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	respBody, err := c.retryCfg.DoHTTPRequest(ctx, c.httpClient, req)
	if err != nil {
		return nil, err
	}

	// Convert HTML to markdown
	content, err := md.ConvertString(string(respBody))
	if err != nil {
		content = string(respBody)
	}

	content = cleanContent(content)

	// Extract title
	title := extractTitle(string(respBody))
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
		CreditsUsed: 0, // Direct fetch
	}, nil
}

// Crawl crawls a website using Mixedbread AI
// Strategy:
// 1. Use direct fetch to get the starting page
// 2. Parse links and follow them up to MaxPages/MaxDepth
// Note: Mixedbread doesn't have native web search/crawl, so this is limited
func (c *Client) Crawl(ctx context.Context, startURL string, opts providers.CrawlOptions) (*providers.CrawlResult, error) {
	start := time.Now()

	// Parse URL
	parsedURL, err := url.Parse(startURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	if parsedURL.Scheme == "" {
		parsedURL.Scheme = "https"
		startURL = parsedURL.String()
	}

	domain := parsedURL.Host

	// Since Mixedbread doesn't have a web search API accessible to us,
	// we'll do a simple breadth-first crawl using direct fetches
	pages := make([]providers.CrawledPage, 0, opts.MaxPages)
	visited := make(map[string]bool)
	toVisit := []string{startURL}
	creditsUsed := 0
	depth := 0

	extractOpts := providers.DefaultExtractOptions()

	for len(toVisit) > 0 && len(pages) < opts.MaxPages && depth <= opts.MaxDepth {
		nextLevel := make([]string, 0)

		for _, urlStr := range toVisit {
			if len(pages) >= opts.MaxPages {
				break
			}

			// Skip if already visited
			if visited[urlStr] {
				continue
			}
			visited[urlStr] = true

			// Extract page
			extractResult, err := c.Extract(ctx, urlStr, extractOpts)
			if err != nil {
				continue // Skip failed pages
			}

			pages = append(pages, providers.CrawledPage{
				URL:      urlStr,
				Title:    extractResult.Title,
				Content:  extractResult.Content,
				Markdown: extractResult.Markdown,
			})

			creditsUsed += extractResult.CreditsUsed

			// Parse links from content for next level (simplified)
			if depth < opts.MaxDepth {
				links := extractLinks(extractResult.Content, domain)
				for _, link := range links {
					if !visited[link] {
						nextLevel = append(nextLevel, link)
					}
				}
			}
		}

		toVisit = nextLevel
		depth++
	}

	if len(pages) == 0 {
		return nil, fmt.Errorf("crawl failed: no pages could be extracted")
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

// Helper functions

func extractTitle(html string) string {
	start := strings.Index(html, "<title>")
	end := strings.Index(html, "</title>")
	if start != -1 && end != -1 && end > start+7 {
		return strings.TrimSpace(html[start+7 : end])
	}
	return ""
}

func cleanContent(content string) string {
	for strings.Contains(content, "\n\n\n") {
		content = strings.ReplaceAll(content, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(content)
}

func extractLinks(_, _ string) []string {
	// Simplified link extraction - in practice would parse HTML
	// For now, return empty (this is a limitation of the simple approach)
	return []string{}
}

// Response types for Mixedbread Stores Search API
// API Documentation: https://www.mixedbread.com/docs/stores/search/web-store

type searchResponse struct {
	Object string `json:"object"`
	Data   []struct {
		Type     string  `json:"type"`
		Text     string  `json:"text"`
		Score    float64 `json:"score"`
		FileID   string  `json:"file_id"`
		Filename string  `json:"filename"`
		Metadata struct {
			Title  string `json:"title"`
			URL    string `json:"url"`
			Source string `json:"source"`
		} `json:"metadata"`
	} `json:"data"`
}
