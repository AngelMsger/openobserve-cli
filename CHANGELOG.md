# Changelog

All notable changes to `openobserve-cli` are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] - 2026-06-16

Initial release — a read-only, agent-facing CLI for OpenObserve (O2).

### Added

- **Stream discovery.** `stream list` (the high-signal map), `stream schema`,
  `stream get` and `stream stats` over logs / metrics / traces streams, so SQL
  only ever references real stream and column names.
- **SQL search.** `search run` builds a query from `--stream`/`--where`/`--order`
  or runs a full `--sql`; `search histogram` returns time-bucketed counts (the
  volume map) before pulling rows. Human time ranges — `--since 1h`, `--from`/
  `--to` as RFC3339, a date, an epoch (seconds/millis/micros) or `now-30m` — are
  converted to the microsecond timestamps the API requires.
- **Organizations.** `org list` discovers identifiers; `org use` sets the default
  one. Org objects are decoded leniently (into a raw map) so fields that vary in
  type across OpenObserve versions/editions — e.g. `plan` as a number — never
  break the response; output is a lean curated projection.
- **Auth.** Email + password (Basic) or a pre-generated token, stored in the OS
  keychain with a `0600` file fallback, plus `OPENOBSERVE_*` environment
  passthrough for headless / agent use. SSO (dex / Authentik / Okta) is supported
  via a Service Account, and a 403 returns RBAC-aware `next_steps`.
- **Setup & diagnostics.** `config init` (interactive TUI with `--pretty`, plain
  line-by-line wizard otherwise — also works over a pipe), `config show` /
  `contexts` / `use-context`, multiple kubectl-style named contexts, `auth login`
  / `status` / `logout`, and `doctor`.
- **Agent-friendly output.** JSON by default, a `{items, next, has_more}` list
  envelope, `--format json|table|ndjson`, `--fields` projection, and structured
  errors (`category` / `code` / `hint` / `next_steps` / `retryable`) mapped to
  stable exit codes (0–11). Common LLM argv slips (`--streamName`, `--limit100`)
  are corrected before parsing and echoed as a `_notice` on stderr.
- **Read-only safety scaffolding.** `defaults.read_only` /
  `OPENOBSERVE_CLI_READ_ONLY` / `--allow-writes` are wired end-to-end, ready for
  the write commands planned after v0.1.
- **Companion Skill.** An `openobserve` Skill embedded in the binary and deployed
  with `skill install`, with `references/` covering getting-started (incl. SSO),
  searching, streams and errors.
- **Distribution.** npm (`@angelmsger/openobserve-cli`), `go install`, prebuilt
  release binaries and `make install`. A generated CLI reference
  (`docs/cli/`) and a GitHub Pages landing page.

[Unreleased]: https://github.com/AngelMsger/openobserve-cli/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/AngelMsger/openobserve-cli/releases/tag/v0.1.0
