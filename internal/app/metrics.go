package app

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/angelmsger/openobserve-cli/internal/timeutil"
	"github.com/angelmsger/openobserve-cli/pkg/apiclient"
	cerrors "github.com/angelmsger/openobserve-cli/pkg/errors"
	"github.com/spf13/cobra"
)

func newMetricsCmd(s *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "metrics",
		Short: "Query metrics with PromQL (instant and range)",
		Long: "Metrics in OpenObserve are queried with PromQL, not SQL. `metrics query`\n" +
			"evaluates an expression at a single instant; `metrics query-range` evaluates\n" +
			"it across a time window at a step resolution (the shape you graph). Discover\n" +
			"the metric names you can use with `stream list --type metrics`.",
	}
	cmd.AddCommand(newMetricsQueryCmd(s), newMetricsQueryRangeCmd(s))
	return cmd
}

func newMetricsQueryCmd(s *appState) *cobra.Command {
	var (
		query   string
		instant string
	)
	cmd := &cobra.Command{
		Use:   "query",
		Short: "Evaluate a PromQL expression at a single instant",
		Long: "Runs an instant PromQL query. The result is a vector (one value per\n" +
			"series) at the evaluation time, which defaults to now. Pass --query @file\n" +
			"or --query @- to read a long expression from a file or stdin.",
		Example: "  # current request rate per service\n" +
			"  openobserve-cli metrics query --query 'sum by (service)(rate(http_requests_total[5m]))'\n\n" +
			"  # evaluate at a past instant\n" +
			"  openobserve-cli metrics query --query 'up' --time 2024-01-02T15:04:05Z",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			promql, err := resolvePromQL(query)
			if err != nil {
				return err
			}
			timeSec, err := instantSeconds(instant)
			if err != nil {
				return err
			}

			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient()
			if err != nil {
				return err
			}
			resp, err := client.QueryMetricsInstant(ctx, s.org(), promql, timeSec)
			if err != nil {
				return enrichPromQLError(err)
			}
			return s.emitPromQL(promql, resp)
		},
	}
	f := cmd.Flags()
	f.StringVar(&query, "query", "", "PromQL expression (or @file / @- for stdin)")
	f.StringVar(&instant, "time", "", "evaluation instant: RFC3339, an epoch, or now-1h (default now)")
	_ = cmd.MarkFlagRequired("query")
	return cmd
}

func newMetricsQueryRangeCmd(s *appState) *cobra.Command {
	var (
		tf    timeFlags
		query string
		step  string
	)
	cmd := &cobra.Command{
		Use:   "query-range",
		Short: "Evaluate a PromQL expression across a time window",
		Long: "Runs a range PromQL query: the expression is evaluated at every --step\n" +
			"between the start and end of the time range, yielding a matrix (a series of\n" +
			"timestamped values per series) — the data behind a graph. The time range is\n" +
			"required (--since or --from/--to).",
		Example: "  # error rate over the last hour at 1-minute resolution\n" +
			"  openobserve-cli metrics query-range \\\n" +
			"    --query 'sum(rate(http_requests_total{status=~\"5..\"}[5m]))' --since 1h --step 1m",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			promql, err := resolvePromQL(query)
			if err != nil {
				return err
			}
			startMicros, endMicros, err := tf.resolve()
			if err != nil {
				return err
			}
			if strings.TrimSpace(step) == "" {
				return cerrors.New(cerrors.CategoryUsage, "NO_STEP",
					"--step is required (e.g. 30s, 1m, 5m)").
					WithNextSteps("openobserve-cli metrics query-range --query '<promql>' --since 1h --step 1m")
			}

			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient()
			if err != nil {
				return err
			}
			resp, err := client.QueryMetricsRange(ctx, s.org(), promql,
				microsToSeconds(startMicros), microsToSeconds(endMicros), step)
			if err != nil {
				return enrichPromQLError(err)
			}
			return s.emitPromQL(promql, resp)
		},
	}
	f := cmd.Flags()
	f.StringVar(&query, "query", "", "PromQL expression (or @file / @- for stdin)")
	f.StringVar(&step, "step", "", "evaluation step / resolution, e.g. 30s, 1m, 5m (required)")
	addTimeFlags(cmd, &tf)
	_ = cmd.MarkFlagRequired("query")
	return cmd
}

// emitPromQL renders a PromQL reply. JSON returns a summary plus the result;
// ndjson streams one series per line for piping. A 200-with-error envelope is
// surfaced as a structured PROMQL_ERROR.
func (s *appState) emitPromQL(promql string, resp *apiclient.PromQLResponse) error {
	if resp.Status == "error" || resp.Error != "" {
		return promQLError(resp.Error, resp.ErrorType)
	}
	var data struct {
		ResultType string          `json:"resultType"`
		Result     json.RawMessage `json:"result"`
	}
	_ = json.Unmarshal(resp.Data, &data)

	if s.cfg().Defaults.Format == "ndjson" {
		var series []any
		if json.Unmarshal(data.Result, &series) == nil && series != nil {
			return s.emitList(series, pageInfo{})
		}
	}
	return s.emit(map[string]any{
		"query":       promql,
		"result_type": data.ResultType,
		"result":      data.Result,
	})
}

// resolvePromQL resolves the --query flag (supporting @file / @-) and rejects an
// empty expression with discovery guidance.
func resolvePromQL(query string) (string, error) {
	promql, err := readInlineOrFile(query)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(promql) == "" {
		return "", cerrors.New(cerrors.CategoryUsage, "NO_QUERY",
			"--query is empty").
			WithNextSteps("openobserve-cli stream list --type metrics")
	}
	return promql, nil
}

// instantSeconds resolves the --time flag to Unix seconds, defaulting to now.
func instantSeconds(value string) (float64, error) {
	now := time.Now()
	if strings.TrimSpace(value) == "" {
		return microsToSeconds(now.UnixMicro()), nil
	}
	t, err := timeutil.ParseInstant(value, now)
	if err != nil {
		return 0, cerrors.Wrap(err, cerrors.CategoryUsage, "BAD_TIME",
			"invalid --time "+value+": "+err.Error()).
			WithHint("Use RFC3339, a unix epoch, 2006-01-02, or now-1h.")
	}
	return microsToSeconds(t.UnixMicro()), nil
}

// microsToSeconds converts Unix microseconds to fractional Unix seconds — PromQL
// endpoints take seconds, the _search API takes microseconds.
func microsToSeconds(micros int64) float64 { return float64(micros) / 1e6 }

// promQLError builds a structured error for a PromQL evaluation failure, pointing
// the caller at metric-name discovery (the most common cause is a typo'd or
// non-existent metric).
func promQLError(msg, errType string) error {
	if msg == "" {
		msg = "PromQL query failed"
	}
	if errType != "" {
		msg = errType + ": " + msg
	}
	return cerrors.New(cerrors.CategoryUsage, "PROMQL_ERROR", msg).
		WithHint("Check the metric name and label matchers — PromQL is not SQL.").
		WithNextSteps("openobserve-cli stream list --type metrics")
}

// enrichPromQLError adds metric-discovery guidance to a client-side (4xx) query
// failure so an agent isn't left at a dead end; network/server errors pass
// through unchanged.
func enrichPromQLError(err error) error {
	ce := cerrors.AsCLIError(err)
	if ce.Category == cerrors.CategoryUsage || ce.Category == cerrors.CategoryParse {
		ce.Code = "PROMQL_ERROR"
		return ce.WithHint("Check the metric name and label matchers — PromQL is not SQL.").
			WithNextSteps("openobserve-cli stream list --type metrics")
	}
	return err
}
