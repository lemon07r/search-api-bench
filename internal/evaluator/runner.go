// Package evaluator executes benchmark tests against search providers.
package evaluator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/lamim/search-api-bench/internal/config"
	"github.com/lamim/search-api-bench/internal/debug"
	"github.com/lamim/search-api-bench/internal/metrics"
	"github.com/lamim/search-api-bench/internal/progress"
	"github.com/lamim/search-api-bench/internal/providers"
	"github.com/lamim/search-api-bench/internal/quality"
)

// costCalculator is used for USD cost calculations
var costCalculator = metrics.NewCostCalculator()

// Runner executes benchmark tests
type Runner struct {
	providers   []providers.Provider
	config      *config.Config
	collector   *metrics.Collector
	progress    *progress.Manager
	debugLogger *debug.Logger
	scorer      *quality.Scorer
}

// NewRunner creates a new test runner
func NewRunner(cfg *config.Config, provs []providers.Provider, prog *progress.Manager, debugLog *debug.Logger, scorer *quality.Scorer) *Runner {
	return &Runner{
		providers:   provs,
		config:      cfg,
		collector:   metrics.NewCollector(),
		progress:    prog,
		debugLogger: debugLog,
		scorer:      scorer,
	}
}

// Run executes all tests against all providers
func (r *Runner) Run(ctx context.Context) error {
	if r.progress != nil && r.progress.IsEnabled() {
		// Progress bar will be shown, print minimal header
		fmt.Println("Starting benchmark...")
	} else {
		// No progress bar, print full header
		fmt.Printf("Starting benchmark with %d tests against %d providers\n", len(r.config.Tests), len(r.providers))
		fmt.Printf("Concurrency: %d, Timeout: %s\n\n", r.config.General.Concurrency, r.config.General.Timeout)
	}

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

	if r.progress != nil {
		r.progress.Finish()
	}

	fmt.Println("\nBenchmark completed!")

	return nil
}

func (r *Runner) runTest(ctx context.Context, test config.TestConfig, prov providers.Provider) {
	result := metrics.Result{
		TestName:  test.Name,
		Provider:  prov.Name(),
		TestType:  test.Type,
		Timestamp: time.Now(),
	}

	// Check if provider supports this operation type
	if !prov.SupportsOperation(test.Type) {
		result.Skipped = true
		result.SkipReason = fmt.Sprintf("%s provider does not support %s operations", prov.Name(), test.Type)

		// Report to progress manager (skipped counts as success for progress)
		if r.progress != nil {
			r.progress.StartTest(prov.Name(), test.Name)
			r.progress.CompleteTest(prov.Name(), test.Name, true, nil)
		}

		if r.progress == nil || !r.progress.IsEnabled() {
			fmt.Printf("[%s] Skipping '%s': %s\n", prov.Name(), test.Name, result.SkipReason)
		}

		r.collector.AddResult(result)
		return
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, r.config.General.TimeoutDuration())
	defer cancel()

	// Start debug logging for this test
	var testLog *debug.TestLog
	if r.debugLogger != nil && r.debugLogger.IsEnabled() {
		testLog = r.debugLogger.StartTest(prov.Name(), test.Name, test.Type)
		// Pass debug logger and test log via context
		timeoutCtx = providers.WithDebugLogger(timeoutCtx, r.debugLogger)
		timeoutCtx = providers.WithTestLog(timeoutCtx, testLog)
		// Use defer to ensure EndTest is called even if a panic occurs
		defer r.debugLogger.EndTest(testLog)
	}

	// Report test start to progress manager
	if r.progress != nil {
		r.progress.StartTest(prov.Name(), test.Name)
	}

	if r.progress == nil || !r.progress.IsEnabled() {
		fmt.Printf("[%s] Running '%s'...\n", prov.Name(), test.Name)
	}

	switch test.Type {
	case "search":
		r.runSearchTest(timeoutCtx, test, prov, &result, testLog)
	case "extract":
		r.runExtractTest(timeoutCtx, test, prov, &result, testLog)
	case "crawl":
		r.runCrawlTest(timeoutCtx, test, prov, &result, testLog)
	}

	// Report test completion to progress manager
	if r.progress != nil {
		var testErr error
		if !result.Success {
			testErr = fmt.Errorf("%s", result.Error)
		}
		r.progress.CompleteTest(prov.Name(), test.Name, result.Success, testErr)
	}

	r.collector.AddResult(result)
}

