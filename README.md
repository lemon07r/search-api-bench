# Search API Benchmark Tool

A Go CLI tool for benchmarking web search, content extraction, and crawling APIs. Compares **Firecrawl**, **Tavily**, and a built-in **Local** crawler across performance, cost, and quality dimensions.

## Features

- **Three Test Types**: Search queries, single-page extraction, multi-page crawling
- **Three Providers**: Firecrawl, Tavily, and Local (Colly-based, no API key required)
- **Performance Metrics**: Latency (avg, p50, p95, p99), success rates
- **Cost Tracking**: Credit usage per provider and operation
- **Rich Reports**: HTML with Chart.js, Markdown tables, JSON export
- **Real-time Progress**: Terminal UI with progress bar
- **Debug Logging**: Request/response capture for troubleshooting
- **Extensible Libraries**: Quality scoring, domain validators, cross-provider comparison, edge cases, stress testing, and golden dataset (available for custom integrations)

## Quick Start

```bash
# Build and run with default config
make build
./build/search-api-bench

# Run with specific provider
./search-api-bench -providers firecrawl
./search-api-bench -providers tavily
./search-api-bench -providers local  # No API key required

# Quick test mode (3 tests, 20s timeout)
./search-api-bench -quick

# Debug mode with full request logging
./search-api-bench -debug

# Disable progress bar for CI
./search-api-bench -no-progress
```

## Setup

### 1. Configure API Keys

Create `.env` in the project root:

```env
# Required for Firecrawl and Tavily providers
FIRECRAWL_API_KEY=your_key_here
TAVILY_API_KEY=your_key_here

# Optional: Enable AI quality scoring (Nebius Embeddings + Novita Reranker)
EMBEDDING_MODEL_BASE_URL=https://api.tokenfactory.nebius.com/v1
EMBEDDING_EMBEDDING_MODEL_API_KEY=your_key
RERANKER_MODEL_BASE_URL=https://api.novita.ai/openai/v1
RERANKER_MODEL_API_KEY=your_key
```

### 2. Configure Tests

Edit `config.toml` to define test scenarios:

```toml
[general]
concurrency = 3          # Max concurrent requests
timeout = "45s"          # Per-test timeout
output_dir = "./results" # Report output directory

[[tests]]
name = "Search - AI Research"
type = "search"
query = "latest transformer architecture advances"
expected_topics = ["transformer", "attention", "LLM"]

[[tests]]
name = "Extract - Documentation"
type = "extract"
url = "https://docs.python.org/3/tutorial/"
expected_content = ["Python", "tutorial", "programming"]

[[tests]]
name = "Crawl - Documentation Site"
type = "crawl"
url = "https://example.com"
max_pages = 10
max_depth = 2
```

## CLI Reference

| Flag | Description | Default |
|------|-------------|---------|
| `-config` | Path to TOML config file | `config.toml` |
| `-output` | Output directory override | (from config) |
| `-providers` | Comma-separated or `all` | `all` |
| `-format` | Report format: `html`, `md`, `json`, `all` | `all` |
| `-quick` | Run reduced test set (1 of each type) | `false` |
| `-debug` | Enable request/response logging | `false` |
| `-no-progress` | Disable progress bar (useful for CI) | `false` |
| `-verbose` | Show full error details | `false` |

## Reports

After execution, reports are saved to `results/YYYY-MM-DD_HH-MM-SS/`:

| File | Description |
|------|-------------|
| `report.html` | Interactive charts with Chart.js (latency, credits, success rate) |
| `report.md` | Markdown summary with comparison tables |
| `report.json` | Raw data export with all metrics |
| `debug.json` | Full request/response logs (with `-debug` flag) |

## Extensibility Libraries

The codebase includes several library packages for advanced use cases. These are available for import and custom integration:

### Quality Scoring (`internal/quality`)

AI-powered quality evaluation using embeddings and rerankers:

```go
scorer := quality.NewScorer(embeddingClient, rerankerClient)

// Score search results
score, _ := scorer.ScoreSearch(ctx, query, searchResults)
// Metrics: SemanticRelevance, RerankerScore, TopKAccuracy, ResultDiversity, AuthorityScore, FreshnessScore

// Score extraction
score := scorer.ScoreExtract(content, url, expectedContent)
// Metrics: ContentCompleteness, StructurePreservation, MarkdownQuality, SignalToNoise, CodePreservation

// Score crawl
score := scorer.ScoreCrawl(crawlResult, opts)
// Metrics: CoverageScore, DepthAccuracy, LinkDiscovery, ContentConsistency, DuplicateRatio
```

### Domain Validators (`internal/domains`)

Specialized validators for content types:

```go
// Code validation
validator := domains.NewCodeValidator([]string{"go", "python"})
result := validator.ValidateExtract(content)
// Returns: CodeBlocksFound, LanguagesDetected, SyntaxHighlighted, FunctionSignatures, etc.

// News validation
validator := domains.NewNewsValidator(48) // max age in hours
result := validator.ValidateExtract(content, sourceURL)
// Returns: FreshnessScore, HasHeadline, HasAuthor, ArticleStructure, etc.

// Academic validation
validator := domains.NewAcademicValidator("apa")
result := validator.ValidateExtract(content)
// Returns: CitationCount, HasAbstract, HasReferences, CitationFormatScore, etc.
```

### Cross-Provider Comparison (`internal/evaluation`)

Analyze results between providers:

