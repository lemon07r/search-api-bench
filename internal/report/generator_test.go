package report

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lamim/search-api-bench/internal/metrics"
)

func setupMockCollector() *metrics.Collector {
	c := metrics.NewCollector()

	// Add test results for provider1
	c.AddResult(metrics.Result{
		TestName:      "Search Test",
		Provider:      "provider1",
		TestType:      "search",
		Success:       true,
		Latency:       100 * time.Millisecond,
		CreditsUsed:   1,
		ContentLength: 500,
		ResultsCount:  5,
		Timestamp:     time.Now(),
	})
	c.AddResult(metrics.Result{
		TestName:      "Extract Test",
		Provider:      "provider1",
		TestType:      "extract",
		Success:       true,
		Latency:       200 * time.Millisecond,
		CreditsUsed:   1,
		ContentLength: 1000,
		Timestamp:     time.Now(),
	})
	c.AddResult(metrics.Result{
		TestName:      "Search Test",
		Provider:      "provider2",
		TestType:      "search",
		Success:       true,
		Latency:       150 * time.Millisecond,
		CreditsUsed:   2,
		ContentLength: 600,
		ResultsCount:  6,
		Timestamp:     time.Now(),
	})
	c.AddResult(metrics.Result{
		TestName:  "Failed Test",
		Provider:  "provider1",
		TestType:  "search",
		Success:   false,
		Error:     "timeout",
		Latency:   500 * time.Millisecond,
		Timestamp: time.Now(),
	})

	return c
}

