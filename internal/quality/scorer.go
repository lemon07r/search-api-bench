package quality

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"strings"
	"time"

	"github.com/lamim/search-api-bench/internal/providers"
)

// Scorer provides comprehensive quality scoring for search/crawl/extract operations
type Scorer struct {
	embedding *EmbeddingClient
	reranker  *RerankerClient
	weights   ScoreWeights
}

// NewScorer creates a new quality scorer
func NewScorer(embedding *EmbeddingClient, reranker *RerankerClient) *Scorer {
	return &Scorer{
		embedding: embedding,
		reranker:  reranker,
		weights:   DefaultScoreWeights(),
	}
}

// SetWeights updates the scoring weights
func (s *Scorer) SetWeights(weights ScoreWeights) {
	s.weights = weights
}

// ScoreSearch performs comprehensive quality scoring for search results
func (s *Scorer) ScoreSearch(ctx context.Context, query string, results []providers.SearchItem) (SearchQualityScore, error) {
	if len(results) == 0 {
		return SearchQualityScore{}, nil
	}

	score := SearchQualityScore{}

	// 1. Semantic relevance via embeddings
	semanticScore, err := s.calculateSemanticRelevance(ctx, query, results)
	if err != nil {
		return score, fmt.Errorf("semantic scoring failed: %w", err)
	}
	score.SemanticRelevance = semanticScore

	// 2. Reranker score
	if s.reranker != nil {
		rerankScore, err := s.calculateRerankerScore(ctx, query, results)
		if err == nil {
			score.RerankerScore = rerankScore
		}
	}

	// 3. Top-K accuracy (top 3 results)
	score.TopKAccuracy = s.calculateTopKAccuracy(results, 3)

	// 4. Result diversity
	diversity := s.calculateDiversity(results)
	score.ResultDiversity = diversity.DomainDiversity

	// 5. Authority score
	score.AuthorityScore = s.calculateAuthorityScore(results)

	// 6. Freshness score
	score.FreshnessScore = s.calculateFreshnessScore(results)

	// Calculate overall weighted score
	score.OverallScore = CalculateSearchScore(score, s.weights)

	return score, nil
}

// ScoreExtract performs quality scoring for extraction results
func (s *Scorer) ScoreExtract(content string, url string, expectedContent []string) ExtractQualityScore {
	score := ExtractQualityScore{}

	// 1. Content completeness (check for truncation indicators)
	score.ContentCompleteness = s.assessCompleteness(content)

	// 2. Structure preservation
	score.StructurePreservation = s.assessStructure(content)

	// 3. Markdown quality
	score.MarkdownQuality = s.assessMarkdownQuality(content)

	// 4. Freshness (extract and check dates)
	score.FreshnessScore = s.assessContentFreshness(content)

	// 5. Signal to noise ratio
	score.SignalToNoise = s.assessSignalToNoise(content)

	// 6. Code preservation
	score.CodePreservation = s.assessCodePreservation(content)

	// Calculate overall score
	score.OverallScore = CalculateExtractScore(score)

	return score
}

// ScoreCrawl performs quality scoring for crawl results
func (s *Scorer) ScoreCrawl(result *providers.CrawlResult, opts providers.CrawlOptions) CrawlQualityScore {
	score := CrawlQualityScore{}

	// 1. Coverage score
	if opts.MaxPages > 0 {
		score.CoverageScore = float64(result.TotalPages) / float64(opts.MaxPages) * 100
		if score.CoverageScore > 100 {
			score.CoverageScore = 100
		}
	} else {
		score.CoverageScore = 100
	}

	// 2. Depth accuracy (we can't directly verify, but check if we got expected pages)
	score.DepthAccuracy = score.CoverageScore

	// 3. Link discovery (heuristic based on page variety)
	score.LinkDiscovery = s.assessLinkDiscovery(result.Pages)

	// 4. Content consistency
	score.ContentConsistency = s.assessContentConsistency(result.Pages)

	// 5. Duplicate ratio
	score.DuplicateRatio = s.calculateDuplicateRatio(result.Pages)

	// Calculate overall score
	score.OverallScore = CalculateCrawlScore(score)

	return score
}

