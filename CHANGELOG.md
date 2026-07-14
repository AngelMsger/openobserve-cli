# Changelog

All notable changes to `openobserve-cli` are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.7.0] - 2026-07-14

### Added

- Credential-resolution failures now include an optional machine-readable
  `recovery` action for Agent hosts. When the user's home or OS keychain is not
  visible, the CLI requests one retry in host scope; `doctor` also reports a
  per-check `status` and `recovery_scope`.

### Fixed

- Keychain access failures are no longer collapsed into `AUTH_NO_TOKEN` and no
  longer steer sandboxed agents toward re-running `config init`. Browser-session
  recovery keeps o3 as the only refresh path after a host retry confirms the
  session is genuinely missing.
- Local cross-build artifacts under `dist/` are now ignored and removed by
  `make clean`, keeping release preparation from dirtying the worktree.

## [0.6.0] - 2026-07-12

### Added

- **The public auth package now supports browser-captured sessions shared with
  o3.** A `session` credential replays captured cookies and an optional
  Authorization fallback, allowing the CLI and desktop GUI to use the same
  context and keychain entry without requiring a service account.

### Fixed

- **Invalid captured sessions can no longer pass validation and produce an
  anonymous request.** Empty, malformed, cookie-less, and header-injection
  session values now fail with `AUTH_BAD_SESSION`; request decoration also fails
  closed if validation was skipped by a library caller.
- **CLI credential flows now explain that browser sessions are managed by o3.**
  Editing such a context with `config init` or running `auth login` returns the
  actionable `SESSION_BROWSER_MANAGED` error instead of falling through to the
  basic/token wizard and failing with `BAD_SCHEME`.

## [0.5.0] - 2026-06-29

### Added

- **The CLI now flags which context your commands will hit when several are
  configured.** A config can hold multiple named contexts, but an agent shelling
  out usually has no idea more than one exists — when none is selected
  explicitly it silently uses the saved `current_context` and can query the
  wrong O2 instance or org. Now, gated on `>1` context (single-context setups see
  nothing): `--help` ends with the active context, the full list, and how it was
  selected; and a real command run emits a structured `_notice` on stderr when
  the active context was chosen implicitly, so the ambiguity is visible before
  results are trusted. The notice self-silences once a context is selected
  explicitly (`--use-context` or `OPENOBSERVE_CONTEXT`); opt out entirely with
  `OPENOBSERVE_CLI_NO_CONTEXT_HINT=1`.

## [0.4.0] - 2026-06-28

### Fixed

- **An unknown subcommand of a command group no longer looks like success.** A
  typo such as `config use-contexts` (for `config use-context`) printed the group
  help to stdout and exited `0`, so an agent or script read it as a successful
  no-op. Cobra flags unknown commands only at the root; a nested non-runnable
  group instead falls through to help-and-exit-0. Every group (`config`, `org`,
  `search`, `metrics`, `trace`, `auth`, `stream`, `skill`) now returns a
  structured `UNKNOWN_COMMAND` usage error on stderr with exit code 2 and a
  "Did you mean" suggestion; a bare group invocation still prints help.

### Changed

- **The credential and config-file models are now importable too.** Following
  the API-client move in 0.3.0, the pure credential model moved to `pkg/auth`
  (`Credential`, header construction, validation, account keying, and the
  `transport.Decorator` it becomes) and the on-disk YAML config-file model moved
  to `pkg/config` (the file schema with named contexts, file IO, and context
  helpers). An external consumer — e.g. the o3 desktop GUI — can now build
  authenticated requests and read/write the same config file without
  reimplementing either. The CLI-only layered loader (flags/env/file) and the
  keychain/secret resolution stay in `internal/config` and `internal/auth`,
  which re-export the moved symbols so all existing call sites compile unchanged.
  No CLI behavior change — a package-path move plus documentation.

## [0.3.0] - 2026-06-25

### Added

- **The API client is now an importable Go library.** The HTTP client that
  powers the CLI moved out of `internal/` into `pkg/` (`pkg/apiclient`, `pkg/transport` and `pkg/errors`), so external
  Go projects — e.g. a GUI — can import and reuse it: the `Client` interface, the
  `Build` factory, the normalized models and the structured `*errors.CLIError`
  values. See the "Use as a Go library" section in the README. No CLI behavior
  change — a package-path move plus documentation.

## [0.2.4] - 2026-06-25

### Fixed

- **The companion Skill drifted out of sync with the CLI.** The agent-facing
  Skill (`skills/openobserve/`) — read by coding agents instead of `--help` —
  did not document the `{"_notice":{"update":{…}}}` stderr notice (only the
  `skill` one), so agents met an unexplained notice; nor did it list the `skill`
  command group, `config contexts` / `use-context`, the `--format table` output,
  or `--use-context`. All are now documented, and an AGENTS.md rule requires the
  Skill to be updated in lockstep with the CLI. (Skill content only — no behavior
  change.)

