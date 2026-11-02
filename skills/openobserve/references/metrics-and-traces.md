# Metrics (PromQL) and traces (span trees)

Use SQL for record inspection (`search run` — see [searching.md](searching.md)),
PromQL for metric calculations, and `trace` for trace summaries and span trees.
Raw metric and trace records remain queryable with SQL when `--type metrics`
or `--type traces` selects their namespace.

## Metrics — `metrics query` and `metrics query-range`

Metric names are the `metrics`-type streams. Discover them when unknown:

```
openobserve-cli stream list --type metrics
```

Then write PromQL against those names. Prefer PromQL for rates, quantiles and
arithmetic over series; raw SQL records are a different view of the data.

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
usually `vector` (instant) or `matrix` (range); an instant expression can also
produce a matrix, scalar or string. `--format ndjson` streams one series per
line. Empty or null series lists write no rows. Scalar/string results stay in
one summary record, retaining the timestamp/value pair together. When the server
attaches advisories, summary documents also carry `warnings` / `infos`. Series
NDJSON and `--fields` projections also emit them on stderr as a
`query_advisories` notice. Read them before trusting the
numbers: a warning such as a truncated result changes their meaning. A rejected
expression returns `PROMQL_ERROR`; a reply that is not a successful PromQL
result returns `PROMQL_RESPONSE_INVALID` rather than an empty result. An empty
successful result does not establish that a metric is absent: check the metric,
labels, org, and time window before drawing that conclusion.

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
JSON exposes `next` and `has_more`; NDJSON emits only rows on stdout and reports
the same continuation in `_notice.pagination` on stderr. Pass its `next` value
as `--offset` with the same stream/filter and fixed absolute `--from` **and**
`--to` values. Repeating `--since` moves the window and can duplicate or skip
traces. Table footers also use `--offset`. Empty backend pages stop continuation
even if the server returns a stale larger total.

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
query its raw records with:

```
openobserve-cli search run --type traces --stream <name> --since 1h \
  --where "trace_id = '<id>'" --limit 100
```

Keep the trace's known time window. `trace get` selects the traces namespace
automatically; raw SQL needs the explicit `--type traces`.

## Tips

- A span's parent is read defensively across `reference_parent_span_id` /
  `parent_span_id`; spans whose parent is missing surface as additional roots
  rather than disappearing.
- `trace search --filter` takes an SQL-style predicate (e.g.
  `"span_status = 'ERROR'"`); it also accepts `@file` / `@-`.
- The time range must contain the trace — widen `--since` if `trace get` reports
  `TRACE_NOT_FOUND`. Keep the discovered trace ID and its incident window fixed
  while inspecting it; widen the window deliberately if spans may fall outside it.
