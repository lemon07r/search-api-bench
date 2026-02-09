// Package metrics provides collection and aggregation of benchmark test results.
package metrics

import (
	"sort"
	"sync"
	"time"
)

// Result represents a single test result
type Result struct {
	TestName      string        `json:"test_name"`
	Provider      string        `json:"provider"`
	TestType      string        `json:"test_type"`
	Success       bool          `json:"success"`
	Skipped       bool          `json:"skipped,omitempty"`
	SkipReason    string        `json:"skip_reason,omitempty"`
	Error         string        `json:"error,omitempty"`
	ErrorCategory string        `json:"error_category,omitempty"`
	Latency       time.Duration `json:"latency"`
	CreditsUsed   int           `json:"credits_used"`
	ContentLength int           `json:"content_length"`
	ResultsCount  int           `json:"results_count"`
	Timestamp     time.Time     `json:"timestamp"`

	// Cost in USD (calculated from provider-specific pricing)
	CostUSD float64 `json:"cost_usd"`

	// Quality scores (0-100)
	QualityScore      float64                `json:"quality_score,omitempty"`
	SemanticScore     float64                `json:"semantic_score,omitempty"`
	RerankerScore     float64                `json:"reranker_score,omitempty"`
	DomainScores      map[string]float64     `json:"domain_scores,omitempty"`
	RawQualityMetrics map[string]interface{} `json:"raw_quality_metrics,omitempty"`
}

// Summary contains aggregated metrics for a provider
type Summary struct {
	Provider           string        `json:"provider"`
	TotalTests         int           `json:"total_tests"`
	ExecutedTests      int           `json:"executed_tests"`
	SuccessfulTests    int           `json:"successful_tests"`
	FailedTests        int           `json:"failed_tests"`
	SkippedTests       int           `json:"skipped_tests"`
	SuccessRate        float64       `json:"success_rate"`
	TotalLatency       time.Duration `json:"total_latency"`
	AvgLatency         time.Duration `json:"avg_latency"`
	MinLatency         time.Duration `json:"min_latency"`
	MaxLatency         time.Duration `json:"max_latency"`
	P50Latency         time.Duration `json:"p50_latency"`
	P95Latency         time.Duration `json:"p95_latency"`
	P99Latency         time.Duration `json:"p99_latency"`
	TotalCreditsUsed   int           `json:"total_credits_used"`
	AvgCreditsPerReq   float64       `json:"avg_credits_per_req"`
	TotalContentLength int           `json:"total_content_length"`
	AvgContentLength   float64       `json:"avg_content_length"`
	ResultsPerQuery    float64       `json:"results_per_query"`

	// Cost metrics in USD
	TotalCostUSD  float64 `json:"total_cost_usd"`   // total USD cost
	AvgCostPerReq float64 `json:"avg_cost_per_req"` // average USD cost per request
	CostPerResult float64 `json:"cost_per_result"`  // USD cost per successful result

	// Efficiency metrics (kept for backward compatibility, but prefer CostUSD versions)
	CreditsPerResult float64 `json:"credits_per_result"` // credits per successful result
	CharsPerCredit   float64 `json:"chars_per_credit"`   // content chars per credit
	ResultsPerCredit float64 `json:"results_per_credit"` // results per credit spent

	// Quality metrics
	AvgQualityScore  float64        `json:"avg_quality_score"`
	MinQualityScore  float64        `json:"min_quality_score"`
	MaxQualityScore  float64        `json:"max_quality_score"`
	QualityScoreDist map[string]int `json:"quality_score_dist"` // distribution buckets

	// Error breakdown
	ErrorBreakdown map[string]int `json:"error_breakdown,omitempty"`
}

// Collector handles collection and aggregation of test results
type Collector struct {
	results []Result
	mu      sync.RWMutex
}

// NewCollector creates a new metrics collector
func NewCollector() *Collector {
	return &Collector{
		results: make([]Result, 0),
	}
}

// AddResult adds a test result to the collector
func (c *Collector) AddResult(r Result) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.results = append(c.results, r)
}

// GetResults returns all collected results
func (c *Collector) GetResults() []Result {
	c.mu.RLock()
	defer c.mu.RUnlock()
	results := make([]Result, len(c.results))
	copy(results, c.results)
	return results
}

