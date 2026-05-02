package report_test

import (
	"strings"
	"testing"

	"github.com/Orest-2/nyc-taxi-analytics/internal/db"
	"github.com/Orest-2/nyc-taxi-analytics/internal/report"
)

var sampleResult = db.Result{
	QueryMeta: db.QueryMeta{
		Key:       "Q01",
		Title:     "Daily trip volume",
		Answers:   "How does demand vary day-by-day?",
		Technique: "DATE_TRUNC, window function",
	},
	Rows: []map[string]any{
		{"trip_date": "2025-01-01", "total_trips": 3216, "avg_fare": 36.01},
		{"trip_date": "2025-01-02", "total_trips": 3181, "avg_fare": 37.00},
	},
	Count: 2,
}

func TestWriteTable(t *testing.T) {
	var buf strings.Builder
	if err := report.Write(&buf, sampleResult, report.FormatTable); err != nil {
		t.Fatalf("WriteTable: %v", err)
	}
	out := buf.String()

	for _, want := range []string{"trip_date", "total_trips", "avg_fare", "2025-01-01", "3216"} {
		if !strings.Contains(out, want) {
			t.Errorf("table output missing %q\ngot:\n%s", want, out)
		}
	}
}

func TestWriteCSV(t *testing.T) {
	var buf strings.Builder
	if err := report.Write(&buf, sampleResult, report.FormatCSV); err != nil {
		t.Fatalf("WriteCSV: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")

	if len(lines) != 3 { // header + 2 data rows
		t.Errorf("expected 3 CSV lines, got %d\n%s", len(lines), buf.String())
	}
	if !strings.Contains(lines[0], "trip_date") {
		t.Errorf("CSV header missing 'trip_date': %s", lines[0])
	}
}

func TestWriteJSON(t *testing.T) {
	var buf strings.Builder
	if err := report.Write(&buf, sampleResult, report.FormatJSON); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, `"2025-01-01"`) {
		t.Errorf("JSON output missing date value\ngot:\n%s", out)
	}
	if !strings.HasPrefix(strings.TrimSpace(out), "[") {
		t.Errorf("JSON output should be a JSON array\ngot:\n%s", out)
	}
}

func TestWriteEmptyResult(t *testing.T) {
	empty := db.Result{Count: 0}
	var buf strings.Builder
	if err := report.Write(&buf, empty, report.FormatTable); err != nil {
		t.Fatalf("WriteTable empty: %v", err)
	}
	if !strings.Contains(buf.String(), "no rows") {
		t.Errorf("expected 'no rows' message for empty result\ngot: %s", buf.String())
	}
}
