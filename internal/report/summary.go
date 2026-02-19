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

	"github.com/lamim/sanity-web-eval/internal/metrics"
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

type testTypeQualityStats struct {
	AvgQuality    float64 `json:"avg_quality"`
	ScoredTests   int     `json:"scored_tests"`
	ExecutedTests int     `json:"executed_tests"`
}

func (s testTypeQualityStats) CoveragePct() float64 {
	if s.ExecutedTests == 0 {
		return 0
	}
	return float64(s.ScoredTests) / float64(s.ExecutedTests) * 100
}

type providerQualityByTestType struct {
	Search             testTypeQualityStats `json:"search"`
	SearchSemantic     testTypeQualityStats `json:"search_semantic"`
	SearchReranker     testTypeQualityStats `json:"search_reranker"`
	SearchComponentAvg testTypeQualityStats `json:"search_component_avg"`
	Extract            testTypeQualityStats `json:"extract"`
	Crawl              testTypeQualityStats `json:"crawl"`
}

// formatCostUSD formats a cost value in USD for display
func formatCostUSD(cost float64) string {
	if cost == 0 {
		return "$0.00"
	}
	if cost > 0 && cost < 0.0001 {
		return "<$0.0001"
	}
	if cost < 0.01 {
		return fmt.Sprintf("$%.4f", cost)
	}
	return fmt.Sprintf("$%.2f", cost)
}

func (g *Generator) computeProviderQualityByTestType(provider string) providerQualityByTestType {
	results := g.collector.GetResultsByProvider(provider)
	typeAccumulator := map[string]*testTypeQualityStats{
		"search":  {},
		"extract": {},
		"crawl":   {},
	}
	typeTotals := map[string]float64{
		"search":  0,
		"extract": 0,
		"crawl":   0,
	}
	searchSemantic := testTypeQualityStats{}
	searchReranker := testTypeQualityStats{}
	searchComponent := testTypeQualityStats{}
	searchSemanticTotal := 0.0
	searchRerankerTotal := 0.0
	searchComponentTotal := 0.0

	for _, r := range results {
		acc, ok := typeAccumulator[r.TestType]
		if !ok || r.Skipped {
			continue
		}

		acc.ExecutedTests++
		if r.QualityScored || r.QualityScore > 0 {
			acc.ScoredTests++
			typeTotals[r.TestType] += r.QualityScore
		}

		if r.TestType != "search" {
			continue
		}

		searchSemantic.ExecutedTests++
		searchReranker.ExecutedTests++
		searchComponent.ExecutedTests++

		componentCount := 0
		componentTotal := 0.0

		if r.SemanticScore > 0 {
			searchSemantic.ScoredTests++
			searchSemanticTotal += r.SemanticScore
			componentCount++
			componentTotal += r.SemanticScore
		}
		if r.RerankerScore > 0 {
			searchReranker.ScoredTests++
			searchRerankerTotal += r.RerankerScore
			componentCount++
			componentTotal += r.RerankerScore
		}
		if componentCount > 0 {
			searchComponent.ScoredTests++
			searchComponentTotal += componentTotal / float64(componentCount)
		}
	}

	for testType, acc := range typeAccumulator {
		if acc.ScoredTests > 0 {
			acc.AvgQuality = typeTotals[testType] / float64(acc.ScoredTests)
		}
	}
	if searchSemantic.ScoredTests > 0 {
		searchSemantic.AvgQuality = searchSemanticTotal / float64(searchSemantic.ScoredTests)
	}
	if searchReranker.ScoredTests > 0 {
		searchReranker.AvgQuality = searchRerankerTotal / float64(searchReranker.ScoredTests)
	}
	if searchComponent.ScoredTests > 0 {
		searchComponent.AvgQuality = searchComponentTotal / float64(searchComponent.ScoredTests)
	}

	return providerQualityByTestType{
		Search:             *typeAccumulator["search"],
		SearchSemantic:     searchSemantic,
		SearchReranker:     searchReranker,
		SearchComponentAvg: searchComponent,
		Extract:            *typeAccumulator["extract"],
		Crawl:              *typeAccumulator["crawl"],
	}
}

