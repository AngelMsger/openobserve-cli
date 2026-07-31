# Native browser login for `openobserve-cli`

Date: 2026-07-31
Status: approved, not yet implemented
Repos touched: `openobserve-cli` (owner), `o3` (consumer)

## Problem

`openobserve-cli auth login` supports two schemes natively: `basic` (email +
password) and `token`. A third scheme, `session`, already exists in `pkg/auth` —
browser-captured cookies plus an optional `Authorization` fallback — but the CLI
can only *replay* such a session, never *establish* one. Capture lives in the o3
desktop app, which drives a native WKWebView. A context on `session` therefore
fails both `auth login` and `config init` with `SESSION_BROWSER_MANAGED`,
directing the user to o3.

That has two costs:

1. **The CLI is unusable standalone on SSO instances.** Where basic/token auth
   is not an option, the only path to a working CLI runs through installing a
   macOS GUI app.
2. **The capture logic cannot be shared.** The decisive part — a JS hook on
   `fetch` and `XMLHttpRequest.setRequestHeader` that captures the durable
   `Authorization` header the SPA sends — is an Objective-C string literal in
   `webauth_darwin.m`. A fix there can never reach the CLI.

## Goals

- `openobserve-cli auth login --browser` establishes a `session` credential on
  macOS, Linux and Windows, on a desktop with a Chromium-family browser.
- The capture heuristic, the injected JS, the success policy and the
  verify-by-Ping contract exist **once**, shared by the CLI and o3.
- o3 keeps its native WKWebView window on macOS (no regression), and gains real
  browser sign-in on Windows and Linux, where it currently returns
  "browser sign-in is only supported on macOS".
- Both clients write the same keychain entry, so signing in through either one
  leaves the other authenticated.

## Non-goals

- Headless or over-SSH sign-in. `--browser` requires a local GUI browser and
  fails with an actionable error otherwise; basic/token remain the answer for
  agent sandboxes and CI.
- Replacing o3's WKWebView with CDP. A desktop app should not require Chrome.
- Any change to how a `session` credential is stored, replayed or validated.
  `pkg/auth`'s `Session`, `ParseSession` and `Decorator` are unchanged.

## Architecture

The existing seam is the right one and is kept:

```go
Capture(loginURL, host string, verify VerifyFunc) (auth.Session, error)
```

Everything else sorts into *pure policy* (shared) or *transport* (per-client).

### New packages in `openobserve-cli`

```
pkg/webauth/                  pure, dependency-light — the AGENTS.md pkg/ rule holds
  session.go                  cookie shaping, host scoping, expiry, LoginSucceeded
  capture.go                  probe decoding, AssembleSession, Replayable
  probe.go                    the injected JS as a Go const            [NEW]
  tracker.go                  replayable/in-flight/unchanged policy    [NEW]
  driver.go                   Driver interface, VerifyFunc, PingVerifier [NEW]

pkg/webauth/cdp/              the CLI's transport; imports coder/websocket
  launch.go                   browser discovery, spawn, DevToolsActivePort
  conn.go                     minimal CDP client (id -> response mux)
  capture.go                  implements webauth.Driver

internal/app/
  auth_browser.go             `auth login --browser` wiring
```

`session.go` and `capture.go` move from o3's `internal/webauth` **verbatim**,
with their existing tests. They have no platform dependencies today; the move is
mechanical and the tests are the regression net for it.

A consumer importing only `pkg/auth` never imports `pkg/webauth/cdp`, so the
WebSocket dependency is listed in `go.mod` but linked into nothing that does not
ask for it. `pkg/webauth` itself stays dependency-free.

### Changes in `o3`

`internal/webauth/` is deleted. `webauth_darwin.go` moves up into the app as o3's
own thin shell implementing `webauth.Driver` over the shared core.
`webauth_other.go` stops returning an error and returns `cdp.New()`.
`sessionVerifier` in `app.go` is deleted in favour of `webauth.PingVerifier`.

## Components

### `probe.go` — the shared JS

The fetch/XHR hook is lifted out of `webauth_darwin.m` into a Go const,
parameterised by the name of its delivery function:

```go
// ProbeJS returns the capture script, delivering observations by calling the
// global function named deliver with {authorization?, email?}.
func ProbeJS(deliver string) string
```

