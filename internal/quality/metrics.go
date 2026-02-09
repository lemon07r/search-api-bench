package quality

// ErrorCategory classifies different types of errors
type ErrorCategory int

const (
	// ErrUnknown represents an unknown error category
	ErrUnknown ErrorCategory = iota
	// ErrTimeout represents a timeout error
	ErrTimeout
	// ErrRateLimit represents a rate limit error
	ErrRateLimit
	// ErrAuth represents an authentication error
	ErrAuth
	// ErrServer5xx represents a server 5xx error
	ErrServer5xx
	// ErrClient4xx represents a client 4xx error
	ErrClient4xx
	// ErrNetwork represents a network error
	ErrNetwork
	// ErrParse represents a parse error
	ErrParse
	// ErrContextCanceled represents a context canceled error
	ErrContextCanceled
	// ErrValidation represents a validation error
	ErrValidation
)

func (e ErrorCategory) String() string {
	switch e {
	case ErrTimeout:
		return "timeout"
	case ErrRateLimit:
		return "rate_limit"
	case ErrAuth:
		return "authentication"
	case ErrServer5xx:
		return "server_error"
	case ErrClient4xx:
		return "client_error"
	case ErrNetwork:
		return "network"
	case ErrParse:
		return "parse"
	case ErrContextCanceled:
		return "canceled"
	case ErrValidation:
		return "validation"
	default:
		return "unknown"
	}
}

// CategorizeError analyzes an error and returns its category
func CategorizeError(err error) ErrorCategory {
	if err == nil {
		return ErrUnknown
	}

	errStr := err.Error()

	// Check for context cancellation first
	if contains(errStr, "context canceled") || contains(errStr, "context deadline exceeded") {
		return ErrContextCanceled
	}

	// Check for timeout
	if contains(errStr, "timeout") || contains(errStr, "deadline exceeded") || contains(errStr, "i/o timeout") {
		return ErrTimeout
	}

	// Check for rate limiting
	if contains(errStr, "rate limit") || contains(errStr, "too many requests") || contains(errStr, "429") {
		return ErrRateLimit
	}

	// Check for auth errors
	if contains(errStr, "unauthorized") || contains(errStr, "authentication") || contains(errStr, "api key") ||
		contains(errStr, "401") || contains(errStr, "403") {
		return ErrAuth
	}

	// Check for server errors
	if contains(errStr, "500") || contains(errStr, "502") || contains(errStr, "503") || contains(errStr, "504") {
		return ErrServer5xx
	}

	// Check for client errors
	if contains(errStr, "400") || contains(errStr, "404") || contains(errStr, "405") || contains(errStr, "422") {
		return ErrClient4xx
	}

	// Check for network errors
	if contains(errStr, "connection refused") || contains(errStr, "no such host") ||
		contains(errStr, "temporary failure") || contains(errStr, "network") {
		return ErrNetwork
	}

	// Check for parse errors
	if contains(errStr, "unmarshal") || contains(errStr, "parse") || contains(errStr, "invalid character") {
		return ErrParse
	}

	return ErrUnknown
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsInternal(s, substr))
}

