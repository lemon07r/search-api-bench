// Package evaluator executes benchmark tests against search providers.
package evaluator

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/lamim/sanity-web-eval/internal/config"
	"github.com/lamim/sanity-web-eval/internal/debug"
	"github.com/lamim/sanity-web-eval/internal/metrics"
	"github.com/lamim/sanity-web-eval/internal/progress"
	"github.com/lamim/sanity-web-eval/internal/providers"
	"github.com/lamim/sanity-web-eval/internal/quality"
)

// costCalculator is used for USD cost calculations
var costCalculator = metrics.NewCostCalculator()

// Runner executes benchmark tests
type Runner struct {
	providers   []providers.Provider
	config      *config.Config
	providerSem map[string]chan struct{}
	collector   *metrics.Collector
	progress    *progress.Manager
	debugLogger *debug.Logger
	scorer      *quality.Scorer
	options     RunnerOptions
}

// CapabilityPolicy defines normalized-mode handling for emulated operations.
type CapabilityPolicy string

const (
	// CapabilityPolicyStrict skips emulated operations in normalized mode.
	CapabilityPolicyStrict CapabilityPolicy = "strict"
	// CapabilityPolicyTagged runs emulated operations but excludes them from primary comparisons.
	CapabilityPolicyTagged CapabilityPolicy = "tagged"
)

// RunnerOptions controls execution behavior.
type RunnerOptions struct {
	Mode             providers.RunMode
	Repeats          int
	CapabilityPolicy CapabilityPolicy
}

// DefaultRunnerOptions returns production defaults.
func DefaultRunnerOptions() RunnerOptions {
	return RunnerOptions{
		Mode:             providers.ModeNormalized,
		Repeats:          1,
		CapabilityPolicy: CapabilityPolicyStrict,
	}
}

// NewRunner creates a new test runner
func NewRunner(cfg *config.Config, provs []providers.Provider, prog *progress.Manager, debugLog *debug.Logger, scorer *quality.Scorer, opts ...RunnerOptions) *Runner {
	runnerOptions := DefaultRunnerOptions()
	if len(opts) > 0 {
		runnerOptions = opts[0]
	}
	if runnerOptions.Repeats <= 0 {
		runnerOptions.Repeats = 1
	}
	if runnerOptions.Mode == "" {
		runnerOptions.Mode = providers.ModeNormalized
	}
	if runnerOptions.CapabilityPolicy == "" {
		runnerOptions.CapabilityPolicy = CapabilityPolicyStrict
	}
	providerSem := make(map[string]chan struct{}, len(provs))
	for _, prov := range provs {
		name := strings.ToLower(strings.TrimSpace(prov.Name()))
		if name == "" {
			continue
		}
		if _, exists := providerSem[name]; exists {
			continue
		}
		providerSem[name] = make(chan struct{}, cfg.General.ConcurrencyForProvider(name))
	}
	return &Runner{
		providers:   provs,
		config:      cfg,
		providerSem: providerSem,
		collector:   metrics.NewCollector(),
		progress:    prog,
		debugLogger: debugLog,
		scorer:      scorer,
		options:     runnerOptions,
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
		if len(r.config.General.ProviderConcurrency) > 0 {
			fmt.Printf("Mode: %s, Repeats: %d, Concurrency: %d (provider overrides: %d), Timeout: %s\n\n",
				r.options.Mode, r.options.Repeats, r.config.General.Concurrency, len(r.config.General.ProviderConcurrency), r.config.General.Timeout)
		} else {
			fmt.Printf("Mode: %s, Repeats: %d, Concurrency: %d, Timeout: %s\n\n",
				r.options.Mode, r.options.Repeats, r.config.General.Concurrency, r.config.General.Timeout)
		}
	}

	// Create semaphore for concurrency control.
	globalLimit := r.config.General.Concurrency
	if globalLimit <= 0 {
		globalLimit = 1
	}
	globalSem := make(chan struct{}, globalLimit)
	var wg sync.WaitGroup

	// Run tests
	for repeat := 1; repeat <= r.options.Repeats; repeat++ {
		for _, test := range r.config.Tests {
			for _, prov := range r.providers {
				wg.Add(1)
				go func(rep int, t config.TestConfig, p providers.Provider) {
					defer wg.Done()
					providerSem := r.providerSem[strings.ToLower(strings.TrimSpace(p.Name()))]
					if providerSem == nil {
						// Fallback for unexpected provider naming mismatches.
						providerSem = globalSem
					}

					globalSem <- struct{}{}
					providerSem <- struct{}{}
					defer func() {
						<-providerSem
						<-globalSem
					}()

					r.runTest(ctx, rep, t, p)
				}(repeat, test, prov)
			}
		}
	}

	wg.Wait()

	if r.progress != nil {
		r.progress.Finish()
	}

	fmt.Println("\nBenchmark completed!")

	return nil
}

