// Package metrics provides cost calculation utilities for benchmark results.
package metrics

import "sync"

// CostCalculator computes USD costs for different providers based on their pricing models.
// All costs are in USD (US Dollars).
type CostCalculator struct {
	// Pricing rates (USD per unit)
	// Firecrawl: $0.005 per credit (pay-as-you-go rate)
	firecrawlPerCredit float64

	// Tavily: $0.008 per credit (pay-as-you-go rate)
	tavilyPerCredit float64

	// Brave: $0.004 per request (average of Base $0.003 and Pro $0.005)
	bravePerRequest float64

	// Exa: $0.005 per search (1-25 results tier)
	// Contents: $0.001 per page
	exaPerSearch  float64
	exaPerContent float64

	// Jina: $0.02 per million tokens = $0.00000002 per token
	jinaPerToken float64

	// Mixedbread: $0.004 per query (search without rerank)
	mixedbreadPerQuery float64

	mu sync.RWMutex
}

// NewCostCalculator creates a new cost calculator with default pricing.
// All prices are in USD and based on official pay-as-you-go rates as of 2025.
func NewCostCalculator() *CostCalculator {
	return &CostCalculator{
		// Firecrawl: Hobby plan pay-as-you-go rate
		// Source: https://www.firecrawl.dev/pricing
		firecrawlPerCredit: 0.005,

		// Tavily: Pay-as-you-go rate
		// Source: https://docs.tavily.com/documentation/api-credits
		tavilyPerCredit: 0.008,

		// Brave: Average of Base ($0.003) and Pro ($0.005) per 1k requests
		// Source: https://api-dashboard.search.brave.com/app/plans
		bravePerRequest: 0.004,

		// Exa: Fast/neural search 1-25 results tier
		// Contents retrieval rate
		// Source: https://exa.ai/pricing
		exaPerSearch:  0.005,
		exaPerContent: 0.001,

		// Jina: $0.02 per million tokens
		// Source: https://www.jinaai.cn/reader/
		jinaPerToken: 0.02 / 1000000,

		// Mixedbread: $4 per 1K queries = $0.004 per query
		// Source: https://www.mixedbread.com/pricing
		mixedbreadPerQuery: 0.004,
	}
}

// CostResult holds the calculated USD costs for a provider.
type CostResult struct {
	// TotalCost is the total USD cost for all operations
	TotalCost float64 `json:"total_cost_usd"`

	// SearchCost is the USD cost for search operations
	SearchCost float64 `json:"search_cost_usd"`

	// ExtractCost is the USD cost for extract operations
	ExtractCost float64 `json:"extract_cost_usd"`

	// CrawlCost is the USD cost for crawl operations
	CrawlCost float64 `json:"crawl_cost_usd"`

	// CostPerRequest is the average USD cost per request
	CostPerRequest float64 `json:"cost_per_request_usd"`

	// CostPerResult is the USD cost per successful result
	CostPerResult float64 `json:"cost_per_result_usd"`
}

// CalculateFirecrawlCost computes USD cost for Firecrawl based on credits used.
// Firecrawl charges per credit: scrape=1, crawl=1/page, map=1/page, search=2/10 results.
func (cc *CostCalculator) CalculateFirecrawlCost(creditsUsed int, _ string) float64 {
	cc.mu.RLock()
	defer cc.mu.RUnlock()
	return float64(creditsUsed) * cc.firecrawlPerCredit
}

// CalculateTavilyCost computes USD cost for Tavily based on credits used.
// Tavily charges: search=1-2 credits, extract=1 credit per 5 URLs, map=1 credit per 10 pages.
func (cc *CostCalculator) CalculateTavilyCost(creditsUsed int, _ string) float64 {
	cc.mu.RLock()
	defer cc.mu.RUnlock()
	return float64(creditsUsed) * cc.tavilyPerCredit
}

// CalculateBraveCost computes USD cost for Brave based on requests made.
// Brave charges per request: $3-5 per 1,000 requests depending on tier.
func (cc *CostCalculator) CalculateBraveCost(requestsMade int, _ string) float64 {
	cc.mu.RLock()
	defer cc.mu.RUnlock()
	return float64(requestsMade) * cc.bravePerRequest
}

// CalculateExaCost computes USD cost for Exa based on operations.
// Exa charges: search=$5 per 1K, contents=$1 per 1K pages.
func (cc *CostCalculator) CalculateExaCost(creditsUsed int, _ string, isContentFetch bool) float64 {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	if isContentFetch {
		return float64(creditsUsed) * cc.exaPerContent
	}
	return float64(creditsUsed) * cc.exaPerSearch
}

