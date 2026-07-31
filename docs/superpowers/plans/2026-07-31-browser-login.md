# Native Browser Login Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `openobserve-cli auth login --browser`, which signs the user in through a real Chromium-family browser and stores the captured session — on macOS, Linux and Windows — using a capture core shared with the o3 desktop app.

**Architecture:** The pure capture policy (cookie shaping, the login-success heuristic, the injected JS, the verify-by-Ping contract) moves out of o3's `internal/webauth` into `openobserve-cli/pkg/webauth`, which has no dependencies. A new `pkg/webauth/cdp` implements the `webauth.Driver` seam by launching a browser with `--remote-debugging-port` and speaking a small subset of the Chrome DevTools Protocol over one WebSocket. o3 keeps its WKWebView window on macOS as a second, thinner `Driver`, and uses the CDP driver on Windows/Linux where it currently refuses to work.

**Tech Stack:** Go 1.24, cobra, `github.com/coder/websocket` (new), Chrome DevTools Protocol. The o3 side is Wails v2 + cgo/Objective-C.

**Spec:** `docs/superpowers/specs/2026-07-31-browser-login-design.md`

## Global Constraints

- The CLI builds with `CGO_ENABLED=0` (`Makefile:22`). Every new package in this repo must be pure Go. No cgo, ever.
- `pkg/webauth` must import nothing outside the standard library, `pkg/auth`, `pkg/apiclient` and `pkg/errors` — all of which are themselves stdlib-only. It must NOT import `pkg/config` (which pulls `yaml.v3`) and must NOT import the WebSocket library; that belongs to `pkg/webauth/cdp` alone. This is the `pkg/` rule in `AGENTS.md:49`.
- Extend the `pkg/` surface **additively**. Do not change existing exported shapes in `pkg/auth`, `pkg/config`, `pkg/apiclient` or `pkg/credstore`.
- stdout is data only. Errors, notices and `--verbose` output go to stderr.
- All errors use `cerrors` (`github.com/angelmsger/openobserve-cli/pkg/errors`) with a `Category`, a stable `CODE`, and populated `WithNextSteps(...)`.
- Config lives at `~/.angelmsger/openobserve/` via `config.DefaultConfigDir()`. Do not introduce XDG or per-OS application-support paths.
- Run `make test` and `make build` before claiming any task complete.
- Never commit credentials, `.env`, or build artifacts.
- Commit messages are English and end with the `Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>` trailer.
- Work happens on branch `claude/browser-login-session-capture` in the `openobserve-cli` repo at `~/Development/Workspaces/oa-cli/src/openobserve-cli`, except Task 11 which is in `~/Development/Workspaces/o3`.

## Orientation for someone new to this codebase

`openobserve-cli` talks to OpenObserve, a logs/metrics/traces backend. A user's
credential is a `pkg/auth.Credential` with one of three `Scheme` values:
`basic` (email + password), `token`, or `session`. A `session` credential's
`Secret` is a JSON envelope — `pkg/auth.Session` — holding a `Cookie` header
string and an optional `Authorization` header, captured from a real browser
after the user logs in. `Credential.Decorator()` replays those onto every
outgoing request.

Today the CLI can *replay* a session but not *create* one; only the o3 desktop
app can, using a native macOS WebView. This plan gives the CLI its own capture
path and makes both clients share the decision logic.

The critical, non-obvious domain fact: **OpenObserve's web app authenticates
itself with an `Authorization` header that its own JavaScript builds and sends
via XHR, and on some instances sets no cookies at all.** So capture must hook
both `window.fetch` *and* `XMLHttpRequest.prototype.setRequestHeader`. Hooking
`fetch` alone captures nothing on real instances. This is why the injected
JavaScript exists and why it looks the way it does.

## File Structure

**Created in `openobserve-cli`:**

| File | Responsibility |
| --- | --- |
| `pkg/webauth/session.go` | Cookie shaping: `Cookie`, `SerializeCookies`, `HostMatches`, `FilterForHost`, `EarliestExpiry`, `LoginSucceeded`. Moved from o3. |
| `pkg/webauth/capture.go` | `AssembleSession`, `Replayable`. Moved from o3. |
| `pkg/webauth/probe.go` | `ProbeJS(deliver string) string` — the injected capture script. |
| `pkg/webauth/tracker.go` | `Tracker` — when a capture counts as a completed login. |
| `pkg/webauth/driver.go` | `Driver` interface, `VerifyFunc`, `PingVerifier`. |
| `pkg/webauth/cdp/conn.go` | Minimal CDP client: one WebSocket, id→response mux, event dispatch. |
| `pkg/webauth/cdp/launch.go` | Browser discovery, spawn, `DevToolsActivePort` handshake. |
| `pkg/webauth/cdp/capture.go` | `Driver` implementation wiring launch + conn + tracker. |
| `internal/app/auth_browser.go` | `--browser` flag handling and the CLI-side error codes. |

**Modified in `openobserve-cli`:** `internal/app/auth.go`, `internal/app/config.go`, `go.mod`, `CHANGELOG.md`, `README.md`, `docs/technical-design.md`, `skills/openobserve/SKILL.md`.

**Deleted in `o3`:** `internal/webauth/session.go`, `internal/webauth/capture.go`, `internal/webauth/session_test.go`, `internal/webauth/capture_test.go`, `internal/webauth/webauth_other.go`.

### Deliberate refinement to the spec

The spec says `capture.go` moves verbatim. It does not: `nativeProbe`,
`nativeCookie` and `parseProbe` decode the JSON payload that o3's
Objective-C layer constructs. That is WebKit transport detail, not shared
policy, and the CDP driver never uses it. Those three symbols and
`TestParseProbe` stay in o3, moving into the retained darwin shell. Only
`AssembleSession` and `Replayable` move to `pkg/webauth`.

---

### Task 1: Move the pure capture core into `pkg/webauth`

This task is a mechanical relocation. Its whole value is that the moved tests
pass **without edits** — if you find yourself changing test logic, the move was
not mechanical and you have introduced a bug.

