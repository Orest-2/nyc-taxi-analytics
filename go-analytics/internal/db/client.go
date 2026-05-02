// Package db provides a client for the DuckDB Analytics HTTP server.
//
// It wraps all HTTP communication and JSON decoding so the rest of the
// CLI never sees raw HTTP. Only the standard library is used — no
// external dependencies required.
package db

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// QueryMeta describes a named query registered on the server.
type QueryMeta struct {
	Key       string `json:"key"`
	Title     string `json:"title"`
	Answers   string `json:"answers"`
	Technique string `json:"technique"`
}

// Result holds the response from a query execution.
type Result struct {
	QueryMeta
	Rows  []map[string]any `json:"rows"`
	Count int              `json:"count"`
}

// HealthResponse is the /health endpoint payload.
type HealthResponse struct {
	Status string `json:"status"`
	Rows   int64  `json:"rows"`
}

// Client talks to the DuckDB Analytics HTTP server.
type Client struct {
	base string
	http *http.Client
}

// New creates a Client pointed at baseURL (e.g. "http://127.0.0.1:8123").
func New(baseURL string) *Client {
	return &Client{
		base: baseURL,
		http: &http.Client{Timeout: 30 * time.Second},
	}
}

// Health calls GET /health and returns the server status.
func (c *Client) Health(ctx context.Context) (HealthResponse, error) {
	var h HealthResponse
	if err := c.get(ctx, "/health", &h); err != nil {
		return h, fmt.Errorf("health check: %w", err)
	}
	return h, nil
}

// ListQueries calls GET /queries and returns all registered query metadata.
func (c *Client) ListQueries(ctx context.Context) ([]QueryMeta, error) {
	var qs []QueryMeta
	if err := c.get(ctx, "/queries", &qs); err != nil {
		return nil, fmt.Errorf("listing queries: %w", err)
	}
	return qs, nil
}

// RunNamed calls POST /queries/{key} and returns the full result set.
func (c *Client) RunNamed(ctx context.Context, key string) (Result, error) {
	var r Result
	if err := c.post(ctx, "/queries/"+key, nil, &r); err != nil {
		return r, fmt.Errorf("running query %s: %w", key, err)
	}
	return r, nil
}

// RunSQL calls POST /query with an ad-hoc SQL string.
func (c *Client) RunSQL(ctx context.Context, sql string) (Result, error) {
	body := map[string]string{"sql": sql}
	var r Result
	if err := c.post(ctx, "/query", body, &r); err != nil {
		return r, fmt.Errorf("running SQL: %w", err)
	}
	return r, nil
}

// ── low-level helpers ─────────────────────────────────────────────────────────

func (c *Client) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return err
	}
	return c.do(req, out)
}

func (c *Client) post(ctx context.Context, path string, body any, out any) error {
	var buf io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshalling request: %w", err)
		}
		buf = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+path, buf)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.do(req, out)
}

func (c *Client) do(req *http.Request, out any) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		var e struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(data, &e) == nil && e.Error != "" {
			return fmt.Errorf("server error %d: %s", resp.StatusCode, e.Error)
		}
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}
	return nil
}