Each transport supplies a one-line shim binding that name: WebKit maps it to
`window.webkit.messageHandlers.o3.postMessage`, CDP to a `Runtime.addBinding`
callback. The hooks themselves — `window.fetch`, then
`XMLHttpRequest.prototype.setRequestHeader`, then `localStorage.user_info` for
the email — are identical in both.

Both hooks are required and neither is sufficient: OpenObserve's web app issues
requests through axios (XHR), so hooking `fetch` alone captured nothing on real
instances and left only the short-lived session cookie. This is recorded here
because it is the kind of detail a well-meaning simplification would delete.

CDP injects at document-start, versus WebKit's document-end. This is a strict
improvement — the hooks are installed before the SPA's first request rather than
racing it — and is the one intentional behavioural difference between the two
transports.

### `tracker.go` — the success policy

The `verifyBusy` / `verifySig` bookkeeping currently inside `webauthProbe` in
`webauth_darwin.go` is transport-independent policy stranded in a
platform-specific file. It becomes:

```go
type Tracker struct{ ... }
func NewTracker(host string, verify VerifyFunc) *Tracker
func (t *Tracker) Observe(cookies []Cookie, currentURL, authz, email string) (auth.Session, bool)
```

`Observe` assembles the session, returns early unless `Replayable`, skips while a
verification is in flight or the cookie+authz signature is unchanged, and reports
success only once `verify` returns true. With a nil verifier it falls back to the
pure `LoginSucceeded` heuristic, preserving today's test and off-darwin
behaviour.

The rule this encodes — *an authenticated API probe is the sole success signal* —
is what keeps a benign login-page cookie or an in-progress SSO redirect from
being mistaken for a completed login. Sharing it means the two clients cannot
diverge on when they believe login finished.

### `driver.go` — the seam and the verifier

```go
type Driver interface {
    Capture(loginURL, host string, verify VerifyFunc) (auth.Session, error)
}

func PingVerifier(baseURL, org string, d config.Defaults) VerifyFunc
```

`PingVerifier` is o3's `sessionVerifier` (`app.go:577`) moved unchanged. It is
built entirely from `pkg/auth` + `pkg/apiclient` and contains nothing
o3-specific.

### `pkg/webauth/cdp` — the CLI transport

**Discovery** probes, in order, `$OPENOBSERVE_BROWSER`, then the
platform's known Chrome / Chromium / Edge / Brave locations (`PATH` lookups on
Linux, `/Applications/*.app/Contents/MacOS/*` on macOS, `%ProgramFiles%` and
`%LOCALAPPDATA%` on Windows). No browser found is a hard, actionable error.

**Launch** spawns the browser with `--user-data-dir=<profile>`,
`--remote-debugging-port=0`, `--no-first-run`, `--no-default-browser-check` and
the login URL. Port 0 makes Chrome pick a free port and write it, with the
browser WebSocket path, into `<profile>/DevToolsActivePort`; the driver polls for
that file rather than guessing a port. Never `--headless` — the user must be able
to sign in.

**Profile.** A CLI-owned persistent profile at `browser-profile/` inside
`config.DefaultConfigDir()` — i.e. `~/.angelmsger/openobserve/browser-profile/`,
mode `0700`. This follows the flat, cross-platform convention the CLI already
uses for its config directory rather than introducing XDG or per-OS
application-support paths. Persistence matches
o3, whose `WKWebsiteDataStore` is persistent by default, and means an IdP
session survives between logins — re-running the full SSO dance on every
`auth login` is the friction that drives people back to pasting tokens.
`--fresh-profile` uses a temp directory discarded on exit.

The profile is a cookie jar on disk holding live IdP session material. It is
mode-`0700` and separate from the user's real browser profile, but it is a real
asset: `auth logout` removes it alongside the keychain entry.

**Capture loop.** Attach to the page target (`Target.attachToTarget` with
`flatten: true`), then `Runtime.enable`, `Page.enable`,
`Runtime.addBinding`, `Page.addScriptToEvaluateOnNewDocument`. Authorization and
email arrive as `Runtime.bindingCalled` events — the exact analogue of WebKit's
`messageHandlers`. A 1s ticker reads cookies via `Storage.getCookies`
(`Network.getAllCookies` as the fallback on older Chrome; the former is the
non-deprecated spelling) plus the current URL, and feeds `Tracker.Observe`.
Cookies come from the browser rather than `document.cookie`, so HttpOnly cookies
are captured — a capability the WebKit path does not have.

