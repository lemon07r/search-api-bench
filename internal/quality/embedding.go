// Package quality provides AI-powered quality evaluation using embeddings and reranking
package quality

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"sync"
	"time"
)

const (
	defaultEmbeddingModel = "BAAI/bge-en-icl"
	defaultTimeout        = 30 * time.Second
)

// EmbeddingClient provides embedding generation via Nebius API
type EmbeddingClient struct {
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
	cache      *embeddingCache
}

// EmbeddingResponse represents the API response
type EmbeddingResponse struct {
	Object string `json:"object"`
	Data   []struct {
		Object    string    `json:"object"`
		Embedding []float64 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Model string `json:"model"`
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
}

// embeddingCache provides simple in-memory caching for embeddings
type embeddingCache struct {
	mu    sync.RWMutex
	items map[string][]float64
}

// NewEmbeddingClient creates a new embedding client from environment variables
func NewEmbeddingClient() (*EmbeddingClient, error) {
	baseURL := os.Getenv("EMBEDDING_MODEL_BASE_URL")
	if baseURL == "" {
		return nil, fmt.Errorf("EMBEDDING_MODEL_BASE_URL not set")
	}

	apiKey := os.Getenv("EMBEDDING_EMBEDDING_MODEL_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("EMBEDDING_EMBEDDING_MODEL_API_KEY not set")
	}

	return &EmbeddingClient{
		baseURL: baseURL,
		apiKey:  apiKey,
		model:   defaultEmbeddingModel,
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
		cache: &embeddingCache{
			items: make(map[string][]float64),
		},
	}, nil
}

// SetModel changes the embedding model
func (c *EmbeddingClient) SetModel(model string) {
	c.model = model
}

// Embed generates embeddings for a batch of texts
func (c *EmbeddingClient) Embed(ctx context.Context, texts []string) ([][]float64, error) {
	if len(texts) == 0 {
		return [][]float64{}, nil
	}

	// Check cache first
	results := make([][]float64, len(texts))
	uncachedIndices := make([]int, 0, len(texts))
	uncachedTexts := make([]string, 0, len(texts))

	for i, text := range texts {
		if cached, ok := c.cache.get(text); ok {
			results[i] = cached
		} else {
			uncachedIndices = append(uncachedIndices, i)
			uncachedTexts = append(uncachedTexts, text)
		}
	}

	// If all cached, return immediately
	if len(uncachedTexts) == 0 {
		return results, nil
	}

	// Fetch uncached embeddings
	payload := map[string]interface{}{
		"model": c.model,
		"input": uncachedTexts,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/embeddings", bytes.NewReader(body))
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

	var result EmbeddingResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// Store in cache and populate results
	for _, data := range result.Data {
		originalIndex := uncachedIndices[data.Index]
		results[originalIndex] = data.Embedding
		c.cache.set(uncachedTexts[data.Index], data.Embedding)
	}

	return results, nil
}

// EmbedSingle generates embedding for a single text
func (c *EmbeddingClient) EmbedSingle(ctx context.Context, text string) ([]float64, error) {
	embeddings, err := c.Embed(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(embeddings) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}
	return embeddings[0], nil
}

// CosineSimilarity calculates cosine similarity between two embeddings
func CosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) {
		return 0
	}

	var dotProduct, normA, normB float64
	for i := 0; i < len(a); i++ {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}

// SimilarityToScore converts cosine similarity (typically -1 to 1) to 0-100 score
func SimilarityToScore(similarity float64) float64 {
	// Cosine similarity is typically 0-1 for positive embeddings
	// Map to 0-100 scale
	score := (similarity) * 100
	if score < 0 {
		return 0
	}
	if score > 100 {
		return 100
	}
	return score
}

// cache methods
func (c *embeddingCache) get(key string) ([]float64, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	val, ok := c.items[key]
	return val, ok
}

func (c *embeddingCache) set(key string, value []float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[key] = value
}

// ClearCache clears the embedding cache
func (c *EmbeddingClient) ClearCache() {
	c.cache.mu.Lock()
	defer c.cache.mu.Unlock()
	c.cache.items = make(map[string][]float64)
}
