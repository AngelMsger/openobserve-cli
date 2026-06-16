package app

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/angelmsger/openobserve-cli/internal/apiclient"
	cerrors "github.com/angelmsger/openobserve-cli/internal/errors"
	"github.com/spf13/cobra"
)

func newTraceCmd(s *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "trace",
		Short: "Search distributed traces and inspect a single trace",
		Long: "Traces are first-class in OpenObserve. `trace search` lists recent traces\n" +
			"(newest first) with their duration and the services involved — the map for\n" +
			"finding a slow or erroring request. `trace get <trace_id>` then reassembles\n" +
			"every span of one trace into a parent/child waterfall. Discover trace stream\n" +
			"names with `stream list --type traces`.",
	}
	cmd.AddCommand(newTraceSearchCmd(s), newTraceGetCmd(s))
	return cmd
}

func newTraceSearchCmd(s *appState) *cobra.Command {
	var (
		tf     timeFlags
		stream string
		filter string
		limit  int
		offset int
	)
	cmd := &cobra.Command{
		Use:   "search",
		Short: "List recent traces in a trace stream",
		Long: "Returns recent traces newest-first, each with its trace_id, duration and the\n" +
			"services that participated — enough to pick which trace to inspect with\n" +
			"`trace get`. Narrow the set with --filter (e.g. \"duration > 1000000\" for\n" +
			"traces slower than 1s). The time range is required.",
		Example: "  # slowest-first triage is done client-side; here, last hour of traces\n" +
			"  openobserve-cli trace search --stream default --since 1h --limit 20\n\n" +
			"  # only traces with an errored span\n" +
			"  openobserve-cli trace search --stream default --since 1h --filter \"span_status = 'ERROR'\"",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(stream) == "" {
				return noTraceStream()
			}
			start, end, err := tf.resolve()
			if err != nil {
				return err
			}
			resolvedFilter, err := readInlineOrFile(filter)
			if err != nil {
				return err
			}
			size := limit
			if size <= 0 {
				size = 50
			}

			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient()
			if err != nil {
				return err
			}
			resp, err := client.LatestTraces(ctx, s.org(), stream, start, end, offset, size, resolvedFilter)
			if err != nil {
				return err
			}
			hasMore := int64(offset+len(resp.Hits)) < resp.Total
			next := ""
			if hasMore {
				next = strconv.Itoa(offset + len(resp.Hits))
			}
			return s.emitList(resp.Hits, pageInfo{Next: next, HasMore: hasMore})
		},
	}
	f := cmd.Flags()
	f.StringVar(&stream, "stream", "", "trace stream to search (required)")
	f.StringVar(&filter, "filter", "", "predicate scoping which traces match (or @file / @-)")
	f.IntVar(&limit, "limit", 0, "max traces to return (default 50)")
	f.IntVar(&offset, "offset", 0, "trace offset for pagination (maps to 'from')")
	addTimeFlags(cmd, &tf)
	return cmd
}

func newTraceGetCmd(s *appState) *cobra.Command {
	var (
		tf     timeFlags
		stream string
	)
	cmd := &cobra.Command{
		Use:   "get <trace_id>",
		Short: "Reassemble one trace into a span waterfall",
		Long: "Fetches every span of a trace and assembles them into a parent/child tree,\n" +
			"with each span's offset from the trace start so you can read it as a\n" +
			"waterfall. JSON returns a nested tree; --format ndjson streams the spans\n" +
			"flat, one per line. The time range must contain the trace.",
		Example: "  openobserve-cli trace get 7be29a... --stream default --since 1h",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			traceID := strings.TrimSpace(args[0])
			if traceID == "" {
				return cerrors.New(cerrors.CategoryUsage, "NO_TRACE_ID",
					"a trace_id argument is required").
					WithNextSteps("openobserve-cli trace search --stream <name> --since 1h")
			}
			if strings.TrimSpace(stream) == "" {
				return noTraceStream()
			}
			start, end, err := tf.resolve()
			if err != nil {
				return err
			}

			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient()
			if err != nil {
				return err
			}
			// size: -1 retrieves every span of the trace (per the OpenObserve trace
			// API). Single-quotes in a trace_id are not valid, but escape defensively.
			sql := fmt.Sprintf(`SELECT * FROM "%s" WHERE trace_id = '%s' ORDER BY start_time`,
				stream, strings.ReplaceAll(traceID, "'", "''"))
			resp, err := client.Search(ctx, s.org(), apiclient.SearchRequest{
				Query: apiclient.SearchQuery{SQL: sql, StartTime: start, EndTime: end, Size: -1},
			})
			if err != nil {
				return err
			}
			if len(resp.Hits) == 0 {
				return cerrors.Newf(cerrors.CategoryNotFound, "TRACE_NOT_FOUND",
					"no spans found for trace_id %q in the given time range", traceID).
					WithHint("Widen the time range, or confirm the trace_id and stream.").
					WithNextSteps("openobserve-cli trace search --stream " + stream + " --since 24h")
			}

			tree := assembleTrace(traceID, resp.Hits)
			if s.cfg().Defaults.Format == "ndjson" {
				return s.emitList(tree.flatSpans, pageInfo{})
			}
			return s.emit(tree.summary)
		},
	}
	f := cmd.Flags()
	f.StringVar(&stream, "stream", "", "trace stream the trace lives in (required)")
	addTimeFlags(cmd, &tf)
	return cmd
}

