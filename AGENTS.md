# Agent Guide

This file orients coding agents (Claude Code and others) working in this
repository. It is intentionally short.

## What this is

`openobserve-cli` is an agent-facing CLI for OpenObserve (O2): discover streams
and run SQL searches over logs / metrics / traces. It is a Go + Cobra CLI that
mirrors the architecture of the sibling `confluence-cli` / `bitbucket-cli`.

## Layout

- `cmd/openobserve-cli` — entry point; `cmd/gen-docs` — CLI reference generator.
- `internal/app` — one file per noun (org, stream, search, auth, config, doctor,
  skill); `root.go` assembles the tree; `context.go` holds the shared `appState`.
- `internal/apiclient` — the OpenObserve HTTP surface and models.
- `internal/errors` — the `CLIError` model + exit-code map (0–11).
- `internal/output` — JSON / table / ndjson rendering, `{items,next,has_more}`.
- `internal/config`, `internal/auth` — layered config + keychain credentials.
- `internal/timeutil` — human time ranges → microsecond epochs (the search API).
- `skills/openobserve` — the companion Skill, embedded into the binary.

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