func TestGenerateMarkdown_SingleProvider(t *testing.T) {
	c := metrics.NewCollector()
	c.AddResult(metrics.Result{
		TestName:      "Test 1",
		Provider:      "single",
		TestType:      "search",
		Success:       true,
		Latency:       100 * time.Millisecond,
		CreditsUsed:   1,
		ContentLength: 500,
		ResultsCount:  5,
	})

	tmpDir := t.TempDir()
	gen := NewGenerator(c, tmpDir)

	err := gen.GenerateMarkdown()
	if err != nil {
		t.Fatalf("GenerateMarkdown failed: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(tmpDir, "report.md"))
	if err != nil {
		t.Fatalf("failed to read report: %v", err)
	}

	if !strings.Contains(string(content), "single") {
		t.Error("report should contain provider name")
	}
	if !strings.Contains(string(content), "Search API Benchmark Report") {
		t.Error("report should contain title")
	}
}

func TestGenerateMarkdown_MultipleProviders(t *testing.T) {
	c := setupMockCollector()
	tmpDir := t.TempDir()
	gen := NewGenerator(c, tmpDir)

	err := gen.GenerateMarkdown()
	if err != nil {
		t.Fatalf("GenerateMarkdown failed: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(tmpDir, "report.md"))
	if err != nil {
		t.Fatalf("failed to read report: %v", err)
	}

	// Check for comparison section
	if !strings.Contains(string(content), "Provider Comparison") {
		t.Error("report should contain comparison section for multiple providers")
	}
	if !strings.Contains(string(content), "provider1") {
		t.Error("report should contain provider1")
	}
	if !strings.Contains(string(content), "provider2") {
		t.Error("report should contain provider2")
	}
}

func TestGenerateMarkdown_ComparisonMath(t *testing.T) {
	c := metrics.NewCollector()
	// Provider1: faster, more credits
	c.AddResult(metrics.Result{
		TestName:    "Test",
		Provider:    "fast",
		Success:     true,
		Latency:     100 * time.Millisecond,
		CreditsUsed: 2,
	})
	// Provider2: slower, fewer credits
	c.AddResult(metrics.Result{
		TestName:    "Test",
		Provider:    "slow",
		Success:     true,
		Latency:     200 * time.Millisecond,
		CreditsUsed: 1,
	})

	tmpDir := t.TempDir()
	gen := NewGenerator(c, tmpDir)

	err := gen.GenerateMarkdown()
	if err != nil {
		t.Fatalf("GenerateMarkdown failed: %v", err)
	}

	content, _ := os.ReadFile(filepath.Join(tmpDir, "report.md"))

	// Check for speed comparison (fast should be mentioned as faster)
	if !strings.Contains(string(content), "faster") {
		t.Error("report should contain speed comparison")
	}
	// Check for cost comparison (USD)
	if !strings.Contains(string(content), "cost") && !strings.Contains(string(content), "USD") {
		t.Error("report should contain cost comparison")
	}
}

func TestGenerateMarkdown_PairwiseDiffsArePositive(t *testing.T) {
	c := metrics.NewCollector()
	c.AddResult(metrics.Result{
		TestName:      "Test",
		Provider:      "a",
		TestType:      "search",
		Success:       true,
		Latency:       100 * time.Millisecond,
		CreditsUsed:   1,
		ContentLength: 100,
		QualityScore:  50,
	})
	c.AddResult(metrics.Result{
		TestName:      "Test",
		Provider:      "b",
		TestType:      "search",
		Success:       true,
		Latency:       80 * time.Millisecond,
		CreditsUsed:   2,
		ContentLength: 200,
		QualityScore:  75,
	})

	tmpDir := t.TempDir()
	gen := NewGenerator(c, tmpDir)
	if err := gen.GenerateMarkdown(); err != nil {
		t.Fatalf("GenerateMarkdown failed: %v", err)
	}

	content, _ := os.ReadFile(filepath.Join(tmpDir, "report.md"))
	report := string(content)
	if strings.Contains(report, "returns -") {
		t.Fatal("pairwise content comparison should not be negative")
	}
	if strings.Contains(report, "has -") {
		t.Fatal("pairwise quality comparison should not be negative")
	}
}

func TestGenerateMarkdown_WritesFile(t *testing.T) {
	c := setupMockCollector()
	tmpDir := t.TempDir()
	gen := NewGenerator(c, tmpDir)

	err := gen.GenerateMarkdown()
	if err != nil {
		t.Fatalf("GenerateMarkdown failed: %v", err)
	}

	// Check file exists
	_, err = os.Stat(filepath.Join(tmpDir, "report.md"))
	if err != nil {
		t.Errorf("report.md was not created: %v", err)
	}
}

func TestGenerateJSON_Structure(t *testing.T) {
	c := setupMockCollector()
	tmpDir := t.TempDir()
	gen := NewGenerator(c, tmpDir)

	err := gen.GenerateJSON()
	if err != nil {
		t.Fatalf("GenerateJSON failed: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(tmpDir, "report.json"))
	if err != nil {
		t.Fatalf("failed to read report: %v", err)
	}

	var data map[string]interface{}
	if err := json.Unmarshal(content, &data); err != nil {
		t.Fatalf("report is not valid JSON: %v", err)
	}

	// Check required keys
	requiredKeys := []string{"timestamp", "providers", "tests", "results", "summaries"}
	for _, key := range requiredKeys {
		if _, ok := data[key]; !ok {
			t.Errorf("report missing key: %s", key)
		}
	}
}

func TestGenerateJSON_Summaries(t *testing.T) {
	c := setupMockCollector()
	tmpDir := t.TempDir()
	gen := NewGenerator(c, tmpDir)

	err := gen.GenerateJSON()
	if err != nil {
		t.Fatalf("GenerateJSON failed: %v", err)
	}

	content, _ := os.ReadFile(filepath.Join(tmpDir, "report.json"))
	var data map[string]interface{}
	json.Unmarshal(content, &data)

	summaries, ok := data["summaries"].(map[string]interface{})
	if !ok {
		t.Fatal("summaries should be a map")
	}

	if _, ok := summaries["provider1"]; !ok {
		t.Error("summaries should contain provider1")
	}
	if _, ok := summaries["provider2"]; !ok {
		t.Error("summaries should contain provider2")
	}
}

func TestGenerateJSON_SummariesIncludeQualityCoverageFields(t *testing.T) {
	c := metrics.NewCollector()
	c.AddResult(metrics.Result{
		TestName:      "Search",
		Provider:      "provider1",
		TestType:      "search",
		Success:       true,
		Latency:       100 * time.Millisecond,
		CreditsUsed:   1,
		ContentLength: 10,
		ResultsCount:  1,
		QualityScore:  80,
	})
	c.AddResult(metrics.Result{
		TestName:      "Search",
		Provider:      "provider1",
		TestType:      "search",
		Success:       false,
		Latency:       100 * time.Millisecond,
		CreditsUsed:   1,
		ContentLength: 10,
		ResultsCount:  1,
	})

	tmpDir := t.TempDir()
	gen := NewGenerator(c, tmpDir)
	if err := gen.GenerateJSON(); err != nil {
		t.Fatalf("GenerateJSON failed: %v", err)
	}

	content, _ := os.ReadFile(filepath.Join(tmpDir, "report.json"))
	var data map[string]interface{}
	if err := json.Unmarshal(content, &data); err != nil {
		t.Fatalf("failed to parse generated report: %v", err)
	}

	summaries := data["summaries"].(map[string]interface{})
	provider := summaries["provider1"].(map[string]interface{})
	if _, ok := provider["quality_coverage_pct"]; !ok {
		t.Fatal("expected quality_coverage_pct in summary JSON")
	}
	if _, ok := provider["reliability_adjusted_quality"]; !ok {
		t.Fatal("expected reliability_adjusted_quality in summary JSON")
	}
}

func TestGenerateJSON_ValidJSON(t *testing.T) {
	c := setupMockCollector()
	tmpDir := t.TempDir()
	gen := NewGenerator(c, tmpDir)

	err := gen.GenerateJSON()
	if err != nil {
		t.Fatalf("GenerateJSON failed: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(tmpDir, "report.json"))
	if err != nil {
		t.Fatalf("failed to read report: %v", err)
	}

	// Verify it's valid JSON by unmarshaling into generic map
	var data interface{}
	if err := json.Unmarshal(content, &data); err != nil {
		t.Errorf("generated JSON is invalid: %v", err)
	}
}

func TestGenerateMarkdown_IncludesReliabilityAdjustedQualityLeaderboard(t *testing.T) {
	c := metrics.NewCollector()
	c.AddResult(metrics.Result{
		TestName:      "Search",
		Provider:      "provider1",
		TestType:      "search",
		Success:       true,
		Latency:       100 * time.Millisecond,
		CreditsUsed:   1,
		ContentLength: 100,
		ResultsCount:  3,
		QualityScore:  90,
	})
	c.AddResult(metrics.Result{
		TestName:      "Search",
		Provider:      "provider2",
		TestType:      "search",
		Success:       false,
		Latency:       200 * time.Millisecond,
		CreditsUsed:   1,
		ContentLength: 100,
		ResultsCount:  3,
		QualityScore:  95,
	})

	tmpDir := t.TempDir()
	gen := NewGenerator(c, tmpDir)
	if err := gen.GenerateMarkdown(); err != nil {
		t.Fatalf("GenerateMarkdown failed: %v", err)
	}

	content, _ := os.ReadFile(filepath.Join(tmpDir, "report.md"))
	report := string(content)
	if !strings.Contains(report, "Raw Quality Score (scored tests only):") {
		t.Fatal("expected raw quality ranking section")
	}
	if !strings.Contains(report, "Reliability-Adjusted Quality (quality x success x coverage):") {
		t.Fatal("expected reliability-adjusted quality section")
	}
}

func TestGenerateMarkdown_IncludesQualityByTestTypeSection(t *testing.T) {
	c := metrics.NewCollector()
	c.AddResult(metrics.Result{
		TestName:     "Search",
		Provider:     "provider1",
		TestType:     "search",
		Success:      true,
		QualityScore: 70,
	})
	c.AddResult(metrics.Result{
		TestName:     "Extract",
		Provider:     "provider1",
		TestType:     "extract",
		Success:      true,
		QualityScore: 90,
	})
	c.AddResult(metrics.Result{
		TestName:     "Crawl",
		Provider:     "provider1",
		TestType:     "crawl",
		Success:      false,
		QualityScore: 0,
	})

	tmpDir := t.TempDir()
	gen := NewGenerator(c, tmpDir)
	if err := gen.GenerateMarkdown(); err != nil {
		t.Fatalf("GenerateMarkdown failed: %v", err)
	}

	content, _ := os.ReadFile(filepath.Join(tmpDir, "report.md"))
	report := string(content)
	if !strings.Contains(report, "### Quality by Test Type") {
		t.Fatal("expected quality by test type section")
	}
	if !strings.Contains(report, "| Provider | Search Quality | Search Coverage | Extract Quality | Extract Coverage | Crawl Quality | Crawl Coverage |") {
		t.Fatal("expected quality by test type table header")
	}
	if !strings.Contains(report, "| provider1 | 70.0 | 100.0% (1/1) | 90.0 | 100.0% (1/1) | - | 0.0% (0/1) |") {
		t.Fatal("expected provider quality breakdown row")
	}
}

func TestFormatCostUSD_TinyNonZero(t *testing.T) {
	if got := formatCostUSD(0.000002); got != "<$0.0001" {
		t.Fatalf("expected '<$0.0001' for tiny non-zero cost, got %q", got)
	}
	if got := formatCostUSD(0); got != "$0.00" {
		t.Fatalf("expected '$0.00' for zero cost, got %q", got)
	}
}

func TestGenerateJSON_WritesFile(t *testing.T) {
	c := setupMockCollector()
	tmpDir := t.TempDir()
	gen := NewGenerator(c, tmpDir)

	err := gen.GenerateJSON()
	if err != nil {
		t.Fatalf("GenerateJSON failed: %v", err)
	}

	_, err = os.Stat(filepath.Join(tmpDir, "report.json"))
	if err != nil {
		t.Errorf("report.json was not created: %v", err)
	}
}

func TestGenerateJSON_IncludesQualityByTestType(t *testing.T) {
	c := metrics.NewCollector()
	c.AddResult(metrics.Result{
		TestName:     "Search",
		Provider:     "provider1",
		TestType:     "search",
		Success:      true,
		QualityScore: 88,
	})
	tmpDir := t.TempDir()
	gen := NewGenerator(c, tmpDir)

	if err := gen.GenerateJSON(); err != nil {
		t.Fatalf("GenerateJSON failed: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(tmpDir, "report.json"))
	if err != nil {
		t.Fatalf("failed to read report: %v", err)
	}

	var payload map[string]json.RawMessage
	if err := json.Unmarshal(content, &payload); err != nil {
		t.Fatalf("failed to unmarshal report JSON: %v", err)
	}
	if _, ok := payload["quality_by_test_type"]; !ok {
		t.Fatal("expected quality_by_test_type in JSON report")
	}
}

func TestGenerateHTML_IncludesQualityByTestTypeSection(t *testing.T) {
	c := metrics.NewCollector()
	c.AddResult(metrics.Result{
		TestName:     "Search",
		Provider:     "provider1",
		TestType:     "search",
		Success:      true,
		QualityScore: 75,
	})
	tmpDir := t.TempDir()
	gen := NewGenerator(c, tmpDir)

	if err := gen.GenerateHTML(); err != nil {
		t.Fatalf("GenerateHTML failed: %v", err)
	}

	content, _ := os.ReadFile(filepath.Join(tmpDir, "report.html"))
	html := string(content)
	if !strings.Contains(html, "Quality by Test Type") {
		t.Fatal("expected quality by test type section in HTML")
	}
	if !strings.Contains(html, "Search Quality") || !strings.Contains(html, "Extract Quality") || !strings.Contains(html, "Crawl Quality") {
		t.Fatal("expected quality by type columns in HTML")
	}
}

func TestGenerateHTML_Structure(t *testing.T) {
	c := setupMockCollector()
	tmpDir := t.TempDir()
	gen := NewGenerator(c, tmpDir)

	err := gen.GenerateHTML()
	if err != nil {
		t.Fatalf("GenerateHTML failed: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(tmpDir, "report.html"))
	if err != nil {
		t.Fatalf("failed to read report: %v", err)
	}

	html := string(content)

	// Check for essential HTML structure
	if !strings.Contains(html, "<!DOCTYPE html>") {
		t.Error("HTML should contain doctype")
	}
	if !strings.Contains(html, "chart.js") && !strings.Contains(html, "Chart.js") {
		t.Error("HTML should reference Chart.js")
	}
	if !strings.Contains(html, "Search API Benchmark Report") {
		t.Error("HTML should contain title")
	}
}

func TestGenerateHTML_TableRows(t *testing.T) {
	c := metrics.NewCollector()
	c.AddResult(metrics.Result{
		TestName:     "Test 1",
		Provider:     "provider1",
		TestType:     "search",
		Success:      true,
		Latency:      100 * time.Millisecond,
		CreditsUsed:  1,
		ResultsCount: 5,
	})
	c.AddResult(metrics.Result{
		TestName:    "Test 1",
		Provider:    "provider2",
		TestType:    "search",
		Success:     false,
		Error:       "error",
		Latency:     200 * time.Millisecond,
		CreditsUsed: 1,
	})

	tmpDir := t.TempDir()
	gen := NewGenerator(c, tmpDir)

	err := gen.GenerateHTML()
	if err != nil {
		t.Fatalf("GenerateHTML failed: %v", err)
	}

	content, _ := os.ReadFile(filepath.Join(tmpDir, "report.html"))
	html := string(content)

	// Should have table rows for both results
	if !strings.Contains(html, "Test 1") {
		t.Error("HTML should contain test name")
	}
	if !strings.Contains(html, "provider1") {
		t.Error("HTML should contain provider1")
	}
	if !strings.Contains(html, "provider2") {
		t.Error("HTML should contain provider2")
	}
}

func TestGenerateHTML_ChartScripts(t *testing.T) {
	c := setupMockCollector()
	tmpDir := t.TempDir()
	gen := NewGenerator(c, tmpDir)

	err := gen.GenerateHTML()
	if err != nil {
		t.Fatalf("GenerateHTML failed: %v", err)
	}

	content, _ := os.ReadFile(filepath.Join(tmpDir, "report.html"))
	html := string(content)

	// Check for chart canvas elements
	if !strings.Contains(html, "latencyChart") {
		t.Error("HTML should contain latency chart canvas")
	}
	if !strings.Contains(html, "costChart") {
		t.Error("HTML should contain cost chart canvas")
	}
	if !strings.Contains(html, "successChart") {
		t.Error("HTML should contain success chart canvas")
	}

	// Check for Chart.js initialization
	if !strings.Contains(html, "new Chart") {
		t.Error("HTML should contain Chart.js initialization")
	}
}

func TestGenerateHTML_ProviderBadges(t *testing.T) {
	c := metrics.NewCollector()
	c.AddResult(metrics.Result{
		TestName: "Test",
		Provider: "firecrawl",
		Success:  true,
	})
	c.AddResult(metrics.Result{
		TestName: "Test",
		Provider: "tavily",
		Success:  true,
	})

	tmpDir := t.TempDir()
	gen := NewGenerator(c, tmpDir)

	err := gen.GenerateHTML()
	if err != nil {
		t.Fatalf("GenerateHTML failed: %v", err)
	}

	content, _ := os.ReadFile(filepath.Join(tmpDir, "report.html"))
	html := string(content)

	// Check for provider badge CSS classes
	if !strings.Contains(html, "provider-firecrawl") {
		t.Error("HTML should contain firecrawl badge class")
	}
	if !strings.Contains(html, "provider-tavily") {
		t.Error("HTML should contain tavily badge class")
	}
}

func TestGenerateHTML_WritesFile(t *testing.T) {
	c := setupMockCollector()
	tmpDir := t.TempDir()
	gen := NewGenerator(c, tmpDir)

	err := gen.GenerateHTML()
	if err != nil {
		t.Fatalf("GenerateHTML failed: %v", err)
	}

	_, err = os.Stat(filepath.Join(tmpDir, "report.html"))
	if err != nil {
		t.Errorf("report.html was not created: %v", err)
	}
}

func TestGenerateAll_CreatesAll(t *testing.T) {
	c := setupMockCollector()
	tmpDir := t.TempDir()
	gen := NewGenerator(c, tmpDir)

	err := gen.GenerateAll()
	if err != nil {
		t.Fatalf("GenerateAll failed: %v", err)
	}

	// Verify all three files exist
	files := []string{"report.md", "report.json", "report.html"}
	for _, file := range files {
		_, err := os.Stat(filepath.Join(tmpDir, file))
		if err != nil {
			t.Errorf("%s was not created: %v", file, err)
		}
	}
}

func TestGenerateMarkdown_EmptyResults(t *testing.T) {
	c := metrics.NewCollector()
	tmpDir := t.TempDir()
	gen := NewGenerator(c, tmpDir)

	err := gen.GenerateMarkdown()
	if err != nil {
		t.Fatalf("GenerateMarkdown failed: %v", err)
	}

	// Should still generate a report (with empty tables)
	content, err := os.ReadFile(filepath.Join(tmpDir, "report.md"))
	if err != nil {
		t.Fatalf("failed to read report: %v", err)
	}

	if !strings.Contains(string(content), "Search API Benchmark Report") {
		t.Error("report should still contain title even with empty results")
	}
}
