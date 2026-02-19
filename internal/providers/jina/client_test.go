package jina

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/lamim/sanity-web-eval/internal/providers"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func newJSONResponse(statusCode int, body string, req *http.Request) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Header: http.Header{
			"Content-Type": []string{"application/json"},
		},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
		Status:     http.StatusText(statusCode),
		ProtoMajor: 1,
		ProtoMinor: 1,
	}
}

func newTextResponse(statusCode int, body string, req *http.Request) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Header: http.Header{
			"Content-Type": []string{"text/plain; charset=utf-8"},
		},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
		Status:     http.StatusText(statusCode),
		ProtoMajor: 1,
		ProtoMinor: 1,
	}
}

func TestNewClient_DefaultRetryPolicies(t *testing.T) {
	t.Setenv("JINA_API_KEY", "")
	t.Setenv("JINA_TIMEOUT", "")
	t.Setenv("JINA_SEARCH_TIMEOUT", "")
	t.Setenv("JINA_SEARCH_MAX_RETRIES", "")
	t.Setenv("JINA_EXTRACT_MAX_RETRIES", "")
	t.Setenv("JINA_SEARCH_MAX_RESULTS", "")
	t.Setenv("JINA_SEARCH_NO_CONTENT", "")
	t.Setenv("JINA_EXTRACT_TOKEN_BUDGET", "")
	t.Setenv("JINA_CRAWL_TOKEN_BUDGET", "")
	t.Setenv("JINA_WITH_GENERATED_ALT", "")

	client, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	if client.searchRetryCfg.MaxRetries != defaultSearchMaxRetries {
		t.Fatalf("expected search retries=%d, got %d", defaultSearchMaxRetries, client.searchRetryCfg.MaxRetries)
	}
	if client.extractRetryCfg.MaxRetries != defaultExtractMaxRetries {
		t.Fatalf("expected extract retries=%d, got %d", defaultExtractMaxRetries, client.extractRetryCfg.MaxRetries)
	}
	if client.searchTimeout != defaultSearchTimeout {
		t.Fatalf("expected default search timeout %v, got %v", defaultSearchTimeout, client.searchTimeout)
	}
	if client.searchMaxResult != defaultSearchMaxResults {
		t.Fatalf("expected search max results %d, got %d", defaultSearchMaxResults, client.searchMaxResult)
	}
	if !client.searchNoContent {
		t.Fatal("expected search no-content mode enabled by default")
	}
	if client.extractBudget != defaultExtractTokenBudget {
		t.Fatalf("expected extract budget %d, got %d", defaultExtractTokenBudget, client.extractBudget)
	}
	if client.crawlBudget != defaultCrawlTokenBudget {
		t.Fatalf("expected crawl budget %d, got %d", defaultCrawlTokenBudget, client.crawlBudget)
	}
}

func TestNewClient_SearchTimeoutOverride(t *testing.T) {
	t.Setenv("JINA_SEARCH_TIMEOUT", "3s")
	t.Setenv("JINA_SEARCH_MAX_RETRIES", "2")
	t.Setenv("JINA_EXTRACT_MAX_RETRIES", "1")
	t.Setenv("JINA_SEARCH_MAX_RESULTS", "5")
	t.Setenv("JINA_SEARCH_NO_CONTENT", "false")
	t.Setenv("JINA_EXTRACT_TOKEN_BUDGET", "9000")
	t.Setenv("JINA_CRAWL_TOKEN_BUDGET", "7000")
	t.Setenv("JINA_WITH_GENERATED_ALT", "true")

	client, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	if client.searchTimeout.String() != "3s" {
		t.Fatalf("expected search timeout 3s, got %v", client.searchTimeout)
	}
	if client.searchRetryCfg.MaxRetries != 2 {
		t.Fatalf("expected search retries=2, got %d", client.searchRetryCfg.MaxRetries)
	}
	if client.extractRetryCfg.MaxRetries != 1 {
		t.Fatalf("expected extract retries=1, got %d", client.extractRetryCfg.MaxRetries)
	}
	if client.searchMaxResult != 5 {
		t.Fatalf("expected search max results=5, got %d", client.searchMaxResult)
	}
	if client.searchNoContent {
		t.Fatal("expected search no-content mode to be disabled via env override")
	}
	if client.extractBudget != 9000 {
		t.Fatalf("expected extract budget=9000, got %d", client.extractBudget)
	}
	if client.crawlBudget != 7000 {
		t.Fatalf("expected crawl budget=7000, got %d", client.crawlBudget)
	}
	if !client.withGeneratedAlt {
		t.Fatal("expected generated-alt to be enabled via env override")
	}
}

