package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/Orest-2/nyc-taxi-analytics/internal/db"
	"github.com/Orest-2/nyc-taxi-analytics/internal/query"
	"github.com/Orest-2/nyc-taxi-analytics/internal/report"
)

func main() {
	if err := newRootCmd(os.Stdout, os.Stderr).Execute(); err != nil {
		os.Exit(1)
	}
}

// newRootCmd builds the full command tree with injected writers so every
// command is testable without touching os.Stdout / os.Stderr.
func newRootCmd(stdout, stderr io.Writer) *cobra.Command {
	var (
		serverURL string
		formatStr string
		outFile   string
		noColor   bool
	)

	root := &cobra.Command{
		Use:           "taxi",
		Short:         "NYC Taxi Analytics CLI",
		Long:          "Query the DuckDB analytics server for NYC Yellow Taxi insights.",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRun: func(_ *cobra.Command, _ []string) {
			if noColor {
				os.Setenv("NO_COLOR", "1")
			}
		},
	}
	root.SetOut(stdout)
	root.SetErr(stderr)

	pf := root.PersistentFlags()
	pf.StringVar(&serverURL, "server", "http://127.0.0.1:8123", "analytics server base URL")
	pf.StringVar(&formatStr, "format", "table", "output format: table | csv | json")
	pf.StringVar(&outFile, "out", "", "write results to file (default: stdout)")
	pf.BoolVar(&noColor, "no-color", false, "disable ANSI colour codes")

	// openOut returns the writer for query results.
	// When --out is set it opens the file; caller must close it.
	openOut := func(fallback io.Writer) (io.Writer, func(), error) {
		if outFile == "" {
			return fallback, func() {}, nil
		}
		if err := os.MkdirAll(filepath.Dir(outFile), 0o755); err != nil {
			return nil, nil, fmt.Errorf("creating output directory: %w", err)
		}
		f, err := os.Create(outFile)
		if err != nil {
			return nil, nil, fmt.Errorf("creating output file: %w", err)
		}
		return f, func() { f.Close() }, nil
	}

	// ── health ────────────────────────────────────────────────────────────────
	root.AddCommand(&cobra.Command{
		Use:   "health",
		Short: "Check server connectivity and dataset row count",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			h, err := db.New(serverURL).Health(appCtx())
			if err != nil {
				return fmt.Errorf("server unreachable — is scripts/server.py running?\n  %w", err)
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "\n  \033[32m✓\033[0m Status:  %s\n", h.Status)
			fmt.Fprintf(w, "  \033[32m✓\033[0m Dataset: %s rows\n\n", formatInt(h.Rows))
			return nil
		},
	})

	// ── list ──────────────────────────────────────────────────────────────────
	root.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List all available named queries (Q01–Q12)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			qs, err := query.New(db.New(serverURL)).ListAll(appCtx())
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "\n  \033[1;36m%-6s %-38s %s\033[0m\n", "Key", "Title", "Technique")
			fmt.Fprintf(w, "  %s\n", strings.Repeat("─", 76))
			for _, q := range qs {
				fmt.Fprintf(w, "  \033[1;33m%-6s\033[0m %-38s %s\n",
					q.Key, truncate(q.Title, 37), truncate(q.Technique, 34))
			}
			fmt.Fprintln(w)
			return nil
		},
	})

	// ── run ───────────────────────────────────────────────────────────────────
	var runAll bool

	runCmd := &cobra.Command{
		Use:   "run [key]",
		Short: "Run a named query or all queries",
		Long: `Run a named query by key (e.g. Q07), or use --all to run every query.

Examples:
  taxi run Q07
  taxi run Q03 --format csv --out results/Q03.csv
  taxi run --all
  taxi run --all --format json --out results/all.json`,
		Args: func(cmd *cobra.Command, args []string) error {
			if !runAll && len(args) != 1 {
				return fmt.Errorf("requires a query key (e.g. Q07) or the --all flag")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := appCtx()
			runner := query.New(db.New(serverURL))
			fmt2 := report.Format(formatStr)
			term := cmd.ErrOrStderr()

			out, closeOut, err := openOut(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			defer closeOut()

			if runAll {
				return cmdRunAll(ctx, runner, out, fmt2, term)
			}
			return cmdRun(ctx, runner, args[0], out, fmt2, term)
		},
	}
	runCmd.Flags().BoolVar(&runAll, "all", false, "run every named query")
	root.AddCommand(runCmd)

	// ── sql ───────────────────────────────────────────────────────────────────
	root.AddCommand(&cobra.Command{
		Use:   "sql <statement>",
		Short: "Run an ad-hoc SQL query against the trips table",
		Long: `Execute any SQL statement against the trips table.

Examples:
  taxi sql "SELECT COUNT(*) FROM trips"
  taxi sql "SELECT VendorID, AVG(fare_amount) FROM trips GROUP BY 1"
  taxi sql "SELECT * FROM trips LIMIT 5" --format json`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := appCtx()
			runner := query.New(db.New(serverURL))
			fmt2 := report.Format(formatStr)
			term := cmd.ErrOrStderr()

			out, closeOut, err := openOut(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			defer closeOut()

			return cmdSQL(ctx, runner, strings.Join(args, " "), out, fmt2, term)
		},
	})

	root.InitDefaultCompletionCmd()
	return root
}

// ── command implementations ───────────────────────────────────────────────────

func cmdRun(ctx context.Context, r *query.Runner, key string, out io.Writer, fmt2 report.Format, term io.Writer) error {
	res, err := r.RunNamed(ctx, strings.ToUpper(key))
	if err != nil {
		return err
	}
	report.Header(term, res)
	if err := report.Write(out, res, fmt2); err != nil {
		return fmt.Errorf("writing output: %w", err)
	}
	report.Footer(term, res.Count)
	return nil
}

func cmdRunAll(ctx context.Context, r *query.Runner, out io.Writer, fmt2 report.Format, term io.Writer) error {
	qs, err := r.ListAll(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintf(term, "\n\033[1;36m=== Running all %d queries ===\033[0m\n", len(qs))
	for _, q := range qs {
		res, err := r.RunNamed(ctx, q.Key)
		if err != nil {
			fmt.Fprintf(term, "  \033[31m✗\033[0m %s: %v\n", q.Key, err)
			continue
		}
		report.Header(term, res)
		if err := report.Write(out, res, fmt2); err != nil {
			return err
		}
		report.Footer(term, res.Count)
	}
	fmt.Fprintf(term, "\n\033[32mAll done.\033[0m\n\n")
	return nil
}

func cmdSQL(ctx context.Context, r *query.Runner, sql string, out io.Writer, fmt2 report.Format, term io.Writer) error {
	res, err := r.RunSQL(ctx, sql)
	if err != nil {
		return err
	}
	res.Key = "AD-HOC"
	res.Title = "Custom SQL"
	report.Header(term, res)
	if err := report.Write(out, res, fmt2); err != nil {
		return fmt.Errorf("writing output: %w", err)
	}
	report.Footer(term, res.Count)
	return nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func appCtx() context.Context {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	_ = stop
	return ctx
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func formatInt(n int64) string {
	s := fmt.Sprintf("%d", n)
	out := make([]byte, 0, len(s)+len(s)/3)
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, byte(c))
	}
	return string(out)
}