func noTraceStream() error {
	return cerrors.New(cerrors.CategoryUsage, "NO_STREAM",
		"--stream is required (the trace stream to query)").
		WithNextSteps("openobserve-cli stream list --type traces")
}

// traceTree is the assembled view of a trace: a nested summary for JSON and a
// flat span list for ndjson.
type traceTree struct {
	summary   map[string]any
	flatSpans []map[string]any
}

// spanNode is one span plus its children, used while building the tree.
type spanNode struct {
	compact  map[string]any
	spanID   string
	parentID string
	start    float64
	children []*spanNode
}

// assembleTrace turns a flat list of span rows into a parent/child waterfall.
// Parent linkage and field names are read defensively because span payloads vary
// by instrumentation (OpenTelemetry attributes, field casing).
func assembleTrace(traceID string, hits []map[string]any) traceTree {
	// Find the trace start (earliest span) to express offsets relative to it.
	traceStart := 0.0
	for i, h := range hits {
		if st, ok := spanNumber(h, "start_time", "_timestamp"); ok {
			if i == 0 || st < traceStart {
				traceStart = st
			}
		}
	}

	nodes := make([]*spanNode, 0, len(hits))
	byID := make(map[string]*spanNode, len(hits))
	services := map[string]struct{}{}
	var traceEnd float64
	for _, h := range hits {
		n := &spanNode{
			spanID:   spanString(h, "span_id", "spanId"),
			parentID: spanString(h, "reference_parent_span_id", "parent_span_id", "parent_id", "parentSpanId"),
			compact:  compactSpan(h, traceStart),
		}
		n.start, _ = spanNumber(h, "start_time", "_timestamp")
		if end, ok := spanNumber(h, "end_time"); ok && end > traceEnd {
			traceEnd = end
		}
		if svc := spanString(h, "service_name", "service"); svc != "" {
			services[svc] = struct{}{}
		}
		nodes = append(nodes, n)
		if n.spanID != "" {
			byID[n.spanID] = n
		}
	}

	// Link children to parents; spans whose parent is absent are roots.
	roots := make([]*spanNode, 0)
	for _, n := range nodes {
		if parent, ok := byID[n.parentID]; ok && n.parentID != "" && parent != n {
			parent.children = append(parent.children, n)
		} else {
			roots = append(roots, n)
		}
	}
	sortNodes(roots)

	flat := make([]map[string]any, 0, len(nodes))
	for _, n := range nodes {
		flat = append(flat, n.compact)
	}

	rootViews := make([]any, 0, len(roots))
	for _, r := range roots {
		rootViews = append(rootViews, renderNode(r))
	}
	summary := map[string]any{
		"trace_id":   traceID,
		"span_count": len(nodes),
		"services":   sortedKeys(services),
		"spans":      rootViews,
	}
	if traceEnd > traceStart {
		summary["duration_micros"] = int64(traceEnd - traceStart)
	}
	return traceTree{summary: summary, flatSpans: flat}
}

// renderNode produces the nested JSON view of a span and its children.
func renderNode(n *spanNode) map[string]any {
	if len(n.children) == 0 {
		return n.compact
	}
	sortNodes(n.children)
	out := make(map[string]any, len(n.compact)+1)
	for k, v := range n.compact {
		out[k] = v
	}
	kids := make([]any, 0, len(n.children))
	for _, c := range n.children {
		kids = append(kids, renderNode(c))
	}
	out["children"] = kids
	return out
}

// compactSpan projects a span row to the high-signal waterfall fields, dropping
// the verbose attribute soup so a multi-hundred-span trace stays token-lean. For
// full attributes, query the stream directly with `search run`.
func compactSpan(h map[string]any, traceStart float64) map[string]any {
	out := map[string]any{}
	put := func(key, val string) {
		if val != "" {
			out[key] = val
		}
	}
	put("span_id", spanString(h, "span_id", "spanId"))
	put("parent_span_id", spanString(h, "reference_parent_span_id", "parent_span_id", "parent_id", "parentSpanId"))
	put("service_name", spanString(h, "service_name", "service"))
	put("operation_name", spanString(h, "operation_name", "name", "span_name"))
	put("span_status", spanString(h, "span_status", "status"))
	if d, ok := spanNumber(h, "duration"); ok {
		out["duration"] = d
	}
	if st, ok := spanNumber(h, "start_time", "_timestamp"); ok {
		out["start_time"] = int64(st)
		out["offset_micros"] = int64(st - traceStart)
	}
	if et, ok := spanNumber(h, "end_time"); ok {
		out["end_time"] = int64(et)
	}
	return out
}

func sortNodes(ns []*spanNode) {
	sort.SliceStable(ns, func(i, j int) bool { return ns[i].start < ns[j].start })
}

func sortedKeys(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// spanString returns the first non-empty string value among the candidate keys.
func spanString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if s, ok := m[k].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

// spanNumber returns the first numeric value among the candidate keys. Hits are
// decoded with encoding/json, so numbers arrive as float64.
func spanNumber(m map[string]any, keys ...string) (float64, bool) {
	for _, k := range keys {
		switch n := m[k].(type) {
		case float64:
			return n, true
		case int64:
			return float64(n), true
		}
	}
	return 0, false
}
