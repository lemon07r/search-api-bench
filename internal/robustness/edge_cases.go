// Package robustness provides edge case and stress testing capabilities
package robustness

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
)

// EdgeCase represents an edge case test scenario
type EdgeCase struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Query       string `json:"query,omitempty"`
	URL         string `json:"url,omitempty"`
	ExpectError bool   `json:"expect_error"`
}

// EdgeCaseGenerator generates edge case test scenarios
type EdgeCaseGenerator struct{}

// NewEdgeCaseGenerator creates a new edge case generator
func NewEdgeCaseGenerator() *EdgeCaseGenerator {
	return &EdgeCaseGenerator{}
}

// GenerateSearchEdgeCases returns a set of edge case search queries
func (g *EdgeCaseGenerator) GenerateSearchEdgeCases() []EdgeCase {
	return []EdgeCase{
		{
			Name:        "empty_query",
			Description: "Empty search query",
			Query:       "",
			ExpectError: true,
		},
		{
			Name:        "whitespace_only",
			Description: "Query with only whitespace",
			Query:       "     ",
			ExpectError: true,
		},
		{
			Name:        "very_long_query",
			Description: "Extremely long query (500+ chars)",
			Query:       g.generateLongQuery(500),
			ExpectError: false,
		},
		{
			Name:        "single_character",
			Description: "Single character query",
			Query:       "a",
			ExpectError: false,
		},
		{
			Name:        "unicode_characters",
			Description: "Query with various unicode characters",
			Query:       "日本語 中文 العربية emoji 🚀 🎉 émojis",
			ExpectError: false,
		},
		{
			Name:        "special_characters",
			Description: "Query with special characters",
			Query:       `C++ & Java | Python! <script>alert(1)</script> "quoted" 'single'`,
			ExpectError: false,
		},
		{
			Name:        "sql_injection_attempt",
			Description: "Query resembling SQL injection",
			Query:       `1' OR '1'='1'; DROP TABLE users; --`,
			ExpectError: false,
		},
		{
			Name:        "path_traversal_attempt",
			Description: "Query with path traversal patterns",
			Query:       "../../../etc/passwd",
			ExpectError: false,
		},
		{
			Name:        "null_bytes",
			Description: "Query with null bytes",
			Query:       "test\x00query",
			ExpectError: false,
		},
		{
			Name:        "newlines_tabs",
			Description: "Query with newlines and tabs",
			Query:       "line1\nline2\n\tindented",
			ExpectError: false,
		},
		{
			Name:        "repeated_characters",
			Description: "Query with repeated characters",
			Query:       strings.Repeat("a", 100),
			ExpectError: false,
		},
		{
			Name:        "mixed_languages",
			Description: "Query mixing multiple languages",
			Query:       "machine learning 機器學習 التعلم الآلي машинное обучение",
			ExpectError: false,
		},
		{
			Name:        "html_entities",
			Description: "Query with HTML entities",
			Query:       "&lt;div&gt;test&lt;/div&gt; &amp; more",
			ExpectError: false,
		},
		{
			Name:        "url_encoded",
			Description: "Query with URL encoding",
			Query:       "test%20query%26more%3Dvalue",
			ExpectError: false,
		},
		{
			Name:        "mathematical_symbols",
			Description: "Query with mathematical notation",
			Query:       "E=mc² ∫∂x/∂y ∑∞ α±β≠γ",
			ExpectError: false,
		},
		{
			Name:        "control_characters",
			Description: "Query with control characters",
			Query:       "test\x01\x02\x03query",
			ExpectError: false,
		},
		{
			Name:        "emoji_only",
			Description: "Query with only emojis",
			Query:       "🚀💻🔬📊🎯",
			ExpectError: false,
		},
		{
			Name:        "zero_width_chars",
			Description: "Query with zero-width characters",
			Query:       "test\u200Bquery\u200Cmore",
			ExpectError: false,
		},
		{
			Name:        "right_to_left",
			Description: "Right-to-left text",
			Query:       "عربي English عربي",
			ExpectError: false,
		},
		{
			Name:        "extremely_niche",
			Description: "Very obscure topic that may have no results",
			Query:       "medieval Icelandic sheep farming techniques 1347 manuscript",
			ExpectError: false,
		},
		{
			Name:        "common_words_only",
			Description: "Query with only stop words",
			Query:       "the and or but in on at to for of",
			ExpectError: false,
		},
		{
			Name:        "numbers_only",
			Description: "Query with only numbers",
			Query:       "123456789 3.14159 0xDEADBEEF",
			ExpectError: false,
		},
		{
			Name:        "extremely_broad",
			Description: "Overly broad query",
			Query:       "everything",
			ExpectError: false,
		},
		{
			Name:        "question_format",
			Description: "Natural language question",
			Query:       "What is the meaning of life, the universe, and everything?",
			ExpectError: false,
		},
		{
			Name:        "code_snippet",
			Description: "Query containing code",
			Query:       `func main() { fmt.Println("Hello") }`,
			ExpectError: false,
		},
	}
}

