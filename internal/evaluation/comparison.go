// Package evaluation provides cross-provider comparison and analysis
package evaluation

import (
	"fmt"
	"strings"
	"time"

	"github.com/lamim/search-api-bench/internal/metrics"
	"github.com/lamim/search-api-bench/internal/providers"
)

// Comparison provides cross-provider comparison capabilities
type Comparison struct {
	ProviderA string
	ProviderB string
}

// NewComparison creates a new comparison between two providers
func NewComparison(providerA, providerB string) *Comparison {
	return &Comparison{
		ProviderA: providerA,
		ProviderB: providerB,
	}
}

// ComparisonResult contains detailed comparison metrics
type ComparisonResult struct {
	TestName       string        `json:"test_name"`
	TestType       string        `json:"test_type"`
	ProviderA      string        `json:"provider_a"`
	ProviderB      string        `json:"provider_b"`
	ResultOverlap  float64       `json:"result_overlap"` // 0-100, Jaccard similarity
	UniqueToA      []string      `json:"unique_to_a"`
	UniqueToB      []string      `json:"unique_to_b"`
	SharedResults  []string      `json:"shared_results"`
	LatencyDiff    time.Duration `json:"latency_diff"` // A - B
	LatencyWinner  string        `json:"latency_winner"`
	QualityDiff    float64       `json:"quality_diff"` // A - B
	QualityWinner  string        `json:"quality_winner"`
	CostDiff       int           `json:"cost_diff"` // A - B
	CostWinner     string        `json:"cost_winner"`
	OverallWinner  string        `json:"overall_winner"`
	Recommendation string        `json:"recommendation"`
}

// CompareSearch compares search results between two providers
func (c *Comparison) CompareSearch(resultA, resultB metrics.Result, itemsA, itemsB []providers.SearchItem) ComparisonResult {
	comp := ComparisonResult{
		TestName:  resultA.TestName,
		TestType:  "search",
		ProviderA: c.ProviderA,
		ProviderB: c.ProviderB,
	}

	// Extract URLs for comparison
	urlsA := extractURLs(itemsA)
	urlsB := extractURLs(itemsB)

	// Calculate Jaccard similarity
	comp.ResultOverlap = calculateJaccardSimilarity(urlsA, urlsB)

	// Find unique and shared results
	comp.UniqueToA, comp.UniqueToB, comp.SharedResults = findDifferences(urlsA, urlsB)

	// Compare latency
	comp.LatencyDiff = resultA.Latency - resultB.Latency
	comp.LatencyWinner = determineLatencyWinner(resultA.Latency, resultB.Latency, c.ProviderA, c.ProviderB)

	// Compare cost
	comp.CostDiff = resultA.CreditsUsed - resultB.CreditsUsed
	comp.CostWinner = determineCostWinner(resultA.CreditsUsed, resultB.CreditsUsed, c.ProviderA, c.ProviderB)

	// Determine overall winner
	comp.OverallWinner = c.determineOverallWinner(comp)
	comp.Recommendation = c.generateRecommendation(comp)

	return comp
}

// CompareExtract compares extraction results between two providers
func (c *Comparison) CompareExtract(resultA, resultB metrics.Result, contentA, contentB string) ComparisonResult {
	comp := ComparisonResult{
		TestName:  resultA.TestName,
		TestType:  "extract",
		ProviderA: c.ProviderA,
		ProviderB: c.ProviderB,
	}

	// Content similarity (simplified)
	comp.ResultOverlap = calculateContentSimilarity(contentA, contentB)

	// Compare latency
	comp.LatencyDiff = resultA.Latency - resultB.Latency
	comp.LatencyWinner = determineLatencyWinner(resultA.Latency, resultB.Latency, c.ProviderA, c.ProviderB)

	// Compare cost
	comp.CostDiff = resultA.CreditsUsed - resultB.CreditsUsed
	comp.CostWinner = determineCostWinner(resultA.CreditsUsed, resultB.CreditsUsed, c.ProviderA, c.ProviderB)

	// Compare content length
	lenA := len(contentA)
	lenB := len(contentB)

	switch {
	case lenA > lenB:
		comp.QualityDiff = float64(lenA-lenB) / float64(lenB) * 100
		comp.QualityWinner = c.ProviderA
	case lenB > lenA:
		comp.QualityDiff = float64(lenB-lenA) / float64(lenA) * 100
		comp.QualityWinner = c.ProviderB
	default:
		comp.QualityWinner = "tie"
	}

	comp.OverallWinner = c.determineOverallWinner(comp)
	comp.Recommendation = c.generateRecommendation(comp)

	return comp
}