// CalculateJinaCost computes USD cost for Jina based on tokens used.
// Jina charges $0.02 per million tokens.
// Search: 10,000 tokens per request fixed.
// Extract: tokens based on content length.
func (cc *CostCalculator) CalculateJinaCost(tokensUsed int, _ string) float64 {
	cc.mu.RLock()
	defer cc.mu.RUnlock()
	return float64(tokensUsed) * cc.jinaPerToken
}

// CalculateMixedbreadCost computes USD cost for Mixedbread based on queries made.
// Mixedbread charges $4 per 1,000 search queries.
func (cc *CostCalculator) CalculateMixedbreadCost(queriesMade int, _ string) float64 {
	cc.mu.RLock()
	defer cc.mu.RUnlock()
	return float64(queriesMade) * cc.mixedbreadPerQuery
}

// CalculateLocalCost always returns 0 as local provider is free.
func (cc *CostCalculator) CalculateLocalCost(_ int, _ string) float64 {
	return 0
}

// CalculateProviderCost computes the USD cost for any provider based on its name.
// This is a convenience method that routes to the appropriate calculator.
func (cc *CostCalculator) CalculateProviderCost(provider string, creditsUsed int, testType string) float64 {
	switch provider {
	case "firecrawl":
		return cc.CalculateFirecrawlCost(creditsUsed, testType)
	case "tavily":
		return cc.CalculateTavilyCost(creditsUsed, testType)
	case "brave":
		// For Brave, creditsUsed represents request count
		return cc.CalculateBraveCost(creditsUsed, testType)
	case "exa":
		return cc.CalculateExaCost(creditsUsed, testType, false)
	case "jina":
		// For Jina, creditsUsed represents token count
		return cc.CalculateJinaCost(creditsUsed, testType)
	case "mixedbread":
		// For Mixedbread, creditsUsed represents query count
		return cc.CalculateMixedbreadCost(creditsUsed, testType)
	case "local":
		return cc.CalculateLocalCost(creditsUsed, testType)
	default:
		return 0
	}
}

// GetPricingInfo returns a map of provider pricing information for display.
func (cc *CostCalculator) GetPricingInfo() map[string]map[string]string {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	return map[string]map[string]string{
		"firecrawl": {
			"unit":        "credit",
			"rate":        "$0.005",
			"source":      "https://www.firecrawl.dev/pricing",
			"description": "Scrape/Crawl/Map: 1 credit/page, Search: 2 credits/10 results",
		},
		"tavily": {
			"unit":        "credit",
			"rate":        "$0.008",
			"source":      "https://docs.tavily.com/documentation/api-credits",
			"description": "Search: 1-2 credits, Extract: 1 credit/5 URLs, Map: 1 credit/10 pages",
		},
		"brave": {
			"unit":        "request",
			"rate":        "$0.004",
			"source":      "https://api-dashboard.search.brave.com/app/plans",
			"description": "$3-5 per 1,000 requests depending on tier",
		},
		"exa": {
			"unit":        "request",
			"rate":        "$0.005 (search), $0.001 (content)",
			"source":      "https://exa.ai/pricing",
			"description": "Search: $5/1K, Contents: $1/1K pages",
		},
		"jina": {
			"unit":        "token",
			"rate":        "$0.02 per million",
			"source":      "https://www.jinaai.cn/reader/",
			"description": "Search: 10,000 tokens fixed, Extract: tokens based on content",
		},
		"mixedbread": {
			"unit":        "query",
			"rate":        "$0.004",
			"source":      "https://www.mixedbread.com/pricing",
			"description": "$4 per 1,000 search queries",
		},
		"local": {
			"unit":        "N/A",
			"rate":        "$0",
			"source":      "N/A",
			"description": "Local crawling - no API costs",
		},
	}
}

// SetCustomRate allows overriding default rates (useful for enterprise pricing).
func (cc *CostCalculator) SetCustomRate(provider string, rate float64) {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	switch provider {
	case "firecrawl":
		cc.firecrawlPerCredit = rate
	case "tavily":
		cc.tavilyPerCredit = rate
	case "brave":
		cc.bravePerRequest = rate
	case "exa":
		cc.exaPerSearch = rate
	case "jina":
		cc.jinaPerToken = rate
	case "mixedbread":
		cc.mixedbreadPerQuery = rate
	}
}
