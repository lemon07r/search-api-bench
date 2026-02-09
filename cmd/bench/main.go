// Package main provides the entry point for the search API benchmark tool.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lamim/search-api-bench/internal/config"
	"github.com/lamim/search-api-bench/internal/debug"
	"github.com/lamim/search-api-bench/internal/evaluator"
	"github.com/lamim/search-api-bench/internal/metrics"
	"github.com/lamim/search-api-bench/internal/progress"
	"github.com/lamim/search-api-bench/internal/providers"
	"github.com/lamim/search-api-bench/internal/providers/brave"
	"github.com/lamim/search-api-bench/internal/providers/exa"
	"github.com/lamim/search-api-bench/internal/providers/firecrawl"
	"github.com/lamim/search-api-bench/internal/providers/jina"
	"github.com/lamim/search-api-bench/internal/providers/local"
	"github.com/lamim/search-api-bench/internal/providers/mixedbread"
	"github.com/lamim/search-api-bench/internal/providers/tavily"
	"github.com/lamim/search-api-bench/internal/quality"
	"github.com/lamim/search-api-bench/internal/report"
)

type cliFlags struct {
	configPath    *string
	outputDir     *string
	providersFlag *string
	format        *string
	noProgress    *bool
	debugMode     *bool
	debugFullMode *bool
	quickMode     *bool
	noSearch      *bool
	noLocal       *bool
	qualityMode   *bool
}

func parseFlags() *cliFlags {
	return &cliFlags{
		configPath:    flag.String("config", "config.toml", "Path to configuration file"),
		outputDir:     flag.String("output", "", "Output directory for reports (overrides config)"),
		providersFlag: flag.String("providers", "all", "Providers to test: all, firecrawl, tavily, local, brave, exa, mixedbread, jina"),
		format:        flag.String("format", "all", "Report format: all, html, md, json"),
		noProgress:    flag.Bool("no-progress", false, "Disable progress bar (useful for CI)"),
		debugMode:     flag.Bool("debug", false, "Enable debug logging with request/response data"),
		debugFullMode: flag.Bool("debug-full", false, "Enable full debug logging with complete request/response bodies and timing breakdown"),
		quickMode:     flag.Bool("quick", false, "Run quick test with reduced test set and shorter timeouts"),
		noSearch:      flag.Bool("no-search", false, "Exclude search tests"),
		noLocal:       flag.Bool("no-local", false, "Exclude local provider"),
		qualityMode:   flag.Bool("quality", false, "Enable AI quality scoring (requires EMBEDDING_* and RERANKER_* env vars)"),
	}
}

func loadEnvFile() {
	if data, err := os.ReadFile(".env"); err == nil {
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])
				value = strings.Trim(value, `"'`)
				_ = os.Setenv(key, value)
			}
		}
	}
}

