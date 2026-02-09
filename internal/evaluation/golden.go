// Package evaluation provides golden dataset management and regression detection
package evaluation

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// GoldenDataset represents a set of canonical test cases with expected results
type GoldenDataset struct {
	Version   string       `json:"version"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
	Tests     []GoldenTest `json:"tests"`
}

// GoldenTest represents a single test case in the golden dataset
type GoldenTest struct {
	ID              string           `json:"id"`
	Name            string           `json:"name"`
	Type            string           `json:"type"` // search, extract, crawl
	Query           string           `json:"query,omitempty"`
	URL             string           `json:"url,omitempty"`
	ExpectedResults []ExpectedResult `json:"expected_results,omitempty"`
	MinQualityScore float64          `json:"min_quality_score"`
	ExpectedURLs    []string         `json:"expected_urls,omitempty"`
	ExpectedContent []string         `json:"expected_content,omitempty"`
}

// ExpectedResult represents an expected search result
type ExpectedResult struct {
	URL      string `json:"url"`
	MinRank  int    `json:"min_rank"` // Expected minimum rank (1-indexed)
	Required bool   `json:"required"` // Must be present
}

// BaselineScores stores baseline quality scores for regression detection
type BaselineScores struct {
	Version        string                      `json:"version"`
	CreatedAt      time.Time                   `json:"created_at"`
	ProviderScores map[string]ProviderBaseline `json:"provider_scores"`
}

// ProviderBaseline stores baseline scores for a provider
type ProviderBaseline struct {
	Provider     string                  `json:"provider"`
	TestScores   map[string]TestBaseline `json:"test_scores"`
	OverallScore float64                 `json:"overall_score"`
}

// TestBaseline stores baseline for a specific test
type TestBaseline struct {
	TestName     string  `json:"test_name"`
	QualityScore float64 `json:"quality_score"`
	LatencyMs    float64 `json:"latency_ms"`
	SuccessRate  float64 `json:"success_rate"`
}

// RegressionResult represents a detected regression
type RegressionResult struct {
	TestName      string  `json:"test_name"`
	Provider      string  `json:"provider"`
	Metric        string  `json:"metric"` // quality, latency, success_rate
	BaselineValue float64 `json:"baseline_value"`
	CurrentValue  float64 `json:"current_value"`
	ChangePercent float64 `json:"change_percent"`
	IsRegression  bool    `json:"is_regression"`
	Severity      string  `json:"severity"` // critical, warning, info
}

// GoldenManager manages golden datasets and baseline scores
type GoldenManager struct {
	datasetPath  string
	baselinePath string
	dataset      *GoldenDataset
	baseline     *BaselineScores
}

// NewGoldenManager creates a new golden manager
func NewGoldenManager(datasetPath, baselinePath string) *GoldenManager {
	return &GoldenManager{
		datasetPath:  datasetPath,
		baselinePath: baselinePath,
	}
}

// LoadDataset loads the golden dataset from disk
func (m *GoldenManager) LoadDataset() error {
	data, err := os.ReadFile(m.datasetPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Create empty dataset
			m.dataset = &GoldenDataset{
				Version:   "1.0",
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
				Tests:     []GoldenTest{},
			}
			return nil
		}
		return fmt.Errorf("failed to read dataset: %w", err)
	}

	var dataset GoldenDataset
	if err := json.Unmarshal(data, &dataset); err != nil {
		return fmt.Errorf("failed to parse dataset: %w", err)
	}

	m.dataset = &dataset
	return nil
}

// SaveDataset saves the golden dataset to disk
func (m *GoldenManager) SaveDataset() error {
	if m.dataset == nil {
		return fmt.Errorf("no dataset loaded")
	}

	m.dataset.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(m.dataset, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal dataset: %w", err)
	}

	// #nosec G306 - 0640 allows owner/group to read, which is appropriate for dataset files
	if err := os.WriteFile(m.datasetPath, data, 0640); err != nil {
		return fmt.Errorf("failed to write dataset: %w", err)
	}

	return nil
}

// LoadBaseline loads baseline scores from disk
func (m *GoldenManager) LoadBaseline() error {
	data, err := os.ReadFile(m.baselinePath)
	if err != nil {
		if os.IsNotExist(err) {
			// Create empty baseline
			m.baseline = &BaselineScores{
				Version:        "1.0",
				CreatedAt:      time.Now(),
				ProviderScores: make(map[string]ProviderBaseline),
			}
			return nil
		}
		return fmt.Errorf("failed to read baseline: %w", err)
	}

	var baseline BaselineScores
	if err := json.Unmarshal(data, &baseline); err != nil {
		return fmt.Errorf("failed to parse baseline: %w", err)
	}

	m.baseline = &baseline
	return nil
}

// SaveBaseline saves baseline scores to disk
func (m *GoldenManager) SaveBaseline() error {
	if m.baseline == nil {
		return fmt.Errorf("no baseline loaded")
	}

	data, err := json.MarshalIndent(m.baseline, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal baseline: %w", err)
	}

	// #nosec G306 - 0640 allows owner/group to read
	if err := os.WriteFile(m.baselinePath, data, 0640); err != nil {
		return fmt.Errorf("failed to write baseline: %w", err)
	}

	return nil
}

// AddTest adds a test to the golden dataset
func (m *GoldenManager) AddTest(test GoldenTest) error {
	if m.dataset == nil {
		if err := m.LoadDataset(); err != nil {
			return err
		}
	}

	// Generate ID if not set
	if test.ID == "" {
		test.ID = fmt.Sprintf("test_%d", len(m.dataset.Tests)+1)
	}

	// Check for duplicates
	for i, existing := range m.dataset.Tests {
		if existing.ID == test.ID {
			// Update existing
			m.dataset.Tests[i] = test
			return m.SaveDataset()
		}
	}

	// Add new
	m.dataset.Tests = append(m.dataset.Tests, test)
	return m.SaveDataset()
}

// GetTest retrieves a test by ID
func (m *GoldenManager) GetTest(id string) (*GoldenTest, bool) {
	if m.dataset == nil {
		return nil, false
	}

	for _, test := range m.dataset.Tests {
		if test.ID == id {
			return &test, true
		}
	}

	return nil, false
}

// GetTestsByType returns all tests of a specific type
func (m *GoldenManager) GetTestsByType(testType string) []GoldenTest {
	if m.dataset == nil {
		return nil
	}

	var result []GoldenTest
	for _, test := range m.dataset.Tests {
		if test.Type == testType {
			result = append(result, test)
		}
	}

	return result
}

// UpdateBaseline updates baseline scores from current results
func (m *GoldenManager) UpdateBaseline(provider string, testScores map[string]TestBaseline, overallScore float64) error {
	if m.baseline == nil {
		if err := m.LoadBaseline(); err != nil {
			return err
		}
	}

	m.baseline.ProviderScores[provider] = ProviderBaseline{
		Provider:     provider,
		TestScores:   testScores,
		OverallScore: overallScore,
	}

	return m.SaveBaseline()
}

// DetectRegressions compares current scores against baseline
func (m *GoldenManager) DetectRegressions(provider string, currentScores map[string]TestBaseline, threshold float64) []RegressionResult {
	if m.baseline == nil {
		return nil
	}

	baseline, ok := m.baseline.ProviderScores[provider]
	if !ok {
		return nil
	}

	var regressions []RegressionResult

	for testName, current := range currentScores {
		expected, ok := baseline.TestScores[testName]
		if !ok {
			continue // No baseline for this test
		}

		// Check quality score regression
		if expected.QualityScore > 0 {
			change := (current.QualityScore - expected.QualityScore) / expected.QualityScore
			if change < -threshold {
				severity := "warning"
				if change < -threshold*2 {
					severity = "critical"
				}

				regressions = append(regressions, RegressionResult{
					TestName:      testName,
					Provider:      provider,
					Metric:        "quality",
					BaselineValue: expected.QualityScore,
					CurrentValue:  current.QualityScore,
					ChangePercent: change * 100,
					IsRegression:  true,
					Severity:      severity,
				})
			}
		}

		// Check latency regression (higher is worse)
		if expected.LatencyMs > 0 && current.LatencyMs > expected.LatencyMs*1.5 {
			change := (current.LatencyMs - expected.LatencyMs) / expected.LatencyMs
			severity := "warning"
			if change > 1.0 {
				severity = "critical"
			}

			regressions = append(regressions, RegressionResult{
				TestName:      testName,
				Provider:      provider,
				Metric:        "latency",
				BaselineValue: expected.LatencyMs,
				CurrentValue:  current.LatencyMs,
				ChangePercent: change * 100,
				IsRegression:  true,
				Severity:      severity,
			})
		}

		// Check success rate regression
		if expected.SuccessRate > 0.95 && current.SuccessRate < expected.SuccessRate-threshold {
			change := current.SuccessRate - expected.SuccessRate
			severity := "warning"
			if current.SuccessRate < 0.8 {
				severity = "critical"
			}

			regressions = append(regressions, RegressionResult{
				TestName:      testName,
				Provider:      provider,
				Metric:        "success_rate",
				BaselineValue: expected.SuccessRate,
				CurrentValue:  current.SuccessRate,
				ChangePercent: change * 100,
				IsRegression:  true,
				Severity:      severity,
			})
		}
	}

	return regressions
}

// ValidateAgainstGolden validates results against golden expectations
func (m *GoldenManager) ValidateAgainstGolden(testID string, results []string, qualityScore float64) (*ValidationResult, error) {
	test, ok := m.GetTest(testID)
	if !ok {
		return nil, fmt.Errorf("test %s not found in golden dataset", testID)
	}

	validation := &ValidationResult{
		TestID:         testID,
		TestName:       test.Name,
		QualityScore:   qualityScore,
		MinQualityMet:  qualityScore >= test.MinQualityScore,
		ExpectedURLs:   test.ExpectedURLs,
		FoundURLs:      []string{},
		MissingURLs:    []string{},
		UnexpectedURLs: []string{},
	}

	// Check for expected URLs
	resultSet := make(map[string]bool)
	for _, url := range results {
		resultSet[url] = true
	}

	for _, expected := range test.ExpectedURLs {
		if resultSet[expected] {
			validation.FoundURLs = append(validation.FoundURLs, expected)
		} else {
			validation.MissingURLs = append(validation.MissingURLs, expected)
		}
	}

	// Find unexpected URLs (simplified - just checking if we got any results)
	for _, url := range results {
		found := false
		for _, expected := range test.ExpectedURLs {
			if url == expected {
				found = true
				break
			}
		}
		if !found {
			validation.UnexpectedURLs = append(validation.UnexpectedURLs, url)
		}
	}

	// Calculate precision and recall
	if len(test.ExpectedURLs) > 0 {
		validation.Recall = float64(len(validation.FoundURLs)) / float64(len(test.ExpectedURLs)) * 100
	} else {
		validation.Recall = 100
	}

	if len(results) > 0 {
		validation.Precision = float64(len(validation.FoundURLs)) / float64(len(results)) * 100
	} else if len(test.ExpectedURLs) == 0 {
		validation.Precision = 100
	} else {
		validation.Precision = 0
	}

	// Overall pass/fail
	validation.Passed = validation.MinQualityMet && validation.Recall >= 80

	return validation, nil
}

// ValidationResult represents validation against golden dataset
type ValidationResult struct {
	TestID         string   `json:"test_id"`
	TestName       string   `json:"test_name"`
	Passed         bool     `json:"passed"`
	QualityScore   float64  `json:"quality_score"`
	MinQualityMet  bool     `json:"min_quality_met"`
	Precision      float64  `json:"precision"`
	Recall         float64  `json:"recall"`
	ExpectedURLs   []string `json:"expected_urls"`
	FoundURLs      []string `json:"found_urls"`
	MissingURLs    []string `json:"missing_urls"`
	UnexpectedURLs []string `json:"unexpected_urls"`
}

// CreateDefaultDataset creates a default golden dataset
func CreateDefaultDataset() *GoldenDataset {
	return &GoldenDataset{
		Version:   "1.0",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Tests: []GoldenTest{
			{
				ID:              "search_basic",
				Name:            "Basic Search Test",
				Type:            "search",
				Query:           "capital of France",
				MinQualityScore: 70,
				ExpectedURLs: []string{
					"https://en.wikipedia.org/wiki/Paris",
				},
			},
			{
				ID:              "search_technical",
				Name:            "Technical Search Test",
				Type:            "search",
				Query:           "Rust programming language",
				MinQualityScore: 70,
				ExpectedURLs: []string{
					"https://www.rust-lang.org",
					"https://github.com/rust-lang/rust",
				},
			},
			{
				ID:              "extract_docs",
				Name:            "Documentation Extraction Test",
				Type:            "extract",
				URL:             "https://docs.python.org/3/tutorial/",
				MinQualityScore: 60,
				ExpectedContent: []string{
					"Python",
					"tutorial",
					"interpreter",
				},
			},
		},
	}
}

// FormatRegressionReport creates a human-readable regression report
func FormatRegressionReport(regressions []RegressionResult) string {
	if len(regressions) == 0 {
		return "No regressions detected."
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Detected %d Regressions:\n", len(regressions)))

	criticalCount, warningCount := 0, 0
	for _, r := range regressions {
		if r.Severity == "critical" {
			criticalCount++
		} else {
			warningCount++
		}
	}

	sb.WriteString(fmt.Sprintf("  Critical: %d, Warnings: %d\n\n", criticalCount, warningCount))

	for _, r := range regressions {
		sb.WriteString(fmt.Sprintf("  [%s] %s - %s\n", r.Severity, r.TestName, r.Metric))
		sb.WriteString(fmt.Sprintf("    Baseline: %.2f, Current: %.2f (%.1f%%)\n",
			r.BaselineValue, r.CurrentValue, r.ChangePercent))
	}

	return sb.String()
}