func (r *Runner) runTest(ctx context.Context, repeat int, test config.TestConfig, prov providers.Provider) {
	capabilities := prov.Capabilities()
	supportLevel := capabilities.ForOperation(test.Type)

	result := metrics.Result{
		TestName:            test.Name,
		Provider:            prov.Name(),
		TestType:            test.Type,
		RunMode:             string(r.options.Mode),
		Repeat:              repeat,
		ImplementationType:  string(supportLevel),
		ExcludedFromPrimary: supportLevel != providers.SupportNative,
		Timestamp:           time.Now(),
	}

	// Check if provider supports this operation type
	if !capabilities.SupportsOperation(test.Type) {
		result.Skipped = true
		result.SkipReason = fmt.Sprintf("%s provider does not support %s operations", prov.Name(), test.Type)
		result.ExcludedFromPrimary = true
		result.ExclusionReason = "unsupported"
		r.completeSkippedResult(prov, test, result)
		return
	}

	if r.options.Mode == providers.ModeNormalized && supportLevel == providers.SupportEmulated {
		result.ExcludedFromPrimary = true
		result.ExclusionReason = "emulated_in_normalized_mode"
		if r.options.CapabilityPolicy == CapabilityPolicyStrict {
			result.Skipped = true
			result.SkipReason = fmt.Sprintf("%s %s operation is emulated and skipped in normalized strict mode", prov.Name(), test.Type)
			r.completeSkippedResult(prov, test, result)
			return
		}
	}

	if result.ExcludedFromPrimary && result.ExclusionReason == "" {
		result.ExclusionReason = "not_primary_comparable"
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
		fmt.Printf("[%s][%s][run %d] Running '%s'...\n", prov.Name(), r.options.Mode, repeat, test.Name)
	}

	switch test.Type {
	case "search":
		r.runSearchTest(timeoutCtx, test, prov, &result, testLog)
	case "extract":
		r.runExtractTest(timeoutCtx, test, prov, &result, testLog)
	case "crawl":
		r.runCrawlTest(timeoutCtx, test, prov, &result, testLog)
	}

	if testLog != nil && r.debugLogger != nil && r.debugLogger.IsEnabled() {
		if result.Success {
			r.debugLogger.SetStatus(testLog, "completed")
		} else {
			r.debugLogger.SetStatus(testLog, "failed")
		}
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

func (r *Runner) completeSkippedResult(prov providers.Provider, test config.TestConfig, result metrics.Result) {
	// Report to progress manager (skipped counts as success for progress)
	if r.progress != nil {
		r.progress.StartTest(prov.Name(), test.Name)
		r.progress.CompleteTest(prov.Name(), test.Name, true, nil)
	}

	if r.progress == nil || !r.progress.IsEnabled() {
		fmt.Printf("[%s] Skipping '%s': %s\n", prov.Name(), test.Name, result.SkipReason)
	}

	r.collector.AddResult(result)
}

func (r *Runner) runSearchTest(ctx context.Context, test config.TestConfig, prov providers.Provider, result *metrics.Result, testLog *debug.TestLog) {
	opts := r.searchOptionsForMode()

	startTime := time.Now()
	searchResult, err := prov.Search(ctx, test.Query, opts)
	wallClockLatency := time.Since(startTime)

	if err != nil {
		result.Success = false
		result.Latency = wallClockLatency
		result.Error = err.Error()
		result.ErrorCategory = categorizeError(err)
		if r.debugLogger != nil && r.debugLogger.IsEnabled() {
			r.debugLogger.LogError(testLog, err.Error(), result.ErrorCategory, "search execution")
			r.debugLogger.SetMetadata(testLog, "latency_ms", wallClockLatency.Milliseconds())
		}
		if r.progress == nil || !r.progress.IsEnabled() {
			fmt.Printf("  ✗ %s failed: %v\n", prov.Name(), err)
		}
		return
	}

	result.Success = true
	result.Latency = wallClockLatency
	result.ProviderLatency = searchResult.Latency
	result.CreditsUsed = searchResult.CreditsUsed
	result.RequestCount = searchResult.RequestCount
	result.UsageReported = searchResult.UsageReported
	if result.RequestCount <= 0 {
		result.RequestCount = 1
	}
	result.ResultsCount = searchResult.TotalResults
	result.CostUSD = costCalculator.CalculateProviderCost(prov.Name(), searchResult.CreditsUsed, "search")

	// Log debug info
	if r.debugLogger != nil && r.debugLogger.IsEnabled() {
		r.debugLogger.SetMetadata(testLog, "query", test.Query)
		r.debugLogger.SetMetadata(testLog, "result_count", searchResult.TotalResults)
		r.debugLogger.SetMetadata(testLog, "latency_ms", wallClockLatency.Milliseconds())
		r.debugLogger.SetMetadata(testLog, "provider_latency_ms", searchResult.Latency.Milliseconds())
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
				prov.Name(), searchResult.TotalResults, wallClockLatency.Round(time.Millisecond), matchedTopics, len(test.ExpectedTopics))
		} else {
			fmt.Printf("  ✓ %s: %d results, %v latency, $%.4f cost\n",
				prov.Name(), searchResult.TotalResults, wallClockLatency.Round(time.Millisecond), result.CostUSD)
		}
	}

	groundTruthScore, groundTruthMetrics := evaluateSearchGroundTruth(test, searchResult.Results)
	hasGroundTruth := groundTruthMetrics["ground_truth_available"] == float64(1)
	var modelScore float64
	hasModelScore := false

	// Perform model-assisted quality scoring if scorer is available
	if r.scorer != nil && len(searchResult.Results) > 0 {
		qualityScore, err := r.scorer.ScoreSearch(ctx, test.Query, searchResult.Results)
		if err != nil {
			if r.debugLogger != nil && r.debugLogger.IsEnabled() {
				r.debugLogger.LogError(testLog, fmt.Sprintf("quality scoring failed: %v", err), "quality_error", "search quality scoring")
			}
		} else {
			modelScore = qualityScore.OverallScore
			hasModelScore = true
			result.SemanticScore = qualityScore.SemanticRelevance
			result.RerankerScore = qualityScore.RerankerScore
			if r.debugLogger != nil && r.debugLogger.IsEnabled() {
				r.debugLogger.SetMetadata(testLog, "quality_model_score", qualityScore.OverallScore)
				r.debugLogger.SetMetadata(testLog, "semantic_score", qualityScore.SemanticRelevance)
				r.debugLogger.SetMetadata(testLog, "reranker_score", qualityScore.RerankerScore)
				r.debugLogger.SetMetadata(testLog, "semantic_available", qualityScore.SemanticAvailable)
				r.debugLogger.SetMetadata(testLog, "reranker_available", qualityScore.RerankerAvailable)
			}
		}
	}

	combined, scored := combineQualityScores(groundTruthScore, hasGroundTruth, modelScore, hasModelScore)
	result.QualityScore = combined
	result.QualityScored = scored
	result.RawQualityMetrics = buildSearchQualityMetricsMap(groundTruthMetrics, hasModelScore, modelScore)

	if r.debugLogger != nil && r.debugLogger.IsEnabled() {
		r.debugLogger.SetMetadata(testLog, "quality_score", result.QualityScore)
		r.debugLogger.SetMetadata(testLog, "quality_scored", result.QualityScored)
	}
}

