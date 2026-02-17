package evaluator

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lamim/search-api-bench/internal/config"
	"github.com/lamim/search-api-bench/internal/providers"
)

// mockProvider implements providers.Provider for testing
type mockProvider struct {
	name         string
	capabilities providers.CapabilitySet
	searchFn     func(ctx context.Context, query string, opts providers.SearchOptions) (*providers.SearchResult, error)
	extractFn    func(ctx context.Context, url string, opts providers.ExtractOptions) (*providers.ExtractResult, error)
	crawlFn      func(ctx context.Context, url string, opts providers.CrawlOptions) (*providers.CrawlResult, error)
	searchCalls  int32
	extractCalls int32
	crawlCalls   int32
}

func intPtr(v int) *int {
	return &v
}

func (m *mockProvider) Name() string {
	return m.name
}

func (m *mockProvider) Capabilities() providers.CapabilitySet {
	if m.capabilities.Search != "" || m.capabilities.Extract != "" || m.capabilities.Crawl != "" {
		return m.capabilities
	}
	return providers.CapabilitySet{
		Search:  providers.SupportNative,
		Extract: providers.SupportNative,
		Crawl:   providers.SupportNative,
	}
}

func (m *mockProvider) Search(ctx context.Context, query string, opts providers.SearchOptions) (*providers.SearchResult, error) {
	atomic.AddInt32(&m.searchCalls, 1)
	if m.searchFn != nil {
		return m.searchFn(ctx, query, opts)
	}
	return &providers.SearchResult{
		Query:        query,
		Results:      []providers.SearchItem{{Title: "Result", URL: "https://example.com", Content: "Content"}},
		TotalResults: 1,
		Latency:      100 * time.Millisecond,
		CreditsUsed:  1,
	}, nil
}

func (m *mockProvider) Extract(ctx context.Context, url string, opts providers.ExtractOptions) (*providers.ExtractResult, error) {
	atomic.AddInt32(&m.extractCalls, 1)
	if m.extractFn != nil {
		return m.extractFn(ctx, url, opts)
	}
	return &providers.ExtractResult{
		URL:         url,
		Title:       "Title",
		Content:     "Content",
		Markdown:    "Content",
		Latency:     100 * time.Millisecond,
		CreditsUsed: 1,
	}, nil
}

func (m *mockProvider) Crawl(ctx context.Context, url string, opts providers.CrawlOptions) (*providers.CrawlResult, error) {
	atomic.AddInt32(&m.crawlCalls, 1)
	if m.crawlFn != nil {
		return m.crawlFn(ctx, url, opts)
	}
	return &providers.CrawlResult{
		URL:         url,
		Pages:       []providers.CrawledPage{{URL: url, Title: "Title", Content: "Content"}},
		TotalPages:  1,
		Latency:     100 * time.Millisecond,
		CreditsUsed: 1,
	}, nil
}

func (m *mockProvider) SupportsOperation(opType string) bool {
	return m.Capabilities().SupportsOperation(opType)
}

func TestRun_SingleProviderSingleTest(t *testing.T) {
	cfg := &config.Config{
		General: config.GeneralConfig{
			Concurrency: 1,
			Timeout:     "30s",
			OutputDir:   t.TempDir(),
		},
		Tests: []config.TestConfig{
			{Name: "test1", Type: "search", Query: "query1"},
		},
	}

	mock := &mockProvider{name: "mock"}
	runner := NewRunner(cfg, []providers.Provider{mock}, nil, nil, nil)

	err := runner.Run(context.Background())
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	results := runner.GetCollector().GetResults()
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}

	if atomic.LoadInt32(&mock.searchCalls) != 1 {
		t.Errorf("expected 1 search call, got %d", mock.searchCalls)
	}
}

func TestRun_MultipleProviders(t *testing.T) {
	cfg := &config.Config{
		General: config.GeneralConfig{
			Concurrency: 2,
			Timeout:     "30s",
			OutputDir:   t.TempDir(),
		},
		Tests: []config.TestConfig{
			{Name: "test1", Type: "search", Query: "query1"},
		},
	}

	mock1 := &mockProvider{name: "provider1"}
	mock2 := &mockProvider{name: "provider2"}
	runner := NewRunner(cfg, []providers.Provider{mock1, mock2}, nil, nil, nil)

	err := runner.Run(context.Background())
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	results := runner.GetCollector().GetResults()
	if len(results) != 2 {
		t.Errorf("expected 2 results (1 test x 2 providers), got %d", len(results))
	}

	if atomic.LoadInt32(&mock1.searchCalls) != 1 {
		t.Errorf("expected provider1 to be called once, got %d", mock1.searchCalls)
	}
	if atomic.LoadInt32(&mock2.searchCalls) != 1 {
		t.Errorf("expected provider2 to be called once, got %d", mock2.searchCalls)
	}
}

