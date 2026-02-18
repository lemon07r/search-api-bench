// Package config provides configuration loading and validation for the benchmark tool.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// Config represents the main configuration structure
type Config struct {
	General GeneralConfig `toml:"general"`
	Tests   []TestConfig  `toml:"tests"`
}

// GeneralConfig contains general settings
type GeneralConfig struct {
	Concurrency         int            `toml:"concurrency"`
	ProviderConcurrency map[string]int `toml:"provider_concurrency"`
	Timeout             string         `toml:"timeout"`
	OutputDir           string         `toml:"output_dir"`
}

func defaultProviderConcurrency() map[string]int {
	return map[string]int{
		"brave":      1,
		"exa":        1,
		"firecrawl":  1,
		"jina":       1,
		"local":      1,
		"mixedbread": 1,
		"tavily":     1,
	}
}

// TestConfig represents a single test case
type TestConfig struct {
	Name  string `toml:"name"`
	Type  string `toml:"type"` // search, extract, crawl
	Query string `toml:"query,omitempty"`
	URL   string `toml:"url,omitempty"`
	// MaxPages and MaxDepth are pointers so explicit zero values in TOML
	// are distinguishable from unset fields.
	MaxPages               *int     `toml:"max_pages,omitempty"`
	MaxDepth               *int     `toml:"max_depth,omitempty"`
	ExpectedTopics         []string `toml:"expected_topics,omitempty"`
	ExpectedContent        []string `toml:"expected_content,omitempty"`
	ExpectedURLs           []string `toml:"expected_urls,omitempty"`
	MustIncludeTerms       []string `toml:"must_include_terms,omitempty"`
	MustNotIncludeTerms    []string `toml:"must_not_include_terms,omitempty"`
	ExpectedSnippets       []string `toml:"expected_snippets,omitempty"`
	ForbiddenSnippets      []string `toml:"forbidden_snippets,omitempty"`
	ExpectedURLPatterns    []string `toml:"expected_url_patterns,omitempty"`
	ExpectedMaxDepth       *int     `toml:"expected_max_depth,omitempty"`
	FreshnessReferenceDate string   `toml:"freshness_reference_date,omitempty"`
}

// TimeoutDuration parses the timeout string into a Duration
func (g GeneralConfig) TimeoutDuration() time.Duration {
	d, err := time.ParseDuration(g.Timeout)
	if err != nil {
		return 30 * time.Second
	}
	return d
}

// ConcurrencyForProvider returns the effective concurrency for a provider.
// It uses provider-specific overrides when present and falls back to the
// global concurrency value.
func (g GeneralConfig) ConcurrencyForProvider(provider string) int {
	base := g.Concurrency
	if base <= 0 {
		base = 1
	}
	if len(g.ProviderConcurrency) == 0 {
		return base
	}
	normalized := strings.ToLower(strings.TrimSpace(provider))
	if normalized != "" {
		if limit, ok := g.ProviderConcurrency[normalized]; ok && limit > 0 {
			return limit
		}
	}
	// Support programmatic configs that may not have normalized keys.
	if limit, ok := g.ProviderConcurrency[provider]; ok && limit > 0 {
		return limit
	}
	return base
}

func normalizeProviderConcurrency(raw map[string]int) (map[string]int, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	normalized := make(map[string]int, len(raw))
	for provider, limit := range raw {
		name := strings.ToLower(strings.TrimSpace(provider))
		if name == "" {
			return nil, fmt.Errorf("provider_concurrency contains an empty provider name")
		}
		if limit <= 0 {
			return nil, fmt.Errorf("provider_concurrency for '%s' must be > 0", provider)
		}
		normalized[name] = limit
	}
	return normalized, nil
}

// validatePath checks for path traversal attempts
func validatePath(path string) error {
	// Clean the path
	cleanPath := filepath.Clean(path)

	// Check for path traversal sequences that go above current directory
	// This prevents ../../../etc/passwd type attacks
	if strings.HasPrefix(cleanPath, "..") || strings.Contains(cleanPath, "../") {
		return fmt.Errorf("path contains invalid traversal sequence: %s", path)
	}

	return nil
}

// Load reads and parses the TOML configuration file
func Load(path string) (*Config, error) {
	// Validate path for security
	if err := validatePath(path); err != nil {
		return nil, fmt.Errorf("invalid config path: %w", err)
	}

	// #nosec G304 - Path validated above, this is intentional file inclusion
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Set defaults
	if cfg.General.Concurrency <= 0 {
		cfg.General.Concurrency = 5
	}
	if cfg.General.Timeout == "" {
		cfg.General.Timeout = "30s"
	}
	if cfg.General.OutputDir == "" {
		cfg.General.OutputDir = "./results"
	}
	if len(cfg.General.ProviderConcurrency) == 0 {
		cfg.General.ProviderConcurrency = defaultProviderConcurrency()
	}
	normalizedProviderConcurrency, err := normalizeProviderConcurrency(cfg.General.ProviderConcurrency)
	if err != nil {
		return nil, err
	}
	cfg.General.ProviderConcurrency = normalizedProviderConcurrency

	// Validate tests
	if len(cfg.Tests) == 0 {
		return nil, fmt.Errorf("no tests defined in configuration")
	}

	for i, test := range cfg.Tests {
		if test.Name == "" {
			return nil, fmt.Errorf("test at index %d is missing a name", i)
		}
		if test.Type != "search" && test.Type != "extract" && test.Type != "crawl" {
			return nil, fmt.Errorf("test '%s' has invalid type: %s", test.Name, test.Type)
		}
		if test.Type == "search" && test.Query == "" {
			return nil, fmt.Errorf("test '%s' of type 'search' requires a query", test.Name)
		}
		if (test.Type == "extract" || test.Type == "crawl") && test.URL == "" {
			return nil, fmt.Errorf("test '%s' of type '%s' requires a URL", test.Name, test.Type)
		}
		if test.MaxPages != nil && *test.MaxPages < 0 {
			return nil, fmt.Errorf("test '%s' has invalid max_pages: %d", test.Name, *test.MaxPages)
		}
		if test.MaxDepth != nil && *test.MaxDepth < 0 {
			return nil, fmt.Errorf("test '%s' has invalid max_depth: %d", test.Name, *test.MaxDepth)
		}
		if test.ExpectedMaxDepth != nil && *test.ExpectedMaxDepth < 0 {
			return nil, fmt.Errorf("test '%s' has invalid expected_max_depth: %d", test.Name, *test.ExpectedMaxDepth)
		}
	}

	return &cfg, nil
}

// Save writes the configuration to a TOML file
func (c *Config) Save(path string) error {
	// Validate path for security
	if err := validatePath(path); err != nil {
		return fmt.Errorf("invalid config path: %w", err)
	}

	// #nosec G304 - Path validated above, this is intentional file creation
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create config file: %w", err)
	}
	defer func() {
		_ = f.Close()
	}()

	encoder := toml.NewEncoder(f)
	return encoder.Encode(c)
}