func (r *Runner) runExtractTest(ctx context.Context, test config.TestConfig, prov providers.Provider, result *metrics.Result, testLog *debug.TestLog) {
	opts := providers.DefaultExtractOptions()

	startTime := time.Now()
	extractResult, err := prov.Extract(ctx, test.URL, opts)
	wallClockLatency := time.Since(startTime)

	if err != nil {
		result.Success = false
		result.Latency = wallClockLatency
		result.Error = err.Error()
		result.ErrorCategory = categorizeError(err)
		if r.debugLogger != nil && r.debugLogger.IsEnabled() {
			r.debugLogger.LogError(testLog, err.Error(), result.ErrorCategory, "extract execution")
			r.debugLogger.SetMetadata(testLog, "latency_ms", wallClockLatency.Milliseconds())
		}
		if r.progress == nil || !r.progress.IsEnabled() {
			fmt.Printf("  ✗ %s failed: %v\n", prov.Name(), err)
		}
		return
	}

	result.Success = true
	result.Latency = wallClockLatency
	result.ProviderLatency = extractResult.Latency
	result.CreditsUsed = extractResult.CreditsUsed
	result.RequestCount = extractResult.RequestCount
	result.UsageReported = extractResult.UsageReported
	if result.RequestCount <= 0 {
		result.RequestCount = 1
	}
	result.ContentLength = len(extractResult.Content)
	result.CostUSD = costCalculator.CalculateProviderCost(prov.Name(), extractResult.CreditsUsed, "extract")

	// Log debug info
	if r.debugLogger != nil && r.debugLogger.IsEnabled() {
		r.debugLogger.SetMetadata(testLog, "url", test.URL)
		r.debugLogger.SetMetadata(testLog, "content_length", len(extractResult.Content))
		r.debugLogger.SetMetadata(testLog, "latency_ms", wallClockLatency.Milliseconds())
		r.debugLogger.SetMetadata(testLog, "provider_latency_ms", extractResult.Latency.Milliseconds())
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
				prov.Name(), len(extractResult.Content), wallClockLatency.Round(time.Millisecond), matchedContent, len(test.ExpectedContent))
		} else {
			fmt.Printf("  ✓ %s: %d chars, %v latency, $%.4f cost\n",
				prov.Name(), len(extractResult.Content), wallClockLatency.Round(time.Millisecond), result.CostUSD)
		}
	}

	groundTruthScore, groundTruthMetrics := evaluateExtractGroundTruth(test, extractResult.Content)
	hasGroundTruth := groundTruthMetrics["ground_truth_available"] == float64(1)
	var modelScore float64
	hasModelScore := false

	// Perform heuristic/model quality scoring if scorer is available
	if r.scorer != nil {
		qualityScore := r.scorer.ScoreExtract(extractResult.Content, extractResult.URL, test.ExpectedContent)
		modelScore = qualityScore.OverallScore
		hasModelScore = true
		if r.debugLogger != nil && r.debugLogger.IsEnabled() {
			r.debugLogger.SetMetadata(testLog, "quality_model_score", qualityScore.OverallScore)
			r.debugLogger.SetMetadata(testLog, "completeness_score", qualityScore.ContentCompleteness)
			r.debugLogger.SetMetadata(testLog, "structure_score", qualityScore.StructurePreservation)
		}
	}

	combined, scored := combineQualityScores(groundTruthScore, hasGroundTruth, modelScore, hasModelScore)
	result.QualityScore = combined
	result.QualityScored = scored
	result.RawQualityMetrics = buildExtractQualityMetricsMap(groundTruthMetrics, hasModelScore, modelScore)

	if r.debugLogger != nil && r.debugLogger.IsEnabled() {
		r.debugLogger.SetMetadata(testLog, "quality_score", result.QualityScore)
		r.debugLogger.SetMetadata(testLog, "quality_scored", result.QualityScored)
	}
}

