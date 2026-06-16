# Metrics (PromQL) and traces (span trees)

Logs are queried with SQL (`search run` — see [searching.md](searching.md)).
**Metrics and traces are not.** OpenObserve queries metrics with PromQL and gives
traces a dedicated span model, so this CLI exposes them as their own commands.

## Metrics — `metrics query` and `metrics query-range`

Metric names are the `metrics`-type streams. Discover them first:

```
openobserve-cli stream list --type metrics
```

Then write PromQL against those names. Don't try to `search run` a metrics stream
— PromQL is the right tool for rates, quantiles and arithmetic over series.

**Instant** — evaluate an expression at one moment (defaults to now):

```
openobserve-cli metrics query \
  --query 'sum by (service)(rate(http_requests_total[5m]))'
openobserve-cli metrics query --query 'up' --time 2025-06-16T14:00:00Z
```

**Range** — evaluate across a window at a step resolution (the data behind a graph):

```
# 5xx error rate over the last hour, one point per minute
openobserve-cli metrics query-range \
  --query 'sum(rate(http_requests_total{status=~"5.."}[5m]))' --since 1h --step 1m
```

- `--query` is required; it accepts `@file` / `@-` (stdin) for long expressions.
- `--step` is required for `query-range` (e.g. `15s`, `1m`, `5m`).
- Time range uses the same `--since` / `--from` / `--to` forms as search. PromQL
  works in seconds; the CLI converts for you, so never pass raw epochs.

Output is `{ query, result_type, result }` where `result_type` is
`vector` (instant) or `matrix` (range); `--format ndjson` streams one series per
line. A bad expression or unknown metric returns a structured `PROMQL_ERROR` whose
`next_steps` points back at `stream list --type metrics`.

## Traces — `trace search` and `trace get`

Trace streams are the `traces`-type streams:

```
openobserve-cli stream list --type traces
```

**`trace search`** lists recent traces (newest first) — the map for finding a slow
or erroring request:

```
openobserve-cli trace search --stream default --since 1h --limit 20
openobserve-cli trace search --stream default --since 1h --filter "duration > 1000000"
```

Each item carries `trace_id`, `duration`, and the services involved — enough to
pick which trace to open. Results page via `--limit` / `--offset`.

**`trace get <trace_id>`** reassembles every span of one trace into a parent/child
waterfall, with each span's `offset_micros` from the trace start:

```
openobserve-cli trace get 7be29a… --stream default --since 1h
```

JSON returns a nested tree:

```json
{
  "trace_id": "7be29a…", "span_count": 12, "duration_micros": 482000,
  "services": ["api", "db", "web"],
  "spans": [
    { "span_id": "a", "service_name": "web", "operation_name": "GET /",
      "duration": 482000, "offset_micros": 0,
      "children": [ { "span_id": "b", "service_name": "api", "offset_micros": 1200, "children": [] } ] }
  ]
}
```

`--format ndjson` streams the spans flat, one per line. The waterfall is kept
token-lean (well-known span fields only); for the full attribute set of a trace,
query the stream directly with `search run --where "trace_id = '<id>'"`.

## Tips

- A span's parent is read defensively across `reference_parent_span_id` /
  `parent_span_id`; spans whose parent is missing surface as additional roots
  rather than disappearing.
- `trace search --filter` takes an SQL-style predicate (e.g.
  `"span_status = 'ERROR'"`); it also accepts `@file` / `@-`.
- The time range must contain the trace — widen `--since` if `trace get` reports
  `TRACE_NOT_FOUND`.
