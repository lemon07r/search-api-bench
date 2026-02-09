// Package domains provides domain-specific quality validators
package domains

import (
	"regexp"
	"strings"
)

// CodeValidator validates code extraction quality
type CodeValidator struct {
	expectedLanguages []string
}

// NewCodeValidator creates a new code validator
func NewCodeValidator(languages []string) *CodeValidator {
	return &CodeValidator{
		expectedLanguages: languages,
	}
}

// CodeValidationResult contains code extraction metrics
type CodeValidationResult struct {
	CodeBlocksFound    int         `json:"code_blocks_found"`
	InlineCodeFound    int         `json:"inline_code_found"`
	LanguagesDetected  []string    `json:"languages_detected"`
	SyntaxHighlighted  float64     `json:"syntax_highlighted"` // 0-100
	FunctionSignatures int         `json:"function_signatures"`
	ImportStatements   int         `json:"import_statements"`
	CodeCompleteness   float64     `json:"code_completeness"`  // 0-100
	CommentsPreserved  float64     `json:"comments_preserved"` // 0-100
	Score              float64     `json:"score"`              // 0-100
	Issues             []CodeIssue `json:"issues"`
}

// CodeIssue represents a code extraction problem
type CodeIssue struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	Severity    string `json:"severity"` // error, warning, info
}

// ValidateExtract validates extracted code content
func (v *CodeValidator) ValidateExtract(content string) CodeValidationResult {
	result := CodeValidationResult{
		LanguagesDetected: make([]string, 0),
		Issues:            make([]CodeIssue, 0),
	}

	// Count fenced code blocks
	result.CodeBlocksFound = countFencedCodeBlocks(content)

	// Count inline code
	result.InlineCodeFound = countInlineCode(content)

	// Detect languages
	result.LanguagesDetected = detectCodeLanguages(content)

	// Check syntax highlighting hints
	result.SyntaxHighlighted = assessSyntaxHighlighting(content, result.LanguagesDetected)

	// Count function signatures
	result.FunctionSignatures = countFunctionSignatures(content, result.LanguagesDetected)

	// Count import statements
	result.ImportStatements = countImportStatements(content, result.LanguagesDetected)

	// Assess code completeness
	result.CodeCompleteness = assessCodeCompleteness(content, result.CodeBlocksFound)

	// Assess comments preservation
	result.CommentsPreserved = assessCommentsPreserved(content)

	// Calculate overall score
	result.Score = v.calculateScore(result)

	// Collect issues
	result.Issues = v.collectIssues(result)

	return result
}

// countFencedCodeBlocks counts ``` fenced code blocks
func countFencedCodeBlocks(content string) int {
	// Count occurrences of ``` (must be at start of line or after newline)
	count := 0
	lines := strings.Split(content, "\n")
	inCodeBlock := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			if !inCodeBlock {
				count++
				inCodeBlock = true
			} else {
				inCodeBlock = false
			}
		}
	}

	return count
}

// countInlineCode counts `inline code` snippets
func countInlineCode(content string) int {
	// Match single backticks not followed by another backtick
	re := regexp.MustCompile("`[^`]+`")
	matches := re.FindAllString(content, -1)
	return len(matches)
}

// detectCodeLanguages identifies programming languages from code blocks
func detectCodeLanguages(content string) []string {
	languages := make(map[string]bool)

	// Look for language hints after opening ```
	re := regexp.MustCompile("```(\\w+)")
	matches := re.FindAllStringSubmatch(content, -1)

	for _, match := range matches {
		if len(match) > 1 {
			lang := strings.ToLower(match[1])
			languages[lang] = true
		}
	}

	// Also detect from code patterns
	if isGoCode(content) {
		languages["go"] = true
	}
	if isPythonCode(content) {
		languages["python"] = true
	}
	if isJavaScriptCode(content) {
		languages["javascript"] = true
	}
	if isRustCode(content) {
		languages["rust"] = true
	}

	result := make([]string, 0, len(languages))
	for lang := range languages {
		result = append(result, lang)
	}

	return result
}