func (r *Runner) runCrawlTest(ctx context.Context, test config.TestConfig, prov providers.Provider, result *metrics.Result, testLog *debug.TestLog) {
	opts := providers.DefaultCrawlOptions()
	if test.MaxPages != nil {
		opts.MaxPages = *test.MaxPages
	}
	if test.MaxDepth != nil {
		opts.MaxDepth = *test.MaxDepth
	}

	startTime := time.Now()
	crawlResult, err := prov.Crawl(ctx, test.URL, opts)
	wallClockLatency := time.Since(startTime)

	if err != nil {
		result.Success = false
		result.Latency = wallClockLatency
		result.Error = err.Error()
		result.ErrorCategory = categorizeError(err)
		if r.debugLogger != nil && r.debugLogger.IsEnabled() {
			r.debugLogger.LogError(testLog, err.Error(), result.ErrorCategory, "crawl execution")
			r.debugLogger.SetMetadata(testLog, "latency_ms", wallClockLatency.Milliseconds())
		}
		if r.progress == nil || !r.progress.IsEnabled() {
			fmt.Printf("  ✗ %s failed: %v\n", prov.Name(), err)
		}
		return
	}

	result.Success = true
	result.Latency = wallClockLatency
	result.ProviderLatency = crawlResult.Latency
	result.CreditsUsed = crawlResult.CreditsUsed
	result.RequestCount = crawlResult.RequestCount
	result.UsageReported = crawlResult.UsageReported
	if result.RequestCount <= 0 {
		result.RequestCount = 1
	}
	result.ResultsCount = crawlResult.TotalPages
	result.CostUSD = costCalculator.CalculateProviderCost(prov.Name(), crawlResult.CreditsUsed, "crawl")

	// Log debug info
	if r.debugLogger != nil && r.debugLogger.IsEnabled() {
		r.debugLogger.SetMetadata(testLog, "url", test.URL)
		r.debugLogger.SetMetadata(testLog, "pages_crawled", crawlResult.TotalPages)
		r.debugLogger.SetMetadata(testLog, "max_pages", opts.MaxPages)
		r.debugLogger.SetMetadata(testLog, "max_depth", opts.MaxDepth)
		r.debugLogger.SetMetadata(testLog, "latency_ms", wallClockLatency.Milliseconds())
		r.debugLogger.SetMetadata(testLog, "provider_latency_ms", crawlResult.Latency.Milliseconds())
	}

	// Calculate total content length
	contentLength := 0
	for _, page := range crawlResult.Pages {
		contentLength += len(page.Content)
	}
	result.ContentLength = contentLength

	if r.progress == nil || !r.progress.IsEnabled() {
		fmt.Printf("  ✓ %s: %d pages, %d chars, %v latency, $%.4f cost\n",
			prov.Name(), crawlResult.TotalPages, contentLength, wallClockLatency.Round(time.Millisecond), result.CostUSD)
	}

	groundTruthScore, groundTruthMetrics := evaluateCrawlGroundTruth(test, crawlResult)
	hasGroundTruth := groundTruthMetrics["ground_truth_available"] == float64(1)
	var modelScore float64
	hasModelScore := false

	// Perform heuristic quality scoring if scorer is available
	if r.scorer != nil {
		qualityScore := r.scorer.ScoreCrawl(crawlResult, opts)
		modelScore = qualityScore.OverallScore
		hasModelScore = true
		if r.debugLogger != nil && r.debugLogger.IsEnabled() {
			r.debugLogger.SetMetadata(testLog, "quality_model_score", qualityScore.OverallScore)
			r.debugLogger.SetMetadata(testLog, "coverage_score", qualityScore.CoverageScore)
			r.debugLogger.SetMetadata(testLog, "depth_accuracy", qualityScore.DepthAccuracy)
		}
	}

	combined, scored := combineQualityScores(groundTruthScore, hasGroundTruth, modelScore, hasModelScore)
	result.QualityScore = combined
	result.QualityScored = scored
	result.RawQualityMetrics = buildCrawlQualityMetricsMap(groundTruthMetrics, hasModelScore, modelScore)

	if r.debugLogger != nil && r.debugLogger.IsEnabled() {
		r.debugLogger.SetMetadata(testLog, "quality_score", result.QualityScore)
		r.debugLogger.SetMetadata(testLog, "quality_scored", result.QualityScored)
	}
}

