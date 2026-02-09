package domains

import (
	"regexp"
	"strings"
)

// AcademicValidator validates academic/research content extraction quality
type AcademicValidator struct {
	expectedCitationFormat string
}

// NewAcademicValidator creates a new academic validator
func NewAcademicValidator(citationFormat string) *AcademicValidator {
	return &AcademicValidator{
		expectedCitationFormat: citationFormat,
	}
}

// AcademicValidationResult contains academic extraction metrics
type AcademicValidationResult struct {
	HasAbstract         bool            `json:"has_abstract"`
	HasCitations        bool            `json:"has_citations"`
	CitationCount       int             `json:"citation_count"`
	HasReferences       bool            `json:"has_references"`
	HasMethodology      bool            `json:"has_methodology"`
	HasResults          bool            `json:"has_results"`
	DetectedSections    []string        `json:"detected_sections"`
	AcademicTermCount   int             `json:"academic_term_count"`
	PaperLengthScore    float64         `json:"paper_length_score"`    // 0-100
	CitationFormatScore float64         `json:"citation_format_score"` // 0-100
	AcademicToneScore   float64         `json:"academic_tone_score"`   // 0-100
	Score               float64         `json:"score"`                 // 0-100
	Issues              []AcademicIssue `json:"issues"`
}

// AcademicIssue represents an academic extraction problem
type AcademicIssue struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	Severity    string `json:"severity"`
}

// ValidateExtract validates extracted academic content
func (v *AcademicValidator) ValidateExtract(content string) AcademicValidationResult {
	result := AcademicValidationResult{
		DetectedSections: make([]string, 0),
		Issues:           make([]AcademicIssue, 0),
	}

	// Check for abstract
	result.HasAbstract = v.hasAbstract(content)
	if result.HasAbstract {
		result.DetectedSections = append(result.DetectedSections, "abstract")
	}

	// Count citations
	result.CitationCount = v.countCitations(content)
	result.HasCitations = result.CitationCount > 0

	// Check for references section
	result.HasReferences = v.hasReferences(content)
	if result.HasReferences {
		result.DetectedSections = append(result.DetectedSections, "references")
	}

	// Check for methodology section
	result.HasMethodology = v.hasMethodology(content)
	if result.HasMethodology {
		result.DetectedSections = append(result.DetectedSections, "methodology")
	}

	// Check for results section
	result.HasResults = v.hasResults(content)
	if result.HasResults {
		result.DetectedSections = append(result.DetectedSections, "results")
	}

	// Count academic terminology
	result.AcademicTermCount = v.countAcademicTerms(content)

	// Calculate paper length score
	result.PaperLengthScore = v.calculatePaperLengthScore(content)

	// Calculate citation format score
	result.CitationFormatScore = v.assessCitationFormat(content)

	// Calculate academic tone score
	result.AcademicToneScore = v.assessAcademicTone(content)

	// Calculate overall score
	result.Score = v.calculateScore(result)

	// Collect issues
	result.Issues = v.collectIssues(result)

	return result
}

// hasAbstract checks for abstract section
func (v *AcademicValidator) hasAbstract(content string) bool {
	abstractPatterns := []string{
		`(?i)^\s*abstract\s*\n`,
		`(?i)\n\s*abstract\s*\n`,
		`(?i)^\s*summary\s*\n`,
	}

	for _, pattern := range abstractPatterns {
		re := regexp.MustCompile(pattern)
		if re.FindString(content) != "" {
			return true
		}
	}

	return false
}

// countCitations counts academic citations
func (v *AcademicValidator) countCitations(content string) int {
	citationPatterns := []string{
		`\[\d+\]`,           // [1], [23]
		`\(\w+\s+et\s+al\.`, // (Smith et al.
		`\(\w+,\s*\d{4}\)`,  // (Smith, 2024)
		`\[\w+\s+\d{4}\]`,   // [Smith 2024]
	}

	total := 0
	for _, pattern := range citationPatterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindAllString(content, -1)
		total += len(matches)
	}

	return total
}

// hasReferences checks for references/bibliography section
func (v *AcademicValidator) hasReferences(content string) bool {
	referencePatterns := []string{
		`(?i)\n\s*references?\s*\n`,
		`(?i)\n\s*bibliography\s*\n`,
		`(?i)\n\s*works?\s+cited\s*\n`,
	}

	for _, pattern := range referencePatterns {
		re := regexp.MustCompile(pattern)
		if re.FindString(content) != "" {
			return true
		}
	}

	return false
}