**Files:**
- Create: `pkg/webauth/session.go` (from `~/Development/Workspaces/o3/internal/webauth/session.go`)
- Create: `pkg/webauth/capture.go` (from o3's `capture.go`, partial)
- Test: `pkg/webauth/session_test.go`, `pkg/webauth/capture_test.go` (from o3)

**Interfaces:**
- Consumes: `pkg/auth.Session` (existing).
- Produces: `webauth.Cookie` struct; `type VerifyFunc func(auth.Session) bool`; `SerializeCookies([]Cookie) string`; `HostMatches(cookieDomain, host string) bool`; `FilterForHost([]Cookie, host string) []Cookie`; `EarliestExpiry([]Cookie) time.Time`; `LoginSucceeded(currentURL, host string, cookies []Cookie) bool`; `AssembleSession(cookies []Cookie, host, authorization, email string) auth.Session`; `Replayable(auth.Session) bool`.

- [ ] **Step 1: Copy the two source files and their tests**

```bash
cd ~/Development/Workspaces/oa-cli/src/openobserve-cli
mkdir -p pkg/webauth
O3=~/Development/Workspaces/o3/internal/webauth
cp "$O3/session.go"      pkg/webauth/session.go
cp "$O3/capture.go"      pkg/webauth/capture.go
cp "$O3/session_test.go" pkg/webauth/session_test.go
cp "$O3/capture_test.go" pkg/webauth/capture_test.go
```

- [ ] **Step 2: Strip the WebKit-only payload decoder out of `capture.go`**

In `pkg/webauth/capture.go`, delete the `nativeProbe` struct, the
`nativeCookie` struct, the `parseProbe` function, and the now-unused
`encoding/json` import.

What must remain in `capture.go`: `VerifyFunc` (keep it here, where it already
lives — Task 4's `driver.go` must not redeclare it), `Replayable` and
`AssembleSession`, with their doc comments intact. The resulting import block is
exactly:

```go
import (
	"strings"

	pkgauth "github.com/angelmsger/openobserve-cli/pkg/auth"
)
```

Correspondingly delete `TestParseProbe` and `TestProbeFromNativeLoginInstance`
from `pkg/webauth/capture_test.go` — both exercise `parseProbe`. They stay
behind in o3 (Task 11).

- [ ] **Step 3: Fix the package doc comment in `session.go`**

Replace the existing package comment at the top of `pkg/webauth/session.go`
with one that describes the shared package rather than o3:

```go
// Package webauth holds the browser sign-in capture core shared by
// openobserve-cli and the o3 desktop app: cookie shaping, the login-success
// heuristic, the injected capture script, and the policy deciding when a
// captured state counts as a completed login.
//
// It is pure and dependency-light — it imports only the standard library and
// pkg/auth — so both a CLI driving a browser over the DevTools Protocol and a
// desktop app driving a native WebView can share it. Transport-specific code
// lives with its client (see pkg/webauth/cdp for the CLI's).
package webauth
```

Delete the package comment block at the top of `capture.go` if one is present;
a package needs exactly one.

- [ ] **Step 4: Run the moved tests**

Run: `go test ./pkg/webauth/ -v`
Expected: PASS. `TestSerializeCookies`, `TestHostMatches`, `TestFilterForHost`, `TestEarliestExpiry`, `TestLoginSucceeded`, `TestAssembleSession`, `TestReplayable` all pass with no edits to their bodies.

- [ ] **Step 5: Verify the dependency rule holds**

Run: `go list -deps ./pkg/webauth | grep -v '^github.com/angelmsger' | grep '\.'`
Expected: no output. `pkg/webauth` pulls no third-party dependency at all — in particular not `yaml.v3` (which would mean `pkg/config` crept in) and not `coder/websocket`.

- [ ] **Step 6: Commit**

```bash
git add pkg/webauth/
git commit -m "$(cat <<'EOF'
feat(webauth): add the shared browser capture core

Moves the pure capture logic — cookie shaping, host scoping, expiry and
the login-success heuristic — out of o3's internal/webauth into a public
package both clients can share. The WebKit JSON payload decoder stays
behind in o3; it is transport detail, not shared policy.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: The injected capture script

**Files:**
- Create: `pkg/webauth/probe.go`
- Test: `pkg/webauth/probe_test.go`

**Interfaces:**
- Produces: `ProbeJS(deliver string) string`.

- [ ] **Step 1: Write the failing test**

Create `pkg/webauth/probe_test.go`:

```go
package webauth

import (
	"strings"
	"testing"
)

func TestProbeJSBindsDeliverName(t *testing.T) {
	js := ProbeJS("__oo_probe")
	if !strings.Contains(js, "__oo_probe") {
		t.Fatal("script does not call the delivery function it was given")
	}
	if strings.Contains(js, "webkit.messageHandlers") {
		t.Fatal("script must not hardcode a transport; the caller binds the delivery name")
	}
}

// Both hooks are load-bearing: OpenObserve's web app issues requests through
// axios (XHR), so a fetch-only hook captures nothing on real instances and
// leaves only the short-lived session cookie. Guard against a well-meaning
// simplification deleting one.
func TestProbeJSHooksBothFetchAndXHR(t *testing.T) {
	js := ProbeJS("d")
	for _, want := range []string{
		"window.fetch",
		"XMLHttpRequest.prototype.setRequestHeader",
		"localStorage.getItem('user_info')",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("script is missing %q", want)
		}
	}
}

func TestProbeJSIsSelfContainedExpression(t *testing.T) {
	js := strings.TrimSpace(ProbeJS("d"))
	if !strings.HasPrefix(js, "(function()") || !strings.HasSuffix(js, ")();") {
		t.Fatalf("script must be one self-invoking expression, got prefix/suffix of %q", js)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./pkg/webauth/ -run TestProbeJS -v`
Expected: FAIL — `undefined: ProbeJS`.

- [ ] **Step 3: Write the implementation**

Create `pkg/webauth/probe.go`. The body is a port of the Objective-C literal in
o3's `webauth_darwin.m:127-140`, with the delivery call parameterised:

```go
package webauth

import "strings"

// ProbeJS returns the capture script injected into the sign-in page. The script
// calls the global function named deliver with an object carrying an
// {authorization} or {email} key as it observes them.
//
// It hooks BOTH window.fetch and XMLHttpRequest.prototype.setRequestHeader.
// Both are required: OpenObserve's web app issues its API requests through
// axios, which uses XHR, so a fetch-only hook captures nothing on a real
// instance and leaves only the short-lived session cookie. The Authorization
// header the SPA sends is the durable credential — typically
// Basic base64(email:token) — and it outlives the session cookie, so it is the
// more valuable of the two captures.
//
// The script is defensive to the point of paranoia — every step is wrapped in
// try/catch and the original function is always called — because it runs inside
// a page we do not control and must never break the user's ability to log in.
func ProbeJS(deliver string) string {
	return strings.ReplaceAll(probeTemplate, "__DELIVER__", deliver)
}

const probeTemplate = `(function(){try{` +
	`function post(a){try{a=String(a||'');if(a){__DELIVER__(JSON.stringify({authorization:a}));}}catch(e){}}` +
	`try{var of=window.fetch;window.fetch=function(){try{` +
	`var h=arguments[1]&&arguments[1].headers;` +
	`if(h){var a=h['Authorization']||h['authorization']||(h.get&&(h.get('Authorization')||h.get('authorization')));if(a)post(a);}` +
	`}catch(e){}return of.apply(this,arguments);};}catch(e){}` +
	`try{var XS=XMLHttpRequest.prototype.setRequestHeader;` +
	`XMLHttpRequest.prototype.setRequestHeader=function(k,v){try{` +
	`if(k&&String(k).toLowerCase()==='authorization')post(v);` +
	`}catch(e){}return XS.apply(this,arguments);};}catch(e){}` +
	`try{var raw=localStorage.getItem('user_info')||localStorage.getItem('userInfo');` +
	`if(raw){var j=JSON.parse(raw);var em=j.email||(j.data&&j.data.email);` +
	`if(em){__DELIVER__(JSON.stringify({email:em}));}}}catch(e){}` +
	`}catch(e){}})();`
```

Note the one deliberate difference from the Objective-C original: the payload is
`JSON.stringify(...)` rather than a live object, because a CDP binding accepts
only a single string argument. The WebKit shell in Task 11 parses the same JSON.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./pkg/webauth/ -run TestProbeJS -v`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add pkg/webauth/probe.go pkg/webauth/probe_test.go
git commit -m "$(cat <<'EOF'
feat(webauth): share the injected capture script

Lifts the fetch/XHR Authorization hook out of the Objective-C string
literal it was trapped in and into a Go const, parameterised by the name
of its delivery function so each transport can bind its own. A fix to
the hook now reaches both clients instead of only o3.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: The success-policy `Tracker`

This extracts the `verifyBusy`/`verifySig` bookkeeping currently inside
`webauthProbe` in o3's `webauth_darwin.go:95-150` — transport-independent policy
stranded in a platform-specific file.

**Files:**
- Create: `pkg/webauth/tracker.go`
- Test: `pkg/webauth/tracker_test.go`

**Interfaces:**
- Consumes: `Cookie`, `VerifyFunc`, `AssembleSession`, `Replayable`, `LoginSucceeded` (all from Task 1).
- Produces: `NewTracker(host string, verify VerifyFunc) *Tracker`; `(*Tracker).Observe(cookies []Cookie, currentURL, authz, email string) (auth.Session, bool)`.

- [ ] **Step 1: Write the failing test**

Create `pkg/webauth/tracker_test.go`:

```go
package webauth

import (
	"sync"
	"testing"

	pkgauth "github.com/angelmsger/openobserve-cli/pkg/auth"
)

func hostCookies() []Cookie {
	return []Cookie{{Name: "auth_tokens", Value: "abc", Domain: "o2.example.com", Path: "/"}}
}

func TestTrackerIgnoresNothingToReplay(t *testing.T) {
	tr := NewTracker("o2.example.com", func(pkgauth.Session) bool { return true })
	if _, ok := tr.Observe(nil, "https://o2.example.com/web/login", "", ""); ok {
		t.Fatal("no cookies and no authorization is not replayable; must not report success")
	}
}

func TestTrackerRequiresVerification(t *testing.T) {
	tr := NewTracker("o2.example.com", func(pkgauth.Session) bool { return false })
	if _, ok := tr.Observe(hostCookies(), "https://o2.example.com/web/logs", "", ""); ok {
		t.Fatal("verifier rejected the session; must not report success")
	}
}

func TestTrackerReportsVerifiedSession(t *testing.T) {
	tr := NewTracker("o2.example.com", func(pkgauth.Session) bool { return true })
	sess, ok := tr.Observe(hostCookies(), "https://o2.example.com/web/logs", "Basic xyz", "ops@example.com")
	if !ok {
		t.Fatal("verified session must report success")
	}
	if sess.Authorization != "Basic xyz" || sess.Email != "ops@example.com" {
		t.Fatalf("session did not carry the captured fields: %+v", sess)
	}
	if sess.Cookies == "" {
		t.Fatal("session did not carry the host cookies")
	}
}

// A static page must not be re-verified on every tick; that would hammer the
// instance with an authenticated request once a second.
func TestTrackerVerifiesEachDistinctStateOnce(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	tr := NewTracker("o2.example.com", func(pkgauth.Session) bool {
		mu.Lock()
		defer mu.Unlock()
		calls++
		return false
	})
	for i := 0; i < 5; i++ {
		tr.Observe(hostCookies(), "https://o2.example.com/web/logs", "", "")
	}
	mu.Lock()
	defer mu.Unlock()
	if calls != 1 {
		t.Fatalf("verifier called %d times for an unchanged state, want 1", calls)
	}
}

func TestTrackerRetriesWhenStateChanges(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	tr := NewTracker("o2.example.com", func(s pkgauth.Session) bool {
		mu.Lock()
		defer mu.Unlock()
		calls++
		return s.Authorization != ""
	})
	tr.Observe(hostCookies(), "https://o2.example.com/web/logs", "", "")
	_, ok := tr.Observe(hostCookies(), "https://o2.example.com/web/logs", "Basic xyz", "")
	mu.Lock()
	defer mu.Unlock()
	if calls != 2 {
		t.Fatalf("verifier called %d times, want 2 (state changed between observations)", calls)
	}
	if !ok {
		t.Fatal("second observation carried a working credential and must succeed")
	}
}

// Off-darwin builds and older tests run without a verifier; the pure heuristic
// is the fallback so behaviour is preserved.
func TestTrackerFallsBackToHeuristicWithoutVerifier(t *testing.T) {
	// A generic session cookie, NOT one of the known post-login auth cookies —
	// those succeed on their own and would bypass the URL check below.
	plain := []Cookie{{Name: "sid", Value: "x", Domain: "o2.example.com", Path: "/"}}

	tr := NewTracker("o2.example.com", nil)
	if _, ok := tr.Observe(plain, "https://o2.example.com/web/login", "", ""); ok {
		t.Fatal("still on the login page with only a generic cookie; heuristic must not report success")
	}
	if _, ok := tr.Observe(plain, "https://o2.example.com/web/logs", "", ""); !ok {
		t.Fatal("navigated off the login page with a host cookie; heuristic must report success")
	}
}

// A known post-login auth cookie is sufficient on its own: OpenObserve sets it
// only after a successful login, so capture must not wait for a navigation that
// a single-page app may never make.
func TestTrackerHeuristicAcceptsAuthCookieOnLoginPage(t *testing.T) {
	tr := NewTracker("o2.example.com", nil)
	if _, ok := tr.Observe(hostCookies(), "https://o2.example.com/web/login", "", ""); !ok {
		t.Fatal("auth_tokens present; heuristic must report success even on the login path")
	}
}
```

`LoginSucceeded` has two independent success paths: a known post-login auth
cookie (`auth_ext` / `auth_tokens`) succeeds immediately regardless of URL,
and otherwise a host cookie plus a navigation away from `/login` succeeds.
`hostCookies()` returns `auth_tokens`, so the fallback test must use a
different cookie name or it silently tests the first path twice.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./pkg/webauth/ -run TestTracker -v`
Expected: FAIL — `undefined: NewTracker`.

- [ ] **Step 3: Write the implementation**

Create `pkg/webauth/tracker.go`:

```go
package webauth

import (
	"sync"

	pkgauth "github.com/angelmsger/openobserve-cli/pkg/auth"
)

// Tracker decides when an observed browser state counts as a completed login.
// It is shared by every transport so the CLI and o3 cannot drift on the
// question of when sign-in has actually finished.
//
// The policy, in order:
//
//   - Assemble the observed state into a session and drop it unless something
//     is replayable. An instance using native login sets no cookies at all, so
//     "has cookies" is not the test — "has cookies OR an Authorization header"
//     is.
//   - An authenticated API probe is the SOLE success signal. A benign cookie
//     set on the login page, or an in-progress redirect to an external identity
//     provider, must never be mistaken for a completed login, and only a real
//     authenticated request can tell the difference.
//   - Verify each distinct state at most once. Without this a static page is
//     re-probed on every tick, sending an authenticated request per second.
//
// A nil verifier disables the probe and falls back to the pure cookie/URL
// heuristic in LoginSucceeded. That path exists for unit tests and for any
// caller with no API client to hand; production callers always pass a verifier.
type Tracker struct {
	host   string
	verify VerifyFunc

	mu   sync.Mutex
	seen map[string]bool // signature -> already verified
}

// NewTracker returns a Tracker scoping cookies to host and confirming captures
// with verify. A nil verify selects the heuristic fallback.
func NewTracker(host string, verify VerifyFunc) *Tracker {
	return &Tracker{host: host, verify: verify, seen: map[string]bool{}}
}

// Observe feeds one sampled browser state to the policy. It returns the
// captured session and true exactly when sign-in is complete.
//
// Observe blocks for the duration of the verification request, so callers
// should not hold a UI lock across it.
func (t *Tracker) Observe(cookies []Cookie, currentURL, authz, email string) (pkgauth.Session, bool) {
	sess := AssembleSession(cookies, t.host, authz, email)

	if t.verify == nil {
		if !LoginSucceeded(currentURL, t.host, cookies) {
			return pkgauth.Session{}, false
		}
		return sess, true
	}

	if !Replayable(sess) {
		return pkgauth.Session{}, false
	}

	sig := sess.Cookies + "\n" + sess.Authorization
	t.mu.Lock()
	already := t.seen[sig]
	t.seen[sig] = true
	t.mu.Unlock()
	if already {
		return pkgauth.Session{}, false
	}

	if !t.verify(sess) {
		return pkgauth.Session{}, false
	}
	return sess, true
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./pkg/webauth/ -run TestTracker -v`
Expected: PASS (7 tests).

- [ ] **Step 5: Run the race detector**

Run: `go test ./pkg/webauth/ -race`
Expected: PASS, no race reported.

- [ ] **Step 6: Commit**

```bash
git add pkg/webauth/tracker.go pkg/webauth/tracker_test.go
git commit -m "$(cat <<'EOF'
feat(webauth): share the login-success policy

Extracts the replayable/in-flight/unchanged bookkeeping from o3's
platform-specific darwin file into a transport-independent Tracker, so
both clients apply the same rule for when sign-in has finished — and so
a static page is not re-verified on every sampling tick.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: The `Driver` seam and the Ping verifier

**Files:**
- Create: `pkg/webauth/driver.go`
- Test: `pkg/webauth/driver_test.go`

**Interfaces:**
- Consumes: `VerifyFunc` (Task 1); `pkg/auth.Session`, `pkg/auth.Credential`, `pkg/auth.EncodeSession`, `pkg/apiclient.Build`.
- Produces: `type Driver interface { Capture(loginURL, host string, verify VerifyFunc) (auth.Session, error) }`; `PingVerifier(baseURL, org string, timeout time.Duration, maxRetries int) VerifyFunc`.
- `PingVerifier` deliberately takes plain values rather than a `config.Defaults`: importing `pkg/config` would pull `yaml.v3` into `pkg/webauth` and break the global constraint, and a verifier has no business knowing the config-file type. Callers pass `d.Timeout, d.MaxRetries`.
- **Does not** declare `VerifyFunc` — it stays in `capture.go` where it already lives. Declaring it in both files will not compile.

- [ ] **Step 1: Write the failing test**

Create `pkg/webauth/driver_test.go`:

```go
package webauth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	pkgauth "github.com/angelmsger/openobserve-cli/pkg/auth"
)

// An instance using native login sets no cookies; the SPA authenticates with an
// Authorization header alone. Such a session must verify.
func TestPingVerifierAcceptsHeaderOnlySession(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Basic good" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"identifier":"default","name":"default"}]}`))
	}))
	defer srv.Close()

	verify := PingVerifier(srv.URL, "default", 5*time.Second, 0)
	if !verify(pkgauth.Session{Authorization: "Basic good"}) {
		t.Fatal("header-only session that authenticates must verify")
	}
	if verify(pkgauth.Session{Authorization: "Basic bad"}) {
		t.Fatal("session that does not authenticate must not verify")
	}
}

