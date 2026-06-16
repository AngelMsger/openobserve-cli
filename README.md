# openobserve-cli

An agent-facing command-line interface for [OpenObserve](https://openobserve.ai)
(O2) — discover streams and run SQL searches over logs, metrics and traces, with
JSON output and structured errors designed for coding agents (Claude Code,
Codex) as much as for humans.

It is built to the patterns in
[`docs/agent-facing-cli-best-practices.md`](../../docs/agent-facing-cli-best-practices.md)
and is a sibling of `confluence-cli` / `bitbucket-cli`.

## Install

```bash
# from source (Go 1.24+)
make build            # -> ./bin/openobserve-cli
make install          # -> $GOBIN/openobserve-cli
```

## Quick start

```bash
# configure interactively (or use env vars, below)
openobserve-cli config init

# discover, then query
openobserve-cli org list
openobserve-cli stream list
openobserve-cli stream schema default
openobserve-cli search histogram --stream default --since 6h --interval 5m
openobserve-cli search run --stream default --where "level = 'ERROR'" --since 1h --limit 20
```

### Headless / CI / agents

No terminal needed — configure via environment:

```bash
export OPENOBSERVE_URL=http://localhost:5080
export OPENOBSERVE_ORG=default
export OPENOBSERVE_EMAIL=root@example.com
export OPENOBSERVE_PASSWORD='Complexpass#123'
# or: export OPENOBSERVE_TOKEN='<base64-or-Basic/Bearer value>'
```

## Highlights

- **`<noun> <verb>` command tree** — `org`, `stream`, `search`, `auth`, `config`,
  `doctor`, `skill`.
- **Search owns the footguns** — human time ranges (`--since 1h`, `--from`/`--to`)
  are converted to the microsecond epochs the API needs; stream/column names come
  from discovery commands, not guesses.
- **Map before terrain** — `search histogram` shows volume over time before
  `search run` pulls rows.
- **Structured errors** — every failure carries `category`, `code`, `hint`,
  `next_steps`, `retryable`, mapped to stable exit codes (0–11).
- **Token-friendly output** — `--format json|table|ndjson`, `--fields` projection,
  `{items, next, has_more}` lists.
- **Companion Skill** — embedded in the binary; `openobserve-cli skill install`
  deploys a version-matched copy to detected agents.

## Scope (v0.1)

Read-only: organizations, streams (discovery + schema), and SQL search /
histogram. Dashboards, alerts, functions/pipelines, users and ingestion are
planned (P1) and will plug into the already-wired `--dry-run` / `--yes` /
read-only safety gates.

## Configuration

Resolution precedence (highest first): flags → environment → `.env` → config file
(`~/.angelmsger/openobserve/config.yaml`) → defaults. Secrets live in the OS
keychain (with a `0600` file fallback), never in the config file. See the
companion Skill's [getting-started](skills/openobserve/references/getting-started.md).
