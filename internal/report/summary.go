// Package report generates HTML, Markdown, and JSON reports from benchmark results.
package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lamim/search-api-bench/internal/metrics"
)

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
		sb.WriteString(fmt.Sprintf("| %s | %d | %.1f%% | %v | %d | %.0f chars |\n",
			provider,
			summary.TotalTests,
			successRate,
			summary.AvgLatency.Round(time.Millisecond),
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
			status := "✅ Pass"
			if !r.Success {
				status = "❌ Fail"
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

			sb.WriteString(fmt.Sprintf("| %s | %s | %v | %d | %s |\n",
				r.Provider,
				status,
				r.Latency.Round(time.Millisecond),
				r.CreditsUsed,
				details,
			))
		}
		sb.WriteString("\n")
	}

	// Provider comparison
	sb.WriteString("## Provider Comparison\n\n")

	if len(providers) == 2 {
		summary1 := g.collector.ComputeSummary(providers[0])
		summary2 := g.collector.ComputeSummary(providers[1])

		sb.WriteString("### Speed Comparison\n\n")
		if summary1.AvgLatency > 0 && summary2.AvgLatency > 0 {
			speedDiff := float64(summary2.AvgLatency-summary1.AvgLatency) / float64(summary1.AvgLatency) * 100
			faster := providers[0]
			if summary2.AvgLatency < summary1.AvgLatency {
				faster = providers[1]
				speedDiff = -speedDiff
			}
			sb.WriteString(fmt.Sprintf("- **%s** is %.1f%% faster on average\n", faster, speedDiff))
		}
		sb.WriteString(fmt.Sprintf("- %s avg latency: %v\n", providers[0], summary1.AvgLatency.Round(time.Millisecond)))
		sb.WriteString(fmt.Sprintf("- %s avg latency: %v\n\n", providers[1], summary2.AvgLatency.Round(time.Millisecond)))

		sb.WriteString("### Cost Comparison\n\n")
		if summary1.TotalCreditsUsed > 0 && summary2.TotalCreditsUsed > 0 {
			costDiff := float64(summary2.TotalCreditsUsed-summary1.TotalCreditsUsed) / float64(summary1.TotalCreditsUsed) * 100
			cheaper := providers[0]
			if summary2.TotalCreditsUsed < summary1.TotalCreditsUsed {
				cheaper = providers[1]
				costDiff = -costDiff
			}
			sb.WriteString(fmt.Sprintf("- **%s** uses %.1f%% fewer credits\n", cheaper, costDiff))
		}
		sb.WriteString(fmt.Sprintf("- %s total credits: %d\n", providers[0], summary1.TotalCreditsUsed))
		sb.WriteString(fmt.Sprintf("- %s total credits: %d\n\n", providers[1], summary2.TotalCreditsUsed))

		sb.WriteString("### Content Volume Comparison\n\n")
		if summary1.AvgContentLength > 0 && summary2.AvgContentLength > 0 {
			contentDiff := (summary2.AvgContentLength - summary1.AvgContentLength) / summary1.AvgContentLength * 100
			moreContent := providers[0]
			if summary2.AvgContentLength > summary1.AvgContentLength {
				moreContent = providers[1]
				contentDiff = -contentDiff
			}
			sb.WriteString(fmt.Sprintf("- **%s** returns %.1f%% more content on average\n", moreContent, contentDiff))
		}
		sb.WriteString(fmt.Sprintf("- %s avg content: %.0f chars\n", providers[0], summary1.AvgContentLength))
		sb.WriteString(fmt.Sprintf("- %s avg content: %.0f chars\n", providers[1], summary2.AvgContentLength))
	}

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
