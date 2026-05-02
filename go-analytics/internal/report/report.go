// Package report formats query results as tables, CSV, or JSON.
package report

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/Orest-2/nyc-taxi-analytics/internal/db"
	"github.com/Orest-2/nyc-taxi-analytics/internal/query"
)

// Format selects the output format for a result.
type Format string

const (
	FormatTable Format = "table"
	FormatCSV   Format = "csv"
	FormatJSON  Format = "json"
)

// Write renders result to w in the requested format.
func Write(w io.Writer, res db.Result, f Format) error {
	switch f {
	case FormatCSV:
		return writeCSV(w, res)
	case FormatJSON:
		return writeJSON(w, res)
	default:
		return writeTable(w, res)
	}
}

// Header prints the query metadata banner to w.
func Header(w io.Writer, res db.Result) {
	sep := strings.Repeat("─", 68)
	fmt.Fprintf(w, "\n\033[1;36m%s\033[0m\n", sep)
	fmt.Fprintf(w, "\033[1;36m  %s  %s\033[0m\n", res.Key, res.Title)
	fmt.Fprintf(w, "\033[1;36m%s\033[0m\n", sep)
	if res.Answers != "" {
		fmt.Fprintf(w, "  \033[1;33mAnswers  :\033[0m %s\n", res.Answers)
	}
	if res.Technique != "" {
		fmt.Fprintf(w, "  \033[1;33mTechnique:\033[0m %s\n", res.Technique)
	}
	fmt.Fprintln(w)
}

// Footer prints row count and a success marker.
func Footer(w io.Writer, count int) {
	fmt.Fprintf(w, "\n  \033[32m✓\033[0m %d rows returned\n", count)
}

// ── formatters ────────────────────────────────────────────────────────────────

func writeTable(w io.Writer, res db.Result) error {
	cols := query.Columns(res.Rows)
	if len(cols) == 0 {
		fmt.Fprintln(w, "  (no rows)")
		return nil
	}

	// tabwriter aligns columns with tabs
	tw := tabwriter.NewWriter(w, 2, 2, 2, ' ', 0)

	// header row
	fmt.Fprintln(tw, "  "+strings.Join(cols, "\t"))
	// separator
	seps := make([]string, len(cols))
	for i, c := range cols {
		seps[i] = strings.Repeat("─", max(len(c), 4))
	}
	fmt.Fprintln(tw, "  "+strings.Join(seps, "\t"))

	// data rows
	for _, row := range res.Rows {
		vals := make([]string, len(cols))
		for i, c := range cols {
			vals[i] = fmt.Sprintf("%v", row[c])
		}
		fmt.Fprintln(tw, "  "+strings.Join(vals, "\t"))
	}

	return tw.Flush()
}

func writeCSV(w io.Writer, res db.Result) error {
	cols := query.Columns(res.Rows)
	cw := csv.NewWriter(w)

	if err := cw.Write(cols); err != nil {
		return fmt.Errorf("writing CSV header: %w", err)
	}
	for _, row := range res.Rows {
		rec := make([]string, len(cols))
		for i, c := range cols {
			rec[i] = fmt.Sprintf("%v", row[c])
		}
		if err := cw.Write(rec); err != nil {
			return fmt.Errorf("writing CSV row: %w", err)
		}
	}
	cw.Flush()
	return cw.Error()
}

func writeJSON(w io.Writer, res db.Result) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(res.Rows)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