func main() {
	flags := parseFlags()
	flag.Parse()

	loadEnvFile()

	providerNames, err := parseProviders(*flags.providersFlag, *flags.noLocal)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing providers: %v\n", err)
		os.Exit(1)
	}

	formats, err := parseFormats(*flags.format)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing formats: %v\n", err)
		os.Exit(1)
	}

	cfg, err := config.Load(*flags.configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	if *flags.outputDir != "" {
		cfg.General.OutputDir = *flags.outputDir
	}

	if *flags.quickMode {
		cfg = applyQuickMode(cfg)
	}

	// Apply test type filters
	if *flags.noSearch {
		cfg.Tests = filterTests(cfg.Tests, *flags.noSearch)
		if len(cfg.Tests) == 0 {
			fmt.Fprintf(os.Stderr, "Error: no tests match the specified filters\n")
			os.Exit(1)
		}
	}

	finalOutputDir, err := ensureOutputDir(cfg.General.OutputDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating output directory: %v\n", err)
		os.Exit(1)
	}
	cfg.General.OutputDir = finalOutputDir

	// Enable debug mode if debug-full is set
	enableDebug := *flags.debugMode || *flags.debugFullMode
	debugLogger := debug.NewLogger(enableDebug, *flags.debugFullMode, cfg.General.OutputDir)

	// Initialize quality scorer if enabled
	var scorer *quality.Scorer
	if *flags.qualityMode {
		var err error
		scorer, err = initializeQualityScorer()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: -quality flag set but failed to initialize: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("🎯 Quality scoring enabled: using embedding + reranker models")
		fmt.Println()
	}

	printBanner()

	if *flags.quickMode {
		fmt.Println("⚡ Quick mode enabled: running subset of tests with reduced parameters")
		fmt.Printf("   Tests: %d | Timeout: %s\n\n", len(cfg.Tests), cfg.General.Timeout)
	}

	if *flags.noSearch {
		fmt.Printf("🚫 No-search mode: running %d non-search tests\n\n", len(cfg.Tests))
	}

	if enableDebug {
		if *flags.debugFullMode {
			fmt.Printf("🐛 Debug-full mode enabled: complete bodies + timing breakdown\n")
			fmt.Printf("   Logging to: %s/\n\n", debugLogger.GetOutputPath())
		} else {
			fmt.Printf("🐛 Debug mode enabled: logging to %s/\n\n", debugLogger.GetOutputPath())
		}
	}

	provs := initializeProviders(providerNames, debugLogger)

	if len(provs) == 0 {
		fmt.Fprintf(os.Stderr, "Error: no providers initialized. Check API keys for selected providers: %s\n", strings.Join(providerNames, ", "))
		os.Exit(1)
	}

	// Calculate total tests
	totalTests := len(cfg.Tests) * len(provs)

	// Get provider names for progress display
	progressProviderNames := make([]string, 0, len(provs))
	for _, p := range provs {
		progressProviderNames = append(progressProviderNames, p.Name())
	}

	// Create progress manager
	prog := progress.NewManager(totalTests, progressProviderNames, !*flags.noProgress)

	// Create runner with progress manager, debug logger, and optional quality scorer
	runner := evaluator.NewRunner(cfg, provs, prog, debugLogger, scorer)

	// Print initial banner if not using progress bar
	if *flags.noProgress {
		printBanner()
	}

	// Run benchmarks
	ctx := context.Background()
	if err := runner.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error running benchmarks: %v\n", err)
		os.Exit(1)
	}

	// Finalize debug logging
	if enableDebug {
		if err := debugLogger.Finalize(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to write debug log: %v\n", err)
		} else {
			fmt.Printf("✓ Debug logs written to: %s/\n", debugLogger.GetOutputPath())
		}
	}

	// Generate reports
	generateReports(formats, runner.GetCollector(), cfg.General.OutputDir)
}

func printBanner() {
	fmt.Println(`
╔══════════════════════════════════════════════════════════════╗
║               Search API Benchmark Tool                      ║
║    Compare Firecrawl, Tavily, Brave, Exa, Mixedbread, Jina   ║
╚══════════════════════════════════════════════════════════════╝`)
	fmt.Println()
}

func printSummary(collector *metrics.Collector) {
	fmt.Println("\n═══════════════════════════════════════════════════════════════")
	fmt.Println("                     BENCHMARK SUMMARY")
	fmt.Println("═══════════════════════════════════════════════════════════════")

	providerList := collector.GetAllProviders()
	for _, provider := range providerList {
		summary := collector.ComputeSummary(provider)

		fmt.Printf("\n%s:\n", strings.ToUpper(provider))
		fmt.Printf("  Tests: %d (%d executed, %d skipped, %.1f%% success)\n",
			summary.TotalTests,
			summary.ExecutedTests,
			summary.SkippedTests,
			summary.SuccessRate,
		)
		fmt.Printf("  Avg Latency: %s\n", report.FormatLatency(summary.AvgLatency))
		fmt.Printf("  Total Credits: %d\n", summary.TotalCreditsUsed)
		fmt.Printf("  Avg Content: %.0f chars\n", summary.AvgContentLength)
	}

	fmt.Println("\nReports generated successfully!")
	fmt.Println("View detailed results in the output directory.")
}

func initializeProviders(providerNames []string, debugLogger *debug.Logger) []providers.Provider {
	var provs []providers.Provider

	for _, name := range providerNames {
		switch name {
		case "firecrawl":
			client, err := firecrawl.NewClient()
			debugLogger.LogProviderInit("firecrawl", err)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to initialize Firecrawl: %v\n", err)
				continue
			}
			provs = append(provs, client)
			fmt.Printf("✓ Initialized Firecrawl provider\n")

		case "tavily":
			client, err := tavily.NewClient()
			debugLogger.LogProviderInit("tavily", err)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to initialize Tavily: %v\n", err)
				continue
			}
			provs = append(provs, client)
			fmt.Printf("✓ Initialized Tavily provider\n")

		case "local":
			client, err := local.NewClient()
			debugLogger.LogProviderInit("local", err)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to initialize Local crawler: %v\n", err)
				continue
			}
			provs = append(provs, client)
			fmt.Printf("✓ Initialized Local crawler provider (no API key required)\n")
			fmt.Printf("  Note: Local provider does not support search operations (extract/crawl only)\n")

		case "brave":
			client, err := brave.NewClient()
			debugLogger.LogProviderInit("brave", err)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to initialize Brave: %v\n", err)
				continue
			}
			provs = append(provs, client)
			fmt.Printf("✓ Initialized Brave Search provider\n")

		case "exa":
			client, err := exa.NewClient()
			debugLogger.LogProviderInit("exa", err)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to initialize Exa: %v\n", err)
				continue
			}
			provs = append(provs, client)
			fmt.Printf("✓ Initialized Exa AI provider\n")

		case "mixedbread":
			client, err := mixedbread.NewClient()
			debugLogger.LogProviderInit("mixedbread", err)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to initialize Mixedbread: %v\n", err)
				continue
			}
			provs = append(provs, client)
			fmt.Printf("✓ Initialized Mixedbread AI provider\n")

		case "jina":
			client, err := jina.NewClient()
			debugLogger.LogProviderInit("jina", err)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to initialize Jina: %v\n", err)
				continue
			}
			provs = append(provs, client)
			fmt.Printf("✓ Initialized Jina AI provider (Reader + Search)\n")
		}
	}

	return provs
}