func (r *Runner) runSearchTest(ctx context.Context, test config.TestConfig, prov providers.Provider, result *metrics.Result, testLog *debug.TestLog) {
	opts := providers.DefaultSearchOptions()
	opts.MaxResults = 5
	opts.IncludeAnswer = true

	startTime := time.Now()
	searchResult, err := prov.Search(ctx, test.Query, opts)
	latency := time.Since(startTime)

	if err != nil {
		result.Success = false
		result.Error = err.Error()
		result.ErrorCategory = categorizeError(err)
		if r.debugLogger != nil && r.debugLogger.IsEnabled() {
			r.debugLogger.LogError(testLog, err.Error(), result.ErrorCategory, "search execution")
		}
		if r.progress == nil || !r.progress.IsEnabled() {
			fmt.Printf("  ✗ %s failed: %v\n", prov.Name(), err)
		}
		return
	}

	result.Success = true
	result.Latency = searchResult.Latency
	result.CreditsUsed = searchResult.CreditsUsed
	result.ResultsCount = searchResult.TotalResults
	result.CostUSD = costCalculator.CalculateProviderCost(prov.Name(), searchResult.CreditsUsed, "search")

	// Log debug info
	if r.debugLogger != nil && r.debugLogger.IsEnabled() {
		r.debugLogger.SetMetadata(testLog, "query", test.Query)
		r.debugLogger.SetMetadata(testLog, "result_count", searchResult.TotalResults)
		r.debugLogger.SetMetadata(testLog, "latency_ms", latency.Milliseconds())
	}

	// Calculate content length from all results
	contentLength := 0
	for _, item := range searchResult.Results {
		contentLength += len(item.Content)
	}
	result.ContentLength = contentLength

	// Check expected topics
	if r.progress == nil || !r.progress.IsEnabled() {
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
			fmt.Printf("  ✓ %s: %d results, %v latency, $%.4f cost\n",
				prov.Name(), searchResult.TotalResults, searchResult.Latency.Round(time.Millisecond), result.CostUSD)
		}
	}

	// Perform quality scoring if scorer is available
	if r.scorer != nil && len(searchResult.Results) > 0 {
		qualityScore, err := r.scorer.ScoreSearch(ctx, test.Query, searchResult.Results)
		if err != nil {
			if r.debugLogger != nil && r.debugLogger.IsEnabled() {
				r.debugLogger.LogError(testLog, fmt.Sprintf("quality scoring failed: %v", err), "quality_error", "search quality scoring")
			}
		} else {
			result.QualityScore = qualityScore.OverallScore
			result.SemanticScore = qualityScore.SemanticRelevance
			result.RerankerScore = qualityScore.RerankerScore
			if r.debugLogger != nil && r.debugLogger.IsEnabled() {
				r.debugLogger.SetMetadata(testLog, "quality_score", qualityScore.OverallScore)
				r.debugLogger.SetMetadata(testLog, "semantic_score", qualityScore.SemanticRelevance)
				r.debugLogger.SetMetadata(testLog, "reranker_score", qualityScore.RerankerScore)
			}
		}
	}
}

