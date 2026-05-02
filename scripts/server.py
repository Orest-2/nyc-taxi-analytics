#!/usr/bin/env python3
"""
server.py  —  DuckDB Analytics HTTP Server
-------------------------------------------
A lightweight HTTP server that wraps DuckDB and exposes a simple JSON API.
The Go CLI (cmd/taxi) connects to this server to run queries.

This is a common real-world pattern:
  - A specialized analytics engine (DuckDB, ClickHouse, Trino) exposes HTTP
  - Go microservices / CLIs consume the API for data platform tooling

Endpoints:
  GET  /health          — liveness probe
  POST /query           — run a SQL query, returns JSON rows
  GET  /queries         — list all named queries
  POST /queries/{name}  — run a named query by key

Usage:
  python scripts/server.py [--port 8123] [--db taxi.duckdb]
"""

import argparse
import json
import math
import pathlib
import re
import sys
from http.server import BaseHTTPRequestHandler, HTTPServer
from urllib.parse import urlparse

import duckdb

ROOT     = pathlib.Path(__file__).parent.parent
DB_PATH  = ROOT / "taxi.duckdb"
SQL_FILE = ROOT / "queries" / "analysis.sql"

# ── Query parser ─────────────────────────────────────────────────────────────

def load_named_queries() -> dict[str, dict]:
    pattern = re.compile(
        r"--\s+──\s+(Q\d{2})\s+──\s+(.+?)\s+─+\s*\n"
        r"-- Answers:\s*(.+?)\n"
        r"-- Technique:\s*(.+?)\n"
        r"(.*?)(?=\n--\s+──\s+Q\d|$)",
        re.DOTALL,
    )
    sql = SQL_FILE.read_text()
    return {
        m.group(1): {
            "key":       m.group(1),
            "title":     m.group(2).strip(),
            "answers":   m.group(3).strip(),
            "technique": m.group(4).strip(),
            "sql":       m.group(5).strip(),
        }
        for m in pattern.finditer(sql)
    }


# ── JSON serialisation (handles NaN / Inf from DuckDB) ───────────────────────

def _safe(v):
    if isinstance(v, float) and (math.isnan(v) or math.isinf(v)):
        return None
    return v


def rows_to_json(cursor) -> list[dict]:
    cols = [d[0] for d in cursor.description]
    return [{c: _safe(v) for c, v in zip(cols, row)} for row in cursor.fetchall()]


# ── Handler ───────────────────────────────────────────────────────────────────

class Handler(BaseHTTPRequestHandler):
    con: duckdb.DuckDBPyConnection
    queries: dict[str, dict]

    def log_message(self, fmt, *args):  # quiet access log
        print(f"  {self.address_string()} {fmt % args}")

    # helpers ─────────────────────────────────────────────────────────────────

    def _send_json(self, code: int, body) -> None:
        payload = json.dumps(body, default=str).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)

    def _send_error(self, code: int, msg: str) -> None:
        self._send_json(code, {"error": msg})

    def _read_body(self) -> bytes:
        length = int(self.headers.get("Content-Length", 0))
        return self.rfile.read(length) if length else b""

    # routes ──────────────────────────────────────────────────────────────────

    def do_GET(self):
        path = urlparse(self.path).path

        if path == "/health":
            self._send_json(200, {"status": "ok", "rows": self.con.execute(
                "SELECT COUNT(*) FROM trips").fetchone()[0]})

        elif path == "/queries":
            self._send_json(200, [
                {"key": q["key"], "title": q["title"],
                 "answers": q["answers"], "technique": q["technique"]}
                for q in self.queries.values()
            ])

        else:
            self._send_error(404, f"unknown route: {path}")

    def do_POST(self):
        path = urlparse(self.path).path

        if path == "/query":
            body = self._read_body()
            try:
                payload = json.loads(body)
                sql = payload.get("sql", "").strip()
                if not sql:
                    return self._send_error(400, "missing 'sql' field")
                cur = self.con.execute(sql)
                rows = rows_to_json(cur)
                self._send_json(200, {"rows": rows, "count": len(rows)})
            except json.JSONDecodeError:
                self._send_error(400, "invalid JSON body")
            except Exception as exc:
                self._send_error(500, str(exc))

        elif path.startswith("/queries/"):
            key = path[len("/queries/"):].upper()
            q = self.queries.get(key)
            if not q:
                return self._send_error(404, f"query '{key}' not found")
            try:
                cur = self.con.execute(q["sql"])
                rows = rows_to_json(cur)
                self._send_json(200, {
                    "key":       q["key"],
                    "title":     q["title"],
                    "answers":   q["answers"],
                    "technique": q["technique"],
                    "rows":      rows,
                    "count":     len(rows),
                })
            except Exception as exc:
                self._send_error(500, str(exc))

        else:
            self._send_error(404, f"unknown route: {path}")


# ── Main ──────────────────────────────────────────────────────────────────────

def main():
    ap = argparse.ArgumentParser(description="DuckDB Analytics HTTP Server")
    ap.add_argument("--port", type=int, default=8123)
    ap.add_argument("--db",   default=str(DB_PATH))
    args = ap.parse_args()

    db = pathlib.Path(args.db)
    if not db.exists():
        sys.exit(f"Database not found: {db}\nRun: python scripts/load_data.py")

    queries = load_named_queries()

    con = duckdb.connect(str(db), read_only=True)
    Handler.con     = con
    Handler.queries = queries

    addr = ("127.0.0.1", args.port)
    srv  = HTTPServer(addr, Handler)

    print(f"\n🦆 DuckDB Analytics Server")
    print(f"   DB      : {db}")
    print(f"   Queries : {len(queries)} loaded")
    print(f"   Listening: http://127.0.0.1:{args.port}\n")
    print("   Routes:")
    print("     GET  /health")
    print("     GET  /queries")
    print("     POST /queries/<key>  (Q01 … Q12)")
    print("     POST /query          body: {\"sql\": \"SELECT ...\"}\n")
    print("   Press Ctrl+C to stop\n")

    try:
        srv.serve_forever()
    except KeyboardInterrupt:
        print("\nShutting down.")


if __name__ == "__main__":
    main()