**Termination.** Success closes the browser and returns the session. The browser
process exiting (the user closed the window) is `BROWSER_SIGNIN_CANCELLED`. A
10 minute ceiling matches o3's.

## Data flow

1. `openobserve-cli auth login --browser`, or `config init` with
   `scheme: session`.
2. Resolve base URL + org from the active context; `NO_BASE_URL` if unset.
3. `verify := webauth.PingVerifier(base, org, defaults)`.
4. `driver := cdp.New(cdp.Options{ProfileDir: ..., Fresh: ...})`.
5. `sess, err := driver.Capture(base+"/web/login", host, verify)`.
6. `blob, _ := auth.EncodeSession(sess)`; save via `auth.Save` under
   `Credential{Scheme: session, Username: sess.Email, Secret: blob}` — the same
   keychain account key o3 writes.
7. Emit `{logged_in, base_url, org, scheme, email, expires_at, stored_in}`.

Step 6 is deliberately the existing `auth.Save` path with no new storage
concepts, which is what makes the two clients interchangeable.

## Error handling

New codes, following the existing `cerrors` shape with populated `next_steps`:

| Code | Cause | Next steps |
| --- | --- | --- |
| `BROWSER_NOT_FOUND` | no Chromium-family browser located | `auth login` (basic/token); set `OPENOBSERVE_BROWSER` |
| `BROWSER_LAUNCH_FAILED` | spawn or `DevToolsActivePort` timeout | `auth login`; `--verbose` |
| `BROWSER_NO_DISPLAY` | Linux, no `DISPLAY`/`WAYLAND_DISPLAY` | `auth login`; env vars |
| `BROWSER_SIGNIN_CANCELLED` | browser closed before success | retry; `auth login` |
| `BROWSER_SIGNIN_TIMEOUT` | 10 min elapsed | retry; `auth login` |

`SESSION_BROWSER_MANAGED` is **repointed, not deleted**. `auth login` and
`config init` on a `session` context stop directing the user to o3 and instead
offer `auth login --browser`. This is a user-visible behaviour change and needs a
CHANGELOG entry.

`AUTH_BAD_SESSION` and the `CREDENTIAL_STORE_*` codes are unchanged.

## Testing

- **The move.** `session_test.go` and `capture_test.go` (211 lines) move to
  `pkg/webauth` unchanged. They must pass without edits; any edit needed means
  the move was not mechanical.
- **`Tracker`.** Unit tests for: not replayable, verification in flight,
  unchanged signature, verify fails then succeeds, nil verifier falls back to
  `LoginSucceeded`.
- **CDP client.** The id→response mux and `bindingCalled` dispatch, tested
  against an `httptest` WebSocket server. No browser required.
- **End to end**, behind a `browser_e2e` build tag, skipped when no browser is
  found: an `httptest` server serving a fake OpenObserve SPA — a login page that
  sets `auth_tokens`, an XHR carrying `Authorization`, and an API that 401s
  without it — driven by a real headful Chrome. This is the only test that
  exercises the injected JS, which nothing tests today.
- **o3.** `webauth_e2e_test.go` is retained with imports retargeted.
- `make test` and `make build` in `openobserve-cli`; `make` in o3. Both must
  build, since `go.work` links them.

## Rollout

Ordered so every step leaves both repos building:

1. `openobserve-cli`: add `pkg/webauth` (pure core + `probe.go`, `tracker.go`,
   `driver.go`) with tests. o3 is untouched and still compiles against its own
   `internal/webauth`.
2. `openobserve-cli`: add `pkg/webauth/cdp`, the `--browser` flag, the new error
   codes, and the `SESSION_BROWSER_MANAGED` repoint.
3. `o3`: delete `internal/webauth`; the darwin shell implements `webauth.Driver`
   over the shared core; the non-darwin path returns `cdp.New()`; delete
   `sessionVerifier`.
4. `oa-cli`: bump the `src/openobserve-cli` submodule pointer.

Steps 1-2 ship independently of o3. Step 3 depends on step 1 being pushed, though
o3's `go.work` `replace` makes local development work immediately.

## Documentation

- `openobserve-cli`: `docs/technical-design.md` (the auth section's "established
  and managed by o3" is no longer true), `README.md`, `CHANGELOG.md`,
  `skills/openobserve/SKILL.md`, and the generated `docs/cli/` reference.
- `o3`: `README.md` architecture section and `CHANGELOG.md` — browser sign-in is
  no longer macOS-only.