func containsInternal(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// SearchQualityScore represents comprehensive quality metrics for search results
type SearchQualityScore struct {
	SemanticRelevance float64 `json:"semantic_relevance"` // 0-100, embedding similarity
	RerankerScore     float64 `json:"reranker_score"`     // 0-100, reranker confidence
	SemanticAvailable bool    `json:"semantic_available,omitempty"`
	RerankerAvailable bool    `json:"reranker_available,omitempty"`
	TopKAccuracy      float64 `json:"top_k_accuracy"`   // 0-100, relevance of top N
	ResultDiversity   float64 `json:"result_diversity"` // 0-100, domain/content variety
	AuthorityScore    float64 `json:"authority_score"`  // 0-100, domain reputation
	FreshnessScore    float64 `json:"freshness_score"`  // 0-100, content recency
	OverallScore      float64 `json:"overall_score"`    // 0-100, weighted composite
}

// ExtractQualityScore represents quality metrics for content extraction
type ExtractQualityScore struct {
	ContentCompleteness   float64 `json:"content_completeness"`   // 0-100, full vs truncated
	StructurePreservation float64 `json:"structure_preservation"` // 0-100, headers/lists intact
	MarkdownQuality       float64 `json:"markdown_quality"`       // 0-100, valid markdown
	FreshnessScore        float64 `json:"freshness_score"`        // 0-100, content age
	SignalToNoise         float64 `json:"signal_to_noise"`        // 0-100, content vs ads
	CodePreservation      float64 `json:"code_preservation"`      // 0-100, code blocks intact
	OverallScore          float64 `json:"overall_score"`          // 0-100, weighted composite
}

// CrawlQualityScore represents quality metrics for crawl operations
type CrawlQualityScore struct {
	CoverageScore      float64 `json:"coverage_score"`      // 0-100, % of expected pages
	DepthAccuracy      float64 `json:"depth_accuracy"`      // 0-100, reached requested depth
	LinkDiscovery      float64 `json:"link_discovery"`      // 0-100, quality of found links
	ContentConsistency float64 `json:"content_consistency"` // 0-100, quality across pages
	DuplicateRatio     float64 `json:"duplicate_ratio"`     // 0-100, % unique content
	OverallScore       float64 `json:"overall_score"`       // 0-100, weighted composite
}

// DiversityMetrics measures result diversity
type DiversityMetrics struct {
	DomainCount      int      `json:"domain_count"`
	DomainDiversity  float64  `json:"domain_diversity"` // 0-100, entropy of domain distribution
	ContentTypeCount int      `json:"content_type_count"`
	UniqueDomains    []string `json:"unique_domains"`
}

// ComparisonAnalysis provides cross-provider comparison metrics
type ComparisonAnalysis struct {
	TestName          string             `json:"test_name"`
	ResultOverlap     float64            `json:"result_overlap"` // Jaccard similarity
	UniqueToProviderA []string           `json:"unique_to_provider_a"`
	UniqueToProviderB []string           `json:"unique_to_provider_b"`
	SharedResults     []string           `json:"shared_results"`
	QualityAdvantage  string             `json:"quality_advantage"` // Which provider scored higher
	SpeedAdvantage    string             `json:"speed_advantage"`
	CostEfficiency    map[string]float64 `json:"cost_efficiency"`     // Score per credit
	QualityPerLatency map[string]float64 `json:"quality_per_latency"` // Score per ms
}

// ScoreWeights configures how quality scores are calculated
type ScoreWeights struct {
	SemanticWeight  float64
	RerankerWeight  float64
	AuthorityWeight float64
	DiversityWeight float64
	FreshnessWeight float64
}

// DefaultScoreWeights returns default weights for scoring
func DefaultScoreWeights() ScoreWeights {
	return ScoreWeights{
		SemanticWeight:  0.35,
		RerankerWeight:  0.25,
		AuthorityWeight: 0.20,
		DiversityWeight: 0.10,
		FreshnessWeight: 0.10,
	}
}

// CalculateSearchScore computes weighted overall score
func CalculateSearchScore(scores SearchQualityScore, weights ScoreWeights) float64 {
	overall := scores.SemanticRelevance*weights.SemanticWeight +
		scores.RerankerScore*weights.RerankerWeight +
		scores.AuthorityScore*weights.AuthorityWeight +
		scores.ResultDiversity*weights.DiversityWeight +
		scores.FreshnessScore*weights.FreshnessWeight

	// Normalize if weights don't sum to 1
	totalWeight := weights.SemanticWeight + weights.RerankerWeight +
		weights.AuthorityWeight + weights.DiversityWeight + weights.FreshnessWeight

	if totalWeight > 0 {
		overall /= totalWeight
	}

	return clamp(overall, 0, 100)
}

// CalculateExtractScore computes weighted overall score for extraction
func CalculateExtractScore(scores ExtractQualityScore) float64 {
	weights := []float64{0.25, 0.20, 0.20, 0.15, 0.10, 0.10}
	values := []float64{
		scores.ContentCompleteness,
		scores.StructurePreservation,
		scores.MarkdownQuality,
		scores.FreshnessScore,
		scores.SignalToNoise,
		scores.CodePreservation,
	}

	var total, weightSum float64
	for i := range weights {
		total += values[i] * weights[i]
		weightSum += weights[i]
	}

	if weightSum > 0 {
		total /= weightSum
	}

	return clamp(total, 0, 100)
}

// CalculateCrawlScore computes weighted overall score for crawling
func CalculateCrawlScore(scores CrawlQualityScore) float64 {
	weights := []float64{0.35, 0.25, 0.20, 0.10, 0.10}
	values := []float64{
		scores.CoverageScore,
		scores.DepthAccuracy,
		scores.LinkDiscovery,
		scores.ContentConsistency,
		100 - scores.DuplicateRatio, // Lower duplicate ratio is better
	}

	var total, weightSum float64
	for i := range weights {
		total += values[i] * weights[i]
		weightSum += weights[i]
	}

	if weightSum > 0 {
		total /= weightSum
	}

	return clamp(total, 0, 100)
}

func clamp(v, minVal, maxVal float64) float64 { //nolint:unparam // maxVal is always 100 but kept for clarity
	if v < minVal {
		return minVal
	}
	if v > maxVal {
		return maxVal
	}
	return v
}