func (r *Runner) searchOptionsForMode() providers.SearchOptions {
	opts := providers.DefaultSearchOptions()
	opts.MaxResults = 5
	switch r.options.Mode {
	case providers.ModeNative:
		opts.SearchDepth = "advanced"
		opts.IncludeAnswer = true
	case providers.ModeNormalized:
		opts.SearchDepth = "advanced"
		opts.IncludeAnswer = false
		opts.IncludeImages = false
	default:
		opts.SearchDepth = "advanced"
		opts.IncludeAnswer = false
	}
	return opts
}

func combineQualityScores(groundTruthScore float64, hasGroundTruth bool, modelScore float64, hasModelScore bool) (float64, bool) {
	switch {
	case hasGroundTruth && hasModelScore:
		return clampScore((groundTruthScore * 0.7) + (modelScore * 0.3)), true
	case hasGroundTruth:
		return clampScore(groundTruthScore), true
	case hasModelScore:
		return clampScore(modelScore), true
	default:
		return 0, false
	}
}

func evaluateSearchGroundTruth(test config.TestConfig, results []providers.SearchItem) (float64, map[string]float64) {
	metrics := map[string]float64{
		"ground_truth_available": 0,
	}

	expectedTerms := uniqueNonEmptyStrings(append(append([]string{}, test.ExpectedTopics...), test.MustIncludeTerms...))
	expectedURLs := uniqueNonEmptyStrings(test.ExpectedURLs)
	forbiddenTerms := uniqueNonEmptyStrings(test.MustNotIncludeTerms)

	if len(expectedTerms) == 0 && len(expectedURLs) == 0 && len(forbiddenTerms) == 0 {
		return 0, metrics
	}
	metrics["ground_truth_available"] = 1

	allContent := ""
	for _, item := range results {
		allContent += " " + item.Title + " " + item.Content
	}
	allContent = strings.ToLower(allContent)

	matchedTerms := 0
	for _, term := range expectedTerms {
		if strings.Contains(allContent, strings.ToLower(term)) {
			matchedTerms++
		}
	}
	termRecall := ratioPct(matchedTerms, len(expectedTerms))

	expectedURLSet := make(map[string]struct{}, len(expectedURLs))
	for _, raw := range expectedURLs {
		expectedURLSet[normalizeURLForMatch(raw)] = struct{}{}
	}
	matchedURLSet := make(map[string]struct{})
	matchedResultURLs := 0

	for _, item := range results {
		norm := normalizeURLForMatch(item.URL)
		if _, ok := expectedURLSet[norm]; ok {
			matchedURLSet[norm] = struct{}{}
			matchedResultURLs++
			continue
		}
		for expected := range expectedURLSet {
			if strings.Contains(norm, expected) || strings.Contains(expected, norm) {
				matchedURLSet[expected] = struct{}{}
				matchedResultURLs++
				break
			}
		}
	}

	urlRecall := ratioPct(len(matchedURLSet), len(expectedURLs))
	urlPrecision := ratioPct(matchedResultURLs, len(results))

	forbiddenHits := 0
	for _, term := range forbiddenTerms {
		if strings.Contains(allContent, strings.ToLower(term)) {
			forbiddenHits++
		}
	}
	forbiddenPenalty := ratioPct(forbiddenHits, len(forbiddenTerms)) * 0.4

	scoreComponents := make([]float64, 0, 3)
	weights := make([]float64, 0, 3)
	if len(expectedTerms) > 0 {
		scoreComponents = append(scoreComponents, termRecall)
		weights = append(weights, 0.6)
	}
	if len(expectedURLs) > 0 {
		scoreComponents = append(scoreComponents, (urlRecall+urlPrecision)/2)
		weights = append(weights, 0.4)
	}
	if len(scoreComponents) == 0 {
		scoreComponents = append(scoreComponents, 100)
		weights = append(weights, 1)
	}

	score := weightedAverage(scoreComponents, weights) - forbiddenPenalty
	score = clampScore(score)

	metrics["expected_terms"] = float64(len(expectedTerms))
	metrics["matched_terms"] = float64(matchedTerms)
	metrics["term_recall"] = termRecall
	metrics["expected_urls"] = float64(len(expectedURLs))
	metrics["matched_urls"] = float64(len(matchedURLSet))
	metrics["url_recall"] = urlRecall
	metrics["url_precision"] = urlPrecision
	metrics["forbidden_hits"] = float64(forbiddenHits)

	return score, metrics
}