// GenerateExtractEdgeCases returns edge case URLs for extraction
func (g *EdgeCaseGenerator) GenerateExtractEdgeCases() []EdgeCase {
	return []EdgeCase{
		{
			Name:        "empty_url",
			Description: "Empty URL",
			URL:         "",
			ExpectError: true,
		},
		{
			Name:        "malformed_url",
			Description: "Malformed URL",
			URL:         "not a url",
			ExpectError: true,
		},
		{
			Name:        "missing_protocol",
			Description: "URL without protocol",
			URL:         "example.com/page",
			ExpectError: false,
		},
		{
			Name:        "ftp_protocol",
			Description: "FTP URL",
			URL:         "ftp://ftp.example.com/file.txt",
			ExpectError: false,
		},
		{
			Name:        "file_protocol",
			Description: "File protocol URL",
			URL:         "file:///etc/passwd",
			ExpectError: true,
		},
		{
			Name:        "very_long_url",
			Description: "URL with very long query string",
			URL:         g.generateLongURL(),
			ExpectError: false,
		},
		{
			Name:        "special_chars_in_path",
			Description: "URL with special characters in path",
			URL:         "https://example.com/path/with spaces/and+plus?key=value&other=test",
			ExpectError: false,
		},
		{
			Name:        "unicode_domain",
			Description: "URL with unicode domain (punycode)",
			URL:         "https://münchen.example/",
			ExpectError: false,
		},
		{
			Name:        "fragment_only",
			Description: "URL with only fragment",
			URL:         "https://example.com/page#section-1",
			ExpectError: false,
		},
		{
			Name:        "localhost",
			Description: "Localhost URL",
			URL:         "http://localhost:8080/",
			ExpectError: true,
		},
		{
			Name:        "private_ip",
			Description: "Private IP address",
			URL:         "http://192.168.1.1/",
			ExpectError: true,
		},
		{
			Name:        "nonexistent_domain",
			Description: "Non-existent domain",
			URL:         "https://this-domain-definitely-does-not-exist-12345.com/",
			ExpectError: true,
		},
		{
			Name:        "redirect_chain",
			Description: "URL with known redirect chain",
			URL:         "http://bit.ly/3example", // May or may not exist
			ExpectError: false,
		},
		{
			Name:        "javascript_scheme",
			Description: "JavaScript protocol",
			URL:         "javascript:alert('xss')",
			ExpectError: true,
		},
		{
			Name:        "data_uri",
			Description: "Data URI",
			URL:         "data:text/html,<script>alert('xss')</script>",
			ExpectError: true,
		},
		{
			Name:        "very_deep_path",
			Description: "URL with very deep path",
			URL:         g.generateDeepPathURL(),
			ExpectError: false,
		},
	}
}

// GenerateCrawlEdgeCases returns edge case URLs for crawling
func (g *EdgeCaseGenerator) GenerateCrawlEdgeCases() []EdgeCase {
	return []EdgeCase{
		{
			Name:        "single_page_site",
			Description: "Single page website",
			URL:         "https://example.com/",
			ExpectError: false,
		},
		{
			Name:        "large_site",
			Description: "Very large website",
			URL:         "https://wikipedia.org/",
			ExpectError: false,
		},
		{
			Name:        "spa_application",
			Description: "Single page application",
			URL:         "https://example.com/app",
			ExpectError: false,
		},
		{
			Name:        "heavy_javascript",
			Description: "Site requiring JavaScript",
			URL:         "https://example.com/js-heavy",
			ExpectError: false,
		},
		{
			Name:        "redirect_homepage",
			Description: "Homepage that redirects",
			URL:         "http://example.com", // May redirect to https
			ExpectError: false,
		},
		{
			Name:        "canonical_redirects",
			Description: "Site with many canonical redirects",
			URL:         "https://example.com/redirects/",
			ExpectError: false,
		},
		{
			Name:        "query_params_everywhere",
			Description: "Site with heavy query parameter usage",
			URL:         "https://example.com/search?q=test&page=1&sort=date",
			ExpectError: false,
		},
		{
			Name:        "pdf_site",
			Description: "Site serving primarily PDFs",
			URL:         "https://example.com/documents/",
			ExpectError: false,
		},
		{
			Name:        "image_heavy",
			Description: "Image-heavy site",
			URL:         "https://example.com/gallery/",
			ExpectError: false,
		},
		{
			Name:        "video_content",
			Description: "Video content site",
			URL:         "https://example.com/videos/",
			ExpectError: false,
		},
	}
}

// Helper methods

func (g *EdgeCaseGenerator) generateLongQuery(length int) string {
	words := []string{
		"the", "quick", "brown", "fox", "jumps", "over", "lazy", "dog",
		"lorem", "ipsum", "dolor", "sit", "amet", "consectetur", "adipiscing", "elit",
	}

	var result strings.Builder
	for result.Len() < length {
		idx, _ := rand.Int(rand.Reader, big.NewInt(int64(len(words))))
		result.WriteString(words[idx.Int64()])
		result.WriteString(" ")
	}

	return result.String()
}

func (g *EdgeCaseGenerator) generateLongURL() string {
	base := "https://example.com/search?"
	params := make([]string, 20)
	for i := range params {
		val, _ := rand.Int(rand.Reader, big.NewInt(1000))
		params[i] = fmt.Sprintf("param%d=value%d", i, val.Int64())
	}
	return base + strings.Join(params, "&")
}

func (g *EdgeCaseGenerator) generateDeepPathURL() string {
	parts := make([]string, 15)
	for i := range parts {
		parts[i] = fmt.Sprintf("level%d", i)
	}
	return "https://example.com/" + strings.Join(parts, "/")
}

// GetRandomSubset returns a random subset of edge cases
func (g *EdgeCaseGenerator) GetRandomSubset(cases []EdgeCase, count int) []EdgeCase {
	if count >= len(cases) {
		return cases
	}

	// Fisher-Yates shuffle using crypto/rand
	shuffled := make([]EdgeCase, len(cases))
	copy(shuffled, cases)

	for i := len(shuffled) - 1; i > 0; i-- {
		jBig, _ := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		j := int(jBig.Int64())
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	}

	return shuffled[:count]
}
