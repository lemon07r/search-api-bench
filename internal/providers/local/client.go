// Package local provides a client for local web crawling and scraping.
// It implements the providers.Provider interface using Colly for crawling
// and html-to-markdown for content conversion.
//
// This provider demonstrates what can be achieved with pure Go libraries
// without relying on paid APIs, showing the trade-offs in terms of
// capabilities (no JS rendering, no search index) vs cost (free).
package local

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	md "github.com/JohannesKaufmann/html-to-markdown/v2"
	"github.com/gocolly/colly/v2"
	"github.com/lamim/search-api-bench/internal/providers"
)

// Client represents a local crawler/scraper using Colly
type Client struct {
	httpClient *http.Client
}

// NewClient creates a new local crawler client
func NewClient() (*Client, error) {
	return &Client{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

// Name returns the provider name
func (c *Client) Name() string {
	return "local"
}

// Search is not supported for a local crawler as it cannot index the entire web.
// This operation requires a search index which only APIs like Tavily/Firecrawl provide.
func (c *Client) Search(_ context.Context, _ string, _ providers.SearchOptions) (*providers.SearchResult, error) {
	return nil, fmt.Errorf("search operation is not supported by the local crawler provider: local crawlers cannot index the web like search engines")
}

// Extract visits a single URL and converts the HTML content to Markdown.
// It extracts the title and main content from the page.
func (c *Client) Extract(ctx context.Context, pageURL string, opts providers.ExtractOptions) (*providers.ExtractResult, error) {
	start := time.Now()

	// Validate URL
	parsedURL, err := url.Parse(pageURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	// Ensure URL has a scheme
	if parsedURL.Scheme == "" {
		parsedURL.Scheme = "https"
	}

	var (
		title       string
		htmlContent string
		extractErr  error
		done        bool
	)

	// Create collector (synchronous mode for single page)
	collector := colly.NewCollector(
		colly.UserAgent("Search-API-Bench/1.0 (Local Crawler)"),
		colly.MaxDepth(1),
	)

	// Set up context cancellation handling
	collector.OnRequest(func(r *colly.Request) {
		select {
		case <-ctx.Done():
			r.Abort()
			extractErr = ctx.Err()
		default:
		}
	})

	// Extract title
	collector.OnHTML("title", func(e *colly.HTMLElement) {
		title = strings.TrimSpace(e.Text)
	})

	// Extract main content - prefer article/main content areas
	collector.OnHTML("body", func(e *colly.HTMLElement) {
		if done {
			return
		}
		// Try to find main content area
		mainContent := e.DOM.Find("article, main, [role='main'], .content, #content, .post, .entry").First()
		if mainContent.Length() == 0 {
			mainContent = e.DOM
		}

		var err error
		htmlContent, err = mainContent.Html()
		if err != nil {
			extractErr = err
		}
		done = true
	})

	// Handle errors
	collector.OnError(func(r *colly.Response, e error) {
		if extractErr == nil {
			extractErr = fmt.Errorf("HTTP %d: %w", r.StatusCode, e)
		}
	})

	// Visit the URL
	if err := collector.Visit(parsedURL.String()); err != nil {
		return nil, fmt.Errorf("failed to visit URL: %w", err)
	}

	if extractErr != nil {
		return nil, extractErr
	}

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Convert HTML to Markdown
	markdown, err := md.ConvertString(htmlContent)
	if err != nil {
		// Fallback to plain text if conversion fails
		markdown = htmlContent
	}

	// Clean up the markdown
	markdown = cleanMarkdown(markdown)

	// Build metadata
	metadata := map[string]interface{}{
		"generator":   "local-colly",
		"source":      "html-to-markdown",
		"url":         pageURL,
		"contentType": opts.Format,
	}

	latency := time.Since(start)

	return &providers.ExtractResult{
		URL:         pageURL,
		Title:       title,
		Content:     markdown, // Return markdown as content (consistent with other providers)
		Markdown:    markdown,
		Metadata:    metadata,
		Latency:     latency,
		CreditsUsed: 0, // Local crawling is free!
	}, nil
}

// Crawl recursively visits URLs starting from the given URL.
// It respects MaxPages and MaxDepth options, using async processing
// with polite rate limiting.
func (c *Client) Crawl(ctx context.Context, startURL string, opts providers.CrawlOptions) (*providers.CrawlResult, error) {
	start := time.Now()

	// Validate URL
	parsedURL, err := url.Parse(startURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	// Ensure URL has a scheme
	if parsedURL.Scheme == "" {
		parsedURL.Scheme = "https"
		startURL = parsedURL.String()
	}

	pages := make([]providers.CrawledPage, 0, opts.MaxPages)
	visited := make(map[string]bool)
	var mu sync.Mutex
	var crawlErr error

	// Helper to get clean URL (without fragment) for deduplication
	getCleanURL := func(u *url.URL) string {
		clean := *u
		clean.Fragment = ""
		clean.RawFragment = ""
		return clean.String()
	}

	// Mark start URL as visited to prevent re-processing
	visited[getCleanURL(parsedURL)] = true

	// Create collector with async mode for better performance
	collector := colly.NewCollector(
		colly.MaxDepth(opts.MaxDepth),
		colly.UserAgent("Search-API-Bench/1.0 (Local Crawler)"),
		colly.Async(true),
	)

	// Polite rate limiting: 2 concurrent, 500ms delay
	if err := collector.Limit(&colly.LimitRule{
		DomainGlob:  "*",
		Parallelism: 2,
		RandomDelay: 500 * time.Millisecond,
	}); err != nil {
		return nil, fmt.Errorf("failed to set rate limit: %w", err)
	}

	// Context cancellation handling
	collector.OnRequest(func(r *colly.Request) {
		select {
		case <-ctx.Done():
			r.Abort()
			mu.Lock()
			if crawlErr == nil {
				crawlErr = ctx.Err()
			}
			mu.Unlock()
		default:
		}

		// Check if we've reached max pages
		mu.Lock()
		if len(pages) >= opts.MaxPages {
			r.Abort()
		}
		mu.Unlock()
	})

	// Handle errors gracefully - don't fail entire crawl on single page error
	collector.OnError(func(r *colly.Response, _ error) {
		// Log error but continue crawling other pages
		if r.StatusCode == http.StatusNotFound {
			// 404s are expected, don't treat as fatal
			return
		}
		// For other errors, we continue but could log them
	})

	// Extract page content
	collector.OnHTML("html", func(e *colly.HTMLElement) {
		mu.Lock()
		defer mu.Unlock()

		// Check context and page limit
		select {
		case <-ctx.Done():
			return
		default:
		}

		if len(pages) >= opts.MaxPages {
			return
		}

		currentURL := e.Request.URL
		cleanURL := getCleanURL(currentURL)

		if visited[cleanURL] {
			return
		}
		visited[cleanURL] = true

		// Skip non-HTML content
		contentType := e.Response.Headers.Get("Content-Type")
		if contentType != "" && !strings.Contains(contentType, "text/html") {
			return
		}

		// Extract content
		htmlStr, err := e.DOM.Html()
		if err != nil {
			return
		}

		title := e.ChildText("title")
		if title == "" {
			title = e.ChildText("h1")
		}

		// Try to find main content area for better extraction
		mainContent := e.DOM.Find("article, main, [role='main'], .content, #content").First()
		if mainContent.Length() > 0 {
			htmlStr, _ = mainContent.Html()
		}

		// Convert to markdown
		markdown, err := md.ConvertString(htmlStr)
		if err != nil {
			markdown = htmlStr
		}
		markdown = cleanMarkdown(markdown)

		pages = append(pages, providers.CrawledPage{
			URL:      cleanURL,
			Title:    strings.TrimSpace(title),
			Content:  markdown, // Return markdown instead of HTML
			Markdown: markdown,
		})
	})

	// Follow links
	collector.OnHTML("a[href]", func(e *colly.HTMLElement) {
		mu.Lock()
		if len(pages) >= opts.MaxPages {
			mu.Unlock()
			return
		}
		mu.Unlock()

		link := e.Attr("href")
		if link == "" || strings.HasPrefix(link, "#") || strings.HasPrefix(link, "javascript:") {
			return
		}

		// Only follow same-domain links for crawling
		absoluteURL := e.Request.AbsoluteURL(link)
		if absoluteURL == "" {
			return
		}

		linkURL, err := url.Parse(absoluteURL)
		if err != nil {
			return
		}

		// Stay on same domain
		if linkURL.Host != parsedURL.Host {
			return
		}

		// Skip common non-content URLs
		lowerPath := strings.ToLower(linkURL.Path)
		skipExtensions := []string{".pdf", ".jpg", ".jpeg", ".png", ".gif", ".css", ".js", ".zip", ".tar", ".gz"}
		for _, ext := range skipExtensions {
			if strings.HasSuffix(lowerPath, ext) {
				return
			}
		}

		// Visit the link - ignore errors as they may be due to limits or duplicates
		_ = e.Request.Visit(absoluteURL)
	})

	// Start crawling
	if err := collector.Visit(parsedURL.String()); err != nil {
		return nil, fmt.Errorf("failed to start crawl: %w", err)
	}

	// Wait for completion with context awareness
	done := make(chan struct{})
	go func() {
		collector.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Completed normally
	case <-ctx.Done():
		// Context cancelled
		return nil, ctx.Err()
	}

	if crawlErr != nil {
		return nil, crawlErr
	}

	latency := time.Since(start)

	return &providers.CrawlResult{
		URL:         startURL,
		Pages:       pages,
		TotalPages:  len(pages),
		Latency:     latency,
		CreditsUsed: 0, // Local crawling is free!
	}, nil
}

// cleanMarkdown removes excessive whitespace and normalizes the markdown output
func cleanMarkdown(markdown string) string {
	// Replace multiple newlines with maximum two
	for strings.Contains(markdown, "\n\n\n") {
		markdown = strings.ReplaceAll(markdown, "\n\n\n", "\n\n")
	}

	// Trim whitespace
	markdown = strings.TrimSpace(markdown)

	return markdown
}
