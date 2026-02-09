// Package report generates HTML, Markdown, and JSON reports from benchmark results.
package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/lamim/search-api-bench/internal/metrics"
)

// FormatLatency formats a duration as milliseconds for consistent comparison.
func FormatLatency(d time.Duration) string {
	return fmt.Sprintf("%dms", d.Milliseconds())
}

// Generator creates reports from benchmark results
type Generator struct {
	collector *metrics.Collector
	outputDir string
}

// NewGenerator creates a new report generator
func NewGenerator(collector *metrics.Collector, outputDir string) *Generator {
	return &Generator{
		collector: collector,
		outputDir: outputDir,
	}
}

// GenerateAll generates all report formats
func (g *Generator) GenerateAll() error {
	if err := g.GenerateMarkdown(); err != nil {
		return fmt.Errorf("failed to generate markdown report: %w", err)
	}
	if err := g.GenerateJSON(); err != nil {
		return fmt.Errorf("failed to generate JSON report: %w", err)
	}
	if err := g.GenerateHTML(); err != nil {
		return fmt.Errorf("failed to generate HTML report: %w", err)
	}
	return nil
}

// providerSummary wraps a provider name with its summary metrics
type providerSummary struct {
	name    string
	summary *metrics.Summary
}

// writeComparisonTable writes the comparison table for all providers
func (g *Generator) writeComparisonTable(sb *strings.Builder, providers []string) {
	sb.WriteString("### Summary by Provider\n\n")
	sb.WriteString("| Provider | Tests | Success Rate | Avg Latency | Total Credits | Avg Content |\n")
	sb.WriteString("|----------|-------|--------------|-------------|---------------|-------------|\n")
	for _, provider := range providers {
		summary := g.collector.ComputeSummary(provider)
		successRate := float64(0)
		if summary.TotalTests > 0 {
			successRate = float64(summary.SuccessfulTests) / float64(summary.TotalTests) * 100
		}
		fmt.Fprintf(sb, "| %s | %d | %.1f%% | %s | %d | %.0f chars |\n",
			provider,
			summary.TotalTests,
			successRate,
			FormatLatency(summary.AvgLatency),
			summary.TotalCreditsUsed,
			summary.AvgContentLength,
		)
	}
	sb.WriteString("\n")
}

// writeRankings writes the rankings section for speed, cost, and content
func (g *Generator) writeRankings(sb *strings.Builder, providers []string) {
	if len(providers) < 2 {
		return
	}

	// Collect summaries
	allSummaries := make([]providerSummary, 0, len(providers))
	for _, provider := range providers {
		allSummaries = append(allSummaries, providerSummary{
			name:    provider,
			summary: g.collector.ComputeSummary(provider),
		})
	}

	sb.WriteString("### Rankings\n\n")

	// Speed ranking (by avg latency - lower is better)
	sb.WriteString("**Speed (by avg latency - lower is better):**\n")
	sortedBySpeed := make([]providerSummary, len(allSummaries))
	copy(sortedBySpeed, allSummaries)
	sort.Slice(sortedBySpeed, func(i, j int) bool {
		return sortedBySpeed[i].summary.AvgLatency < sortedBySpeed[j].summary.AvgLatency
	})
	for i, ps := range sortedBySpeed {
		fmt.Fprintf(sb, "%d. **%s**: %s\n", i+1, ps.name, FormatLatency(ps.summary.AvgLatency))
	}
	sb.WriteString("\n")

	// Cost ranking (by total credits used - lower is better)
	sb.WriteString("**Cost (by total credits - lower is better):**\n")
	sortedByCost := make([]providerSummary, len(allSummaries))
	copy(sortedByCost, allSummaries)
	sort.Slice(sortedByCost, func(i, j int) bool {
		return sortedByCost[i].summary.TotalCreditsUsed < sortedByCost[j].summary.TotalCreditsUsed
	})
	for i, ps := range sortedByCost {
		fmt.Fprintf(sb, "%d. **%s**: %d credits\n", i+1, ps.name, ps.summary.TotalCreditsUsed)
	}
	sb.WriteString("\n")

	// Content ranking (by avg content length - higher is better)
	sb.WriteString("**Content Volume (by avg chars - higher is better):**\n")
	sortedByContent := make([]providerSummary, len(allSummaries))
	copy(sortedByContent, allSummaries)
	sort.Slice(sortedByContent, func(i, j int) bool {
		return sortedByContent[i].summary.AvgContentLength > sortedByContent[j].summary.AvgContentLength
	})
	for i, ps := range sortedByContent {
		fmt.Fprintf(sb, "%d. **%s**: %.0f chars\n", i+1, ps.name, ps.summary.AvgContentLength)
	}
	sb.WriteString("\n")
}

