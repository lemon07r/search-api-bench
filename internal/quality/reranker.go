package quality

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

const (
	defaultRerankerModel = "BAAI/bge-reranker-v2-m3"
	defaultTopN          = 10
)

// RerankerClient provides document reranking via Novita AI
type RerankerClient struct {
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
}

// RerankResult represents a single reranked document
type RerankResult struct {
	Index      int     `json:"index"`
	Text       string  `json:"text"`
	Relevance  float64 `json:"relevance_score"`
	DocumentID string  `json:"document_id,omitempty"`
}

// RerankResponse represents the API response
type RerankResponse struct {
	Results []RerankResult `json:"results"`
	Usage   struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// RerankRequest represents the API request
type RerankRequest struct {
	Model     string   `json:"model"`
	Query     string   `json:"query"`
	Documents []string `json:"documents"`
	TopN      int      `json:"top_n,omitempty"`
}

// NewRerankerClient creates a new reranker client from environment variables
func NewRerankerClient() (*RerankerClient, error) {
	baseURL := os.Getenv("RERANKER_MODEL_BASE_URL")
	if baseURL == "" {
		return nil, fmt.Errorf("RERANKER_MODEL_BASE_URL not set")
	}

	apiKey := os.Getenv("RERANKER_MODEL_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("RERANKER_MODEL_API_KEY not set")
	}

	return &RerankerClient{
		baseURL: baseURL,
		apiKey:  apiKey,
		model:   defaultRerankerModel,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

// SetModel changes the reranker model
func (c *RerankerClient) SetModel(model string) {
	c.model = model
}

// Rerank ranks documents by relevance to the query
func (c *RerankerClient) Rerank(ctx context.Context, query string, documents []string) ([]RerankResult, error) {
	if len(documents) == 0 {
		return []RerankResult{}, nil
	}

	payload := RerankRequest{
		Model:     c.model,
		Query:     query,
		Documents: documents,
		TopN:      len(documents), // Get scores for all
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/rerank", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result RerankResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return result.Results, nil
}

// RerankWithLimit returns top N documents after reranking
func (c *RerankerClient) RerankWithLimit(ctx context.Context, query string, documents []string, topN int) ([]RerankResult, error) {
	results, err := c.Rerank(ctx, query, documents)
	if err != nil {
		return nil, err
	}

	if topN > 0 && topN < len(results) {
		return results[:topN], nil
	}
	return results, nil
}

// CalculateAverageScore computes the mean relevance score
func CalculateAverageScore(results []RerankResult) float64 {
	if len(results) == 0 {
		return 0
	}

	var sum float64
	for _, r := range results {
		sum += r.Relevance
	}
	return sum / float64(len(results))
}

// CalculateTopKScore computes average of top K scores
func CalculateTopKScore(results []RerankResult, k int) float64 {
	if len(results) == 0 {
		return 0
	}

	if k > len(results) {
		k = len(results)
	}

	var sum float64
	for i := 0; i < k; i++ {
		sum += results[i].Relevance
	}
	return sum / float64(k)
}

// NormalizeScore converts reranker score to 0-100 scale
// Novita reranker typically returns scores in various ranges depending on model
func NormalizeScore(score float64) float64 {
	// BGE reranker typically returns scores around -10 to 10
	// Map to 0-100 using sigmoid-like transformation
	// First clamp to reasonable range, then normalize

	// Assuming scores can be negative, map to 0-1 then to 0-100
	normalized := (score + 10) / 20 // rough estimate for BGE
	if normalized < 0 {
		normalized = 0
	}
	if normalized > 1 {
		normalized = 1
	}
	return normalized * 100
}
