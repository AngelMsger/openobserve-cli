package apiclient

import (
	"context"
	"net/url"
	"strconv"
)

// QueryMetricsInstant runs an instant PromQL query via
// GET /api/{org}/prometheus/api/v1/query. timeSec is the evaluation instant in
// Unix seconds — PromQL works in seconds, unlike the _search API which needs
// microseconds, so the caller (timeutil) owns that conversion.
func (c *apiClient) QueryMetricsInstant(ctx context.Context, org, promql string, timeSec float64) (*PromQLResponse, error) {
	q := url.Values{}
	q.Set("query", promql)
	q.Set("time", formatSeconds(timeSec))
	return c.promQL(ctx, org, "query", q)
}

// QueryMetricsRange runs a range PromQL query via
// GET /api/{org}/prometheus/api/v1/query_range. start/end are Unix seconds and
// step is a Prometheus duration (e.g. "30s", "1m", "5m").
func (c *apiClient) QueryMetricsRange(ctx context.Context, org, promql string, startSec, endSec float64, step string) (*PromQLResponse, error) {
	q := url.Values{}
	q.Set("query", promql)
	q.Set("start", formatSeconds(startSec))
	q.Set("end", formatSeconds(endSec))
	q.Set("step", step)
	return c.promQL(ctx, org, "query_range", q)
}

// promQL issues a GET against the org-scoped Prometheus-compatible endpoint and
// decodes the standard {status,data,...} envelope.
func (c *apiClient) promQL(ctx context.Context, org, op string, q url.Values) (*PromQLResponse, error) {
	path := "/api/" + url.PathEscape(org) + "/prometheus/api/v1/" + op
	var resp PromQLResponse
	if err := c.getJSON(ctx, path, q, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// formatSeconds renders a Unix-seconds instant without a trailing-zero suffix,
// preserving sub-second precision when present (Prometheus accepts fractional
// seconds).
func formatSeconds(s float64) string {
	return strconv.FormatFloat(s, 'f', -1, 64)
}
