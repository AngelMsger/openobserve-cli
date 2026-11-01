# Agent Guide

This file orients coding agents (Claude Code and others) working in this
repository. It is intentionally short.

## Start here

1. Read [`CONTRIBUTING.md`](CONTRIBUTING.md) — project layout, the build/test/
   lint/docs commands, the coding conventions, and the commit/PR expectations
   every change must follow.
2. Then read, only as the task needs them, the docs under [`docs/`](docs/):
   [`technical-design.md`](docs/technical-design.md) (architecture and the
   `internal/` and `pkg/` packages — read before changing core behavior),
   [`installation.md`](docs/installation.md) (install / setup / distribution UX),
   [`read-only-mode.md`](docs/read-only-mode.md) (the write-safety posture), and
   [`releasing.md`](docs/releasing.md) (versioning, tagging, the release/CI
   workflows — read before cutting a release or touching `.github/workflows/`).

## What this is

`openobserve-cli` is an agent-facing CLI for OpenObserve (O2): discover streams
and run SQL searches over logs / metrics / traces. It is a Go + Cobra CLI that
mirrors the architecture of the sibling `confluence-cli` / `bitbucket-cli`.

## Layout

- `cmd/openobserve-cli` — entry point; `cmd/gen-docs` — CLI reference generator.
- `internal/app` — one file per noun (org, stream, search, auth, config, doctor,
  skill); `root.go` assembles the tree; `context.go` holds the shared `appState`.
- `pkg/apiclient` — the OpenObserve HTTP surface and models.
- `pkg/errors` — re-exports the shared `CLIError` model and exit-code map (0–11)
  and installs openobserve-cli's hints (`hints.go`).
- `pkg/transport` — re-exports the shared retrying HTTP client with
  openobserve-cli's User-Agent.
- `pkg/credstore` — re-exports the shared credential store under the
  `openobserve-cli` keychain service.
- `pkg/auth`, `pkg/config` — the public credential model and on-disk YAML
  config model, shared with external consumers (see the rule below). `pkg/auth`
  declares openobserve-cli's auth schemes and recovery wording on top of the
  shared credential model. `pkg/config` registers openobserve-cli's settings,
  file location and wording with the shared config engine (`schema.go`) and
  keeps the YAML shape of `config.yaml`.
- `internal/config`, `internal/auth` — CLI-only layered loader + keychain
  resolution, built over `pkg/config` / `pkg/auth` (which they re-export);
  `internal/auth` maps config onto the shared resolver.