// hasMethodology checks for methodology section
func (v *AcademicValidator) hasMethodology(content string) bool {
	methodPatterns := []string{
		`(?i)\n\s*methodology\s*\n`,
		`(?i)\n\s*methods?\s*\n`,
		`(?i)\n\s*experimental\s+setup\s*\n`,
		`(?i)\n\s*materials?\s+and\s+methods?\s*\n`,
	}

	for _, pattern := range methodPatterns {
		re := regexp.MustCompile(pattern)
		if re.FindString(content) != "" {
			return true
		}
	}

	return false
}

// hasResults checks for results section
func (v *AcademicValidator) hasResults(content string) bool {
	resultPatterns := []string{
		`(?i)\n\s*results?\s*\n`,
		`(?i)\n\s*findings?\s*\n`,
		`(?i)\n\s*outcomes?\s*\n`,
	}

	for _, pattern := range resultPatterns {
		re := regexp.MustCompile(pattern)
		if re.FindString(content) != "" {
			return true
		}
	}

	return false
}

// countAcademicTerms counts academic terminology
func (v *AcademicValidator) countAcademicTerms(content string) int {
	academicTerms := []string{
		"hypothesis", "methodology", "significant", "correlation",
		"analysis", "conclusion", "framework", "literature review",
		"empirical", "theoretical", "quantitative", "qualitative",
		"statistical", "evidence", "demonstrates", "indicates",
		"participants", "sample", "data", "findings",
		"peer-reviewed", "doi", "journal", "conference",
		"abstract", "introduction", "discussion", "implications",
	}

	contentLower := strings.ToLower(content)
	count := 0
	for _, term := range academicTerms {
		count += strings.Count(contentLower, term)
	}

	return count
}

// calculatePaperLengthScore assesses if content length is appropriate for academic paper
func (v *AcademicValidator) calculatePaperLengthScore(content string) float64 {
	length := len(content)

	// Academic papers typically 3000-15000 words (~15000-75000 chars)
	switch {
	case length >= 5000 && length <= 50000:
		return 100 // Good range
	case length >= 3000 && length < 5000:
		return 80 // Short but acceptable
	case length > 50000 && length <= 80000:
		return 90 // Long but acceptable
	case length >= 1000 && length < 3000:
		return 60 // Abstract or short paper
	case length > 80000:
		return 50 // Possibly multiple papers
	default:
		return 30 // Too short
	}
}

// assessCitationFormat checks if citations follow expected format
func (v *AcademicValidator) assessCitationFormat(content string) float64 {
	if v.expectedCitationFormat == "" {
		// Just check for any consistent citation pattern
		return v.detectAnyCitationFormat(content)
	}

	switch strings.ToLower(v.expectedCitationFormat) {
	case "ieee":
		return v.assessIEEECitations(content)
	case "apa":
		return v.assessAPACitations(content)
	case "mla":
		return v.assessMLACitations(content)
	case "harvard":
		return v.assessHarvardCitations(content)
	default:
		return v.detectAnyCitationFormat(content)
	}
}

// assessIEEECitations checks for IEEE format [1], [2,3]
func (v *AcademicValidator) assessIEEECitations(content string) float64 {
	ieeePattern := regexp.MustCompile(`\[\d+(?:\s*,\s*\d+)*\]`)
	matches := ieeePattern.FindAllString(content, -1)

	if len(matches) == 0 {
		return 0
	}

	// Score based on citation count and consistency
	if len(matches) >= 5 {
		return 100
	} else if len(matches) >= 3 {
		return 80
	}
	return 60
}

// assessAPACitations checks for APA format (Author, Year)
func (v *AcademicValidator) assessAPACitations(content string) float64 {
	apaPattern := regexp.MustCompile(`\(\w+(?:\s+et\s+al\.)?,?\s*\d{4}[a-z]?\)`)
	matches := apaPattern.FindAllString(content, -1)

	if len(matches) == 0 {
		return 0
	}

	if len(matches) >= 5 {
		return 100
	} else if len(matches) >= 3 {
		return 80
	}
	return 60
}

