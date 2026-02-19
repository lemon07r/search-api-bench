// Package report generates HTML, Markdown, and JSON reports from benchmark results.
package report

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/lamim/sanity-web-eval/internal/metrics"
)

// GenerateHTML creates an HTML report with charts
func (g *Generator) GenerateHTML() error {
	providers := g.collector.GetAllProviders()
	tests := g.collector.GetAllTests()
	timestamp := time.Now().Format("2006-01-02 15:04:05")

	var html strings.Builder

	html.WriteString(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>SanityWebEval Report</title>
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
        .card .value { font-size: 2em; font-weight: bold; color: #2c3e50; }
        .card .subtitle { color: #999; font-size: 0.9em; margin-top: 5px; }
        .chart-container { background: white; padding: 20px; border-radius: 8px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); margin-bottom: 20px; }
        .chart-wrapper { position: relative; height: 300px; }
        table { width: 100%; border-collapse: collapse; background: white; border-radius: 8px; overflow: hidden; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
        th, td { padding: 12px; text-align: left; border-bottom: 1px solid #eee; }
        th { background: #2c3e50; color: white; font-weight: 600; }
        tr:hover { background: #f9f9f9; }
        .success { color: #27ae60; }
        .failure { color: #e74c3c; }
        .skipped { color: #7f8c8d; }
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
        .quality-note { color: #666; margin: -8px 0 16px; font-size: 0.9em; }
        .chart-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(380px, 1fr)); gap: 20px; margin-bottom: 20px; }
        .chart-grid .chart-container { margin-bottom: 0; }
        .chart-grid .chart-wrapper { height: 320px; }
        .heatmap-cell { width: 100%; height: 100%; display: flex; align-items: center; justify-content: center; font-weight: bold; font-size: 0.9em; border-radius: 4px; }
        .heatmap-excellent { background: #27ae60; color: white; }
        .heatmap-good { background: #2ecc71; color: white; }
        .heatmap-acceptable { background: #f39c12; color: white; }
        .heatmap-poor { background: #e74c3c; color: white; }
        .heatmap-na { background: #ecf0f1; color: #7f8c8d; }
        .heatmap-grid { display: grid; gap: 4px; margin-top: 20px; }
        .heatmap-header { font-weight: 600; font-size: 0.85em; color: #666; padding: 8px; text-align: center; }
        .heatmap-provider { font-weight: 600; font-size: 0.85em; color: #333; padding: 8px; display: flex; align-items: center; }
        .scatter-tooltip { background: rgba(0,0,0,0.8); color: white; padding: 8px 12px; border-radius: 4px; font-size: 0.85em; }
    </style>
</head>
<body>
    <div class="container">
        <h1>SanityWebEval Report</h1>
        <p class="timestamp">Generated: `)
	html.WriteString(timestamp)
	html.WriteString(`</p>
        
        <div class="section">
            <div class="cards">
                <div class="card">
                    <h3>Total Tests</h3>
                    <div class="value">`)
	fmt.Fprintf(&html, "%d", len(tests)*len(providers))
	html.WriteString(`</div>
                    <div class="subtitle">Across `)
	fmt.Fprintf(&html, "%d", len(providers))
	html.WriteString(` providers</div>
                </div>
                <div class="card">
                    <h3>Providers Tested</h3>
                    <div class="value">`)
	fmt.Fprintf(&html, "%d", len(providers))
	html.WriteString(`</div>
                    <div class="subtitle">`)
	html.WriteString(joinProviders(providers))
	html.WriteString(`</div>
                </div>
                <div class="card">
                    <h3>Test Scenarios</h3>
                    <div class="value">`)
	fmt.Fprintf(&html, "%d", len(tests))
	html.WriteString(`</div>
                    <div class="subtitle">Search, Extract, Crawl</div>
                </div>
            </div>
        </div>

        <div class="section">
            <h2>Performance Comparison</h2>
            <div class="chart-container">
                <div class="chart-wrapper">
                    <canvas id="latencyChart"></canvas>
                </div>
            </div>
        </div>

        <div class="section">
            <h2>Cost Analysis - Total USD Cost</h2>
            <div class="chart-container">
                <div class="chart-wrapper">
                    <canvas id="costChart"></canvas>
                </div>
            </div>
        </div>

        <div class="section">
            <h2>Success Rate</h2>
            <div class="chart-container">
                <div class="chart-wrapper">
                    <canvas id="successChart"></canvas>
                </div>
            </div>
        </div>

` + g.generateQualitySection() + g.generateQualityByTestTypeSection() + g.generateSemanticRerankerSection() + g.generateAdvancedAnalyticsSection() + `
        <div class="section">
            <h2>Detailed Results</h2>
            <table>
                <thead>
                    <tr>
                        <th>Test</th>
                        <th>Provider</th>
                        <th>Status</th>
                        <th>Latency</th>
                        <th>Cost (USD)</th>
                        <th>Results/Content</th>
` + g.generateQualityTableHeader() + `
                    </tr>
                </thead>
                <tbody>
`)
	html.WriteString(g.generateTableRows())
	html.WriteString(`                </tbody>
            </table>
        </div>
    </div>

    <script>
`)
	html.WriteString(g.generateChartScripts())
	html.WriteString(`    </script>
</body>
</html>`)

	outputPath := filepath.Join(g.outputDir, "report.html")
	// #nosec G306 - 0640 allows owner/group to read, which is appropriate for report files
	return os.WriteFile(outputPath, []byte(html.String()), 0640)
}

func joinProviders(providers []string) string {
	result := ""
	for i, p := range providers {
		if i > 0 {
			result += " vs "
		}
		result += capitalize(p)
	}
	return result
}

func capitalize(s string) string {
	if len(s) == 0 {
		return s
	}
	return string(s[0]-32) + s[1:]
}

// hasQualityScores checks if any results have quality scores
func (g *Generator) hasQualityScores() bool {
	providers := g.collector.GetAllProviders()
	for _, provider := range providers {
		results := g.collector.GetResultsByProvider(provider)
		for _, r := range results {
			if r.QualityScored || r.QualityScore > 0 {
				return true
			}
		}
	}
	return false
}

// generateQualitySection returns the quality chart HTML section if quality scores exist
func (g *Generator) generateQualitySection() string {
	if !g.hasQualityScores() {
		return ""
	}
	return `        <div class="section">
            <h2>Scoring Overview</h2>
            <p class="quality-note">Search relevance uses model-assisted signals. Extract and crawl use rule-based heuristics. These score families are reported separately.</p>
            <div class="chart-container">
                <div class="chart-wrapper">
                    <canvas id="qualityChart"></canvas>
                </div>
            </div>
        </div>

`
}

func (g *Generator) generateQualityByTestTypeSection() string {
	if !g.hasQualityScores() {
		return ""
	}

	var rows strings.Builder
	for _, provider := range g.collector.GetAllProviders() {
		byType := g.computeProviderQualityByTestType(provider)
		rows.WriteString(fmt.Sprintf(`
                    <tr>
                        <td><span class="provider-badge provider-%s">%s</span></td>
                        <td>%s</td>
                        <td>%s</td>
                        <td>%s</td>
                        <td>%s</td>
                        <td>%s</td>
                        <td>%s</td>
                        <td>%s</td>
                        <td>%s</td>
                        <td>%s</td>
                        <td>%s</td>
                        <td>%s</td>
                        <td>%s</td>
                    </tr>`,
			provider,
			capitalize(provider),
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
		))
	}

	return `
        <div class="section">
            <h2>Scoring by Test Type</h2>
            <p class="quality-note">Search relevance uses model signals; extract and crawl scores are heuristic diagnostics.</p>
            <table>
                <thead>
                    <tr>
                        <th>Provider</th>
                        <th>Search Relevance</th>
                        <th>Search Coverage</th>
                        <th>Semantic</th>
                        <th>Semantic Coverage</th>
                        <th>Reranker</th>
                        <th>Reranker Coverage</th>
                        <th>Search Component Avg</th>
                        <th>Component Coverage</th>
                        <th>Extract Heuristic</th>
                        <th>Extract Coverage</th>
                        <th>Crawl Heuristic</th>
                        <th>Crawl Coverage</th>
                    </tr>
                </thead>
                <tbody>` + rows.String() + `
                </tbody>
            </table>
        </div>

`
}

// generateSemanticRerankerSection returns the semantic/reranker score chart HTML when data exists.
func (g *Generator) generateSemanticRerankerSection() string {
	providers := g.collector.GetAllProviders()
	_, hasData := g.computeSemanticRerankerScores(providers)
	if !hasData {
		return ""
	}
	return `        <div class="section">
            <h2>Search Scoring Breakdown: Semantic &amp; Reranker</h2>
            <p class="quality-note">Embedding similarity (semantic) and reranker confidence scores for search tests. These are the two components that compose the blended search relevance score above.</p>
            <div class="chart-container">
                <div class="chart-wrapper">
                    <canvas id="semanticRerankerChart"></canvas>
                </div>
            </div>
        </div>

`
}

// generateQualityTableHeader returns the quality column headers if quality scores exist
func (g *Generator) generateQualityTableHeader() string {
	if !g.hasQualityScores() {
		return ""
	}
	return `                        <th>Score Family</th>
                        <th>Score</th>
                        <th>Semantic (Search)</th>
                        <th>Reranker (Search)</th>`
}

func (g *Generator) generateTableRows() string {
	var rows string
	tests := g.collector.GetAllTests()
	showQuality := g.hasQualityScores()

	formatLatency := func(d time.Duration) string {
		return fmt.Sprintf("%.0fms", float64(d.Milliseconds()))
	}

	formatCost := func(cost float64) string {
		if cost == 0 {
			return "-"
		}
		if cost > 0 && cost < 0.0001 {
			return "<$0.0001"
		}
		if cost < 0.01 {
			return fmt.Sprintf("$%.4f", cost)
		}
		return fmt.Sprintf("$%.2f", cost)
	}

	for _, testName := range tests {
		results := g.collector.GetResultsByTest(testName)
		for _, r := range results {
			status := `<span class="success">✓ Pass</span>`
			if r.Skipped {
				status = `<span class="skipped">⊘ Skip</span>`
			} else if !r.Success {
				status = `<span class="failure">✗ Fail</span>`
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

			providerClass := "provider-" + r.Provider
			costStr := formatCost(r.CostUSD)

			if showQuality {
				qualityStr := "-"
				if r.QualityScored || r.QualityScore > 0 {
					qualityStr = fmt.Sprintf("%.1f", r.QualityScore)
				}
				scoreFamily := scoreLabelForTestType(r.TestType)
				semanticStr := "-"
				if r.SemanticScore > 0 {
					semanticStr = fmt.Sprintf("%.1f", r.SemanticScore)
				}
				rerankerStr := "-"
				if r.RerankerScore > 0 {
					rerankerStr = fmt.Sprintf("%.1f", r.RerankerScore)
				}
				rows += fmt.Sprintf(`                    <tr>
                        <td>%s</td>
                        <td><span class="provider-badge %s">%s</span></td>
                        <td>%s</td>
                        <td>%s</td>
                        <td>%s</td>
                        <td>%s</td>
                        <td>%s</td>
                        <td>%s</td>
                        <td>%s</td>
                        <td>%s</td>
                    </tr>
`, testName, providerClass, capitalize(r.Provider), status, formatLatency(r.Latency), costStr, details, scoreFamily, qualityStr, semanticStr, rerankerStr)
			} else {
				rows += fmt.Sprintf(`                    <tr>
                        <td>%s</td>
                        <td><span class="provider-badge %s">%s</span></td>
                        <td>%s</td>
                        <td>%s</td>
                        <td>%s</td>
                        <td>%s</td>
                    </tr>
`, testName, providerClass, capitalize(r.Provider), status, formatLatency(r.Latency), costStr, details)
			}
		}
	}

	return rows
}

func (g *Generator) generateChartScripts() string {
	providers := g.collector.GetAllProviders()

	// Prepare data for charts
	providerNames := make([]string, len(providers))
	avgLatencies := make([]float64, len(providers))
	successRates := make([]float64, len(providers))
	searchRelevanceScores := make([]float64, len(providers))
	extractHeuristicScores := make([]float64, len(providers))
	crawlHeuristicScores := make([]float64, len(providers))
	// USD cost metrics
	totalCostUSD := make([]float64, len(providers))

	baseColors := []string{"'#ff6b35'", "'#3498db'", "'#27ae60'", "'#9b59b6'", "'#e74c3c'", "'#f39c12'", "'#1abc9c'"}
	colors := make([]string, len(providers))
	for i := range providers {
		colors[i] = baseColors[i%len(baseColors)]
	}

	showQuality := g.hasQualityScores()
	semScores := make([]float64, len(providers))
	rerScores := make([]float64, len(providers))
	hasSemanticReranker := false

	for i, provider := range providers {
		summary := g.collector.ComputeSummary(provider)
		byType := g.computeProviderQualityByTestType(provider)
		providerNames[i] = "'" + capitalize(provider) + "'"
		avgLatencies[i] = float64(summary.AvgLatency.Milliseconds())
		successRates[i] = summary.SuccessRate
		searchRelevanceScores[i] = byType.Search.AvgQuality
		extractHeuristicScores[i] = byType.Extract.AvgQuality
		crawlHeuristicScores[i] = byType.Crawl.AvgQuality
		// USD cost metrics
		totalCostUSD[i] = summary.TotalCostUSD
	}
	if showQuality {
		srData, hasSR := g.computeSemanticRerankerScores(providers)
		hasSemanticReranker = hasSR
		if hasSR {
			for i, d := range srData {
				semScores[i] = d.AvgSemantic
				rerScores[i] = d.AvgReranker
			}
		}
	}

	latencyBounds := calcPaddedBounds(avgLatencies, 0.25, 100)
	latencyBounds.Min = math.Max(0, latencyBounds.Min)

	costBounds := calcPaddedBounds(totalCostUSD, 0.25, 0.0005)
	costBounds.Min = math.Max(0, costBounds.Min)

	successBounds := axisBounds{Min: 0, Max: 100}
	if !hasMeaningfulSpread(successRates, 20) {
		successBounds = clampAxisBounds(calcPaddedBounds(successRates, 0.25, 5), 0, 100)
	}

	qualityBounds := axisBounds{Min: 0, Max: 100}
	if showQuality {
		qualityBounds = clampAxisBounds(
			calcPaddedBounds(combineFloatSlices(searchRelevanceScores, extractHeuristicScores, crawlHeuristicScores), 0.2, 5),
			0,
			100,
		)
	}

	semanticRerankerBounds := axisBounds{Min: 0, Max: 100}
	if hasSemanticReranker {
		semanticRerankerBounds = clampAxisBounds(calcPaddedBounds(combineFloatSlices(semScores, rerScores), 0.2, 5), 0, 100)
	}

	scoreChartScript := ""
	if showQuality {
		scoreChartScript = fmt.Sprintf(`
        // Score Chart (separate score families)
        new Chart(document.getElementById('qualityChart'), {
            type: 'bar',
            data: {
                labels: [%s],
                datasets: [
                    {
                        label: 'Search Relevance',
                        data: [%s],
                        backgroundColor: [%s],
                        borderRadius: 4
                    },
                    {
                        label: 'Extract Heuristic',
                        data: [%s],
                        backgroundColor: 'rgba(52, 152, 219, 0.65)',
                        borderRadius: 4
                    },
                    {
                        label: 'Crawl Heuristic',
                        data: [%s],
                        backgroundColor: 'rgba(44, 62, 80, 0.65)',
                        borderRadius: 4
                    }
                ]
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                plugins: {
                    legend: { display: true, position: 'bottom' },
                    title: { display: true, text: 'Scores by Type (no blended single score)' }
                },
                scales: {
                    y: {
                        min: %.2f,
                        max: %.2f,
                        title: { display: true, text: 'Score' }
                    }
                }
            }
        });
`, joinStrings(providerNames), formatFloatSlice(searchRelevanceScores), joinStrings(colors), formatFloatSlice(extractHeuristicScores), formatFloatSlice(crawlHeuristicScores), qualityBounds.Min, qualityBounds.Max)
	}

	semanticRerankerScript := ""
	if hasSemanticReranker {
		semanticRerankerScript = fmt.Sprintf(`
        // Semantic & Reranker Score Chart
        new Chart(document.getElementById('semanticRerankerChart'), {
            type: 'bar',
            data: {
                labels: [%s],
                datasets: [
                    {
                        label: 'Semantic (Embedding Similarity)',
                        data: [%s],
                        backgroundColor: 'rgba(52, 152, 219, 0.75)',
                        borderRadius: 4
                    },
                    {
                        label: 'Reranker Confidence',
                        data: [%s],
                        backgroundColor: 'rgba(155, 89, 182, 0.75)',
                        borderRadius: 4
                    }
                ]
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                plugins: {
                    legend: { display: true, position: 'bottom' },
                    title: { display: true, text: 'Search Score Components: Embedding Similarity vs Reranker Confidence' }
                },
                scales: {
                    y: {
                        min: %.2f,
                        max: %.2f,
                        title: { display: true, text: 'Score (0-100)' }
                    }
                }
            }
        });
`, joinStrings(providerNames), formatFloatSlice(semScores), formatFloatSlice(rerScores), semanticRerankerBounds.Min, semanticRerankerBounds.Max)
	}

	advancedScripts := g.generateAdvancedChartScripts(providers, providerNames, colors, baseColors)

	return scoreChartScript + semanticRerankerScript + fmt.Sprintf(`
        // Latency Chart
        new Chart(document.getElementById('latencyChart'), {
            type: 'bar',
            data: {
                labels: [%s],
                datasets: [{
                    label: 'Average Latency (ms)',
                    data: [%s],
                    backgroundColor: [%s],
                    borderRadius: 4
                }]
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                plugins: {
                    legend: { display: false },
                    title: { display: true, text: 'Average Response Time' }
                },
                scales: {
                    y: {
                        min: %.2f,
                        max: %.2f,
                        title: { display: true, text: 'Milliseconds' }
                    }
                }
            }
        });

        // Success Rate Chart
        new Chart(document.getElementById('successChart'), {
            type: 'bar',
            data: {
                labels: [%s],
                datasets: [{
                    label: 'Success Rate (%%)',
                    data: [%s],
                    backgroundColor: [%s],
                    borderRadius: 4
                }]
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                plugins: {
                    legend: { display: false },
                    title: { display: true, text: 'Request Success Rate' }
                },
                scales: {
                    y: {
                        min: %.2f,
                        max: %.2f,
                        title: { display: true, text: 'Percentage' }
                    }
                }
            }
        });

        // USD Cost Chart - Total Cost
        new Chart(document.getElementById('costChart'), {
            type: 'bar',
            data: {
                labels: [%s],
                datasets: [{
                    label: 'Total USD Cost',
                    data: [%s],
                    backgroundColor: [%s],
                    borderRadius: 4
                }]
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                plugins: {
                    legend: { display: false },
                    title: { display: true, text: 'Total API Cost in USD (lower is better)' }
                },
                scales: {
                    y: { 
                        min: %.6f,
                        max: %.6f,
                        title: { display: true, text: 'USD ($)' },
                        ticks: {
                            callback: function(value) {
                                const abs = Math.abs(Number(value));
                                const digits = abs < 0.001 ? 5 : (abs < 0.01 ? 4 : 3);
                                return '$' + Number(value).toFixed(digits);
                            }
                        }
                    }
                }
            }
        });

%s`, joinStrings(providerNames), formatFloatSlice(avgLatencies), joinStrings(colors), latencyBounds.Min, latencyBounds.Max,
		joinStrings(providerNames), formatFloatSlice(successRates), joinStrings(colors), successBounds.Min, successBounds.Max,
		joinStrings(providerNames), formatFloatSlice(totalCostUSD), joinStrings(colors), costBounds.Min, costBounds.Max,
		advancedScripts)
}

// generateAdvancedChartScripts creates JavaScript for all advanced analytics charts
func (g *Generator) generateAdvancedChartScripts(providers []string, providerNames []string, _ []string, baseColors []string) string {
	if len(providers) < 2 {
		return ""
	}

	// Prepare normalized data for radar chart (0-100 scale)
	radars := g.prepareRadarData(providers)
	latencyDists := g.prepareLatencyDistributionData(providers)
	scatterData := g.prepareScatterData(providers)
	errorData := g.prepareErrorBreakdownData(providers)
	heatmapData := g.prepareHeatmapData(providers)
	showQuality := g.hasQualityScores()
	radarLabels := "'Success Rate', 'Speed Score', 'Cost Efficiency', 'Content Volume'"
	if showQuality {
		radarLabels += ", 'Search Relevance'"
	}

	// Build radar datasets
	radarDatasets := ""
	radarHasData := false
	for i, p := range providers {
		series := []float64{
			radars[i].SuccessRate,
			radars[i].SpeedScore,
			radars[i].CostEfficiency,
			radars[i].ContentScore,
		}
		if showQuality {
			series = append(series, radars[i].SearchRelevanceScore)
		}
		if hasAnyPositive(series) {
			radarHasData = true
		}
		radarDatasets += fmt.Sprintf(`{
                    label: '%s',
                    data: [%s],
                    backgroundColor: %s.replace("'", "").replace("'", "") + '33',
                    borderColor: %s,
                    pointBackgroundColor: %s,
                    pointBorderColor: '#fff',
                    pointHoverBackgroundColor: '#fff',
                    pointHoverBorderColor: %s,
                    borderWidth: 2
                },`, capitalize(p),
			formatFloatSlice(series),
			baseColors[i%len(baseColors)], baseColors[i%len(baseColors)], baseColors[i%len(baseColors)], baseColors[i%len(baseColors)])
	}
	radarDatasets = strings.TrimSuffix(radarDatasets, ",")

	// Build latency chart datasets. If ranges collapse to zero, fall back to average latency bars.
	minP50Ranges := latencyDistsToMinP50(latencyDists)
	p50P95Ranges := latencyDistsToP50P95(latencyDists)
	p95MaxRanges := latencyDistsToP95Max(latencyDists)
	avgLatency := latencyDistsToAvg(latencyDists)
	latencyRangesHasData := hasAnyPositive(minP50Ranges) || hasAnyPositive(p50P95Ranges) || hasAnyPositive(p95MaxRanges)
	latencyHasAnyData := latencyRangesHasData || hasAnyPositive(avgLatency)
	latencyChartTitle := "Latency Distribution: Min, P50, P95, Max (ms)"
	latencyDatasets := fmt.Sprintf(`[
                    {
                        label: 'Min-P50 Range',
                        data: [%s],
                        backgroundColor: 'rgba(52, 152, 219, 0.8)',
                        borderRadius: 4
                    },
                    {
                        label: 'P50-P95 Range',
                        data: [%s],
                        backgroundColor: 'rgba(46, 204, 113, 0.8)',
                        borderRadius: 4
                    },
                    {
                        label: 'P95-Max Range',
                        data: [%s],
                        backgroundColor: 'rgba(231, 76, 60, 0.8)',
                        borderRadius: 4
                    }
                ]`, formatFloatSlice(minP50Ranges), formatFloatSlice(p50P95Ranges), formatFloatSlice(p95MaxRanges))
	latencyRangesPayload := g.formatLatencyRanges(latencyDists)
	latencyXStacked := "true"
	latencyYStacked := "true"
	latencyYBounds := calcPaddedBounds(latencyDistsToMax(latencyDists), 0.2, 100)
	latencyYBounds.Min = math.Max(0, latencyYBounds.Min)
	if !latencyRangesHasData {
		latencyChartTitle = "Latency Distribution (Fallback to Average Latency)"
		latencyDatasets = fmt.Sprintf(`[
                    {
                        label: 'Average Latency',
                        data: [%s],
                        backgroundColor: 'rgba(52, 152, 219, 0.8)',
                        borderRadius: 4
                    }
                ]`, formatFloatSlice(avgLatency))
		latencyRangesPayload = "[]"
		latencyXStacked = "false"
		latencyYStacked = "false"
		latencyYBounds = calcPaddedBounds(avgLatency, 0.25, 100)
		latencyYBounds.Min = math.Max(0, latencyYBounds.Min)
	}
	latencyEmptyMessage := ""
	if !latencyHasAnyData {
		latencyEmptyMessage = "No latency data available for this run"
	}

	// Build scatter datasets (one per provider)
	scatterDatasets := ""
	for i, p := range providers {
		color := baseColors[i%len(baseColors)]
		scatterDatasets += fmt.Sprintf(`{
                    label: '%s',
                    data: [{x: %f, y: %f}],
                    backgroundColor: %s,
                    borderColor: %s,
                    borderWidth: 2,
                    pointRadius: 8,
                    pointHoverRadius: 11
                },`, capitalize(p), scatterData[i].CostPerResult, scatterData[i].SearchRelevance, color, color)
	}
	scatterDatasets = strings.TrimSuffix(scatterDatasets, ",")

	// Build speed-quality scatter
	speedQualityDatasets := ""
	for i, p := range providers {
		color := baseColors[i%len(baseColors)]
		speedQualityDatasets += fmt.Sprintf(`{
                    label: '%s',
                    data: [{x: %f, y: %f}],
                    backgroundColor: %s,
                    borderColor: %s,
                    borderWidth: 2,
                    pointRadius: 8,
                    pointHoverRadius: 11
                },`, capitalize(p), scatterData[i].Speed, scatterData[i].SearchRelevance, color, color)
	}
	speedQualityDatasets = strings.TrimSuffix(speedQualityDatasets, ",")

	// Build cost-speed scatter
	costSpeedDatasets := ""
	for i, p := range providers {
		color := baseColors[i%len(baseColors)]
		costSpeedDatasets += fmt.Sprintf(`{
                    label: '%s',
                    data: [{x: %f, y: %f}],
                    backgroundColor: %s,
                    borderColor: %s,
                    borderWidth: 2,
                    pointRadius: 8,
                    pointHoverRadius: 11
                },`, capitalize(p), scatterData[i].CostPerResult, scatterData[i].Speed, color, color)
	}
	costSpeedDatasets = strings.TrimSuffix(costSpeedDatasets, ",")

	// Build error breakdown datasets
	errorDatasets := g.buildErrorDatasets(providers, errorData, baseColors)
	errorHasData := strings.TrimSpace(errorDatasets) != ""
	if !errorHasData {
		zeros := make([]int, len(providers))
		errorDatasets = fmt.Sprintf(`{
                    label: 'No Errors',
                    data: [%s],
                    backgroundColor: 'rgba(189, 195, 199, 0.45)',
                    borderRadius: 4
                }`, formatIntSlice(zeros))
	}

	// Heatmap generation
	heatmapScript := g.generateHeatmapScript(providers, heatmapData)

	// Compute quadrant midpoints (median of each axis)
	costMid, speedMid, relevanceMid := computeQuadrantMidpoints(scatterData)
	costValues, speedValues, relevanceValues := splitScatterAxes(scatterData)
	costBounds := calcCenteredBounds(costValues, costMid, 0.35, 0.0005)
	costAxisType := "linear"
	if shouldPreferLogScale(costValues, costMid, 4.0) {
		if b, ok := calcLogCenteredBounds(costValues, costMid, 0.20, 2.0, 0.000001); ok {
			costBounds = b
			costAxisType = "logarithmic"
		}
	}
	speedBounds := calcCenteredBounds(speedValues, speedMid, 0.35, 200)
	speedAxisType := "linear"
	if shouldPreferLogScale(speedValues, speedMid, 4.0) {
		if b, ok := calcLogCenteredBounds(speedValues, speedMid, 0.20, 2.0, 1); ok {
			speedBounds = b
			speedAxisType = "logarithmic"
		}
	}
	relevanceBounds := clampAxisBounds(calcCenteredBounds(relevanceValues, relevanceMid, 0.25, 4), 0, 100)
	costTickPrecision := 4
	if costBounds.Max-costBounds.Min >= 0.01 && costAxisType == "linear" {
		costTickPrecision = 3
	}

	return fmt.Sprintf(`
        // Empty state annotation plugin for charts with no meaningful data
        const emptyStatePlugin = {
            id: 'emptyState',
            afterDraw(chart, args, opts) {
                if (!opts || !opts.enabled) return;
                const chartArea = chart.chartArea;
                if (!chartArea) return;
                const {ctx} = chart;
                const {left, right, top, bottom} = chartArea;
                ctx.save();
                ctx.fillStyle = 'rgba(80, 80, 80, 0.8)';
                ctx.textAlign = 'center';
                ctx.textBaseline = 'middle';
                ctx.font = '600 13px sans-serif';
                ctx.fillText(opts.text || 'No data available', (left + right) / 2, (top + bottom) / 2);
                ctx.restore();
            }
        };

        // Quadrant background plugin for scatter plots
        const quadrantPlugin = {
            id: 'quadrantBackground',
            beforeDraw(chart) {
                const opts = chart.options.plugins.quadrantBackground;
                if (!opts) return;
                const chartArea = chart.chartArea;
                const x = chart.scales.x;
                const y = chart.scales.y;
                if (!chartArea || !x || !y) return;
                const {ctx} = chart;
                const {left, top, right, bottom} = chartArea;
                const midX = x.getPixelForValue(opts.xMid);
                const midY = y.getPixelForValue(opts.yMid);
                const clampX = Math.max(left, Math.min(right, midX));
                const clampY = Math.max(top, Math.min(bottom, midY));

                ctx.save();
                // Sweet-spot quadrant (low cost/fast + high quality)
                ctx.fillStyle = opts.sweetSpot || 'rgba(46, 204, 113, 0.08)';
                if (opts.sweetCorner === 'topLeft') {
                    ctx.fillRect(left, top, clampX - left, clampY - top);
                } else if (opts.sweetCorner === 'bottomLeft') {
                    ctx.fillRect(left, clampY, clampX - left, bottom - clampY);
                }

                // Worst quadrant (opposite corner)
                ctx.fillStyle = opts.worst || 'rgba(231, 76, 60, 0.06)';
                if (opts.sweetCorner === 'topLeft') {
                    ctx.fillRect(clampX, clampY, right - clampX, bottom - clampY);
                } else if (opts.sweetCorner === 'bottomLeft') {
                    ctx.fillRect(clampX, top, right - clampX, clampY - top);
                }

                // Draw crosshair lines
                ctx.strokeStyle = 'rgba(0,0,0,0.10)';
                ctx.lineWidth = 1;
                ctx.setLineDash([4, 4]);
                ctx.beginPath();
                ctx.moveTo(clampX, top); ctx.lineTo(clampX, bottom);
                ctx.moveTo(left, clampY); ctx.lineTo(right, clampY);
                ctx.stroke();
                ctx.restore();
            }
        };
        Chart.register(emptyStatePlugin, quadrantPlugin);

        // Radar Chart - Provider Performance Profile
        new Chart(document.getElementById('radarChart'), {
            type: 'radar',
            data: {
                labels: [%s],
                datasets: [%s]
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                plugins: {
                    title: { display: true, text: 'Multi-Dimensional Performance Comparison (0-100, higher is better)' },
                    legend: { position: 'bottom' },
                    emptyState: { enabled: %t, text: 'No comparable performance profile data' }
                },
                scales: {
                    r: {
                        beginAtZero: true,
                        max: 100,
                        ticks: { stepSize: 20 }
                    }
                }
            }
        });

        // Latency Distribution Chart
        new Chart(document.getElementById('latencyDistChart'), {
            type: 'bar',
            data: {
                labels: [%s],
                datasets: %s
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                plugins: {
                    title: { display: true, text: '%s' },
                    legend: { display: true, position: 'bottom' },
                    emptyState: { enabled: %t, text: '%s' },
                    tooltip: {
                        callbacks: {
                            label: function(context) {
                                const datasetIndex = context.datasetIndex;
                                const dataIndex = context.dataIndex;
                                const ranges = %s;
                                if (!ranges.length) {
                                    return context.dataset.label + ': ' + context.raw.toFixed(0) + 'ms';
                                }
                                const range = ranges[dataIndex];
                                if (!range) {
                                    return context.dataset.label + ': ' + context.raw.toFixed(0) + 'ms';
                                }
                                return context.dataset.label + ': ' + range[datasetIndex];
                            }
                        }
                    }
                },
                scales: {
                    x: { stacked: %s },
                    y: { 
                        stacked: %s,
                        min: %.2f,
                        max: %.2f,
                        title: { display: true, text: 'Milliseconds' }
                    }
                }
            }
        });

        // Cost vs Search Relevance Scatter Plot
        new Chart(document.getElementById('costQualityScatter'), {
            type: 'scatter',
            data: { datasets: [%s] },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                plugins: {
                    title: { display: true, text: 'Cost vs Search Relevance', font: { size: 14 } },
                    legend: { display: true, position: 'bottom' },
                    quadrantBackground: {
                        xMid: %.6f, yMid: %.2f,
                        sweetCorner: 'topLeft',
                        sweetSpot: 'rgba(46, 204, 113, 0.08)',
                        worst: 'rgba(231, 76, 60, 0.06)'
                    },
                    tooltip: {
                        callbacks: {
                            label: function(context) {
                                return context.dataset.label + ': $' + context.raw.x.toFixed(4) + ', relevance ' + context.raw.y.toFixed(1);
                            }
                        }
                    }
                },
                scales: {
                    x: {
                        type: '%s',
                        title: { display: true, text: 'Cost per Result (USD)' },
                        min: %.6f,
                        max: %.6f,
                        ticks: {
                            callback: function(v) {
                                const abs = Math.abs(Number(v));
                                const digits = abs < 0.001 ? 5 : (abs < 0.01 ? 4 : %d);
                                return '$' + Number(v).toFixed(digits);
                            }
                        }
                    },
                    y: {
                        title: { display: true, text: 'Search Relevance (0-100)' },
                        min: %.2f,
                        max: %.2f
                    }
                }
            }
        });

        // Speed vs Search Relevance Scatter Plot
        new Chart(document.getElementById('speedQualityScatter'), {
            type: 'scatter',
            data: { datasets: [%s] },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                plugins: {
                    title: { display: true, text: 'Speed vs Search Relevance', font: { size: 14 } },
                    legend: { display: true, position: 'bottom' },
                    quadrantBackground: {
                        xMid: %.2f, yMid: %.2f,
                        sweetCorner: 'topLeft',
                        sweetSpot: 'rgba(46, 204, 113, 0.08)',
                        worst: 'rgba(231, 76, 60, 0.06)'
                    },
                    tooltip: {
                        callbacks: {
                            label: function(context) {
                                return context.dataset.label + ': ' + context.raw.x.toFixed(0) + 'ms, relevance ' + context.raw.y.toFixed(1);
                            }
                        }
                    }
                },
                scales: {
                    x: {
                        type: '%s',
                        title: { display: true, text: 'Average Latency (ms)' },
                        min: %.2f,
                        max: %.2f,
                        ticks: { callback: function(v) { return Number(v).toLocaleString(); } }
                    },
                    y: {
                        title: { display: true, text: 'Search Relevance (0-100)' },
                        min: %.2f,
                        max: %.2f
                    }
                }
            }
        });

        // Cost vs Speed Scatter Plot
        new Chart(document.getElementById('costSpeedScatter'), {
            type: 'scatter',
            data: { datasets: [%s] },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                plugins: {
                    title: { display: true, text: 'Cost vs Speed', font: { size: 14 } },
                    legend: { display: true, position: 'bottom' },
                    quadrantBackground: {
                        xMid: %.6f, yMid: %.2f,
                        sweetCorner: 'bottomLeft',
                        sweetSpot: 'rgba(46, 204, 113, 0.08)',
                        worst: 'rgba(231, 76, 60, 0.06)'
                    },
                    tooltip: {
                        callbacks: {
                            label: function(context) {
                                return context.dataset.label + ': $' + context.raw.x.toFixed(4) + ', ' + context.raw.y.toFixed(0) + 'ms';
                            }
                        }
                    }
                },
                scales: {
                    x: {
                        type: '%s',
                        title: { display: true, text: 'Cost per Result (USD)' },
                        min: %.6f,
                        max: %.6f,
                        ticks: {
                            callback: function(v) {
                                const abs = Math.abs(Number(v));
                                const digits = abs < 0.001 ? 5 : (abs < 0.01 ? 4 : %d);
                                return '$' + Number(v).toFixed(digits);
                            }
                        }
                    },
                    y: {
                        type: '%s',
                        title: { display: true, text: 'Average Latency (ms)' },
                        min: %.2f,
                        max: %.2f,
                        ticks: { callback: function(v) { return Number(v).toLocaleString(); } }
                    }
                }
            }
        });

        // Error Breakdown Chart
        new Chart(document.getElementById('errorBreakdownChart'), {
            type: 'bar',
            data: {
                labels: [%s],
                datasets: [%s]
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                plugins: {
                    title: { display: true, text: 'Error Distribution by Category' },
                    legend: { position: 'bottom' },
                    emptyState: { enabled: %t, text: 'No errors recorded in this run' }
                },
                scales: {
                    x: { stacked: true },
                    y: { 
                        stacked: true,
                        beginAtZero: true,
                        title: { display: true, text: 'Number of Errors' }
                    }
                }
            }
        });

%s`,
		radarLabels, radarDatasets, !radarHasData,
		joinStrings(providerNames), latencyDatasets, latencyChartTitle, !latencyHasAnyData, latencyEmptyMessage,
		latencyRangesPayload, latencyXStacked, latencyYStacked, latencyYBounds.Min, latencyYBounds.Max,
		scatterDatasets, costMid, relevanceMid, costAxisType, costBounds.Min, costBounds.Max, costTickPrecision, relevanceBounds.Min, relevanceBounds.Max,
		speedQualityDatasets, speedMid, relevanceMid, speedAxisType, speedBounds.Min, speedBounds.Max, relevanceBounds.Min, relevanceBounds.Max,
		costSpeedDatasets, costMid, speedMid, costAxisType, costBounds.Min, costBounds.Max, costTickPrecision, speedAxisType, speedBounds.Min, speedBounds.Max,
		joinStrings(providerNames),
		errorDatasets, !errorHasData,
		heatmapScript)
}

func joinStrings(strs []string) string {
	const sep = ", "
	result := ""
	for i, s := range strs {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}

func formatFloatSlice(nums []float64) string {
	const sep = ", "
	result := ""
	for i, n := range nums {
		if i > 0 {
			result += sep
		}
		result += fmt.Sprintf("%.2f", n)
	}
	return result
}

func formatIntSlice(nums []int) string {
	const sep = ", "
	result := ""
	for i, n := range nums {
		if i > 0 {
			result += sep
		}
		result += fmt.Sprintf("%d", n)
	}
	return result
}

type axisBounds struct {
	Min float64
	Max float64
}

func hasAnyPositive(values []float64) bool {
	for _, v := range values {
		if v > 0 && !math.IsNaN(v) && !math.IsInf(v, 0) {
			return true
		}
	}
	return false
}

func hasMeaningfulSpread(values []float64, threshold float64) bool {
	minV, maxV, ok := minMaxFloat(values)
	if !ok {
		return false
	}
	return maxV-minV >= threshold
}

func minMaxFloat(values []float64) (float64, float64, bool) {
	hasValue := false
	minV := 0.0
	maxV := 0.0
	for _, v := range values {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			continue
		}
		if !hasValue {
			minV = v
			maxV = v
			hasValue = true
			continue
		}
		if v < minV {
			minV = v
		}
		if v > maxV {
			maxV = v
		}
	}
	return minV, maxV, hasValue
}

func calcPaddedBounds(values []float64, padRatio float64, minSpan float64) axisBounds {
	minV, maxV, ok := minMaxFloat(values)
	if !ok {
		return axisBounds{Min: 0, Max: 1}
	}
	if minSpan <= 0 {
		minSpan = 1
	}
	span := maxV - minV
	if span < minSpan {
		center := (minV + maxV) / 2
		minV = center - minSpan/2
		maxV = center + minSpan/2
		span = minSpan
	}
	pad := span * padRatio
	return axisBounds{Min: minV - pad, Max: maxV + pad}
}

func calcCenteredBounds(values []float64, midpoint float64, padRatio float64, minSpan float64) axisBounds {
	minV, maxV, ok := minMaxFloat(values)
	if !ok {
		halfSpan := math.Max(minSpan/2, 0.5)
		return axisBounds{Min: midpoint - halfSpan, Max: midpoint + halfSpan}
	}
	if minSpan <= 0 {
		minSpan = 1
	}
	halfSpan := math.Max(math.Abs(maxV-midpoint), math.Abs(midpoint-minV))
	halfSpan = math.Max(halfSpan, minSpan/2)
	halfSpan *= (1 + padRatio)
	if halfSpan == 0 {
		halfSpan = minSpan / 2
	}
	return axisBounds{Min: midpoint - halfSpan, Max: midpoint + halfSpan}
}

func allPositive(values []float64) bool {
	hasValue := false
	for _, v := range values {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			continue
		}
		hasValue = true
		if v <= 0 {
			return false
		}
	}
	return hasValue
}

func calcLogCenteredBounds(values []float64, midpoint float64, padRatio float64, minRatio float64, floor float64) (axisBounds, bool) {
	if midpoint <= 0 || !allPositive(values) {
		return axisBounds{}, false
	}
	minV, maxV, ok := minMaxFloat(values)
	if !ok || minV <= 0 || maxV <= 0 {
		return axisBounds{}, false
	}
	ratio := math.Max(maxV/midpoint, midpoint/minV)
	if ratio < 1 {
		ratio = 1
	}
	if minRatio < 1 {
		minRatio = 1
	}
	ratio *= (1 + padRatio)
	if ratio < minRatio {
		ratio = minRatio
	}

	minBound := midpoint / ratio
	maxBound := midpoint * ratio

	if floor > 0 && minBound < floor {
		minBound = floor
		maxBound = (midpoint * midpoint) / minBound
	}
	if maxBound <= minBound {
		maxBound = minBound * 10
	}
	return axisBounds{Min: minBound, Max: maxBound}, true
}

func shouldPreferLogScale(values []float64, midpoint float64, ratioThreshold float64) bool {
	if midpoint <= 0 || !allPositive(values) {
		return false
	}
	minV, maxV, ok := minMaxFloat(values)
	if !ok || minV <= 0 {
		return false
	}
	ratio := math.Max(maxV/midpoint, midpoint/minV)
	return ratio >= ratioThreshold
}

func clampAxisBounds(bounds axisBounds, minClamp float64, maxClamp float64) axisBounds { //nolint:unparam // lower clamp is currently 0 in callers but kept for clarity
	bounds.Min = math.Max(minClamp, bounds.Min)
	bounds.Max = math.Min(maxClamp, bounds.Max)
	if bounds.Max <= bounds.Min {
		center := (minClamp + maxClamp) / 2
		halfSpan := math.Max((maxClamp-minClamp)/10, 1)
		bounds.Min = math.Max(minClamp, center-halfSpan)
		bounds.Max = math.Min(maxClamp, center+halfSpan)
	}
	return bounds
}

func combineFloatSlices(slices ...[]float64) []float64 {
	total := 0
	for _, s := range slices {
		total += len(s)
	}
	combined := make([]float64, 0, total)
	for _, s := range slices {
		combined = append(combined, s...)
	}
	return combined
}

func splitScatterAxes(data []scatterData) (costs []float64, speeds []float64, relevances []float64) {
	costs = make([]float64, 0, len(data))
	speeds = make([]float64, 0, len(data))
	relevances = make([]float64, 0, len(data))
	for _, d := range data {
		costs = append(costs, d.CostPerResult)
		speeds = append(speeds, d.Speed)
		relevances = append(relevances, d.SearchRelevance)
	}
	return costs, speeds, relevances
}

// Radar data structures
type radarData struct {
	SuccessRate          float64
	SpeedScore           float64
	CostEfficiency       float64
	ContentScore         float64
	SearchRelevanceScore float64
}

// Latency distribution data
type latencyDist struct {
	Min float64
	P50 float64
	P95 float64
	Max float64
	Avg float64
}

// scatterData holds per-provider aggregates for trade-off scatter plots.
type scatterData struct {
	CostPerResult   float64
	SearchRelevance float64
	Speed           float64
	SuccessRate     float64
}

// computeQuadrantMidpoints returns median cost, speed, and relevance across
// providers. These values position the quadrant crosshairs in scatter charts.
func computeQuadrantMidpoints(data []scatterData) (costMid, speedMid, relevanceMid float64) {
	if len(data) == 0 {
		return 0, 0, 50
	}
	costs := make([]float64, len(data))
	speeds := make([]float64, len(data))
	relevances := make([]float64, len(data))
	for i, d := range data {
		costs[i] = d.CostPerResult
		speeds[i] = d.Speed
		relevances[i] = d.SearchRelevance
	}
	sort.Float64s(costs)
	sort.Float64s(speeds)
	sort.Float64s(relevances)

	median := func(s []float64) float64 {
		n := len(s)
		if n%2 == 0 {
			return (s[n/2-1] + s[n/2]) / 2
		}
		return s[n/2]
	}

	return median(costs), median(speeds), median(relevances)
}

// semanticRerankerData holds per-provider averages for search-only semantic/reranker scores.
type semanticRerankerData struct {
	AvgSemantic float64
	AvgReranker float64
	HasData     bool
}

// computeSemanticRerankerScores returns per-provider average semantic & reranker scores for search tests.
func (g *Generator) computeSemanticRerankerScores(providers []string) ([]semanticRerankerData, bool) {
	data := make([]semanticRerankerData, len(providers))
	anyData := false
	for i, p := range providers {
		results := g.collector.GetResultsByProvider(p)
		var semSum, rerSum float64
		var semN, rerN int
		for _, r := range results {
			if r.TestType != "search" || r.Skipped || !r.Success {
				continue
			}
			if r.SemanticScore > 0 {
				semSum += r.SemanticScore
				semN++
			}
			if r.RerankerScore > 0 {
				rerSum += r.RerankerScore
				rerN++
			}
		}
		if semN > 0 {
			data[i].AvgSemantic = semSum / float64(semN)
			data[i].HasData = true
			anyData = true
		}
		if rerN > 0 {
			data[i].AvgReranker = rerSum / float64(rerN)
			data[i].HasData = true
			anyData = true
		}
	}
	return data, anyData
}

// Error breakdown data
type errorBreakdown struct {
	Timeout         int
	RateLimit       int
	Auth            int
	Server5xx       int
	Client4xx       int
	Network         int
	Parse           int
	ContextCanceled int
	Validation      int
	Unknown         int
}

// Heatmap data
type heatmapCell struct {
	Provider string
	TestName string
	Score    float64
	Category string // excellent, good, acceptable, poor
}

// prepareRadarData creates normalized data for the radar chart
func (g *Generator) prepareRadarData(providers []string) []radarData {
	data := make([]radarData, len(providers))

	// Find max values for normalization
	var maxLatency float64
	var maxCost float64
	var maxContent float64
	var maxSearchRelevance float64

	summaries := make([]*metrics.Summary, len(providers))
	searchRelevanceScores := make([]float64, len(providers))
	for i, p := range providers {
		s := g.collector.ComputeSummary(p)
		byType := g.computeProviderQualityByTestType(p)
		summaries[i] = s
		searchRelevanceScores[i] = byType.Search.AvgQuality
		if float64(s.AvgLatency.Milliseconds()) > maxLatency {
			maxLatency = float64(s.AvgLatency.Milliseconds())
		}
		if s.CostPerResult > maxCost {
			maxCost = s.CostPerResult
		}
		if s.AvgContentLength > maxContent {
			maxContent = s.AvgContentLength
		}
		if searchRelevanceScores[i] > maxSearchRelevance {
			maxSearchRelevance = searchRelevanceScores[i]
		}
	}

	// Avoid division by zero
	if maxLatency == 0 {
		maxLatency = 1
	}
	if maxCost == 0 {
		maxCost = 1
	}
	if maxContent == 0 {
		maxContent = 1
	}
	if maxSearchRelevance == 0 {
		maxSearchRelevance = 100
	}

	for i, s := range summaries {
		data[i] = radarData{
			SuccessRate:          s.SuccessRate,
			SpeedScore:           (1 - float64(s.AvgLatency.Milliseconds())/maxLatency) * 100, // Inverse: faster = higher score
			CostEfficiency:       (1 - s.CostPerResult/maxCost) * 100,                         // Inverse: cheaper = higher score
			ContentScore:         (s.AvgContentLength / maxContent) * 100,
			SearchRelevanceScore: (searchRelevanceScores[i] / maxSearchRelevance) * 100,
		}
	}

	return data
}

// prepareLatencyDistributionData creates latency distribution data
func (g *Generator) prepareLatencyDistributionData(providers []string) []latencyDist {
	data := make([]latencyDist, len(providers))

	for i, p := range providers {
		s := g.collector.ComputeSummary(p)
		data[i] = latencyDist{
			Min: float64(s.MinLatency.Milliseconds()),
			P50: float64(s.P50Latency.Milliseconds()),
			P95: float64(s.P95Latency.Milliseconds()),
			Max: float64(s.MaxLatency.Milliseconds()),
			Avg: float64(s.AvgLatency.Milliseconds()),
		}
	}

	return data
}

// prepareScatterData creates data for scatter plots
func (g *Generator) prepareScatterData(providers []string) []scatterData {
	data := make([]scatterData, len(providers))

	for i, p := range providers {
		s := g.collector.ComputeSummary(p)

		data[i] = scatterData{
			CostPerResult:   s.CostPerResult,
			SearchRelevance: g.computeProviderQualityByTestType(p).Search.AvgQuality,
			Speed:           float64(s.AvgLatency.Milliseconds()),
			SuccessRate:     s.SuccessRate,
		}
	}

	return data
}

// prepareErrorBreakdownData creates error categorization data
func (g *Generator) prepareErrorBreakdownData(providers []string) []errorBreakdown {
	data := make([]errorBreakdown, len(providers))

	for i, p := range providers {
		results := g.collector.GetResultsByProvider(p)
		eb := &data[i]

		for _, r := range results {
			if r.Success {
				continue
			}

			category := r.ErrorCategory
			if category == "" && r.Error != "" {
				category = "unknown"
			}

			switch category {
			case "timeout":
				eb.Timeout++
			case "rate_limit":
				eb.RateLimit++
			case "auth":
				eb.Auth++
			case "server_5xx", "server":
				eb.Server5xx++
			case "client_4xx", "client":
				eb.Client4xx++
			case "network":
				eb.Network++
			case "parse":
				eb.Parse++
			case "context_canceled":
				eb.ContextCanceled++
			case "validation":
				eb.Validation++
			default:
				eb.Unknown++
			}
		}
	}

	return data
}

// prepareHeatmapData creates performance heatmap data
func (g *Generator) prepareHeatmapData(_ []string) []heatmapCell {
	var cells []heatmapCell
	tests := g.collector.GetAllTests()

	for _, testName := range tests {
		results := g.collector.GetResultsByTest(testName)

		// Find max values for normalization
		var maxLatency time.Duration
		var maxCost float64
		for _, r := range results {
			if r.Success {
				if r.Latency > maxLatency {
					maxLatency = r.Latency
				}
				if r.CostUSD > maxCost {
					maxCost = r.CostUSD
				}
			}
		}

		if maxLatency == 0 {
			maxLatency = 1
		}
		if maxCost == 0 {
			maxCost = 1
		}

		for _, r := range results {
			score := calculateCompositeScore(r, maxLatency, maxCost)
			cells = append(cells, heatmapCell{
				Provider: r.Provider,
				TestName: testName,
				Score:    score,
				Category: scoreToCategory(score),
			})
		}
	}

	return cells
}

func calculateCompositeScore(r metrics.Result, maxLatency time.Duration, maxCost float64) float64 {
	if !r.Success {
		return 0
	}

	// Combine success (40%), speed (30%), cost efficiency (30%)
	successScore := 100.0
	speedScore := (1 - float64(r.Latency)/float64(maxLatency)) * 100
	costScore := (1 - r.CostUSD/maxCost) * 100

	return successScore*0.4 + speedScore*0.3 + costScore*0.3
}

func scoreToCategory(score float64) string {
	switch {
	case score >= 80:
		return "excellent"
	case score >= 60:
		return "good"
	case score >= 40:
		return "acceptable"
	case score > 0:
		return "poor"
	default:
		return "na"
	}
}

// Helper functions for chart data formatting
func latencyDistsToMinP50(dists []latencyDist) []float64 {
	result := make([]float64, len(dists))
	for i, d := range dists {
		result[i] = d.P50 - d.Min
	}
	return result
}

func latencyDistsToP50P95(dists []latencyDist) []float64 {
	result := make([]float64, len(dists))
	for i, d := range dists {
		result[i] = d.P95 - d.P50
	}
	return result
}

func latencyDistsToP95Max(dists []latencyDist) []float64 {
	result := make([]float64, len(dists))
	for i, d := range dists {
		result[i] = d.Max - d.P95
	}
	return result
}

func latencyDistsToAvg(dists []latencyDist) []float64 {
	result := make([]float64, len(dists))
	for i, d := range dists {
		result[i] = d.Avg
	}
	return result
}

func latencyDistsToMax(dists []latencyDist) []float64 {
	result := make([]float64, len(dists))
	for i, d := range dists {
		result[i] = d.Max
	}
	return result
}

func (g *Generator) formatLatencyRanges(dists []latencyDist) string {
	var parts []string
	for _, d := range dists {
		parts = append(parts, fmt.Sprintf("['%s', '%s', '%s']", formatLatency(d.Min), formatLatency(d.P50), formatLatency(d.Max)))
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func formatLatency(ms float64) string {
	return fmt.Sprintf("%.0fms", ms)
}

func (g *Generator) buildErrorDatasets(providers []string, data []errorBreakdown, _ []string) string {
	errorTypes := []struct {
		name  string
		color string
	}{
		{"Timeout", "'#e74c3c'"},
		{"Rate Limit", "'#f39c12'"},
		{"Auth", "'#9b59b6'"},
		{"Server 5xx", "'#e67e22'"},
		{"Client 4xx", "'#3498db'"},
		{"Network", "'#1abc9c'"},
		{"Parse", "'#95a5a6'"},
		{"Context Canceled", "'#34495e'"},
		{"Validation", "'#16a085'"},
		{"Unknown", "'#7f8c8d'"},
	}

	datasets := ""
	for _, et := range errorTypes {
		values := make([]int, len(providers))
		hasValues := false
		for i := range providers {
			switch et.name {
			case "Timeout":
				values[i] = data[i].Timeout
			case "Rate Limit":
				values[i] = data[i].RateLimit
			case "Auth":
				values[i] = data[i].Auth
			case "Server 5xx":
				values[i] = data[i].Server5xx
			case "Client 4xx":
				values[i] = data[i].Client4xx
			case "Network":
				values[i] = data[i].Network
			case "Parse":
				values[i] = data[i].Parse
			case "Context Canceled":
				values[i] = data[i].ContextCanceled
			case "Validation":
				values[i] = data[i].Validation
			case "Unknown":
				values[i] = data[i].Unknown
			}
			if values[i] > 0 {
				hasValues = true
			}
		}

		if hasValues {
			datasets += fmt.Sprintf(`{
                    label: '%s',
                    data: [%s],
                    backgroundColor: %s,
                    borderRadius: 4
                },`, et.name, formatIntSlice(values), et.color)
		}
	}

	return strings.TrimSuffix(datasets, ",")
}

func (g *Generator) generateHeatmapScript(providers []string, cells []heatmapCell) string {
	tests := g.collector.GetAllTests()
	sort.Strings(tests)

	// Build heatmap grid HTML
	var html strings.Builder

	html.WriteString(fmt.Sprintf(`<div class="heatmap-grid" style="grid-template-columns: 150px repeat(%d, 1fr);">`, len(tests)))

	// Header row
	html.WriteString(`<div class="heatmap-header"></div>`)
	for _, test := range tests {
		html.WriteString(fmt.Sprintf(`<div class="heatmap-header">%s</div>`, test))
	}

	// Data rows
	for _, provider := range providers {
		html.WriteString(fmt.Sprintf(`<div class="heatmap-provider"><span class="provider-badge provider-%s">%s</span></div>`, provider, capitalize(provider)))

		for _, test := range tests {
			// Find cell for this provider-test combination
			var cell *heatmapCell
			for i := range cells {
				if cells[i].Provider == provider && cells[i].TestName == test {
					cell = &cells[i]
					break
				}
			}

			if cell == nil {
				html.WriteString(`<div class="heatmap-cell heatmap-na">N/A</div>`)
			} else {
				scoreText := fmt.Sprintf("%.0f", cell.Score)
				html.WriteString(fmt.Sprintf(`<div class="heatmap-cell heatmap-%s" title="Score: %s">%s</div>`, cell.Category, scoreText, scoreText))
			}
		}
	}

	html.WriteString(`</div>`)

	return fmt.Sprintf(`
        // Performance Heatmap
        document.getElementById('performanceHeatmap').innerHTML = %s;`, "`"+html.String()+"`")
}

// generateAdvancedAnalyticsSection creates the advanced analytics HTML section
func (g *Generator) generateAdvancedAnalyticsSection() string {
	providers := g.collector.GetAllProviders()
	if len(providers) < 2 {
		return ""
	}

	return `        <div class="section">
            <h2>Advanced Analytics</h2>
        </div>

        <div class="section">
            <h2>Provider Performance Profile</h2>
            <p class="quality-note">All dimensions normalized to 0-100 scale (higher is better). Speed and cost are inverted so higher = faster/cheaper.</p>
            <div class="chart-container">
                <div class="chart-wrapper" style="height: 450px;">
                    <canvas id="radarChart"></canvas>
                </div>
            </div>
        </div>

        <div class="section">
            <h2>Latency Distribution</h2>
            <p class="quality-note">Stacked bars show Min→P50, P50→P95, and P95→Max latency ranges per provider.</p>
            <div class="chart-container">
                <div class="chart-wrapper">
                    <canvas id="latencyDistChart"></canvas>
                </div>
            </div>
        </div>

        <div class="section">
            <h2>Trade-off Analysis</h2>
            <p class="quality-note">Green quadrant = sweet spot (low cost/fast + high quality). Red quadrant = least desirable. Crosshairs at provider median values.</p>
            <div class="chart-grid">
                <div class="chart-container">
                    <div class="chart-wrapper">
                        <canvas id="costQualityScatter"></canvas>
                    </div>
                </div>
                <div class="chart-container">
                    <div class="chart-wrapper">
                        <canvas id="speedQualityScatter"></canvas>
                    </div>
                </div>
                <div class="chart-container">
                    <div class="chart-wrapper">
                        <canvas id="costSpeedScatter"></canvas>
                    </div>
                </div>
            </div>
        </div>

` + g.generateHeatmapSection() + g.generateErrorBreakdownSection()
}

// generateHeatmapSection creates the performance heatmap HTML
func (g *Generator) generateHeatmapSection() string {
	return `        <div class="section">
            <h2>Performance Heatmap by Test Scenario</h2>
            <div class="chart-container">
                <div id="performanceHeatmap"></div>
            </div>
        </div>

`
}

// generateErrorBreakdownSection creates the error breakdown chart HTML
func (g *Generator) generateErrorBreakdownSection() string {
	return `        <div class="section">
            <h2>Error Breakdown by Category</h2>
            <div class="chart-container">
                <div class="chart-wrapper">
                    <canvas id="errorBreakdownChart"></canvas>
                </div>
            </div>
        </div>

`
}
