# SanityWebEval

A Go CLI for benchmarking web `search`, `extract`, and `crawl` capabilities across multiple providers with consistent tests, latency/cost metrics, and optional scoring diagnostics.

## Quick Start

### 1. Build

```bash
make build
```

### 2. Run without API keys (local crawler only)

```bash
./build/sanity-web-eval -providers local -no-search
```

### 3. Run a cloud provider

```bash
# Requires FIRECRAWL_API_KEY in .env or environment
./build/sanity-web-eval -providers firecrawl
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

`.env.example` includes all provider keys. Copy it to `.env` and fill in your keys:

```env
# Cloud provider keys
FIRECRAWL_API_KEY=your_key
TAVILY_API_KEY=your_key
BRAVE_API_KEY=your_key
EXA_API_KEY=your_key
JINA_API_KEY=your_key                # Only needed if using -jina flag
MXB_API_KEY=your_key                 # Preferred for Mixedbread
# MIXEDBREAD_API_KEY=your_key        # Also supported

# Optional Jina tuning (only relevant with -jina flag)
# ⚠️  Jina is excluded by default because its token-based billing makes it
#    significantly more expensive than other providers for equivalent operations.
#    Use -jina to opt in.
# Defaults:
# - search no-content mode enabled
# - search max results: 10
# - search retries: 0
# - extract/crawl retries: 0
# - extract token budget: 6000
# - crawl token budget: 6000
# - search timeout: 30s
# - generated image captions disabled
JINA_SEARCH_NO_CONTENT=true
JINA_SEARCH_MAX_RESULTS=10
JINA_SEARCH_TIMEOUT=30s
JINA_SEARCH_MAX_RETRIES=0
JINA_EXTRACT_MAX_RETRIES=0
JINA_EXTRACT_TOKEN_BUDGET=6000
JINA_CRAWL_TOKEN_BUDGET=6000
JINA_WITH_GENERATED_ALT=false

# Firecrawl rate limiting (requests per minute)
# Default: 5 (conservative for free tier's 6 req/min ceiling)
# Set higher for paid plans, or 0 to disable.
FIRECRAWL_RATE_LIMIT=5

# Optional Firecrawl retry tuning (defaults shown)
FIRECRAWL_MAX_RETRIES=5
FIRECRAWL_RETRY_INITIAL_BACKOFF=1s
FIRECRAWL_RETRY_MAX_BACKOFF=90s

# Optional scoring diagnostics
EMBEDDING_MODEL_BASE_URL=https://api.provider.com/v1
EMBEDDING_MODEL_API_KEY=your_key
EMBEDDING_MODEL=Qwen/Qwen3-Embedding-8B

RERANKER_MODEL_BASE_URL=https://api.provider.com/v1
RERANKER_MODEL_API_KEY=your_key
RERANKER_MODEL=Qwen/Qwen3-Reranker-8B
```

If `-quality` is enabled, all 4 required `EMBEDDING_*`/`RERANKER_*` base URL + key vars must be set.
Search scoring uses ground-truth metrics when provided (`expected_*` fields), optionally blended with model-assisted signals when `-quality` is enabled.
Extract/crawl scoring also uses ground-truth-aware checks when expectations are present.

#### How scoring works

**Search Relevance (Model-Assisted)** is a weighted composite of 5 sub-scores, all normalized to 0-100:

| Component | Weight | Source |
|---|---|---|
| Semantic Relevance | 35% | Cosine similarity between query embedding and each result's text (Qwen3-Embedding-8B, 4096-dim). Averaged across results. |
| Reranker Score | 25% | Qwen3-Reranker-8B scores each result against the query. Raw scores auto-normalized to 0-100. Averaged. |
| Authority Score | 20% | Domain lookup table (wikipedia=100, github=95, medium=70, unknown=50). Averaged. |
| Result Diversity | 10% | Shannon entropy of domain distribution. All-same-domain = 0, max spread = 100. |
| Freshness Score | 10% | Age-based buckets (<1 day=100, <1 week=90, ..., unknown=50). Averaged. |

If either AI model (semantic/reranker) is unavailable, its weight drops to 0 and remaining weights renormalize.
When ground-truth test config fields exist (`expected_topics`, `must_include_terms`, etc.), the final score blends 70% ground-truth + 30% model score; otherwise the model score is used directly.

**Extract and Crawl** scores are rule-based heuristics that check content completeness and structure (not model-assisted).

### Test Configuration (`config.toml`)

```toml
[general]
concurrency = 3
provider_concurrency = { firecrawl = 1 } # optional per-provider override(s)
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
- `max_depth = 0` behavior is provider-dependent: Firecrawl auto-calculates depth from the seed URL's path (e.g., `/3/tutorial/` → depth 2); other providers treat it as start page only (no link expansion).
- `max_pages` and `max_depth` are optional; provider defaults are used if omitted.
- `-no-search` removes all search tests at runtime.
- `provider_concurrency` is optional. If omitted, defaults are `1` per built-in provider (`firecrawl`, `tavily`, `brave`, `exa`, `mixedbread`, `local`, `jina`), with global `concurrency` still acting as the overall cap.

## Providers

