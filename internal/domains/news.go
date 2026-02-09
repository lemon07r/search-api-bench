package domains

import (
	"regexp"
	"strings"
	"time"
)

// NewsValidator validates news/current affairs extraction quality
type NewsValidator struct {
	maxAgeHours int
}

// NewNewsValidator creates a new news validator
func NewNewsValidator(maxAgeHours int) *NewsValidator {
	if maxAgeHours <= 0 {
		maxAgeHours = 48 // Default: 2 days
	}
	return &NewsValidator{
		maxAgeHours: maxAgeHours,
	}
}

// NewsValidationResult contains news extraction metrics
type NewsValidationResult struct {
	HasHeadline        bool        `json:"has_headline"`
	HasPublicationDate bool        `json:"has_publication_date"`
	HasAuthor          bool        `json:"has_author"`
	HasLeadParagraph   bool        `json:"has_lead_paragraph"`
	ContentLength      int         `json:"content_length"`
	EstimatedReadTime  int         `json:"estimated_read_time"` // minutes
	FreshnessScore     float64     `json:"freshness_score"`     // 0-100
	DomainAuthority    float64     `json:"domain_authority"`    // 0-100
	Score              float64     `json:"score"`               // 0-100
	DetectedDate       time.Time   `json:"detected_date,omitempty"`
	Issues             []NewsIssue `json:"issues"`
}

// NewsIssue represents a news extraction problem
type NewsIssue struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	Severity    string `json:"severity"`
}

// ValidateExtract validates extracted news content
func (v *NewsValidator) ValidateExtract(content string, sourceURL string) NewsValidationResult {
	result := NewsValidationResult{
		Issues: make([]NewsIssue, 0),
	}

	// Check for headline (usually first line or in content)
	result.HasHeadline = v.hasHeadline(content)

	// Try to extract publication date
	date := v.extractDate(content)
	result.HasPublicationDate = !date.IsZero()
	result.DetectedDate = date

	// Check for author
	result.HasAuthor = v.hasAuthor(content)

	// Check for lead paragraph (first substantial paragraph)
	result.HasLeadParagraph = v.hasLeadParagraph(content)

	// Calculate content metrics
	result.ContentLength = len(content)
	result.EstimatedReadTime = estimateReadTime(content)

	// Calculate freshness score
	result.FreshnessScore = v.calculateFreshness(date)

	// Calculate domain authority (from URL)
	result.DomainAuthority = calculateNewsDomainAuthority(sourceURL)

	// Calculate overall score
	result.Score = v.calculateScore(result)

	// Collect issues
	result.Issues = v.collectIssues(result)

	return result
}

// hasHeadline checks if content has a headline structure
func (v *NewsValidator) hasHeadline(content string) bool {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		return false
	}

	// Check first line for header markdown or all caps
	firstLine := strings.TrimSpace(lines[0])
	if strings.HasPrefix(firstLine, "# ") {
		return true
	}

	// Check if it looks like a headline (not too long, no punctuation at end)
	if len(firstLine) < 150 && len(firstLine) > 10 {
		lastChar := firstLine[len(firstLine)-1]
		if lastChar != '.' && lastChar != ',' && lastChar != ';' {
			return true
		}
	}

	return false
}

// extractDate attempts to find and parse a date in the content
func (v *NewsValidator) extractDate(content string) time.Time {
	// Common date patterns
	datePatterns := []string{
		`\b(\d{1,2})[/-](\d{1,2})[/-](\d{2,4})\b`, // 01/02/2024 or 01-02-24
		`\b(\d{4})[/-](\d{1,2})[/-](\d{1,2})\b`,   // 2024-01-02
		`\b(January|February|March|April|May|June|July|August|September|October|November|December)\s+(\d{1,2}),?\s+(\d{4})\b`, // January 2, 2024
		`\b(\d{1,2})\s+(January|February|March|April|May|June|July|August|September|October|November|December)\s+(\d{4})\b`,   // 2 January 2024
		`\b(\d{1,2})\s+(Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)\.?\s+(\d{4})\b`,                                      // 2 Jan 2024
	}

	for _, pattern := range datePatterns {
		re := regexp.MustCompile(pattern)
		match := re.FindString(content)
		if match != "" {
			// Try to parse the date
			parsed := tryParseDate(match)
			if !parsed.IsZero() {
				return parsed
			}
		}
	}

	return time.Time{}
}

// tryParseDate attempts to parse various date formats
func tryParseDate(dateStr string) time.Time {
	formats := []string{
		"2006-01-02",
		"2006/01/02",
		"01/02/2006",
		"01-02-2006",
		"January 2, 2006",
		"January 2 2006",
		"2 January 2006",
		"02 Jan 2006",
		"2 Jan 2006",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, dateStr); err == nil {
			return t
		}
	}

	return time.Time{}
}

// hasAuthor checks if content has author information
func (v *NewsValidator) hasAuthor(content string) bool {
	authorPatterns := []string{
		`(?i)by\s+[A-Z][a-z]+\s+[A-Z][a-z]+`, // By John Smith
		`(?i)author[s]?:?\s*[A-Z]`,           // Author: John
		`(?i)written\s+by\s+[A-Z]`,           // Written by John
		`(?i)reporting\s+by\s+[A-Z]`,         // Reporting by John
	}

	for _, pattern := range authorPatterns {
		re := regexp.MustCompile(pattern)
		if re.FindString(content) != "" {
			return true
		}
	}

	return false
}

