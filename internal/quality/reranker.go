package quality

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"
)

const (
	defaultRerankerModel = "Qwen/Qwen3-Reranker-8B"
	defaultTopN          = 10
	rerankerMaxRetries   = 3
	rerankerRetryDelay   = 1 * time.Second
)

// RerankerOptions configures reranking behavior
type RerankerOptions struct {
	// Instruction is a task-specific prompt to improve relevance
	Instruction string

	// MaxTokensPerDoc limits the length of each document
	// Default: 0 (no limit, uses full text)
	MaxTokensPerDoc int

	// ReturnDocuments indicates whether to return the document text in results
	ReturnDocuments bool
}

// DefaultRerankerOptions returns recommended default options
func DefaultRerankerOptions() RerankerOptions {
	return RerankerOptions{
		Instruction:     "",
		MaxTokensPerDoc: 0,
		ReturnDocuments: true,
	}
}

// BatchRerankRequest represents a single rerank operation in a batch
type BatchRerankRequest struct {
	QueryID   string   `json:"query_id"`
	Query     string   `json:"query"`
	Documents []string `json:"documents"`
}

// BatchRerankResponse represents a batch reranking result
type BatchRerankResponse struct {
	Results []BatchRerankResult `json:"results"`
	Usage   struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// BatchRerankResult represents results for a single query in a batch
type BatchRerankResult struct {
	QueryID string         `json:"query_id"`
	Results []RerankResult `json:"results"`
}

// RerankerClient provides document reranking via API with Qwen3 support
type RerankerClient struct {
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
	options    RerankerOptions
	mu         sync.RWMutex
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
// Defaults to Qwen3-Reranker-8B
func NewRerankerClient() (*RerankerClient, error) {
	baseURL := os.Getenv("RERANKER_MODEL_BASE_URL")
	if baseURL == "" {
		return nil, fmt.Errorf("RERANKER_MODEL_BASE_URL not set")
	}

	apiKey := os.Getenv("RERANKER_MODEL_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("RERANKER_MODEL_API_KEY not set")
	}

	// Use RERANKER_MODEL env var if set, otherwise use default
	model := os.Getenv("RERANKER_MODEL")
	if model == "" {
		model = defaultRerankerModel
	}

	return NewRerankerClientWithOptions(baseURL, apiKey, model, DefaultRerankerOptions())
}

// NewRerankerClientWithOptions creates a client with custom configuration
func NewRerankerClientWithOptions(baseURL, apiKey, model string, options RerankerOptions) (*RerankerClient, error) {
	return &RerankerClient{
		baseURL: baseURL,
		apiKey:  apiKey,
		model:   model,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		options: options,
	}, nil
}

// SetOptions updates the reranker options
func (c *RerankerClient) SetOptions(options RerankerOptions) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.options = options
}

// GetOptions returns current reranker options
func (c *RerankerClient) GetOptions() RerankerOptions {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.options
}

// SetModel changes the reranker model
func (c *RerankerClient) SetModel(model string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.model = model
}

// Rerank ranks documents by relevance to the query with Qwen3 instruction support
func (c *RerankerClient) Rerank(ctx context.Context, query string, documents []string) ([]RerankResult, error) {
	if len(documents) == 0 {
		return []RerankResult{}, nil
	}
	c.mu.RLock()
	model := c.model
	options := c.options
	c.mu.RUnlock()

	payload := c.buildRequestPayload(model, options, query, documents, len(documents))

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	var result RerankResponse
	var lastErr error

	for attempt := 0; attempt < rerankerMaxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 1s, 2s, 4s
			delay := time.Duration(1<<(attempt-1)) * rerankerRetryDelay
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/rerank", bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("request failed (attempt %d/%d): %w", attempt+1, rerankerMaxRetries, err)
			continue // Retry on network errors
		}

		respBody, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("failed to read response (attempt %d/%d): %w", attempt+1, rerankerMaxRetries, err)
			continue
		}

		if isRetryableQualityStatus(resp.StatusCode, string(respBody)) {
			// Retry on known transient failures.
			lastErr = fmt.Errorf("API returned status %d (attempt %d/%d): %s", resp.StatusCode, attempt+1, rerankerMaxRetries, string(respBody))
			continue
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(respBody))
		}

		if err := json.Unmarshal(respBody, &result); err != nil {
			return nil, fmt.Errorf("failed to unmarshal response: %w", err)
		}

		// Success - break out of retry loop
		lastErr = nil
		break
	}

	if lastErr != nil {
		return nil, lastErr
	}

	return result.Results, nil
}

// buildRequestPayload creates the API request payload with instruction support
func (c *RerankerClient) buildRequestPayload(model string, options RerankerOptions, query string, documents []string, topN int) map[string]interface{} {
	payload := map[string]interface{}{
		"model":            model,
		"query":            query,
		"documents":        documents,
		"top_n":            topN,
		"return_documents": options.ReturnDocuments,
	}

	// Add instruction if provided (Qwen3 is instruction-aware)
	if options.Instruction != "" {
		payload["instruction"] = options.Instruction
	}

	// Add max tokens per document if specified
	if options.MaxTokensPerDoc > 0 {
		payload["max_tokens_per_doc"] = options.MaxTokensPerDoc
	}

	return payload
}