func formatQualityValue(stats testTypeQualityStats) string {
	if stats.ScoredTests == 0 {
		return "-"
	}
	return fmt.Sprintf("%.1f", stats.AvgQuality)
}

func formatQualityCoverage(stats testTypeQualityStats) string {
	if stats.ExecutedTests == 0 {
		return "N/A"
	}
	return fmt.Sprintf("%.1f%% (%d/%d)", stats.CoveragePct(), stats.ScoredTests, stats.ExecutedTests)
}

func scoreLabelForTestType(testType string) string {
	switch testType {
	case "search":
		return "Search Relevance (Model-Assisted)"
	case "extract":
		return "Extraction Heuristic"
	case "crawl":
		return "Crawl Heuristic"
	default:
		return "-"
	}
}

func (g *Generator) writeQualityByTestType(sb *strings.Builder, providers []string) {
	sb.WriteString("### Scoring by Test Type\n\n")
	sb.WriteString("_Search relevance uses model-assisted signals; extract and crawl scores are rule-based heuristics._\n\n")
	sb.WriteString("| Provider | Search Relevance | Search Coverage | Semantic | Semantic Coverage | Reranker | Reranker Coverage | Search Component Avg | Component Coverage | Extract Heuristic | Extract Coverage | Crawl Heuristic | Crawl Coverage |\n")
	sb.WriteString("|----------|------------------|-----------------|----------|-------------------|----------|-------------------|----------------------|--------------------|-------------------|------------------|-----------------|---------------|\n")

	for _, provider := range providers {
		byType := g.computeProviderQualityByTestType(provider)
		fmt.Fprintf(
			sb,
			"| %s | %s | %s | %s | %s | %s | %s | %s | %s | %s | %s | %s | %s |\n",
			provider,
			formatQualityValue(byType.Search),
			formatQualityCoverage(byType.Search),
			formatQualityValue(byType.SearchSemantic),
			formatQualityCoverage(byType.SearchSemantic),
			formatQualityValue(byType.SearchReranker),
			formatQualityCoverage(byType.SearchReranker),
			formatQualityValue(byType.SearchComponentAvg),
			formatQualityCoverage(byType.SearchComponentAvg),
			formatQualityValue(byType.Extract),
			formatQualityCoverage(byType.Extract),
			formatQualityValue(byType.Crawl),
			formatQualityCoverage(byType.Crawl),
		)
	}
	sb.WriteString("\n")
}

