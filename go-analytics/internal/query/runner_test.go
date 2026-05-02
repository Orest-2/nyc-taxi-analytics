package query_test

import (
	"testing"

	"github.com/Orest-2/nyc-taxi-analytics/internal/query"
)

func TestColumnsEmpty(t *testing.T) {
	cols := query.Columns(nil)
	if cols != nil {
		t.Errorf("expected nil for empty rows, got %v", cols)
	}
}

func TestColumnsSorted(t *testing.T) {
	rows := []map[string]any{
		{"zebra": 1, "apple": 2, "mango": 3},
	}
	cols := query.Columns(rows)
	want := []string{"apple", "mango", "zebra"}
	if len(cols) != len(want) {
		t.Fatalf("expected %d cols, got %d", len(want), len(cols))
	}
	for i, w := range want {
		if cols[i] != w {
			t.Errorf("cols[%d] = %q, want %q", i, cols[i], w)
		}
	}
}

func TestColumnsStable(t *testing.T) {
	// calling Columns twice on the same rows should return the same order
	rows := []map[string]any{{"b": 1, "a": 2, "c": 3}}
	first := query.Columns(rows)
	second := query.Columns(rows)
	for i := range first {
		if first[i] != second[i] {
			t.Errorf("non-deterministic column order at index %d: %q vs %q", i, first[i], second[i])
		}
	}
}
