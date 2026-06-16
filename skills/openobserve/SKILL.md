---
name: openobserve
version: 0.2.0
description: "Query an OpenObserve (O2) backend from the command line across logs, metrics and traces: discover streams and schema, run SQL searches and histograms over logs, follow a stream live (tail -f), query metrics with PromQL (instant and range), and rebuild a trace into a span waterfall — with agent-friendly JSON and structured errors. Use this skill when the user mentions OpenObserve or O2, gives an OpenObserve URL, or asks to search / query / grep / tail logs; to query metrics or write PromQL (rate, p99, error rate); to inspect a trace, span or request latency; to find why a service is erroring, slow or crashing; to look at error rates, log volume or recent events; or to list or inspect streams or their fields. Covers self-hosted (localhost:5080) and Cloud. Set up with `openobserve-cli config init`, or OPENOBSERVE_URL / OPENOBSERVE_ORG / OPENOBSERVE_EMAIL / OPENOBSERVE_PASSWORD (or OPENOBSERVE_TOKEN) env vars. It is read-only: it cannot create dashboards, alerts, functions/pipelines or users yet."
metadata:
  requires:
    bins: ["openobserve-cli"]
  cliHelp: "openobserve-cli --help; openobserve-cli search run --help; openobserve-cli stream schema --help"
---

# openobserve

`openobserve-cli` queries an OpenObserve (O2) backend for you across all three
pillars — logs (SQL), metrics (PromQL) and traces (span trees). Output is JSON by
default; errors are JSON on stderr with a `category`, a `hint` and `next_steps`.
Everything is read-only.

## Golden rule — discover before you query

The single biggest mistake is **inventing a stream name, a column name, or a raw
microsecond timestamp**. Don't. The CLI gives you discovery commands for each:

1. **Org** — `org list` shows the organizations you can use. The default is
   `default`. Pass `--org` or set it once with `org use <id>`.
2. **Stream** — `stream list` shows the streams (the data sets). Never guess a
   name; list first.
3. **Columns** — `stream schema <name>` shows the queryable fields and the
   full-text-search keys, so your SQL `WHERE`/`SELECT` reference real columns.
4. **Time** — never compute epochs by hand. Use `--since 1h` (or `--from`/`--to`
   with RFC3339, a date, or `now-30m`); the CLI converts to the microseconds the
   API needs.

## Decision tree

- User asks to **look at / search / grep logs** (or traces/metrics) →
  `stream list` to find the stream, then `search run --stream <name> --since <window>`.
  Add `--where "<sql condition>"` to filter (e.g. `--where "level = 'ERROR'"`).
- User asks **"why is X erroring / how much / what's the volume"** → start with
  `search histogram --stream <name> --since <window> --interval <bucket>` to see
  the shape over time (the *map*), then pull the interesting window with
  `search run` (the *terrain*). See [searching.md](references/searching.md).
- User asks to **follow / tail logs live** → `search tail --stream <name>`
  (optionally `--where ...`, `--since 5m` to backfill); it streams new rows as
  ndjson until interrupted.
- User asks about **metrics / a PromQL expression / rate / error rate / p99** →
  metrics are PromQL, not SQL. `stream list --type metrics` to find the metric
  names, then `metrics query --query '<promql>'` (instant) or
  `metrics query-range --query '<promql>' --since 1h --step 1m` (over time). See
  [metrics-and-traces.md](references/metrics-and-traces.md).
- User asks to **inspect a trace / why a request is slow / a span** → traces are
  first-class. `stream list --type traces` to find the trace stream, then
  `trace search --stream <name> --since 1h` to find the trace, then
  `trace get <trace_id> --stream <name> --since 1h` for the span waterfall. See
  [metrics-and-traces.md](references/metrics-and-traces.md).
- User asks **what streams / fields exist** → `stream list`, then
  `stream schema <name>`. See [streams.md](references/streams.md).
- A query needs a column you're unsure of → `stream schema <name>` first.
- Anything fails → read the error's `next_steps`; they name the exact command to
  run next. See [errors-and-exit-codes.md](references/errors-and-exit-codes.md).
- Nothing configured yet / auth fails → [getting-started.md](references/getting-started.md).

## Guardrails

- **Always bound the time range.** Every `search` requires `--since` or
  `--from`/`--to`. Default to a narrow window (e.g. `1h`) and widen only if
  needed — unbounded scans are slow and flood your context.
- **Keep `--limit` small** (default 100). Pull a histogram first; only fetch the
  rows you actually need.
- **Reference real names only.** If you didn't get a stream or column from
  `stream list` / `stream schema`, don't put it in SQL. The same holds for metric
  names (`stream list --type metrics`) and trace streams (`--type traces`).
- **Metrics are PromQL, not SQL.** Use `metrics query` / `query-range` with a
  PromQL expression; don't try to `search run` a metrics stream.
- Prefer `--format ndjson` when piping hits into `jq`/`grep`; it streams one row
  per line. For very large result sets, `search run --all` pages through every
  matching row as ndjson (bound it with `--max`).

## Commands

```
openobserve-cli org list                       # discover organizations
openobserve-cli stream list [--type logs|metrics|traces]   # discover streams (the map)
openobserve-cli stream schema <name>           # queryable columns + settings
openobserve-cli search run --stream <name> --since 1h --where "level='ERROR'" --limit 20
openobserve-cli search run --sql @query.sql --since 24h    # read a long query from a file (@- = stdin)
openobserve-cli search histogram --stream <name> --since 6h --interval 5m
openobserve-cli search tail --stream <name> --where "level='ERROR'"   # follow live (Ctrl-C to stop)
openobserve-cli metrics query --query 'sum by (service)(rate(http_requests_total[5m]))'
openobserve-cli metrics query-range --query 'up' --since 1h --step 1m
openobserve-cli trace search --stream <name> --since 1h    # recent traces (trace_id, duration, services)
openobserve-cli trace get <trace_id> --stream <name> --since 1h    # span waterfall
openobserve-cli auth status                    # who am I / can I reach the server
openobserve-cli doctor                         # diagnose config / creds / connectivity
```

## Agent-facing conventions

- stdout is data only; diagnostics and errors go to stderr.
- Exit codes are stable and categorized (0 ok, 2 usage, 3 config, 4 auth, …);
  see [errors-and-exit-codes.md](references/errors-and-exit-codes.md).
- Lists come back as `{ "items": [...], "has_more": false }`.
- `--fields a,b.c` projects output to just those dot-paths to save tokens.