func evaluateExtractGroundTruth(test config.TestConfig, content string) (float64, map[string]float64) {
	metrics := map[string]float64{
		"ground_truth_available": 0,
	}

	expectedSnippets := uniqueNonEmptyStrings(append(append([]string{}, test.ExpectedContent...), test.ExpectedSnippets...))
	forbiddenSnippets := uniqueNonEmptyStrings(append(append([]string{}, test.ForbiddenSnippets...), test.MustNotIncludeTerms...))

	if len(expectedSnippets) == 0 && len(forbiddenSnippets) == 0 {
		return 0, metrics
	}
	metrics["ground_truth_available"] = 1

	contentLower := strings.ToLower(content)
	matched := 0
	for _, snippet := range expectedSnippets {
		if strings.Contains(contentLower, strings.ToLower(snippet)) {
			matched++
		}
	}

	forbiddenHits := 0
	for _, forbidden := range forbiddenSnippets {
		if strings.Contains(contentLower, strings.ToLower(forbidden)) {
			forbiddenHits++
		}
	}

	recall := ratioPct(matched, len(expectedSnippets))
	safety := 100 - ratioPct(forbiddenHits, len(forbiddenSnippets))
	if len(forbiddenSnippets) == 0 {
		safety = 100
	}

	score := clampScore((recall * 0.8) + (safety * 0.2))

	metrics["expected_snippets"] = float64(len(expectedSnippets))
	metrics["matched_snippets"] = float64(matched)
	metrics["snippet_recall"] = recall
	metrics["forbidden_snippets"] = float64(len(forbiddenSnippets))
	metrics["forbidden_hits"] = float64(forbiddenHits)
	metrics["safety_score"] = safety

	return score, metrics
}

