.PHONY: setup load run run-save query all clean help \
        go-build go-test go-serve go-health go-list go-run go-sql go-all

BIN := bin/taxi

help:  ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN{FS=":.*?## "}; {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

# ── Python pipeline (via uv) ──────────────────────────────────

setup:  ## Create venv and install dependencies (uv sync)
	uv sync

load:  ## Download (or generate) data and load into DuckDB
	uv run scripts/load_data.py

run:  ## Run all 12 analytical queries
	uv run scripts/run_queries.py

run-save:  ## Run all queries and save CSVs to results/
	uv run scripts/run_queries.py --save-csv

query:  ## Run a single query, e.g. make query Q=Q03
	uv run scripts/run_queries.py --query $(Q)

all: setup load run-save  ## Full pipeline: setup → load → run

# ── Go CLI ────────────────────────────────────────────────────

go-build:  ## Build the Go CLI binary → bin/taxi
	mkdir -p bin
	cd go-analytics && go build -o ../$(BIN) ./cmd/taxi/

go-test:  ## Run Go unit tests (with race detector)
	cd go-analytics && go test -v -race ./...

go-serve:  ## Start the DuckDB HTTP server
	uv run scripts/server.py

go-health:  ## Check server connectivity
	./$(BIN) health

go-list:  ## List all 12 named queries
	./$(BIN) list

go-run:  ## Run a named query,  e.g. make go-run Q=Q07
	./$(BIN) run $(Q)

go-sql:  ## Run ad-hoc SQL,  e.g. make go-sql SQL="SELECT COUNT(*) FROM trips"
	./$(BIN) sql "$(SQL)"

go-all: go-build go-test  ## Build + test the Go CLI

# ── Cleanup ───────────────────────────────────────────────────

clean:  ## Remove generated files
	rm -f taxi.duckdb
	rm -rf results/ bin/ .venv/
	rm -f data/yellow_2025_01.parquet