func TestSearch_PostRequestWithJSONResponse(t *testing.T) {
	var gotMethod, gotAuth, gotAccept, gotContentType, gotRespondWith, gotRetainImages string
	var gotBody searchRequest
	clientHTTP := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			gotMethod = r.Method
			gotAuth = r.Header.Get("Authorization")
			gotAccept = r.Header.Get("Accept")
			gotContentType = r.Header.Get("Content-Type")
			gotRespondWith = r.Header.Get("X-Respond-With")
			gotRetainImages = r.Header.Get("X-Retain-Images")
			json.NewDecoder(r.Body).Decode(&gotBody)

			resp := searchResponse{
				Code: 200,
				Data: []struct {
					Title   string `json:"title"`
					URL     string `json:"url"`
					Content string `json:"content"`
				}{
					{Title: "Paris", URL: "https://en.wikipedia.org/wiki/Paris", Content: "Capital of France"},
					{Title: "France Population", URL: "https://www.worldometers.info/world-population/france-population/", Content: "Latest population stats"},
				},
			}
			respJSON, _ := json.Marshal(resp)
			return newJSONResponse(http.StatusOK, string(respJSON), r), nil
		}),
	}

	client := &Client{
		apiKey:          "test-key",
		httpClient:      clientHTTP,
		searchBaseURL:   "https://s.jina.ai",
		searchRetryCfg:  providers.RetryConfig{MaxRetries: 0},
		searchTimeout:   2 * time.Second,
		searchMaxResult: 3,
		searchNoContent: true,
	}

	result, err := client.Search(context.Background(), "capital of france population", providers.SearchOptions{MaxResults: 2})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if gotMethod != "POST" {
		t.Fatalf("expected POST request, got %s", gotMethod)
	}
	if gotAuth == "" {
		t.Fatal("expected Authorization header to be set")
	}
	if gotAccept != "application/json" {
		t.Fatalf("expected Accept=application/json, got %q", gotAccept)
	}
	if gotContentType != "application/json" {
		t.Fatalf("expected Content-Type=application/json, got %q", gotContentType)
	}
	if gotRespondWith != "no-content" {
		t.Fatalf("expected X-Respond-With=no-content, got %q", gotRespondWith)
	}
	if gotRetainImages != "none" {
		t.Fatalf("expected X-Retain-Images=none, got %q", gotRetainImages)
	}
	if gotBody.Query != "capital of france population" {
		t.Fatalf("unexpected query value %q", gotBody.Query)
	}
	if result.TotalResults != 2 {
		t.Fatalf("expected 2 results, got %d", result.TotalResults)
	}
	if result.Results[0].Title != "Paris" {
		t.Fatalf("expected first title Paris, got %q", result.Results[0].Title)
	}
	if result.CreditsUsed < minSearchTokens {
		t.Fatalf("expected at least %d tokens (min search), got %d", minSearchTokens, result.CreditsUsed)
	}
}

