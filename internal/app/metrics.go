package app

import (
	"encoding/json"
	"os"
	"strings"
	"time"

	"github.com/angelmsger/agent-cli-kit/output"
	"github.com/angelmsger/openobserve-cli/internal/timeutil"
	"github.com/angelmsger/openobserve-cli/pkg/apiclient"
	cerrors "github.com/angelmsger/openobserve-cli/pkg/errors"
	"github.com/spf13/cobra"
)

func newMetricsCmd(s *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "metrics",
		Short: "Query metrics with PromQL (instant and range)",
		Long: "Use PromQL for metric rates, quantiles and series arithmetic. `metrics query`\n" +
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
		Long: "Runs an instant PromQL query at the evaluation time, which defaults to now.\n" +
			"The expression determines the result type (vector, matrix, scalar or string). Pass --query @file\n" +
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

// emitPromQL renders a validated PromQL reply. JSON returns a summary, the
// result and the server's advisories; ndjson streams vector/matrix series
// and keeps scalar/string pairs together in one summary record. As in prometheus-cli, the advisories are re-emitted on stderr
// whenever stdout would not carry them: in ndjson mode, where only the series
// are written, and under --fields, where the projection can drop them.
func (s *appState) emitPromQL(promql string, resp *apiclient.PromQLResponse) error {
	if s.cfg().Defaults.Format == "ndjson" && (resp.ResultType == "vector" || resp.ResultType == "matrix") {
		var series []json.RawMessage
		if err := json.Unmarshal(resp.Result, &series); err != nil {
			return cerrors.Wrap(err, cerrors.CategoryParse, "PROMQL_RESPONSE_INVALID", "could not decode PromQL series")
		}
		if err := s.emitList(series, pageInfo{}); err != nil {
			return err
		}
		emitPromQLAdvisories(promql, resp)
		return nil
	}
	if len(s.fieldList()) > 0 {
		emitPromQLAdvisories(promql, resp)
	}
	doc := map[string]any{
		"query":       promql,
		"result_type": resp.ResultType,
		"result":      resp.Result,
	}
	if len(resp.Warnings) > 0 {
		doc["warnings"] = resp.Warnings
	}
	if len(resp.Infos) > 0 {
		doc["infos"] = resp.Infos
	}
	return s.emit(doc)
}

// emitPromQLAdvisories re-emits a reply's server warnings and infos on stderr
// as a structured notice. A warning such as "results are truncated" changes how
// the numbers should be read, so losing it silently would be a wrong answer.
func emitPromQLAdvisories(promql string, resp *apiclient.PromQLResponse) {
	if len(resp.Warnings) == 0 && len(resp.Infos) == 0 {
		return
	}
	notice := map[string]any{"query": promql}
	if len(resp.Warnings) > 0 {
		notice["warnings"] = resp.Warnings
	}
	if len(resp.Infos) > 0 {
		notice["infos"] = resp.Infos
	}
	output.EmitNotice(os.Stderr, map[string]any{"_notice": map[string]any{
		"query_advisories": notice,
		"next_steps": []string{
			"Read the server advisories before treating this result as complete.",
			"For a truncated result, narrow the selector or aggregate, then retry.",
		},
	}})
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

// enrichPromQLError points a rejected query (a 4xx, or a 200 whose envelope
// reports an error) at metric discovery, since the most common cause is a
// typo'd or non-existent metric. A reply that could not be read, and
// network or server errors, keep their own diagnosis.
func enrichPromQLError(err error) error {
	ce := cerrors.AsCLIError(err)
	if ce.Category == cerrors.CategoryUsage {
		ce.Code = "PROMQL_ERROR"
		return ce.WithHint("Check the metric name and label matchers — PromQL is not SQL.").
			WithNextSteps("openobserve-cli stream list --type metrics")
	}
	return err
}