| Provider | Search | Extract | Crawl | Env Var | Capability Notes |
|---|---:|---:|---:|---|---|
| Firecrawl | yes | yes | yes | `FIRECRAWL_API_KEY` (+ optional `FIRECRAWL_*` tuning vars) | Native for all ops |
| Tavily | yes | yes | yes | `TAVILY_API_KEY` | Search/extract native; crawl emulated (map+extract) |
| Brave | yes | yes | yes | `BRAVE_API_KEY` | Search native; extract/crawl emulated |
| Exa | yes | yes | yes | `EXA_API_KEY` | Search/extract native; crawl emulated |
| Mixedbread | yes | yes | yes | `MXB_API_KEY` or `MIXEDBREAD_API_KEY` | Search native; extract/crawl emulated |
| Local | no | yes | yes | none | Search unsupported; extract/crawl native local engine |
| **Jina** ⚠️ | yes | yes | yes | `JINA_API_KEY` | **Opt-in only** (`-jina` flag). Token-based billing is significantly more expensive than other providers. Search/extract native; crawl emulated |

Primary comparable rankings use normalized mode and native-capability operation results only.

## CLI Essentials

```bash
./build/sanity-web-eval [flags]
```

### Common commands

```bash
# All default providers (excludes Local and Jina)
./build/sanity-web-eval

# Specific providers
./build/sanity-web-eval -providers firecrawl,tavily

# Include Local provider (opt-in, no API key needed)
./build/sanity-web-eval -local
./build/sanity-web-eval -providers local    # Local only

# Include Jina (opt-in, high cost)
./build/sanity-web-eval -jina
./build/sanity-web-eval -providers jina    # Jina only

# Output only markdown + json
./build/sanity-web-eval -format md,json

# Select execution mode
./build/sanity-web-eval -mode normalized
./build/sanity-web-eval -mode native

# Repeat each test/provider run to reduce noise
./build/sanity-web-eval -repeats 5

# Normalized-mode policy for emulated operations
./build/sanity-web-eval -capability-policy strict
./build/sanity-web-eval -capability-policy tagged

# Quick mode (up to 3 tests, timeout forced to 30s, crawl max_depth normalized to 1)
./build/sanity-web-eval -quick

# Debug logs
./build/sanity-web-eval -debug
./build/sanity-web-eval -debug-full
```

### Flags

| Flag | Description | Default |
|---|---|---|
| `-config` | Config file path | `config.toml` |
| `-output` | Output base directory (overrides config) | config value |
| `-providers` | `all` or comma list of providers | `all` |
| `-format` | `all`, `html`, `md`, `json` | `all` |
| `-mode` | `normalized`, `native` | `normalized` |
| `-repeats` | repeated runs per test/provider | `3` |
| `-capability-policy` | normalized emulated-op handling: `strict`, `tagged` | `strict` |
| `-quality` | Enable scoring diagnostics (search model-assisted, extract/crawl heuristic) | `false` |
| `-quick` | Reduced test run (up to 3 tests, `30s` timeout, crawl `max_depth=1`) | `false` |
| `-debug` | Request/response debug logging | `false` |
| `-debug-full` | Full body capture + timing breakdown | `false` |
| `-no-progress` | Disable progress bar | `false` |
| `-no-search` | Exclude search tests | `false` |
| `-local` | Include local provider (excluded by default) | `false` |
| `-jina` | Include Jina provider (excluded by default due to high cost) | `false` |

### Validation behavior

- `-providers` accepts only: `all, firecrawl, tavily, local, brave, exa, mixedbread, jina`.
- `all` expands to all providers **except** Local and Jina (use `-local` / `-jina` to include them).
- Local and Jina can still be selected explicitly with `-providers local` or `-providers jina` without the opt-in flags.
- Provider list entries are normalized (trim + lowercase) and deduplicated.
- Empty entries or invalid names return an error.
- If filters result in zero providers, execution stops with an error.
- `-format` accepts only: `all, html, md, json`.
- `-format all` cannot be combined with other formats.
- `-mode` accepts only: `normalized, native`.
- `-capability-policy` accepts only: `strict, tagged`.
- In normalized+strict mode, emulated operations are skipped from execution.

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
- Primary comparable success metrics in summaries exclude non-native/emulated rows.

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

## Pricing and Free-Tier Notes (As of February 17, 2026)

The table below is an operational estimate from benchmark usage patterns, not a billing guarantee.
Pricing was verified against official docs on the date above; always re-check before large runs.

| Provider | Estimated Cost per Full Default Run (13 tests) | Free Tier (approx) | Unit & Rate |
|---|---:|---:|---|
| Firecrawl | ~$0.05-0.08 | 500 credits | $0.005/credit |
| Tavily | ~$0.01-0.02 | 1,000 credits/month | $0.008/credit |
| Brave | ~$0.03-0.07 | 2,000 queries/month | $0.005/req ($5/1K) |
| Exa | ~$0.03-0.15 | $10 credits | $0.005/search, $0.001/page |
| Mixedbread | ~$0.05-0.10 | 1,000 files | $0.0075/query ($7.50/1K with rerank) |
| Local | Free | Unlimited | N/A |
| **Jina** ⚠️ | ~$0.05-0.20 | 10M tokens | $0.02/M tokens (min 10K/search). Opt-in only (`-jina`); substantially more expensive than other providers for equivalent operations |

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