// assessMLACitations checks for MLA format (Author Page)
func (v *AcademicValidator) assessMLACitations(content string) float64 {
	mlaPattern := regexp.MustCompile(`\(\w+\s+\d+\)`)
	matches := mlaPattern.FindAllString(content, -1)

	if len(matches) == 0 {
		return 0
	}

	if len(matches) >= 5 {
		return 100
	} else if len(matches) >= 3 {
		return 80
	}
	return 60
}

// assessHarvardCitations checks for Harvard format (Author, Year)
func (v *AcademicValidator) assessHarvardCitations(content string) float64 {
	// Harvard is similar to APA
	return v.assessAPACitations(content)
}

// detectAnyCitationFormat scores based on presence of any citation format
func (v *AcademicValidator) detectAnyCitationFormat(content string) float64 {
	citationPatterns := []string{
		`\[\d+\]`,           // [1]
		`\(\w+\s+et\s+al\.`, // (Smith et al.)
		`\(\w+,?\s*\d{4}\)`, // (Smith, 2024)
		`\w+\s+\(\d{4}\)`,   // Smith (2024)
		`\d{4}\.\s*\w+`,     // 2024. Title
	}

	totalMatches := 0
	for _, pattern := range citationPatterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindAllString(content, -1)
		totalMatches += len(matches)
	}

	if totalMatches >= 10 {
		return 100
	} else if totalMatches >= 5 {
		return 80
	} else if totalMatches >= 2 {
		return 60
	}
	return 30
}

// assessAcademicTone checks for academic writing characteristics
func (v *AcademicValidator) assessAcademicTone(content string) float64 {
	score := 50.0 // Start neutral

	// Positive indicators
	academicPhrases := []string{
		"this study", "the results indicate", "we found that",
		"our findings", "the data suggest", "in conclusion",
		"further research", "limitations", "implications",
	}

	contentLower := strings.ToLower(content)
	for _, phrase := range academicPhrases {
		if strings.Contains(contentLower, phrase) {
			score += 5
		}
	}

	// Negative indicators (too informal)
	informalPatterns := []string{
		"i think", "you should", "a lot of", "really",
		"very", "just", "basically", "actually",
	}

	for _, pattern := range informalPatterns {
		if strings.Count(contentLower, pattern) > 3 {
			score -= 5
		}
	}

	return clamp(score, 0, 100)
}

// calculateScore computes overall academic extraction score
func (v *AcademicValidator) calculateScore(result AcademicValidationResult) float64 {
	weights := []float64{
		0.15, // Has abstract
		0.15, // Has citations
		0.10, // Has references
		0.10, // Has methodology
		0.10, // Has results
		0.10, // Academic term density
		0.10, // Paper length
		0.10, // Citation format
		0.10, // Academic tone
	}

	// Normalize academic term count (cap at 50 for 100%)
	termScore := float64(result.AcademicTermCount) / 50.0 * 100
	if termScore > 100 {
		termScore = 100
	}

	scores := []float64{
		boolToScore(result.HasAbstract),
		boolToScore(result.HasCitations),
		boolToScore(result.HasReferences),
		boolToScore(result.HasMethodology),
		boolToScore(result.HasResults),
		termScore,
		result.PaperLengthScore,
		result.CitationFormatScore,
		result.AcademicToneScore,
	}

	var total float64
	for i, score := range scores {
		total += score * weights[i]
	}

	return clamp(total, 0, 100)
}

// collectIssues identifies academic extraction problems
func (v *AcademicValidator) collectIssues(result AcademicValidationResult) []AcademicIssue {
	issues := make([]AcademicIssue, 0)

	if !result.HasAbstract {
		issues = append(issues, AcademicIssue{
			Type:        "missing_abstract",
			Description: "No abstract section detected",
			Severity:    "warning",
		})
	}

	if result.CitationCount == 0 {
		issues = append(issues, AcademicIssue{
			Type:        "no_citations",
			Description: "No academic citations found",
			Severity:    "error",
		})
	} else if result.CitationCount < 3 {
		issues = append(issues, AcademicIssue{
			Type:        "few_citations",
			Description: "Very few citations detected",
			Severity:    "warning",
		})
	}

	if !result.HasReferences {
		issues = append(issues, AcademicIssue{
			Type:        "missing_references",
			Description: "No references/bibliography section",
			Severity:    "warning",
		})
	}

	if result.AcademicToneScore < 50 {
		issues = append(issues, AcademicIssue{
			Type:        "informal_tone",
			Description: "Content appears to have informal tone",
			Severity:    "info",
		})
	}

	return issues
}
