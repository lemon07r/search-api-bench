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

	provs := initializeProviders(flags.providersFlag, *flags.noLocal, debugLogger)

	if len(provs) == 0 {
		fmt.Fprintf(os.Stderr, "Error: no providers available. Please check your API keys.\n")
		os.Exit(1)
	}

	// Calculate total tests
	totalTests := len(cfg.Tests) * len(provs)

	// Get provider names for progress display
	providerNames := make([]string, 0, len(provs))
	for _, p := range provs {
		providerNames = append(providerNames, p.Name())
	}

	// Create progress manager
	prog := progress.NewManager(totalTests, providerNames, !*flags.noProgress)

	// Create runner with progress manager and debug logger
	runner := evaluator.NewRunner(cfg, provs, prog, debugLogger)

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
	generateReports(flags.format, runner.GetCollector(), cfg.General.OutputDir)
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
		successRate := float64(summary.SuccessfulTests) / float64(summary.TotalTests) * 100

		fmt.Printf("\n%s:\n", strings.ToUpper(provider))
		fmt.Printf("  Tests: %d (%.1f%% success)\n", summary.TotalTests, successRate)
		fmt.Printf("  Avg Latency: %s\n", report.FormatLatency(summary.AvgLatency))
		fmt.Printf("  Total Credits: %d\n", summary.TotalCreditsUsed)
		fmt.Printf("  Avg Content: %.0f chars\n", summary.AvgContentLength)
	}

	fmt.Println("\nReports generated successfully!")
	fmt.Println("View detailed results in the output directory.")
}

func initializeProviders(providersFlag *string, noLocal bool, debugLogger *debug.Logger) []providers.Provider {
	var provs []providers.Provider
	providerNames := parseProviders(*providersFlag, noLocal)

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

func generateReports(formatFlag *string, collector *metrics.Collector, outputDir string) {
	fmt.Println("\nGenerating reports...")
	gen := report.NewGenerator(collector, outputDir)

	formats := parseFormats(*formatFlag)
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

func parseProviders(s string, noLocal bool) []string {
	var providers []string
	if s == "all" {
		providers = []string{"firecrawl", "tavily", "local", "brave", "exa", "mixedbread", "jina"}
	} else {
		providers = strings.Split(s, ",")
	}

	if noLocal {
		var filtered []string
		for _, p := range providers {
			if p != "local" {
				filtered = append(filtered, p)
			}
		}
		providers = filtered
	}

	return providers
}

func parseFormats(s string) []string {
	if s == "all" {
		return []string{"all"}
	}
	return strings.Split(s, ",")
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

// applyQuickMode modifies the configuration for quick testing
func applyQuickMode(cfg *config.Config) *config.Config {
	// Create a copy of the config
	quickCfg := &config.Config{
		General: config.GeneralConfig{
			Concurrency: cfg.General.Concurrency,
			Timeout:     "20s", // Shorter timeout for quick mode
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
				if quickTest.MaxPages > 2 {
					quickTest.MaxPages = 2
				}
				if quickTest.MaxDepth > 1 {
					quickTest.MaxDepth = 1
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
