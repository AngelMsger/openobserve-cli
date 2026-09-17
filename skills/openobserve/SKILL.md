---
name: openobserve
version: 0.3.5
description: "Query OpenObserve (O2) logs, metrics, and traces: discover streams and fields, search SQL logs, inspect histograms, evaluate PromQL, reconstruct traces, and follow live logs. Use for an OpenObserve URL or an investigation whose backend is known to be OpenObserve, including errors, latency, request volume, and stream discovery. Supports self-hosted and Cloud instances. Remote operations are read-only; dashboards, alerts, pipelines, and user management are not supported."
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
2. **Stream** — reuse a name supplied by the user or already verified in the
   same server and org; otherwise discover it with `stream list`.
3. **Columns** — `stream schema <name>` shows the queryable fields and the
   full-text-search keys, so your SQL `WHERE`/`SELECT` reference real columns.
4. **Time** — never compute epochs by hand. Use `--since 1h` (or `--from`/`--to`
   with RFC3339, a date, or `now-30m`); the CLI converts to the microseconds the
   API needs.

Reuse discovery results within the same context. Prefer per-command `--org` and
`--use-context`; persist a different default only when the user asks for it.

## Decision tree

- User asks to **look at / search / grep logs** → discover the stream if unknown,
  then `search run --stream <name> --since <window>`.
  Add `--where "<sql condition>"` to filter (e.g. `--where "level = 'ERROR'"`).
- User asks **"why is X erroring / how much / what's the volume"** → start with
  `search histogram --stream <name> --since <window> --interval <bucket>` to see
  the shape over time (the *map*), then pull the interesting window with
  `search run` (the *terrain*). See [searching.md](references/searching.md).
- User asks to **follow / tail logs live** → `search tail --stream <name>`
  (optionally `--where ...`, `--since 5m` to backfill); it streams new rows as
  ndjson until interrupted. Before starting, choose a deadline or stopping
  condition from the request; see [Live monitoring](references/searching.md#live-monitoring).
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

## Configuration & credentials (agents)

Assume an already-configured user wants you to reuse their host config and OS
keychain. If an error is `CREDENTIAL_STORE_INACCESSIBLE` or
`CREDENTIAL_NOT_VISIBLE_OR_MISSING`, or has `recovery.scope=host`, request host
access and retry the same command once. Do not run `config init` / `auth login`
inside the sandbox; only ask the user to configure credentials when the host
retry also reports them missing. In a sandbox, use `OPENOBSERVE_EMAIL` /
`OPENOBSERVE_PASSWORD` (or `OPENOBSERVE_TOKEN`) env vars. An instance behind
SSO has no local password; on the user's own host (never in a sandbox) they
can run `openobserve-cli auth login --browser` to sign in through a real
browser window instead of installing the o3 desktop app — it needs a display,
so it only works where there is one. See
[getting-started.md](references/getting-started.md).

## Guardrails

- **Always bound the time range.** `search run` and `search histogram` require `--since` or
  `--from`/`--to`. Default to a narrow window (e.g. `1h`) and widen only if
  needed — unbounded scans are slow and flood your context.
- **Keep `--limit` small** (default 100). Use a histogram for broad volume
  investigations; go directly to a known request or narrow incident window.
- **Reference real names only.** Use known stream names and confirmed schema
  fields. Discover unknown metrics with `stream list --type metrics` and trace
  streams with `--type traces`.
- **Metrics are PromQL, not SQL.** Use `metrics query` / `query-range` with a
  PromQL expression; don't try to `search run` a metrics stream.
- Prefer `--format ndjson` when piping hits into `jq`/`grep`; it streams one row
  per line. For very large result sets, `search run --all` pages through every
  matching row as ndjson (bound it with `--max`).

Report the finding first, then the org/stream, exact time window with timezone,
and decisive evidence. Distinguish observations from likely causes and disclose
sampling or truncation that limits the conclusion. Quote short, relevant log
excerpts with credentials and personal data removed, not whole responses.

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
openobserve-cli config init|show               # configuration
openobserve-cli config contexts|use-context <name>   # list / switch named server contexts
openobserve-cli auth status                    # who am I / can I reach the server
openobserve-cli doctor                         # diagnose config / creds / connectivity
openobserve-cli skill status|install|path|show|uninstall   # manage the companion Skill
```

`--format` accepts `json` (default), `table` (human-readable), or `ndjson` (one
row per line, for piping). A per-invocation `--use-context <name>` overrides the
current context.

## Agent-facing conventions

- **Skill handshake — set `OPENOBSERVE_CLI_SKILL=0.3.5`.** Once you have
  loaded this Skill, export that exact value in the environment used to run the
  CLI. The CLI compares it with the embedded Skill version and emits a
  structured stderr notice when the Skill is missing, old, or uses the legacy
  unversioned handshake. `openobserve-cli skill status` reports loaded,
  installed, and embedded versions. To suppress the notice without loading the
  Skill, set `OPENOBSERVE_CLI_NO_SKILL_HINT=1`.
- **Update notices on stderr.** When a newer release exists, commands print a
  one-line `{"_notice":{"update":{…}}}` to **stderr** (never stdout) — on failed
  commands too, not just successful ones. Follow every `next_steps` entry:
  upgrade the CLI, run `openobserve-cli skill install`, then reload the agent
  context. `doctor` reports CLI and Skill status too. Silence update notices
  with `OPENOBSERVE_CLI_NO_UPDATE_NOTIFIER=1`, or skip the check per-run with
  `doctor --no-update-check`.
- stdout is data only; diagnostics and errors go to stderr.
- Exit codes are stable and categorized (0 ok, 2 usage, 3 config, 4 auth, …);
  see [errors-and-exit-codes.md](references/errors-and-exit-codes.md).
- Lists come back as `{ "items": [...], "has_more": false }`.
- `--fields a,b.c` projects output to just those dot-paths to save tokens.

## Team service presets and authentication

- Inspect existing configuration and reuse it. `config set-context <name>` is the
  offline installer entrypoint; it accepts `--base-url`, `--auth-scheme`,
  `--credential-url`, `--activate`, `--overwrite`, and `--dry-run`, plus `--org`.
- `OPENOBSERVE_AUTH_SCHEME` and `OPENOBSERVE_CREDENTIAL_URL` complement the existing
  service variables. Presets never copy a personal username or secret from the
  environment. Conflicts preserve existing values unless explicitly overwritten.
- Run `auth guide` to obtain the current instance's credential page, its source,
  navigation steps, and limitations. Links are hints, not evidence of server
  capabilities. Follow the returned product-specific instructions; do not invent
  a token URL or assume ingestion credentials authorize queries.
- Once a service is preset, direct the member to `auth login` in their terminal
  (or `auth login --browser` for OpenObserve SSO) to save their verified personal identity and secret. Do not ask for secrets in
  chat. In non-interactive environments use transient credential variables.
- Preserve host-keychain recovery for inaccessible credentials. A server/context
  mismatch requires selecting or creating a matching context; a partial login
  write error identifies what was stored and provides recovery steps.

See [team setup](references/team-setup.md) for the output fields, conflict
semantics, credential URL overrides, and failure recovery.