func TestExtract_JSONResponseWithWrappedData(t *testing.T) {
	var gotAccept, gotTokenBudget, gotRetainImages string
	clientHTTP := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			gotAccept = r.Header.Get("Accept")
			gotTokenBudget = r.Header.Get("X-Token-Budget")
			gotRetainImages = r.Header.Get("X-Retain-Images")

			resp := readerResponse{
				Code:   200,
				Status: 20000,
				Data: readerDataItem{
					URL:       "https://example.com",
					Title:     "Sample Page",
					Content:   "# Heading\nHello world",
					Timestamp: "2025-01-01T00:00:00Z",
				},
			}
			respJSON, _ := json.Marshal(resp)
			return newJSONResponse(http.StatusOK, string(respJSON), r), nil
		}),
	}

	client := &Client{
		httpClient:      clientHTTP,
		readerBaseURL:   "https://r.jina.ai",
		extractBudget:   1234,
		extractRetryCfg: providers.RetryConfig{MaxRetries: 0},
	}

	result, err := client.Extract(context.Background(), "https://example.com", providers.DefaultExtractOptions())
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	if gotAccept != "application/json" {
		t.Fatalf("expected Accept=application/json, got %q", gotAccept)
	}
	if gotTokenBudget != "1234" {
		t.Fatalf("expected X-Token-Budget=1234, got %q", gotTokenBudget)
	}
	if gotRetainImages != "none" {
		t.Fatalf("expected X-Retain-Images=none, got %q", gotRetainImages)
	}
	if result.Title != "Sample Page" {
		t.Fatalf("expected title Sample Page, got %q", result.Title)
	}
	if result.Content != "# Heading\nHello world" {
		t.Fatalf("expected content from data wrapper, got %q", result.Content)
	}
	if result.Metadata["url"] != "https://example.com" {
		t.Fatalf("expected metadata url=https://example.com, got %v", result.Metadata["url"])
	}
	if result.CreditsUsed != tokenEstimateFromString(result.Content) {
		t.Fatalf("expected credits to match token estimate, got %d", result.CreditsUsed)
	}
}

func TestExtract_PlainTextFallback(t *testing.T) {
	clientHTTP := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return newTextResponse(http.StatusOK, strings.Join([]string{
				"Title: Sample Page",
				"URL Source: https://example.com",
				"",
				"Markdown Content:",
				"# Heading",
				"Hello world",
			}, "\n"), r), nil
		}),
	}

	client := &Client{
		httpClient:      clientHTTP,
		readerBaseURL:   "https://r.jina.ai",
		extractBudget:   1234,
		extractRetryCfg: providers.RetryConfig{MaxRetries: 0},
	}

	result, err := client.Extract(context.Background(), "https://example.com", providers.DefaultExtractOptions())
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	if result.Title != "Sample Page" {
		t.Fatalf("expected title Sample Page, got %q", result.Title)
	}
}

func TestCrawl_UsesCrawlTokenBudget(t *testing.T) {
	var gotTokenBudget string
	clientHTTP := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			gotTokenBudget = r.Header.Get("X-Token-Budget")
			resp := readerResponse{
				Code:   200,
				Status: 20000,
				Data: readerDataItem{
					URL:     "https://example.com",
					Title:   "Crawl Page",
					Content: "hello",
				},
			}
			respJSON, _ := json.Marshal(resp)
			return newJSONResponse(http.StatusOK, string(respJSON), r), nil
		}),
	}

	client := &Client{
		httpClient:      clientHTTP,
		readerBaseURL:   "https://r.jina.ai",
		crawlBudget:     4321,
		extractRetryCfg: providers.RetryConfig{MaxRetries: 0},
	}

	result, err := client.Crawl(context.Background(), "https://example.com", providers.DefaultCrawlOptions())
	if err != nil {
		t.Fatalf("Crawl() error = %v", err)
	}

	if gotTokenBudget != "4321" {
		t.Fatalf("expected X-Token-Budget=4321, got %q", gotTokenBudget)
	}
	if result.TotalPages != 1 {
		t.Fatalf("expected total pages=1, got %d", result.TotalPages)
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
	if result.CreditsUsed < minSearchTokens {
		t.Fatalf("expected at least %d tokens (min search), got %d", minSearchTokens, result.CreditsUsed)
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
