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

// exitGrace bounds how long the kill defer waits for the browser process to
// actually go away. Waiting matters on Windows, where removing the temporary
// profile directory fails with a sharing violation while the browser still
// holds handles inside it; the bound keeps a wedged process from hanging the
// command instead.
const exitGrace = 5 * time.Second

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

// detachedIsOurs reports whether a Target.detachedFromTarget event refers to
// the page session Capture is driving. The protocol carries the detached
// session in the event's own params; anything else (a worker, a
// service-worker target, another tab) must not cancel the sign-in.
func detachedIsOurs(params json.RawMessage, sessionID string) bool {
	if sessionID == "" {
		return false
	}
	var p struct {
		SessionID string `json:"sessionId"`
	}
	if json.Unmarshal(params, &p) != nil {
		return false
	}
	return p.SessionID == sessionID
}

// killAndReap kills the browser and waits, for at most grace, for launch's
// reaper to observe the exit. Callers rely on the wait: a deferred removal of a
// temporary profile directory registered before this one runs afterwards, and
// on Windows it fails with a sharing violation while the browser still has the
// profile open — silently leaving a directory full of live identity-provider
// cookies behind. The bound means a process that refuses to die delays the
// command instead of hanging it.
func killAndReap(cmd *exec.Cmd, exited <-chan struct{}, grace time.Duration) {
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	if exited == nil {
		return
	}
	timer := time.NewTimer(grace)
	defer timer.Stop()
	select {
	case <-exited:
	case <-timer.C:
	}
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
		// Registered BEFORE the kill defer below, so LIFO ordering runs the
		// kill-and-reap first and this removal second — the browser is gone by
		// the time the directory is deleted. A throwaway profile holds live
		// identity-provider cookies, and removing it is the entire point of
		// --fresh-profile, so it must not be left behind.
		defer os.RemoveAll(tmp)
		profileDir = tmp
	}

	ctx, cancel := context.WithTimeout(context.Background(), d.timeout)
	defer cancel()

	cmd, wsURL, exited, err := launch(ctx, exe, profileDir)
	if err != nil {
		return pkgauth.Session{}, err
	}
	// Kill only — never Wait here. launch owns the single Wait on this process
	// and reports exit through the `exited` channel; a second Wait would error
	// and race the first. Draining that channel is what makes the temporary
	// profile removal registered above safe.
	defer killAndReap(cmd, exited, exitGrace)

	c, err := dial(ctx, wsURL)
	if err != nil {
		return pkgauth.Session{}, err
	}
	defer c.Close()

	sessionID, err := attachToPage(ctx, c)
	if err != nil {
		return pkgauth.Session{}, err
	}

	// Closing the sign-in window does NOT end the browser process on macOS —
	// the app stays running with no windows — so `exited` never fires. What
	// does fire is Target.detachedFromTarget for our page session, after which
	// every sample() fails permanently ("Session with given id not found") and
	// the poll loop would spin until the ten-minute timeout. Both signals are
	// real; watch for both.
	detached := make(chan struct{})
	var detachOnce sync.Once
	c.onEvent("Target.detachedFromTarget", func(_ string, params json.RawMessage) {
		if !detachedIsOurs(params, sessionID) {
			return
		}
		detachOnce.Do(func() { close(detached) })
	})

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

	if err := prepare(ctx, c, sessionID, loginURL, host); err != nil {
		return pkgauth.Session{}, err
	}

	tracker := webauth.NewTracker(host, verify)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-exited:
			return pkgauth.Session{}, ErrCancelled
		case <-detached:
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
//
// That same breadth is why the script is scoped to host: it runs in every
// document the target loads, including an identity provider on another origin.
func prepare(ctx context.Context, c *conn, sessionID, loginURL, host string) error {
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
		map[string]any{"source": webauth.ProbeJS(bindingName, host)}, nil); err != nil {
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

// DefaultProfileDir is the CLI-owned persistent browser profile. Keeping the
// identity-provider session between logins is what stops every sign-in from
// requiring a full SSO round trip.
func DefaultProfileDir(configDir string) string {
	return filepath.Join(configDir, "browser-profile")
}