// calculateSemanticRelevance computes embedding-based relevance
func (s *Scorer) calculateSemanticRelevance(ctx context.Context, query string, results []providers.SearchItem) (float64, error) {
	if s.embedding == nil {
		return 50, nil // Neutral score if no embedding client
	}

	// Prepare texts to embed
	texts := make([]string, len(results)+1)
	texts[0] = query
	for i, r := range results {
		texts[i+1] = r.Title + " " + r.Content
	}

	// Get embeddings
	embeddings, err := s.embedding.Embed(ctx, texts)
	if err != nil {
		return 0, err
	}

	// Calculate average similarity between query and results
	queryEmbedding := embeddings[0]
	var totalSimilarity float64

	for i := 1; i < len(embeddings); i++ {
		sim := CosineSimilarity(queryEmbedding, embeddings[i])
		totalSimilarity += sim
	}

	avgSimilarity := totalSimilarity / float64(len(results))
	return SimilarityToScore(avgSimilarity), nil
}

// calculateRerankerScore uses the reranker API for relevance scoring
func (s *Scorer) calculateRerankerScore(ctx context.Context, query string, results []providers.SearchItem) (float64, error) {
	if s.reranker == nil {
		return 50, nil
	}

	documents := make([]string, len(results))
	for i, r := range results {
		documents[i] = r.Title + " " + truncate(r.Content, 500)
	}

	rerankResults, err := s.reranker.Rerank(ctx, query, documents)
	if err != nil {
		return 0, err
	}

	if len(rerankResults) == 0 {
		return 0, nil
	}

	// Calculate average normalized score
	var totalScore float64
	for _, r := range rerankResults {
		totalScore += NormalizeScore(r.Relevance)
	}

	return totalScore / float64(len(rerankResults)), nil
}

// calculateTopKAccuracy estimates relevance of top results
func (s *Scorer) calculateTopKAccuracy(results []providers.SearchItem, k int) float64 {
	if len(results) == 0 {
		return 0
	}

	if k > len(results) {
		k = len(results)
	}

	// Score based on content length and score field if available
	var totalScore float64
	for i := 0; i < k; i++ {
		r := results[i]
		score := 50.0 // Base score

		// Boost for non-empty content
		if len(r.Content) > 100 {
			score += 20
		}
		if len(r.Content) > 500 {
			score += 15
		}

		// Boost for provider relevance score
		if r.Score > 0 {
			score += r.Score * 15 // Scale provider score
		}

		// Slight penalty for very short titles
		if len(r.Title) < 10 {
			score -= 10
		}

		totalScore += clamp(score, 0, 100)
	}

	return totalScore / float64(k)
}

// calculateDiversity measures domain and content diversity
func (s *Scorer) calculateDiversity(results []providers.SearchItem) DiversityMetrics {
	metrics := DiversityMetrics{
		UniqueDomains: make([]string, 0),
	}

	domainCount := make(map[string]int)
	for _, r := range results {
		domain := extractDomain(r.URL)
		if domain != "" {
			domainCount[domain]++
		}
	}

	metrics.DomainCount = len(domainCount)
	for domain := range domainCount {
		metrics.UniqueDomains = append(metrics.UniqueDomains, domain)
	}

	// Calculate Shannon entropy for domain diversity
	if len(results) > 0 {
		var entropy float64
		for _, count := range domainCount {
			p := float64(count) / float64(len(results))
			if p > 0 {
				entropy -= p * math.Log(p)
			}
		}
		// Normalize to 0-100 (max entropy is log(n))
		maxEntropy := math.Log(float64(len(domainCount)))
		if maxEntropy > 0 {
			metrics.DomainDiversity = (entropy / maxEntropy) * 100
		} else {
			metrics.DomainDiversity = 100
		}
	}

	return metrics
}

