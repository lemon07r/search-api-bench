package exa

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/lamim/search-api-bench/internal/providers"
	"github.com/lamim/search-api-bench/internal/providers/testutil"
)

func TestSearch_UsesContextFallbackWhenResultTextMissing(t *testing.T) {
	server := testutil.NewIPv4Server(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search" {
			t.Fatalf("expected /search path, got %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST method, got %s", r.Method)
		}

		resp := searchResponse{
			RequestID: "req-1",
			Context: "Title: Paris Population 2026\n" +
				"URL: https://worldpopulationreview.com/cities/france/paris\n" +
				"Summary: The estimated population is 11,418,300.\n\n" +
				"Title: Demographics of Paris\n" +
				"URL: https://en.wikipedia.org/wiki/Demographics_of_Paris\n" +
				"Summary: The city had 2.1 million inhabitants.",
			Results: []exaSearchHit{
				{
					Title:         "Paris Population 2026",
					URL:           "https://worldpopulationreview.com/cities/france/paris",
					PublishedDate: "2025-01-01T00:00:00.000Z",
				},
				{
					Title: "Demographics of Paris",
					URL:   "https://en.wikipedia.org/wiki/Demographics_of_Paris",
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := &Client{
		apiKey:   "test-key",
		baseURL:  server.URL,
		retryCfg: providers.DefaultRetryConfig(),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}

	result, err := client.Search(context.Background(), "capital of France population", providers.DefaultSearchOptions())
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(result.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result.Results))
	}
	if result.Results[0].Content == "" {
		t.Fatal("expected first result content from context fallback")
	}
	if result.Results[1].Content == "" {
		t.Fatal("expected second result content from context fallback")
	}
	if result.Results[0].PublishedAt == nil || result.Results[0].PublishedAt.Year() != 2025 {
		t.Fatalf("expected RFC3339 published date to parse, got %+v", result.Results[0].PublishedAt)
	}
}

func TestParseExaPublishedAt_SupportsCommonFormats(t *testing.T) {
	t.Run("rfc3339", func(t *testing.T) {
		parsed, ok := parseExaPublishedAt("2025-01-01T00:00:00.000Z")
		if !ok || parsed == nil {
			t.Fatal("expected RFC3339 timestamp to parse")
		}
		if parsed.Year() != 2025 {
			t.Fatalf("expected year 2025, got %d", parsed.Year())
		}
	})

	t.Run("date-only", func(t *testing.T) {
		parsed, ok := parseExaPublishedAt("2025-01-01")
		if !ok || parsed == nil {
			t.Fatal("expected date-only timestamp to parse")
		}
		if parsed.Month() != time.January {
			t.Fatalf("expected January, got %s", parsed.Month())
		}
	})
}
