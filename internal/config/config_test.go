package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_ValidConfig(t *testing.T) {
	content := `
[general]
concurrency = 10
timeout = "60s"
output_dir = "./output"

[[tests]]
name = "Test 1"
type = "search"
query = "test query"

[[tests]]
name = "Test 2"
type = "extract"
url = "https://example.com"
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.General.Concurrency != 10 {
		t.Errorf("expected concurrency 10, got %d", cfg.General.Concurrency)
	}
	if cfg.General.Timeout != "60s" {
		t.Errorf("expected timeout 60s, got %s", cfg.General.Timeout)
	}
	if cfg.General.OutputDir != "./output" {
		t.Errorf("expected output_dir ./output, got %s", cfg.General.OutputDir)
	}
	if len(cfg.Tests) != 2 {
		t.Errorf("expected 2 tests, got %d", len(cfg.Tests))
	}
}

func TestLoad_DefaultsApplied(t *testing.T) {
	content := `
[[tests]]
name = "Minimal Test"
type = "search"
query = "test"
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.General.Concurrency != 5 {
		t.Errorf("expected default concurrency 5, got %d", cfg.General.Concurrency)
	}
	if cfg.General.Timeout != "30s" {
		t.Errorf("expected default timeout 30s, got %s", cfg.General.Timeout)
	}
	if cfg.General.OutputDir != "./results" {
		t.Errorf("expected default output_dir ./results, got %s", cfg.General.OutputDir)
	}
}

func TestLoad_EmptyTestsError(t *testing.T) {
	content := `
[general]
concurrency = 5
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Fatal("expected error for empty tests, got nil")
	}
	if err.Error() != "no tests defined in configuration" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestLoad_MissingFileError(t *testing.T) {
	_, err := Load("/nonexistent/path/config.toml")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
	if !os.IsNotExist(err) {
		t.Logf("error is wrapped: %v", err)
	}
}

func TestLoad_InvalidTOMLError(t *testing.T) {
	content := `this is not valid toml [[[`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Fatal("expected error for invalid TOML, got nil")
	}
}

func TestLoad_InvalidTestType(t *testing.T) {
	content := `
[[tests]]
name = "Invalid Type"
type = "invalid"
query = "test"
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Fatal("expected error for invalid test type, got nil")
	}
	if err.Error() != "test 'Invalid Type' has invalid type: invalid" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestLoad_SearchMissingQuery(t *testing.T) {
	content := `
[[tests]]
name = "Search Without Query"
type = "search"
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Fatal("expected error for search without query, got nil")
	}
	if err.Error() != "test 'Search Without Query' of type 'search' requires a query" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestLoad_ExtractMissingURL(t *testing.T) {
	content := `
[[tests]]
name = "Extract Without URL"
type = "extract"
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Fatal("expected error for extract without URL, got nil")
	}
	if err.Error() != "test 'Extract Without URL' of type 'extract' requires a URL" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestLoad_CrawlMissingURL(t *testing.T) {
	content := `
[[tests]]
name = "Crawl Without URL"
type = "crawl"
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Fatal("expected error for crawl without URL, got nil")
	}
	if err.Error() != "test 'Crawl Without URL' of type 'crawl' requires a URL" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestSave_LoadRoundtrip(t *testing.T) {
	original := &Config{
		General: GeneralConfig{
			Concurrency: 7,
			Timeout:     "45s",
			OutputDir:   "./test-output",
		},
		Tests: []TestConfig{
			{
				Name:           "Test 1",
				Type:           "search",
				Query:          "test query",
				ExpectedTopics: []string{"topic1", "topic2"},
			},
			{
				Name:            "Test 2",
				Type:            "extract",
				URL:             "https://example.com",
				ExpectedContent: []string{"content1"},
			},
		},
	}

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "roundtrip.toml")

	if err := original.Save(configPath); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loaded.General.Concurrency != original.General.Concurrency {
		t.Errorf("concurrency mismatch: %d vs %d", loaded.General.Concurrency, original.General.Concurrency)
	}
	if loaded.General.Timeout != original.General.Timeout {
		t.Errorf("timeout mismatch: %s vs %s", loaded.General.Timeout, original.General.Timeout)
	}
	if loaded.General.OutputDir != original.General.OutputDir {
		t.Errorf("output_dir mismatch: %s vs %s", loaded.General.OutputDir, original.General.OutputDir)
	}
	if len(loaded.Tests) != len(original.Tests) {
		t.Errorf("tests count mismatch: %d vs %d", len(loaded.Tests), len(original.Tests))
	}
}

func TestTimeoutDuration_Parse(t *testing.T) {
	tests := []struct {
		input    string
		expected int64 // milliseconds
	}{
		{"30s", 30000},
		{"1m", 60000},
		{"500ms", 500},
		{"2h", 7200000},
	}

	for _, tt := range tests {
		g := GeneralConfig{Timeout: tt.input}
		d := g.TimeoutDuration()
		if d.Milliseconds() != tt.expected {
			t.Errorf("TimeoutDuration(%s) = %dms, want %dms", tt.input, d.Milliseconds(), tt.expected)
		}
	}
}

func TestTimeoutDuration_Invalid(t *testing.T) {
	g := GeneralConfig{Timeout: "invalid"}
	d := g.TimeoutDuration()
	if d != 30000000000 { // 30s in nanoseconds
		t.Errorf("expected default 30s for invalid duration, got %v", d)
	}
}