func generateReports(formats []string, collector *metrics.Collector, outputDir string) {
	fmt.Println("\nGenerating reports...")
	gen := report.NewGenerator(collector, outputDir)

	for _, f := range formats {
		switch f {
		case "html":
			if err := gen.GenerateHTML(); err != nil {
				fmt.Fprintf(os.Stderr, "Error generating HTML report: %v\n", err)
			} else {
				fmt.Printf("✓ Generated HTML report: %s/report.html\n", outputDir)
			}
		case "md":
			if err := gen.GenerateMarkdown(); err != nil {
				fmt.Fprintf(os.Stderr, "Error generating Markdown report: %v\n", err)
			} else {
				fmt.Printf("✓ Generated Markdown report: %s/report.md\n", outputDir)
			}
		case "json":
			if err := gen.GenerateJSON(); err != nil {
				fmt.Fprintf(os.Stderr, "Error generating JSON report: %v\n", err)
			} else {
				fmt.Printf("✓ Generated JSON report: %s/report.json\n", outputDir)
			}
		case "all":
			if err := gen.GenerateAll(); err != nil {
				fmt.Fprintf(os.Stderr, "Error generating reports: %v\n", err)
			} else {
				fmt.Printf("✓ Generated all reports in: %s/\n", outputDir)
			}
		}
	}

	printSummary(collector)
}

// initializeQualityScorer creates a quality scorer from environment variables
func initializeQualityScorer() (*quality.Scorer, error) {
	// Check required environment variables
	if os.Getenv("EMBEDDING_MODEL_BASE_URL") == "" {
		return nil, fmt.Errorf("EMBEDDING_MODEL_BASE_URL not set")
	}
	if os.Getenv("EMBEDDING_MODEL_API_KEY") == "" {
		return nil, fmt.Errorf("EMBEDDING_MODEL_API_KEY not set")
	}
	if os.Getenv("RERANKER_MODEL_BASE_URL") == "" {
		return nil, fmt.Errorf("RERANKER_MODEL_BASE_URL not set")
	}
	if os.Getenv("RERANKER_MODEL_API_KEY") == "" {
		return nil, fmt.Errorf("RERANKER_MODEL_API_KEY not set")
	}

	// Create embedding client
	embeddingClient, err := quality.NewEmbeddingClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create embedding client: %w", err)
	}

	// Create reranker client
	rerankerClient, err := quality.NewRerankerClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create reranker client: %w", err)
	}

	// Create scorer
	scorer := quality.NewScorer(embeddingClient, rerankerClient)

	return scorer, nil
}

func parseProviders(s string, noLocal bool) ([]string, error) {
	allProviders := []string{"firecrawl", "tavily", "local", "brave", "exa", "mixedbread", "jina"}
	validProviders := map[string]struct{}{
		"firecrawl":  {},
		"tavily":     {},
		"local":      {},
		"brave":      {},
		"exa":        {},
		"mixedbread": {},
		"jina":       {},
	}

	input := strings.ToLower(strings.TrimSpace(s))
	if input == "" {
		return nil, fmt.Errorf("providers cannot be empty (valid values: all, %s)", strings.Join(allProviders, ", "))
	}

	var selected []string
	if input == "all" {
		selected = append(selected, allProviders...)
	} else {
		seen := make(map[string]struct{})
		var invalid []string
		for _, raw := range strings.Split(s, ",") {
			p := strings.ToLower(strings.TrimSpace(raw))
			if p == "" {
				return nil, fmt.Errorf("provider list contains an empty entry")
			}
			if _, ok := validProviders[p]; !ok {
				invalid = append(invalid, p)
				continue
			}
			if _, ok := seen[p]; ok {
				continue
			}
			seen[p] = struct{}{}
			selected = append(selected, p)
		}
		if len(invalid) > 0 {
			return nil, fmt.Errorf("invalid provider(s): %s (valid values: all, %s)", strings.Join(invalid, ", "), strings.Join(allProviders, ", "))
		}
	}

	if noLocal {
		filtered := make([]string, 0, len(selected))
		for _, p := range selected {
			if p != "local" {
				filtered = append(filtered, p)
			}
		}
		selected = filtered
	}

	if len(selected) == 0 {
		return nil, fmt.Errorf("no providers selected after applying flags")
	}

	return selected, nil
}

