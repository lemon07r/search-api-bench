// Package main provides the entry point for the search API benchmark tool.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/lamim/search-api-bench/internal/config"
	"github.com/lamim/search-api-bench/internal/evaluator"
	"github.com/lamim/search-api-bench/internal/metrics"
	"github.com/lamim/search-api-bench/internal/providers"
	"github.com/lamim/search-api-bench/internal/providers/firecrawl"
	"github.com/lamim/search-api-bench/internal/providers/tavily"
	"github.com/lamim/search-api-bench/internal/report"
)

func main() {
	var (
		configPath   = flag.String("config", "config.toml", "Path to configuration file")
		outputDir    = flag.String("output", "", "Output directory for reports (overrides config)")
		providersFlag = flag.String("providers", "all", "Providers to test: all, firecrawl, tavily")
		format       = flag.String("format", "all", "Report format: all, html, md, json")
	)
	flag.Parse()

	// Load .env file if present
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
				// Remove quotes if present
				value = strings.Trim(value, `"'`)
				_ = os.Setenv(key, value)
			}
		}
	}

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Override output dir if provided
	if *outputDir != "" {
		cfg.General.OutputDir = *outputDir
	}

	// Print banner
	printBanner()

	// Initialize providers
	var provs []providers.Provider
	providerNames := parseProviders(*providersFlag)

	for _, name := range providerNames {
		switch name {
		case "firecrawl":
			client, err := firecrawl.NewClient()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to initialize Firecrawl: %v\n", err)
				continue
			}
			provs = append(provs, client)
			fmt.Printf("✓ Initialized Firecrawl provider\n")

		case "tavily":
			client, err := tavily.NewClient()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to initialize Tavily: %v\n", err)
				continue
			}
			provs = append(provs, client)
			fmt.Printf("✓ Initialized Tavily provider\n")
		}
	}

	if len(provs) == 0 {
		fmt.Fprintf(os.Stderr, "Error: no providers available. Please check your API keys.\n")
		os.Exit(1)
	}

	// Create runner
	runner := evaluator.NewRunner(cfg, provs)

	// Ensure output directory exists
	if err := runner.EnsureOutputDir(); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating output directory: %v\n", err)
		os.Exit(1)
	}

	// Run benchmarks
	ctx := context.Background()
	if err := runner.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error running benchmarks: %v\n", err)
		os.Exit(1)
	}

	// Generate reports
	fmt.Println("\nGenerating reports...")
	gen := report.NewGenerator(runner.GetCollector(), cfg.General.OutputDir)

	formats := parseFormats(*format)
	for _, f := range formats {
		switch f {
		case "html":
			if err := gen.GenerateHTML(); err != nil {
				fmt.Fprintf(os.Stderr, "Error generating HTML report: %v\n", err)
			} else {
				fmt.Printf("✓ Generated HTML report: %s/report.html\n", cfg.General.OutputDir)
			}
		case "md":
			if err := gen.GenerateMarkdown(); err != nil {
				fmt.Fprintf(os.Stderr, "Error generating Markdown report: %v\n", err)
			} else {
				fmt.Printf("✓ Generated Markdown report: %s/report.md\n", cfg.General.OutputDir)
			}
		case "json":
			if err := gen.GenerateJSON(); err != nil {
				fmt.Fprintf(os.Stderr, "Error generating JSON report: %v\n", err)
			} else {
				fmt.Printf("✓ Generated JSON report: %s/report.json\n", cfg.General.OutputDir)
			}
		case "all":
			if err := gen.GenerateAll(); err != nil {
				fmt.Fprintf(os.Stderr, "Error generating reports: %v\n", err)
			} else {
				fmt.Printf("✓ Generated all reports in: %s/\n", cfg.General.OutputDir)
			}
		}
	}

	// Print summary
	printSummary(runner.GetCollector())
}

func printBanner() {
	fmt.Println(`
╔══════════════════════════════════════════════════════════════╗
║               Search API Benchmark Tool                      ║
║         Compare Firecrawl vs Tavily Performance              ║
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
		fmt.Printf("  Avg Latency: %v\n", summary.AvgLatency)
		fmt.Printf("  Total Credits: %d\n", summary.TotalCreditsUsed)
		fmt.Printf("  Avg Content: %.0f chars\n", summary.AvgContentLength)
	}

	fmt.Println("\nReports generated successfully!")
	fmt.Println("View detailed results in the output directory.")
}

func parseProviders(s string) []string {
	if s == "all" {
		return []string{"firecrawl", "tavily"}
	}
	return strings.Split(s, ",")
}

func parseFormats(s string) []string {
	if s == "all" {
		return []string{"all"}
	}
	return strings.Split(s, ",")
}