```go
comparison := evaluation.NewComparison("firecrawl", "tavily")
result := comparison.CompareSearch(resultA, resultB, itemsA, itemsB)
// Returns: ResultOverlap (Jaccard similarity), UniqueToA, UniqueToB, LatencyWinner, CostWinner, OverallWinner
```

### Edge Cases & Stress Testing (`internal/robustness`)

```go
gen := robustness.NewEdgeCaseGenerator()
searchCases := gen.GenerateSearchEdgeCases()   // 25+ cases
extractCases := gen.GenerateExtractEdgeCases() // 15+ cases
crawlCases := gen.GenerateCrawlEdgeCases()     // 10+ cases

// Stress testing
runner := robustness.NewStressTestRunner(10, 30*time.Second)
result, _ := runner.Run(ctx, requestFunc)         // Sustained load
result, _ = runner.RunBurst(ctx, requestFunc, 100) // Burst test
result, _ = runner.RunSequential(ctx, requestFunc, 50) // Sequential rapid
```

### Golden Dataset (`internal/evaluation`)

Regression testing support:

```go
manager := evaluation.NewGoldenManager("./golden/dataset.json", "./golden/baseline.json")
manager.AddTest(evaluation.GoldenTest{...})
manager.UpdateBaseline("firecrawl", testScores, overallScore)
regressions := manager.DetectRegressions("firecrawl", currentScores, 0.10)
```

## API Pricing

| Provider | Free Tier | Paid Tier | Search | Extract | Crawl |
|----------|-----------|-----------|--------|---------|-------|
| **Firecrawl** | 500 credits (one-time) | $16/mo (3K), $83/mo (100K) | 1 credit | 1 credit | 1 credit/page |
| **Tavily** | 1,000 credits/mo | $30/mo (4K) | 1 credit | 1 credit/5 extractions | N/A |
| **Local** | Unlimited | Free | N/A | N/A | N/A |

## Error Categories

Errors are automatically categorized for better reporting:

| Category | Description |
|----------|-------------|
| `timeout` | Request timeout or context deadline exceeded |
| `auth` | Authentication error (401/403) or missing API key |
| `rate_limit` | Rate limit hit (429) |
| `server_error` | 5xx server errors |
| `client_error` | 4xx client errors (404, etc.) |
| `network` | Connection refused, DNS failures |
| `parse` | JSON parsing or unmarshaling errors |
| `crawl_failed` | Crawl-specific failures |

## Development

```bash
# Full CI pipeline: fmt → vet → lint → test → build
make ci

# Individual commands
make test                # Run unit tests
make test-coverage       # Generate coverage.html
make lint                # Run golangci-lint
make build-all           # Cross-compile for all platforms
make release             # Create release archives with checksums
make run-firecrawl       # Run with Firecrawl only
make run-tavily          # Run with Tavily only
make run-local           # Run with Local crawler only
```

## Architecture

```
cmd/bench/main.go              # CLI entry, flags, provider init
├── internal/
│   ├── config/config.go       # TOML config loading & validation
│   ├── providers/
│   │   ├── interface.go       # Provider interface + result types
│   │   ├── firecrawl/client.go
│   │   ├── tavily/client.go
│   │   └── local/client.go    # Colly-based local crawler
│   ├── evaluator/runner.go    # Concurrent test execution
│   ├── metrics/collector.go   # Thread-safe metrics aggregation
│   ├── quality/               # AI scoring library (embeddings/rerankers)
│   ├── domains/               # Domain validators (code/news/academic)
│   ├── evaluation/            # Cross-provider analysis & golden dataset
│   ├── robustness/            # Edge cases & stress testing
│   ├── progress/manager.go    # Terminal UI & progress bar
│   ├── debug/logger.go        # Request/response logging
│   └── report/                # HTML/Markdown/JSON generators
```

### Provider Interface

```go
type Provider interface {
    Name() string
    Search(ctx context.Context, query string, opts SearchOptions) (*SearchResult, error)
    Extract(ctx context.Context, url string, opts ExtractOptions) (*ExtractResult, error)
    Crawl(ctx context.Context, url string, opts CrawlOptions) (*CrawlResult, error)
}
```

Register new providers in `cmd/bench/main.go`.

### Design Principles

- **No SDKs**: Pure HTTP clients using Go standard library only
- **Single external dependency**: `github.com/BurntSushi/toml`
- **Concurrent execution**: Semaphore-based rate limiting
- **Thread-safe**: Mutex-protected metrics collection
- **Provider pattern**: Easy to extend with new APIs

## CI/CD

GitHub Actions workflow (`.github/workflows/release.yml`) triggers on `v*.*.*` tags:

- Builds for Linux (amd64/arm64), macOS (amd64/arm64), Windows (amd64)
- Creates release archives (.tar.gz for Unix, .zip for Windows)
- Generates SHA256 checksums
- Publishes GitHub release with auto-generated notes

Local release test: `make release`

## File Locations

| Purpose | Path |
|---------|------|
| Binary output | `./build/search-api-bench` |
| Config | `./config.toml` |
| Environment | `./.env` (gitignored) |
| Reports | `./results/YYYY-MM-DD_HH-MM-SS/` |
| Debug logs | `./results/YYYY-MM-DD_HH-MM-SS/debug.json` |
| Lint config | `./.golangci.yml` |
| Golden dataset | `./golden/dataset.json` |
| Baseline scores | `./golden/baseline.json` |

## License

MIT
