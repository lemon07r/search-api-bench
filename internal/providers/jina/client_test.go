package jina

import (
	"strings"
	"testing"
	"time"
)

func TestNewClient_DefaultRetryPolicies(t *testing.T) {
	t.Setenv("JINA_API_KEY", "")
	t.Setenv("JINA_TIMEOUT", "")
	t.Setenv("JINA_SEARCH_TIMEOUT", "")

	client, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	if client.searchRetryCfg.MaxRetries != 2 {
		t.Fatalf("expected search retries=2, got %d", client.searchRetryCfg.MaxRetries)
	}
	if client.extractRetryCfg.MaxRetries != 1 {
		t.Fatalf("expected extract retries=1, got %d", client.extractRetryCfg.MaxRetries)
	}
	if client.searchTimeout != defaultSearchTimeout {
		t.Fatalf("expected default search timeout %v, got %v", defaultSearchTimeout, client.searchTimeout)
	}
}

func TestNewClient_SearchTimeoutOverride(t *testing.T) {
	t.Setenv("JINA_SEARCH_TIMEOUT", "3s")

	client, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	if client.searchTimeout.String() != "3s" {
		t.Fatalf("expected search timeout 3s, got %v", client.searchTimeout)
	}
}

func TestParsePlainTextSearchResponse_MultipleResults(t *testing.T) {
	client := &Client{}
	start := time.Now()

	body := strings.Join([]string{
		"[1] Title: France",
		"[1] URL Source: https://en.wikipedia.org/wiki/France",
		"[1] Description: Country in Western Europe",
		"Paris is the capital and largest city.",
		"",
		"[2] Title: Paris Demographics",
		"[2] URL Source: https://en.wikipedia.org/wiki/Demographics_of_Paris",
		"[2] Description: Population and demographics",
	}, "\n")

	result, err := client.parsePlainTextSearchResponse("capital of france population", body, start)
	if err != nil {
		t.Fatalf("parsePlainTextSearchResponse() error = %v", err)
	}
	if result.TotalResults != 2 {
		t.Fatalf("expected 2 parsed results, got %d", result.TotalResults)
	}
	if result.Results[0].Title != "France" {
		t.Fatalf("expected first title France, got %q", result.Results[0].Title)
	}
	if result.Results[0].URL != "https://en.wikipedia.org/wiki/France" {
		t.Fatalf("expected first URL parsed, got %q", result.Results[0].URL)
	}
	if !strings.Contains(strings.ToLower(result.Results[0].Content), "paris") {
		t.Fatalf("expected first content to include paris, got %q", result.Results[0].Content)
	}
}

func TestParsePlainTextSearchResponse_DoesNotFailOnWordErrorInContent(t *testing.T) {
	client := &Client{}
	start := time.Now()

	body := "[1] Title: Reliability engineering\n[1] URL Source: https://example.com\nA lower error budget can improve reliability."
	result, err := client.parsePlainTextSearchResponse("error budget", body, start)
	if err != nil {
		t.Fatalf("parsePlainTextSearchResponse() unexpected error: %v", err)
	}
	if result.TotalResults != 1 {
		t.Fatalf("expected 1 result, got %d", result.TotalResults)
	}
	if !strings.Contains(strings.ToLower(result.Results[0].Content), "error budget") {
		t.Fatalf("expected parsed content to retain phrase, got %q", result.Results[0].Content)
	}
}

func TestParsePlainTextSearchResponse_DetectsExplicitError(t *testing.T) {
	client := &Client{}
	start := time.Now()

	_, err := client.parsePlainTextSearchResponse("capital of france population", "Error: service unavailable", start)
	if err == nil {
		t.Fatal("expected error for explicit plain-text error response")
	}
}
