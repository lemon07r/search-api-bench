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
	"github.com/lamim/sanity-web-eval/internal/providers"
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

// Capabilities returns local provider operation support levels.
func (c *Client) Capabilities() providers.CapabilitySet {
	return providers.CapabilitySet{
		Search:  providers.SupportUnsupported,
		Extract: providers.SupportNative,
		Crawl:   providers.SupportNative,
	}
}

// SupportsOperation returns whether the local provider supports the given operation type
// Local provider only supports extract and crawl, not search (no web index)
func (c *Client) SupportsOperation(opType string) bool {
	return c.Capabilities().SupportsOperation(opType)
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
		URL:          pageURL,
		Title:        title,
		Content:      markdown, // Return markdown as content (consistent with other providers)
		Markdown:     markdown,
		Metadata:     metadata,
		Latency:      latency,
		CreditsUsed:  0, // Local crawling is free!
		RequestCount: 1,
	}, nil
}

// Crawl recursively visits URLs starting from the given URL.
// It respects MaxPages and MaxDepth options, using async processing
// with polite rate limiting.
func (c *Client) Crawl(ctx context.Context, startURL string, opts providers.CrawlOptions) (*providers.CrawlResult, error) {
	start := time.Now()

	startURL, parsedURL, err := normalizeStartURL(startURL)
	if err != nil {
		return nil, err
	}

	state := newCrawlState(opts.MaxPages)
	collector, err := newCrawlCollector(parsedURL, opts.MaxDepth)
	if err != nil {
		return nil, err
	}

	collector.OnRequest(func(r *colly.Request) {
		if err := ctx.Err(); err != nil {
			r.Abort()
			state.setError(err)
			return
		}
		if state.shouldAbortRequest(cleanURL(r.URL)) {
			r.Abort()
		}
	})

	collector.OnError(func(r *colly.Response, _ error) {
		if r.StatusCode == http.StatusNotFound {
			return
		}
	})

	collector.OnHTML("html", func(e *colly.HTMLElement) {
		if ctx.Err() != nil {
			return
		}
		page, ok := extractCrawledPage(e)
		if !ok {
			return
		}
		state.addPage(page)
	})

	collector.OnHTML("a[href]", func(e *colly.HTMLElement) {
		if opts.MaxPages == 1 || opts.MaxDepth == 0 {
			return
		}
		if !state.hasCapacity() {
			return
		}

		link := e.Attr("href")
		if skipCrawlLink(link) {
			return
		}

		absoluteURL := e.Request.AbsoluteURL(link)
		if absoluteURL == "" {
			return
		}

		linkURL, err := url.Parse(absoluteURL)
		if err != nil {
			return
		}
		if linkURL.Host != parsedURL.Host {
			return
		}
		if skipCrawlPath(linkURL.Path) {
			return
		}

		// Ignore visit errors (limits, duplicates, etc).
		_ = e.Request.Visit(absoluteURL)
	})

	if err := collector.Visit(parsedURL.String()); err != nil {
		return nil, fmt.Errorf("failed to start crawl: %w", err)
	}
	if err := waitForCrawl(ctx, collector); err != nil {
		return nil, err
	}

	pages, crawlErr := state.result()
	if crawlErr != nil {
		return nil, crawlErr
	}

	latency := time.Since(start)

	return &providers.CrawlResult{
		URL:          startURL,
		Pages:        pages,
		TotalPages:   len(pages),
		Latency:      latency,
		CreditsUsed:  0, // Local crawling is free!
		RequestCount: len(pages),
	}, nil
}

type crawlState struct {
	mu       sync.Mutex
	pages    []providers.CrawledPage
	visited  map[string]bool
	maxPages int
	crawlErr error
}

func newCrawlState(maxPages int) *crawlState {
	if maxPages < 0 {
		maxPages = 0
	}
	return &crawlState{
		pages:    make([]providers.CrawledPage, 0, maxPages),
		visited:  make(map[string]bool),
		maxPages: maxPages,
	}
}