func TestPingVerifierAcceptsCookieSession(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "auth_tokens=good" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"identifier":"default","name":"default"}]}`))
	}))
	defer srv.Close()

	verify := PingVerifier(srv.URL, "default", 5*time.Second, 0)
	if !verify(pkgauth.Session{Cookies: "auth_tokens=good"}) {
		t.Fatal("cookie session that authenticates must verify")
	}
}

func TestPingVerifierRejectsUnusableSession(t *testing.T) {
	verify := PingVerifier("http://127.0.0.1:1", "default", 5*time.Second, 0)
	if verify(pkgauth.Session{}) {
		t.Fatal("an empty session must never verify")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./pkg/webauth/ -run TestPingVerifier -v`
Expected: FAIL — `undefined: PingVerifier`.

- [ ] **Step 3: Write the implementation**

Create `pkg/webauth/driver.go`. `PingVerifier` is o3's `sessionVerifier`
(`app.go:577`) moved with the o3-specific client builder replaced by
`apiclient.Build`:

```go
package webauth

import (
	"context"
	"time"

	"github.com/angelmsger/openobserve-cli/pkg/apiclient"
	pkgauth "github.com/angelmsger/openobserve-cli/pkg/auth"
)

// Driver opens a browser at loginURL and captures the session established
// there. host scopes which cookies are kept. Implementations block until login
// is verified, the user gives up, or a timeout elapses.
//
// Two implementations exist: pkg/webauth/cdp drives a Chromium-family browser
// over the DevTools Protocol (used by the CLI, and by o3 off macOS), and o3's
// own darwin shell drives a native WKWebView.
type Driver interface {
	Capture(loginURL, host string, verify VerifyFunc) (pkgauth.Session, error)
}

// verifyTimeout bounds a single verification request. It is generous because
// the probe fires while the user is mid-login on a possibly slow instance.
const verifyTimeout = 20 * time.Second

// PingVerifier returns a VerifyFunc that confirms a session by making a real
// authenticated request to the instance. This is the sole success signal for
// capture: only an authenticated response proves the captured state works.
//
// It takes plain values rather than a config.Defaults so this package stays
// free of pkg/config (and so of yaml.v3). Callers pass d.Timeout, d.MaxRetries.
func PingVerifier(baseURL, org string, timeout time.Duration, maxRetries int) VerifyFunc {
	return func(sess pkgauth.Session) bool {
		blob, err := pkgauth.EncodeSession(sess)
		if err != nil {
			return false
		}
		cred := pkgauth.Credential{
			Scheme:   pkgauth.SchemeSession,
			Username: sess.Email,
			Secret:   blob,
		}
		if err := cred.Validate(); err != nil {
			return false
		}
		client, err := apiclient.Build(apiclient.BuildParams{
			BaseURL:       baseURL,
			Org:           org,
			AuthDecorator: cred.Decorator(),
			Timeout:       timeout,
			MaxRetries:    maxRetries,
		})
		if err != nil {
			return false
		}
		ctx, cancel := context.WithTimeout(context.Background(), verifyTimeout)
		defer cancel()
		_, err = client.Ping(ctx)
		return err == nil
	}
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./pkg/webauth/ -v`
Expected: PASS — all tests from Tasks 1, 2, 3 and 4.

- [ ] **Step 5: Commit**

```bash
git add pkg/webauth/driver.go pkg/webauth/driver_test.go
git commit -m "$(cat <<'EOF'
feat(webauth): add the Driver seam and the Ping verifier

Formalises Capture as an interface so a transport can be swapped, and
moves o3's sessionVerifier here: it was built entirely from pkg/auth and
pkg/apiclient and contained nothing o3-specific.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 5: Minimal CDP client

The DevTools Protocol is JSON-RPC over one WebSocket. Messages carrying an `id`
are responses to our commands; messages carrying a `method` are events. A
`sessionId` field scopes a message to an attached page target.

**Files:**
- Create: `pkg/webauth/cdp/conn.go`
- Modify: `go.mod`
- Test: `pkg/webauth/cdp/conn_test.go`

**Interfaces:**
- Produces: `dial(ctx context.Context, wsURL string) (*conn, error)`; `(*conn).call(ctx context.Context, sessionID, method string, params map[string]any, out any) error`; `(*conn).onEvent(method string, fn func(sessionID string, params json.RawMessage))`; `(*conn).Close() error`. All unexported — the package's only exported surface is `New`/`Options` from Task 7.

- [ ] **Step 1: Add the WebSocket dependency**

```bash
cd ~/Development/Workspaces/oa-cli/src/openobserve-cli
go get github.com/coder/websocket@latest
```

Expected: `go.mod` gains one `require` line. This library is chosen because it
is single-purpose and dependency-free; do not substitute a larger CDP framework.

- [ ] **Step 2: Write the failing test**

Create `pkg/webauth/cdp/conn_test.go`:

```go
package cdp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// fakeCDP serves a WebSocket that echoes a canned result for every command and
// can push events on demand.
func fakeCDP(t *testing.T, handle func(id float64, method string, send func(any))) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("accept: %v", err)
			return
		}
		defer c.CloseNow()
		ctx := r.Context()
		var mu sync.Mutex
		send := func(v any) {
			b, _ := json.Marshal(v)
			mu.Lock()
			defer mu.Unlock()
			_ = c.Write(ctx, websocket.MessageText, b)
		}
		for {
			_, data, err := c.Read(ctx)
			if err != nil {
				return
			}
			var msg struct {
				ID     float64 `json:"id"`
				Method string  `json:"method"`
			}
			if json.Unmarshal(data, &msg) != nil {
				return
			}
			handle(msg.ID, msg.Method, send)
		}
	}))
}

func wsURL(s *httptest.Server) string { return "ws" + strings.TrimPrefix(s.URL, "http") }

func TestConnCallReturnsResult(t *testing.T) {
	srv := fakeCDP(t, func(id float64, method string, send func(any)) {
		send(map[string]any{"id": id, "result": map[string]any{"targetId": "T1"}})
	})
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := dial(ctx, wsURL(srv))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()

	var out struct {
		TargetID string `json:"targetId"`
	}
	if err := c.call(ctx, "", "Target.createTarget", nil, &out); err != nil {
		t.Fatalf("call: %v", err)
	}
	if out.TargetID != "T1" {
		t.Fatalf("targetId = %q, want T1", out.TargetID)
	}
}

// Responses must be routed by id, not by arrival order.
func TestConnCallDemultiplexesOutOfOrderResponses(t *testing.T) {
	var mu sync.Mutex
	var pending []float64
	srv := fakeCDP(t, func(id float64, method string, send func(any)) {
		mu.Lock()
		pending = append(pending, id)
		n := len(pending)
		ids := append([]float64(nil), pending...)
		mu.Unlock()
		if n < 2 {
			return // hold the first response back
		}
		// Answer in reverse order.
		for i := len(ids) - 1; i >= 0; i-- {
			send(map[string]any{"id": ids[i], "result": map[string]any{"n": ids[i]}})
		}
	})
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := dial(ctx, wsURL(srv))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()

	type res struct {
		N float64 `json:"n"`
	}
	var wg sync.WaitGroup
	got := make([]float64, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			var r res
			if err := c.call(ctx, "", "Some.method", nil, &r); err != nil {
				t.Errorf("call %d: %v", i, err)
				return
			}
			got[i] = r.N
		}(i)
	}
	wg.Wait()
	if got[0] == got[1] {
		t.Fatalf("both calls got the same id %v; responses were not demultiplexed", got[0])
	}
}

func TestConnCallSurfacesProtocolError(t *testing.T) {
	srv := fakeCDP(t, func(id float64, method string, send func(any)) {
		send(map[string]any{"id": id, "error": map[string]any{"code": -32000, "message": "boom"}})
	})
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := dial(ctx, wsURL(srv))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()

	err = c.call(ctx, "", "Bad.method", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("err = %v, want it to carry the protocol message", err)
	}
}