// RerankWithOptions reranks with specific options for this request only
func (c *RerankerClient) RerankWithOptions(ctx context.Context, query string, documents []string, options RerankerOptions, topN int) ([]RerankResult, error) {
	if topN <= 0 {
		topN = len(documents)
	}

	// Use built-in min function (Go 1.21+)
	n := len(documents)
	if topN < n {
		n = topN
	}
	c.mu.RLock()
	model := c.model
	c.mu.RUnlock()
	payload := c.buildRequestPayload(model, options, query, documents[:n], n)
	return c.rerankWithPayload(ctx, payload)
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

func (c *RerankerClient) rerankWithPayload(ctx context.Context, payload map[string]interface{}) ([]RerankResult, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	var result RerankResponse
	var lastErr error

	for attempt := 0; attempt < rerankerMaxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 1s, 2s, 4s
			delay := time.Duration(1<<(attempt-1)) * rerankerRetryDelay
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/rerank", bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("request failed (attempt %d/%d): %w", attempt+1, rerankerMaxRetries, err)
			continue // Retry on network errors
		}

		respBody, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("failed to read response (attempt %d/%d): %w", attempt+1, rerankerMaxRetries, err)
			continue
		}

		if isRetryableQualityStatus(resp.StatusCode, string(respBody)) {
			// Retry on known transient failures.
			lastErr = fmt.Errorf("API returned status %d (attempt %d/%d): %s", resp.StatusCode, attempt+1, rerankerMaxRetries, string(respBody))
			continue
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(respBody))
		}

		if err := json.Unmarshal(respBody, &result); err != nil {
			return nil, fmt.Errorf("failed to unmarshal response: %w", err)
		}

		// Success - break out of retry loop
		lastErr = nil
		break
	}

	if lastErr != nil {
		return nil, lastErr
	}

	return result.Results, nil
}

// BatchRerank performs reranking for multiple queries in parallel
// Falls back to concurrent individual requests if batch endpoint not available
func (c *RerankerClient) BatchRerank(ctx context.Context, requests []BatchRerankRequest) ([]BatchRerankResult, error) {
	if len(requests) == 0 {
		return []BatchRerankResult{}, nil
	}

	// Try batch endpoint first (if supported by API)
	results, err := c.tryBatchEndpoint(ctx, requests)
	if err == nil {
		return results, nil
	}

	// Fallback to concurrent individual requests
	return c.batchRerankConcurrent(ctx, requests)
}

// tryBatchEndpoint attempts to use the batch rerank endpoint
func (c *RerankerClient) tryBatchEndpoint(ctx context.Context, requests []BatchRerankRequest) ([]BatchRerankResult, error) {
	c.mu.RLock()
	model := c.model
	options := c.options
	c.mu.RUnlock()

	payload := map[string]interface{}{
		"model":    model,
		"requests": requests,
	}

	if options.Instruction != "" {
		payload["instruction"] = options.Instruction
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/rerank/batch", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("batch endpoint returned %d", resp.StatusCode)
	}

	var result BatchRerankResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result.Results, nil
}

// batchRerankConcurrent performs reranking using concurrent individual requests
func (c *RerankerClient) batchRerankConcurrent(ctx context.Context, requests []BatchRerankRequest) ([]BatchRerankResult, error) {
	results := make([]BatchRerankResult, len(requests))
	var wg sync.WaitGroup
	errChan := make(chan error, len(requests))
	sem := make(chan struct{}, 5) // Limit concurrency to 5

	for i, req := range requests {
		wg.Add(1)
		go func(index int, r BatchRerankRequest) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			rerankResults, err := c.Rerank(ctx, r.Query, r.Documents)
			if err != nil {
				errChan <- fmt.Errorf("query %s failed: %w", r.QueryID, err)
				return
			}

			results[index] = BatchRerankResult{
				QueryID: r.QueryID,
				Results: rerankResults,
			}
		}(i, req)
	}

	wg.Wait()
	close(errChan)

	// Check for any errors
	var errs []error
	for err := range errChan {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return results, fmt.Errorf("batch rerank had %d errors: %v", len(errs), errs[0])
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

// NormalizeScore converts reranker score to 0-100 scale (legacy - supports BGE models)
// BGE reranker typically returns scores around -10 to 10
func NormalizeScore(score float64) float64 {
	// Map -10 to 10 range to 0-100
	normalized := (score + 10) / 20
	if normalized < 0 {
		normalized = 0
	}
	if normalized > 1 {
		normalized = 1
	}
	return normalized * 100
}

// NormalizeQwen3Score converts Qwen3 reranker score to 0-100 scale
// Qwen3 reranker outputs 0-1 probability scores directly
func NormalizeQwen3Score(score float64) float64 {
	// Qwen3 outputs 0-1 range directly - simple scaling
	score *= 100
	if score < 0 {
		return 0
	}
	if score > 100 {
		return 100
	}
	return score
}

// AutoNormalizeScore automatically detects and normalizes based on score range
func AutoNormalizeScore(score float64) float64 {
	// Detect score range:
	// Qwen3: 0-1 (or slightly beyond due to floating point)
	// BGE: -10 to 10 (or beyond)
	if score >= 0 && score <= 1.5 {
		// Likely Qwen3 (0-1 range)
		return NormalizeQwen3Score(score)
	}
	// Likely BGE or other model with wider range
	return NormalizeScore(score)
}