func (s *crawlState) setError(err error) {
	if err == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.crawlErr == nil {
		s.crawlErr = err
	}
}

func (s *crawlState) shouldAbortRequest(requestURL string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.pages) >= s.maxPages {
		return true
	}
	if s.visited[requestURL] {
		return true
	}

	s.visited[requestURL] = true
	return false
}

func (s *crawlState) hasCapacity() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.pages) < s.maxPages
}

func (s *crawlState) addPage(page providers.CrawledPage) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.pages) >= s.maxPages {
		return
	}
	s.pages = append(s.pages, page)
}

func (s *crawlState) result() ([]providers.CrawledPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	pages := make([]providers.CrawledPage, len(s.pages))
	copy(pages, s.pages)
	return pages, s.crawlErr
}

func normalizeStartURL(startURL string) (string, *url.URL, error) {
	parsedURL, err := url.Parse(startURL)
	if err != nil {
		return "", nil, fmt.Errorf("invalid URL: %w", err)
	}
	if parsedURL.Scheme == "" {
		parsedURL.Scheme = "https"
		startURL = parsedURL.String()
	}
	return startURL, parsedURL, nil
}

func newCrawlCollector(parsedURL *url.URL, maxDepth int) (*colly.Collector, error) {
	effectiveMaxDepth := maxDepth
	if effectiveMaxDepth <= 0 {
		// Explicit depth 0 means crawl only the starting page.
		effectiveMaxDepth = 1
	}

	collector := colly.NewCollector(
		colly.MaxDepth(effectiveMaxDepth),
		colly.UserAgent("Search-API-Bench/1.0 (Local Crawler)"),
		colly.Async(true),
	)

	if err := collector.Limit(&colly.LimitRule{
		DomainGlob:  parsedURL.Host,
		Parallelism: 2,
		RandomDelay: 500 * time.Millisecond,
	}); err != nil {
		return nil, fmt.Errorf("failed to set rate limit: %w", err)
	}

	return collector, nil
}

func cleanURL(u *url.URL) string {
	clean := *u
	clean.Fragment = ""
	clean.RawFragment = ""
	return clean.String()
}

func extractCrawledPage(e *colly.HTMLElement) (providers.CrawledPage, bool) {
	contentType := e.Response.Headers.Get("Content-Type")
	if contentType != "" && !strings.Contains(contentType, "text/html") {
		return providers.CrawledPage{}, false
	}

	htmlStr, err := e.DOM.Html()
	if err != nil {
		return providers.CrawledPage{}, false
	}

	title := e.ChildText("title")
	if title == "" {
		title = e.ChildText("h1")
	}

	mainContent := e.DOM.Find("article, main, [role='main'], .content, #content").First()
	if mainContent.Length() > 0 {
		if mainHTML, err := mainContent.Html(); err == nil {
			htmlStr = mainHTML
		}
	}

	markdown, err := md.ConvertString(htmlStr)
	if err != nil {
		markdown = htmlStr
	}
	markdown = cleanMarkdown(markdown)

	return providers.CrawledPage{
		URL:      cleanURL(e.Request.URL),
		Title:    strings.TrimSpace(title),
		Content:  markdown,
		Markdown: markdown,
	}, true
}

func skipCrawlLink(link string) bool {
	return link == "" || strings.HasPrefix(link, "#") || strings.HasPrefix(link, "javascript:")
}

func skipCrawlPath(path string) bool {
	lowerPath := strings.ToLower(path)
	skipExtensions := []string{".pdf", ".jpg", ".jpeg", ".png", ".gif", ".css", ".js", ".zip", ".tar", ".gz"}
	for _, ext := range skipExtensions {
		if strings.HasSuffix(lowerPath, ext) {
			return true
		}
	}
	return false
}

func waitForCrawl(ctx context.Context, collector *colly.Collector) error {
	done := make(chan struct{})
	go func() {
		collector.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
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
