package metrics

import (
	"sync"
	"testing"
	"time"
)

func TestCollector_AddAndGet(t *testing.T) {
	c := NewCollector()

	r1 := Result{
		TestName:      "test1",
		Provider:      "firecrawl",
		TestType:      "search",
		Success:       true,
		Latency:       100 * time.Millisecond,
		CreditsUsed:   1,
		ContentLength: 1000,
		ResultsCount:  5,
	}
	r2 := Result{
		TestName:      "test2",
		Provider:      "tavily",
		TestType:      "extract",
		Success:       false,
		Error:         "timeout",
		Latency:       500 * time.Millisecond,
		CreditsUsed:   2,
		ContentLength: 0,
	}

	c.AddResult(r1)
	c.AddResult(r2)

	results := c.GetResults()
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}

	// Verify data integrity
	if results[0].TestName != "test1" {
		t.Errorf("expected first result test1, got %s", results[0].TestName)
	}
	if results[1].Provider != "tavily" {
		t.Errorf("expected second result provider tavily, got %s", results[1].Provider)
	}
}

func TestCollector_GetByProvider(t *testing.T) {
	c := NewCollector()

	c.AddResult(Result{Provider: "firecrawl", TestName: "t1"})
	c.AddResult(Result{Provider: "firecrawl", TestName: "t2"})
	c.AddResult(Result{Provider: "tavily", TestName: "t3"})

	firecrawlResults := c.GetResultsByProvider("firecrawl")
	if len(firecrawlResults) != 2 {
		t.Errorf("expected 2 firecrawl results, got %d", len(firecrawlResults))
	}

	tavilyResults := c.GetResultsByProvider("tavily")
	if len(tavilyResults) != 1 {
		t.Errorf("expected 1 tavily result, got %d", len(tavilyResults))
	}

	emptyResults := c.GetResultsByProvider("nonexistent")
	if len(emptyResults) != 0 {
		t.Errorf("expected 0 results for nonexistent provider, got %d", len(emptyResults))
	}
}

func TestCollector_GetByTest(t *testing.T) {
	c := NewCollector()

	c.AddResult(Result{TestName: "search-test", Provider: "firecrawl"})
	c.AddResult(Result{TestName: "search-test", Provider: "tavily"})
	c.AddResult(Result{TestName: "extract-test", Provider: "firecrawl"})

	searchResults := c.GetResultsByTest("search-test")
	if len(searchResults) != 2 {
		t.Errorf("expected 2 search-test results, got %d", len(searchResults))
	}

	extractResults := c.GetResultsByTest("extract-test")
	if len(extractResults) != 1 {
		t.Errorf("expected 1 extract-test result, got %d", len(extractResults))
	}
}

func TestCollector_GetAllProviders(t *testing.T) {
	c := NewCollector()

	c.AddResult(Result{Provider: "firecrawl"})
	c.AddResult(Result{Provider: "firecrawl"})
	c.AddResult(Result{Provider: "tavily"})
	c.AddResult(Result{Provider: "google"})

	providers := c.GetAllProviders()
	if len(providers) != 3 {
		t.Errorf("expected 3 unique providers, got %d", len(providers))
	}

	// Check all providers are present
	providerMap := make(map[string]bool)
	for _, p := range providers {
		providerMap[p] = true
	}
	if !providerMap["firecrawl"] || !providerMap["tavily"] || !providerMap["google"] {
		t.Error("not all expected providers found")
	}
}

func TestCollector_GetAllProviders_Sorted(t *testing.T) {
	c := NewCollector()
	c.AddResult(Result{Provider: "zeta"})
	c.AddResult(Result{Provider: "alpha"})
	c.AddResult(Result{Provider: "mid"})

	providers := c.GetAllProviders()
	expected := []string{"alpha", "mid", "zeta"}
	if len(providers) != len(expected) {
		t.Fatalf("expected %d providers, got %d", len(expected), len(providers))
	}
	for i := range expected {
		if providers[i] != expected[i] {
			t.Fatalf("expected providers[%d]=%q, got %q", i, expected[i], providers[i])
		}
	}
}

