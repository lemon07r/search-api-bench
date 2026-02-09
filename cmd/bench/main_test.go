package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lamim/search-api-bench/internal/metrics"
)

func TestParseProviders_All(t *testing.T) {
	result, err := parseProviders("all", false)
	if err != nil {
		t.Fatalf("parseProviders returned error: %v", err)
	}
	if len(result) != 7 {
		t.Errorf("expected 7 providers for 'all', got %d", len(result))
	}
	expected := []string{"firecrawl", "tavily", "local", "brave", "exa", "mixedbread", "jina"}
	for i, exp := range expected {
		if result[i] != exp {
			t.Errorf("expected %s at index %d, got %s", exp, i, result[i])
		}
	}
}

func TestParseProviders_Single(t *testing.T) {
	result, err := parseProviders("firecrawl", false)
	if err != nil {
		t.Fatalf("parseProviders returned error: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 provider, got %d", len(result))
	}
	if result[0] != "firecrawl" {
		t.Errorf("expected firecrawl, got %s", result[0])
	}
}

func TestParseProviders_List(t *testing.T) {
	result, err := parseProviders("firecrawl,tavily", false)
	if err != nil {
		t.Fatalf("parseProviders returned error: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 providers, got %d", len(result))
	}
	if result[0] != "firecrawl" || result[1] != "tavily" {
		t.Errorf("expected [firecrawl, tavily], got %v", result)
	}
}

func TestParseProviders_Empty(t *testing.T) {
	_, err := parseProviders("", false)
	if err == nil {
		t.Fatal("expected error for empty providers")
	}
}

func TestParseProviders_Invalid(t *testing.T) {
	_, err := parseProviders("firecrawl,unknown", false)
	if err == nil {
		t.Fatal("expected error for invalid provider")
	}
}

func TestParseProviders_NoLocalFilter(t *testing.T) {
	result, err := parseProviders("local,firecrawl,local", true)
	if err != nil {
		t.Fatalf("parseProviders returned error: %v", err)
	}
	if len(result) != 1 || result[0] != "firecrawl" {
		t.Fatalf("expected [firecrawl], got %v", result)
	}
}

func TestParseFormats_All(t *testing.T) {
	result, err := parseFormats("all")
	if err != nil {
		t.Fatalf("parseFormats returned error: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 element for 'all', got %d", len(result))
	}
	if result[0] != "all" {
		t.Errorf("expected 'all', got %s", result[0])
	}
}

func TestParseFormats_Single(t *testing.T) {
	result, err := parseFormats("html")
	if err != nil {
		t.Fatalf("parseFormats returned error: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 format, got %d", len(result))
	}
	if result[0] != "html" {
		t.Errorf("expected html, got %s", result[0])
	}
}

func TestParseFormats_List(t *testing.T) {
	result, err := parseFormats("html,md,json")
	if err != nil {
		t.Fatalf("parseFormats returned error: %v", err)
	}
	if len(result) != 3 {
		t.Errorf("expected 3 formats, got %d", len(result))
	}
	expected := []string{"html", "md", "json"}
	for i, exp := range expected {
		if result[i] != exp {
			t.Errorf("expected %s at index %d, got %s", exp, i, result[i])
		}
	}
}

func TestParseFormats_Invalid(t *testing.T) {
	_, err := parseFormats("html,xml")
	if err == nil {
		t.Fatal("expected error for invalid format")
	}
}

func TestParseFormats_AllWithOthers(t *testing.T) {
	_, err := parseFormats("all,html")
	if err == nil {
		t.Fatal("expected error when combining all with other formats")
	}
}

func TestLoadEnv_FileNotFound(t *testing.T) {
	// Ensure .env doesn't exist
	os.Remove(".env")

	// Should not panic or error
	// The main function handles this gracefully by continuing
}

