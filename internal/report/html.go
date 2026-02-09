// Package report generates HTML, Markdown, and JSON reports from benchmark results.
package report

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/lamim/search-api-bench/internal/metrics"
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
    <title>Search API Benchmark Report</title>
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
        .advanced-section { background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); color: white; padding: 15px 20px; margin: 40px 0 30px 0; border-radius: 8px; }
        .advanced-section h2 { color: white; border-bottom: 2px solid rgba(255,255,255,0.3); margin-bottom: 5px; }
        .advanced-subtitle { font-size: 0.9em; opacity: 0.9; }
        .chart-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(450px, 1fr)); gap: 20px; margin-bottom: 20px; }
        .chart-grid .chart-container { margin-bottom: 0; }
        .chart-grid .chart-wrapper { height: 350px; }
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
        <h1>Search API Benchmark Report</h1>
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

` + g.generateQualitySection() + g.generateQualityByTestTypeSection() + g.generateAdvancedAnalyticsSection() + `
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
			if r.QualityScore > 0 {
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
                    </tr>`,
			provider,
			capitalize(provider),
			formatQualityValue(byType.Search),
			formatQualityCoverage(byType.Search),
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
				if r.QualityScore > 0 {
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
                        beginAtZero: true,
                        max: 100,
                        title: { display: true, text: 'Score' }
                    }
                }
            }
        });
`, joinStrings(providerNames), formatFloatSlice(searchRelevanceScores), joinStrings(colors), formatFloatSlice(extractHeuristicScores), formatFloatSlice(crawlHeuristicScores))
	}

	advancedScripts := g.generateAdvancedChartScripts(providers, providerNames, colors, baseColors)

	return scoreChartScript + fmt.Sprintf(`
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
                    y: { beginAtZero: true, title: { display: true, text: 'Milliseconds' } }
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
                        beginAtZero: true,
                        max: 100,
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
                        beginAtZero: true, 
                        title: { display: true, text: 'USD ($)' },
                        ticks: {
                            callback: function(value) {
                                return '$' + value.toFixed(4);
                            }
                        }
                    }
                }
            }
        });

%s`, joinStrings(providerNames), formatFloatSlice(avgLatencies), joinStrings(colors),
		joinStrings(providerNames), formatFloatSlice(successRates), joinStrings(colors),
		joinStrings(providerNames), formatFloatSlice(totalCostUSD), joinStrings(colors),
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

	// Build radar datasets
	radarDatasets := ""
	for i, p := range providers {
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
			formatFloatSlice([]float64{radars[i].SuccessRate, radars[i].SpeedScore, radars[i].CostEfficiency, radars[i].ContentScore, radars[i].SearchRelevanceScore}),
			baseColors[i%len(baseColors)], baseColors[i%len(baseColors)], baseColors[i%len(baseColors)], baseColors[i%len(baseColors)])
	}
	radarDatasets = strings.TrimSuffix(radarDatasets, ",")

	// Latency distribution data prepared (used in chart script via latencyDists)

	// Build scatter datasets (one per provider)
	scatterDatasets := ""
	for i, p := range providers {
		color := baseColors[i%len(baseColors)]
		scatterDatasets += fmt.Sprintf(`{
                    label: '%s',
                    data: [{x: %f, y: %f, r: %f}],
                    backgroundColor: %s,
                    borderColor: %s,
                    borderWidth: 2
                },`, capitalize(p), scatterData[i].CostPerResult, scatterData[i].SearchRelevance, scatterData[i].BubbleSize, color, color)
	}
	scatterDatasets = strings.TrimSuffix(scatterDatasets, ",")

	// Build speed-quality scatter
	speedQualityDatasets := ""
	for i, p := range providers {
		color := baseColors[i%len(baseColors)]
		speedQualityDatasets += fmt.Sprintf(`{
                    label: '%s',
                    data: [{x: %f, y: %f, r: %f}],
                    backgroundColor: %s,
                    borderColor: %s,
                    borderWidth: 2
                },`, capitalize(p), scatterData[i].Speed, scatterData[i].SearchRelevance, scatterData[i].BubbleSize, color, color)
	}
	speedQualityDatasets = strings.TrimSuffix(speedQualityDatasets, ",")

	// Build cost-speed scatter
	costSpeedDatasets := ""
	for i, p := range providers {
		color := baseColors[i%len(baseColors)]
		costSpeedDatasets += fmt.Sprintf(`{
                    label: '%s',
                    data: [{x: %f, y: %f, r: %f}],
                    backgroundColor: %s,
                    borderColor: %s,
                    borderWidth: 2
                },`, capitalize(p), scatterData[i].CostPerResult, scatterData[i].Speed, scatterData[i].BubbleSize, color, color)
	}
	costSpeedDatasets = strings.TrimSuffix(costSpeedDatasets, ",")

	// Build error breakdown datasets
	errorDatasets := g.buildErrorDatasets(providers, errorData, baseColors)

	// Heatmap generation
	heatmapScript := g.generateHeatmapScript(providers, heatmapData)

	searchRelevanceRadarLabel := ""
	if showQuality {
		searchRelevanceRadarLabel = ", 'Search Relevance'"
	}

	return fmt.Sprintf(`
        // Radar Chart - Provider Performance Profile
        new Chart(document.getElementById('radarChart'), {
            type: 'radar',
            data: {
                labels: ['Success Rate', 'Speed Score', 'Cost Efficiency', 'Content Volume'%s],
                datasets: [%s]
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                plugins: {
                    title: { display: true, text: 'Multi-Dimensional Performance Comparison (0-100, higher is better)' },
                    legend: { position: 'bottom' }
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
                datasets: [
                    {
                        label: 'Min-P50 Range',
                        data: [%s],
                        backgroundColor: %s,
                        borderRadius: 4
                    },
                    {
                        label: 'P50-P95 Range',
                        data: [%s],
                        backgroundColor: %s,
                        borderRadius: 4
                    },
                    {
                        label: 'P95-Max Range',
                        data: [%s],
                        backgroundColor: %s,
                        borderRadius: 4
                    }
                ]
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                plugins: {
                    title: { display: true, text: 'Latency Distribution: Min, P50, P95, Max (ms)' },
                    legend: { display: true, position: 'bottom' },
                    tooltip: {
                        callbacks: {
                            label: function(context) {
                                const datasetIndex = context.datasetIndex;
                                const dataIndex = context.dataIndex;
                                const ranges = [%s];
                                const range = ranges[dataIndex];
                                return context.dataset.label + ': ' + range[datasetIndex] + 'ms';
                            }
                        }
                    }
                },
                scales: {
                    x: { stacked: true },
                    y: { 
                        stacked: true,
                        beginAtZero: true,
                        title: { display: true, text: 'Milliseconds' }
                    }
                }
            }
        });

        // Cost vs Search Relevance Scatter Plot
        new Chart(document.getElementById('costQualityScatter'), {
            type: 'bubble',
            data: { datasets: [%s] },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                layout: { padding: 30 },
                plugins: {
                    title: { display: true, text: 'Cost vs Search Relevance (Bubble size = Results count)' },
                    legend: { display: true, position: 'bottom' },
                    tooltip: {
                        callbacks: {
                            label: function(context) {
                                return context.dataset.label + ': Cost $' + context.raw.x.toFixed(4) + ', Search relevance ' + context.raw.y.toFixed(1);
                            }
                        }
                    }
                },
                scales: {
                    x: {
                        title: { display: true, text: 'Cost per Result (USD)' },
                        ticks: { callback: function(v) { return '$' + v.toFixed(4); } }
                    },
                    y: {
                        title: { display: true, text: 'Search Relevance' },
                        min: 0,
                        max: 110
                    }
                }
            }
        });

        // Speed vs Search Relevance Scatter Plot
        new Chart(document.getElementById('speedQualityScatter'), {
            type: 'bubble',
            data: { datasets: [%s] },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                layout: { padding: 30 },
                plugins: {
                    title: { display: true, text: 'Speed vs Search Relevance (Bubble size = Results count)' },
                    legend: { display: true, position: 'bottom' },
                    tooltip: {
                        callbacks: {
                            label: function(context) {
                                return context.dataset.label + ': Speed ' + context.raw.x.toFixed(0) + 'ms, Search relevance ' + context.raw.y.toFixed(1);
                            }
                        }
                    }
                },
                scales: {
                    x: {
                        title: { display: true, text: 'Average Latency (ms)' },
                        reverse: true
                    },
                    y: {
                        title: { display: true, text: 'Search Relevance' },
                        min: 0,
                        max: 110
                    }
                }
            }
        });

        // Cost vs Speed Scatter Plot
        new Chart(document.getElementById('costSpeedScatter'), {
            type: 'bubble',
            data: { datasets: [%s] },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                layout: { padding: 30 },
                plugins: {
                    title: { display: true, text: 'Cost vs Speed Trade-off (Bubble size = Success Rate)' },
                    legend: { display: true, position: 'bottom' },
                    tooltip: {
                        callbacks: {
                            label: function(context) {
                                return context.dataset.label + ': Cost $' + context.raw.x.toFixed(4) + ', Speed ' + context.raw.y.toFixed(0) + 'ms';
                            }
                        }
                    }
                },
                scales: {
                    x: {
                        title: { display: true, text: 'Cost per Result (USD)' },
                        ticks: { callback: function(v) { return '$' + v.toFixed(4); } }
                    },
                    y: {
                        title: { display: true, text: 'Average Latency (ms)' },
                        reverse: true
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
                    legend: { position: 'bottom' }
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
		searchRelevanceRadarLabel, radarDatasets,
		joinStrings(providerNames),
		formatFloatSlice(latencyDistsToMinP50(latencyDists)), "'rgba(52, 152, 219, 0.8)'",
		formatFloatSlice(latencyDistsToP50P95(latencyDists)), "'rgba(46, 204, 113, 0.8)'",
		formatFloatSlice(latencyDistsToP95Max(latencyDists)), "'rgba(231, 76, 60, 0.8)'",
		g.formatLatencyRanges(latencyDists),
		scatterDatasets,
		speedQualityDatasets,
		costSpeedDatasets,
		joinStrings(providerNames),
		errorDatasets,
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

// Scatter plot data
type scatterData struct {
	CostPerResult   float64
	SearchRelevance float64
	Speed           float64
	BubbleSize      float64
	SuccessRate     float64
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
		results := g.collector.GetResultsByProvider(p)

		// Calculate total successful results for bubble size
		var totalResults int
		for _, r := range results {
			if r.Success {
				totalResults += r.ResultsCount
			}
		}

		data[i] = scatterData{
			CostPerResult:   s.CostPerResult,
			SearchRelevance: g.computeProviderQualityByTestType(p).Search.AvgQuality,
			Speed:           float64(s.AvgLatency.Milliseconds()),
			BubbleSize:      float64(totalResults) * 2, // Scale bubble size
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

	return `        <div class="advanced-section">
            <h2>Advanced Analytics</h2>
            <div class="advanced-subtitle">Multi-dimensional performance analysis & comparative insights</div>
        </div>

        <div class="section">
            <h2>Provider Performance Profile (Radar Chart)</h2>
            <div class="chart-container">
                <div class="chart-wrapper" style="height: 450px;">
                    <canvas id="radarChart"></canvas>
                </div>
            </div>
        </div>

        <div class="section">
            <h2>Latency Distribution Analysis</h2>
            <div class="chart-container">
                <div class="chart-wrapper">
                    <canvas id="latencyDistChart"></canvas>
                </div>
            </div>
        </div>

        <div class="section">
            <h2>Cost vs Search Relevance</h2>
            <div class="chart-container">
                <div class="chart-wrapper" style="height: 400px;">
                    <canvas id="costQualityScatter"></canvas>
                </div>
            </div>
        </div>

        <div class="section">
            <h2>Speed vs Search Relevance</h2>
            <div class="chart-container">
                <div class="chart-wrapper" style="height: 400px;">
                    <canvas id="speedQualityScatter"></canvas>
                </div>
            </div>
        </div>

        <div class="section">
            <h2>Cost vs Speed Analysis</h2>
            <div class="chart-container">
                <div class="chart-wrapper" style="height: 400px;">
                    <canvas id="costSpeedScatter"></canvas>
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
