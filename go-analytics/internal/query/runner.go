// Package query provides helpers for executing and working with query results.
package query

import (
	"context"
	"fmt"
	"sort"

	"github.com/Orest-2/nyc-taxi-analytics/internal/db"
)

// Runner executes named and ad-hoc queries via the db.Client.
type Runner struct {
	client *db.Client
}

// New creates a Runner backed by the given client.
func New(client *db.Client) *Runner {
	return &Runner{client: client}
}

// RunNamed executes a named query (Q01–Q12) and returns the result.
func (r *Runner) RunNamed(ctx context.Context, key string) (db.Result, error) {
	res, err := r.client.RunNamed(ctx, key)
	if err != nil {
		return db.Result{}, fmt.Errorf("runner: %w", err)
	}
	return res, nil
}

// RunSQL executes an ad-hoc SQL string and returns the result.
func (r *Runner) RunSQL(ctx context.Context, sql string) (db.Result, error) {
	res, err := r.client.RunSQL(ctx, sql)
	if err != nil {
		return db.Result{}, fmt.Errorf("runner: %w", err)
	}
	return res, nil
}

// ListAll returns metadata for every registered query.
func (r *Runner) ListAll(ctx context.Context) ([]db.QueryMeta, error) {
	qs, err := r.client.ListQueries(ctx)
	if err != nil {
		return nil, fmt.Errorf("runner: %w", err)
	}
	return qs, nil
}

// Columns returns a stable, sorted list of column names from the first row.
// This ensures consistent column ordering for tabular output.
func Columns(rows []map[string]any) []string {
	if len(rows) == 0 {
		return nil
	}
	cols := make([]string, 0, len(rows[0]))
	for k := range rows[0] {
		cols = append(cols, k)
	}
	sort.Strings(cols)
	return cols
}
