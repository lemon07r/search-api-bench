# Search API Benchmark Tool

A Go-based CLI tool for comparing the performance, quality, and cost-effectiveness of web search and crawling APIs. Currently supports **Firecrawl** and **Tavily**.

## Features

- **Multiple Test Types**: Search, content extraction, and website crawling
- **Performance Metrics**: Latency, throughput, success rate
- **Cost Analysis**: Credit usage tracking and efficiency comparison
- **Quality Metrics**: Content completeness, relevance scoring
- **Visual Reports**: HTML reports with charts, Markdown summaries, and JSON data export
- **Configurable**: TOML-based configuration for custom test scenarios
- **Fast Execution**: Concurrent test execution with configurable parallelism

## Installation

### From Source

```bash
git clone <repository>
cd search-api-bench
make build
```

### Pre-built Binaries

Download the latest release for your platform from the [Releases](https://github.com/lamim/search-api-bench/releases) page.

## Configuration

Create a `.env` file with your API keys:

```env
FIRECRAWL_API_KEY=your_firecrawl_key_here
TAVILY_API_KEY=your_tavily_key_here
```

Edit `config.toml` to define your test scenarios:

```toml
[general]
concurrency = 3
timeout = "45s"
output_dir = "./results"

[[tests]]
name = "Search - AI Research"
type = "search"
query = "latest advances in transformer architecture 2024"
expected_topics = ["transformer", "attention", "LLM"]

[[tests]]
name = "Extract - Documentation"
type = "extract"
url = "https://docs.python.org/3/tutorial/"
expected_content = ["Python", "tutorial", "programming"]
```

## Usage

### Run Full Benchmark

```bash
./search-api-bench
```

### Test Specific Provider

```bash
./search-api-bench -providers firecrawl
./search-api-bench -providers tavily
```

### Custom Configuration

```bash
./search-api-bench -config my-config.toml -output ./my-results
```

### Generate Specific Report Format

```bash
./search-api-bench -format html   # HTML only
./search-api-bench -format md     # Markdown only
./search-api-bench -format json   # JSON only
```

## Reports

After running, check the `results/` directory for:

- `report.html` - Interactive HTML report with charts
- `report.md` - Markdown summary
- `report.json` - Raw data export

## Development

```bash
# Run all tests
make test

# Run with coverage
make test-coverage

# Format code
make fmt

# Run linter
make lint

# Build for all platforms
make build-all

# Create release
make release
```

## GitHub Actions

The repository includes a workflow that automatically builds and releases multi-platform binaries when you push a version tag:

```bash
git tag v1.0.0
git push origin v1.0.0
```

## API Pricing Reference

### Firecrawl
- Free tier: 500 credits (one-time)
- Hobby: $16/month (3,000 credits)
- Standard: $83/month (100,000 credits)
- Search: 1 credit per request
- Scrape: 1 credit per page
- Crawl: 1 credit per page

### Tavily
- Free tier: 1,000 credits/month
- Project: $30/month (4,000 credits)
- Pay-as-you-go: $0.008 per credit
- Basic Search: 1 credit per request
- Advanced Search: 2 credits per request
- Extract: 1 credit per 5 successful extractions

## License

MIT