func (r *Runner) runExtractTest(ctx context.Context, test config.TestConfig, prov providers.Provider, result *metrics.Result, testLog *debug.TestLog) {
	opts := providers.DefaultExtractOptions()

	startTime := time.Now()
	extractResult, err := prov.Extract(ctx, test.URL, opts)
	latency := time.Since(startTime)

	if err != nil {
		result.Success = false
		result.Error = err.Error()
		result.ErrorCategory = categorizeError(err)
		if r.debugLogger != nil && r.debugLogger.IsEnabled() {
			r.debugLogger.LogError(testLog, err.Error(), result.ErrorCategory, "extract execution")
		}
		if r.progress == nil || !r.progress.IsEnabled() {
			fmt.Printf("  ✗ %s failed: %v\n", prov.Name(), err)
		}
		return
	}

	result.Success = true
	result.Latency = extractResult.Latency
	result.CreditsUsed = extractResult.CreditsUsed
	result.ContentLength = len(extractResult.Content)
	result.CostUSD = costCalculator.CalculateProviderCost(prov.Name(), extractResult.CreditsUsed, "extract")

	// Log debug info
	if r.debugLogger != nil && r.debugLogger.IsEnabled() {
		r.debugLogger.SetMetadata(testLog, "url", test.URL)
		r.debugLogger.SetMetadata(testLog, "content_length", len(extractResult.Content))
		r.debugLogger.SetMetadata(testLog, "latency_ms", latency.Milliseconds())
	}

	// Check expected content
	if r.progress == nil || !r.progress.IsEnabled() {
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
			fmt.Printf("  ✓ %s: %d chars, %v latency, $%.4f cost\n",
				prov.Name(), len(extractResult.Content), extractResult.Latency.Round(time.Millisecond), result.CostUSD)
		}
	}

	// Perform quality scoring if scorer is available
	if r.scorer != nil {
		qualityScore := r.scorer.ScoreExtract(extractResult.Content, extractResult.URL, test.ExpectedContent)
		result.QualityScore = qualityScore.OverallScore
		if r.debugLogger != nil && r.debugLogger.IsEnabled() {
			r.debugLogger.SetMetadata(testLog, "quality_score", qualityScore.OverallScore)
			r.debugLogger.SetMetadata(testLog, "completeness_score", qualityScore.ContentCompleteness)
			r.debugLogger.SetMetadata(testLog, "structure_score", qualityScore.StructurePreservation)
		}
	}
}

func (r *Runner) runCrawlTest(ctx context.Context, test config.TestConfig, prov providers.Provider, result *metrics.Result, testLog *debug.TestLog) {
	opts := providers.DefaultCrawlOptions()
	if test.MaxPages > 0 {
		opts.MaxPages = test.MaxPages
	}
	if test.MaxDepth > 0 {
		opts.MaxDepth = test.MaxDepth
	}

	startTime := time.Now()
	crawlResult, err := prov.Crawl(ctx, test.URL, opts)
	latency := time.Since(startTime)

	if err != nil {
		result.Success = false
		result.Error = err.Error()
		result.ErrorCategory = categorizeError(err)
		if r.debugLogger != nil && r.debugLogger.IsEnabled() {
			r.debugLogger.LogError(testLog, err.Error(), result.ErrorCategory, "crawl execution")
		}
		if r.progress == nil || !r.progress.IsEnabled() {
			fmt.Printf("  ✗ %s failed: %v\n", prov.Name(), err)
		}
		return
	}

	result.Success = true
	result.Latency = crawlResult.Latency
	result.CreditsUsed = crawlResult.CreditsUsed
	result.ResultsCount = crawlResult.TotalPages
	result.CostUSD = costCalculator.CalculateProviderCost(prov.Name(), crawlResult.CreditsUsed, "crawl")

	// Log debug info
	if r.debugLogger != nil && r.debugLogger.IsEnabled() {
		r.debugLogger.SetMetadata(testLog, "url", test.URL)
		r.debugLogger.SetMetadata(testLog, "pages_crawled", crawlResult.TotalPages)
		r.debugLogger.SetMetadata(testLog, "max_pages", opts.MaxPages)
		r.debugLogger.SetMetadata(testLog, "max_depth", opts.MaxDepth)
		r.debugLogger.SetMetadata(testLog, "latency_ms", latency.Milliseconds())
	}

	// Calculate total content length
	contentLength := 0
	for _, page := range crawlResult.Pages {
		contentLength += len(page.Content)
	}
	result.ContentLength = contentLength

	if r.progress == nil || !r.progress.IsEnabled() {
		fmt.Printf("  ✓ %s: %d pages, %d chars, %v latency, $%.4f cost\n",
			prov.Name(), crawlResult.TotalPages, contentLength, crawlResult.Latency.Round(time.Millisecond), result.CostUSD)
	}

	// Perform quality scoring if scorer is available
	if r.scorer != nil {
		qualityScore := r.scorer.ScoreCrawl(crawlResult, opts)
		result.QualityScore = qualityScore.OverallScore
		if r.debugLogger != nil && r.debugLogger.IsEnabled() {
			r.debugLogger.SetMetadata(testLog, "quality_score", qualityScore.OverallScore)
			r.debugLogger.SetMetadata(testLog, "coverage_score", qualityScore.CoverageScore)
			r.debugLogger.SetMetadata(testLog, "depth_accuracy", qualityScore.DepthAccuracy)
		}
	}
}

