---
name: openobserve
version: 0.6.0
description: "Query OpenObserve (O2) telemetry and manage alert and dashboard definitions. Use for an OpenObserve URL, a known OpenObserve investigation, stream/schema discovery, SQL logs, PromQL metrics, trace inspection, live logs, or alert/dashboard backup and editing. Supports self-hosted and Cloud instances. Telemetry queries are read-only; remote changes support dry-run and read-only gating. Pipelines, user management and ingestion are not supported."
metadata:
  requires:
    bins: ["openobserve-cli"]
  cliHelp: "openobserve-cli --help; openobserve-cli search run --help"
---

# openobserve

Use `openobserve-cli` for OpenObserve. stdout is data; structured errors and
notices go to stderr. After loading this Skill, set
`OPENOBSERVE_CLI_SKILL=0.6.0` in the environment used for CLI commands.

## Choose the shortest useful path

Reuse the user's server, org, stream, schema and resource IDs when already
verified in that context. Discover only what is missing; do not run `doctor`,
list every resource, or read every reference before each task.

| Task | Start here | Details when needed |
| --- | --- | --- |
| Known request, log excerpt or narrow incident | `search run --stream <name> --since 1h --limit 20` | [Searching](references/searching.md) |
| Broad volume or error-spike investigation | `search histogram`, then query the interesting window | [Searching](references/searching.md) |
| Requested live monitoring | `search tail --stream <name>`; choose a stopping condition first | [Live monitoring](references/searching.md#live-monitoring) |
| Metric rate, quantile or series arithmetic | `metrics query` or `metrics query-range` (PromQL) | [Metrics and traces](references/metrics-and-traces.md) |
| Trace inspection | Known ID → `trace get`; otherwise `trace search` | [Metrics and traces](references/metrics-and-traces.md) |
| Unknown stream or field | `stream list`, then `stream schema <name> --type <type>` | [Streams](references/streams.md) |
| Alert backup, editing or enable/disable | `alert list`, then `alert get` | [Alerts](references/alerts.md) |
| Dashboard backup or editing | `dashboard list --title <text>`, then `dashboard get` | [Dashboards](references/dashboards.md) |
| Configuration/auth failure | Inspect the error's `code`, `recovery` and `next_steps` | [Getting started](references/getting-started.md), [errors](references/errors-and-exit-codes.md) |
| Preset a team service without credentials | `config set-context`, then `auth guide` / `auth reuse` | [Team setup](references/team-setup.md) |

## Scope, cost and correctness

- Prefer per-command `--use-context <name>` and `--org <id>`. Persist a different
  default with `config use-context` or `org use` only when requested. `org list`
  and `config get-contexts` discover those IDs.
- `stream list` returns names **and types**. SQL `search run|histogram|tail`
  defaults to `--type logs`; select `metrics` or `traces` explicitly for raw
  records in those namespaces. Prefer PromQL for metric calculations and
  `trace get` for a span tree; SQL remains useful for full record attributes.
- Confirm unfamiliar columns with `stream schema`; use the same stream type.
  Reuse discovery results rather than inventing identifiers or repeating calls.
- Bound queries with `--since` or `--from` and optional `--to`; never combine
  the two forms. `--to` requires `--from`. Human times such as `1h`, RFC3339 and
  `now-30m` avoid hand-computed epochs. Instant PromQL uses `--time` instead.
- Keep retrieval small (`search run --limit 20` is a useful start). Use
  `--fields a,b.c` to project output. `search run --all --max N` traverses one
  resolved window as NDJSON; `--limit` then means page size. Keep truncation and
  PromQL warnings/infos in the conclusion.
- Tail runs until stopped; `--since` is backfill and `--timeout` bounds each
  request, not the loop. Use the host's process controls to enforce a deadline
  or observed stopping condition and stop it before finishing the task.

## Remote writes

Alert and dashboard writes are **enabled by default**. Preview the intended
change with `--dry-run`; it is offline and checks request construction, not
server permissions, schema validity, alert delivery or dashboard rendering.
Execute within the user's authorized scope; existing authorization in this
session does not need to be requested again.

`OPENOBSERVE_CLI_READ_ONLY=1` or `defaults.read_only: true` blocks remote writes.
Use `--allow-writes` only when overriding that posture is authorized, not as a
routine prefix. Actual deletion also requires `--yes`; deletion previews do
not. Local config, login and Skill management are outside this remote gate.
`auth reuse --dry-run` may verify credentials over the network; it is not an
offline remote-write preview.

Retain discovered folders when fetching or modifying an alert/dashboard.
Dashboard updates use `--hash`, then the backup hash, otherwise a GET prefetch;
after a conflict, fetch the current definition and re-apply the intended edit.
Get the resource after a write to verify stored state. Working alert delivery
or dashboard rendering needs separate evidence; a successful save is not proof.

## Commands

These are canonical command paths. Read the linked reference or a command's
`--help` for optional flags; placeholders must come from the current context.

```text
openobserve-cli org list
openobserve-cli org use <id>
openobserve-cli stream list [--type logs|metrics|traces]
openobserve-cli stream get <name> [--type <type>]
openobserve-cli stream schema <name> [--type <type>]
openobserve-cli stream stats <name> [--type <type>]
openobserve-cli search run --stream <name> --type logs --since 1h --limit 20
openobserve-cli search run --sql @query.sql --type traces --since 1h
openobserve-cli search histogram --stream <name> --type logs --since 6h --interval 5m
openobserve-cli search tail --stream <name> --type logs --since 5m --interval 2s
openobserve-cli metrics query --query '<promql>'
openobserve-cli metrics query-range --query '<promql>' --since 1h --step 1m
openobserve-cli trace search --stream <name> --since 1h --limit 20
openobserve-cli trace get <trace_id> --stream <name> --since 1h
openobserve-cli alert list [--folder <folder-id>]
openobserve-cli alert get <id> [--folder <folder-id>]
openobserve-cli alert create --file alert.json --dry-run
openobserve-cli alert update <id> --file alert.json --dry-run
openobserve-cli alert enable <id> --dry-run
openobserve-cli alert disable <id> --dry-run
openobserve-cli alert delete <id> --dry-run
openobserve-cli dashboard list [--folder <folder-id>] [--title <text>]
openobserve-cli dashboard get <id> [--folder <folder-id>]
openobserve-cli dashboard create --file dashboard.json --dry-run
openobserve-cli dashboard update <id> --file dashboard.json --dry-run
openobserve-cli dashboard delete <id> --dry-run
openobserve-cli config init
openobserve-cli config show
openobserve-cli config get-contexts
openobserve-cli config use-context <name>
openobserve-cli config set-context <name> --base-url <url> --org <id> --dry-run
openobserve-cli config delete-context <name>
openobserve-cli auth guide
openobserve-cli auth reuse --dry-run
openobserve-cli auth login
openobserve-cli auth status
openobserve-cli auth logout
openobserve-cli doctor
openobserve-cli skill status
openobserve-cli skill install
openobserve-cli skill path
openobserve-cli skill show
openobserve-cli skill uninstall
openobserve-cli version
openobserve-cli completion bash|zsh|fish|powershell
```

`--sql` / `--query` / trace `--filter` accept `@file` and `@-` for stdin.
Alert/dashboard `--file` takes a path or `-` for stdin. `config contexts` is a
legacy alias for `config get-contexts`; prefer the canonical spelling.

## Output and continuation

`--format` accepts `json` (default), `table` or `ndjson`. Lists use
`{items, has_more, next?}`; SQL search returns `{hits, total, returned, ...}`,
and other commands return their own summary. NDJSON emits rows for lists,
SQL hits, trace spans and PromQL vector/matrix series; scalar/string PromQL
results stay together in a summary record. Empty lists emit no NDJSON rows.

Paginated NDJSON puts continuation on stderr as `_notice.pagination`, with
`next`, `has_more` and sibling `next_steps`. Pass `next` as `--offset`, keeping
the same query, type, org and fixed absolute `--from` **and** `--to`. Repeating
`--since` changes the window. JSON SQL summaries provide `start_micros` and
`end_micros` that can be reused unchanged as those flags. A full SQL page can
need one extra single-row read to check continuation.

Report the finding first, then org/stream, exact time window with timezone and
decisive evidence. Distinguish observations from likely causes, disclose sampling
or truncation, and quote short excerpts with secrets and personal data removed.

## Authentication and maintenance

Reuse host config and the OS keychain. For `CREDENTIAL_STORE_INACCESSIBLE`,
`CREDENTIAL_NOT_VISIBLE_OR_MISSING` or `recovery.scope=host`, retry the same
command once with host access when available. Do not replace inaccessible
credentials with new setup. Follow `auth reuse` guidance before fresh sign-in;
never collect secrets in chat. Headless sessions use transient credential
variables. Browser login needs a host browser and display. Details, including
`--from-context`, `--from-scheme` and browser `--replace`, are in
[getting started](references/getting-started.md).

`skill status` reports loaded/installed/embedded versions. After a CLI upgrade,
run `skill install` and reload the agent context. Update notices are
informational; do not interrupt an investigation to upgrade automatically.
`OPENOBSERVE_CLI_NO_SKILL_HINT=1` suppresses Skill notices;
`OPENOBSERVE_CLI_NO_UPDATE_NOTIFIER=1` suppresses update notices, and
`doctor --no-update-check` skips that check once. See
[getting started](references/getting-started.md) for installation options.