// calculateAuthorityScore estimates source authority
func (s *Scorer) calculateAuthorityScore(results []providers.SearchItem) float64 {
	if len(results) == 0 {
		return 0
	}

	authorityDomains := map[string]int{
		"wikipedia.org":         100,
		"github.com":            95,
		"stackoverflow.com":     95,
		"docs.python.org":       95,
		"developer.mozilla.org": 95,
		"arxiv.org":             90,
		"ieee.org":              90,
		"acm.org":               90,
		"nature.com":            90,
		"science.org":           90,
		"reuters.com":           85,
		"bloomberg.com":         85,
		"techcrunch.com":        75,
		"medium.com":            70,
		"reddit.com":            60,
		"youtube.com":           60,
	}

	var totalScore float64
	for _, r := range results {
		domain := extractDomain(r.URL)
		if score, ok := authorityDomains[domain]; ok {
			totalScore += float64(score)
		} else {
			// Default score for unknown domains
			totalScore += 50
		}
	}

	return totalScore / float64(len(results))
}

// calculateFreshnessScore estimates content freshness
func (s *Scorer) calculateFreshnessScore(results []providers.SearchItem) float64 {
	if len(results) == 0 {
		return 50
	}

	var totalScore float64
	now := time.Now()

	for _, r := range results {
		if r.PublishedAt == nil {
			totalScore += 50 // Neutral for unknown date
			continue
		}

		age := now.Sub(*r.PublishedAt).Hours()
		var score float64
		switch {
		case age < 24:
			score = 100 // Less than 1 day
		case age < 168:
			score = 90 // Less than 1 week
		case age < 720:
			score = 80 // Less than 1 month
		case age < 8760:
			score = 70 // Less than 1 year
		default:
			score = 50 // Older
		}
		totalScore += score
	}

	return totalScore / float64(len(results))
}

// assessCompleteness checks for truncation indicators
func (s *Scorer) assessCompleteness(content string) float64 {
	if len(content) == 0 {
		return 0
	}

	score := 100.0

	// Check for truncation indicators
	truncationPatterns := []string{
		"...", "[…]", "(continued)", "read more", "click to read",
		"[View full article]", "[See more]",
	}

	contentLower := strings.ToLower(content)
	for _, pattern := range truncationPatterns {
		if strings.Contains(contentLower, pattern) {
			score -= 15
		}
	}

	// Penalize very short content
	if len(content) < 500 {
		score -= 20
	}
	if len(content) < 200 {
		score -= 30
	}

	return clamp(score, 0, 100)
}

// assessStructure checks markdown structure preservation
func (s *Scorer) assessStructure(content string) float64 {
	if len(content) == 0 {
		return 0
	}

	score := 100.0

	// Count headers
	headerCount := strings.Count(content, "#")
	if headerCount > 0 {
		score += 5 // Bonus for having headers
	}

	// Count list items
	listCount := strings.Count(content, "- ") + strings.Count(content, "* ")
	if listCount > 3 {
		score += 5 // Bonus for having lists
	}

	// Check for broken structure indicators
	brokenPatterns := []string{
		"##\n\n##", // Empty sections
		"#######",  // Too many header levels (likely parsing error)
	}

	for _, pattern := range brokenPatterns {
		if strings.Contains(content, pattern) {
			score -= 15
		}
	}

	return clamp(score, 0, 100)
}

// assessMarkdownQuality checks for valid markdown
func (s *Scorer) assessMarkdownQuality(content string) float64 {
	if len(content) == 0 {
		return 0
	}

	score := 100.0

	// Check for common markdown issues
	issues := 0

	// Unclosed code blocks
	codeBlockCount := strings.Count(content, "```")
	if codeBlockCount%2 != 0 {
		issues++
	}

	// Unclosed inline code
	inlineCodeCount := strings.Count(content, "`")
	if inlineCodeCount%2 != 0 {
		issues++
	}

	// Unmatched brackets that might be links
	openBracket := strings.Count(content, "[")
	closeBracket := strings.Count(content, "]")
	if openBracket != closeBracket {
		issues++
	}

	openParen := strings.Count(content, "(")
	closeParen := strings.Count(content, ")")
	if openParen != closeParen {
		issues++
	}

	score -= float64(issues) * 10

	return clamp(score, 0, 100)
}

// assessContentFreshness extracts and scores date freshness
func (s *Scorer) assessContentFreshness(content string) float64 {
	// This is a simplified version - in production, use date extraction
	// For now, return neutral score
	return 70
}