func TestCollector_GetAllTests(t *testing.T) {
	c := NewCollector()

	c.AddResult(Result{TestName: "test1"})
	c.AddResult(Result{TestName: "test1"})
	c.AddResult(Result{TestName: "test2"})

	tests := c.GetAllTests()
	if len(tests) != 2 {
		t.Errorf("expected 2 unique tests, got %d", len(tests))
	}
}

func TestCollector_GetAllTests_Sorted(t *testing.T) {
	c := NewCollector()
	c.AddResult(Result{TestName: "test-z"})
	c.AddResult(Result{TestName: "test-a"})
	c.AddResult(Result{TestName: "test-m"})

	tests := c.GetAllTests()
	expected := []string{"test-a", "test-m", "test-z"}
	if len(tests) != len(expected) {
		t.Fatalf("expected %d tests, got %d", len(expected), len(tests))
	}
	for i := range expected {
		if tests[i] != expected[i] {
			t.Fatalf("expected tests[%d]=%q, got %q", i, expected[i], tests[i])
		}
	}
}

func TestComputeSummary_Empty(t *testing.T) {
	c := NewCollector()

	summary := c.ComputeSummary("firecrawl")

	if summary.Provider != "firecrawl" {
		t.Errorf("expected provider firecrawl, got %s", summary.Provider)
	}
	if summary.TotalTests != 0 {
		t.Errorf("expected 0 total tests, got %d", summary.TotalTests)
	}
}

func TestComputeSummary_SingleResult(t *testing.T) {
	c := NewCollector()
	c.AddResult(Result{
		Provider:      "firecrawl",
		Success:       true,
		Latency:       100 * time.Millisecond,
		CreditsUsed:   1,
		ContentLength: 500,
	})

	summary := c.ComputeSummary("firecrawl")

	if summary.TotalTests != 1 {
		t.Errorf("expected 1 total test, got %d", summary.TotalTests)
	}
	if summary.SuccessfulTests != 1 {
		t.Errorf("expected 1 successful test, got %d", summary.SuccessfulTests)
	}
	if summary.AvgLatency != 100*time.Millisecond {
		t.Errorf("expected avg latency 100ms, got %v", summary.AvgLatency)
	}
	if summary.MinLatency != 100*time.Millisecond || summary.MaxLatency != 100*time.Millisecond {
		t.Errorf("expected min=max=100ms, got min=%v max=%v", summary.MinLatency, summary.MaxLatency)
	}
}

func TestComputeSummary_UsesMeasuredCostUSDWhenPresent(t *testing.T) {
	c := NewCollector()
	c.AddResult(Result{
		Provider:      "firecrawl",
		TestType:      "search",
		Success:       true,
		Latency:       100 * time.Millisecond,
		CreditsUsed:   3,
		ContentLength: 500,
		ResultsCount:  5,
		CostUSD:       1.2345,
	})

	summary := c.ComputeSummary("firecrawl")
	if summary.TotalCostUSD != 1.2345 {
		t.Fatalf("expected measured total cost 1.2345, got %.4f", summary.TotalCostUSD)
	}
	if summary.AvgCostPerReq != 1.2345 {
		t.Fatalf("expected measured avg cost 1.2345, got %.4f", summary.AvgCostPerReq)
	}
}

