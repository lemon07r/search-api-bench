# Search API Benchmark

A Go CLI tool for benchmarking web search, content extraction, and crawling APIs. Compares **7 providers** across performance, cost, and quality.

## Supported Providers

| Provider | Search | Extract | Crawl | Free Tier |
|----------|--------|---------|-------|-----------|
| **Firecrawl** | + | + | + | 500 credits |
| **Tavily** | + | + | + | 1,000 credits/mo |
| **Brave** | + | + | + | 2,000 queries/mo |
| **Exa** | + | + | + | $10 credits |
| **Jina** | + | + | + | 10M tokens |
| **Mixedbread** | + | + | + | 1,000 files |
| **Local** | - | + | + | Unlimited |

## Quick Start

```bash
# Build and run with all providers
make build
./build/search-api-bench

# Run specific provider
./build/search-api-bench -providers firecrawl
./build/search-api-bench -providers local  # No API key needed

# Quick test mode (3 tests, 20s timeout)
./build/search-api-bench -quick

# Debug mode with request logging
./build/search-api-bench -debug
```

## Setup

### 1. Configure API Keys

Create `.env` in the project root:

```env
# Required for cloud providers
FIRECRAWL_API_KEY=your_key
TAVILY_API_KEY=your_key
BRAVE_API_KEY=your_key
EXA_API_KEY=your_key
JINA_API_KEY=your_key
MIXEDBREAD_API_KEY=your_key

# Optional: AI quality scoring
EMBEDDING_MODEL_BASE_URL=https://api.tokenfactory.nebius.com/v1
EMBEDDING_EMBEDDING_MODEL_API_KEY=your_key
RERANKER_MODEL_BASE_URL=https://api.novita.ai/openai/v1
RERANKER_MODEL_API_KEY=your_key
```

### 2. Configure Tests

Edit `config.toml`:

```toml
[general]
concurrency = 3
timeout = "45s"

[[tests]]
name = "Search - AI Research"
type = "search"
query = "latest transformer architecture"
expected_topics = ["transformer", "attention", "LLM"]

[[tests]]
name = "Extract - Docs"
type = "extract"
url = "https://docs.python.org/3/tutorial/"
expected_content = ["Python", "tutorial"]

[[tests]]
name = "Crawl - Site"
type = "crawl"
url = "https://example.com"
max_pages = 10
max_depth = 2
```

## CLI Reference

| Flag | Description | Default |
|------|-------------|---------|
| `-config` | Config file path | `config.toml` |
| `-output` | Output directory | (from config) |
| `-providers` | Comma-separated or `all` | `all` |
| `-format` | `html`, `md`, `json`, `all` | `all` |
| `-quick` | Reduced test set | `false` |
| `-debug` | Request/response logging | `false` |
| `-no-progress` | Disable progress bar | `false` |

## Reports

Results saved to `results/YYYY-MM-DD_HH-MM-SS/`:

| File | Description |
|------|-------------|
| `report.html` | Interactive charts (Chart.js) |
| `report.md` | Markdown summary tables |
| `report.json` | Raw data export |
| `debug.json` | Request/response logs (with `-debug`) |

## Architecture

```
cmd/bench/main.go              # CLI entry, provider init
├── internal/
│   ├── config/                # TOML loading
│   ├── providers/             # Provider implementations
│   │   ├── interface.go       # Provider interface
│   │   ├── firecrawl/
│   │   ├── tavily/
│   │   ├── local/             # Colly-based, no API key
│   │   ├── brave/
│   │   ├── exa/
│   │   ├── jina/
│   │   └── mixedbread/
│   ├── evaluator/             # Concurrent test execution
│   ├── metrics/               # Thread-safe aggregation
│   ├── report/                # HTML/Markdown/JSON generators
│   ├── progress/              # Terminal UI
│   ├── debug/                 # Request/response logging
│   ├── quality/               # AI scoring (optional)
│   ├── domains/               # Content validators (optional)
│   ├── evaluation/            # Cross-provider analysis (optional)
│   └── robustness/            # Edge cases & stress tests (optional)
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

## Development

```bash
make ci              # Full pipeline: fmt → vet → lint → test → build
make test            # Unit tests
make test-coverage   # Generate coverage.html
make build-all       # Cross-compile for all platforms
make release         # Create release archives
```

### Design Principles

- **No SDKs**: Pure HTTP clients using Go standard library
- **Minimal dependencies**: Only `github.com/BurntSushi/toml`
- **Concurrent**: Semaphore-based rate limiting
- **Thread-safe**: Mutex-protected metrics
- **Extensible**: Provider interface for easy additions

## API Pricing (February 2026)

Running the **full benchmark suite** (13 tests):

| Provider | Cost per Run | Free Tier |
|----------|--------------|-----------|
| Firecrawl | ~$0.05-0.08 | 500 credits |
| Tavily | ~$0.01-0.02 | 1,000/mo |
| Brave | ~$0.03 | 2,000/mo |
| Exa | ~$0.03-0.15 | $10 credits |
| Jina | ~$0.001-0.01 | 10M tokens |
| Mixedbread | ~$0.01-0.03 | 1,000 files |
| Local | Free | Unlimited |

## Libraries for Custom Integration

The codebase includes optional libraries for advanced use cases:

### Quality Scoring (`internal/quality`)

AI-powered evaluation using Qwen3-Embedding-8B with MRL and instruction support:

```go
scorer := quality.NewScorer(embeddingClient, rerankerClient)
score, _ := scorer.ScoreSearch(ctx, query, results)
score := scorer.ScoreExtract(content, url, expectedContent)
score := scorer.ScoreCrawl(crawlResult, opts)
```

### Domain Validators (`internal/domains`)

```go
validator := domains.NewCodeValidator([]string{"go", "python"})
validator := domains.NewNewsValidator(48) // max age in hours
validator := domains.NewAcademicValidator("apa")
```

### Cross-Provider Comparison (`internal/evaluation`)

```go
comparison := evaluation.NewComparison("firecrawl", "tavily")
result := comparison.CompareSearch(resultA, resultB, itemsA, itemsB)
```

### Stress Testing (`internal/robustness`)

```go
gen := robustness.NewEdgeCaseGenerator()
cases := gen.GenerateSearchEdgeCases() // 25+ cases

runner := robustness.NewStressTestRunner(10, 30*time.Second)
result, _ := runner.RunBurst(ctx, requestFunc, 100)
```

## CI/CD

GitHub Actions triggers on `v*.*.*` tags:
- Builds for Linux (amd64/arm64), macOS (amd64/arm64), Windows (amd64)
- Creates release archives with SHA256 checksums
- Publishes GitHub release

Local test: `make release`

## File Locations

| Purpose | Path |
|---------|------|
| Binary | `./build/search-api-bench` |
| Config | `./config.toml` |
| Environment | `./.env` (gitignored) |
| Reports | `./results/YYYY-MM-DD_HH-MM-SS/` |
| Lint config | `./.golangci.yml` |
| Golden dataset | `./golden/dataset.json` |

## License

MIT