// CompareCrawl compares crawl results between two providers
func (c *Comparison) CompareCrawl(resultA, resultB metrics.Result, pagesA, pagesB []providers.CrawledPage) ComparisonResult {
	comp := ComparisonResult{
		TestName:  resultA.TestName,
		TestType:  "crawl",
		ProviderA: c.ProviderA,
		ProviderB: c.ProviderB,
	}

	// Extract URLs
	urlsA := extractCrawlURLs(pagesA)
	urlsB := extractCrawlURLs(pagesB)

	// Calculate overlap
	comp.ResultOverlap = calculateJaccardSimilarity(urlsA, urlsB)
	comp.UniqueToA, comp.UniqueToB, comp.SharedResults = findDifferences(urlsA, urlsB)

	// Compare latency
	comp.LatencyDiff = resultA.Latency - resultB.Latency
	comp.LatencyWinner = determineLatencyWinner(resultA.Latency, resultB.Latency, c.ProviderA, c.ProviderB)

	// Compare cost
	comp.CostDiff = resultA.CreditsUsed - resultB.CreditsUsed
	comp.CostWinner = determineCostWinner(resultA.CreditsUsed, resultB.CreditsUsed, c.ProviderA, c.ProviderB)

	// Compare page count
	countA := len(pagesA)
	countB := len(pagesB)

	switch {
	case countA > countB:
		comp.QualityDiff = float64(countA-countB) / float64(countB) * 100
		comp.QualityWinner = c.ProviderA
	case countB > countA:
		comp.QualityDiff = float64(countB-countA) / float64(countA) * 100
		comp.QualityWinner = c.ProviderB
	default:
		comp.QualityWinner = "tie"
	}

	comp.OverallWinner = c.determineOverallWinner(comp)
	comp.Recommendation = c.generateRecommendation(comp)

	return comp
}

// determineOverallWinner picks an overall winner based on multiple factors
func (c *Comparison) determineOverallWinner(comp ComparisonResult) string {
	scoreA := 0
	scoreB := 0

	// Latency (30%)
	switch comp.LatencyWinner {
	case c.ProviderA:
		scoreA += 30
	case c.ProviderB:
		scoreB += 30
	default:
		scoreA += 15
		scoreB += 15
	}

	// Cost (30%)
	switch comp.CostWinner {
	case c.ProviderA:
		scoreA += 30
	case c.ProviderB:
		scoreB += 30
	default:
		scoreA += 15
		scoreB += 15
	}

	// Quality/overlap (40%)
	if comp.TestType == "search" || comp.TestType == "crawl" {
		// For search/crawl: higher overlap is good, but also consider unique results
		// Provider with more unique results gets bonus
		switch {
		case len(comp.UniqueToA) > len(comp.UniqueToB):
			scoreA += 40
		case len(comp.UniqueToB) > len(comp.UniqueToA):
			scoreB += 40
		default:
			scoreA += 20
			scoreB += 20
		}
	} else {
		// For extract: higher quality/content wins
		switch comp.QualityWinner {
		case c.ProviderA:
			scoreA += 40
		case c.ProviderB:
			scoreB += 40
		default:
			scoreA += 20
			scoreB += 20
		}
	}

	switch {
	case scoreA > scoreB:
		return c.ProviderA
	case scoreB > scoreA:
		return c.ProviderB
	default:
		return "tie"
	}
}