// assessSignalToNoise estimates content vs noise ratio
func (s *Scorer) assessSignalToNoise(content string) float64 {
	if len(content) == 0 {
		return 0
	}

	score := 100.0
	contentLower := strings.ToLower(content)

	// Check for navigation elements
	navIndicators := []string{
		"home", "about us", "contact", "privacy policy",
		"terms of service", "cookie policy", "sign up", "login",
	}

	navCount := 0
	for _, indicator := range navIndicators {
		if strings.Contains(contentLower, indicator) {
			navCount++
		}
	}

	score -= float64(navCount) * 3

	// Check for ad indicators
	adIndicators := []string{
		"advertisement", "sponsored", "promoted", "ad choices",
	}

	adCount := 0
	for _, indicator := range adIndicators {
		if strings.Contains(contentLower, indicator) {
			adCount++
		}
	}

	score -= float64(adCount) * 10

	return clamp(score, 20, 100)
}

// assessCodePreservation checks if code blocks are preserved
func (s *Scorer) assessCodePreservation(content string) float64 {
	if len(content) == 0 {
		return 0
	}

	// Count code blocks
	codeBlocks := strings.Count(content, "```")
	if codeBlocks >= 2 {
		return 100 // Has fenced code blocks
	}

	// Check for inline code
	inlineCode := strings.Count(content, "`")
	if inlineCode >= 2 {
		return 80 // Has some inline code
	}

	// Check for common code patterns
	codePatterns := []string{
		"func ", "def ", "class ", "import ", "#include",
		"console.log", "print(", "SELECT ", "INSERT ",
	}

	for _, pattern := range codePatterns {
		if strings.Contains(content, pattern) {
			return 60 // Has code-like content but not in blocks
		}
	}

	return 50 // No code detected
}

// assessLinkDiscovery scores the quality of discovered links
func (s *Scorer) assessLinkDiscovery(pages []providers.CrawledPage) float64 {
	if len(pages) <= 1 {
		return 100 // Single page crawl is fine
	}

	// Check URL diversity
	domainCount := make(map[string]int)
	for _, p := range pages {
		domain := extractDomain(p.URL)
		if domain != "" {
			domainCount[domain]++
		}
	}

	// Score based on internal link variety
	if len(domainCount) == 1 {
		return 100 // All from same domain (expected for single-site crawl)
	}

	return 90 // Some external links (might be okay)
}

// assessContentConsistency checks quality across pages
func (s *Scorer) assessContentConsistency(pages []providers.CrawledPage) float64 {
	if len(pages) == 0 {
		return 0
	}

	var lengths []int
	for _, p := range pages {
		lengths = append(lengths, len(p.Content))
	}

	if len(lengths) == 1 {
		return 100
	}

	// Calculate variance in content length
	avg := 0
	for _, l := range lengths {
		avg += l
	}
	avg /= len(lengths)

	var variance float64
	for _, l := range lengths {
		diff := float64(l - avg)
		variance += diff * diff
	}
	variance /= float64(len(lengths))
	stdDev := math.Sqrt(variance)

	// Coefficient of variation
	cv := stdDev / float64(avg)

	// Lower CV = more consistent = higher score
	if cv < 0.5 {
		return 100
	} else if cv < 1.0 {
		return 80
	} else if cv < 2.0 {
		return 60
	}
	return 40
}

// calculateDuplicateRatio calculates percentage of duplicate content
func (s *Scorer) calculateDuplicateRatio(pages []providers.CrawledPage) float64 {
	if len(pages) <= 1 {
		return 0
	}

	seen := make(map[string]bool)
	duplicates := 0

	for _, p := range pages {
		// Use first 200 chars as fingerprint
		fingerprint := truncate(p.Content, 200)
		if seen[fingerprint] {
			duplicates++
		} else {
			seen[fingerprint] = true
		}
	}

	return float64(duplicates) / float64(len(pages)) * 100
}

// Helper functions
func extractDomain(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

// timeNow returns current time
func timeNow() time.Time {
	return time.Now()
}