func TestConnDispatchesEvents(t *testing.T) {
	srv := fakeCDP(t, func(id float64, method string, send func(any)) {
		send(map[string]any{"id": id, "result": map[string]any{}})
		send(map[string]any{
			"method":    "Runtime.bindingCalled",
			"sessionId": "S1",
			"params":    map[string]any{"payload": `{"email":"ops@example.com"}`},
		})
	})
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := dial(ctx, wsURL(srv))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()

	seen := make(chan string, 1)
	c.onEvent("Runtime.bindingCalled", func(sessionID string, params json.RawMessage) {
		var p struct {
			Payload string `json:"payload"`
		}
		_ = json.Unmarshal(params, &p)
		seen <- sessionID + "|" + p.Payload
	})

	if err := c.call(ctx, "", "Runtime.enable", nil, nil); err != nil {
		t.Fatalf("call: %v", err)
	}
	select {
	case got := <-seen:
		if got != `S1|{"email":"ops@example.com"}` {
			t.Fatalf("event payload = %q", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("event handler never fired")
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./pkg/webauth/cdp/ -v`
Expected: FAIL — `undefined: dial`.

- [ ] **Step 4: Write the implementation**

Create `pkg/webauth/cdp/conn.go`:

```go
// Package cdp captures an OpenObserve browser session by driving a
// Chromium-family browser over the Chrome DevTools Protocol. It implements
// webauth.Driver for openobserve-cli, and for o3 on platforms with no native
// WebView shell.
package cdp

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/coder/websocket"
)

// readLimit raises the WebSocket read limit well above the library default of
// 32 KiB. Storage.getCookies on an instance with many cookies, and any
// Runtime.evaluate returning a large value, exceed the default and would
// otherwise fail the whole connection rather than one call.
const readLimit = 32 << 20 // 32 MiB

// conn is a DevTools Protocol connection: one WebSocket carrying command
// responses (keyed by id) and events (keyed by method), demultiplexed by a
// single reader goroutine.
type conn struct {
	ws     *websocket.Conn
	nextID atomic.Int64

	mu       sync.Mutex
	pending  map[int64]chan rpcResponse
	handlers map[string][]func(sessionID string, params json.RawMessage)
	closed   bool

	readerDone chan struct{}
	readErr    error
}

type rpcResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *rpcError       `json:"error"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcMessage struct {
	ID        *int64          `json:"id"`
	Method    string          `json:"method"`
	SessionID string          `json:"sessionId"`
	Params    json.RawMessage `json:"params"`
	Result    json.RawMessage `json:"result"`
	Error     *rpcError       `json:"error"`
}

// dial opens a DevTools connection and starts its reader.
func dial(ctx context.Context, wsURL string) (*conn, error) {
	ws, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("connect to devtools: %w", err)
	}
	ws.SetReadLimit(readLimit)
	c := &conn{
		ws:         ws,
		pending:    map[int64]chan rpcResponse{},
		handlers:   map[string][]func(string, json.RawMessage){},
		readerDone: make(chan struct{}),
	}
	go c.read()
	return c, nil
}

// read is the single reader: it routes responses to their waiting caller and
// fans events out to registered handlers.
func (c *conn) read() {
	defer close(c.readerDone)
	for {
		_, data, err := c.ws.Read(context.Background())
		if err != nil {
			c.mu.Lock()
			c.readErr = err
			for id, ch := range c.pending {
				close(ch)
				delete(c.pending, id)
			}
			c.mu.Unlock()
			return
		}
		var msg rpcMessage
		if json.Unmarshal(data, &msg) != nil {
			continue // a message we cannot parse is not fatal to the session
		}
		if msg.ID != nil {
			c.mu.Lock()
			ch, ok := c.pending[*msg.ID]
			delete(c.pending, *msg.ID)
			c.mu.Unlock()
			if ok {
				ch <- rpcResponse{Result: msg.Result, Error: msg.Error}
				close(ch)
			}
			continue
		}
		if msg.Method == "" {
			continue
		}
		c.mu.Lock()
		fns := append([]func(string, json.RawMessage)(nil), c.handlers[msg.Method]...)
		c.mu.Unlock()
		for _, fn := range fns {
			fn(msg.SessionID, msg.Params)
		}
	}
}

// onEvent registers a handler for a DevTools event. Handlers run on the reader
// goroutine, so they must not block or call back into conn.call.
func (c *conn) onEvent(method string, fn func(sessionID string, params json.RawMessage)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.handlers[method] = append(c.handlers[method], fn)
}

// call issues a command and decodes its result into out (which may be nil).
// sessionID scopes the command to an attached target; "" addresses the browser.
func (c *conn) call(ctx context.Context, sessionID, method string, params map[string]any, out any) error {
	id := c.nextID.Add(1)
	req := map[string]any{"id": id, "method": method}
	if params != nil {
		req["params"] = params
	}
	if sessionID != "" {
		req["sessionId"] = sessionID
	}
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}

	ch := make(chan rpcResponse, 1)
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return fmt.Errorf("devtools connection closed")
	}
	c.pending[id] = ch
	c.mu.Unlock()

	if err := c.ws.Write(ctx, websocket.MessageText, body); err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return fmt.Errorf("%s: %w", method, err)
	}

	select {
	case resp, ok := <-ch:
		if !ok {
			return fmt.Errorf("%s: devtools connection lost", method)
		}
		if resp.Error != nil {
			return fmt.Errorf("%s: %s (code %d)", method, resp.Error.Message, resp.Error.Code)
		}
		if out != nil && len(resp.Result) > 0 {
			return json.Unmarshal(resp.Result, out)
		}
		return nil
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return ctx.Err()
	}
}

// Close tears the connection down; in-flight calls fail rather than hang.
func (c *conn) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	c.mu.Unlock()
	return c.ws.CloseNow()
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./pkg/webauth/cdp/ -race -v`
Expected: PASS (4 tests), no race reported.

- [ ] **Step 6: Confirm the dependency did not leak into the pure core**

Run: `go list -deps ./pkg/webauth | grep coder/websocket`
Expected: no output. The dependency belongs to the subpackage only.

- [ ] **Step 7: Commit**

```bash
git add go.mod go.sum pkg/webauth/cdp/
git commit -m "$(cat <<'EOF'
feat(cdp): add a minimal DevTools Protocol client

One WebSocket, a single reader goroutine demultiplexing responses by id
and fanning events out by method. Enough protocol for capture and no
more, which keeps the dependency to one small single-purpose library
instead of a full CDP framework.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 6: Browser discovery and launch

**Files:**
- Create: `pkg/webauth/cdp/launch.go`
- Test: `pkg/webauth/cdp/launch_test.go`

**Interfaces:**
- Produces: `findBrowser() (string, error)`; `browserCandidates() []string`; `launch(ctx context.Context, exe, profileDir string) (*exec.Cmd, string, error)` returning the process and the browser-level WebSocket URL; `errNoBrowser`.

- [ ] **Step 1: Write the failing test**

Create `pkg/webauth/cdp/launch_test.go`:

```go
package cdp

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestBrowserCandidatesAreAbsoluteOrBareNames(t *testing.T) {
	got := browserCandidates()
	if len(got) == 0 {
		t.Fatal("no browser candidates for this platform")
	}
	for _, c := range got {
		if c == "" {
			t.Fatal("empty candidate")
		}
	}
	// Sanity-check the platform actually got its own list rather than a default.
	switch runtime.GOOS {
	case "darwin":
		if !strings.Contains(strings.Join(got, "|"), "/Applications/") {
			t.Errorf("darwin candidates should include /Applications paths: %v", got)
		}
	case "windows":
		if !strings.Contains(strings.ToLower(strings.Join(got, "|")), "chrome.exe") {
			t.Errorf("windows candidates should include chrome.exe: %v", got)
		}
	default:
		if !strings.Contains(strings.Join(got, "|"), "google-chrome") {
			t.Errorf("linux candidates should include google-chrome: %v", got)
		}
	}
}

func TestFindBrowserPrefersEnvOverride(t *testing.T) {
	fake := filepath.Join(t.TempDir(), "my-browser")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OPENOBSERVE_BROWSER", fake)
	got, err := findBrowser()
	if err != nil {
		t.Fatalf("findBrowser: %v", err)
	}
	if got != fake {
		t.Fatalf("findBrowser = %q, want the override %q", got, fake)
	}
}

func TestFindBrowserRejectsMissingOverride(t *testing.T) {
	t.Setenv("OPENOBSERVE_BROWSER", filepath.Join(t.TempDir(), "nope"))
	if _, err := findBrowser(); err == nil {
		t.Fatal("a set-but-missing override must be an error, not a silent fallback")
	}
}

func TestReadDevToolsURLWaitsForTheFile(t *testing.T) {
	dir := t.TempDir()
	go func() {
		time.Sleep(150 * time.Millisecond)
		_ = os.WriteFile(filepath.Join(dir, "DevToolsActivePort"),
			[]byte("54321\n/devtools/browser/abc-123\n"), 0o600)
	}()
	got, err := readDevToolsURL(dir, 3*time.Second, func() bool { return true })
	if err != nil {
		t.Fatalf("readDevToolsURL: %v", err)
	}
	want := "ws://127.0.0.1:54321/devtools/browser/abc-123"
	if got != want {
		t.Fatalf("url = %q, want %q", got, want)
	}
}

func TestReadDevToolsURLFailsWhenProcessDies(t *testing.T) {
	_, err := readDevToolsURL(t.TempDir(), 3*time.Second, func() bool { return false })
	if err == nil {
		t.Fatal("must fail fast when the browser process is gone, not wait for the timeout")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./pkg/webauth/cdp/ -run 'TestBrowser|TestFind|TestReadDevTools' -v`
Expected: FAIL — `undefined: browserCandidates`.

- [ ] **Step 3: Write the implementation**

Create `pkg/webauth/cdp/launch.go`:

```go
package cdp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// errNoBrowser is returned when no Chromium-family browser can be located. The
// CLI layer translates it into a BROWSER_NOT_FOUND cerrors value with next
// steps; keeping it a sentinel here keeps this package free of cerrors.
var errNoBrowser = errors.New("no Chromium-family browser found")

// browserCandidates lists the executables to try, most-preferred first. Bare
// names are resolved through PATH; absolute paths are probed directly.
func browserCandidates() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
			"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
		}
	case "windows":
		var out []string
		for _, base := range []string{
			os.Getenv("ProgramFiles"),
			os.Getenv("ProgramFiles(x86)"),
			os.Getenv("LOCALAPPDATA"),
		} {
			if base == "" {
				continue
			}
			out = append(out,
				filepath.Join(base, `Google\Chrome\Application\chrome.exe`),
				filepath.Join(base, `Microsoft\Edge\Application\msedge.exe`),
			)
		}
		return out
	default:
		return []string{
			"google-chrome", "google-chrome-stable", "chromium",
			"chromium-browser", "microsoft-edge", "brave-browser",
		}
	}
}

// findBrowser locates a usable browser. OPENOBSERVE_BROWSER overrides the
// search entirely; when it is set but unusable that is an error rather than a
// silent fallback, because silently ignoring an explicit choice is worse than
// failing.
func findBrowser() (string, error) {
	if override := strings.TrimSpace(os.Getenv("OPENOBSERVE_BROWSER")); override != "" {
		if _, err := os.Stat(override); err != nil {
			if resolved, lookErr := exec.LookPath(override); lookErr == nil {
				return resolved, nil
			}
			return "", fmt.Errorf("OPENOBSERVE_BROWSER is set to %q, which is not executable: %w", override, err)
		}
		return override, nil
	}
	for _, c := range browserCandidates() {
		if filepath.IsAbs(c) {
			if _, err := os.Stat(c); err == nil {
				return c, nil
			}
			continue
		}
		if resolved, err := exec.LookPath(c); err == nil {
			return resolved, nil
		}
	}
	return "", errNoBrowser
}

// launch starts the browser on about:blank with remote debugging enabled and
// returns the process plus the browser-level WebSocket URL.
//
// The browser starts on about:blank rather than the login URL so the capture
// script can be installed before any instance page loads. Injecting after the
// SPA has already booted would race its first request, which is exactly the
// request carrying the Authorization header worth capturing.
//
// Port 0 makes the browser choose a free port and write it, with the WebSocket
// path, to DevToolsActivePort inside the profile directory. Reading that file
// is the only reliable way to learn the port.
func launch(ctx context.Context, exe, profileDir string) (*exec.Cmd, string, error) {
	if err := os.MkdirAll(profileDir, 0o700); err != nil {
		return nil, "", fmt.Errorf("create browser profile directory: %w", err)
	}
	// A stale port file from a previous run would be read as if it were ours.
	_ = os.Remove(filepath.Join(profileDir, "DevToolsActivePort"))

	cmd := exec.CommandContext(ctx, exe,
		"--user-data-dir="+profileDir,
		"--remote-debugging-port=0",
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-background-networking",
		"about:blank",
	)
	if err := cmd.Start(); err != nil {
		return nil, "", fmt.Errorf("start browser: %w", err)
	}

	alive := func() bool { return cmd.ProcessState == nil }
	wsURL, err := readDevToolsURL(profileDir, 30*time.Second, alive)
	if err != nil {
		_ = cmd.Process.Kill()
		return nil, "", err
	}
	return cmd, wsURL, nil
}

// readDevToolsURL polls for the DevToolsActivePort file the browser writes on
// startup. Its first line is the port; its second is the browser WebSocket
// path. alive reports whether the browser is still running, so a browser that
// exits immediately fails fast instead of burning the full timeout.
func readDevToolsURL(profileDir string, timeout time.Duration, alive func() bool) (string, error) {
	path := filepath.Join(profileDir, "DevToolsActivePort")
	deadline := time.Now().Add(timeout)
	for {
		data, err := os.ReadFile(path)
		if err == nil {
			lines := strings.Split(strings.TrimSpace(string(data)), "\n")
			if len(lines) >= 2 {
				port := strings.TrimSpace(lines[0])
				wsPath := strings.TrimSpace(lines[1])
				if port != "" && wsPath != "" {
					return "ws://127.0.0.1:" + port + wsPath, nil
				}
			}
		}
		if !alive() {
			return "", errors.New("the browser exited before remote debugging became available")
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("the browser did not report a debugging port within %s", timeout)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
```

The `context` import is used by `exec.CommandContext` in `launch`; no other
reference is needed.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./pkg/webauth/cdp/ -run 'TestBrowser|TestFind|TestReadDevTools' -v`
Expected: PASS (5 tests).

- [ ] **Step 5: Commit**

```bash
git add pkg/webauth/cdp/launch.go pkg/webauth/cdp/launch_test.go
git commit -m "$(cat <<'EOF'
feat(cdp): locate and launch a Chromium-family browser

Probes the platform's known Chrome/Chromium/Edge/Brave locations, with
OPENOBSERVE_BROWSER as an explicit override that fails loudly rather
than falling back when it is wrong. Starts on about:blank so the capture
script is installed before any instance page loads.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 7: The CDP capture driver

**Files:**
- Create: `pkg/webauth/cdp/capture.go`
- Test: `pkg/webauth/cdp/capture_test.go`

**Interfaces:**
- Consumes: `dial`, `call`, `onEvent` (Task 5); `findBrowser`, `launch` (Task 6); `webauth.NewTracker`, `webauth.Cookie`, `webauth.VerifyFunc` (Tasks 1, 3, 4).
- Produces: `New(opts Options) *Driver`; `type Options struct { ProfileDir string; Timeout time.Duration }`; `(*Driver).Capture(loginURL, host string, verify webauth.VerifyFunc) (auth.Session, error)`; sentinel errors `ErrNoBrowser`, `ErrCancelled`, `ErrTimeout`.

- [ ] **Step 1: Write the failing test**

Create `pkg/webauth/cdp/capture_test.go`:

```go
package cdp

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/angelmsger/openobserve-cli/pkg/webauth"
)

func TestConvertCookies(t *testing.T) {
	raw := []byte(`[
		{"name":"auth_tokens","value":"abc","domain":"o2.example.com","path":"/","expires":1785000000,"secure":true,"httpOnly":true},
		{"name":"sid","value":"x","domain":"o2.example.com","path":"/","expires":-1}
	]`)
	var cdpCookies []cdpCookie
	if err := json.Unmarshal(raw, &cdpCookies); err != nil {
		t.Fatal(err)
	}
	got := convertCookies(cdpCookies)
	if len(got) != 2 {
		t.Fatalf("got %d cookies, want 2", len(got))
	}
	if got[0].Name != "auth_tokens" || got[0].Expires.IsZero() {
		t.Fatalf("first cookie wrong: %+v", got[0])
	}
	// CDP uses -1, not 0, for a session cookie. Treating -1 as a real timestamp
	// would date the session to 1969 and make it look already expired.
	if !got[1].Expires.IsZero() {
		t.Fatalf("session cookie must have a zero expiry, got %v", got[1].Expires)
	}
}

func TestParseBindingPayload(t *testing.T) {
	authz, email := parseBindingPayload(`{"authorization":"Basic xyz"}`)
	if authz != "Basic xyz" || email != "" {
		t.Fatalf("authz=%q email=%q", authz, email)
	}
	authz, email = parseBindingPayload(`{"email":"ops@example.com"}`)
	if authz != "" || email != "ops@example.com" {
		t.Fatalf("authz=%q email=%q", authz, email)
	}
	if a, e := parseBindingPayload("not json"); a != "" || e != "" {
		t.Fatal("malformed payload must be ignored, not panic")
	}
}

func TestDriverSatisfiesWebauthDriver(t *testing.T) {
	var _ webauth.Driver = New(Options{})
}

func TestOptionsDefaultTimeout(t *testing.T) {
	d := New(Options{})
	if d.timeout != defaultTimeout {
		t.Fatalf("timeout = %v, want the %v default", d.timeout, defaultTimeout)
	}
	d = New(Options{Timeout: time.Minute})
	if d.timeout != time.Minute {
		t.Fatalf("explicit timeout was not honoured: %v", d.timeout)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./pkg/webauth/cdp/ -run 'TestConvert|TestParseBinding|TestDriver|TestOptions' -v`
Expected: FAIL — `undefined: cdpCookie`.

- [ ] **Step 3: Write the implementation**

Create `pkg/webauth/cdp/capture.go`:

```go
package cdp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	pkgauth "github.com/angelmsger/openobserve-cli/pkg/auth"
	"github.com/angelmsger/openobserve-cli/pkg/webauth"
)

// Sentinel errors the CLI layer maps onto its own cerrors codes.
var (
	// ErrNoBrowser reports that no Chromium-family browser could be located.
	ErrNoBrowser = errNoBrowser
	// ErrCancelled reports that the user closed the browser before signing in.
	ErrCancelled = errors.New("browser sign-in was cancelled")
	// ErrTimeout reports that sign-in did not complete in time.
	ErrTimeout = errors.New("browser sign-in timed out")
)

// defaultTimeout bounds a whole sign-in, matching o3's native window.
const defaultTimeout = 10 * time.Minute

// pollInterval is how often the browser state is sampled. The Tracker makes
// each distinct state verify at most once, so this cadence costs nothing while
// the user is typing.
const pollInterval = time.Second

// bindingName is the global function the injected script calls. It is scoped
// with an unlikely prefix to avoid colliding with anything in the page.
const bindingName = "__openobserveCliProbe"

// Options configures a Driver.
type Options struct {
	// ProfileDir is the browser profile to use. Empty means a temporary
	// directory removed when Capture returns, which forces a fresh login every
	// time; callers wanting a remembered identity-provider session pass a
	// persistent path.
	ProfileDir string
	// Timeout bounds a whole sign-in. Zero selects defaultTimeout.
	Timeout time.Duration
}

// Driver captures a session by driving a Chromium-family browser over the
// DevTools Protocol. It implements webauth.Driver.
type Driver struct {
	profileDir string
	timeout    time.Duration
}

// New returns a Driver configured by opts.
func New(opts Options) *Driver {
	t := opts.Timeout
	if t <= 0 {
		t = defaultTimeout
	}
	return &Driver{profileDir: opts.ProfileDir, timeout: t}
}

// cdpCookie mirrors the Network.Cookie shape the protocol returns.
type cdpCookie struct {
	Name     string  `json:"name"`
	Value    string  `json:"value"`
	Domain   string  `json:"domain"`
	Path     string  `json:"path"`
	Expires  float64 `json:"expires"` // unix seconds; -1 for a session cookie
	Secure   bool    `json:"secure"`
	HTTPOnly bool    `json:"httpOnly"`
}

// convertCookies maps protocol cookies onto the shared type. CDP signals a
// session cookie with -1, which must become a zero time rather than a 1969
// timestamp that would read as long expired.
func convertCookies(in []cdpCookie) []webauth.Cookie {
	out := make([]webauth.Cookie, 0, len(in))
	for _, c := range in {
		var exp time.Time
		if c.Expires > 0 {
			exp = time.Unix(int64(c.Expires), 0).UTC()
		}
		out = append(out, webauth.Cookie{
			Name: c.Name, Value: c.Value, Domain: c.Domain, Path: c.Path,
			Expires: exp, Secure: c.Secure, HTTPOnly: c.HTTPOnly,
		})
	}
	return out
}

// parseBindingPayload decodes one observation pushed by the injected script. A
// malformed payload is ignored: the script runs in a page we do not control.
func parseBindingPayload(payload string) (authorization, email string) {
	var p struct {
		Authorization string `json:"authorization"`
		Email         string `json:"email"`
	}
	if json.Unmarshal([]byte(payload), &p) != nil {
		return "", ""
	}
	return p.Authorization, p.Email
}

// Capture opens the browser at loginURL and blocks until the session is
// verified, the user closes the browser, or the timeout elapses.
func (d *Driver) Capture(loginURL, host string, verify webauth.VerifyFunc) (pkgauth.Session, error) {
	exe, err := findBrowser()
	if err != nil {
		return pkgauth.Session{}, err
	}

	profileDir := d.profileDir
	if profileDir == "" {
		tmp, err := os.MkdirTemp("", "openobserve-signin-")
		if err != nil {
			return pkgauth.Session{}, err
		}
		defer os.RemoveAll(tmp)
		profileDir = tmp
	}

	ctx, cancel := context.WithTimeout(context.Background(), d.timeout)
	defer cancel()

	cmd, wsURL, err := launch(ctx, exe, profileDir)
	if err != nil {
		return pkgauth.Session{}, err
	}
	// Kill only — never Wait here. processExited below owns the single Wait on
	// this process; calling Wait twice returns an error and races the first.
	defer func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}()

	c, err := dial(ctx, wsURL)
	if err != nil {
		return pkgauth.Session{}, err
	}
	defer c.Close()

	sessionID, err := attachToPage(ctx, c)
	if err != nil {
		return pkgauth.Session{}, err
	}

	// Observations pushed by the injected script. Guarded because they arrive
	// on the connection's reader goroutine.
	var obsMu sync.Mutex
	var authz, email string
	c.onEvent("Runtime.bindingCalled", func(sid string, params json.RawMessage) {
		var p struct {
			Name    string `json:"name"`
			Payload string `json:"payload"`
		}
		if json.Unmarshal(params, &p) != nil || p.Name != bindingName {
			return
		}
		a, e := parseBindingPayload(p.Payload)
		obsMu.Lock()
		defer obsMu.Unlock()
		if a != "" {
			authz = a
		}
		if e != "" {
			email = e
		}
	})

	if err := prepare(ctx, c, sessionID, loginURL); err != nil {
		return pkgauth.Session{}, err
	}

	tracker := webauth.NewTracker(host, verify)
	exited := processExited(cmd)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-exited:
			return pkgauth.Session{}, ErrCancelled
		case <-ctx.Done():
			return pkgauth.Session{}, ErrTimeout
		case <-ticker.C:
			cookies, currentURL, err := sample(ctx, c, sessionID)
			if err != nil {
				continue // a transient protocol error mid-navigation is normal
			}
			obsMu.Lock()
			a, e := authz, email
			obsMu.Unlock()
			if sess, ok := tracker.Observe(cookies, currentURL, a, e); ok {
				return sess, nil
			}
		}
	}
}

// attachToPage finds the browser's page target and attaches to it, returning
// the session id that scopes subsequent commands.
func attachToPage(ctx context.Context, c *conn) (string, error) {
	var targets struct {
		TargetInfos []struct {
			TargetID string `json:"targetId"`
			Type     string `json:"type"`
		} `json:"targetInfos"`
	}
	if err := c.call(ctx, "", "Target.getTargets", nil, &targets); err != nil {
		return "", err
	}
	for _, t := range targets.TargetInfos {
		if t.Type != "page" {
			continue
		}
		var att struct {
			SessionID string `json:"sessionId"`
		}
		err := c.call(ctx, "", "Target.attachToTarget",
			map[string]any{"targetId": t.TargetID, "flatten": true}, &att)
		if err != nil {
			return "", err
		}
		return att.SessionID, nil
	}
	return "", errors.New("the browser opened no page to sign in with")
}

// prepare installs the capture script, then navigates to the login page. The
// order matters: addScriptToEvaluateOnNewDocument applies to documents loaded
// after it is registered, so navigating first would miss the SPA's own boot.
func prepare(ctx context.Context, c *conn, sessionID, loginURL string) error {
	if err := c.call(ctx, sessionID, "Runtime.enable", nil, nil); err != nil {
		return err
	}
	if err := c.call(ctx, sessionID, "Page.enable", nil, nil); err != nil {
		return err
	}
	if err := c.call(ctx, sessionID, "Runtime.addBinding",
		map[string]any{"name": bindingName}, nil); err != nil {
		return err
	}
	if err := c.call(ctx, sessionID, "Page.addScriptToEvaluateOnNewDocument",
		map[string]any{"source": webauth.ProbeJS(bindingName)}, nil); err != nil {
		return err
	}
	return c.call(ctx, sessionID, "Page.navigate", map[string]any{"url": loginURL}, nil)
}

// sample reads the current cookies and URL.
func sample(ctx context.Context, c *conn, sessionID string) ([]webauth.Cookie, string, error) {
	var cookieResp struct {
		Cookies []cdpCookie `json:"cookies"`
	}
	// Storage.getCookies is the non-deprecated spelling; fall back to the older
	// Network.getAllCookies for browsers that predate it.
	if err := c.call(ctx, sessionID, "Storage.getCookies", nil, &cookieResp); err != nil {
		if err2 := c.call(ctx, sessionID, "Network.getAllCookies", nil, &cookieResp); err2 != nil {
			return nil, "", err
		}
	}

	var eval struct {
		Result struct {
			Value string `json:"value"`
		} `json:"result"`
	}
	err := c.call(ctx, sessionID, "Runtime.evaluate", map[string]any{
		"expression":    "location.href",
		"returnByValue": true,
	}, &eval)
	if err != nil {
		return nil, "", err
	}
	return convertCookies(cookieResp.Cookies), eval.Result.Value, nil
}

// processExited returns a channel closed when the browser process ends, which
// is how a user closing the window reaches the capture loop.
func processExited(cmd *exec.Cmd) <-chan struct{} {
	ch := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(ch)
	}()
	return ch
}

// DefaultProfileDir is the CLI-owned persistent browser profile. Keeping the
// identity-provider session between logins is what stops every sign-in from
// requiring a full SSO round trip.
func DefaultProfileDir(configDir string) string {
	return filepath.Join(configDir, "browser-profile")
}
```

Note `processExited` is started once, at the point shown in `Capture`, and owns
the only `cmd.Wait()` on the process.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./pkg/webauth/cdp/ -race -v`
Expected: PASS — all tests from Tasks 5, 6 and 7.

- [ ] **Step 5: Build and vet**

Run: `make build && go vet ./...`
Expected: both succeed with no output.

- [ ] **Step 6: Commit**

```bash
git add pkg/webauth/cdp/capture.go pkg/webauth/cdp/capture_test.go
git commit -m "$(cat <<'EOF'
feat(cdp): implement webauth.Driver over the DevTools Protocol

Installs the shared capture script before navigating, receives the
Authorization header and email through a Runtime binding, samples
cookies once a second, and defers every success decision to the shared
Tracker. Cookies come from the browser rather than document.cookie, so
HttpOnly cookies are captured too.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 8: Wire `auth login --browser` into the CLI

**Files:**
- Create: `internal/app/auth_browser.go`
- Modify: `internal/app/auth.go:21-85` (`newAuthLoginCmd`), `internal/app/config.go:136-141` (`browserManagedSessionError`)
- Test: `internal/app/auth_browser_test.go`

**Interfaces:**
- Consumes: `cdp.New`, `cdp.Options`, `cdp.DefaultProfileDir`, `cdp.ErrNoBrowser`, `cdp.ErrCancelled`, `cdp.ErrTimeout`, `webauth.PingVerifier`, `auth.Save`, `apiclient.NormalizeBaseURL`.
- Produces: `runBrowserLogin(s *appState, freshProfile bool) error`; `browserCaptureError(err error) error`.

- [ ] **Step 1: Write the failing test**

Create `internal/app/auth_browser_test.go`:

```go
package app

import (
	"errors"
	"strings"
	"testing"

	cerrors "github.com/angelmsger/openobserve-cli/pkg/errors"
	"github.com/angelmsger/openobserve-cli/pkg/webauth/cdp"
)

func TestBrowserCaptureErrorMapsSentinels(t *testing.T) {
	cases := []struct {
		in   error
		code string
	}{
		{cdp.ErrNoBrowser, "BROWSER_NOT_FOUND"},
		{cdp.ErrCancelled, "BROWSER_SIGNIN_CANCELLED"},
		{cdp.ErrTimeout, "BROWSER_SIGNIN_TIMEOUT"},
		{errors.New("something else"), "BROWSER_LAUNCH_FAILED"},
	}
	for _, tc := range cases {
		ce := cerrors.AsCLIError(browserCaptureError(tc.in))
		if ce.Code != tc.code {
			t.Errorf("browserCaptureError(%v).Code = %q, want %q", tc.in, ce.Code, tc.code)
		}
		if len(ce.NextSteps) == 0 {
			t.Errorf("%s carries no next steps; every error must be actionable", tc.code)
		}
	}
}

// Wrapped sentinels must still map: cdp returns them through fmt.Errorf in
// places.
func TestBrowserCaptureErrorUnwraps(t *testing.T) {
	wrapped := errors.Join(errors.New("context"), cdp.ErrNoBrowser)
	if ce := cerrors.AsCLIError(browserCaptureError(wrapped)); ce.Code != "BROWSER_NOT_FOUND" {
		t.Fatalf("wrapped sentinel mapped to %q", ce.Code)
	}
}

// The old error told users to go and use o3. Now that the CLI can sign in
// itself, it must offer its own command.
func TestSessionBrowserManagedOffersBrowserLogin(t *testing.T) {
	ce := cerrors.AsCLIError(browserManagedSessionError("prod"))
	joined := strings.Join(ce.NextSteps, " ") + " " + ce.Hint
	if !strings.Contains(joined, "auth login --browser") {
		t.Fatalf("next steps do not offer browser login: %+v", ce)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/app/ -run 'TestBrowserCapture|TestSessionBrowserManaged' -v`
Expected: FAIL — `undefined: browserCaptureError`.

- [ ] **Step 3: Write `internal/app/auth_browser.go`**

```go
package app

import (
	"errors"
	"os"
	"runtime"

	"github.com/angelmsger/openobserve-cli/internal/auth"
	cerrors "github.com/angelmsger/openobserve-cli/pkg/errors"
	"github.com/angelmsger/openobserve-cli/pkg/webauth"
	"github.com/angelmsger/openobserve-cli/pkg/webauth/cdp"
)

// runBrowserLogin signs in through a real browser and stores the captured
// session under the active context, in the same keychain entry the o3 desktop
// app uses — so signing in through either client authenticates both.
func runBrowserLogin(s *appState, freshProfile bool) error {
	cfg := s.cfg()
	if cfg.BaseURL == "" {
		return cerrors.New(cerrors.CategoryConfig, "NO_BASE_URL",
			"no server configured yet").
			WithNextSteps("openobserve-cli config init")
	}
	if err := requireDisplay(); err != nil {
		return err
	}

	profileDir := ""
	if !freshProfile {
		profileDir = cdp.DefaultProfileDir(s.cfgDir)
	}

	host, err := hostOfBaseURL(cfg.BaseURL)
	if err != nil {
		return err
	}

	driver := cdp.New(cdp.Options{ProfileDir: profileDir})
	verify := webauth.PingVerifier(cfg.BaseURL, s.org(), s.cfg().Defaults.Timeout, s.cfg().Defaults.MaxRetries)

	sess, err := driver.Capture(cfg.BaseURL+"/web/login", host, verify)
	if err != nil {
		return browserCaptureError(err)
	}

	blob, err := webauthEncode(sess)
	if err != nil {
		return err
	}
	cred := auth.Credential{
		Scheme:   auth.SchemeSession,
		Username: sess.Email,
		Secret:   blob,
	}
	backend, err := auth.Save(cfg.BaseURL, cred, s.store)
	if err != nil {
		return cerrors.Wrap(err, cerrors.CategoryConfig, "SAVE_FAILED",
			"captured the session but could not store it")
	}

	out := map[string]any{
		"logged_in": true,
		"base_url":  cfg.BaseURL,
		"org":       s.org(),
		"scheme":    auth.SchemeSession,
		"stored_in": backend,
	}
	if sess.Email != "" {
		out["email"] = sess.Email
	}
	if !sess.ExpiresAt.IsZero() {
		out["expires_at"] = sess.ExpiresAt.UTC().Format(time.RFC3339)
	}
	return s.emit(out)
}

// requireDisplay rejects a browser sign-in on a headless Linux host up front,
// rather than launching a browser that cannot draw and waiting for the timeout.
func requireDisplay() error {
	if runtime.GOOS != "linux" {
		return nil
	}
	if os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != "" {
		return nil
	}
	return cerrors.New(cerrors.CategoryUsage, "BROWSER_NO_DISPLAY",
		"browser sign-in needs a graphical session, but neither DISPLAY nor WAYLAND_DISPLAY is set").
		WithHint("Over SSH or in a container, use a password or token instead.").
		WithNextSteps("openobserve-cli auth login",
			"Set OPENOBSERVE_EMAIL + OPENOBSERVE_PASSWORD, or OPENOBSERVE_TOKEN.")
}

// browserCaptureError translates the driver's sentinel errors into the CLI's
// actionable error shape.
func browserCaptureError(err error) error {
	switch {
	case errors.Is(err, cdp.ErrNoBrowser):
		return cerrors.Wrap(err, cerrors.CategoryConfig, "BROWSER_NOT_FOUND",
			"no Chrome, Chromium, Edge or Brave installation was found").
			WithHint("Browser sign-in drives a Chromium-family browser; set OPENOBSERVE_BROWSER to point at one.").
			WithNextSteps("openobserve-cli auth login",
				"Install Google Chrome, or set OPENOBSERVE_BROWSER=/path/to/browser.")
	case errors.Is(err, cdp.ErrCancelled):
		return cerrors.Wrap(err, cerrors.CategoryAuth, "BROWSER_SIGNIN_CANCELLED",
			"the browser was closed before sign-in completed").
			WithNextSteps("openobserve-cli auth login --browser",
				"openobserve-cli auth login")
	case errors.Is(err, cdp.ErrTimeout):
		return cerrors.Wrap(err, cerrors.CategoryAuth, "BROWSER_SIGNIN_TIMEOUT",
			"sign-in did not complete in time").
			WithNextSteps("openobserve-cli auth login --browser",
				"openobserve-cli auth login")
	default:
		return cerrors.Wrap(err, cerrors.CategoryConfig, "BROWSER_LAUNCH_FAILED",
			"could not run browser sign-in").
			WithHint("Re-run with --verbose to see what the browser reported.").
			WithNextSteps("openobserve-cli auth login",
				"Set OPENOBSERVE_BROWSER=/path/to/browser.")
	}
}
```

Add the imports this needs: `time` for the RFC3339 format, `net/url` for
`hostOfBaseURL`, and `pkgauth "github.com/angelmsger/openobserve-cli/pkg/auth"`
for `webauthEncode`. Define the two small helpers in the same file:

```go
// hostOfBaseURL extracts the host that scopes captured cookies.
func hostOfBaseURL(base string) (string, error) {
	u, err := url.Parse(base)
	if err != nil || u.Host == "" {
		return "", cerrors.Newf(cerrors.CategoryConfig, "BAD_BASE_URL",
			"could not parse the configured server URL %q", base)
	}
	return u.Host, nil
}

// webauthEncode serialises a captured session into the stored envelope.
func webauthEncode(s pkgauth.Session) (string, error) {
	blob, err := pkgauth.EncodeSession(s)
	if err != nil {
		return "", cerrors.Wrap(err, cerrors.CategoryConfig, "AUTH_BAD_SESSION",
			"could not encode the captured session")
	}
	return blob, nil
}
```

- [ ] **Step 4: Add the `--browser` and `--fresh-profile` flags**

In `internal/app/auth.go`, change `newAuthLoginCmd` to declare the flags and
branch before the TTY check — browser sign-in does not need a TTY, only a
display:

```go
func newAuthLoginCmd(s *appState) *cobra.Command {
	var useBrowser bool
	var freshProfile bool
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Store credentials for the active context (interactive)",
		Long: "Prompts for the password (basic) or token, verifies it against the\n" +
			"server, and stores it in the OS keychain. With --browser, signs in\n" +
			"through a real browser window instead and stores the captured session,\n" +
			"which is what SSO instances require. Requires an interactive terminal\n" +
			"(or, for --browser, a graphical session); in CI / agent sandboxes set\n" +
			"OPENOBSERVE_EMAIL + OPENOBSERVE_PASSWORD or OPENOBSERVE_TOKEN instead.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if useBrowser {
				return runBrowserLogin(s, freshProfile)
			}
			if !stdinIsTTY() {
				// ... existing body unchanged from here down
```

Then register the flags before returning the command:

```go
	cmd.Flags().BoolVar(&useBrowser, "browser", false,
		"Sign in through a browser window and store the captured session")
	cmd.Flags().BoolVar(&freshProfile, "fresh-profile", false,
		"Use a throwaway browser profile instead of the remembered one")
	return cmd
}
```

Note the existing function returns the `&cobra.Command{...}` literal directly;
you must assign it to `cmd` first, as shown, for the flag registration to have
something to attach to.

- [ ] **Step 5: Repoint `SESSION_BROWSER_MANAGED`**

In `internal/app/config.go:136-141`, replace `browserManagedSessionError`:

```go
// browserManagedSessionError is returned when a context authenticates with a
// browser-captured session and the caller tried to edit it through a flow that
// prompts for a password or token. Such a session can only be re-established by
// signing in through a browser again — now possible in the CLI itself, so this
// no longer sends the user to the o3 desktop app.
func browserManagedSessionError(contextName string) error {
	return cerrors.Newf(cerrors.CategoryUsage, "SESSION_BROWSER_MANAGED",
		"context %q authenticates with a browser-captured session", contextName).
		WithHint("Sign in through the browser again to refresh it, here or in o3.").
		WithNextSteps("openobserve-cli auth login --browser",
			"openobserve-cli auth status")
}
```

Also remove the `case auth.SchemeSession:` early return in `newAuthLoginCmd`'s
scheme switch — with `--browser` handled above, reaching that switch with
`SchemeSession` means the user ran plain `auth login` on a session context, so
the error is still correct there. Leave it in place; only its text changed.

- [ ] **Step 6: Make `auth logout` remove the browser profile**

The persistent profile holds live identity-provider session cookies. Leaving it
behind after a logout would mean the user believes they signed out while a
browser profile on disk could still walk straight back into the instance.

Add to `internal/app/auth_browser.go`:

```go
// removeBrowserProfile deletes the persistent sign-in profile. A missing
// profile is not an error: most contexts never used browser sign-in.
func removeBrowserProfile(cfgDir string) error {
	dir := cdp.DefaultProfileDir(cfgDir)
	if err := os.RemoveAll(dir); err != nil && !os.IsNotExist(err) {
		return cerrors.Wrap(err, cerrors.CategoryConfig, "PROFILE_REMOVE_FAILED",
			"removed the stored credential but could not remove the browser profile").
			WithHint("Delete " + dir + " by hand to clear the remembered browser session.")
	}
	return nil
}
```

In `internal/app/auth.go`, in `newAuthLogoutCmd`'s `RunE`, after the successful
`auth.Forget(...)` call and before the `s.emit(...)`, add:

```go
			if err := removeBrowserProfile(s.cfgDir); err != nil {
				return err
			}
```

and add `"browser_profile_removed": true` to the emitted map.

Add this test to `internal/app/auth_browser_test.go`:

```go
func TestRemoveBrowserProfileDeletesTheDirectory(t *testing.T) {
	cfgDir := t.TempDir()
	profile := cdp.DefaultProfileDir(cfgDir)
	if err := os.MkdirAll(filepath.Join(profile, "Default"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := removeBrowserProfile(cfgDir); err != nil {
		t.Fatalf("removeBrowserProfile: %v", err)
	}
	if _, err := os.Stat(profile); !os.IsNotExist(err) {
		t.Fatal("browser profile survived logout; the remembered session is still on disk")
	}
}

func TestRemoveBrowserProfileToleratesNoProfile(t *testing.T) {
	if err := removeBrowserProfile(t.TempDir()); err != nil {
		t.Fatalf("a context that never used browser sign-in must log out cleanly: %v", err)
	}
}
```

Add `"os"` and `"path/filepath"` to the test file's imports.

- [ ] **Step 7: Run the tests to verify they pass**

Run: `go test ./internal/app/ -run 'TestBrowserCapture|TestSessionBrowserManaged|TestRemoveBrowserProfile|TestConfigInit' -v`
Expected: PASS. `TestConfigInitRejectsBrowserManagedSessionBeforePrompting` and the existing `browserManagedSessionError` test must still pass — they assert the code and that next steps are non-empty, both unchanged.

- [ ] **Step 8: Full test suite and build**

Run: `make test && make build`
Expected: both succeed.

- [ ] **Step 9: Smoke-test the flag surface**

Run: `./bin/openobserve-cli auth login --help`
Expected: `--browser` and `--fresh-profile` are listed with their descriptions.

- [ ] **Step 10: Commit**

```bash
git add internal/app/auth_browser.go internal/app/auth_browser_test.go internal/app/auth.go internal/app/config.go
git commit -m "$(cat <<'EOF'
feat(auth): add `auth login --browser`

Signs in through a real browser and stores the captured session in the
same keychain entry o3 writes, so either client authenticates both. A
headless Linux host is rejected up front rather than after a ten-minute
timeout, and SESSION_BROWSER_MANAGED now offers this command instead of
directing users to the desktop app.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 9: End-to-end test against a real browser

This is the only test that exercises the injected JavaScript. Nothing tests it
today, in either repo.

**Files:**
- Create: `pkg/webauth/cdp/e2e_test.go`

**Interfaces:**
- Consumes: everything from Tasks 1-7.

- [ ] **Step 1: Write the test**

Create `pkg/webauth/cdp/e2e_test.go`. The build tag keeps it out of `make test`:

```go
//go:build browser_e2e

package cdp

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	pkgauth "github.com/angelmsger/openobserve-cli/pkg/auth"
	"github.com/angelmsger/openobserve-cli/pkg/webauth"
)

// fakeInstance imitates the parts of OpenObserve that capture depends on: a
// login page whose submit handler sets a cookie AND sends an Authorization
// header over XHR (as the real SPA's axios client does), and an API that
// answers only when one of the two is presented.
func fakeInstance(t *testing.T) *httptest.Server {
	t.Helper()
	// The test cannot click, so the page also signs itself in shortly after
	// load. That delay is deliberate: it forces capture to observe a
	// pre-login state first, exercising the Tracker's "not yet" path.
	const loginHTML = `<!doctype html><html><body>
<button id="go">Sign in</button>
<script>
function signIn(){
  document.cookie = 'auth_tokens=secret-token; path=/';
  localStorage.setItem('user_info', JSON.stringify({email:'ops@example.com'}));
  var x = new XMLHttpRequest();
  x.open('GET', '/api/organizations', true);
  x.setRequestHeader('Authorization', 'Basic ops-durable-token');
  x.onload = function(){ location.href = '/web/logs'; };
  x.send();
}
document.getElementById('go').onclick = signIn;
setTimeout(signIn, 1500);
</script></body></html>`

	mux := http.NewServeMux()
	mux.HandleFunc("/web/login", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(loginHTML))
	})
	mux.HandleFunc("/web/logs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<!doctype html><html><body>signed in</body></html>`))
	})
	mux.HandleFunc("/api/organizations", func(w http.ResponseWriter, r *http.Request) {
		authed := r.Header.Get("Authorization") == "Basic ops-durable-token"
		if c, err := r.Cookie("auth_tokens"); err == nil && c.Value == "secret-token" {
			authed = true
		}
		if !authed {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"identifier":"default","name":"default"}]}`))
	})
	return httptest.NewServer(mux)
}

func TestCaptureAgainstRealBrowser(t *testing.T) {
	if _, err := findBrowser(); err != nil {
		t.Skipf("no browser available: %v", err)
	}
	srv := fakeInstance(t)
	defer srv.Close()

	host := mustHost(t, srv.URL)
	// The real verifier, not a stub: apiclient.Ping issues
	// GET /api/organizations, which is exactly what the fake instance serves
	// and gates on credentials. So this exercises the whole chain — injected
	// script, binding, cookie sampling, Tracker, authenticated probe.
	verify := webauth.PingVerifier(srv.URL, "default", 5*time.Second, 0)

	d := New(Options{Timeout: 90 * time.Second})

	done := make(chan struct{})
	var sess pkgauth.Session
	var captureErr error
	go func() {
		defer close(done)
		sess, captureErr = d.Capture(srv.URL+"/web/login", host, verify)
	}()

	select {
	case <-done:
	case <-time.After(100 * time.Second):
		t.Fatal("capture never returned")
	}

	if captureErr != nil {
		t.Fatalf("Capture: %v", captureErr)
	}
	if !strings.Contains(sess.Cookies, "auth_tokens=secret-token") {
		t.Errorf("cookies not captured: %q", sess.Cookies)
	}
	if sess.Authorization != "Basic ops-durable-token" {
		t.Errorf("Authorization not captured from XHR: %q", sess.Authorization)
	}
	if sess.Email != "ops@example.com" {
		t.Errorf("email not captured from localStorage: %q", sess.Email)
	}
}

func mustHost(t *testing.T, raw string) string {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u.Host
}
```

- [ ] **Step 2: Run the end-to-end test**

Run: `go test -tags browser_e2e ./pkg/webauth/cdp/ -run TestCaptureAgainstRealBrowser -v -timeout 3m`
Expected: PASS, having driven a real browser window. If no browser is installed, SKIP with a clear message.

- [ ] **Step 3: Confirm it stays out of the default suite**

Run: `make test`
Expected: PASS, and the e2e test does not appear in the output.

- [ ] **Step 4: Commit**

```bash
git add pkg/webauth/cdp/e2e_test.go
git commit -m "$(cat <<'EOF'
test(cdp): drive a real browser end to end

Serves a fake OpenObserve SPA that sets a cookie, sends the durable
Authorization header over XHR and writes user_info to localStorage, then
captures it with a real browser. This is the only test that exercises
the injected script; it is build-tagged so `make test` stays fast and
works on machines with no browser.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 10: Documentation

**Files:**
- Modify: `CHANGELOG.md`, `README.md`, `docs/technical-design.md`, `skills/openobserve/SKILL.md`, `docs/cli/` (regenerated)

- [ ] **Step 1: Correct the auth section of the technical design**

In `docs/technical-design.md`, the Auth section currently ends the scheme list
with "(browser-captured cookies with an optional Authorization fallback,
established and managed by o3)". That is no longer true. Replace that
parenthetical with:

```
(browser-captured cookies with an optional Authorization fallback, established
by `auth login --browser` here or by o3's native sign-in — both write the same
keychain entry)
```

Then add a paragraph after the Auth section:

```markdown
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
```

- [ ] **Step 2: Document the flag in the README**

In `README.md`, find the authentication/configuration section and add:

```markdown
### Browser sign-in

Instances behind SSO cannot use a password or a generated token. Sign in
through a real browser instead:

```bash
openobserve-cli auth login --browser
```

A browser window opens on your instance's login page; once you are signed in
the CLI captures the session, verifies it with an authenticated request, and
stores it in the OS keychain. The captured session is the same one the
[o3](https://github.com/angelmsger/o3) desktop app uses, so signing in through
either leaves the other authenticated.

Requires a Chromium-family browser (Chrome, Chromium, Edge or Brave) and a
graphical session. Set `OPENOBSERVE_BROWSER` to choose a specific one. The
browser profile is remembered under `~/.angelmsger/openobserve/browser-profile`
so you are not sent through SSO on every login; `--fresh-profile` opts out, and
`auth logout` removes it.
```

- [ ] **Step 3: Update the companion Skill**

In `skills/openobserve/SKILL.md`, find where authentication setup is described
and add `openobserve-cli auth login --browser` as the SSO path, alongside the
existing `OPENOBSERVE_EMAIL`/`OPENOBSERVE_PASSWORD`/`OPENOBSERVE_TOKEN`
guidance. Keep the existing note that agent sandboxes should use env vars —
`--browser` needs a display and is not for sandboxes.

- [ ] **Step 4: Regenerate the CLI reference**

Run: `make docs`
Expected: `docs/cli/` picks up the two new flags. If the target has a different
name, check the Makefile comment above `gen-docs`.

- [ ] **Step 5: Add the CHANGELOG entry**

At the top of `CHANGELOG.md`, under a new `## [Unreleased]` heading:

```markdown
## [Unreleased]

### Added

- **`auth login --browser` signs in through a real browser.** Instances behind
  SSO, where neither a password nor a generated token works, can now be
  authenticated from the CLI alone — previously the only path was installing the
  o3 desktop app. Capture drives a Chromium-family browser over the DevTools
  Protocol, so it works on macOS, Linux and Windows, and stores the session in
  the same keychain entry o3 uses: signing in through either client
  authenticates both. The browser profile is remembered so SSO is not repeated
  on every login; `--fresh-profile` opts out.
- **`pkg/webauth` is the shared capture core.** The cookie shaping, the
  login-success heuristic, the injected capture script and the verify-by-Ping
  contract now live in one public package used by both the CLI and o3, instead
  of being duplicated — the capture script in particular was previously trapped
  in an Objective-C string literal where a fix could never reach the CLI.

### Changed

- **`SESSION_BROWSER_MANAGED` now points at the CLI's own browser sign-in.**
  Running `auth login` or `config init` against a context that uses a captured
  session used to direct the user to the o3 desktop app; it now offers
  `openobserve-cli auth login --browser`.
```

- [ ] **Step 6: Verify the punctuation and links**

Run: `grep -n '[，。、；：？！（）]' CHANGELOG.md README.md docs/technical-design.md`
Expected: no output. Project docs are English and use ASCII punctuation.

- [ ] **Step 7: Commit**

```bash
git add CHANGELOG.md README.md docs/technical-design.md skills/openobserve/SKILL.md docs/cli/
git commit -m "$(cat <<'EOF'
docs: document browser sign-in

Records the new flag, the shared pkg/webauth core, and corrects the
technical design's claim that browser sessions are established and
managed by o3 — the CLI now establishes them too.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 11: Migrate o3 onto the shared core

This task is in a **different repository**: `~/Development/Workspaces/o3`. It has
a `go.work` and a `replace` pointing at the local `openobserve-cli` checkout, so
it picks up Tasks 1-7 with no version bump.

**Files (in `o3`):**
- Delete: `internal/webauth/session.go`, `internal/webauth/capture.go`, `internal/webauth/session_test.go`, `internal/webauth/webauth_other.go`
- Modify: `internal/webauth/webauth_darwin.go`, `internal/webauth/capture_test.go`, `app.go:572-608`
- Create: `internal/webauth/driver.go`

- [ ] **Step 1: Delete the files that moved**

```bash
cd ~/Development/Workspaces/o3
git rm internal/webauth/session.go internal/webauth/session_test.go internal/webauth/webauth_other.go
```

Keep `capture.go` and `capture_test.go` for now — they still hold the WebKit
payload decoder.

- [ ] **Step 2: Reduce `capture.go` to the WebKit payload decoder**

Edit `internal/webauth/capture.go` down to only `nativeProbe`, `nativeCookie`
and `parseProbe`, converting `parseProbe` to return the shared cookie type:

```go
package webauth

import (
	"encoding/json"
	"time"

	shared "github.com/angelmsger/openobserve-cli/pkg/webauth"
)

// nativeProbe is the JSON payload the native window hands to Go on each capture
// probe: the current WebView URL plus the cookies and any Authorization/email
// observed. Parsing it here (rather than in Objective-C) keeps the native shell
// thin and this decoding unit-testable.
type nativeProbe struct {
	URL           string         `json:"url"`
	Authorization string         `json:"authorization"`
	Email         string         `json:"email"`
	Cookies       []nativeCookie `json:"cookies"`
}

type nativeCookie struct {
	Name     string  `json:"name"`
	Value    string  `json:"value"`
	Domain   string  `json:"domain"`
	Path     string  `json:"path"`
	Expires  float64 `json:"expires"` // unix seconds; 0 for a session cookie
	Secure   bool    `json:"secure"`
	HTTPOnly bool    `json:"httpOnly"`
}

// parseProbe decodes a native probe payload into cookies plus the observed URL,
// Authorization header, and email.
func parseProbe(data []byte) (cookies []shared.Cookie, currentURL, authorization, email string, err error) {
	var p nativeProbe
	if err = json.Unmarshal(data, &p); err != nil {
		return nil, "", "", "", err
	}
	cookies = make([]shared.Cookie, 0, len(p.Cookies))
	for _, c := range p.Cookies {
		var exp time.Time
		if c.Expires > 0 {
			exp = time.Unix(int64(c.Expires), 0).UTC()
		}
		cookies = append(cookies, shared.Cookie{
			Name: c.Name, Value: c.Value, Domain: c.Domain, Path: c.Path,
			Expires: exp, Secure: c.Secure, HTTPOnly: c.HTTPOnly,
		})
	}
	return cookies, p.URL, p.Authorization, p.Email, nil
}
```

In `capture_test.go`, delete every test except `TestParseProbe` and
`TestProbeFromNativeLoginInstance` (the two that exercise `parseProbe`), and
change any `Cookie` references to `shared.Cookie`, adding the same import.

- [ ] **Step 3: Add the non-darwin driver**

Create `internal/webauth/driver.go`:

```go
package webauth

import (
	shared "github.com/angelmsger/openobserve-cli/pkg/webauth"
	"github.com/angelmsger/openobserve-cli/pkg/webauth/cdp"
)

// Driver returns the browser sign-in transport for this platform. macOS uses
// the native WKWebView window (see webauth_darwin.go); every other platform
// drives a Chromium-family browser over the DevTools Protocol, which is why
// sign-in is no longer macOS-only.
func Driver(profileDir string) shared.Driver {
	return driverForPlatform(profileDir)
}

// cdpDriver is the shared fallback, used directly off darwin.
func cdpDriver(profileDir string) shared.Driver {
	return cdp.New(cdp.Options{ProfileDir: profileDir})
}
```

Create `internal/webauth/driver_other.go`:

```go
//go:build !darwin

package webauth

import shared "github.com/angelmsger/openobserve-cli/pkg/webauth"

func driverForPlatform(profileDir string) shared.Driver { return cdpDriver(profileDir) }
```

Create `internal/webauth/driver_darwin.go`:

```go
//go:build darwin

package webauth

import shared "github.com/angelmsger/openobserve-cli/pkg/webauth"

// nativeDriver adapts the WKWebView capture in webauth_darwin.go to the shared
// Driver interface.
type nativeDriver struct{}

func (nativeDriver) Capture(loginURL, host string, verify shared.VerifyFunc) (pkgauth.Session, error) {
	return Capture(loginURL, host, verify)
}

func driverForPlatform(_ string) shared.Driver { return nativeDriver{} }
```

Add `pkgauth "github.com/angelmsger/openobserve-cli/pkg/auth"` to
`driver_darwin.go`'s imports.

- [ ] **Step 4: Point the darwin shell at the shared core**

In `internal/webauth/webauth_darwin.go`:

- Add the import `shared "github.com/angelmsger/openobserve-cli/pkg/webauth"`.
- Change the `VerifyFunc` field and parameter types to `shared.VerifyFunc`.
- Replace the body of `webauthProbe` from `sess := AssembleSession(...)` onward
  with a `shared.Tracker`. Create the tracker once in `Capture` and store it
  alongside `captureCh`:

```go
var captureTracker *shared.Tracker
```

In `Capture`, after setting `captureHost`, add:

```go
	captureTracker = shared.NewTracker(host, verify)
```

Then `webauthProbe` becomes:

```go
//export webauthProbe
func webauthProbe(cjson *C.char) C.int {
	data := []byte(C.GoString(cjson))
	captureMu.Lock()
	ch, tracker := captureCh, captureTracker
	captureMu.Unlock()
	if ch == nil || tracker == nil {
		return 0
	}
	cookies, currentURL, authz, email, err := parseProbe(data)
	if err != nil {
		return 0
	}
	// Verification makes a network request; never block the main thread on it.
	go func() {
		sess, ok := tracker.Observe(cookies, currentURL, authz, email)
		if !ok {
			return
		}
		captureMu.Lock()
		active := captureCh == ch
		captureMu.Unlock()
		if active {
			log.Printf("[webauth] probe success email=%q url=%s", email, currentURL)
			deliver(ch, captureResult{session: sess})
			C.o3FinishWebAuth()
		}
	}()
	return 0
}
```

Delete the now-unused `verifyBusy` and `verifySig` variables and the
`captureVerify` variable — the tracker owns all of that state.

- [ ] **Step 5: Update the injected script in the Objective-C shell**

In `internal/webauth/webauth_darwin.m`, the injected script is now owned by Go.
Replace the `NSString *js = @"...";` literal (lines ~127-140) with a value
passed in from Go. The simplest change preserving the existing C entry point:
add a parameter to `o3StartWebAuth`.

In `webauth_darwin.h`, change the declaration to:

```c
void o3StartWebAuth(const char *url, const char *probeJS);
```

In `webauth_darwin.m`, take the second argument and use it in place of the
literal, wrapping the delivery name so the script's `__DELIVER__` calls reach
WebKit. Since `ProbeJS` already substitutes the name, Go passes a script whose
delivery function must exist; define it by prepending a shim in Go rather than
in Objective-C:

In `webauth_darwin.go`'s `Capture`:

```go
	const bindingName = "__o3Probe"
	shim := "window." + bindingName +
		"=function(s){try{window.webkit.messageHandlers.o3.postMessage(JSON.parse(s));}catch(e){}};"
	js := shim + shared.ProbeJS(bindingName)

	cURL := C.CString(loginURL)
	cJS := C.CString(js)
	C.o3StartWebAuth(cURL, cJS)
	C.free(unsafe.Pointer(cURL))
	C.free(unsafe.Pointer(cJS))
```

The existing `userContentController` message handler already expects an object
with `authorization` / `email` keys, so `JSON.parse` in the shim keeps it
working unchanged.

- [ ] **Step 6: Replace `sessionVerifier` with the shared one**

In `app.go`, delete `sessionVerifier` (lines 572-596) and change `BrowserSignIn`
to use the shared verifier and the platform driver:

```go
	d := a.fileDefaults()
	verify := shared.PingVerifier(base, orgOrDefault(org), d.Timeout, d.MaxRetries)
	sess, err := webauth.Driver(browserProfileDir()).Capture(base+"/web/login", host, verify)
```

Add `shared "github.com/angelmsger/openobserve-cli/pkg/webauth"` to `app.go`'s
imports, and define `browserProfileDir()` in `app.go`:

```go
// browserProfileDir is o3's own persistent sign-in profile. It must NOT be the
// CLI's: two browser processes pointed at one user-data-dir refuse to start.
// Ignored on darwin, where the native WebView keeps its own data store.
func browserProfileDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".angelmsger", "o3", "browser-profile")
}
```

Add `"os"` and `"path/filepath"` to `app.go`'s imports if they are not already
present.

- [ ] **Step 7: Build and test o3**

Run: `cd ~/Development/Workspaces/o3 && go build ./... && go test ./...`
Expected: PASS. `webauth_e2e_test.go`'s four tests must still pass — they cover
session verification, capture/store/replay and the keychain round trip, and are
the check that the migration did not change behaviour.

- [ ] **Step 8: Verify the CLI still builds against the same core**

Run: `cd ~/Development/Workspaces/oa-cli/src/openobserve-cli && make test && make build`
Expected: PASS. Both repos build against one copy of the core.

- [ ] **Step 9: Commit in o3**

```bash
cd ~/Development/Workspaces/o3
git add -A
git commit -m "$(cat <<'EOF'
refactor(webauth): use the shared capture core from openobserve-cli