// assessSyntaxHighlighting checks if syntax highlighting hints are preserved
func assessSyntaxHighlighting(content string, languages []string) float64 {
	if len(languages) == 0 {
		return 0
	}

	// Count how many code blocks have language hints
	totalBlocks := countFencedCodeBlocks(content)
	if totalBlocks == 0 {
		return 0
	}

	blocksWithHints := 0
	re := regexp.MustCompile("```(\\w+)")
	matches := re.FindAllString(content, -1)
	blocksWithHints = len(matches)

	if totalBlocks == 0 {
		return 50 // Neutral if no fenced blocks
	}

	return float64(blocksWithHints) / float64(totalBlocks) * 100
}

// countFunctionSignatures counts function/method definitions
func countFunctionSignatures(content string, languages []string) int {
	count := 0

	// Go: func Name(
	goFuncRe := regexp.MustCompile(`(?m)^func\s+\w+\s*\(`)
	count += len(goFuncRe.FindAllString(content, -1))

	// Python: def name(
	pyFuncRe := regexp.MustCompile(`(?m)^def\s+\w+\s*\(`)
	count += len(pyFuncRe.FindAllString(content, -1))

	// JavaScript/TypeScript: function name( or const name = ( or name(
	jsFuncRe := regexp.MustCompile(`(?m)(function\s+\w+|const\s+\w+\s*=\s*\(|\w+\s*:\s*function)`)
	count += len(jsFuncRe.FindAllString(content, -1))

	// Rust: fn name(
	rustFuncRe := regexp.MustCompile(`(?m)^fn\s+\w+\s*\(`)
	count += len(rustFuncRe.FindAllString(content, -1))

	return count
}

// countImportStatements counts import/include statements
func countImportStatements(content string, languages []string) int {
	count := 0

	// Go: import
	goImportRe := regexp.MustCompile(`(?m)^import\s+["(]`)
	count += len(goImportRe.FindAllString(content, -1))

	// Python: import or from ... import
	pyImportRe := regexp.MustCompile(`(?m)^(import\s+\w+|from\s+\w+\s+import)`)
	count += len(pyImportRe.FindAllString(content, -1))

	// JavaScript: import or require
	jsImportRe := regexp.MustCompile(`(?m)^(import\s+|const\s+\w+\s+=\s+require)`)
	count += len(jsImportRe.FindAllString(content, -1))

	// Rust: use
	rustImportRe := regexp.MustCompile(`(?m)^use\s+\w+`)
	count += len(rustImportRe.FindAllString(content, -1))

	return count
}

// assessCodeCompleteness checks for truncated code indicators
func assessCodeCompleteness(content string, codeBlocks int) float64 {
	if codeBlocks == 0 {
		return 100 // No code to check
	}

	score := 100.0

	// Check for truncation indicators in code
	truncationPatterns := []string{
		"// ...", "# ...", "/* ...", "// …",
	}

	for _, pattern := range truncationPatterns {
		if strings.Contains(content, pattern) {
			score -= 10
		}
	}

	// Check for unclosed code blocks
	openCount := strings.Count(content, "```")
	if openCount%2 != 0 {
		score -= 20 // Unclosed code block
	}

	return clamp(score, 0, 100)
}

// assessCommentsPreserved checks if code comments are intact
func assessCommentsPreserved(content string) float64 {
	score := 100.0

	// Count comment indicators
	singleLineComments := strings.Count(content, "//") +
		strings.Count(content, "#") +
		strings.Count(content, "--")

	multiLineComments := strings.Count(content, "/*") +
		strings.Count(content, "*/")

	if singleLineComments == 0 && multiLineComments == 0 {
		return 50 // Neutral - no comments detected
	}

	// Check for broken comment patterns
	openML := strings.Count(content, "/*")
	closeML := strings.Count(content, "*/")
	if openML != closeML {
		score -= 20 // Mismatched multiline comments
	}

	return clamp(score, 0, 100)
}