func TestRun_MultipleTests(t *testing.T) {
	cfg := &config.Config{
		General: config.GeneralConfig{
			Concurrency: 2,
			Timeout:     "30s",
			OutputDir:   t.TempDir(),
		},
		Tests: []config.TestConfig{
			{Name: "test1", Type: "search", Query: "query1"},
			{Name: "test2", Type: "search", Query: "query2"},
			{Name: "test3", Type: "search", Query: "query3"},
		},
	}

	mock := &mockProvider{name: "mock"}
	runner := NewRunner(cfg, []providers.Provider{mock}, nil, nil, nil)

	err := runner.Run(context.Background())
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	results := runner.GetCollector().GetResults()
	if len(results) != 3 {
		t.Errorf("expected 3 results (3 tests x 1 provider), got %d", len(results))
	}

	if atomic.LoadInt32(&mock.searchCalls) != 3 {
		t.Errorf("expected 3 search calls, got %d", mock.searchCalls)
	}
}

func TestRun_MatrixExecution(t *testing.T) {
	cfg := &config.Config{
		General: config.GeneralConfig{
			Concurrency: 4,
			Timeout:     "30s",
			OutputDir:   t.TempDir(),
		},
		Tests: []config.TestConfig{
			{Name: "test1", Type: "search", Query: "query1"},
			{Name: "test2", Type: "search", Query: "query2"},
		},
	}

	mock1 := &mockProvider{name: "provider1"}
	mock2 := &mockProvider{name: "provider2"}
	runner := NewRunner(cfg, []providers.Provider{mock1, mock2}, nil, nil, nil)

	err := runner.Run(context.Background())
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	results := runner.GetCollector().GetResults()
	if len(results) != 4 {
		t.Errorf("expected 4 results (2 tests x 2 providers), got %d", len(results))
	}
}

func TestRun_RespectsConcurrencyLimit(t *testing.T) {
	concurrency := 2
	cfg := &config.Config{
		General: config.GeneralConfig{
			Concurrency: concurrency,
			Timeout:     "30s",
			OutputDir:   t.TempDir(),
		},
		Tests: []config.TestConfig{
			{Name: "test1", Type: "search", Query: "q1"},
			{Name: "test2", Type: "search", Query: "q2"},
			{Name: "test3", Type: "search", Query: "q3"},
			{Name: "test4", Type: "search", Query: "q4"},
		},
	}

	var maxConcurrent int32
	var currentConcurrent int32

	mock := &mockProvider{
		name: "mock",
		searchFn: func(_ context.Context, query string, opts providers.SearchOptions) (*providers.SearchResult, error) {
			current := atomic.AddInt32(&currentConcurrent, 1)
			for {
				currentMax := atomic.LoadInt32(&maxConcurrent)
				if current > currentMax {
					if atomic.CompareAndSwapInt32(&maxConcurrent, currentMax, current) {
						break
					}
				} else {
					break
				}
			}
			time.Sleep(50 * time.Millisecond) // Simulate work
			atomic.AddInt32(&currentConcurrent, -1)
			return &providers.SearchResult{
				Query:       query,
				Latency:     50 * time.Millisecond,
				CreditsUsed: 1,
			}, nil
		},
	}

	runner := NewRunner(cfg, []providers.Provider{mock}, nil, nil, nil)
	runner.Run(context.Background())

	if maxConcurrent > int32(concurrency) {
		t.Errorf("max concurrent (%d) exceeded limit (%d)", maxConcurrent, concurrency)
	}
}