// errorPattern maps error substrings to their categories
type errorPattern struct {
	patterns []string
	category string
}

// errorPatterns defines all error categorization patterns in priority order
var errorPatterns = []errorPattern{
	// Timeout errors
	{
		patterns: []string{"timeout", "context deadline exceeded", "i/o timeout"},
		category: "timeout",
	},
	// Rate limit errors - checked before auth since "too many requests" can appear with 429
	{
		patterns: []string{"rate limit", "ratelimit", "too many requests", "429", "quota exceeded", "limit exceeded"},
		category: "rate_limit",
	},
	// Auth errors
	{
		patterns: []string{"401", "403", "unauthorized", "authentication", "api key"},
		category: "auth",
	},
	// Server errors (5xx)
	{
		patterns: []string{"500", "502", "503", "504", "internal server error", "bad gateway", "service unavailable"},
		category: "server_error",
	},
	// Client errors (4xx)
	{
		patterns: []string{"400", "404", "405", "422", "bad request", "not found", "validation"},
		category: "client_error",
	},
	// Network errors
	{
		patterns: []string{"connection refused", "no such host", "network", "dns", "temporary failure"},
		category: "network",
	},
	// Parse errors
	{
		patterns: []string{"unmarshal", "parse", "invalid character", "invalid syntax"},
		category: "parse",
	},
	// Crawl/scrape specific errors
	{
		patterns: []string{"crawl", "scrape", "not supported"},
		category: "crawl_failed",
	},
	// Context canceled
	{
		patterns: []string{"context canceled"},
		category: "canceled",
	},
}

// categorizeError categorizes an error for better reporting
func categorizeError(err error) string {
	if err == nil {
		return ""
	}

	errStr := strings.ToLower(err.Error())

	for _, ep := range errorPatterns {
		for _, pattern := range ep.patterns {
			if strings.Contains(errStr, pattern) {
				return ep.category
			}
		}
	}

	return "other"
}

// GetCollector returns the metrics collector
func (r *Runner) GetCollector() *metrics.Collector {
	return r.collector
}

// EnsureOutputDir creates a timestamped session subdirectory for results
func (r *Runner) EnsureOutputDir() error {
	// Create a timestamped subdirectory for this session
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	sessionDir := filepath.Join(r.config.General.OutputDir, timestamp)

	// Update config to use the session directory for this run
	r.config.General.OutputDir = sessionDir

	// #nosec G301 - 0750 is more restrictive than 0755 but still allows owner/group access
	return os.MkdirAll(sessionDir, 0750)
}
