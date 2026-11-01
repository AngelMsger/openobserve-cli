package apiclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"

	cerrors "github.com/angelmsger/openobserve-cli/pkg/errors"
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
// decodes the standard {status,data,...} envelope. Only a reply that says
// "success" and names its result type is an answer. Anything else, such as a
// proxy's page or a reply cut short, would otherwise read as an empty result: a
// confident wrong answer instead of a diagnosable failure.
func (c *apiClient) promQL(ctx context.Context, org, op string, q url.Values) (*PromQLResponse, error) {
	path := "/api/" + url.PathEscape(org) + "/prometheus/api/v1/" + op
	var resp PromQLResponse
	if err := c.getJSON(ctx, path, q, &resp); err != nil {
		return nil, err
	}
	if resp.Status == "error" || resp.Error != "" {
		msg := resp.Error
		if msg == "" {
			msg = "PromQL query failed"
		}
		if resp.ErrorType != "" {
			msg = resp.ErrorType + ": " + msg
		}
		return nil, cerrors.New(cerrors.CategoryUsage, "PROMQL_ERROR", msg)
	}
	if resp.Status != "success" {
		return nil, invalidPromQLResponse(path, fmt.Sprintf("its status is %q, not \"success\"", resp.Status))
	}
	var data struct {
		ResultType string          `json:"resultType"`
		Result     json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return nil, invalidPromQLResponse(path, "its data could not be decoded: "+err.Error())
	}
	if data.ResultType == "" {
		return nil, invalidPromQLResponse(path, "its data names no resultType")
	}
	resp.ResultType, resp.Result = data.ResultType, data.Result
	return &resp, nil
}

func invalidPromQLResponse(path, why string) error {
	return cerrors.Newf(cerrors.CategoryParse, "PROMQL_RESPONSE_INVALID",
		"%s did not answer as a PromQL endpoint: %s", path, why).
		WithHint("The reply was not a successful PromQL result, so no result is reported. " +
			"Check that the base URL is the OpenObserve server root and that no proxy answers in its place.").
		WithNextSteps("Retry with --verbose to inspect the raw response.")
}

// formatSeconds renders a Unix-seconds instant without a trailing-zero suffix,
// preserving sub-second precision when present (Prometheus accepts fractional
// seconds).
func formatSeconds(s float64) string {
	return strconv.FormatFloat(s, 'f', -1, 64)
}