func TestRun_SearchTest(t *testing.T) {
	cfg := &config.Config{
		General: config.GeneralConfig{
			Concurrency: 1,
			Timeout:     "30s",
			OutputDir:   t.TempDir(),
		},
		Tests: []config.TestConfig{
			{Name: "search-test", Type: "search", Query: "test query", ExpectedTopics: []string{"topic1"}},
		},
	}

	mock := &mockProvider{
		name: "mock",
		searchFn: func(ctx context.Context, query string, opts providers.SearchOptions) (*providers.SearchResult, error) {
			return &providers.SearchResult{
				Query:        query,
				Results:      []providers.SearchItem{{Title: "topic1 result", Content: "content"}},
				TotalResults: 1,
				Latency:      100 * time.Millisecond,
				CreditsUsed:  1,
			}, nil
		},
	}

	runner := NewRunner(cfg, []providers.Provider{mock}, nil, nil, nil)
	runner.Run(context.Background())

	results := runner.GetCollector().GetResults()
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if results[0].TestType != "search" {
		t.Errorf("expected test type 'search', got %s", results[0].TestType)
	}
	if results[0].ResultsCount != 1 {
		t.Errorf("expected results count 1, got %d", results[0].ResultsCount)
	}
	if !results[0].Success {
		t.Error("expected success to be true")
	}
}

func TestRun_ExtractTest(t *testing.T) {
	cfg := &config.Config{
		General: config.GeneralConfig{
			Concurrency: 1,
			Timeout:     "30s",
			OutputDir:   t.TempDir(),
		},
		Tests: []config.TestConfig{
			{Name: "extract-test", Type: "extract", URL: "https://example.com", ExpectedContent: []string{"content"}},
		},
	}

	mock := &mockProvider{
		name: "mock",
		extractFn: func(ctx context.Context, url string, opts providers.ExtractOptions) (*providers.ExtractResult, error) {
			return &providers.ExtractResult{
				URL:         url,
				Content:     "some content here",
				Latency:     100 * time.Millisecond,
				CreditsUsed: 1,
			}, nil
		},
	}

	runner := NewRunner(cfg, []providers.Provider{mock}, nil, nil, nil)
	runner.Run(context.Background())

	results := runner.GetCollector().GetResults()
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if results[0].TestType != "extract" {
		t.Errorf("expected test type 'extract', got %s", results[0].TestType)
	}
	if results[0].ContentLength != len("some content here") {
		t.Errorf("expected content length %d, got %d", len("some content here"), results[0].ContentLength)
	}
}

func TestRun_CrawlTest(t *testing.T) {
	cfg := &config.Config{
		General: config.GeneralConfig{
			Concurrency: 1,
			Timeout:     "30s",
			OutputDir:   t.TempDir(),
		},
		Tests: []config.TestConfig{
			{Name: "crawl-test", Type: "crawl", URL: "https://example.com", MaxPages: intPtr(2)},
		},
	}

	mock := &mockProvider{
		name: "mock",
		crawlFn: func(ctx context.Context, url string, opts providers.CrawlOptions) (*providers.CrawlResult, error) {
			return &providers.CrawlResult{
				URL:         url,
				Pages:       []providers.CrawledPage{{URL: url + "/1"}, {URL: url + "/2"}},
				TotalPages:  2,
				Latency:     200 * time.Millisecond,
				CreditsUsed: 2,
			}, nil
		},
	}

	runner := NewRunner(cfg, []providers.Provider{mock}, nil, nil, nil)
	runner.Run(context.Background())

	results := runner.GetCollector().GetResults()
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if results[0].TestType != "crawl" {
		t.Errorf("expected test type 'crawl', got %s", results[0].TestType)
	}
	if results[0].ResultsCount != 2 {
		t.Errorf("expected results count 2 (pages), got %d", results[0].ResultsCount)
	}
}

func TestRun_CrawlHonorsExplicitZeroDepth(t *testing.T) {
	cfg := &config.Config{
		General: config.GeneralConfig{
			Concurrency: 1,
			Timeout:     "30s",
			OutputDir:   t.TempDir(),
		},
		Tests: []config.TestConfig{
			{
				Name:     "crawl-zero-depth",
				Type:     "crawl",
				URL:      "https://example.com",
				MaxPages: intPtr(1),
				MaxDepth: intPtr(0),
			},
		},
	}

	var seenDepth int
	mock := &mockProvider{
		name: "mock",
		crawlFn: func(_ context.Context, url string, opts providers.CrawlOptions) (*providers.CrawlResult, error) {
			seenDepth = opts.MaxDepth
			return &providers.CrawlResult{
				URL:         url,
				Pages:       []providers.CrawledPage{{URL: url}},
				TotalPages:  1,
				Latency:     10 * time.Millisecond,
				CreditsUsed: 1,
			}, nil
		},
	}

	runner := NewRunner(cfg, []providers.Provider{mock}, nil, nil, nil)
	if err := runner.Run(context.Background()); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if seenDepth != 0 {
		t.Fatalf("expected explicit MaxDepth=0 to be passed through, got %d", seenDepth)
	}
}