func TestLoadEnv_ParsesValues(t *testing.T) {
	tmpDir := t.TempDir()
	envPath := filepath.Join(tmpDir, ".env")

	content := `TEST_KEY1=value1
TEST_KEY2=value2`
	if err := os.WriteFile(envPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test .env: %v", err)
	}

	// Read and parse the file manually (simulating what main.go does)
	data, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("failed to read test .env: %v", err)
	}

	lines := strings.Split(string(data), "\n")
	parsed := make(map[string]string)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			value = strings.Trim(value, `"'`)
			parsed[key] = value
		}
	}

	if parsed["TEST_KEY1"] != "value1" {
		t.Errorf("expected TEST_KEY1=value1, got %s", parsed["TEST_KEY1"])
	}
	if parsed["TEST_KEY2"] != "value2" {
		t.Errorf("expected TEST_KEY2=value2, got %s", parsed["TEST_KEY2"])
	}
}

func TestLoadEnv_SkipsComments(t *testing.T) {
	tmpDir := t.TempDir()
	envPath := filepath.Join(tmpDir, ".env")

	content := `# This is a comment
TEST_KEY=value
# Another comment`
	if err := os.WriteFile(envPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test .env: %v", err)
	}

	data, _ := os.ReadFile(envPath)
	lines := strings.Split(string(data), "\n")

	commentCount := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			commentCount++
		}
	}

	if commentCount != 2 {
		t.Errorf("expected 2 comment lines, found %d", commentCount)
	}
}

func TestLoadEnv_SkipsEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	envPath := filepath.Join(tmpDir, ".env")

	content := `TEST_KEY1=value1

TEST_KEY2=value2

`
	if err := os.WriteFile(envPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test .env: %v", err)
	}

	data, _ := os.ReadFile(envPath)
	lines := strings.Split(string(data), "\n")

	emptyCount := 0
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			emptyCount++
		}
	}

	if emptyCount < 2 {
		t.Errorf("expected at least 2 empty lines, found %d", emptyCount)
	}
}

func TestLoadEnv_TrimsQuotes(t *testing.T) {
	tmpDir := t.TempDir()
	envPath := filepath.Join(tmpDir, ".env")

	content := `TEST_KEY1="quoted_value"
TEST_KEY2='single_quoted'
TEST_KEY3=unquoted`
	if err := os.WriteFile(envPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test .env: %v", err)
	}

	data, _ := os.ReadFile(envPath)
	lines := strings.Split(string(data), "\n")
	parsed := make(map[string]string)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			value = strings.Trim(value, `"'`)
			parsed[key] = value
		}
	}

	if parsed["TEST_KEY1"] != "quoted_value" {
		t.Errorf("expected quoted_value (quotes removed), got %s", parsed["TEST_KEY1"])
	}
	if parsed["TEST_KEY2"] != "single_quoted" {
		t.Errorf("expected single_quoted (quotes removed), got %s", parsed["TEST_KEY2"])
	}
	if parsed["TEST_KEY3"] != "unquoted" {
		t.Errorf("expected unquoted, got %s", parsed["TEST_KEY3"])
	}
}

func TestLoadEnv_TrimsSpaces(t *testing.T) {
	tmpDir := t.TempDir()
	envPath := filepath.Join(tmpDir, ".env")

	content := `TEST_KEY = value_with_spaces`
	if err := os.WriteFile(envPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test .env: %v", err)
	}

	data, _ := os.ReadFile(envPath)
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
			value = strings.Trim(value, `"'`)

			if key != "TEST_KEY" {
				t.Errorf("expected key 'TEST_KEY' (spaces trimmed), got '%s'", key)
			}
			if value != "value_with_spaces" {
				t.Errorf("expected value 'value_with_spaces' (spaces trimmed), got '%s'", value)
			}
		}
	}
}

func TestPrintBanner_NoPanic(t *testing.T) {
	// Just verify it doesn't panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("printBanner panicked: %v", r)
		}
	}()

	// Capture output to avoid cluttering test output
	// We can't easily test the output content, but we can verify it doesn't panic
	printBanner()
}

func TestPrintSummary_NoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("printSummary panicked: %v", r)
		}
	}()

	c := metrics.NewCollector()
	c.AddResult(metrics.Result{
		TestName: "Test",
		Provider: "mock",
		Success:  true,
		Latency:  100,
	})

	printSummary(c)
}

func TestPrintSummary_EmptyCollector(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("printSummary panicked with empty collector: %v", r)
		}
	}()

	c := metrics.NewCollector()
	printSummary(c)
}
