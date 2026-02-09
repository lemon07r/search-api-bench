# Search API Benchmark

A Go CLI for benchmarking web `search`, `extract`, and `crawl` capabilities across multiple providers with consistent tests, latency/cost metrics, and optional scoring diagnostics.

## Quick Start

### 1. Build

```bash
make build
```

### 2. Run without API keys (local crawler only)

```bash
./build/search-api-bench -providers local -no-search
```

### 3. Run a cloud provider

```bash
# Requires FIRECRAWL_API_KEY in .env or environment
./build/search-api-bench -providers firecrawl
```

### 4. Find outputs

Each run writes to a timestamped folder:

```text
results/YYYY-MM-DD_HH-MM-SS/
```

With reports such as `report.html`, `report.md`, and `report.json`.

## Setup

### Prerequisites

- Go `1.25+`
- Optional API keys depending on providers used

### Environment Variables

The CLI auto-loads `.env` from project root if present.

`.env.example` currently includes starter keys for Firecrawl/Tavily only. Add other keys manually as needed:

```env
# Cloud provider keys
FIRECRAWL_API_KEY=your_key
TAVILY_API_KEY=your_key
BRAVE_API_KEY=your_key
EXA_API_KEY=your_key
JINA_API_KEY=your_key                # Optional: Jina works without key (lower limits)
MXB_API_KEY=your_key                 # Preferred for Mixedbread
# MIXEDBREAD_API_KEY=your_key        # Also supported

# Optional scoring diagnostics
EMBEDDING_MODEL_BASE_URL=https://api.provider.com/v1
EMBEDDING_MODEL_API_KEY=your_key
EMBEDDING_MODEL=Qwen/Qwen3-Embedding-8B

RERANKER_MODEL_BASE_URL=https://api.provider.com/v1
RERANKER_MODEL_API_KEY=your_key
RERANKER_MODEL=Qwen/Qwen3-Reranker-8B
```

If `-quality` is enabled, all 4 required `EMBEDDING_*`/`RERANKER_*` base URL + key vars must be set.
Search scoring is model-assisted (embedding + reranker). Extract/crawl scoring is heuristic.

### Test Configuration (`config.toml`)

```toml
[general]
concurrency = 3
timeout = "45s"
output_dir = "./results"

[[tests]]
name = "Search - Example"
type = "search"
query = "Rust ownership and borrowing"
expected_topics = ["Rust", "ownership", "borrowing"]

[[tests]]
name = "Extract - Example"
type = "extract"
url = "https://docs.python.org/3/tutorial/"
expected_content = ["Python", "tutorial"]

[[tests]]
name = "Crawl - Example"
type = "crawl"
url = "https://example.com"
max_pages = 10
max_depth = 2
```

Notes:
- `max_depth = 0` means start page only (no link expansion).
- `max_pages` and `max_depth` are optional; provider defaults are used if omitted.
- `-no-search` removes all search tests at runtime.

## Providers

| Provider | Search | Extract | Crawl | Env Var | Notes |
|---|---:|---:|---:|---|---|
| Firecrawl | yes | yes | yes | `FIRECRAWL_API_KEY` | Native API for all 3 ops |
| Tavily | yes | yes | yes | `TAVILY_API_KEY` | Native API for all 3 ops |
| Brave | yes | yes | yes | `BRAVE_API_KEY` | Extract/Crawl use direct fetch strategy |
| Exa | yes | yes | yes | `EXA_API_KEY` | Native search + fetch-based flows |
| Jina | yes | yes | yes | `JINA_API_KEY` (optional) | Works without key, usually lower limits |
| Mixedbread | yes | yes | yes | `MXB_API_KEY` or `MIXEDBREAD_API_KEY` | Supports both key names |
| Local | no | yes | yes | none | `search` unsupported by design |

## CLI Essentials

```bash
./build/search-api-bench [flags]
```

### Common commands

```bash
# All providers selected by default
./build/search-api-bench

# Specific providers
./build/search-api-bench -providers firecrawl,tavily

# Exclude local provider even when using all
./build/search-api-bench -providers all -no-local

# Output only markdown + json
./build/search-api-bench -format md,json

# Quick mode (up to 3 tests, timeout forced to 30s, crawl max_depth normalized to 1)
./build/search-api-bench -quick

# Debug logs
./build/search-api-bench -debug
./build/search-api-bench -debug-full
```

### Flags

