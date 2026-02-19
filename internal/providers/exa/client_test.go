package exa

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/lamim/sanity-web-eval/internal/providers"
	"github.com/lamim/sanity-web-eval/internal/providers/testutil"
)

func TestSearch_ReturnsInlineTextContent(t *testing.T) {
	server := testutil.NewIPv4Server(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search" {
			t.Fatalf("expected /search path, got %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST method, got %s", r.Method)
		}

		resp := searchResponse{
			RequestID: "req-1",
			Results: []exaSearchHit{
				{
					Title:         "Paris Population 2026",
					URL:           "https://worldpopulationreview.com/cities/france/paris",
					Text:          "The estimated population is 11,418,300.",
					PublishedDate: "2025-01-01T00:00:00.000Z",
				},
				{
					Title: "Demographics of Paris",
					URL:   "https://en.wikipedia.org/wiki/Demographics_of_Paris",
					Text:  "The city had 2.1 million inhabitants.",
				},
			},
			CostDollars: costDollars{Total: 0.006},
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
		t.Fatal("expected first result content from inline text")
	}
	if result.Results[1].Content == "" {
		t.Fatal("expected second result content from inline text")
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
