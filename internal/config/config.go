// Package config provides configuration loading and validation for the benchmark tool.
package config

import (
	"fmt"
	"os"
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
	Concurrency int    `toml:"concurrency"`
	Timeout     string `toml:"timeout"`
	OutputDir   string `toml:"output_dir"`
}

// TestConfig represents a single test case
type TestConfig struct {
	Name            string   `toml:"name"`
	Type            string   `toml:"type"` // search, extract, crawl
	Query           string   `toml:"query,omitempty"`
	URL             string   `toml:"url,omitempty"`
	MaxPages        int      `toml:"max_pages,omitempty"`
	MaxDepth        int      `toml:"max_depth,omitempty"`
	ExpectedTopics  []string `toml:"expected_topics,omitempty"`
	ExpectedContent []string `toml:"expected_content,omitempty"`
}

// TimeoutDuration parses the timeout string into a Duration
func (g GeneralConfig) TimeoutDuration() time.Duration {
	d, err := time.ParseDuration(g.Timeout)
	if err != nil {
		return 30 * time.Second
	}
	return d
}

// Load reads and parses the TOML configuration file
func Load(path string) (*Config, error) {
	// #nosec G304 - This is intentional file inclusion via variable for config loading
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
	}

	return &cfg, nil
}

// Save writes the configuration to a TOML file
func (c *Config) Save(path string) error {
	// #nosec G304 - This is intentional file creation via variable for config saving
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