func evaluateCrawlGroundTruth(test config.TestConfig, crawlResult *providers.CrawlResult) (float64, map[string]float64) {
	metrics := map[string]float64{
		"ground_truth_available": 0,
	}

	expectedURLs := uniqueNonEmptyStrings(test.ExpectedURLs)
	expectedPatterns := uniqueNonEmptyStrings(test.ExpectedURLPatterns)

	if len(expectedURLs) == 0 && len(expectedPatterns) == 0 {
		return 0, metrics
	}
	metrics["ground_truth_available"] = 1

	actualURLs := make([]string, 0, len(crawlResult.Pages))
	for _, page := range crawlResult.Pages {
		actualURLs = append(actualURLs, normalizeURLForMatch(page.URL))
	}

	expectedURLSet := make(map[string]struct{}, len(expectedURLs))
	for _, expected := range expectedURLs {
		expectedURLSet[normalizeURLForMatch(expected)] = struct{}{}
	}

	matchedURLs := 0
	for expected := range expectedURLSet {
		for _, actual := range actualURLs {
			if actual == expected || strings.Contains(actual, expected) || strings.Contains(expected, actual) {
				matchedURLs++
				break
			}
		}
	}
	urlRecall := ratioPct(matchedURLs, len(expectedURLs))

	matchedPatterns := 0
	for _, pattern := range expectedPatterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			continue
		}
		for _, actual := range actualURLs {
			if re.MatchString(actual) {
				matchedPatterns++
				break
			}
		}
	}
	patternRecall := ratioPct(matchedPatterns, len(expectedPatterns))

	scoreParts := make([]float64, 0, 2)
	weights := make([]float64, 0, 2)
	if len(expectedURLs) > 0 {
		scoreParts = append(scoreParts, urlRecall)
		weights = append(weights, 0.7)
	}
	if len(expectedPatterns) > 0 {
		scoreParts = append(scoreParts, patternRecall)
		weights = append(weights, 0.3)
	}
	score := clampScore(weightedAverage(scoreParts, weights))

	metrics["expected_urls"] = float64(len(expectedURLs))
	metrics["matched_urls"] = float64(matchedURLs)
	metrics["url_recall"] = urlRecall
	metrics["expected_patterns"] = float64(len(expectedPatterns))
	metrics["matched_patterns"] = float64(matchedPatterns)
	metrics["pattern_recall"] = patternRecall

	return score, metrics
}