func TestComputeSummary_Multiple(t *testing.T) {
	c := NewCollector()
	c.AddResult(Result{
		Provider:      "firecrawl",
		Success:       true,
		Latency:       100 * time.Millisecond,
		CreditsUsed:   1,
		ContentLength: 500,
		ResultsCount:  5,
	})
	c.AddResult(Result{
		Provider:      "firecrawl",
		Success:       true,
		Latency:       200 * time.Millisecond,
		CreditsUsed:   1,
		ContentLength: 1500,
		ResultsCount:  10,
	})
	c.AddResult(Result{
		Provider:      "firecrawl",
		Success:       false,
		Latency:       50 * time.Millisecond,
		CreditsUsed:   0,
		ContentLength: 0,
	})

	summary := c.ComputeSummary("firecrawl")

	if summary.TotalTests != 3 {
		t.Errorf("expected 3 total tests, got %d", summary.TotalTests)
	}
	if summary.SuccessfulTests != 2 {
		t.Errorf("expected 2 successful tests, got %d", summary.SuccessfulTests)
	}
	if summary.FailedTests != 1 {
		t.Errorf("expected 1 failed test, got %d", summary.FailedTests)
	}
	// Average of 100ms + 200ms + 50ms = 350ms / 3 = ~116.67ms
	expectedAvg := 350 * time.Millisecond / 3
	if summary.AvgLatency < expectedAvg-1*time.Millisecond || summary.AvgLatency > expectedAvg+1*time.Millisecond {
		t.Errorf("expected avg latency ~%v, got %v", expectedAvg, summary.AvgLatency)
	}
	if summary.MinLatency != 50*time.Millisecond {
		t.Errorf("expected min latency 50ms, got %v", summary.MinLatency)
	}
	if summary.MaxLatency != 200*time.Millisecond {
		t.Errorf("expected max latency 200ms, got %v", summary.MaxLatency)
	}
	if summary.TotalCreditsUsed != 2 {
		t.Errorf("expected total credits 2, got %d", summary.TotalCreditsUsed)
	}
	if summary.AvgCreditsPerReq != 2.0/3.0 {
		t.Errorf("expected avg credits %.2f, got %.2f", 2.0/3.0, summary.AvgCreditsPerReq)
	}
	if summary.TotalContentLength != 2000 {
		t.Errorf("expected total content 2000, got %d", summary.TotalContentLength)
	}
	if summary.AvgContentLength != 2000.0/3.0 {
		t.Errorf("expected avg content %.2f, got %.2f", 2000.0/3.0, summary.AvgContentLength)
	}
	if summary.ResultsPerQuery != 15.0/3.0 {
		t.Errorf("expected results per query %.2f, got %.2f", 15.0/3.0, summary.ResultsPerQuery)
	}
}

func TestComputeSummary_MixedSuccessFail(t *testing.T) {
	c := NewCollector()
	c.AddResult(Result{Provider: "test", Success: true})
	c.AddResult(Result{Provider: "test", Success: true})
	c.AddResult(Result{Provider: "test", Success: false})
	c.AddResult(Result{Provider: "test", Success: false})
	c.AddResult(Result{Provider: "test", Success: false})

	summary := c.ComputeSummary("test")

	if summary.SuccessfulTests != 2 {
		t.Errorf("expected 2 successful, got %d", summary.SuccessfulTests)
	}
	if summary.FailedTests != 3 {
		t.Errorf("expected 3 failed, got %d", summary.FailedTests)
	}
}

func TestCollector_ConcurrentAdd(t *testing.T) {
	c := NewCollector()
	var wg sync.WaitGroup

	// Add results concurrently from multiple goroutines
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			c.AddResult(Result{
				Provider: "concurrent",
				TestName: "test",
				Success:  id%2 == 0,
			})
		}(i)
	}

	wg.Wait()

	results := c.GetResultsByProvider("concurrent")
	if len(results) != 100 {
		t.Errorf("expected 100 concurrent results, got %d", len(results))
	}
}

func TestCollector_ConcurrentReadWrite(t *testing.T) {
	c := NewCollector()
	var wg sync.WaitGroup

	// Writers
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			c.AddResult(Result{Provider: "mixed"})
		}(i)
	}

	// Readers
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = c.GetResults()
			_ = c.GetResultsByProvider("mixed")
			_ = c.ComputeSummary("mixed")
		}()
	}

	wg.Wait()
}

func TestComputeSummary_ZeroLatency(t *testing.T) {
	c := NewCollector()
	c.AddResult(Result{
		Provider: "test",
		Success:  true,
		Latency:  0,
	})

	summary := c.ComputeSummary("test")

	if summary.AvgLatency != 0 {
		t.Errorf("expected avg latency 0, got %v", summary.AvgLatency)
	}
}