// writeComparisonTable writes the comparison table for all providers
func (g *Generator) writeComparisonTable(sb *strings.Builder, providers []string) {
	sb.WriteString("### Summary by Provider\n\n")
	sb.WriteString("| Provider | Tests | Executed | Primary Comparable | Skipped | Excluded Primary | Success Rate | Primary Success | Avg Latency | Total Cost (USD) | Avg Content | Scoring Coverage |\n")
	sb.WriteString("|----------|-------|----------|--------------------|---------|------------------|--------------|-----------------|-------------|------------------|-------------|------------------|\n")
	for _, provider := range providers {
		summary := g.collector.ComputeSummary(provider)
		fmt.Fprintf(sb, "| %s | %d | %d | %d | %d | %d | %.1f%% | %.1f%% | %s | %s | %.0f chars | %.1f%% |\n",
			provider,
			summary.TotalTests,
			summary.ExecutedTests,
			summary.PrimaryComparableTests,
			summary.SkippedTests,
			summary.ExcludedFromPrimary,
			summary.SuccessRate,
			summary.PrimaryComparableSuccessRate,
			FormatLatency(summary.AvgLatency),
			formatCostUSD(summary.TotalCostUSD),
			summary.AvgContentLength,
			summary.QualityCoveragePct,
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

	g.writeScoreRanking(
		sb,
		"**Search Relevance (model-assisted, search tests only):**\n",
		providers,
		func(stats providerQualityByTestType) testTypeQualityStats { return stats.Search },
	)
	g.writeScoreRanking(
		sb,
		"**Extraction Heuristic (extract tests only):**\n",
		providers,
		func(stats providerQualityByTestType) testTypeQualityStats { return stats.Extract },
	)
	g.writeScoreRanking(
		sb,
		"**Crawl Heuristic (crawl tests only):**\n",
		providers,
		func(stats providerQualityByTestType) testTypeQualityStats { return stats.Crawl },
	)
}

type scoreStatsSelector func(providerQualityByTestType) testTypeQualityStats

type scoreRankingEntry struct {
	name  string
	stats testTypeQualityStats
}

func (g *Generator) writeScoreRanking(sb *strings.Builder, heading string, providers []string, selector scoreStatsSelector) {
	entries := make([]scoreRankingEntry, 0, len(providers))
	for _, provider := range providers {
		stats := selector(g.computeProviderQualityByTestType(provider))
		if stats.ScoredTests == 0 {
			continue
		}
		entries = append(entries, scoreRankingEntry{
			name:  provider,
			stats: stats,
		})
	}
	if len(entries) == 0 {
		return
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].stats.AvgQuality > entries[j].stats.AvgQuality
	})

	sb.WriteString(heading)
	for i, entry := range entries {
		fmt.Fprintf(
			sb,
			"%d. **%s**: %.1f/100 (coverage %.1f%%, scored %d/%d)\n",
			i+1,
			entry.name,
			entry.stats.AvgQuality,
			entry.stats.CoveragePct(),
			entry.stats.ScoredTests,
			entry.stats.ExecutedTests,
		)
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
		}
		if speedDiff < 0 {
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
		}
		if costDiff < 0 {
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
		}
		if contentDiff < 0 {
			contentDiff = -contentDiff
		}
		fmt.Fprintf(sb, "- **%s** returns %.1f%% more content on average\n", moreContent, contentDiff)
	}
	fmt.Fprintf(sb, "- %s avg content: %.0f chars\n", providers[0], summary1.AvgContentLength)
	fmt.Fprintf(sb, "- %s avg content: %.0f chars\n", providers[1], summary2.AvgContentLength)

	byType1 := g.computeProviderQualityByTestType(providers[0])
	byType2 := g.computeProviderQualityByTestType(providers[1])
	type pairwiseScore struct {
		label string
		a     testTypeQualityStats
		b     testTypeQualityStats
	}
	comparisons := []pairwiseScore{
		{label: "Search Relevance (model-assisted)", a: byType1.Search, b: byType2.Search},
		{label: "Extraction Heuristic", a: byType1.Extract, b: byType2.Extract},
		{label: "Crawl Heuristic", a: byType1.Crawl, b: byType2.Crawl},
	}
	hasAnyScoreComparison := false
	for _, comp := range comparisons {
		if comp.a.ScoredTests == 0 || comp.b.ScoredTests == 0 {
			continue
		}
		if !hasAnyScoreComparison {
			sb.WriteString("\n**Scoring Comparison by Test Type:**\n")
			hasAnyScoreComparison = true
		}
		scoreDiff := comp.b.AvgQuality - comp.a.AvgQuality
		better := providers[0]
		if comp.b.AvgQuality > comp.a.AvgQuality {
			better = providers[1]
		}
		if scoreDiff < 0 {
			scoreDiff = -scoreDiff
		}
		fmt.Fprintf(sb, "- %s: **%s** leads by %.1f points\n", comp.label, better, scoreDiff)
		fmt.Fprintf(sb, "  %s: %.1f/100 (%d/%d scored)\n", providers[0], comp.a.AvgQuality, comp.a.ScoredTests, comp.a.ExecutedTests)
		fmt.Fprintf(sb, "  %s: %.1f/100 (%d/%d scored)\n", providers[1], comp.b.AvgQuality, comp.b.ScoredTests, comp.b.ExecutedTests)
	}
}