func TestRun_FailureRecordsLatency(t *testing.T) {
	cfg := &config.Config{
		General: config.GeneralConfig{
			Concurrency: 1,
			Timeout:     "30s",
			OutputDir:   t.TempDir(),
		},
		Tests: []config.TestConfig{
			{Name: "search-failure-latency", Type: "search", Query: "query"},
		},
	}

	mock := &mockProvider{
		name: "mock",
		searchFn: func(_ context.Context, _ string, _ providers.SearchOptions) (*providers.SearchResult, error) {
			time.Sleep(15 * time.Millisecond)
			return nil, errors.New("boom")
		},
	}

	runner := NewRunner(cfg, []providers.Provider{mock}, nil, nil, nil)
	if err := runner.Run(context.Background()); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	results := runner.GetCollector().GetResults()
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Success {
		t.Fatal("expected failed result")
	}
	if results[0].Latency <= 0 {
		t.Fatalf("expected non-zero failure latency, got %v", results[0].Latency)
	}
}

func TestRun_TestTimeout(t *testing.T) {
	cfg := &config.Config{
		General: config.GeneralConfig{
			Concurrency: 1,
			Timeout:     "100ms",
			OutputDir:   t.TempDir(),
		},
		Tests: []config.TestConfig{
			{Name: "slow-test", Type: "search", Query: "query"},
		},
	}

	mock := &mockProvider{
		name: "mock",
		searchFn: func(ctx context.Context, query string, opts providers.SearchOptions) (*providers.SearchResult, error) {
			select {
			case <-time.After(500 * time.Millisecond):
				return &providers.SearchResult{}, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		},
	}

	runner := NewRunner(cfg, []providers.Provider{mock}, nil, nil, nil)
	runner.Run(context.Background())

	results := runner.GetCollector().GetResults()
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if results[0].Success {
		t.Error("expected success to be false due to timeout")
	}
	if results[0].Error == "" {
		t.Error("expected error message for timeout")
	}
}

func TestRun_ProviderError(t *testing.T) {
	cfg := &config.Config{
		General: config.GeneralConfig{
			Concurrency: 1,
			Timeout:     "30s",
			OutputDir:   t.TempDir(),
		},
		Tests: []config.TestConfig{
			{Name: "error-test", Type: "search", Query: "query"},
		},
	}

	mock := &mockProvider{
		name: "mock",
		searchFn: func(ctx context.Context, query string, opts providers.SearchOptions) (*providers.SearchResult, error) {
			return nil, errors.New("api error")
		},
	}

	runner := NewRunner(cfg, []providers.Provider{mock}, nil, nil, nil)
	runner.Run(context.Background())

	results := runner.GetCollector().GetResults()
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if results[0].Success {
		t.Error("expected success to be false due to error")
	}
	if results[0].Error != "api error" {
		t.Errorf("expected error message 'api error', got %s", results[0].Error)
	}
}

func TestRun_PartialFailure(t *testing.T) {
	cfg := &config.Config{
		General: config.GeneralConfig{
			Concurrency: 2,
			Timeout:     "30s",
			OutputDir:   t.TempDir(),
		},
		Tests: []config.TestConfig{
			{Name: "test1", Type: "search", Query: "q1"},
			{Name: "test2", Type: "search", Query: "q2"},
		},
	}

	mock := &mockProvider{
		name: "mock",
		searchFn: func(ctx context.Context, query string, opts providers.SearchOptions) (*providers.SearchResult, error) {
			if query == "q1" {
				return &providers.SearchResult{}, nil
			}
			return nil, errors.New("error")
		},
	}

	runner := NewRunner(cfg, []providers.Provider{mock}, nil, nil, nil)
	runner.Run(context.Background())

	results := runner.GetCollector().GetResults()
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	var successCount, failCount int
	for _, r := range results {
		if r.Success {
			successCount++
		} else {
			failCount++
		}
	}

	if successCount != 1 || failCount != 1 {
		t.Errorf("expected 1 success and 1 failure, got %d success and %d failure", successCount, failCount)
	}
}

func TestRun_SearchResultMetrics(t *testing.T) {
	cfg := &config.Config{
		General: config.GeneralConfig{
			Concurrency: 1,
			Timeout:     "30s",
			OutputDir:   t.TempDir(),
		},
		Tests: []config.TestConfig{
			{Name: "search-test", Type: "search", Query: "query"},
		},
	}

	mock := &mockProvider{
		name: "mock",
		searchFn: func(ctx context.Context, query string, opts providers.SearchOptions) (*providers.SearchResult, error) {
			return &providers.SearchResult{
				Query:        query,
				Results:      []providers.SearchItem{{Content: "content1"}, {Content: "content2"}},
				TotalResults: 2,
				Latency:      150 * time.Millisecond,
				CreditsUsed:  1,
			}, nil
		},
	}

	runner := NewRunner(cfg, []providers.Provider{mock}, nil, nil, nil)
	runner.Run(context.Background())

	results := runner.GetCollector().GetResults()
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	r := results[0]
	if r.ProviderLatency != 150*time.Millisecond {
		t.Errorf("expected provider latency 150ms, got %v", r.ProviderLatency)
	}
	if r.Latency <= 0 {
		t.Errorf("expected wall-clock latency > 0, got %v", r.Latency)
	}
	if r.CreditsUsed != 1 {
		t.Errorf("expected credits 1, got %d", r.CreditsUsed)
	}
	if r.ResultsCount != 2 {
		t.Errorf("expected results count 2, got %d", r.ResultsCount)
	}
	if r.ContentLength != len("content1")+len("content2") {
		t.Errorf("expected content length %d, got %d", len("content1")+len("content2"), r.ContentLength)
	}
}

func TestEnsureOutputDir_Creates(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := tmpDir + "/nested/output"

	cfg := &config.Config{
		General: config.GeneralConfig{
			Concurrency: 1,
			Timeout:     "30s",
			OutputDir:   outputDir,
		},
		Tests: []config.TestConfig{},
	}

	runner := NewRunner(cfg, []providers.Provider{}, nil, nil, nil)
	err := runner.EnsureOutputDir()
	if err != nil {
		t.Fatalf("EnsureOutputDir failed: %v", err)
	}

	// Verify directory exists
	info, err := os.Stat(outputDir)
	if err != nil {
		t.Fatalf("output directory not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("output path is not a directory")
	}
}

func TestEnsureOutputDir_Exists(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &config.Config{
		General: config.GeneralConfig{
			Concurrency: 1,
			Timeout:     "30s",
			OutputDir:   tmpDir,
		},
		Tests: []config.TestConfig{},
	}

	runner := NewRunner(cfg, []providers.Provider{}, nil, nil, nil)
	err := runner.EnsureOutputDir()
	if err != nil {
		t.Fatalf("EnsureOutputDir failed on existing directory: %v", err)
	}
}

// TestRun_RaceStress is a dedicated race condition stress test
// It runs many concurrent operations to trigger any race conditions
func TestRun_RaceStress(t *testing.T) {
	// Run multiple times to increase race detection probability
	for i := 0; i < 3; i++ {
		t.Run(fmt.Sprintf("iteration-%d", i), func(t *testing.T) {
			// Create fresh config and runner for each iteration
			cfg := &config.Config{
				General: config.GeneralConfig{
					Concurrency: 10,
					Timeout:     "5s",
					OutputDir:   t.TempDir(),
				},
				Tests: []config.TestConfig{
					{Name: "search-1", Type: "search", Query: "query1"},
					{Name: "search-2", Type: "search", Query: "query2"},
					{Name: "search-3", Type: "search", Query: "query3"},
					{Name: "extract-1", Type: "extract", URL: "https://example.com/1"},
					{Name: "extract-2", Type: "extract", URL: "https://example.com/2"},
					{Name: "crawl-1", Type: "crawl", URL: "https://example.com", MaxPages: intPtr(2)},
				},
			}

			// Multiple providers to increase contention
			mock1 := &mockProvider{name: "provider1"}
			mock2 := &mockProvider{name: "provider2"}
			mock3 := &mockProvider{name: "provider3"}

			runner := NewRunner(cfg, []providers.Provider{mock1, mock2, mock3}, nil, nil, nil)

			err := runner.Run(context.Background())
			if err != nil {
				t.Fatalf("Run failed: %v", err)
			}

			// Verify all results are consistent
			results := runner.GetCollector().GetResults()
			expected := len(cfg.Tests) * 3 // 3 providers
			if len(results) != expected {
				t.Errorf("expected %d results, got %d", expected, len(results))
			}

			// Verify no duplicate results
			seen := make(map[string]bool)
			for _, r := range results {
				key := r.TestName + "/" + r.Provider
				if seen[key] {
					t.Errorf("duplicate result for %s", key)
				}
				seen[key] = true
			}
		})
	}
}

// TestRun_CollectorThreadSafety specifically tests concurrent collector access
func TestRun_CollectorThreadSafety(t *testing.T) {
	cfg := &config.Config{
		General: config.GeneralConfig{
			Concurrency: 20, // High concurrency to maximize race chances
			Timeout:     "10s",
			OutputDir:   t.TempDir(),
		},
		Tests: []config.TestConfig{
			{Name: "test-1", Type: "search", Query: "q1"},
			{Name: "test-2", Type: "search", Query: "q2"},
			{Name: "test-3", Type: "search", Query: "q3"},
			{Name: "test-4", Type: "search", Query: "q4"},
			{Name: "test-5", Type: "search", Query: "q5"},
		},
	}

	// Slow provider to increase overlap
	slowMock := &mockProvider{
		name: "slow",
		searchFn: func(ctx context.Context, query string, opts providers.SearchOptions) (*providers.SearchResult, error) {
			time.Sleep(10 * time.Millisecond) // Small delay to increase overlap
			return &providers.SearchResult{
				Query:       query,
				Latency:     10 * time.Millisecond,
				CreditsUsed: 1,
			}, nil
		},
	}

	runner := NewRunner(cfg, []providers.Provider{slowMock}, nil, nil, nil)

	// Run and concurrently read from collector
	var wg sync.WaitGroup

	// Start the benchmark
	wg.Add(1)
	go func() {
		defer wg.Done()
		runner.Run(context.Background())
	}()

	// Concurrently try to read from collector during execution
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = runner.GetCollector().GetResults()
				_ = runner.GetCollector().GetAllProviders()
				time.Sleep(time.Millisecond)
			}
		}()
	}

	wg.Wait()
}

