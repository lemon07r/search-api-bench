package tavily

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
	os.Unsetenv("TAVILY_API_KEY")
	_, err := NewClient()
	if err == nil {
		t.Fatal("expected error for missing API key, got nil")
	}
}

func TestNewClient_WithAPIKey(t *testing.T) {
	os.Setenv("TAVILY_API_KEY", "test-key")
	defer os.Unsetenv("TAVILY_API_KEY")

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

		// Verify request body
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		if req["query"] != "test query" {
			t.Errorf("expected query 'test query', got %v", req["query"])
		}

		response := searchResponse{
			Answer: "Test answer",
			Query:  "test query",
			Results: []struct {
				Title         string  `json:"title"`
				URL           string  `json:"url"`
				Content       string  `json:"content"`
				Score         float64 `json:"score"`
				PublishedDate string  `json:"published_date"`
			}{
				{
					Title:         "Result 1",
					URL:           "https://example.com/1",
					Content:       "Content 1",
					Score:         0.95,
					PublishedDate: "2024-01-15",
				},
				{
					Title:   "Result 2",
					URL:     "https://example.com/2",
					Content: "Content 2",
					Score:   0.85,
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

	if len(result.Results) != 2 {
		t.Errorf("expected 2 results, got %d", len(result.Results))
	}
	if result.Results[0].Title != "Result 1" {
		t.Errorf("expected title 'Result 1', got %s", result.Results[0].Title)
	}
	if result.Results[0].Score != 0.95 {
		t.Errorf("expected score 0.95, got %f", result.Results[0].Score)
	}
	if result.Results[0].PublishedAt == nil {
		t.Error("expected PublishedAt to be set")
	}
}

func TestSearch_BasicCredits(t *testing.T) {
	server := testutil.NewIPv4Server(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := searchResponse{Results: []struct {
			Title         string  `json:"title"`
			URL           string  `json:"url"`
			Content       string  `json:"content"`
			Score         float64 `json:"score"`
			PublishedDate string  `json:"published_date"`
		}{}}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := &Client{
		apiKey:     "test-key",
		baseURL:    server.URL,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}

	opts := providers.DefaultSearchOptions()
	opts.SearchDepth = "basic"

	result, err := client.Search(context.Background(), "test", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.CreditsUsed != 1 {
		t.Errorf("expected 1 credit for basic search, got %d", result.CreditsUsed)
	}
}

func TestSearch_AdvancedCredits(t *testing.T) {
	server := testutil.NewIPv4Server(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := searchResponse{Results: []struct {
			Title         string  `json:"title"`
			URL           string  `json:"url"`
			Content       string  `json:"content"`
			Score         float64 `json:"score"`
			PublishedDate string  `json:"published_date"`
		}{}}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := &Client{
		apiKey:     "test-key",
		baseURL:    server.URL,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}

	opts := providers.DefaultSearchOptions()
	opts.SearchDepth = "advanced"

	result, err := client.Search(context.Background(), "test", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.CreditsUsed != 2 {
		t.Errorf("expected 2 credits for advanced search, got %d", result.CreditsUsed)
	}
}

func TestSearch_WithTimeRange(t *testing.T) {
	var capturedReq map[string]interface{}
	server := testutil.NewIPv4Server(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedReq)
		response := searchResponse{Results: []struct {
			Title         string  `json:"title"`
			URL           string  `json:"url"`
			Content       string  `json:"content"`
			Score         float64 `json:"score"`
			PublishedDate string  `json:"published_date"`
		}{}}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := &Client{
		apiKey:     "test-key",
		baseURL:    server.URL,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}

	opts := providers.DefaultSearchOptions()
	opts.TimeRange = "month"

	_, err := client.Search(context.Background(), "test", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedReq["time_range"] != "month" {
		t.Errorf("expected time_range 'month', got %v", capturedReq["time_range"])
	}
}

func TestSearch_WithImages(t *testing.T) {
	var capturedReq map[string]interface{}
	server := testutil.NewIPv4Server(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedReq)
		response := searchResponse{Results: []struct {
			Title         string  `json:"title"`
			URL           string  `json:"url"`
			Content       string  `json:"content"`
			Score         float64 `json:"score"`
			PublishedDate string  `json:"published_date"`
		}{}}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := &Client{
		apiKey:     "test-key",
		baseURL:    server.URL,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}

	opts := providers.DefaultSearchOptions()
	opts.IncludeImages = true

	_, err := client.Search(context.Background(), "test", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedReq["include_images"] != true {
		t.Errorf("expected include_images true, got %v", capturedReq["include_images"])
	}
}

func TestSearch_DateParsing(t *testing.T) {
	server := testutil.NewIPv4Server(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := searchResponse{
			Results: []struct {
				Title         string  `json:"title"`
				URL           string  `json:"url"`
				Content       string  `json:"content"`
				Score         float64 `json:"score"`
				PublishedDate string  `json:"published_date"`
			}{
				{
					Title:         "Article",
					URL:           "https://example.com",
					Content:       "Content",
					PublishedDate: "2024-03-15",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := &Client{
		apiKey:     "test-key",
		baseURL:    server.URL,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}

	result, err := client.Search(context.Background(), "test", providers.DefaultSearchOptions())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Results[0].PublishedAt == nil {
		t.Fatal("expected PublishedAt to be set")
	}

	expectedDate := time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)
	if !result.Results[0].PublishedAt.Equal(expectedDate) {
		t.Errorf("expected date %v, got %v", expectedDate, result.Results[0].PublishedAt)
	}
}

func TestSearch_InvalidDate(t *testing.T) {
	server := testutil.NewIPv4Server(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := searchResponse{
			Results: []struct {
				Title         string  `json:"title"`
				URL           string  `json:"url"`
				Content       string  `json:"content"`
				Score         float64 `json:"score"`
				PublishedDate string  `json:"published_date"`
			}{
				{
					Title:         "Article",
					URL:           "https://example.com",
					Content:       "Content",
					PublishedDate: "invalid-date",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := &Client{
		apiKey:     "test-key",
		baseURL:    server.URL,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}

	result, err := client.Search(context.Background(), "test", providers.DefaultSearchOptions())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Invalid date should be silently skipped (not an error)
	if result.Results[0].PublishedAt != nil {
		t.Error("expected PublishedAt to be nil for invalid date")
	}
}

func TestSearch_HTTPError(t *testing.T) {
	server := testutil.NewIPv4Server(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error": "invalid api key"}`))
	}))
	defer server.Close()

	client := &Client{
		apiKey:     "test-key",
		baseURL:    server.URL,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}

	_, err := client.Search(context.Background(), "test", providers.DefaultSearchOptions())
	if err == nil {
		t.Fatal("expected error for 401 response, got nil")
	}
}

func TestSearch_InvalidJSON(t *testing.T) {
	server := testutil.NewIPv4Server(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{invalid`))
	}))
	defer server.Close()

	client := &Client{
		apiKey:     "test-key",
		baseURL:    server.URL,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}

	_, err := client.Search(context.Background(), "test", providers.DefaultSearchOptions())
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestExtract_Success(t *testing.T) {
	server := testutil.NewIPv4Server(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/extract" {
			t.Errorf("expected path /extract, got %s", r.URL.Path)
		}

		response := extractResponse{
			Results: []struct {
				URL        string   `json:"url"`
				Title      string   `json:"title"`
				RawContent string   `json:"raw_content"`
				Images     []string `json:"images"`
			}{
				{
					URL:        "https://example.com",
					Title:      "Extracted Title",
					RawContent: "Extracted content here",
					Images:     []string{"https://example.com/img1.jpg"},
				},
			},
			FailedResults: []struct {
				URL   string `json:"url"`
				Error string `json:"error"`
			}{},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := &Client{
		apiKey:     "test-key",
		baseURL:    server.URL,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}

	result, err := client.Extract(context.Background(), "https://example.com", providers.DefaultExtractOptions())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Title != "Extracted Title" {
		t.Errorf("expected title 'Extracted Title', got %s", result.Title)
	}
	if result.Content != "Extracted content here" {
		t.Errorf("expected content 'Extracted content here', got %s", result.Content)
	}
}

func TestExtract_NoResults(t *testing.T) {
	server := testutil.NewIPv4Server(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := extractResponse{
			Results: []struct {
				URL        string   `json:"url"`
				Title      string   `json:"title"`
				RawContent string   `json:"raw_content"`
				Images     []string `json:"images"`
			}{},
			FailedResults: []struct {
				URL   string `json:"url"`
				Error string `json:"error"`
			}{},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := &Client{
		apiKey:     "test-key",
		baseURL:    server.URL,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}

	_, err := client.Extract(context.Background(), "https://example.com", providers.DefaultExtractOptions())
	if err == nil {
		t.Fatal("expected error for empty results, got nil")
	}
}

func TestExtract_PartialFail(t *testing.T) {
	server := testutil.NewIPv4Server(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := extractResponse{
			Results: []struct {
				URL        string   `json:"url"`
				Title      string   `json:"title"`
				RawContent string   `json:"raw_content"`
				Images     []string `json:"images"`
			}{
				{
					URL:        "https://example.com/success",
					Title:      "Success",
					RawContent: "Content",
				},
			},
			FailedResults: []struct {
				URL   string `json:"url"`
				Error string `json:"error"`
			}{
				{
					URL:   "https://example.com/failed",
					Error: "timeout",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := &Client{
		apiKey:     "test-key",
		baseURL:    server.URL,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}

	// First URL succeeds - should return result for that URL
	result, err := client.Extract(context.Background(), "https://example.com/success", providers.DefaultExtractOptions())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Title != "Success" {
		t.Errorf("expected title 'Success', got %s", result.Title)
	}
}

func TestCrawl_MapSuccess(t *testing.T) {
	server := testutil.NewIPv4Server(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/map":
			response := mapResponse{
				Links:   []string{"https://example.com/page1", "https://example.com/page2"},
				Results: []string{"https://example.com/page1", "https://example.com/page2"},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		case "/extract":
			response := extractResponse{
				Results: []struct {
					URL        string   `json:"url"`
					Title      string   `json:"title"`
					RawContent string   `json:"raw_content"`
					Images     []string `json:"images"`
				}{
					{
						URL:        "https://example.com/page1",
						Title:      "Page 1",
						RawContent: "Content 1",
					},
				},
				FailedResults: []struct {
					URL   string `json:"url"`
					Error string `json:"error"`
				}{},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		}
	}))
	defer server.Close()

	client := &Client{
		apiKey:     "test-key",
		baseURL:    server.URL,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}

	opts := providers.DefaultCrawlOptions()
	opts.MaxPages = 1

	result, err := client.Crawl(context.Background(), "https://example.com", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Pages) != 1 {
		t.Errorf("expected 1 page (respecting max_pages), got %d", len(result.Pages))
	}
}

func TestCrawl_RespectsMaxPages(t *testing.T) {
	server := testutil.NewIPv4Server(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/map":
			response := mapResponse{
				Results: []string{
					"https://example.com/1",
					"https://example.com/2",
					"https://example.com/3",
					"https://example.com/4",
					"https://example.com/5",
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		case "/extract":
			response := extractResponse{
				Results: []struct {
					URL        string   `json:"url"`
					Title      string   `json:"title"`
					RawContent string   `json:"raw_content"`
					Images     []string `json:"images"`
				}{
					{URL: "extracted", Title: "Title", RawContent: "Content"},
				},
				FailedResults: []struct {
					URL   string `json:"url"`
					Error string `json:"error"`
				}{},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		}
	}))
	defer server.Close()

	client := &Client{
		apiKey:     "test-key",
		baseURL:    server.URL,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}

	opts := providers.DefaultCrawlOptions()
	opts.MaxPages = 2

	result, err := client.Crawl(context.Background(), "https://example.com", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should stop at max_pages even if map returns more
	if len(result.Pages) != 2 {
		t.Errorf("expected 2 pages (max_pages), got %d", len(result.Pages))
	}
}

func TestCrawl_EmptyMap(t *testing.T) {
	server := testutil.NewIPv4Server(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/map" {
			response := mapResponse{
				Results: []string{},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		} else if r.URL.Path == "/extract" {
			// When map is empty, we fall back to extracting the start URL
			response := extractResponse{
				Results: []struct {
					URL        string   `json:"url"`
					Title      string   `json:"title"`
					RawContent string   `json:"raw_content"`
					Images     []string `json:"images"`
				}{
					{
						URL:        "https://example.com",
						Title:      "Example",
						RawContent: "Example content",
						Images:     []string{},
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		}
	}))
	defer server.Close()

	client := &Client{
		apiKey:     "test-key",
		baseURL:    server.URL,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}

	result, err := client.Crawl(context.Background(), "https://example.com", providers.DefaultCrawlOptions())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have 1 page from fallback extract
	if len(result.Pages) != 1 {
		t.Errorf("expected 1 page from fallback extract, got %d", len(result.Pages))
	}
}
