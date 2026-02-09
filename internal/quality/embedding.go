// Package quality provides AI-powered quality evaluation using embeddings and reranking
package quality

import (
	"bytes"
	"container/list"
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
	defaultEmbeddingModel = "Qwen/Qwen3-Embedding-8B"
	defaultTimeout        = 30 * time.Second
	defaultCacheTTL       = 5 * time.Minute
	defaultMaxCacheSize   = 1000
)

// Instruction templates for different tasks
const (
	// InstructSearch is optimized for search query relevance
	InstructSearch = "Given a web search query, retrieve relevant passages that answer the query"
	// InstructExtract is optimized for content extraction quality
	InstructExtract = "Given a document, extract the most relevant and complete content"
	// InstructCrawl is optimized for page similarity and relevance
	InstructCrawl = "Given a web page, identify the semantic content for similarity comparison"
	// InstructCode is optimized for code retrieval
	InstructCode = "Given a code query, retrieve relevant code snippets and documentation"
	// InstructQA is optimized for question-answering retrieval
	InstructQA = "Given a question, retrieve passages that can help answer the question"
)

// EmbeddingOptions configures embedding generation
// Supports MRL (Matryoshka Representation Learning) for flexible dimensions
type EmbeddingOptions struct {
	// Dimensions specifies the output embedding dimension (256, 512, 1024, 2048, or 4096 for Qwen3-8B)
	// Lower dimensions = faster, less memory; Higher = more accurate
	// Default: 4096 (full dimension)
	Dimensions int

	// Instruction is a task-specific prompt to improve relevance
	// Use predefined constants: InstructSearch, InstructExtract, InstructCrawl, etc.
	Instruction string

	// Normalize indicates whether to normalize embeddings to unit length
	// Default: true (recommended for cosine similarity)
	Normalize bool
}

// DefaultEmbeddingOptions returns recommended default options
func DefaultEmbeddingOptions() EmbeddingOptions {
	return EmbeddingOptions{
		Dimensions:  4096,
		Instruction: "",
		Normalize:   true,
	}
}

// Validate checks and fixes embedding options
func (o *EmbeddingOptions) Validate() {
	if o.Dimensions <= 0 {
		o.Dimensions = 4096
	}
	// Clamp to valid MRL dimensions for Qwen3 models
	validDims := []int{256, 512, 1024, 2048, 4096}
	valid := false
	for _, d := range validDims {
		if o.Dimensions == d {
			valid = true
			break
		}
	}
	if !valid {
		// Find nearest valid dimension
		for _, d := range validDims {
			if o.Dimensions <= d {
				o.Dimensions = d
				valid = true
				break
			}
		}
		if !valid {
			o.Dimensions = 4096
		}
	}
}