func TestComputeSummary_AllFailures(t *testing.T) {
	c := NewCollector()
	c.AddResult(Result{Provider: "test", Success: false, Latency: 100 * time.Millisecond})
	c.AddResult(Result{Provider: "test", Success: false, Latency: 200 * time.Millisecond})

	summary := c.ComputeSummary("test")

	if summary.SuccessfulTests != 0 {
		t.Errorf("expected 0 successful, got %d", summary.SuccessfulTests)
	}
	if summary.FailedTests != 2 {
		t.Errorf("expected 2 failed, got %d", summary.FailedTests)
	}
}

func TestComputeSummary_AllSuccess(t *testing.T) {
	c := NewCollector()
	c.AddResult(Result{Provider: "test", Success: true})
	c.AddResult(Result{Provider: "test", Success: true})
	c.AddResult(Result{Provider: "test", Success: true})

	summary := c.ComputeSummary("test")

	if summary.SuccessfulTests != 3 {
		t.Errorf("expected 3 successful, got %d", summary.SuccessfulTests)
	}
	if summary.FailedTests != 0 {
		t.Errorf("expected 0 failed, got %d", summary.FailedTests)
	}
}

func TestComputeSummary_SkipsExcludedFromDenominators(t *testing.T) {
	c := NewCollector()
	c.AddResult(Result{
		Provider:      "test",
		Success:       true,
		Latency:       100 * time.Millisecond,
		CreditsUsed:   2,
		ContentLength: 200,
	})
	c.AddResult(Result{
		Provider:   "test",
		Skipped:    true,
		SkipReason: "unsupported",
	})
	c.AddResult(Result{
		Provider:      "test",
		Success:       false,
		Latency:       300 * time.Millisecond,
		CreditsUsed:   4,
		ContentLength: 400,
	})

	summary := c.ComputeSummary("test")

	if summary.TotalTests != 3 {
		t.Fatalf("expected 3 total tests, got %d", summary.TotalTests)
	}
	if summary.SkippedTests != 1 {
		t.Fatalf("expected 1 skipped test, got %d", summary.SkippedTests)
	}
	if summary.ExecutedTests != 2 {
		t.Fatalf("expected 2 executed tests, got %d", summary.ExecutedTests)
	}
	if summary.SuccessRate != 50 {
		t.Fatalf("expected success rate 50, got %.2f", summary.SuccessRate)
	}
	if summary.AvgLatency != 200*time.Millisecond {
		t.Fatalf("expected avg latency 200ms, got %v", summary.AvgLatency)
	}
	if summary.AvgCreditsPerReq != 3 {
		t.Fatalf("expected avg credits/request 3, got %.2f", summary.AvgCreditsPerReq)
	}
	if summary.AvgContentLength != 300 {
		t.Fatalf("expected avg content length 300, got %.2f", summary.AvgContentLength)
	}
}

func TestComputeSummary_AllSkipped(t *testing.T) {
	c := NewCollector()
	c.AddResult(Result{Provider: "test", Skipped: true, SkipReason: "unsupported"})
	c.AddResult(Result{Provider: "test", Skipped: true, SkipReason: "unsupported"})

	summary := c.ComputeSummary("test")
	if summary.TotalTests != 2 {
		t.Fatalf("expected 2 total tests, got %d", summary.TotalTests)
	}
	if summary.ExecutedTests != 0 {
		t.Fatalf("expected 0 executed tests, got %d", summary.ExecutedTests)
	}
	if summary.SkippedTests != 2 {
		t.Fatalf("expected 2 skipped tests, got %d", summary.SkippedTests)
	}
	if summary.SuccessRate != 0 {
		t.Fatalf("expected success rate 0, got %.2f", summary.SuccessRate)
	}
	if summary.AvgLatency != 0 {
		t.Fatalf("expected avg latency 0, got %v", summary.AvgLatency)
	}
}
