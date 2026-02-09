// Package report generates HTML, Markdown, and JSON reports from benchmark results.
package report

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
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
            <h2>Cost Efficiency - Total Credits</h2>
            <div class="chart-container">
                <div class="chart-wrapper">
                    <canvas id="creditsChart"></canvas>
                </div>
            </div>
        </div>

        <div class="section">
            <h2>Cost Efficiency - Credits per Result</h2>
            <div class="chart-container">
                <div class="chart-wrapper">
                    <canvas id="creditsPerResultChart"></canvas>
                </div>
            </div>
        </div>

        <div class="section">
            <h2>Content Efficiency - Characters per Credit</h2>
            <div class="chart-container">
                <div class="chart-wrapper">
                    <canvas id="charsPerCreditChart"></canvas>
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

        <div class="section">
            <h2>Detailed Results</h2>
            <table>
                <thead>
                    <tr>
                        <th>Test</th>
                        <th>Provider</th>
                        <th>Status</th>
                        <th>Latency</th>
                        <th>Credits</th>
                        <th>Results/Content</th>
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

func (g *Generator) generateTableRows() string {
	var rows string
	tests := g.collector.GetAllTests()

	formatLatency := func(d time.Duration) string {
		return fmt.Sprintf("%.0fms", float64(d.Milliseconds()))
	}

	for _, testName := range tests {
		results := g.collector.GetResultsByTest(testName)
		for _, r := range results {
			status := `<span class="success">✓ Pass</span>`
			if !r.Success {
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

			rows += fmt.Sprintf(`                    <tr>
                        <td>%s</td>
                        <td><span class="provider-badge %s">%s</span></td>
                        <td>%s</td>
                        <td>%s</td>
                        <td>%d</td>
                        <td>%s</td>
                    </tr>
`, testName, providerClass, capitalize(r.Provider), status, formatLatency(r.Latency), r.CreditsUsed, details)
		}
	}

	return rows
}

func (g *Generator) generateChartScripts() string {
	providers := g.collector.GetAllProviders()

	// Prepare data for charts
	providerNames := make([]string, len(providers))
	avgLatencies := make([]float64, len(providers))
	totalCredits := make([]int, len(providers))
	successRates := make([]float64, len(providers))
	creditsPerResult := make([]float64, len(providers))
	charsPerCredit := make([]float64, len(providers))
	resultsPerCredit := make([]float64, len(providers))

	baseColors := []string{"'#ff6b35'", "'#3498db'", "'#27ae60'", "'#9b59b6'", "'#e74c3c'", "'#f39c12'", "'#1abc9c'"}
	colors := make([]string, len(providers))
	for i := range providers {
		colors[i] = baseColors[i%len(baseColors)]
	}

	for i, provider := range providers {
		summary := g.collector.ComputeSummary(provider)
		providerNames[i] = "'" + capitalize(provider) + "'"
		avgLatencies[i] = float64(summary.AvgLatency.Milliseconds())
		totalCredits[i] = summary.TotalCreditsUsed
		if summary.TotalTests > 0 {
			successRates[i] = float64(summary.SuccessfulTests) / float64(summary.TotalTests) * 100
		}
		// Efficiency metrics
		creditsPerResult[i] = summary.CreditsPerResult
		charsPerCredit[i] = summary.CharsPerCredit
		resultsPerCredit[i] = summary.ResultsPerCredit
	}

	return fmt.Sprintf(`
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

        // Credits Chart
        new Chart(document.getElementById('creditsChart'), {
            type: 'bar',
            data: {
                labels: [%s],
                datasets: [{
                    label: 'Total Credits Used',
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
                    title: { display: true, text: 'Total API Credits Consumed' }
                },
                scales: {
                    y: { beginAtZero: true, title: { display: true, text: 'Credits' } }
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

        // Efficiency: Credits per Result (lower is better)
        new Chart(document.getElementById('creditsPerResultChart'), {
            type: 'bar',
            data: {
                labels: [%s],
                datasets: [{
                    label: 'Credits per Result',
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
                    title: { display: true, text: 'Cost Efficiency: Credits per Result (lower is better)' }
                },
                scales: {
                    y: { beginAtZero: true, title: { display: true, text: 'Credits' } }
                }
            }
        });

        // Efficiency: Content per Credit (higher is better)
        new Chart(document.getElementById('charsPerCreditChart'), {
            type: 'bar',
            data: {
                labels: [%s],
                datasets: [{
                    label: 'Characters per Credit',
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
                    title: { display: true, text: 'Content Efficiency: Characters per Credit (higher is better)' }
                },
                scales: {
                    y: { beginAtZero: true, title: { display: true, text: 'Characters' } }
                }
            }
        });
`, joinStrings(providerNames), formatFloatSlice(avgLatencies), joinStrings(colors),
		joinStrings(providerNames), formatIntSlice(totalCredits), joinStrings(colors),
		joinStrings(providerNames), formatFloatSlice(successRates), joinStrings(colors),
		joinStrings(providerNames), formatFloatSlice(creditsPerResult), joinStrings(colors),
		joinStrings(providerNames), formatFloatSlice(charsPerCredit), joinStrings(colors))
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
