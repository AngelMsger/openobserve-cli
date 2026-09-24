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
`Hint`, `NextSteps`, `Retryable`, `HTTPStatus` and an optional structured
`Recovery`. The category drives two
deterministic mappings: the **exit code** (`codes.go`, 0–11) and the **default
guidance** (`hints.go`). `FromHTTPStatus` classifies HTTP statuses. The JSON
`Payload` is what's written to stderr. This is the "errors as navigation"
contract — every failure tells an agent the next command to run. `Recovery`
describes an environment change, such as retrying the same command in host
scope; it is deliberately separate from `Retryable`, which means a retry in the
current environment may work. `doctor` exposes the same distinction through
per-check `status` and optional `recovery_scope`.

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
`transport.Decorator` it becomes. Three schemes are supported: `basic` (email +
password → `Authorization: Basic base64(email:pw)`), `token` (a pre-generated
credential sent verbatim, or wrapped as `Basic`), and `session` (browser-captured
cookies with an optional Authorization fallback, established by `auth login
--browser` here or by o3's native sign-in — both write the same keychain
entry). The config/keychain-coupled resolution stays in `internal/auth`: `Resolve`
produces a validated `Credential` from config + secrets, loading the secret from
the keychain when not supplied via flags/env; the `Store` prefers the OS keychain
(`go-keyring`) and falls back to a per-user DPAPI-encrypted file on Windows or
a `0600` JSON file on macOS/Linux. `internal/auth` re-exports
the moved symbols so existing callers compile unchanged. Store access errors are
preserved rather than collapsed into "missing":
`CREDENTIAL_STORE_INACCESSIBLE` and `CREDENTIAL_NOT_VISIBLE_OR_MISSING` carry a
host-scope recovery instruction so an Agent host can retry before asking the
user to reconfigure credentials.

## Browser sign-in (`pkg/webauth` + `pkg/webauth/cdp`)

The pure capture core lives in **`pkg/webauth`**: cookie shaping and host
scoping, the `LoginSucceeded` heuristic, the injected capture script
(`ProbeJS`), and the `Tracker` that decides when a captured state counts as a
completed login. It imports only the standard library and `pkg/auth`, so the o3
desktop app shares it.

The transport is the `Driver` seam. **`pkg/webauth/cdp`** implements it by
launching a Chromium-family browser with `--remote-debugging-port=0` and
speaking a small subset of the DevTools Protocol over one WebSocket; o3 keeps a
second implementation over a native WKWebView on macOS and uses the CDP driver
elsewhere.

An authenticated API request is the sole success signal
(`webauth.PingVerifier`): only a real authenticated response distinguishes a
completed login from a benign login-page cookie or an in-progress redirect to an
external identity provider.

## Time (`internal/timeutil`)

`Range.Resolve` turns `--since` / `--from` / `--to` into start/end microsecond
timestamps, accepting durations (`15m`, `1h`, `7d`), `now±duration`, RFC3339,
bare dates, and magnitude-detected epochs (s / ms / µs). It validates that the
window is non-empty and correctly ordered.

## Skill embedding (`assets.go`)

The companion Skill is embedded with `//go:embed all:skills/openobserve`, so a
binary always ships a Skill matching its version. A test (`assets_test.go`)
guards the Skill `description` against Codex's 1024-character limit.

`skill install` uses an agent path table (`agentSpecs` in
`internal/app/skill.go`) mapping each agent to its global / project skills
directory and probe markers: Claude Code uses `~/.claude/skills` and `./.claude/skills`; Codex uses `~/.codex/skills` and `./.agents/skills`; Cursor uses `~/.cursor/skills` and `./.cursor/skills`; the shared Agents tree uses `~/.agents/skills` and `./.agents/skills`; Gemini CLI uses `~/.gemini/skills` and `./.gemini/skills`; GitHub Copilot uses `~/.copilot/skills` and `./.agents/skills`; OpenCode uses `~/.config/opencode/skills` and `./.opencode/skills`; Continue uses `~/.continue/skills` and `./.continue/skills`; Windsurf uses `~/.codeium/windsurf/skills` and `./.windsurf/skills`; Grok Build uses `~/.grok/skills` and `./.grok/skills`; Pi uses `~/.pi/agent/skills` and `./.pi/skills`; Kilo Code uses `~/.kilocode/skills` and `./.kilocode/skills`; Roo Code uses `~/.roo/skills` and `./.roo/skills`. With no flag it
probes which directories exist and installs / removes for each hit;
`--agent` selects explicitly; `--dir` is the agent-agnostic explicit path.

The Skill handshake carries its version, not a boolean. Runtime hints compare
the loaded version with the embedded copy; `skill status` also reads deployed
`SKILL.md` frontmatter and classifies each installation as current, outdated,
or unknown. Update notices return ordered steps to upgrade the CLI, refresh the
Skill, and reload the agent context. `doctor` reports Skill state as an
informational check that does not change connectivity health.

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

## Team service setup and login persistence

`internal/config/setup.go` owns offline service-field validation, acquisition
links and the pure `PlanServiceContext` merge used by execution and dry-run.
`LoadOptions.Setup` selects the requested destination even when it is new and
filters personal environment fields before credential-based scheme inference.
Ordinary runtime precedence and transient environment secrets remain supported.
`AuthConfig.CredentialURL` is additive, non-secret, and serialized through every
config shape; OpenObserve exposes it in the public `pkg/config` model.

`internal/app/setup.go` wires `config set-context`, `auth guide`, and wizard
prefill. `internal/app/auth_login.go` checks the complete normalized service URL,
verifies authentication, stores the secret, and persists the associated identity.
Expected storage failures preserve their causes and distinguish a stored secret
from a completed login. Error payloads may include optional non-secret `details`;
context conflicts use field-level `before`/`after` values. Setup does not access a
credential store and is available under the remote read-only posture.

The end-to-end setup harness (`scripts/e2e-setup.sh`, part of `make e2e`) runs
without a service or user credentials. Unit tests cover target-specific
precedence, persistence failures, guide URLs, and reloading a stored login.
See [installation](installation.md#team-distribution-and-personal-login) for the
canonical user-facing contract and product-specific authentication guidance.

### Existing credential reuse

`internal/app/auth_reuse.go` matches stored contexts and verifies credentials
through native auth/client code before associating identity with the destination.
It never writes the credential store, consumes environment secrets or changes
current_context. Public `config set-context` remains offline and credential-free.
The destination's complete service identity and the captured config are checked
again before writing. See the installation guide for the result and recovery contract.

Equivalent service URL overrides preserve the persisted native credential lookup
key without redirecting requests or copying secrets. Logout removes that same
entry. A different complete deployment URL cannot use the retained lookup key.