// generateRecommendation creates a human-readable recommendation
func (c *Comparison) generateRecommendation(comp ComparisonResult) string {
	var parts []string

	if comp.LatencyWinner != "tie" {
		diff := comp.LatencyDiff
		if diff < 0 {
			diff = -diff
		}
		parts = append(parts, fmt.Sprintf("%s is %.0fms faster", comp.LatencyWinner, float64(diff)/float64(time.Millisecond)))
	}

	if comp.CostWinner != "tie" {
		diff := comp.CostDiff
		if diff < 0 {
			diff = -diff
		}
		parts = append(parts, fmt.Sprintf("%s uses %d fewer credits", comp.CostWinner, diff))
	}

	if comp.TestType == "search" {
		if len(comp.UniqueToA) > 0 || len(comp.UniqueToB) > 0 {
			if len(comp.UniqueToA) > len(comp.UniqueToB) {
				parts = append(parts, fmt.Sprintf("%s found %d unique results", c.ProviderA, len(comp.UniqueToA)))
			} else if len(comp.UniqueToB) > len(comp.UniqueToA) {
				parts = append(parts, fmt.Sprintf("%s found %d unique results", c.ProviderB, len(comp.UniqueToB)))
			}
		}
	}

	if len(parts) == 0 {
		return "Both providers performed similarly"
	}

	return strings.Join(parts, "; ")
}

// Helper functions

func extractURLs(items []providers.SearchItem) []string {
	urls := make([]string, len(items))
	for i, item := range items {
		urls[i] = item.URL
	}
	return urls
}

func extractCrawlURLs(pages []providers.CrawledPage) []string {
	urls := make([]string, len(pages))
	for i, page := range pages {
		urls[i] = page.URL
	}
	return urls
}

func calculateJaccardSimilarity(setA, setB []string) float64 {
	if len(setA) == 0 && len(setB) == 0 {
		return 100 // Both empty = perfect match
	}

	setAMap := make(map[string]bool)
	for _, a := range setA {
		setAMap[a] = true
	}

	setBMap := make(map[string]bool)
	for _, b := range setB {
		setBMap[b] = true
	}

	// Calculate intersection
	intersection := 0
	for a := range setAMap {
		if setBMap[a] {
			intersection++
		}
	}

	// Calculate union
	union := len(setAMap) + len(setBMap) - intersection

	if union == 0 {
		return 0
	}

	return float64(intersection) / float64(union) * 100
}

func findDifferences(setA, setB []string) (uniqueA, uniqueB, shared []string) {
	setAMap := make(map[string]bool)
	for _, a := range setA {
		setAMap[a] = true
	}

	setBMap := make(map[string]bool)
	for _, b := range setB {
		setBMap[b] = true
	}

	// Find unique to A
	for a := range setAMap {
		if !setBMap[a] {
			uniqueA = append(uniqueA, a)
		} else {
			shared = append(shared, a)
		}
	}

	// Find unique to B
	for b := range setBMap {
		if !setAMap[b] {
			uniqueB = append(uniqueB, b)
		}
	}

	return uniqueA, uniqueB, shared
}

func calculateContentSimilarity(contentA, contentB string) float64 {
	// Simple Jaccard similarity on words
	wordsA := strings.Fields(strings.ToLower(contentA))
	wordsB := strings.Fields(strings.ToLower(contentB))

	setA := make(map[string]bool)
	for _, w := range wordsA {
		setA[w] = true
	}

	setB := make(map[string]bool)
	for _, w := range wordsB {
		setB[w] = true
	}

	// Calculate intersection
	intersection := 0
	for w := range setA {
		if setB[w] {
			intersection++
		}
	}

	// Calculate union
	union := len(setA) + len(setB) - intersection

	if union == 0 {
		return 0
	}

	return float64(intersection) / float64(union) * 100
}

func determineLatencyWinner(latencyA, latencyB time.Duration, providerA, providerB string) string {
	switch {
	case latencyA < latencyB:
		return providerA
	case latencyB < latencyA:
		return providerB
	default:
		return "tie"
	}
}

func determineCostWinner(costA, costB int, providerA, providerB string) string {
	switch {
	case costA < costB:
		return providerA
	case costB < costA:
		return providerB
	default:
		return "tie"
	}
}
