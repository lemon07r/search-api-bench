// Package report generates enhanced quality reports
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

// QualityReportGenerator generates quality-focused reports
type QualityReportGenerator struct {
	collector *metrics.Collector
	outputDir string
}

// NewQualityReportGenerator creates a new quality report generator
func NewQualityReportGenerator(collector *metrics.Collector, outputDir string) *QualityReportGenerator {
	return &QualityReportGenerator{
		collector: collector,
		outputDir: outputDir,
	}
}

// GenerateQualityMarkdown creates a quality-focused markdown report
func (g *QualityReportGenerator) GenerateQualityMarkdown() error {
	providers := g.collector.GetAllProviders()
	timestamp := time.Now().Format("2006-01-02 15:04:05")

	var sb strings.Builder
	sb.WriteString("# Search API Quality Benchmark Report\n\n")
	sb.WriteString(fmt.Sprintf("**Generated:** %s\n\n", timestamp))
	sb.WriteString("This report focuses on quality metrics powered by AI-based evaluation.\n\n")

	// Quality score overview
	sb.WriteString("## Quality Score Overview\n\n")
	sb.WriteString("| Provider | Avg Quality | Min Quality | Max Quality | Success Rate |\n")
	sb.WriteString("|----------|-------------|-------------|-------------|---------------|\n")

	for _, provider := range providers {
		summary := g.collector.ComputeSummary(provider)
		sb.WriteString(fmt.Sprintf("| %s | %.1f | %.1f | %.1f | %.1f%% |\n",
			provider,
			summary.AvgQualityScore,
			summary.MinQualityScore,
			summary.MaxQualityScore,
			summary.SuccessRate,
		))
	}

	sb.WriteString("\n")

	// Quality score distribution
	sb.WriteString("## Quality Score Distribution\n\n")
	for _, provider := range providers {
		summary := g.collector.ComputeSummary(provider)
		sb.WriteString(fmt.Sprintf("### %s\n\n", provider))

		if len(summary.QualityScoreDist) == 0 {
			sb.WriteString("_No quality scores recorded_\n\n")
			continue
		}

		sb.WriteString("| Bucket | Count |\n")
		sb.WriteString("|--------|-------|\n")

		// Order buckets by quality level
		buckets := []string{
			"excellent (90-100)",
			"good (75-89)",
			"acceptable (60-74)",
			"poor (40-59)",
			"failed (0-39)",
		}

		for _, bucket := range buckets {
			if count, ok := summary.QualityScoreDist[bucket]; ok {
				sb.WriteString(fmt.Sprintf("| %s | %d |\n", bucket, count))
			}
		}
		sb.WriteString("\n")
	}

	// Error breakdown
	sb.WriteString("## Error Breakdown\n\n")
	for _, provider := range providers {
		summary := g.collector.ComputeSummary(provider)

		if len(summary.ErrorBreakdown) == 0 {
			continue
		}

		sb.WriteString(fmt.Sprintf("### %s\n\n", provider))
		sb.WriteString("| Error Category | Count |\n")
		sb.WriteString("|----------------|-------|\n")

		for category, count := range summary.ErrorBreakdown {
			sb.WriteString(fmt.Sprintf("| %s | %d |\n", category, count))
		}
		sb.WriteString("\n")
	}

	// Detailed quality results by test
	sb.WriteString("## Detailed Quality Results\n\n")

	tests := g.collector.GetAllTests()
	for _, testName := range tests {
		sb.WriteString(fmt.Sprintf("### %s\n\n", testName))
		sb.WriteString("| Provider | Quality Score | Semantic | Reranker | Status |\n")
		sb.WriteString("|----------|---------------|----------|----------|--------|\n")

		results := g.collector.GetResultsByTest(testName)
		for _, r := range results {
			status := "✓ Pass"
			if !r.Success {
				status = "✗ Fail"
			}

			sb.WriteString(fmt.Sprintf("| %s | %.1f | %.1f | %.1f | %s |\n",
				r.Provider,
				r.QualityScore,
				r.SemanticScore,
				r.RerankerScore,
				status,
			))
		}
		sb.WriteString("\n")
	}

	// Provider comparison
	if len(providers) == 2 {
		sb.WriteString("## Provider Quality Comparison\n\n")

		summary1 := g.collector.ComputeSummary(providers[0])
		summary2 := g.collector.ComputeSummary(providers[1])

		// Quality comparison
		if summary1.AvgQualityScore > 0 && summary2.AvgQualityScore > 0 {
			qualityDiff := summary2.AvgQualityScore - summary1.AvgQualityScore
			better := providers[0]
			if summary2.AvgQualityScore > summary1.AvgQualityScore {
				better = providers[1]
				qualityDiff = -qualityDiff
			}
			sb.WriteString(fmt.Sprintf("- **%s** has %.1f%% higher average quality score\n", better, qualityDiff))
		}

		// Success rate comparison
		if summary1.SuccessRate != summary2.SuccessRate {
			better := providers[0]
			diff := summary1.SuccessRate - summary2.SuccessRate
			if summary2.SuccessRate > summary1.SuccessRate {
				better = providers[1]
				diff = -diff
			}
			sb.WriteString(fmt.Sprintf("- **%s** has %.1f%% better success rate\n", better, diff))
		}

		sb.WriteString("\n")
	}

	// Write file
	outputPath := filepath.Join(g.outputDir, "quality_report.md")
	return os.WriteFile(outputPath, []byte(sb.String()), 0600)
}