// hasLeadParagraph checks if there's a substantial opening paragraph
func (v *NewsValidator) hasLeadParagraph(content string) bool {
	paragraphs := strings.Split(content, "\n\n")
	if len(paragraphs) == 0 {
		return false
	}

	// First substantial paragraph (not just headers)
	for _, p := range paragraphs {
		p = strings.TrimSpace(p)
		// Skip markdown headers
		if strings.HasPrefix(p, "#") {
			continue
		}
		// Check if it's substantial (at least 100 chars)
		if len(p) >= 100 {
			return true
		}
	}

	return false
}

// estimateReadTime estimates reading time in minutes
func estimateReadTime(content string) int {
	words := len(strings.Fields(content))
	// Average reading speed: 200 words per minute
	minutes := words / 200
	if minutes < 1 {
		return 1
	}
	return minutes
}

// calculateFreshness calculates freshness score based on content age
func (v *NewsValidator) calculateFreshness(contentDate time.Time) float64 {
	if contentDate.IsZero() {
		return 50 // Neutral if no date detected
	}

	age := time.Since(contentDate).Hours()
	maxAge := float64(v.maxAgeHours)

	if age <= 0 {
		return 100
	}

	if age >= maxAge {
		return 0
	}

	// Linear decay
	return (1.0 - age/maxAge) * 100
}

// calculateNewsDomainAuthority scores news source authority
func calculateNewsDomainAuthority(sourceURL string) float64 {
	// High authority news domains
	highAuthority := map[string]bool{
		"reuters.com":        true,
		"apnews.com":         true,
		"bbc.com":            true,
		"bbc.co.uk":          true,
		"nytimes.com":        true,
		"washingtonpost.com": true,
		"wsj.com":            true,
		"ft.com":             true,
		"economist.com":      true,
		"bloomberg.com":      true,
		"cnn.com":            true,
		"theguardian.com":    true,
		"npr.org":            true,
		"aljazeera.com":      true,
	}

	// Medium authority
	mediumAuthority := map[string]bool{
		"techcrunch.com":  true,
		"theverge.com":    true,
		"wired.com":       true,
		"arstechnica.com": true,
		"engadget.com":    true,
		"cnet.com":        true,
		"zdnet.com":       true,
		"venturebeat.com": true,
	}

	domain := extractDomainFromURL(sourceURL)

	if highAuthority[domain] {
		return 100
	}
	if mediumAuthority[domain] {
		return 75
	}

	return 50 // Unknown domain
}

// calculateScore computes overall news extraction score
func (v *NewsValidator) calculateScore(result NewsValidationResult) float64 {
	weights := []float64{
		0.20, // Has headline
		0.20, // Has publication date
		0.10, // Has author
		0.15, // Has lead paragraph
		0.15, // Content length (normalized)
		0.10, // Freshness
		0.10, // Domain authority
	}

	scores := []float64{
		boolToScore(result.HasHeadline),
		boolToScore(result.HasPublicationDate),
		boolToScore(result.HasAuthor),
		boolToScore(result.HasLeadParagraph),
		normalizeContentLength(result.ContentLength),
		result.FreshnessScore,
		result.DomainAuthority,
	}

	var total float64
	for i, score := range scores {
		total += score * weights[i]
	}

	return clamp(total, 0, 100)
}

// collectIssues identifies news extraction problems
func (v *NewsValidator) collectIssues(result NewsValidationResult) []NewsIssue {
	issues := make([]NewsIssue, 0)

	if !result.HasHeadline {
		issues = append(issues, NewsIssue{
			Type:        "missing_headline",
			Description: "No headline detected in extracted content",
			Severity:    "warning",
		})
	}

	if !result.HasPublicationDate {
		issues = append(issues, NewsIssue{
			Type:        "missing_date",
			Description: "No publication date detected",
			Severity:    "info",
		})
	}

	if result.FreshnessScore < 30 {
		issues = append(issues, NewsIssue{
			Type:        "stale_content",
			Description: "Content appears to be older than freshness threshold",
			Severity:    "warning",
		})
	}

	if result.ContentLength < 500 {
		issues = append(issues, NewsIssue{
			Type:        "short_content",
			Description: "Extracted content is very short, may be incomplete",
			Severity:    "warning",
		})
	}

	return issues
}

// Helper functions
func boolToScore(b bool) float64 {
	if b {
		return 100
	}
	return 0
}

func normalizeContentLength(length int) float64 {
	// Ideal news article: 1000-3000 chars
	switch {
	case length >= 1000 && length <= 3000:
		return 100
	case length >= 500 && length < 1000:
		return 75
	case length >= 3000 && length <= 5000:
		return 80
	case length > 5000:
		return 60 // Might be too long (multiple articles?)
	case length >= 200 && length < 500:
		return 50
	default:
		return 25
	}
}

func extractDomainFromURL(sourceURL string) string {
	// Simple domain extraction
	if idx := strings.Index(sourceURL, "://"); idx != -1 {
		sourceURL = sourceURL[idx+3:]
	}
	if idx := strings.Index(sourceURL, "/"); idx != -1 {
		sourceURL = sourceURL[:idx]
	}
	if idx := strings.Index(sourceURL, ":"); idx != -1 {
		sourceURL = sourceURL[:idx]
	}
	return strings.TrimPrefix(sourceURL, "www.")
}