// EmbeddingClient provides embedding generation via API with MRL and instruction support
type EmbeddingClient struct {
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
	cache      *embeddingCache
	options    EmbeddingOptions
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

// cacheEntry represents a single cached embedding with metadata
type cacheEntry struct {
	embedding []float64
	timestamp time.Time
	key       string
}

// embeddingCache provides TTL-aware LRU caching for embeddings
type embeddingCache struct {
	mu        sync.RWMutex
	items     map[string]*list.Element
	lru       *list.List
	ttl       time.Duration
	maxSize   int
	hits      int64
	misses    int64
	evictions int64
}

// newEmbeddingCache creates a new cache with TTL and size limits
func newEmbeddingCache(ttl time.Duration, maxSize int) *embeddingCache {
	if ttl <= 0 {
		ttl = defaultCacheTTL
	}
	if maxSize <= 0 {
		maxSize = defaultMaxCacheSize
	}
	return &embeddingCache{
		items:   make(map[string]*list.Element),
		lru:     list.New(),
		ttl:     ttl,
		maxSize: maxSize,
	}
}

// get retrieves an embedding if it exists and is not expired
func (c *embeddingCache) get(key string) ([]float64, bool) {
	c.mu.RLock()
	elem, exists := c.items[key]
	if !exists {
		c.mu.RUnlock()
		return nil, false
	}
	entry := elem.Value.(*cacheEntry)
	if time.Since(entry.timestamp) > c.ttl {
		c.mu.RUnlock()
		// Expired - will be evicted on next write
		return nil, false
	}
	c.mu.RUnlock()

	// Move to front (most recently used)
	c.mu.Lock()
	c.lru.MoveToFront(elem)
	c.hits++
	c.mu.Unlock()

	return entry.embedding, true
}

// set adds or updates an embedding in the cache
func (c *embeddingCache) set(key string, embedding []float64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if already exists
	if elem, exists := c.items[key]; exists {
		c.lru.MoveToFront(elem)
		elem.Value.(*cacheEntry).embedding = embedding
		elem.Value.(*cacheEntry).timestamp = time.Now()
		return
	}

	// Evict expired entries first
	c.evictExpired()

	// Evict oldest if at capacity
	for c.lru.Len() >= c.maxSize {
		elem := c.lru.Back()
		if elem == nil {
			break
		}
		entry := elem.Value.(*cacheEntry)
		delete(c.items, entry.key)
		c.lru.Remove(elem)
		c.evictions++
	}

	// Add new entry
	entry := &cacheEntry{
		embedding: embedding,
		timestamp: time.Now(),
		key:       key,
	}
	elem := c.lru.PushFront(entry)
	c.items[key] = elem
	c.misses++
}

// evictExpired removes entries older than TTL
func (c *embeddingCache) evictExpired() {
	now := time.Now()
	for elem := c.lru.Back(); elem != nil; {
		entry := elem.Value.(*cacheEntry)
		prev := elem.Prev()
		if now.Sub(entry.timestamp) > c.ttl {
			delete(c.items, entry.key)
			c.lru.Remove(elem)
			c.evictions++
		}
		elem = prev
	}
}

// Stats returns cache statistics
func (c *embeddingCache) Stats() (hits, misses, evictions int64, hitRate float64) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	total := c.hits + c.misses
	if total > 0 {
		hitRate = float64(c.hits) / float64(total) * 100
	}
	return c.hits, c.misses, c.evictions, hitRate
}

// clear removes all entries
func (c *embeddingCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[string]*list.Element)
	c.lru.Init()
}

// NewEmbeddingClient creates a new embedding client from environment variables
// Defaults to Qwen3-Embedding-8B with full 4096 dimensions
func NewEmbeddingClient() (*EmbeddingClient, error) {
	baseURL := os.Getenv("EMBEDDING_MODEL_BASE_URL")
	if baseURL == "" {
		return nil, fmt.Errorf("EMBEDDING_MODEL_BASE_URL not set")
	}

	apiKey := os.Getenv("EMBEDDING_EMBEDDING_MODEL_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("EMBEDDING_EMBEDDING_MODEL_API_KEY not set")
	}

	return NewEmbeddingClientWithOptions(baseURL, apiKey, defaultEmbeddingModel, DefaultEmbeddingOptions())
}

// NewEmbeddingClientWithOptions creates a client with custom configuration
func NewEmbeddingClientWithOptions(baseURL, apiKey, model string, options EmbeddingOptions) (*EmbeddingClient, error) {
	options.Validate()

	ttl := defaultCacheTTL
	if envTTL := os.Getenv("EMBEDDING_CACHE_TTL"); envTTL != "" {
		if d, err := time.ParseDuration(envTTL); err == nil {
			ttl = d
		}
	}

	maxSize := defaultMaxCacheSize
	if envSize := os.Getenv("EMBEDDING_CACHE_SIZE"); envSize != "" {
		if s, err := parseInt(envSize); err == nil {
			maxSize = s
		}
	}

	return &EmbeddingClient{
		baseURL: baseURL,
		apiKey:  apiKey,
		model:   model,
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
		cache:   newEmbeddingCache(ttl, maxSize),
		options: options,
	}, nil
}

