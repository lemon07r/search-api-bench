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
	"regexp"
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

// Capabilities returns Mixedbread operation support levels.
func (c *Client) Capabilities() providers.CapabilitySet {
	return providers.CapabilitySet{
		Search:  providers.SupportNative,
		Extract: providers.SupportEmulated,
		Crawl:   providers.SupportEmulated,
	}
}

// SupportsOperation returns whether Mixedbread supports the given operation type
func (c *Client) SupportsOperation(opType string) bool {
	return c.Capabilities().SupportsOperation(opType)
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

	providers.LogRequest(ctx, "POST", searchURL, map[string]string{
		"Content-Type":  "application/json",
		"Authorization": "Bearer [REDACTED]",
	}, string(body))

	req, err := http.NewRequestWithContext(ctx, "POST", searchURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Search-API-Bench/1.0")

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

	// Credits: 1 search query made to the API
	// Cost calculator expects query count, not an internal estimate
	creditsUsed := 1

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

	resp, err := c.retryCfg.DoHTTPRequestDetailed(ctx, c.httpClient, req)
	if err != nil {
		return nil, err
	}
	providers.LogResponse(ctx, resp.StatusCode, providers.HeadersToMap(resp.Headers), string(resp.Body), len(resp.Body), time.Since(start))

	// Convert HTML to markdown
	content, err := md.ConvertString(string(resp.Body))
	if err != nil {
		content = string(resp.Body)
	}

	content = cleanContent(content)

	// Extract title
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
		URL:          pageURL,
		Title:        title,
		Content:      content,
		Markdown:     content,
		Metadata:     metadata,
		Latency:      latency,
		CreditsUsed:  0, // Direct fetch
		RequestCount: 1,
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
		URL:          startURL,
		Pages:        pages,
		TotalPages:   len(pages),
		Latency:      latency,
		CreditsUsed:  creditsUsed,
		RequestCount: len(pages),
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

func extractLinks(content, domain string) []string {
	seen := make(map[string]struct{})
	links := make([]string, 0, 16)

	addLink := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return
		}
		if strings.HasPrefix(raw, "/") {
			raw = "https://" + domain + raw
		}
		parsed, err := url.Parse(raw)
		if err != nil {
			return
		}
		if parsed.Host == "" {
			return
		}
		if parsed.Host != domain {
			return
		}
		parsed.Fragment = ""
		parsed.RawFragment = ""
		normalized := parsed.String()
		if _, ok := seen[normalized]; ok {
			return
		}
		seen[normalized] = struct{}{}
		links = append(links, normalized)
	}

	// Markdown links: [label](url)
	markdownLinkRe := regexp.MustCompile(`\[[^\]]+\]\(([^)\s]+)\)`)
	for _, match := range markdownLinkRe.FindAllStringSubmatch(content, -1) {
		if len(match) > 1 {
			addLink(match[1])
		}
	}

	// Plain absolute URLs
	plainURLRe := regexp.MustCompile(`https?://[^\s)\]">]+`)
	for _, raw := range plainURLRe.FindAllString(content, -1) {
		addLink(raw)
	}

	return links
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
