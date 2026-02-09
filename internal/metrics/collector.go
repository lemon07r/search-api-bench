// Package metrics provides collection and aggregation of benchmark test results.
package metrics

import (
	"sync"
	"time"
)

// Result represents a single test result
type Result struct {
	TestName      string
	Provider      string
	TestType      string
	Success       bool
	Error         string
	Latency       time.Duration
	CreditsUsed   int
	ContentLength int
	ResultsCount  int
	Timestamp     time.Time
}

// Summary contains aggregated metrics for a provider
type Summary struct {
	Provider           string
	TotalTests         int
	SuccessfulTests    int
	FailedTests        int
	TotalLatency       time.Duration
	AvgLatency         time.Duration
	MinLatency         time.Duration
	MaxLatency         time.Duration
	TotalCreditsUsed   int
	AvgCreditsPerReq   float64
	TotalContentLength int
	AvgContentLength   float64
	ResultsPerQuery    float64
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
		Provider:   provider,
		TotalTests: len(results),
		MinLatency: results[0].Latency,
		MaxLatency: results[0].Latency,
	}

	var totalLatency time.Duration
	var totalCredits int
	var totalContentLength int
	var totalResultsCount int

	for _, r := range results {
		if r.Success {
			summary.SuccessfulTests++
		} else {
			summary.FailedTests++
		}

		totalLatency += r.Latency
		totalCredits += r.CreditsUsed
		totalContentLength += r.ContentLength
		totalResultsCount += r.ResultsCount

		if r.Latency < summary.MinLatency {
			summary.MinLatency = r.Latency
		}
		if r.Latency > summary.MaxLatency {
			summary.MaxLatency = r.Latency
		}
	}

	summary.TotalLatency = totalLatency
	summary.AvgLatency = totalLatency / time.Duration(len(results))
	summary.TotalCreditsUsed = totalCredits
	summary.AvgCreditsPerReq = float64(totalCredits) / float64(len(results))
	summary.TotalContentLength = totalContentLength
	summary.AvgContentLength = float64(totalContentLength) / float64(len(results))

	if summary.TotalTests > 0 {
		summary.ResultsPerQuery = float64(totalResultsCount) / float64(summary.TotalTests)
	}

	return summary
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