// GetResultsByProvider returns results filtered by provider
func (c *Collector) GetResultsByProvider(provider string) []Result {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var filtered []Result
	for _, r := range c.results {
		if r.Provider == provider {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

// GetResultsByTest returns results filtered by test name
func (c *Collector) GetResultsByTest(testName string) []Result {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var filtered []Result
	for _, r := range c.results {
		if r.TestName == testName {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

// ComputeSummary computes summary metrics for a provider
func (c *Collector) ComputeSummary(provider string) *Summary {
	results := c.GetResultsByProvider(provider)

	if len(results) == 0 {
		return &Summary{Provider: provider}
	}

	summary := &Summary{
		Provider:         provider,
		TotalTests:       len(results),
		QualityScoreDist: make(map[string]int),
		ErrorBreakdown:   make(map[string]int),
	}

	// Initialize cost calculator for USD conversions
	costCalc := NewCostCalculator()

	var totalLatency time.Duration
	var totalCredits int
	var totalContentLength int
	var totalResultsCount int
	var totalQualityScore float64
	var qualityScoreCount int
	var totalCostUSD float64

	latencies := make([]time.Duration, 0, len(results))
	executedCount := 0

	for _, r := range results {
		// Handle skipped tests separately
		if r.Skipped {
			summary.SkippedTests++
			continue // Don't count skipped tests in success/failure metrics
		}
		executedCount++

		if r.Success {
			summary.SuccessfulTests++
		} else {
			summary.FailedTests++
			// Track error breakdown
			if r.ErrorCategory != "" {
				summary.ErrorBreakdown[r.ErrorCategory]++
			} else if r.Error != "" {
				summary.ErrorBreakdown["unknown"]++
			}
		}

		totalLatency += r.Latency
		latencies = append(latencies, r.Latency)
		totalCredits += r.CreditsUsed
		totalContentLength += r.ContentLength
		totalResultsCount += r.ResultsCount

		// Calculate USD cost for this result
		costUSD := costCalc.CalculateProviderCost(provider, r.CreditsUsed, r.TestType)
		totalCostUSD += costUSD

		if executedCount == 1 {
			summary.MinLatency = r.Latency
			summary.MaxLatency = r.Latency
		} else {
			if r.Latency < summary.MinLatency {
				summary.MinLatency = r.Latency
			}
			if r.Latency > summary.MaxLatency {
				summary.MaxLatency = r.Latency
			}
		}

		// Track quality scores
		if r.QualityScore > 0 {
			totalQualityScore += r.QualityScore
			qualityScoreCount++

			if summary.MinQualityScore == 0 || r.QualityScore < summary.MinQualityScore {
				summary.MinQualityScore = r.QualityScore
			}
			if r.QualityScore > summary.MaxQualityScore {
				summary.MaxQualityScore = r.QualityScore
			}

			// Distribution bucket
			bucket := getQualityBucket(r.QualityScore)
			summary.QualityScoreDist[bucket]++
		}
	}

	summary.TotalLatency = totalLatency
	summary.TotalCreditsUsed = totalCredits
	summary.TotalContentLength = totalContentLength
	summary.ExecutedTests = executedCount
	if executedCount > 0 {
		summary.AvgLatency = totalLatency / time.Duration(executedCount)
		summary.AvgCreditsPerReq = float64(totalCredits) / float64(executedCount)
		summary.AvgContentLength = float64(totalContentLength) / float64(executedCount)
	}

	// Calculate USD cost metrics
	summary.TotalCostUSD = totalCostUSD
	if executedCount > 0 {
		summary.AvgCostPerReq = totalCostUSD / float64(executedCount)
	}
	if summary.SuccessfulTests > 0 && totalResultsCount > 0 {
		summary.CostPerResult = totalCostUSD / float64(totalResultsCount)
	}

	if executedCount > 0 {
		summary.SuccessRate = float64(summary.SuccessfulTests) / float64(executedCount) * 100
		summary.ResultsPerQuery = float64(totalResultsCount) / float64(executedCount)
	}

	// Calculate efficiency metrics (kept for backward compatibility)
	if summary.SuccessfulTests > 0 && totalCredits > 0 && totalResultsCount > 0 {
		summary.CreditsPerResult = float64(totalCredits) / float64(totalResultsCount)
		summary.ResultsPerCredit = float64(totalResultsCount) / float64(totalCredits)
	}
	if totalCredits > 0 {
		summary.CharsPerCredit = float64(totalContentLength) / float64(totalCredits)
	}

	// Calculate latency percentiles
	if len(latencies) > 0 {
		summary.P50Latency = calculatePercentileDuration(latencies, 0.50)
		summary.P95Latency = calculatePercentileDuration(latencies, 0.95)
		summary.P99Latency = calculatePercentileDuration(latencies, 0.99)
	}

	// Calculate average quality score
	if qualityScoreCount > 0 {
		summary.AvgQualityScore = totalQualityScore / float64(qualityScoreCount)
	}

	return summary
}

// getQualityBucket returns a bucket label for a quality score
func getQualityBucket(score float64) string {
	switch {
	case score >= 90:
		return "excellent (90-100)"
	case score >= 75:
		return "good (75-89)"
	case score >= 60:
		return "acceptable (60-74)"
	case score >= 40:
		return "poor (40-59)"
	default:
		return "failed (0-39)"
	}
}

// calculatePercentileDuration calculates the percentile of a duration slice
func calculatePercentileDuration(durations []time.Duration, percentile float64) time.Duration {
	if len(durations) == 0 {
		return 0
	}

	// Use efficient sort (O(n log n)) instead of bubble sort (O(n²))
	sorted := make([]time.Duration, len(durations))
	copy(sorted, durations)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i] < sorted[j]
	})

	index := int(float64(len(sorted)-1) * percentile)
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

// GetAllProviders returns a list of all unique provider names
func (c *Collector) GetAllProviders() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	providerMap := make(map[string]bool)
	for _, r := range c.results {
		providerMap[r.Provider] = true
	}

	providers := make([]string, 0, len(providerMap))
	for p := range providerMap {
		providers = append(providers, p)
	}
	return providers
}

// GetAllTests returns a list of all unique test names
func (c *Collector) GetAllTests() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	testMap := make(map[string]bool)
	for _, r := range c.results {
		testMap[r.TestName] = true
	}

	tests := make([]string, 0, len(testMap))
	for t := range testMap {
		tests = append(tests, t)
	}
	return tests
}
