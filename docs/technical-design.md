# Technical design

`openobserve-cli` is a Go + [Cobra](https://github.com/spf13/cobra) CLI for
OpenObserve (O2), built to the agent-facing conventions it shares with its
sibling projects (`confluence-cli`, `bitbucket-cli`). This document describes the
architecture and the `internal/` and `pkg/` package layout.

## Overview

A command flows through four layers:

```
cmd/openobserve-cli  →  internal/app  →  pkg/apiclient  →  pkg/transport
   (process entry)       (cobra tree,      (OpenObserve API       (retrying HTTP,
                          appState,         surface + models)       auth decorator)
                          rendering)
```

- `cmd/openobserve-cli/main.go` is a three-line entry point: `os.Exit(app.Execute())`.
- `internal/app` builds the cobra command tree, resolves configuration and
  credentials, calls the API client, and renders the result.
- `pkg/apiclient` is the typed OpenObserve API surface.
- `pkg/transport` is a flavor-agnostic retrying HTTP client.

Cross-cutting packages — `errors`, `output`, `config`, `auth`, `timeutil`,
`cliflags`, `constants` — are used across the layers.

## Command layer (`internal/app`)

`root.go` assembles the tree and owns `Execute()`. Before cobra parses argv,
`cliflags.Normalize` rewrites common LLM slips (camelCase flag names,
flag-stuck-to-value) and echoes each correction as a `_notice` on stderr. On
error, the outermost handler converts the error to a `*errors.CLIError`, writes
it to stderr, and returns the mapped exit code.

`context.go` holds **`appState`**, the runtime context built once in the root
command's `PersistentPreRunE` and captured by every subcommand:

- `load()` resolves configuration from all layers (see Config).
- `newClient()` resolves credentials, builds an authenticated API client, and —
  when the session is read-only — wraps it with `apiclient.NewReadOnly`.
- `emit()` / `emitList()` render results; `org()`, `timeout()`, `readOnly()` are
  convenience accessors.

Each noun lives in its own file (`org.go`, `stream.go`, `search.go`, `metrics.go`,
`trace.go`, `auth.go`, `config.go`, `doctor.go`, `skill.go`), organised
`<noun> <verb>`. `search.go` owns the SQL-building helpers and is the heart of the
logs path: it converts human time ranges to microseconds (via `timeutil`) and
builds `SELECT` / `histogram` queries so the API client only ever receives a ready
query; it also hosts `search tail` (poll-and-stream) and `--all` paging.
`metrics.go` queries the Prometheus-compatible PromQL endpoints (times in
**seconds**, not microseconds — `metrics.go` owns that conversion); `trace.go`
lists traces and reassembles a trace's spans into a parent/child waterfall.
`notify.go` emits the post-run update notice (`internal/update`).

## API client (`pkg/apiclient`)

`Client` is an interface (`client.go`); `apiClient` is the single
implementation. Methods are org-scoped (`/api/{org}/…`); `models.go` defines the
returned types.

- `doJSON` builds the request, applies the transport, and on a non-2xx response
  calls `httpError`, which classifies the status into a category and — for 403 —
  attaches RBAC-aware guidance (the common "service account has no role" case).
- **Lenient decoding where the server drifts.** `Org` decodes into a raw map and
  extracts only the fields the CLI relies on, then re-emits a lean curated
  projection — so org fields that change JSON type across OpenObserve
  versions/editions (e.g. `plan` as a number vs string) never break the response.
- `factory.go`'s `Build` normalizes the base URL and constructs the transport
  with the auth decorator. `readonly.go` is the read-only wrapper (see
  [read-only-mode.md](read-only-mode.md)).

The search request carries `start_time`/`end_time` as **microseconds**; this is
the single most error-prone part of the API, so `timeutil` owns the conversion
and the CLI never asks an agent to compute epochs.

## Transport (`pkg/transport`)

A thin `Client` that applies request decorators (auth, user-agent) and retries
transient failures. Retries are limited to idempotent methods (GET/HEAD); a
`Retry-After` header on 429/503 takes precedence over linear backoff. `Doer` is
an interface so tests inject fakes.

## Error model (`pkg/errors`)

Every failure is a `*CLIError` with `Category`, a stable `Code`, `Message`,
`Hint`, `NextSteps`, `Retryable` and `HTTPStatus`. The category drives two
deterministic mappings: the **exit code** (`codes.go`, 0–11) and the **default
guidance** (`hints.go`). `FromHTTPStatus` classifies HTTP statuses. The JSON
`Payload` is what's written to stderr. This is the "errors as navigation"
contract — every failure tells an agent the next command to run.

## Output (`internal/output`)

`Emit` and `EmitList` render any value as `json` (default), `table`, or `ndjson`.
Lists always use the `{items, next, has_more}` envelope. `--fields` projects
results to dot-path keys before rendering (filtering happens before it reaches an
agent's context). `--pretty` enables ANSI-colored JSON on a TTY (and is silently
downgraded to plain JSON off a TTY, so `--pretty | jq` still works).

## Configuration (`pkg/config` + `internal/config`)

The on-disk YAML model — the file schema (named contexts + `current_context` +
shared defaults), file IO, and context helpers — lives in the public
**`pkg/config`** so external consumers (e.g. the o3 desktop GUI) read and write
the same file. The CLI-only **layered loader** stays in `internal/config`:
resolution runs highest precedence first — **flags → env (`OPENOBSERVE_*`) →
`.env` → YAML config file → built-in defaults** — and each field's provenance is
tracked so `config show` can report where a value came from. Secrets
(passwords, tokens) are never written to the YAML file. Contexts are
kubectl-style; `--use-context` and `OPENOBSERVE_CONTEXT` override per invocation.

## Auth (`pkg/auth` + `internal/auth`)

The pure, dependency-light credential model lives in the public **`pkg/auth`**:
`Credential` with header construction, validation, account keying, and the
`transport.Decorator` it becomes. Two schemes: `basic` (email + password →
`Authorization: Basic base64(email:pw)`) and `token` (a pre-generated credential
sent verbatim, or wrapped as `Basic`). The config/keychain-coupled resolution
stays in `internal/auth`: `Resolve` produces a validated `Credential` from
config + secrets, loading the secret from the keychain when not supplied via
flags/env; the `Store` prefers the OS keychain (`go-keyring`) and falls back to a
`0600` JSON file. `internal/auth` re-exports the moved symbols so existing
callers compile unchanged.

## Time (`internal/timeutil`)

`Range.Resolve` turns `--since` / `--from` / `--to` into start/end microsecond
timestamps, accepting durations (`15m`, `1h`, `7d`), `now±duration`, RFC3339,
bare dates, and magnitude-detected epochs (s / ms / µs). It validates that the
window is non-empty and correctly ordered.

## Skill embedding (`assets.go`)

The companion Skill is embedded with `//go:embed all:skills/openobserve`, so a
binary always ships a Skill matching its version. `skill install` detects agent
directories and writes the tree. A test (`assets_test.go`) guards the Skill
`description` against Codex's 1024-character limit.

## Generated reference (`cmd/gen-docs`)

Walks the live cobra tree (`app.NewRootCmd`) and emits `docs/cli/index.html`
(styled, sidebar-grouped, served by Pages) and `docs/cli/README.md` (a
module-grouped table). Because both come from the command tree, they can't drift
from `--help`; CI fails if the committed output is stale.

## Testing

- Unit tests cover `timeutil`, the error mappings, output projection, the
  search SQL builders, and the API client against an `httptest` server
  (including the lenient org decode and the 403 RBAC guidance).
- `scripts/e2e.sh` runs the built binary against `test/mockserver` and asserts
  the agent-facing contract (JSON output, structured errors, exit codes) with no
  real credentials.