// GenerateQualityJSON creates a JSON report with quality metrics
func (g *QualityReportGenerator) GenerateQualityJSON() error {
	providers := g.collector.GetAllProviders()

	data := map[string]interface{}{
		"timestamp":   time.Now(),
		"report_type": "quality",
		"providers":   providers,
	}

	// Add quality summaries
	summaries := make(map[string]*metrics.Summary)
	for _, provider := range providers {
		summaries[provider] = g.collector.ComputeSummary(provider)
	}
	data["summaries"] = summaries

	// Add detailed results with quality scores
	results := g.collector.GetResults()
	data["results"] = results

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	outputPath := filepath.Join(g.outputDir, "quality_report.json")
	return os.WriteFile(outputPath, jsonData, 0600)
}

// GenerateQualityHTML creates an HTML quality report
func (g *QualityReportGenerator) GenerateQualityHTML() error {
	timestamp := time.Now().Format("2006-01-02 15:04:05")

	html := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Search API Quality Benchmark Report</title>
    <script src="https://cdn.jsdelivr.net/npm/chart.js"></script>
    <style>
        * { box-sizing: border-box; margin: 0; padding: 0; }
        body { 
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: #f5f5f5;
            color: #333;
            line-height: 1.6;
            padding: 20px;
        }
        .container { max-width: 1200px; margin: 0 auto; }
        h1 { color: #2c3e50; margin-bottom: 10px; }
        .timestamp { color: #666; margin-bottom: 30px; }
        .cards { display: grid; grid-template-columns: repeat(auto-fit, minmax(280px, 1fr)); gap: 20px; margin-bottom: 30px; }
        .card { background: white; padding: 20px; border-radius: 8px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
        .card h3 { color: #666; font-size: 0.9em; text-transform: uppercase; margin-bottom: 10px; }
        .card .value { font-size: 2em; font-weight: bold; }
        .card .value.excellent { color: #27ae60; }
        .card .value.good { color: #2ecc71; }
        .card .value.acceptable { color: #f39c12; }
        .card .value.poor { color: #e74c3c; }
        .card .subtitle { color: #999; font-size: 0.9em; margin-top: 5px; }
        .chart-container { background: white; padding: 20px; border-radius: 8px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); margin-bottom: 20px; }
        .chart-wrapper { position: relative; height: 300px; }
        table { width: 100%; border-collapse: collapse; background: white; border-radius: 8px; overflow: hidden; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
        th, td { padding: 12px; text-align: left; border-bottom: 1px solid #eee; }
        th { background: #2c3e50; color: white; font-weight: 600; }
        tr:hover { background: #f9f9f9; }
        .quality-excellent { color: #27ae60; font-weight: bold; }
        .quality-good { color: #2ecc71; font-weight: bold; }
        .quality-acceptable { color: #f39c12; font-weight: bold; }
        .quality-poor { color: #e74c3c; font-weight: bold; }
        .quality-failed { color: #c0392b; font-weight: bold; }
        .provider-badge { display: inline-block; padding: 4px 12px; border-radius: 12px; font-size: 0.85em; font-weight: 600; }
        .provider-firecrawl { background: #ff6b35; color: white; }
        .provider-tavily { background: #3498db; color: white; }
        .provider-brave { background: #e74c3c; color: white; }
        .provider-exa { background: #9b59b6; color: white; }
        .provider-jina { background: #27ae60; color: white; }
        .provider-mixedbread { background: #f39c12; color: white; }
        .provider-local { background: #1abc9c; color: white; }
        .section { margin-bottom: 40px; }
        h2 { color: #2c3e50; margin-bottom: 20px; padding-bottom: 10px; border-bottom: 2px solid #3498db; }
    </style>
</head>
<body>
    <div class="container">
        <h1>Search API Quality Benchmark Report</h1>
        <p class="timestamp">Generated: ` + timestamp + `</p>
        
        <div class="section">
            <div class="cards">` + g.generateQualityCards() + `
            </div>
        </div>

        <div class="section">
            <h2>Quality Score Comparison</h2>
            <div class="chart-container">
                <div class="chart-wrapper">
                    <canvas id="qualityChart"></canvas>
                </div>
            </div>
        </div>

        <div class="section">
            <h2>Quality Score Distribution</h2>
            <div class="chart-container">
                <div class="chart-wrapper">
                    <canvas id="distributionChart"></canvas>
                </div>
            </div>
        </div>

        <div class="section">
            <h2>Error Breakdown</h2>
            <div class="chart-container">
                <div class="chart-wrapper">
                    <canvas id="errorChart"></canvas>
                </div>
            </div>
        </div>

        <div class="section">
            <h2>Detailed Quality Results</h2>
            <table>
                <thead>
                    <tr>
                        <th>Test</th>
                        <th>Provider</th>
                        <th>Quality Score</th>
                        <th>Semantic Score</th>
                        <th>Reranker Score</th>
                        <th>Status</th>
                    </tr>
                </thead>
                <tbody>` + g.generateQualityTableRows() + `
                </tbody>
            </table>
        </div>
    </div>

    <script>
        // Quality charts will be added here
    </script>
</body>
</html>`

	outputPath := filepath.Join(g.outputDir, "quality_report.html")
	return os.WriteFile(outputPath, []byte(html), 0600)
}

// Helper functions for quality reports

func (g *QualityReportGenerator) generateQualityCards() string {
	providers := g.collector.GetAllProviders()
	var cards strings.Builder

	for _, provider := range providers {
		summary := g.collector.ComputeSummary(provider)

		// Determine quality class
		qualityClass := "poor"
		if summary.AvgQualityScore >= 90 {
			qualityClass = "excellent"
		} else if summary.AvgQualityScore >= 75 {
			qualityClass = "good"
		} else if summary.AvgQualityScore >= 60 {
			qualityClass = "acceptable"
		}

		cards.WriteString(fmt.Sprintf(`
                <div class="card">
                    <h3>%s Average Quality</h3>
                    <div class="value %s">%.1f</div>
                    <div class="subtitle">Min: %.1f | Max: %.1f</div>
                </div>`, provider, qualityClass, summary.AvgQualityScore, summary.MinQualityScore, summary.MaxQualityScore))
	}

	return cards.String()
}

func (g *QualityReportGenerator) generateQualityTableRows() string {
	var rows strings.Builder
	tests := g.collector.GetAllTests()

	for _, testName := range tests {
		results := g.collector.GetResultsByTest(testName)
		for _, r := range results {
			status := `<span class="quality-excellent">✓ Pass</span>`
			if !r.Success {
				status = `<span class="quality-failed">✗ Fail</span>`
			}

			// Quality class
			qualityClass := "quality-poor"
			if r.QualityScore >= 90 {
				qualityClass = "quality-excellent"
			} else if r.QualityScore >= 75 {
				qualityClass = "quality-good"
			} else if r.QualityScore >= 60 {
				qualityClass = "quality-acceptable"
			}

			providerClass := "provider-" + r.Provider

			rows.WriteString(fmt.Sprintf(`
                    <tr>
                        <td>%s</td>
                        <td><span class="provider-badge %s">%s</span></td>
                        <td class="%s">%.1f</td>
                        <td>%.1f</td>
                        <td>%.1f</td>
                        <td>%s</td>
                    </tr>`,
				testName,
				providerClass,
				capitalize(r.Provider),
				qualityClass,
				r.QualityScore,
				r.SemanticScore,
				r.RerankerScore,
				status))
		}
	}

	return rows.String()
}
