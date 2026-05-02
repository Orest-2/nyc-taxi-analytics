"""
run_queries.py
--------------
Executes all 12 analytical queries against the local DuckDB database
and prints results to stdout. Optionally saves CSV snapshots to results/.

Usage:
    python scripts/run_queries.py              # run all, print to terminal
    python scripts/run_queries.py --save-csv   # also write results/Q*.csv
    python scripts/run_queries.py --query Q03  # run a single query
"""

import re
import sys
import pathlib
import argparse
import textwrap
import duckdb

ROOT       = pathlib.Path(__file__).parent.parent
DB_PATH    = ROOT / "taxi.duckdb"
SQL_FILE   = ROOT / "queries" / "analysis.sql"
RESULT_DIR = ROOT / "results"

# ── helpers ──────────────────────────────────────────────────────────────────

ANSI = {
    "header": "\033[1;36m",   # bold cyan
    "label" : "\033[1;33m",   # bold yellow
    "ok"    : "\033[32m",     # green
    "reset" : "\033[0m",
}

def color(s: str, key: str) -> str:
    return f"{ANSI[key]}{s}{ANSI['reset']}"


def parse_queries(sql: str) -> dict[str, dict]:
    """Split the SQL file into named query blocks keyed by Q01, Q02 …"""
    pattern = re.compile(
        r"--\s+──\s+(Q\d{2})\s+──\s+(.+?)\s+─+\s*\n"   # header line
        r"-- Answers:\s*(.+?)\n"                           # answers line
        r"-- Technique:\s*(.+?)\n"                        # technique line
        r"(.*?)(?=\n--\s+──\s+Q\d|$)",                   # SQL body
        re.DOTALL,
    )
    queries: dict[str, dict] = {}
    for m in pattern.finditer(sql):
        key, title, answers, technique, body = m.groups()
        # strip leading/trailing blank lines from the SQL body
        body = body.strip()
        queries[key] = {
            "title"    : title.strip(),
            "answers"  : answers.strip(),
            "technique": technique.strip(),
            "sql"      : body,
        }
    return queries


def run_query(con: duckdb.DuckDBPyConnection, q: dict, key: str, save: bool) -> None:
    sep = "─" * 70
    print(f"\n{color(sep, 'header')}")
    print(color(f"  {key}  {q['title']}", "header"))
    print(f"{color(sep, 'header')}")
    print(f"  {color('Answers  :', 'label')} {q['answers']}")
    print(f"  {color('Technique:', 'label')} {q['technique']}")
    print()

    try:
        rel    = con.execute(q["sql"])
        cols   = [d[0] for d in rel.description]
        rows   = rel.fetchall()

        # tabulate without pandas
        widths = [len(c) for c in cols]
        for row in rows:
            for i, v in enumerate(row):
                widths[i] = max(widths[i], len(str(v) if v is not None else ""))

        header = "  " + "  ".join(c.ljust(widths[i]) for i, c in enumerate(cols))
        sep    = "  " + "  ".join("-" * w for w in widths)
        print(header)
        print(sep)
        for row in rows:
            line = "  " + "  ".join(str(v if v is not None else "").ljust(widths[i]) for i, v in enumerate(row))
            print(line)

        print(f"\n  {color('✓', 'ok')} {len(rows)} rows returned")

        if save:
            import csv as _csv
            RESULT_DIR.mkdir(exist_ok=True)
            out = RESULT_DIR / f"{key}.csv"
            with open(out, "w", newline="") as f:
                w = _csv.writer(f)
                w.writerow(cols)
                w.writerows(rows)
            print(f"  {color('✓', 'ok')} saved → {out.relative_to(ROOT)}")

    except Exception as exc:
        print(f"  ERROR: {exc}", file=sys.stderr)


# ── main ─────────────────────────────────────────────────────────────────────

def main() -> None:
    parser = argparse.ArgumentParser(description="Run NYC Taxi analytical queries")
    parser.add_argument("--save-csv", action="store_true",
                        help="Write each result to results/Q*.csv")
    parser.add_argument("--query", metavar="QXX",
                        help="Run a single query by key, e.g. --query Q03")
    args = parser.parse_args()

    if not DB_PATH.exists():
        sys.exit(
            f"Database not found at {DB_PATH}.\n"
            "Run  python scripts/load_data.py  first."
        )

    sql     = SQL_FILE.read_text()
    queries = parse_queries(sql)

    if not queries:
        sys.exit("No queries parsed from analysis.sql — check formatting.")

    con = duckdb.connect(str(DB_PATH), read_only=True)

    if args.query:
        key = args.query.upper()
        if key not in queries:
            sys.exit(f"Query '{key}' not found. Available: {', '.join(queries)}")
        run_query(con, queries[key], key, args.save_csv)
    else:
        print(color(f"\n=== NYC Taxi Analytics — {len(queries)} Queries ===\n", "header"))
        for key, q in sorted(queries.items()):
            run_query(con, q, key, args.save_csv)

    print(f"\n{color('All done.', 'ok')}\n")


if __name__ == "__main__":
    main()