func TestRun_NormalizedStrictSkipsEmulatedOperations(t *testing.T) {
	cfg := &config.Config{
		General: config.GeneralConfig{
			Concurrency: 1,
			Timeout:     "30s",
			OutputDir:   t.TempDir(),
		},
		Tests: []config.TestConfig{
			{Name: "extract-test", Type: "extract", URL: "https://example.com"},
		},
	}

	mock := &mockProvider{
		name: "emulated",
		capabilities: providers.CapabilitySet{
			Search:  providers.SupportNative,
			Extract: providers.SupportEmulated,
			Crawl:   providers.SupportNative,
		},
	}

	runner := NewRunner(cfg, []providers.Provider{mock}, nil, nil, nil, RunnerOptions{
		Mode:             providers.ModeNormalized,
		Repeats:          1,
		CapabilityPolicy: CapabilityPolicyStrict,
	})
	if err := runner.Run(context.Background()); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	results := runner.GetCollector().GetResults()
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Skipped {
		t.Fatal("expected emulated operation to be skipped in normalized strict mode")
	}
	if results[0].ExclusionReason == "" {
		t.Fatal("expected exclusion reason to be set")
	}
}

func TestRun_RepeatsExecuteAllRuns(t *testing.T) {
	cfg := &config.Config{
		General: config.GeneralConfig{
			Concurrency: 1,
			Timeout:     "30s",
			OutputDir:   t.TempDir(),
		},
		Tests: []config.TestConfig{
			{Name: "search-test", Type: "search", Query: "q"},
		},
	}

	mock := &mockProvider{name: "repeat-mock"}

	runner := NewRunner(cfg, []providers.Provider{mock}, nil, nil, nil, RunnerOptions{
		Mode:             providers.ModeNormalized,
		Repeats:          3,
		CapabilityPolicy: CapabilityPolicyStrict,
	})
	if err := runner.Run(context.Background()); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	results := runner.GetCollector().GetResults()
	if len(results) != 3 {
		t.Fatalf("expected 3 results for 3 repeats, got %d", len(results))
	}
	if atomic.LoadInt32(&mock.searchCalls) != 3 {
		t.Fatalf("expected 3 search calls, got %d", mock.searchCalls)
	}
}
