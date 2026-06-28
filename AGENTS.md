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
- `pkg/errors` — the `CLIError` model + exit-code map (0–11).
- `pkg/auth`, `pkg/config` — the public credential model and on-disk YAML
  config model, shared with external consumers (see the rule below).
- `internal/output` — JSON / table / ndjson rendering, `{items,next,has_more}`.
- `internal/config`, `internal/auth` — CLI-only layered loader + keychain
  resolution, built over `pkg/config` / `pkg/auth` (which they re-export).
- `internal/timeutil` — human time ranges → microsecond epochs (the search API).
- `skills/openobserve` — the companion Skill, embedded into the binary.

## `pkg/` is a public library — the family reference for GUI reuse

Everything under `pkg/` is an importable Go library with a **stable exported
contract**, not just CLI internals. The o3 desktop (OpenObserve desktop) imports
it directly, so `openobserve-cli` is the family's reference for how a sibling CLI
(`bitbucket-cli` / `confluence-cli` / `jenkins-cli`) exposes itself to a GUI:

- `pkg/apiclient` (the `Client` interface, `Build` factory, normalized models),
  `pkg/transport`, `pkg/errors`, `pkg/constants` mirror the siblings exactly.
- `pkg/auth` (credential model → `transport.Decorator`) and `pkg/config` (the
  config-file model + IO) are **additions** the GUI needs. They hold the pure,
  dependency-light core; the CLI-only loader/keychain wiring stays in `internal/`
  and re-exports the moved symbols so call sites never change. When a sibling
  grows a GUI, replicate this split here — do not invent a different shape.
- Extend the `pkg/` surface **additively**; keep existing shapes and behavior
  stable. Read the package doc comment (`go doc ./pkg/<name>`) before changing it.

## Ground rules

- Run `make test` and `make build` before claiming a change is complete.
- stdout is data only; errors / notices / `--verbose` go to stderr.
- Never commit credentials, `.env`, or build artifacts.

## Discoverability — no dead-end inputs

**Every identifier a command accepts as input must be discoverable through
another command in this CLI.** A stream name → `stream list`. A column name →
`stream schema`. An org → `org list`. When you add a command or flag that takes a
new kind of input, also provide (or point its error `next_steps` at) the command
that lists values of that kind.

## When extending (P1)

Write commands (dashboards, alerts, functions/pipelines, users, ingest) must:
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