- Output rendering, argv correction, the update check, the credential store,
  credential resolution and the configuration engine come from [agent-cli-kit](https://github.com/AngelMsger/agent-cli-kit), shared
  by every CLI in the family. Fix bugs in those there, not here.
- `internal/timeutil` — human time ranges → microsecond epochs (the search API).
- `skills/openobserve` — the companion Skill, embedded into the binary.

## `pkg/` is a public library — the family reference for GUI reuse

Everything under `pkg/` is an importable Go library with a **stable exported
contract**, not just CLI internals. `openobserve-cli` is the family's reference
for how a sibling CLI (`bitbucket-cli` / `confluence-cli` / `jenkins-cli`)
exposes itself to a GUI:

- `pkg/apiclient` (the `Client` interface, `Build` factory, normalized models),
  `pkg/transport`, `pkg/errors`, `pkg/constants` mirror the siblings exactly.
- `pkg/auth` (credential model → `transport.Decorator`) and `pkg/config` (the
  config-file model + IO) are **additions** a GUI needs. They hold the pure,
  dependency-light core; the CLI-only loader/keychain wiring stays in `internal/`
  and re-exports the moved symbols so call sites never change. When a sibling
  grows a GUI, replicate this split here — do not invent a different shape.
- Extend the `pkg/` surface **additively**; keep existing shapes and behavior
  stable. Read the package doc comment (`go doc ./pkg/<name>`) before changing it.

## Ground rules

- Run `make test` and `make build` before claiming a change is complete.
- Credential fallback tests (in agent-cli-kit's `credstore`) assert `0600` only
  on non-Windows platforms. Windows tests must verify DPAPI-encrypted on-disk
  data and a store round trip; Windows `FileMode` does not represent POSIX
  access permissions.
- stdout is data only; errors / notices / `--verbose` go to stderr.
- Keep paginated NDJSON rows on stdout and continuation in a structured
  `_notice.pagination` record on stderr after successful rendering, including
  filtered empty pages. Keep the real continuation flag in table and notice
  guidance; projections never remove pagination metadata.
  Search/trace continuations use `--offset` and must advance; empty backend
  pages never supply continuation. SQL search may make one single-row lookahead
  for a full page with a page-local total, preserving its SQL and resolved time
  window. Keep JSON summaries and `--all` streaming/cap notices unchanged.
  An offset is only stable over a fixed window: keep the "same query, absolute
  `--from` and `--to`" guidance in help text and the Skill, resolve a relative
  window once per `--all` traversal, and keep resolved bounds reusable as flags.
- Time-window flags follow the family contract: `--since` excludes `--from` and
  `--to`, and `--to` requires `--from`. Enforce it in `internal/timeutil`
  (`Range.Resolve`) so every command shares one rule; never let one flag
  silently win, and add a new time-filtered command through `timeFlags`.
- Keep query and monitoring guidance in the companion Skill: reuse verified
  identifiers, bound retrieval and follow loops, and report evidence with its
  time window. Execute shell recovery examples with failing stubs when editing them.
- Never commit credentials, `.env`, or build artifacts.

## Discoverability — no dead-end inputs

**Every identifier a command accepts as input must be discoverable through
another command in this CLI.** A stream name → `stream list`. A column name →
`stream schema`. An org → `org list`. When you add a command or flag that takes a
new kind of input, also provide (or point its error `next_steps` at) the command
that lists values of that kind.

## When extending (P1)

Write commands (alerts; later dashboards, functions/pipelines, users, ingest)
must:
add `--dry-run` (emit the would-be request plan), require `--yes` for destructive
ops, route through `apiclient.NewReadOnly` (override the new method to return
`READONLY_BLOCKED`), and keep the `{items,next,has_more}` + structured-error
contract.

**Keep the companion Skill in sync — it is the agent-facing source of truth.**
Any new command, subcommand, flag, or alias must be reflected in the embedded
Skill ([`skills/openobserve/`](skills/openobserve/): the `SKILL.md` `## Commands`
list and the relevant `references/` file) **in the same commit**. Agents read the
Skill instead of `--help`, so a capability it omits effectively does not exist
for them; a flag whose help text points at another command must have that command
listed in the Skill, and no Skill claim may contradict the code.

## Team setup contract

Keep service presets separate from personal credentials. `config set-context`
must resolve the named destination (including a new one), ignore personal
variables before scheme inference, remain offline and credential-store-free,
and share one merge for dry-run and execution. Preserve other contexts, personal
usernames and shared defaults. Login must persist its identity as well as its
secret and reject a different complete service URL before credential writes.
Use the common acquisition guide in all prompt styles and missing-credential
recovery; never request a guide URL with credentials. Cover a fresh config reload,
conflict/idempotent setup, and partial persistence failures when changing this
flow. The canonical behavior is in the installation guide's team setup section.

## Credential reuse

Keep `auth reuse` separate from public service setup. The decision belongs to
agent-cli-kit's `reuse` package; this repository only describes its contexts,
resolves, verifies and writes. Match complete URLs and organization before
credential access, prefer a long-lived credential to a browser session, never
replace a working login automatically, and retain operational failures. A
username is an identity only under `basic`. Never copy secrets, infer identity
from environment variables or activate a context. Cover dry-run, ambiguity, scope
mismatch, concurrent edits, fresh-load credential resolution and the browser
sign-in confirmation (`BROWSER_LOGIN_REPLACES_LOGIN`). Native self-configuration
follows the existing read-only exception.

## Stored credential lifetime

A stored secret is keyed by the server URL's host and the auth scheme
(`auth.AccountKey`), so contexts on one host resolve the same entry regardless of
organization, path or trailing slash; a browser session is keyed under the
`session` scheme, apart from a password or token on the same host. `config init`
only saves. `config delete-context` and `auth logout` are the paths that remove
one: `delete-context` forgets the deleted context's secret only when no
remaining context resolves the same key (`forgetUnusedCredential`, as in the
Atlassian CLIs), and only after the config write. A change that deletes or
re-homes a context must compare credential keys, not URL strings, and keep an
entry another context still resolves — including the one it has just saved.
Cover it with a fresh-load resolution test
(`internal/app/config_credentials_test.go`), which also pins that
`delete-context` keeps a secret or session another context resolves.
