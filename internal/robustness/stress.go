package robustness

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// StressTestResult contains stress test metrics
type StressTestResult struct {
	TotalRequests      int            `json:"total_requests"`
	SuccessfulRequests int            `json:"successful_requests"`
	FailedRequests     int            `json:"failed_requests"`
	TotalDuration      time.Duration  `json:"total_duration"`
	AvgLatency         time.Duration  `json:"avg_latency"`
	MinLatency         time.Duration  `json:"min_latency"`
	MaxLatency         time.Duration  `json:"max_latency"`
	P50Latency         time.Duration  `json:"p50_latency"`
	P95Latency         time.Duration  `json:"p95_latency"`
	P99Latency         time.Duration  `json:"p99_latency"`
	ThroughputRPS      float64        `json:"throughput_rps"` // Requests per second
	ErrorBreakdown     map[string]int `json:"error_breakdown"`
}

// StressTestRunner executes stress tests
type StressTestRunner struct {
	concurrency int
	duration    time.Duration
}

// NewStressTestRunner creates a new stress test runner
func NewStressTestRunner(concurrency int, duration time.Duration) *StressTestRunner {
	if concurrency <= 0 {
		concurrency = 10
	}
	if duration <= 0 {
		duration = 30 * time.Second
	}

	return &StressTestRunner{
		concurrency: concurrency,
		duration:    duration,
	}
}

// RequestFunc is the function signature for requests to test
type RequestFunc func(ctx context.Context) error

// Run executes a stress test
func (r *StressTestRunner) Run(ctx context.Context, requestFn RequestFunc) (*StressTestResult, error) {
	start := time.Now()

	var (
		mu             sync.Mutex
		totalRequests  int
		successCount   int
		failCount      int
		latencies      []time.Duration
		errorBreakdown = make(map[string]int)
	)

	// Create work channel
	workCh := make(chan struct{}, r.concurrency*2)
	doneCh := make(chan struct{})

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < r.concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range workCh {
				reqStart := time.Now()
				err := requestFn(ctx)
				latency := time.Since(reqStart)

				mu.Lock()
				totalRequests++
				latencies = append(latencies, latency)
				if err != nil {
					failCount++
					errorBreakdown[err.Error()]++
				} else {
					successCount++
				}
				mu.Unlock()
			}
		}()
	}

	// Generate work until duration expires
	go func() {
		ticker := time.NewTicker(time.Second / time.Duration(r.concurrency))
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				close(workCh)
				return
			case <-doneCh:
				close(workCh)
				return
			case workCh <- struct{}{}:
			}
		}
	}()

	// Wait for duration
	time.Sleep(r.duration)
	close(doneCh)

	// Wait for workers
	wg.Wait()

	totalDuration := time.Since(start)

	// Calculate statistics
	result := &StressTestResult{
		TotalRequests:      totalRequests,
		SuccessfulRequests: successCount,
		FailedRequests:     failCount,
		TotalDuration:      totalDuration,
		ErrorBreakdown:     errorBreakdown,
	}

	if len(latencies) > 0 {
		// Sort latencies for percentile calculation
		sorted := make([]time.Duration, len(latencies))
		copy(sorted, latencies)
		sortDurations(sorted)

		result.MinLatency = sorted[0]
		result.MaxLatency = sorted[len(sorted)-1]
		result.P50Latency = calculatePercentile(sorted, 0.50)
		result.P95Latency = calculatePercentile(sorted, 0.95)
		result.P99Latency = calculatePercentile(sorted, 0.99)

		// Calculate average
		var totalLatency time.Duration
		for _, l := range latencies {
			totalLatency += l
		}
		result.AvgLatency = totalLatency / time.Duration(len(latencies))

		// Calculate throughput
		result.ThroughputRPS = float64(totalRequests) / totalDuration.Seconds()
	}

	return result, nil
}