func buildSearchQualityMetricsMap(groundTruthMetrics map[string]float64, hasModelScore bool, modelScore float64) map[string]interface{} {
	m := make(map[string]interface{}, len(groundTruthMetrics)+2)
	for k, v := range groundTruthMetrics {
		m[k] = v
	}
	m["model_score_available"] = hasModelScore
	if hasModelScore {
		m["model_score"] = modelScore
	}
	return m
}

func buildExtractQualityMetricsMap(groundTruthMetrics map[string]float64, hasModelScore bool, modelScore float64) map[string]interface{} {
	return buildSearchQualityMetricsMap(groundTruthMetrics, hasModelScore, modelScore)
}

func buildCrawlQualityMetricsMap(groundTruthMetrics map[string]float64, hasModelScore bool, modelScore float64) map[string]interface{} {
	return buildSearchQualityMetricsMap(groundTruthMetrics, hasModelScore, modelScore)
}

func uniqueNonEmptyStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		normalized := strings.ToLower(trimmed)
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func normalizeURLForMatch(raw string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	raw = strings.TrimPrefix(raw, "http://")
	raw = strings.TrimPrefix(raw, "https://")
	raw = strings.TrimPrefix(raw, "www.")
	raw = strings.TrimSuffix(raw, "/")
	return raw
}

func ratioPct(numerator, denominator int) float64 {
	if denominator <= 0 {
		return 0
	}
	return (float64(numerator) / float64(denominator)) * 100
}

func weightedAverage(values []float64, weights []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	if len(values) != len(weights) {
		return values[0]
	}

	var weightedTotal float64
	var weightTotal float64
	for i := range values {
		weightedTotal += values[i] * weights[i]
		weightTotal += weights[i]
	}
	if weightTotal == 0 {
		return values[0]
	}
	return weightedTotal / weightTotal
}

func clampScore(score float64) float64 {
	return math.Max(0, math.Min(100, score))
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
