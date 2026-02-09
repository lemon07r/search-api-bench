// Package local provides tests for the local crawler provider.
package local

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/lamim/search-api-bench/internal/providers"
	"github.com/lamim/search-api-bench/internal/providers/testutil"
)

// setupTestServer creates a mock HTTP server for testing
func setupTestServer(tb testing.TB) *testutil.Server {
	mux := http.NewServeMux()

	// Home page
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html>
<html>
<head><title>Test Home Page</title></head>
<body>
<main>
<h1>Welcome to Test Site</h1>
<p>This is the main content of the home page.</p>
<pre><code>func main() {
    fmt.Println("Hello, World!")
}</code></pre>
</main>
<a href="/page1">Page 1</a>
<a href="/page2">Page 2</a>
</body>
</html>`)
	})

	// Page 1
	mux.HandleFunc("/page1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html>
<html>
<head><title>Page 1 - Test Site</title></head>
<body>
<article>
<h1>Article One</h1>
<p>This is the content of page 1 with some <strong>bold text</strong>.</p>
<ul>
<li>Item 1</li>
<li>Item 2</li>
</ul>
</article>
<a href="/">Home</a>
<a href="/page2">Page 2</a>
</body>
</html>`)
	})

	// Page 2
	mux.HandleFunc("/page2", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html>
<html>
<head><title>Page 2 - Test Site</title></head>
<body>
<main>
<h1>Second Page</h1>
<p>This is page 2 content.</p>
<blockquote>
<p>A famous quote here.</p>
</blockquote>
</main>
<a href="/">Home</a>
<a href="/page1">Page 1</a>
</body>
</html>`)
	})

	// 404 page
	mux.HandleFunc("/notfound", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, "Not Found")
	})

	return testutil.NewIPv4Server(tb, mux)
}

func TestNewClient(t *testing.T) {
	client, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	if client == nil {
		t.Fatal("NewClient() returned nil")
	}

	if client.Name() != "local" {
		t.Errorf("Name() = %v, want %v", client.Name(), "local")
	}
}

func TestClientSearch(t *testing.T) {
	client, _ := NewClient()
	ctx := context.Background()

	_, err := client.Search(ctx, "test query", providers.DefaultSearchOptions())
	if err == nil {
		t.Error("Search() expected error for local provider, got nil")
	}

	if !strings.Contains(err.Error(), "not supported") {
		t.Errorf("Search() error should indicate unsupported operation, got: %v", err)
	}
}

func TestClientExtract(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	client, _ := NewClient()
	ctx := context.Background()

	opts := providers.DefaultExtractOptions()

	result, err := client.Extract(ctx, server.URL+"/", opts)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	// Check basic fields
	if result.URL != server.URL+"/" {
		t.Errorf("Extract() URL = %v, want %v", result.URL, server.URL+"/")
	}

	if result.Title != "Test Home Page" {
		t.Errorf("Extract() Title = %v, want %v", result.Title, "Test Home Page")
	}

	if result.CreditsUsed != 0 {
		t.Errorf("Extract() CreditsUsed = %v, want %v", result.CreditsUsed, 0)
	}

	// Check content was extracted
	if result.Content == "" {
		t.Error("Extract() Content is empty")
	}

	// Check markdown was generated
	if result.Markdown == "" {
		t.Error("Extract() Markdown is empty")
	}

	// Check content contains expected text
	if !strings.Contains(result.Markdown, "Welcome to Test Site") {
		t.Error("Extract() Markdown should contain 'Welcome to Test Site'")
	}

	// Check metadata
	if result.Metadata == nil {
		t.Error("Extract() Metadata is nil")
	}

	if result.Metadata["generator"] != "local-colly" {
		t.Errorf("Extract() Metadata generator = %v, want %v", result.Metadata["generator"], "local-colly")
	}

	// Check latency was recorded
	if result.Latency <= 0 {
		t.Error("Extract() Latency should be > 0")
	}
}

func TestClientExtractContextCancellation(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	client, _ := NewClient()

	// Create a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	opts := providers.DefaultExtractOptions()

	_, err := client.Extract(ctx, server.URL+"/", opts)
	if err == nil {
		t.Error("Extract() with cancelled context should return error")
	}
}

func TestClientExtractInvalidURL(t *testing.T) {
	client, _ := NewClient()
	ctx := context.Background()
	opts := providers.DefaultExtractOptions()

	_, err := client.Extract(ctx, "not-a-valid-url://[bad]", opts)
	if err == nil {
		t.Error("Extract() with invalid URL should return error")
	}
}

func TestClientCrawl(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	client, _ := NewClient()
	ctx := context.Background()

	opts := providers.CrawlOptions{
		MaxPages: 3,
		MaxDepth: 2,
	}

	result, err := client.Crawl(ctx, server.URL+"/", opts)
	if err != nil {
		t.Fatalf("Crawl() error = %v", err)
	}

	// Should find at least the home page
	if result.TotalPages == 0 {
		t.Error("Crawl() should find at least one page")
	}

	// Check result structure
	if result.URL != server.URL+"/" {
		t.Errorf("Crawl() URL = %v, want %v", result.URL, server.URL+"/")
	}

	if result.CreditsUsed != 0 {
		t.Errorf("Crawl() CreditsUsed = %v, want %v", result.CreditsUsed, 0)
	}

	// Check pages have content
	for i, page := range result.Pages {
		if page.URL == "" {
			t.Errorf("Crawl() Page %d has empty URL", i)
		}
		if page.Markdown == "" {
			t.Errorf("Crawl() Page %d has empty Markdown", i)
		}
	}

	// Check latency was recorded
	if result.Latency <= 0 {
		t.Error("Crawl() Latency should be > 0")
	}
}

func TestClientCrawlMaxPages(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	client, _ := NewClient()
	ctx := context.Background()

	// Request only 1 page
	opts := providers.CrawlOptions{
		MaxPages: 1,
		MaxDepth: 2,
	}

	result, err := client.Crawl(ctx, server.URL+"/", opts)
	if err != nil {
		t.Fatalf("Crawl() error = %v", err)
	}

	// Should respect max pages
	if result.TotalPages > opts.MaxPages {
		t.Errorf("Crawl() found %d pages, expected at most %d", result.TotalPages, opts.MaxPages)
	}

	if result.TotalPages == 1 && result.Pages[0].URL != server.URL+"/" {
		t.Errorf("expected single-page crawl to return start URL %q, got %q", server.URL+"/", result.Pages[0].URL)
	}
}

func TestClientCrawlZeroDepthReturnsStartPageOnly(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	client, _ := NewClient()
	ctx := context.Background()

	opts := providers.CrawlOptions{
		MaxPages: 5,
		MaxDepth: 0,
	}

	result, err := client.Crawl(ctx, server.URL+"/", opts)
	if err != nil {
		t.Fatalf("Crawl() error = %v", err)
	}

	if result.TotalPages != 1 {
		t.Fatalf("expected exactly 1 page for max_depth=0, got %d", result.TotalPages)
	}
	if len(result.Pages) != 1 {
		t.Fatalf("expected exactly 1 page in result set, got %d", len(result.Pages))
	}
	if result.Pages[0].URL != server.URL+"/" {
		t.Fatalf("expected start URL %q, got %q", server.URL+"/", result.Pages[0].URL)
	}
}

func TestClientCrawlContextTimeout(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	client, _ := NewClient()

	// Create a context with very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	opts := providers.CrawlOptions{
		MaxPages: 10,
		MaxDepth: 3,
	}

	// This might or might not error depending on timing,
	// but it should not panic or hang
	_, _ = client.Crawl(ctx, server.URL+"/", opts)
}

func TestClientCrawlInvalidURL(t *testing.T) {
	client, _ := NewClient()
	ctx := context.Background()

	opts := providers.DefaultCrawlOptions()

	_, err := client.Crawl(ctx, "not-a-valid-url://[bad]", opts)
	if err == nil {
		t.Error("Crawl() with invalid URL should return error")
	}
}

func TestCleanMarkdown(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "Hello\n\n\n\nWorld",
			expected: "Hello\n\nWorld",
		},
		{
			input:    "  \n\n  Content  \n\n  ",
			expected: "Content",
		},
		{
			input:    "Normal content",
			expected: "Normal content",
		},
	}

	for _, tt := range tests {
		result := cleanMarkdown(tt.input)
		if result != tt.expected {
			t.Errorf("cleanMarkdown(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func BenchmarkClientExtract(b *testing.B) {
	server := setupTestServer(b)
	defer server.Close()

	client, _ := NewClient()
	ctx := context.Background()
	opts := providers.DefaultExtractOptions()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := client.Extract(ctx, server.URL+"/", opts)
		if err != nil {
			b.Fatalf("Extract() error = %v", err)
		}
	}
}
