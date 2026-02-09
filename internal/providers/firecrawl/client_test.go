package firecrawl

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/lamim/search-api-bench/internal/providers"
	"github.com/lamim/search-api-bench/internal/providers/testutil"
)

func TestNewClient_MissingAPIKey(t *testing.T) {
	os.Unsetenv("FIRECRAWL_API_KEY")
	_, err := NewClient()
	if err == nil {
		t.Fatal("expected error for missing API key, got nil")
	}
}

func TestNewClient_WithAPIKey(t *testing.T) {
	os.Setenv("FIRECRAWL_API_KEY", "test-key")
	defer os.Unsetenv("FIRECRAWL_API_KEY")

	client, err := NewClient()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("expected client, got nil")
	}
}

func TestSearch_Success(t *testing.T) {
	server := testutil.NewIPv4Server(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search" {
			t.Errorf("expected path /search, got %s", r.URL.Path)
		}
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}

		response := searchResponse{
			Success: true,
			Data: []struct {
				Markdown string `json:"markdown"`
				Metadata struct {
					Title     string `json:"title"`
					SourceURL string `json:"sourceURL"`
				} `json:"metadata"`
			}{
				{
					Markdown: "Test content",
					Metadata: struct {
						Title     string `json:"title"`
						SourceURL string `json:"sourceURL"`
					}{
						Title:     "Test Title",
						SourceURL: "https://example.com",
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := &Client{
		apiKey:  "test-key",
		baseURL: server.URL,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}

	result, err := client.Search(context.Background(), "test query", providers.DefaultSearchOptions())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Results) != 1 {
		t.Errorf("expected 1 result, got %d", len(result.Results))
	}
	if result.Results[0].Title != "Test Title" {
		t.Errorf("expected title 'Test Title', got %s", result.Results[0].Title)
	}
	if result.Results[0].URL != "https://example.com" {
		t.Errorf("expected URL 'https://example.com', got %s", result.Results[0].URL)
	}
	if result.CreditsUsed != 1 {
		t.Errorf("expected 1 credit, got %d", result.CreditsUsed)
	}
}

func TestSearch_HTTPError(t *testing.T) {
	server := testutil.NewIPv4Server(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "internal error"}`))
	}))
	defer server.Close()

	client := &Client{
		apiKey:  "test-key",
		baseURL: server.URL,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}

	_, err := client.Search(context.Background(), "test", providers.DefaultSearchOptions())
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}

func TestSearch_InvalidJSON(t *testing.T) {
	server := testutil.NewIPv4Server(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{invalid json`))
	}))
	defer server.Close()

	client := &Client{
		apiKey:  "test-key",
		baseURL: server.URL,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}

	_, err := client.Search(context.Background(), "test", providers.DefaultSearchOptions())
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestSearch_EmptyResults(t *testing.T) {
	server := testutil.NewIPv4Server(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := searchResponse{
			Success: true,
			Data: []struct {
				Markdown string `json:"markdown"`
				Metadata struct {
					Title     string `json:"title"`
					SourceURL string `json:"sourceURL"`
				} `json:"metadata"`
			}{},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := &Client{
		apiKey:  "test-key",
		baseURL: server.URL,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}

	result, err := client.Search(context.Background(), "test", providers.DefaultSearchOptions())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Results) != 0 {
		t.Errorf("expected 0 results, got %d", len(result.Results))
	}
	if result.TotalResults != 0 {
		t.Errorf("expected 0 total results, got %d", result.TotalResults)
	}
}

func TestExtract_Success(t *testing.T) {
	server := testutil.NewIPv4Server(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/scrape" {
			t.Errorf("expected path /scrape, got %s", r.URL.Path)
		}

		response := scrapeResponse{
			Success: true,
			Data: struct {
				Markdown string `json:"markdown"`
				Metadata struct {
					Title     string                 `json:"title"`
					SourceURL string                 `json:"sourceURL"`
					Raw       map[string]interface{} `json:"-"`
				} `json:"metadata"`
			}{
				Markdown: "# Test Content",
				Metadata: struct {
					Title     string                 `json:"title"`
					SourceURL string                 `json:"sourceURL"`
					Raw       map[string]interface{} `json:"-"`
				}{
					Title:     "Page Title",
					SourceURL: "https://example.com/page",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := &Client{
		apiKey:  "test-key",
		baseURL: server.URL,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}

	result, err := client.Extract(context.Background(), "https://example.com", providers.DefaultExtractOptions())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Title != "Page Title" {
		t.Errorf("expected title 'Page Title', got %s", result.Title)
	}
	if result.Content != "# Test Content" {
		t.Errorf("expected content '# Test Content', got %s", result.Content)
	}
	if result.CreditsUsed != 1 {
		t.Errorf("expected 1 credit, got %d", result.CreditsUsed)
	}
}

func TestExtract_NotFound(t *testing.T) {
	server := testutil.NewIPv4Server(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error": "not found"}`))
	}))
	defer server.Close()

	client := &Client{
		apiKey:  "test-key",
		baseURL: server.URL,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}

	_, err := client.Extract(context.Background(), "https://example.com", providers.DefaultExtractOptions())
	if err == nil {
		t.Fatal("expected error for 404 response, got nil")
	}
}

func TestCrawl_SyncSuccess(t *testing.T) {
	server := testutil.NewIPv4Server(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/crawl" {
			// Return immediate completion (no ID)
			response := crawlResponse{
				Success: true,
				Data: []struct {
					Markdown string `json:"markdown"`
					Metadata struct {
						Title     string `json:"title"`
						SourceURL string `json:"sourceURL"`
					} `json:"metadata"`
				}{
					{
						Markdown: "Page 1",
						Metadata: struct {
							Title     string `json:"title"`
							SourceURL string `json:"sourceURL"`
						}{
							Title:     "Page 1",
							SourceURL: "https://example.com/1",
						},
					},
					{
						Markdown: "Page 2",
						Metadata: struct {
							Title     string `json:"title"`
							SourceURL string `json:"sourceURL"`
						}{
							Title:     "Page 2",
							SourceURL: "https://example.com/2",
						},
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		}
	}))
	defer server.Close()

	client := &Client{
		apiKey:  "test-key",
		baseURL: server.URL,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}

	opts := providers.DefaultCrawlOptions()
	opts.MaxPages = 2

	result, err := client.Crawl(context.Background(), "https://example.com", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Pages) != 2 {
		t.Errorf("expected 2 pages, got %d", len(result.Pages))
	}
	if result.TotalPages != 2 {
		t.Errorf("expected TotalPages 2, got %d", result.TotalPages)
	}
	if result.CreditsUsed != 2 {
		t.Errorf("expected 2 credits (1 per page), got %d", result.CreditsUsed)
	}
}

func TestCrawl_AsyncPolling(t *testing.T) {
	callCount := 0
	server := testutil.NewIPv4Server(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/crawl" {
			// Initial request - return ID
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"success": true, "id": "crawl-123"}`))
		} else if r.URL.Path == "/crawl/crawl-123" {
			callCount++
			if callCount < 3 {
				// Still scraping
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"status": "scraping"}`))
			} else {
				// Completed
				response := crawlStatusResponse{
					Status: "completed",
					URL:    "https://example.com",
					Data: []struct {
						Markdown string `json:"markdown"`
						Metadata struct {
							Title     string `json:"title"`
							SourceURL string `json:"sourceURL"`
						} `json:"metadata"`
					}{
						{
							Markdown: "Content",
							Metadata: struct {
								Title     string `json:"title"`
								SourceURL string `json:"sourceURL"`
							}{
								Title:     "Title",
								SourceURL: "https://example.com",
							},
						},
					},
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(response)
			}
		}
	}))
	defer server.Close()

	client := &Client{
		apiKey:  "test-key",
		baseURL: server.URL,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}

	result, err := client.Crawl(context.Background(), "https://example.com", providers.DefaultCrawlOptions())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.TotalPages != 1 {
		t.Errorf("expected 1 page, got %d", result.TotalPages)
	}
	if callCount < 2 {
		t.Errorf("expected multiple polling calls, got %d", callCount)
	}
}

func TestCrawl_AsyncFailed(t *testing.T) {
	server := testutil.NewIPv4Server(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/crawl" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"success": true, "id": "crawl-456"}`))
		} else if r.URL.Path == "/crawl/crawl-456" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"status": "failed", "error": "crawl failed"}`))
		}
	}))
	defer server.Close()

	client := &Client{
		apiKey:  "test-key",
		baseURL: server.URL,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}

	_, err := client.Crawl(context.Background(), "https://example.com", providers.DefaultCrawlOptions())
	if err == nil {
		t.Fatal("expected error for failed crawl, got nil")
	}
	if err.Error() != "crawl failed: crawl failed" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestCrawl_Timeout(t *testing.T) {
	server := testutil.NewIPv4Server(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/crawl" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"success": true, "id": "crawl-789"}`))
		} else if r.URL.Path == "/crawl/crawl-789" {
			// Never completes - will timeout
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"status": "scraping"}`))
		}
	}))
	defer server.Close()

	client := &Client{
		apiKey:  "test-key",
		baseURL: server.URL,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := client.Crawl(ctx, "https://example.com", providers.DefaultCrawlOptions())
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}
