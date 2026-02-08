// Package evaluator executes benchmark tests against search providers.
package evaluator

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/lamim/search-api-bench/internal/config"
	"github.com/lamim/search-api-bench/internal/metrics"
	"github.com/lamim/search-api-bench/internal/providers"
)

// Runner executes benchmark tests
type Runner struct {
	providers []providers.Provider
	config    *config.Config
	collector *metrics.Collector
}

// NewRunner creates a new test runner
func NewRunner(cfg *config.Config, provs []providers.Provider) *Runner {
	return &Runner{
		providers: provs,
		config:    cfg,
		collector: metrics.NewCollector(),
	}
}

// Run executes all tests against all providers
func (r *Runner) Run(ctx context.Context) error {
	fmt.Printf("Starting benchmark with %d tests against %d providers\n", len(r.config.Tests), len(r.providers))
	fmt.Printf("Concurrency: %d, Timeout: %s\n\n", r.config.General.Concurrency, r.config.General.Timeout)

	// Create semaphore for concurrency control
	sem := make(chan struct{}, r.config.General.Concurrency)
	var wg sync.WaitGroup

	// Run tests
	for _, test := range r.config.Tests {
		for _, prov := range r.providers {
			wg.Add(1)
			go func(t config.TestConfig, p providers.Provider) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				r.runTest(ctx, t, p)
			}(test, prov)
		}
	}

	wg.Wait()
	fmt.Println("\nBenchmark completed!")

	return nil
}

func (r *Runner) runTest(ctx context.Context, test config.TestConfig, prov providers.Provider) {
	timeoutCtx, cancel := context.WithTimeout(ctx, r.config.General.TimeoutDuration())
	defer cancel()

	result := metrics.Result{
		TestName:  test.Name,
		Provider:  prov.Name(),
		TestType:  test.Type,
		Timestamp: time.Now(),
	}

	fmt.Printf("[%s] Running '%s'...\n", prov.Name(), test.Name)

	switch test.Type {
	case "search":
		r.runSearchTest(timeoutCtx, test, prov, &result)
	case "extract":
		r.runExtractTest(timeoutCtx, test, prov, &result)
	case "crawl":
		r.runCrawlTest(timeoutCtx, test, prov, &result)
	}

	r.collector.AddResult(result)
}

func (r *Runner) runSearchTest(ctx context.Context, test config.TestConfig, prov providers.Provider, result *metrics.Result) {
	opts := providers.DefaultSearchOptions()
	opts.MaxResults = 5
	opts.IncludeAnswer = true

	searchResult, err := prov.Search(ctx, test.Query, opts)
	if err != nil {
		result.Success = false
		result.Error = err.Error()
		fmt.Printf("  ✗ %s failed: %v\n", prov.Name(), err)
		return
	}

	result.Success = true
	result.Latency = searchResult.Latency
	result.CreditsUsed = searchResult.CreditsUsed
	result.ResultsCount = searchResult.TotalResults

	// Calculate content length from all results
	contentLength := 0
	for _, item := range searchResult.Results {
		contentLength += len(item.Content)
	}
	result.ContentLength = contentLength

	// Check expected topics
	if len(test.ExpectedTopics) > 0 {
		allContent := ""
		for _, item := range searchResult.Results {
			allContent += item.Title + " " + item.Content + " "
		}
		allContent = strings.ToLower(allContent)
		
		matchedTopics := 0
		for _, topic := range test.ExpectedTopics {
			if strings.Contains(allContent, strings.ToLower(topic)) {
				matchedTopics++
			}
		}
		fmt.Printf("  ✓ %s: %d results, %v latency, %d/%d topics matched\n",
			prov.Name(), searchResult.TotalResults, searchResult.Latency.Round(time.Millisecond), matchedTopics, len(test.ExpectedTopics))
	} else {
		fmt.Printf("  ✓ %s: %d results, %v latency, %d credits\n",
			prov.Name(), searchResult.TotalResults, searchResult.Latency.Round(time.Millisecond), searchResult.CreditsUsed)
	}
}

func (r *Runner) runExtractTest(ctx context.Context, test config.TestConfig, prov providers.Provider, result *metrics.Result) {
	opts := providers.DefaultExtractOptions()

	extractResult, err := prov.Extract(ctx, test.URL, opts)
	if err != nil {
		result.Success = false
		result.Error = err.Error()
		fmt.Printf("  ✗ %s failed: %v\n", prov.Name(), err)
		return
	}

	result.Success = true
	result.Latency = extractResult.Latency
	result.CreditsUsed = extractResult.CreditsUsed
	result.ContentLength = len(extractResult.Content)

	// Check expected content
	if len(test.ExpectedContent) > 0 {
		contentLower := strings.ToLower(extractResult.Content)
		matchedContent := 0
		for _, expected := range test.ExpectedContent {
			if strings.Contains(contentLower, strings.ToLower(expected)) {
				matchedContent++
			}
		}
		fmt.Printf("  ✓ %s: %d chars, %v latency, %d/%d content items matched\n",
			prov.Name(), len(extractResult.Content), extractResult.Latency.Round(time.Millisecond), matchedContent, len(test.ExpectedContent))
	} else {
		fmt.Printf("  ✓ %s: %d chars, %v latency, %d credits\n",
			prov.Name(), len(extractResult.Content), extractResult.Latency.Round(time.Millisecond), extractResult.CreditsUsed)
	}
}

func (r *Runner) runCrawlTest(ctx context.Context, test config.TestConfig, prov providers.Provider, result *metrics.Result) {
	opts := providers.DefaultCrawlOptions()
	if test.MaxPages > 0 {
		opts.MaxPages = test.MaxPages
	}
	if test.MaxDepth > 0 {
		opts.MaxDepth = test.MaxDepth
	}

	crawlResult, err := prov.Crawl(ctx, test.URL, opts)
	if err != nil {
		result.Success = false
		result.Error = err.Error()
		fmt.Printf("  ✗ %s failed: %v\n", prov.Name(), err)
		return
	}

	result.Success = true
	result.Latency = crawlResult.Latency
	result.CreditsUsed = crawlResult.CreditsUsed
	result.ResultsCount = crawlResult.TotalPages

	// Calculate total content length
	contentLength := 0
	for _, page := range crawlResult.Pages {
		contentLength += len(page.Content)
	}
	result.ContentLength = contentLength

	fmt.Printf("  ✓ %s: %d pages, %d chars, %v latency, %d credits\n",
		prov.Name(), crawlResult.TotalPages, contentLength, crawlResult.Latency.Round(time.Millisecond), crawlResult.CreditsUsed)
}

// GetCollector returns the metrics collector
func (r *Runner) GetCollector() *metrics.Collector {
	return r.collector
}

// EnsureOutputDir creates the output directory if it doesn't exist
func (r *Runner) EnsureOutputDir() error {
	// #nosec G301 - 0750 is more restrictive than 0755 but still allows owner/group access
	return os.MkdirAll(r.config.General.OutputDir, 0750)
}