| Flag | Description | Default |
|---|---|---|
| `-config` | Config file path | `config.toml` |
| `-output` | Output base directory (overrides config) | config value |
| `-providers` | `all` or comma list of providers | `all` |
| `-format` | `all`, `html`, `md`, `json` | `all` |
| `-quality` | Enable scoring diagnostics (search model-assisted, extract/crawl heuristic) | `false` |
| `-quick` | Reduced test run (up to 3 tests, `30s` timeout, crawl `max_depth=1`) | `false` |
| `-debug` | Request/response debug logging | `false` |
| `-debug-full` | Full body capture + timing breakdown | `false` |
| `-no-progress` | Disable progress bar | `false` |
| `-no-search` | Exclude search tests | `false` |
| `-no-local` | Exclude local provider | `false` |

### Validation behavior

- `-providers` accepts only: `all, firecrawl, tavily, local, brave, exa, mixedbread, jina`.
- Provider list entries are normalized (trim + lowercase) and deduplicated.
- Empty entries or invalid names return an error.
- If filters result in zero providers (for example `-providers local -no-local`), execution stops with an error.
- `-format` accepts only: `all, html, md, json`.
- `-format all` cannot be combined with other formats.

## Reports and Metrics Semantics

Output directory pattern:

```text
results/YYYY-MM-DD_HH-MM-SS/
```

Generated files:

- `report.html`: interactive charts
- `report.md`: markdown summary + details
- `report.json`: raw export
- `debug/`: per-provider debug logs (only with debug flags)

Metrics semantics:

- Success rate and averages are computed from executed (non-skipped) tests.
- Skipped tests are counted and reported separately.
- Cost summaries prefer measured per-result `CostUSD` when available.

## Troubleshooting

- `Error parsing providers ...`: invalid provider token or empty list entry.
- `Error parsing formats ...`: invalid format or `all` combined with others.
- `no providers initialized`: selected cloud providers are missing API keys.
- `-quality flag set but failed to initialize`: required embedding/reranker env vars are missing.
- `no tests match the specified filters`: your config plus `-no-search` left zero runnable tests.
- Local provider and `search` tests: this is expected; local supports only extract/crawl.

## Development

```bash
make test            # Unit tests
make ci              # fmt -> vet -> lint -> test -> build
make test-coverage   # coverage.out + coverage.html
make build-all       # Cross-platform binaries
make release         # Build archives + checksums
```

## Architecture (High-Level)

```text
cmd/bench/main.go          CLI, flags, env loading, provider init
internal/config            TOML loading + validation
internal/providers         Provider implementations + retry/debug helpers
internal/evaluator         Concurrent execution runner
internal/metrics           Thread-safe result aggregation
internal/report            HTML/Markdown/JSON reports
internal/debug             Structured debug logs
internal/quality           Optional scoring diagnostics
```

## Advanced Libraries

These internal packages can be reused in custom tools:

- `internal/quality`: search relevance + heuristic scoring utilities
- `internal/domains`: code/news/academic validators
- `internal/evaluation`: cross-provider comparisons + golden baselines
- `internal/robustness`: edge-case generation + stress testing

See package APIs in source for usage examples.

## Pricing and Free-Tier Notes (As of February 9, 2026)

The table below is an operational estimate from benchmark usage patterns, not a billing guarantee.

| Provider | Estimated Cost per Full Default Run (13 tests) | Free Tier (approx) |
|---|---:|---:|
| Firecrawl | ~$0.05-0.08 | 500 credits |
| Tavily | ~$0.01-0.02 | 1,000 credits/month |
| Brave | ~$0.03 | 2,000 queries/month |
| Exa | ~$0.03-0.15 | $10 credits |
| Jina | ~$0.001-0.01 | 10M tokens |
| Mixedbread | ~$0.01-0.03 | 1,000 files |
| Local | Free | Unlimited |

Always verify current pricing and quotas before large runs:

- Firecrawl: `https://www.firecrawl.dev/pricing`
- Tavily: `https://tavily.com/pricing`
- Brave Search API: `https://brave.com/search/api/`
- Exa: `https://exa.ai/pricing`
- Jina: `https://jina.ai/reader/`
- Mixedbread: `https://www.mixedbread.com/pricing`

## CI/CD

GitHub Actions release workflow (`.github/workflows/release.yml`) triggers on tags matching `v*.*.*` and builds:

- Linux: amd64, arm64
- macOS: amd64, arm64
- Windows: amd64

Artifacts are archived and published with checksums.
