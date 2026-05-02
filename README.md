# 🚕 NYC Taxi Analytics

A SQL analytics portfolio project using the **NYC TLC Yellow Taxi** public dataset, [DuckDB](https://duckdb.org/), Python (managed with [uv](https://docs.astral.sh/uv/)), and a Go CLI.

Built as Phase 1 of a Software Engineer → Data Engineer migration roadmap.

---

## What This Project Demonstrates

| Skill             | How                                                                       |
| ----------------- | ------------------------------------------------------------------------- |
| Advanced SQL      | 12 queries covering window functions, CTEs, aggregates, anomaly detection |
| Data modeling     | Columnar parquet, analytical query patterns, star schema thinking         |
| DuckDB            | In-process OLAP engine, parquet-native queries, HTTP JSON API             |
| Python tooling    | `uv` for dependency management, zero-install onboarding                   |
| Go CLI            | Stdlib-only HTTP client, `internal/` packages, tabular/CSV/JSON output    |
| Project structure | Reproducible pipeline with a Makefile, lockfile committed                 |

---

## Architecture

```
┌─────────────────────────────────┐     HTTP/JSON     ┌──────────────────────┐
│  scripts/server.py              │ ◄────────────────► │  bin/taxi  (Go CLI)  │
│  Python · DuckDB analytics API  │                    │  stdlib only, no deps│
└─────────────────┬───────────────┘                    └──────────────────────┘
                  │ reads
          taxi.duckdb  (DuckDB file)
                  │ loaded from
          data/yellow_2025_01.parquet
```

The Python server wraps DuckDB and exposes a simple JSON API. The Go CLI consumes that API — the same pattern used in real data platforms (Go services querying ClickHouse, Trino, or BigQuery over HTTP).

---

## Dataset

**NYC TLC Yellow Taxi Trip Records — January 2025**

- Source: [NYC Taxi & Limousine Commission Open Data](https://www.nyc.gov/site/tlc/about/tlc-trip-record-data.page)
- Format: Parquet (~50 MB, ~3M rows)
- Key columns: pickup/dropoff timestamps, location IDs, fare, tip, tolls, distance, passenger count

> The loader falls back to 100 000 rows of realistic synthetic data if the download fails (e.g. network restrictions).

---

## Project Structure

```
nyc-taxi-analytics/
├── pyproject.toml               # uv project: requires-python >=3.12, duckdb dep
├── uv.lock                      # pinned lockfile — commit this
├── Makefile                     # all targets documented, run `make help`
│
├── data/
│   └── yellow_2025_01.parquet   # raw data (gitignored)
├── queries/
│   └── analysis.sql             # 12 self-contained analytical queries
├── results/
│   └── Q01.csv … Q12.csv        # query outputs (gitignored)
│
├── scripts/                     # Python pipeline
│   ├── load_data.py             # download or generate data → taxi.duckdb
│   ├── run_queries.py           # execute queries, print results, export CSV
│   └── server.py                # DuckDB HTTP server (used by Go CLI)
│
└── go-analytics/                # Go CLI
    ├── go.mod                   # module: github.com/Orest-2/nyc-taxi-analytics
    ├── cmd/taxi/
    │   └── main.go              # CLI entrypoint: health, list, run, sql
    └── internal/
        ├── db/client.go         # HTTP client for the analytics server
        ├── query/runner.go      # query orchestration + Columns() helper
        └── report/report.go     # table / CSV / JSON formatters
```

---

## Quick Start

**Prerequisites:** [uv](https://docs.astral.sh/uv/getting-started/installation/) · [Go 1.22+](https://go.dev/dl/) (for the CLI)

```bash
# 1. Clone
git clone https://github.com/Orest-2/nyc-taxi-analytics.git
cd nyc-taxi-analytics

# 2. Install dependencies and load data
make setup   # uv sync — creates .venv, installs duckdb
make load    # download dataset (or generate synthetic fallback)

# 3. Run all 12 queries (Python)
make run

# 4. Save CSV results to results/
make run-save

# 5. Run a single query
make query Q=Q07
```

### Go CLI

```bash
# Terminal 1 — start the DuckDB server
make go-serve

# Terminal 2 — build and use the CLI
make go-build

./bin/taxi health
./bin/taxi list
./bin/taxi run Q07
./bin/taxi run Q03 --format csv --out results/Q03.csv
./bin/taxi run --all --format json
./bin/taxi sql "SELECT vendor_id, COUNT(*) FROM trips GROUP BY 1"
```

### All make targets

```
make help
```

| Target             | Description                                              |
| ------------------ | -------------------------------------------------------- |
| `setup`            | `uv sync` — create `.venv` and install deps              |
| `load`             | Download dataset and load into DuckDB                    |
| `run`              | Run all 12 queries (Python)                              |
| `run-save`         | Run all queries and export CSV to `results/`             |
| `query Q=QXX`      | Run a single named query                                 |
| `go-build`         | Compile Go CLI → `bin/taxi`                              |
| `go-test`          | Run Go unit tests with race detector                     |
| `go-serve`         | Start the DuckDB HTTP server                             |
| `go-run Q=QXX`     | Run a named query via Go CLI                             |
| `go-sql SQL="..."` | Run ad-hoc SQL via Go CLI                                |
| `clean`            | Remove `.venv`, `bin/`, `results/`, parquet, DuckDB file |

---

## The 12 Queries

| Key | Title                             | Technique                                     |
| --- | --------------------------------- | --------------------------------------------- |
| Q01 | Daily trip volume & revenue trend | `DATE_TRUNC`, rolling 7-day `AVG() OVER`      |
| Q02 | Peak hour demand heatmap          | `EXTRACT(DOW/HOUR)`, multi-dimension GROUP BY |
| Q03 | Tip percentage by payment type    | `FILTER` aggregate, `NULLIF` guard            |
| Q04 | Fare-per-mile by distance bucket  | `CASE` bucketing, derived ratio               |
| Q05 | Top 10 busiest pickup zones       | Simple aggregation ranked by volume           |
| Q06 | Vendor performance comparison     | `PERCENTILE_CONT`, p95 fare                   |
| Q07 | Revenue cohort: top 1% vs rest    | `PERCENT_RANK()`, CTE, window `SUM`           |
| Q08 | Fare anomaly detection            | Z-score via `STDDEV`, `CROSS JOIN` stats      |
| Q09 | Running cumulative revenue        | `ROWS BETWEEN UNBOUNDED PRECEDING`            |
| Q10 | Day-over-day trip count change    | `LAG()`, percentage delta                     |
| Q11 | Passenger count distribution      | `pct_of_total` with `SUM() OVER ()`           |
| Q12 | Zone-pair route popularity        | Composite `GROUP BY`, `HAVING`                |

---

## Key SQL Patterns

```sql
-- Rolling 7-day average (Q01)
AVG(COUNT(*)) OVER (
    ORDER BY DATE_TRUNC('day', pickup_datetime)
    ROWS BETWEEN 6 PRECEDING AND CURRENT ROW
)

-- Z-score anomaly detection (Q08)
(fare_amount - AVG(fare_amount) OVER ()) / STDDEV(fare_amount) OVER ()

-- Percentage of total (Q11)
100.0 * COUNT(*) / SUM(COUNT(*)) OVER ()

-- Cohort revenue share (Q07)
100.0 * SUM(total) / SUM(SUM(total)) OVER ()
```

---

## Why These Tools?

**DuckDB** — zero setup, parquet-native, full SQL (window functions, `PERCENTILE_CONT`, `FILTER`). The OLAP engine of choice for local analytics.

**uv** — replaces `pip` + `venv` + `pip-tools` in one tool. `uv sync` installs exact pinned versions in under a second. `uv run` executes scripts in the managed environment without activating it manually. Committing `uv.lock` guarantees every contributor gets identical packages.

**Go CLI** — demonstrates that a SWE background is an asset in DE. Real data platforms (Uber, Cloudflare) use Go for high-throughput ingestion services and internal tooling. The `taxi` binary uses only the standard library: no external deps, single static binary, ships anywhere.

---

## Go CLI — What It Demonstrates

```
internal/db/client.go    HTTP client with context, timeouts, proper Body drain
internal/query/runner.go Clean separation: query logic vs presentation
internal/report/         table / CSV / JSON — io.Writer injected for testability
cmd/taxi/main.go         Thin main() → testable run(args, stdout, stderr)
```

All tests pass with `-race`:

```
ok  internal/query   (TestColumnsEmpty, TestColumnsSorted, TestColumnsStable)
ok  internal/report  (TestWriteTable, TestWriteCSV, TestWriteJSON, TestWriteEmptyResult)
```

---

## Next Steps (Phase 2)

- [ ] Model raw trips into `dim_vendor`, `dim_zone`, `fact_trips` with dbt
- [ ] Schedule daily ingestion with Apache Airflow
- [ ] Add dbt data quality tests (`not_null`, `unique`, custom SQL assertions)
- [ ] Write a Go ingestion microservice reading from the TLC API → Kafka → DuckDB

---

## License

MIT