// RunBurst executes a burst stress test (all at once)
func (r *StressTestRunner) RunBurst(ctx context.Context, requestFn RequestFunc, burstSize int) (*StressTestResult, error) {
	start := time.Now()

	var (
		mu             sync.Mutex
		successCount   int
		failCount      int
		latencies      []time.Duration
		errorBreakdown = make(map[string]int)
	)

	var wg sync.WaitGroup

	for i := 0; i < burstSize; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			reqStart := time.Now()
			err := requestFn(ctx)
			latency := time.Since(reqStart)

			mu.Lock()
			latencies = append(latencies, latency)
			if err != nil {
				failCount++
				errorBreakdown[err.Error()]++
			} else {
				successCount++
			}
			mu.Unlock()
		}()
	}

	wg.Wait()

	totalDuration := time.Since(start)

	result := &StressTestResult{
		TotalRequests:      burstSize,
		SuccessfulRequests: successCount,
		FailedRequests:     failCount,
		TotalDuration:      totalDuration,
		ErrorBreakdown:     errorBreakdown,
	}

	if len(latencies) > 0 {
		sorted := make([]time.Duration, len(latencies))
		copy(sorted, latencies)
		sortDurations(sorted)

		result.MinLatency = sorted[0]
		result.MaxLatency = sorted[len(sorted)-1]
		result.P50Latency = calculatePercentile(sorted, 0.50)
		result.P95Latency = calculatePercentile(sorted, 0.95)
		result.P99Latency = calculatePercentile(sorted, 0.99)

		var totalLatency time.Duration
		for _, l := range latencies {
			totalLatency += l
		}
		result.AvgLatency = totalLatency / time.Duration(len(latencies))

		result.ThroughputRPS = float64(burstSize) / totalDuration.Seconds()
	}

	return result, nil
}

// RunSequential executes sequential rapid requests
func (r *StressTestRunner) RunSequential(ctx context.Context, requestFn RequestFunc, requestCount int) (*StressTestResult, error) {
	start := time.Now()

	var (
		successCount   int
		failCount      int
		latencies      []time.Duration
		errorBreakdown = make(map[string]int)
	)

	for i := 0; i < requestCount; i++ {
		reqStart := time.Now()
		err := requestFn(ctx)
		latency := time.Since(reqStart)

		latencies = append(latencies, latency)
		if err != nil {
			failCount++
			errorBreakdown[err.Error()]++
		} else {
			successCount++
		}

		// Small delay between requests to avoid hammering
		time.Sleep(10 * time.Millisecond)
	}

	totalDuration := time.Since(start)

	result := &StressTestResult{
		TotalRequests:      requestCount,
		SuccessfulRequests: successCount,
		FailedRequests:     failCount,
		TotalDuration:      totalDuration,
		ErrorBreakdown:     errorBreakdown,
	}

	if len(latencies) > 0 {
		sorted := make([]time.Duration, len(latencies))
		copy(sorted, latencies)
		sortDurations(sorted)

		result.MinLatency = sorted[0]
		result.MaxLatency = sorted[len(sorted)-1]
		result.P50Latency = calculatePercentile(sorted, 0.50)
		result.P95Latency = calculatePercentile(sorted, 0.95)
		result.P99Latency = calculatePercentile(sorted, 0.99)

		var totalLatency time.Duration
		for _, l := range latencies {
			totalLatency += l
		}
		result.AvgLatency = totalLatency / time.Duration(len(latencies))

		result.ThroughputRPS = float64(requestCount) / totalDuration.Seconds()
	}

	return result, nil
}

// Helper functions

func sortDurations(durations []time.Duration) {
	// Simple bubble sort for small arrays
	n := len(durations)
	for i := 0; i < n; i++ {
		for j := 0; j < n-i-1; j++ {
			if durations[j] > durations[j+1] {
				durations[j], durations[j+1] = durations[j+1], durations[j]
			}
		}
	}
}

func calculatePercentile(sorted []time.Duration, percentile float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	if len(sorted) == 1 {
		return sorted[0]
	}

	index := int(float64(len(sorted)-1) * percentile)
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}

	return sorted[index]
}

// FormatResult returns a formatted string representation of the result
func (r *StressTestResult) FormatResult() string {
	return fmt.Sprintf(
		"Stress Test Results:\n"+
			"  Total Requests: %d\n"+
			"  Successful: %d (%.1f%%)\n"+
			"  Failed: %d (%.1f%%)\n"+
			"  Total Duration: %v\n"+
			"  Throughput: %.1f RPS\n"+
			"  Latency (avg/min/max): %v / %v / %v\n"+
			"  Latency (p50/p95/p99): %v / %v / %v",
		r.TotalRequests,
		r.SuccessfulRequests, float64(r.SuccessfulRequests)/float64(r.TotalRequests)*100,
		r.FailedRequests, float64(r.FailedRequests)/float64(r.TotalRequests)*100,
		r.TotalDuration.Round(time.Millisecond),
		r.ThroughputRPS,
		r.AvgLatency.Round(time.Millisecond),
		r.MinLatency.Round(time.Millisecond),
		r.MaxLatency.Round(time.Millisecond),
		r.P50Latency.Round(time.Millisecond),
		r.P95Latency.Round(time.Millisecond),
		r.P99Latency.Round(time.Millisecond),
	)
}
