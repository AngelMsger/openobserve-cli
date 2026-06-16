# Contributing to openobserve-cli

Thanks for helping improve `openobserve-cli`. This guide covers the project
layout, the everyday commands, and the conventions every change must follow.

## Prerequisites

- Go 1.24+ (the version is pinned in `go.mod`).
- `make`, `bash`, and `curl` (for the end-to-end script).

## Build, test, lint

```bash
make build      # compile to ./bin/openobserve-cli
make test       # unit + httptest integration tests
make e2e        # build, then run against the in-repo mock server (no creds)
make lint       # gofmt + go vet
make docs       # regenerate docs/cli/ from the cobra command tree
make cross      # cross-compile every platform into dist/ (release only)
```

Run `make test` and `make e2e` before claiming a change is complete. CI
(`.github/workflows/ci.yml`) runs gofmt, `go vet`, a `docs/cli/` drift check,
the unit tests and the e2e suite on every push and PR.

## Project layout

```
cmd/openobserve-cli   entry point (delegates to internal/app)
cmd/gen-docs          generates docs/cli/ from the cobra tree
internal/app          one file per noun (org, stream, search, auth, config,
                      doctor, skill) + root wiring and the shared appState
internal/apiclient    the OpenObserve HTTP surface and models
internal/transport    retrying HTTP client with request decorators
internal/errors       the CLIError model and exit-code map (0–11)
internal/output       JSON / table / ndjson rendering, {items,next,has_more}
internal/config       layered configuration + named contexts
internal/auth         credential model + OS keychain storage
internal/timeutil     human time ranges → microsecond epochs
internal/cliflags     argv normalization (LLM slip correction)
pkg/constants         app name, defaults, build-time version vars
skills/openobserve    the companion Skill, embedded into the binary
```

See [docs/technical-design.md](docs/technical-design.md) for how these fit
together.

## Conventions

- **gofmt-clean.** CI fails otherwise. `make fmt` fixes it.
- **stdout is data only.** Errors, notices and `--verbose` diagnostics go to
  stderr; never print anything else to stdout.
- **Structured errors.** Every user-facing failure is a `*errors.CLIError` with a
  `category`, a stable `code`, a `hint` and `next_steps`. The category maps to an
  exit code in `internal/errors/codes.go`.
- **No dead-end inputs.** Every identifier a command accepts must be discoverable
  through another command (a stream from `stream list`, a column from
  `stream schema`, an org from `org list`). When you add an input, also provide —
  or point its error `next_steps` at — the command that lists values of that kind.
- **Keep the CLI reference in sync.** After changing a command or flag, run
  `make docs` and commit the regenerated `docs/cli/`.
- **Update the changelog.** Add a bullet under `[Unreleased]` in
  [CHANGELOG.md](CHANGELOG.md) for any user-visible change.
- **Never commit** credentials, `.env`, `dist/`, `bin/`, or build artifacts.

### Adding a write command (post-v0.1)

v0.1 is read-only. Write commands (dashboards, alerts, functions, users,
ingest) must: support `--dry-run` (emit the would-be request plan), require
`--yes` for destructive operations, route through `apiclient.NewReadOnly` (which
must override the new method to return a `READONLY_BLOCKED` error), and keep the
`{items, next, has_more}` and structured-error contract. See
[docs/read-only-mode.md](docs/read-only-mode.md).

## Commits and pull requests

- Keep commits scoped to one logical change.
- Write imperative commit subjects (`fix: …`, `feat: …`, `docs: …`).
- PRs should describe the change and note how it was verified (`make test`,
  `make e2e`, a live check).