func parseFormats(s string) ([]string, error) {
	validFormats := map[string]struct{}{
		"all":  {},
		"html": {},
		"md":   {},
		"json": {},
	}

	input := strings.ToLower(strings.TrimSpace(s))
	if input == "" {
		return nil, fmt.Errorf("format cannot be empty (valid values: all, html, md, json)")
	}

	seen := make(map[string]struct{})
	formats := make([]string, 0, 4)
	for _, raw := range strings.Split(s, ",") {
		f := strings.ToLower(strings.TrimSpace(raw))
		if f == "" {
			return nil, fmt.Errorf("format list contains an empty entry")
		}
		if _, ok := validFormats[f]; !ok {
			return nil, fmt.Errorf("invalid format: %s (valid values: all, html, md, json)", f)
		}
		if _, ok := seen[f]; ok {
			continue
		}
		seen[f] = struct{}{}
		formats = append(formats, f)
	}

	if len(formats) == 1 && formats[0] == "all" {
		return formats, nil
	}

	if _, hasAll := seen["all"]; hasAll {
		return nil, fmt.Errorf("format 'all' cannot be combined with other formats")
	}

	return formats, nil
}

// ensureOutputDir creates a timestamped subdirectory for results
func ensureOutputDir(baseDir string) (string, error) {
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	sessionDir := filepath.Join(baseDir, timestamp)

	if err := os.MkdirAll(sessionDir, 0750); err != nil {
		return "", err
	}

	return sessionDir, nil
}

// filterTests filters tests based on no-search flag
func filterTests(tests []config.TestConfig, noSearch bool) []config.TestConfig {
	if !noSearch {
		return tests
	}

	var filtered []config.TestConfig
	for _, test := range tests {
		if test.Type != "search" {
			filtered = append(filtered, test)
		}
	}
	return filtered
}

func intPtr(v int) *int {
	return &v
}

// applyQuickMode modifies the configuration for quick testing
func applyQuickMode(cfg *config.Config) *config.Config {
	// Create a copy of the config
	quickCfg := &config.Config{
		General: config.GeneralConfig{
			Concurrency: cfg.General.Concurrency,
			Timeout:     "30s", // Increased from 20s to accommodate slower providers like Jina
			OutputDir:   cfg.General.OutputDir,
		},
		Tests: []config.TestConfig{},
	}

	// Select up to 3 tests: one of each type (search, extract, crawl)
	var hasSearch, hasExtract, hasCrawl bool
	for _, test := range cfg.Tests {
		if len(quickCfg.Tests) >= 3 {
			break
		}

		switch test.Type {
		case "search":
			if !hasSearch {
				quickCfg.Tests = append(quickCfg.Tests, test)
				hasSearch = true
			}
		case "extract":
			if !hasExtract {
				quickCfg.Tests = append(quickCfg.Tests, test)
				hasExtract = true
			}
		case "crawl":
			if !hasCrawl {
				// Reduce crawl parameters for quick mode
				quickTest := test
				if quickTest.MaxPages == nil || *quickTest.MaxPages > 2 {
					quickTest.MaxPages = intPtr(2)
				}
				if quickTest.MaxDepth == nil || *quickTest.MaxDepth > 1 {
					quickTest.MaxDepth = intPtr(1)
				}
				quickCfg.Tests = append(quickCfg.Tests, quickTest)
				hasCrawl = true
			}
		}
	}

	// If we couldn't find one of each type, just take the first available tests
	if len(quickCfg.Tests) == 0 && len(cfg.Tests) > 0 {
		maxTests := 3
		if len(cfg.Tests) < maxTests {
			maxTests = len(cfg.Tests)
		}
		for i := 0; i < maxTests; i++ {
			quickCfg.Tests = append(quickCfg.Tests, cfg.Tests[i])
		}
	}

	return quickCfg
}
