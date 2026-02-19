package quality

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/lamim/sanity-web-eval/internal/providers"
	"github.com/lamim/sanity-web-eval/internal/providers/testutil"
)

func TestScoreSearch_PartialSignalFallbackUsesReranker(t *testing.T) {
	embeddingServer := testutil.NewIPv4Server(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"detail":"invalid request"}`))
	}))
	defer embeddingServer.Close()

	rerankerServer := testutil.NewIPv4Server(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rerank" {
			t.Fatalf("expected /rerank path, got %s", r.URL.Path)
		}
		resp := map[string]interface{}{
			"results": []map[string]interface{}{
				{"index": 0, "text": "Paris population details", "relevance_score": 0.92},
				{"index": 1, "text": "France demographics", "relevance_score": 0.81},
			},
			"usage": map[string]int{
				"prompt_tokens":     10,
				"completion_tokens": 0,
				"total_tokens":      10,
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer rerankerServer.Close()

	embeddingClient, err := NewEmbeddingClientWithOptions(embeddingServer.URL, "test-key", "test-model", DefaultEmbeddingOptions())
	if err != nil {
		t.Fatalf("NewEmbeddingClientWithOptions() error = %v", err)
	}
	rerankerClient, err := NewRerankerClientWithOptions(rerankerServer.URL, "test-key", "test-model", DefaultRerankerOptions())
	if err != nil {
		t.Fatalf("NewRerankerClientWithOptions() error = %v", err)
	}

	scorer := NewScorer(embeddingClient, rerankerClient)
	results := []providers.SearchItem{
		{Title: "Paris Population", URL: "https://example.com/1", Content: "Paris has millions of residents."},
		{Title: "France", URL: "https://example.com/2", Content: "France is in Europe."},
	}

	score, err := scorer.ScoreSearch(context.Background(), "capital of france population", results)
	if err != nil {
		t.Fatalf("ScoreSearch() unexpected error = %v", err)
	}
	if score.SemanticAvailable {
		t.Fatal("expected semantic signal to be unavailable after embedding 400")
	}
	if !score.RerankerAvailable {
		t.Fatal("expected reranker signal to be available")
	}
	if score.RerankerScore <= 0 {
		t.Fatalf("expected reranker score > 0, got %.2f", score.RerankerScore)
	}
	if score.OverallScore <= 0 {
		t.Fatalf("expected overall score > 0, got %.2f", score.OverallScore)
	}
}

func TestScoreSearch_TruncatesSemanticEmbeddingInputs(t *testing.T) {
	var capturedInputs []string
	embeddingServer := testutil.NewIPv4Server(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embeddings" {
			t.Fatalf("expected /embeddings path, got %s", r.URL.Path)
		}

		var req struct {
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode embedding request: %v", err)
		}
		capturedInputs = append([]string(nil), req.Input...)

		data := make([]map[string]interface{}, 0, len(req.Input))
		for idx := range req.Input {
			data = append(data, map[string]interface{}{
				"object":    "embedding",
				"index":     idx,
				"embedding": []float64{1, 0, 0},
			})
		}
		resp := map[string]interface{}{
			"object": "list",
			"data":   data,
			"model":  "test-model",
			"usage": map[string]int{
				"prompt_tokens": 10,
				"total_tokens":  10,
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer embeddingServer.Close()

	embeddingClient, err := NewEmbeddingClientWithOptions(embeddingServer.URL, "test-key", "test-model", DefaultEmbeddingOptions())
	if err != nil {
		t.Fatalf("NewEmbeddingClientWithOptions() error = %v", err)
	}

	scorer := NewScorer(embeddingClient, nil)
	hugeContent := strings.Repeat("x", semanticSearchMaxDocumentChars*3)
	results := []providers.SearchItem{
		{Title: "Very Long Result", URL: "https://example.com", Content: hugeContent},
	}

	score, err := scorer.ScoreSearch(context.Background(), "test query", results)
	if err != nil {
		t.Fatalf("ScoreSearch() error = %v", err)
	}
	if !score.SemanticAvailable {
		t.Fatal("expected semantic signal to be available")
	}
	if len(capturedInputs) != 2 {
		t.Fatalf("expected 2 embedding inputs (query + 1 result), got %d", len(capturedInputs))
	}
	if len(capturedInputs[1]) > semanticSearchMaxDocumentChars {
		t.Fatalf("expected truncated semantic input <= %d chars, got %d", semanticSearchMaxDocumentChars, len(capturedInputs[1]))
	}
}