// writePairwiseComparison writes the detailed comparison for exactly 2 providers
func (g *Generator) writePairwiseComparison(sb *strings.Builder, providers []string) {
	if len(providers) != 2 {
		return
	}

	summary1 := g.collector.ComputeSummary(providers[0])
	summary2 := g.collector.ComputeSummary(providers[1])

	sb.WriteString("### Detailed Pairwise Comparison\n\n")

	sb.WriteString("**Speed Comparison:**\n")
	if summary1.AvgLatency > 0 && summary2.AvgLatency > 0 {
		speedDiff := float64(summary2.AvgLatency-summary1.AvgLatency) / float64(summary1.AvgLatency) * 100
		faster := providers[0]
		if summary2.AvgLatency < summary1.AvgLatency {
			faster = providers[1]
			speedDiff = -speedDiff
		}
		fmt.Fprintf(sb, "- **%s** is %.1f%% faster on average\n", faster, speedDiff)
	}
	fmt.Fprintf(sb, "- %s avg latency: %s\n", providers[0], FormatLatency(summary1.AvgLatency))
	fmt.Fprintf(sb, "- %s avg latency: %s\n\n", providers[1], FormatLatency(summary2.AvgLatency))

	sb.WriteString("**Cost Comparison:**\n")
	if summary1.TotalCreditsUsed > 0 && summary2.TotalCreditsUsed > 0 {
		costDiff := float64(summary2.TotalCreditsUsed-summary1.TotalCreditsUsed) / float64(summary1.TotalCreditsUsed) * 100
		cheaper := providers[0]
		if summary2.TotalCreditsUsed < summary1.TotalCreditsUsed {
			cheaper = providers[1]
			costDiff = -costDiff
		}
		fmt.Fprintf(sb, "- **%s** uses %.1f%% fewer credits\n", cheaper, costDiff)
	}
	fmt.Fprintf(sb, "- %s total credits: %d\n", providers[0], summary1.TotalCreditsUsed)
	fmt.Fprintf(sb, "- %s total credits: %d\n\n", providers[1], summary2.TotalCreditsUsed)

	sb.WriteString("**Content Volume Comparison:**\n")
	if summary1.AvgContentLength > 0 && summary2.AvgContentLength > 0 {
		contentDiff := (summary2.AvgContentLength - summary1.AvgContentLength) / summary1.AvgContentLength * 100
		moreContent := providers[0]
		if summary2.AvgContentLength > summary1.AvgContentLength {
			moreContent = providers[1]
			contentDiff = -contentDiff
		}
		fmt.Fprintf(sb, "- **%s** returns %.1f%% more content on average\n", moreContent, contentDiff)
	}
	fmt.Fprintf(sb, "- %s avg content: %.0f chars\n", providers[0], summary1.AvgContentLength)
	fmt.Fprintf(sb, "- %s avg content: %.0f chars\n", providers[1], summary2.AvgContentLength)
}

// GenerateMarkdown creates a markdown summary report
func (g *Generator) GenerateMarkdown() error {
	providers := g.collector.GetAllProviders()
	timestamp := time.Now().Format("2006-01-02 15:04:05")

	var sb strings.Builder
	sb.WriteString("# Search API Benchmark Report\n\n")
	sb.WriteString(fmt.Sprintf("**Generated:** %s\n\n", timestamp))

	// Overview table
	sb.WriteString("## Summary\n\n")
	sb.WriteString("| Provider | Tests | Success Rate | Avg Latency | Total Credits | Avg Content |\n")
	sb.WriteString("|----------|-------|--------------|-------------|---------------|-------------|\n")

	for _, provider := range providers {
		summary := g.collector.ComputeSummary(provider)
		successRate := float64(summary.SuccessfulTests) / float64(summary.TotalTests) * 100
		sb.WriteString(fmt.Sprintf("| %s | %d | %.1f%% | %s | %d | %.0f chars |\n",
			provider,
			summary.TotalTests,
			successRate,
			FormatLatency(summary.AvgLatency),
			summary.TotalCreditsUsed,
			summary.AvgContentLength,
		))
	}

	sb.WriteString("\n")

	// Detailed results by test
	sb.WriteString("## Detailed Results by Test\n\n")

	tests := g.collector.GetAllTests()
	for _, testName := range tests {
		sb.WriteString(fmt.Sprintf("### %s\n\n", testName))
		sb.WriteString("| Provider | Status | Latency | Credits | Details |\n")
		sb.WriteString("|----------|--------|---------|---------|---------|\n")

		results := g.collector.GetResultsByTest(testName)
		for _, r := range results {
			status := "✓ Pass"
			if !r.Success {
				status = "✗ Fail"
			}

			details := ""
			switch r.TestType {
			case "search":
				details = fmt.Sprintf("%d results", r.ResultsCount)
			case "extract":
				details = fmt.Sprintf("%d chars", r.ContentLength)
			case "crawl":
				details = fmt.Sprintf("%d pages, %d chars", r.ResultsCount, r.ContentLength)
			}

			sb.WriteString(fmt.Sprintf("| %s | %s | %s | %d | %s |\n",
				r.Provider,
				status,
				FormatLatency(r.Latency),
				r.CreditsUsed,
				details,
			))
		}
		sb.WriteString("\n")
	}

	// Provider comparison
	sb.WriteString("## Provider Comparison\n\n")

	// Show summary table for all providers
	g.writeComparisonTable(&sb, providers)

	// Rankings (for 2+ providers)
	g.writeRankings(&sb, providers)

	// Pairwise comparison for exactly 2 providers (original detailed comparison)
	g.writePairwiseComparison(&sb, providers)

	// Write file
	outputPath := filepath.Join(g.outputDir, "report.md")
	// #nosec G306 - 0640 allows owner/group to read, which is appropriate for report files
	return os.WriteFile(outputPath, []byte(sb.String()), 0640)
}

// GenerateJSON creates a JSON report with raw data
func (g *Generator) GenerateJSON() error {
	data := map[string]interface{}{
		"timestamp": time.Now(),
		"providers": g.collector.GetAllProviders(),
		"tests":     g.collector.GetAllTests(),
		"results":   g.collector.GetResults(),
	}

	// Add summaries
	summaries := make(map[string]*metrics.Summary)
	for _, provider := range g.collector.GetAllProviders() {
		summaries[provider] = g.collector.ComputeSummary(provider)
	}
	data["summaries"] = summaries

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	outputPath := filepath.Join(g.outputDir, "report.json")
	// #nosec G306 - 0640 allows owner/group to read, which is appropriate for report files
	return os.WriteFile(outputPath, jsonData, 0640)
}
