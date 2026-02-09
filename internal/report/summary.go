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

// formatCostUSD formats a cost value in USD for display
func formatCostUSD(cost float64) string {
	if cost == 0 {
		return "$0.00"
	}
	if cost < 0.01 {
		return fmt.Sprintf("$%.4f", cost)
	}
	return fmt.Sprintf("$%.2f", cost)
}

// writeComparisonTable writes the comparison table for all providers
func (g *Generator) writeComparisonTable(sb *strings.Builder, providers []string) {
	sb.WriteString("### Summary by Provider\n\n")
	sb.WriteString("| Provider | Tests | Success Rate | Avg Latency | Total Cost (USD) | Avg Content |\n")
	sb.WriteString("|----------|-------|--------------|-------------|------------------|-------------|\n")
	for _, provider := range providers {
		summary := g.collector.ComputeSummary(provider)
		successRate := float64(0)
		if summary.TotalTests > 0 {
			successRate = float64(summary.SuccessfulTests) / float64(summary.TotalTests) * 100
		}
		fmt.Fprintf(sb, "| %s | %d | %.1f%% | %s | %s | %.0f chars |\n",
			provider,
			summary.TotalTests,
			successRate,
			FormatLatency(summary.AvgLatency),
			formatCostUSD(summary.TotalCostUSD),
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

	// Cost ranking (by total USD cost - lower is better)
	sb.WriteString("**Cost (by total USD cost - lower is better):**\n")
	sortedByCost := make([]providerSummary, len(allSummaries))
	copy(sortedByCost, allSummaries)
	sort.Slice(sortedByCost, func(i, j int) bool {
		return sortedByCost[i].summary.TotalCostUSD < sortedByCost[j].summary.TotalCostUSD
	})
	for i, ps := range sortedByCost {
		fmt.Fprintf(sb, "%d. **%s**: %s\n", i+1, ps.name, formatCostUSD(ps.summary.TotalCostUSD))
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

	// Quality ranking (by avg quality score - higher is better) - only if quality scores exist
	hasQualityScores := false
	for _, ps := range allSummaries {
		if ps.summary.AvgQualityScore > 0 {
			hasQualityScores = true
			break
		}
	}
	if hasQualityScores {
		sb.WriteString("**Quality Score (by AI evaluation - higher is better):**\n")
		sortedByQuality := make([]providerSummary, 0, len(allSummaries))
		for _, ps := range allSummaries {
			if ps.summary.AvgQualityScore > 0 {
				sortedByQuality = append(sortedByQuality, ps)
			}
		}
		sort.Slice(sortedByQuality, func(i, j int) bool {
			return sortedByQuality[i].summary.AvgQualityScore > sortedByQuality[j].summary.AvgQualityScore
		})
		for i, ps := range sortedByQuality {
			fmt.Fprintf(sb, "%d. **%s**: %.1f/100\n", i+1, ps.name, ps.summary.AvgQualityScore)
		}
		sb.WriteString("\n")
	}
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

	sb.WriteString("**Cost Comparison (USD):**\n")
	if summary1.TotalCostUSD > 0 && summary2.TotalCostUSD > 0 {
		costDiff := (summary2.TotalCostUSD - summary1.TotalCostUSD) / summary1.TotalCostUSD * 100
		cheaper := providers[0]
		if summary2.TotalCostUSD < summary1.TotalCostUSD {
			cheaper = providers[1]
			costDiff = -costDiff
		}
		fmt.Fprintf(sb, "- **%s** is %.1f%% cheaper\n", cheaper, costDiff)
	}
	fmt.Fprintf(sb, "- %s total cost: %s\n", providers[0], formatCostUSD(summary1.TotalCostUSD))
	fmt.Fprintf(sb, "- %s total cost: %s\n\n", providers[1], formatCostUSD(summary2.TotalCostUSD))

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

	// Quality comparison if scores exist
	if summary1.AvgQualityScore > 0 && summary2.AvgQualityScore > 0 {
		sb.WriteString("\n**Quality Score Comparison:**\n")
		qualityDiff := summary2.AvgQualityScore - summary1.AvgQualityScore
		betterQuality := providers[0]
		if summary2.AvgQualityScore > summary1.AvgQualityScore {
			betterQuality = providers[1]
			qualityDiff = -qualityDiff
		}
		fmt.Fprintf(sb, "- **%s** has %.1f points higher quality score\n", betterQuality, qualityDiff)
		fmt.Fprintf(sb, "- %s avg quality: %.1f/100\n", providers[0], summary1.AvgQualityScore)
		fmt.Fprintf(sb, "- %s avg quality: %.1f/100\n", providers[1], summary2.AvgQualityScore)
	}
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
	sb.WriteString("| Provider | Tests | Success Rate | Avg Latency | Total Cost (USD) | Avg Content |\n")
	sb.WriteString("|----------|-------|--------------|-------------|------------------|-------------|\n")

	for _, provider := range providers {
		summary := g.collector.ComputeSummary(provider)
		successRate := float64(summary.SuccessfulTests) / float64(summary.TotalTests) * 100
		sb.WriteString(fmt.Sprintf("| %s | %d | %.1f%% | %s | %s | %.0f chars |\n",
			provider,
			summary.TotalTests,
			successRate,
			FormatLatency(summary.AvgLatency),
			formatCostUSD(summary.TotalCostUSD),
			summary.AvgContentLength,
		))
	}

	sb.WriteString("\n")

	// Detailed results by test
	sb.WriteString("## Detailed Results by Test\n\n")

	// Check if any results have quality scores
	hasQualityScores := false
	for _, provider := range providers {
		results := g.collector.GetResultsByProvider(provider)
		for _, r := range results {
			if r.QualityScore > 0 {
				hasQualityScores = true
				break
			}
		}
		if hasQualityScores {
			break
		}
	}

	tests := g.collector.GetAllTests()
	for _, testName := range tests {
		sb.WriteString(fmt.Sprintf("### %s\n\n", testName))

		// Use appropriate headers based on whether quality scores exist
		if hasQualityScores {
			sb.WriteString("| Provider | Status | Latency | Cost (USD) | Details | Quality |\n")
			sb.WriteString("|----------|--------|---------|------------|---------|---------|\n")
		} else {
			sb.WriteString("| Provider | Status | Latency | Cost (USD) | Details |\n")
			sb.WriteString("|----------|--------|---------|------------|---------|\n")
		}

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

			if hasQualityScores {
				qualityStr := "-"
				if r.QualityScore > 0 {
					qualityStr = fmt.Sprintf("%.1f", r.QualityScore)
				}
				sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s | %s |\n",
					r.Provider,
					status,
					FormatLatency(r.Latency),
					formatCostUSD(r.CostUSD),
					details,
					qualityStr,
				))
			} else {
				sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s |\n",
					r.Provider,
					status,
					FormatLatency(r.Latency),
					formatCostUSD(r.CostUSD),
					details,
				))
			}
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