## [0.2.3] - 2026-06-24

### Fixed

- **The "update available" notice was suppressed on failed commands.** It was
  emitted from a `PersistentPostRunE`, which cobra runs only after a command
  succeeds — so a command that errored never surfaced the notice, even when a
  newer release existed. It now fires from `Execute` after the command runs, on
  success and failure alike. The stderr-only delivery, the skip list, and the
  `OPENOBSERVE_CLI_NO_UPDATE_NOTIFIER` opt-out are unchanged.

## [0.2.2] - 2026-06-24

### Added

- **Companion-Skill discovery for agents.** Agents sometimes shell out to this
  CLI without loading the `openobserve` Skill, bypassing the usage recipes and
  query guidance it maintains. The root `--help` now carries an `AGENT NOTE`
  pointing at the Skill; `openobserve-cli skill status` reports whether the Skill
  is loaded (via the `OPENOBSERVE_CLI_SKILL` handshake) and installed; and any
  real command run non-interactively without that handshake prints a one-line
  `{"_notice":{"skill":…}}` hint to **stderr** (stdout stays clean). The hint is
  silent for humans (TTY), self-silences once the Skill sets
  `OPENOBSERVE_CLI_SKILL=1`, and can be turned off with `OPENOBSERVE_CLI_NO_SKILL_HINT=1`.

## [0.2.1] - 2026-06-16

### Changed

- **`config init` now handles an existing configuration like the sibling CLIs.**
  When a config already exists it lists the contexts and asks whether to **edit**
  one (prompts prefilled from it, other contexts kept), **add** a new one, or
  **replace** everything — instead of silently overwriting the `default` context.
  `config init --context <name>` remains a non-interactive shortcut that targets
  a context directly and skips the prompt. Works in both the `--pretty` TUI and
  the plain (pipe-friendly) wizard.

## [0.2.0] - 2026-06-16

Completes the three pillars — metrics and traces become first-class — and adds
the highest-value query ergonomics. Still read-only.

### Added

- **Metrics (PromQL).** `metrics query` evaluates a PromQL expression at an
  instant; `metrics query-range` evaluates it across a window at `--step`
  resolution. Times are converted to the seconds the Prometheus-compatible API
  expects (PromQL uses seconds, not the microseconds `_search` needs). A bad
  expression returns a structured `PROMQL_ERROR` pointing at
  `stream list --type metrics`. Metrics are no longer queried awkwardly as SQL.
- **Traces (first-class).** `trace search` lists recent traces (trace_id,
  duration, services) — the map for finding a slow or erroring request; `trace
  get <trace_id>` reassembles every span into a parent/child waterfall with each
  span's offset from the trace start. JSON returns a nested tree; `--format
  ndjson` streams the spans flat. Parent linkage and span fields are read
  defensively, so orphaned spans surface as roots rather than disappearing.
- **Live tail.** `search tail` follows a stream like `tail -f`, polling on
  `--interval` and printing new rows as ndjson until interrupted; `--since`
  backfills a window first.
- **Large / awkward inputs.** `--sql` (and `metrics --query`, `trace --filter`)
  accept `@file` / `@-` to read from a file or stdin. `search run --all` pages
  through every matching row as ndjson, bounded by `--max` (truncation is
  announced on stderr — never silent).
- **Runtime update notice.** After a successful command, a one-line
  `{"_notice":{"update":…}}` is emitted on stderr when a newer release is
  available (24h cached; bounded; never fails the command). Silence it with
  `OPENOBSERVE_CLI_NO_UPDATE_NOTIFIER`; `doctor` also reports update status and
  takes `--no-update-check`.

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

[Unreleased]: https://github.com/AngelMsger/openobserve-cli/compare/v0.7.0...HEAD
[0.7.0]: https://github.com/AngelMsger/openobserve-cli/compare/v0.6.0...v0.7.0
[0.6.0]: https://github.com/AngelMsger/openobserve-cli/compare/v0.5.0...v0.6.0
[0.5.0]: https://github.com/AngelMsger/openobserve-cli/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/AngelMsger/openobserve-cli/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/AngelMsger/openobserve-cli/compare/v0.2.4...v0.3.0
[0.2.4]: https://github.com/AngelMsger/openobserve-cli/compare/v0.2.3...v0.2.4
[0.2.3]: https://github.com/AngelMsger/openobserve-cli/compare/v0.2.2...v0.2.3
[0.2.2]: https://github.com/AngelMsger/openobserve-cli/compare/v0.2.1...v0.2.2
[0.2.1]: https://github.com/AngelMsger/openobserve-cli/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/AngelMsger/openobserve-cli/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/AngelMsger/openobserve-cli/releases/tag/v0.1.0