// SetOptions updates the embedding options
func (c *EmbeddingClient) SetOptions(options EmbeddingOptions) {
	options.Validate()
	c.options = options
}

// GetOptions returns current embedding options
func (c *EmbeddingClient) GetOptions() EmbeddingOptions {
	return c.options
}

// SetModel changes the embedding model
func (c *EmbeddingClient) SetModel(model string) {
	c.model = model
}

// parseInt helper function
func parseInt(s string) (int, error) {
	var result int
	_, err := fmt.Sscanf(s, "%d", &result)
	return result, err
}

// Embed generates embeddings for a batch of texts with MRL and instruction support
func (c *EmbeddingClient) Embed(ctx context.Context, texts []string) ([][]float64, error) {
	if len(texts) == 0 {
		return [][]float64{}, nil
	}

	// Check cache first (include options in cache key for correctness)
	results := make([][]float64, len(texts))
	uncachedIndices := make([]int, 0, len(texts))
	uncachedTexts := make([]string, 0, len(texts))

	for i, text := range texts {
		cacheKey := c.cacheKey(text)
		if cached, ok := c.cache.get(cacheKey); ok {
			results[i] = c.truncateToDimensions(cached)
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
	payload := c.buildRequestPayload(uncachedTexts)

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
		embedding := data.Embedding

		// Normalize if requested (for unit vector embeddings)
		if c.options.Normalize {
			embedding = normalizeVector(embedding)
		}

		results[originalIndex] = c.truncateToDimensions(embedding)
		cacheKey := c.cacheKey(uncachedTexts[data.Index])
		c.cache.set(cacheKey, embedding)
	}

	return results, nil
}

// buildRequestPayload creates the API request payload with MRL and instruction support
func (c *EmbeddingClient) buildRequestPayload(texts []string) map[string]interface{} {
	payload := map[string]interface{}{
		"model": c.model,
		"input": texts,
	}

	// Add MRL dimensions parameter (Qwen3 supports flexible dimensions)
	if c.options.Dimensions > 0 && c.options.Dimensions < 4096 {
		payload["dimensions"] = c.options.Dimensions
	}

	// Add instruction if provided (Qwen3 is instruction-aware)
	if c.options.Instruction != "" {
		payload["instruction"] = c.options.Instruction
	}

	// Add encoding_format for efficiency
	payload["encoding_format"] = "float"

	return payload
}

// cacheKey generates a unique cache key including options
func (c *EmbeddingClient) cacheKey(text string) string {
	// Include dimensions and normalization in cache key
	return fmt.Sprintf("%s:d=%d:n=%t", text, c.options.Dimensions, c.options.Normalize)
}

// truncateToDimensions truncates embedding to requested dimensions (MRL)
func (c *EmbeddingClient) truncateToDimensions(embedding []float64) []float64 {
	if c.options.Dimensions <= 0 || c.options.Dimensions >= len(embedding) {
		return embedding
	}
	return embedding[:c.options.Dimensions]
}

// normalizeVector normalizes a vector to unit length (L2 norm)
func normalizeVector(v []float64) []float64 {
	var norm float64
	for _, x := range v {
		norm += x * x
	}
	norm = math.Sqrt(norm)

	if norm == 0 {
		return v
	}

	result := make([]float64, len(v))
	for i, x := range v {
		result[i] = x / norm
	}
	return result
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

// EmbedWithInstruction generates embedding with a specific instruction
func (c *EmbeddingClient) EmbedWithInstruction(ctx context.Context, text, instruction string) ([]float64, error) {
	// Temporarily set instruction
	oldOptions := c.options
	c.options.Instruction = instruction
	defer func() { c.options = oldOptions }()

	return c.EmbedSingle(ctx, text)
}

// CacheStats returns cache performance statistics
func (c *EmbeddingClient) CacheStats() (hits, misses, evictions int64, hitRate float64) {
	return c.cache.Stats()
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

// ClearCache clears the embedding cache
func (c *EmbeddingClient) ClearCache() {
	c.cache.clear()
}