Deletes the copies of the cookie logic, the success heuristic and the
session verifier now living in openobserve-cli/pkg/webauth, and takes
the injected script from there too instead of an Objective-C literal.
The WKWebView window stays as the macOS transport; Windows and Linux
gain real browser sign-in through the CDP driver, replacing the stub
that refused with "only supported on macOS".

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 10: Update the o3 CHANGELOG and README**

Add an `## [Unreleased]` CHANGELOG entry recording that browser sign-in works on
Windows and Linux, and that the capture core is shared. In `README.md`, correct
any statement that sign-in is macOS-only. Commit with:

```bash
git add CHANGELOG.md README.md
git commit -m "$(cat <<'EOF'
docs: record cross-platform browser sign-in

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 12: Bump the workspace submodule pointer

**Files:**
- Modify: `~/Development/Workspaces/oa-cli` submodule pointer for `src/openobserve-cli`

- [ ] **Step 1: Push the feature branch**

```bash
cd ~/Development/Workspaces/oa-cli/src/openobserve-cli
git push -u origin claude/browser-login-session-capture
```

- [ ] **Step 2: Confirm the commits are on the remote**

Run: `git branch -r --contains HEAD`
Expected: lists `origin/claude/browser-login-session-capture`. The workspace
pointer must never reference a commit that exists only locally.

- [ ] **Step 3: Bump the pointer**

```bash
cd ~/Development/Workspaces/oa-cli
git add src/openobserve-cli
git commit -m "$(cat <<'EOF'
chore: bump openobserve-cli pointer (browser sign-in)

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

## Verification checklist

Before calling this plan complete:

- [ ] `make test && make build` pass in `openobserve-cli`.
- [ ] `go build ./... && go test ./...` pass in `o3`.
- [ ] `go test -tags browser_e2e ./pkg/webauth/cdp/ -timeout 3m` passes on a machine with a browser.
- [ ] `go list -deps ./pkg/webauth | grep coder/websocket` prints nothing.
- [ ] `./bin/openobserve-cli auth login --browser` against a real instance stores a session, and `./bin/openobserve-cli auth status` then reports `authenticated: true`.
- [ ] Running o3 against the same context finds that session already valid, and vice versa.
- [ ] `./bin/openobserve-cli auth logout` removes both the keychain entry and the browser profile directory.