// GenerateMarkdown creates a markdown summary report
func (g *Generator) GenerateMarkdown() error {
	providers := g.collector.GetAllProviders()
	timestamp := time.Now().Format("2006-01-02 15:04:05")

	var sb strings.Builder
	sb.WriteString("# SanityWebEval Report\n\n")
	sb.WriteString(fmt.Sprintf("**Generated:** %s\n\n", timestamp))

	// Overview table
	sb.WriteString("## Summary\n\n")
	sb.WriteString("| Provider | Tests | Executed | Skipped | Success Rate | Avg Latency | Total Cost (USD) | Avg Content | Scoring Coverage |\n")
	sb.WriteString("|----------|-------|----------|---------|--------------|-------------|------------------|-------------|------------------|\n")

	for _, provider := range providers {
		summary := g.collector.ComputeSummary(provider)
		sb.WriteString(fmt.Sprintf("| %s | %d | %d | %d | %.1f%% | %s | %s | %.0f chars | %.1f%% |\n",
			provider,
			summary.TotalTests,
			summary.ExecutedTests,
			summary.SkippedTests,
			summary.SuccessRate,
			FormatLatency(summary.AvgLatency),
			formatCostUSD(summary.TotalCostUSD),
			summary.AvgContentLength,
			summary.QualityCoveragePct,
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
			if r.QualityScored || r.QualityScore > 0 {
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
			sb.WriteString("| Provider | Status | Latency | Cost (USD) | Details | Score Family | Score | Semantic (Search) | Reranker (Search) |\n")
			sb.WriteString("|----------|--------|---------|------------|---------|--------------|-------|-------------------|-------------------|\n")
		} else {
			sb.WriteString("| Provider | Status | Latency | Cost (USD) | Details |\n")
			sb.WriteString("|----------|--------|---------|------------|---------|\n")
		}

		results := g.collector.GetResultsByTest(testName)
		for _, r := range results {
			status := "✓ Pass"
			if r.Skipped {
				status = "⊘ Skip"
			} else if !r.Success {
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
				if r.QualityScored || r.QualityScore > 0 {
					qualityStr = fmt.Sprintf("%.1f", r.QualityScore)
				}
				semanticStr := "-"
				if r.SemanticScore > 0 {
					semanticStr = fmt.Sprintf("%.1f", r.SemanticScore)
				}
				rerankerStr := "-"
				if r.RerankerScore > 0 {
					rerankerStr = fmt.Sprintf("%.1f", r.RerankerScore)
				}
				sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s | %s | %s | %s | %s |\n",
					r.Provider,
					status,
					FormatLatency(r.Latency),
					formatCostUSD(r.CostUSD),
					details,
					scoreLabelForTestType(r.TestType),
					qualityStr,
					semanticStr,
					rerankerStr,
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
	g.writeQualityByTestType(&sb, providers)

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
		"schema_version": "v2",
		"timestamp":      time.Now(),
		"providers":      g.collector.GetAllProviders(),
		"tests":          g.collector.GetAllTests(),
		"results":        g.collector.GetResults(),
	}

	// Add summaries
	summaries := make(map[string]*metrics.Summary)
	qualityByTestType := make(map[string]providerQualityByTestType)
	for _, provider := range g.collector.GetAllProviders() {
		summaries[provider] = g.collector.ComputeSummary(provider)
		qualityByTestType[provider] = g.computeProviderQualityByTestType(provider)
	}
	data["summaries"] = summaries
	data["scoring_by_test_type"] = qualityByTestType
	// Backward-compatible alias for existing downstream consumers.
	data["quality_by_test_type"] = qualityByTestType

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	outputPath := filepath.Join(g.outputDir, "report.json")
	// #nosec G306 - 0640 allows owner/group to read, which is appropriate for report files
	return os.WriteFile(outputPath, jsonData, 0640)
}