// isGoCode detects Go code patterns
func isGoCode(content string) bool {
	patterns := []string{
		"package main", "package ", "func main()", "fmt.",
		"go mod", "struct {", "interface {",
	}
	return hasPatterns(content, patterns)
}

// isPythonCode detects Python code patterns
func isPythonCode(content string) bool {
	patterns := []string{
		"def ", "import ", "print(", "if __name__", ":\n    ",
		"class ", "self.", "pip install",
	}
	return hasPatterns(content, patterns)
}

// isJavaScriptCode detects JavaScript/TypeScript patterns
func isJavaScriptCode(content string) bool {
	patterns := []string{
		"const ", "let ", "var ", "function ", "=>", "console.log",
		"npm install", "require(", "module.exports",
	}
	return hasPatterns(content, patterns)
}

// isRustCode detects Rust code patterns
func isRustCode(content string) bool {
	patterns := []string{
		"fn main()", "let mut", "impl ", "cargo ",
		"unwrap()", "Some(", "Result<", "match ",
	}
	return hasPatterns(content, patterns)
}

// hasPatterns checks if content contains any of the patterns
func hasPatterns(content string, patterns []string) bool {
	for _, pattern := range patterns {
		if strings.Contains(content, pattern) {
			return true
		}
	}
	return false
}

// calculateScore computes overall code extraction score
func (v *CodeValidator) calculateScore(result CodeValidationResult) float64 {
	if result.CodeBlocksFound == 0 && result.InlineCodeFound == 0 {
		return 0
	}

	weights := []float64{
		0.20, // Code blocks
		0.15, // Inline code
		0.15, // Syntax highlighting
		0.15, // Function signatures
		0.10, // Import statements
		0.15, // Code completeness
		0.10, // Comments preserved
	}

	// Normalize code blocks score (cap at 5 blocks = 100%)
	blockScore := float64(result.CodeBlocksFound) / 5.0 * 100
	if blockScore > 100 {
		blockScore = 100
	}

	// Normalize inline code score (cap at 10 snippets = 100%)
	inlineScore := float64(result.InlineCodeFound) / 10.0 * 100
	if inlineScore > 100 {
		inlineScore = 100
	}

	// Normalize function signatures (cap at 10 = 100%)
	funcScore := float64(result.FunctionSignatures) / 10.0 * 100
	if funcScore > 100 {
		funcScore = 100
	}

	// Normalize imports (cap at 5 = 100%)
	importScore := float64(result.ImportStatements) / 5.0 * 100
	if importScore > 100 {
		importScore = 100
	}

	scores := []float64{
		blockScore,
		inlineScore,
		result.SyntaxHighlighted,
		funcScore,
		importScore,
		result.CodeCompleteness,
		result.CommentsPreserved,
	}

	var total float64
	for i, score := range scores {
		total += score * weights[i]
	}

	return clamp(total, 0, 100)
}

// collectIssues identifies code extraction problems
func (v *CodeValidator) collectIssues(result CodeValidationResult) []CodeIssue {
	issues := make([]CodeIssue, 0)

	if result.CodeBlocksFound == 0 && result.InlineCodeFound == 0 {
		issues = append(issues, CodeIssue{
			Type:        "missing_code",
			Description: "No code blocks or inline code detected",
			Severity:    "error",
		})
	}

	if result.SyntaxHighlighted < 50 && result.CodeBlocksFound > 0 {
		issues = append(issues, CodeIssue{
			Type:        "missing_syntax_hints",
			Description: "Most code blocks lack language hints for syntax highlighting",
			Severity:    "warning",
		})
	}

	if result.CodeCompleteness < 70 {
		issues = append(issues, CodeIssue{
			Type:        "truncated_code",
			Description: "Code appears to be truncated or incomplete",
			Severity:    "warning",
		})
	}

	return issues
}

// clamp constrains value between min and max
func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
