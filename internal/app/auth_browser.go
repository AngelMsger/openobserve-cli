package app

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"runtime"
	"time"

	"github.com/angelmsger/openobserve-cli/internal/auth"
	"github.com/angelmsger/openobserve-cli/internal/config"
	"github.com/angelmsger/openobserve-cli/pkg/apiclient"
	pkgauth "github.com/angelmsger/openobserve-cli/pkg/auth"
	cerrors "github.com/angelmsger/openobserve-cli/pkg/errors"
	"github.com/angelmsger/openobserve-cli/pkg/webauth"
	"github.com/angelmsger/openobserve-cli/pkg/webauth/cdp"
)

// runBrowserLogin signs in through a real browser and stores the captured
// session under the active context, in the same keychain entry the o3 desktop
// app uses — so signing in through either client authenticates both.
func runBrowserLogin(s *appState, freshProfile bool) error {
	profileDir := ""
	if !freshProfile {
		profileDir = cdp.DefaultProfileDir(s.cfgDir)
	}
	return runBrowserLoginWith(s, cdp.New(cdp.Options{ProfileDir: profileDir}))
}

// runBrowserLoginWith is the testable core of runBrowserLogin: everything
// except choosing the driver. Splitting the driver out lets tests pass a fake
// that records the login URL and host it was handed — the only way to pin
// that the base URL was normalized before use, since NormalizeBaseURL and
// hostOfBaseURL individually stay correct even if a caller forgets to chain
// them.
func runBrowserLoginWith(s *appState, driver webauth.Driver) error {
	cfg := s.cfg()
	if cfg.BaseURL == "" {
		return cerrors.New(cerrors.CategoryConfig, "NO_BASE_URL",
			"no server configured yet").
			WithNextSteps("openobserve-cli config init")
	}
	// Normalize before anything derives from it. OPENOBSERVE_URL never passes
	// through `config init`, so a bare host:port or a trailing slash reaches
	// here verbatim; every other client path normalizes.
	baseURL, err := apiclient.NormalizeBaseURL(cfg.BaseURL)
	if err != nil {
		return err
	}
	if err := requireDisplay(); err != nil {
		return err
	}

	host, err := hostOfBaseURL(baseURL)
	if err != nil {
		return err
	}

	verify := webauth.PingVerifier(baseURL, s.org(), s.cfg().Defaults.Timeout, s.cfg().Defaults.MaxRetries)

	// The command then blocks for as long as the user takes to sign in — up to
	// ten minutes — so say what is happening. stdout carries the result
	// document only, so this notice goes to stderr.
	fmt.Fprintln(os.Stderr,
		"Opening a browser window; complete the sign-in there. This command waits until it finishes.")

	sess, err := driver.Capture(baseURL+"/web/login", host, verify)
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
	// The keychain key deliberately uses the RAW base URL: auth.Resolve and
	// auth.Forget (used by every other command and by `auth logout`) key off
	// cfg.BaseURL too, exactly as verifyAndSave does for basic/token. Saving
	// under the normalized value here while lookups elsewhere use the raw one
	// would store a credential nothing could find whenever the two disagree
	// (e.g. a trailing slash, or a bare host:port from OPENOBSERVE_URL).
	backend, err := auth.Save(cfg.BaseURL, cred, s.store)
	if err != nil {
		return cerrors.Wrap(err, cerrors.CategoryConfig, "SAVE_FAILED",
			"captured the session but could not store it")
	}

	// Storing the secret is only half the job: without `auth.scheme: session`
	// in the config file nothing would ever resolve it.
	ctxName, err := persistSessionScheme(s, baseURL, s.org(), sess.Email)
	if err != nil {
		return err
	}

	out := map[string]any{
		"logged_in": true,
		"base_url":  baseURL,
		"org":       s.org(),
		"scheme":    auth.SchemeSession,
		"stored_in": backend,
		"context":   ctxName,
	}
	if sess.Email != "" {
		out["email"] = sess.Email
	}
	if !sess.ExpiresAt.IsZero() {
		out["expires_at"] = sess.ExpiresAt.UTC().Format(time.RFC3339)
	}
	return s.emit(out)
}

// persistSessionScheme records `auth.scheme: session` (and the captured email
// as the username) for the active context in the config file, then mirrors it
// into the in-memory resolved config. It returns the context that was written.
//
// This is what makes a captured session usable at all. `auth.Resolve` defaults
// an unset scheme to `basic` and `config init` refuses to write `session`, so
// without this step the credential would sit in the keychain under an account
// key (which mixes the scheme in) that no later command ever looks up: the
// login would report success and every subsequent command would either fail to
// find a credential or silently keep using the context's stale password.
//
// Only the active context's auth block changes. Every other context, the
// shared defaults and the current-context pointer are read back from disk and
// written out again unchanged, so an unrelated field cannot be clobbered. When
// nothing would change, the file is not rewritten at all.
//
// The context NAME comes from the config file (or OPENOBSERVE_CONTEXT), while
// the server can be overridden for a single invocation by OPENOBSERVE_URL or
// --base-url. The two can therefore disagree, so before writing anything this
// checks that the on-disk context actually resolves to the server that was
// just authenticated against — see contextBaseURLMismatchError.
func persistSessionScheme(s *appState, baseURL, org, email string) (string, error) {
	file, _, err := config.ReadFile(s.cfgDir)
	if err != nil {
		return "", cerrors.Wrap(err, cerrors.CategoryConfig, "CONFIG_READ",
			"failed to read config")
	}

	name := s.resolved.ActiveContext
	if name == "" {
		name = config.DefaultContextName
	}

	// The credential was filed under auth.AccountKey(cfg.BaseURL, session) —
	// the RAW base URL, matching what auth.Resolve computes later. The context
	// that carries `scheme: session` must produce that same key, or the scheme
	// only sends every later command looking somewhere the credential is not.
	rawBase := s.cfg().BaseURL
	wantKey := auth.AccountKey(rawBase, auth.SchemeSession)

	nc, found := file.Context(name)
	dirty := !found
	switch {
	case !found:
		// No context on disk yet: the server came from OPENOBSERVE_URL or
		// --base-url. Materialize one so the captured session survives the
		// environment that created it. A brand-new context for this server is
		// legitimate; only an EXISTING context naming a different one is the
		// hazard.
		nc = config.NamedContext{Name: name, BaseURL: contextBaseURL(baseURL, rawBase), Org: org}
	case nc.BaseURL == "":
		// A context that never recorded a server is not pointing at a
		// different one; fill it in rather than refusing.
		nc.BaseURL = contextBaseURL(baseURL, rawBase)
		dirty = true
	case auth.AccountKey(nc.BaseURL, auth.SchemeSession) != wantKey:
		return "", contextBaseURLMismatchError(name, nc.BaseURL, baseURL)
	}

	want := config.AuthConfig{Scheme: auth.SchemeSession, Username: nc.Auth.Username}
	if email != "" {
		want.Username = email
	}
	s.resolved.Config.Auth = want
	if !dirty && nc.Auth == want {
		return nc.Name, nil
	}

	nc.Auth = want
	file.Upsert(nc)
	if file.CurrentContext == "" {
		file.CurrentContext = nc.Name
	}
	if err := config.WriteFile(s.cfgDir, file); err != nil {
		return "", cerrors.Wrap(err, cerrors.CategoryConfig, "CONFIG_WRITE",
			"stored the captured session but could not record it in the config file")
	}
	return nc.Name, nil
}

// contextBaseURL picks the base URL to record for a context that does not have
// one yet. The normalized form is what a user wants to read in their config
// file, but the credential was filed under auth.AccountKey of the RAW base URL
// (see the comment on auth.Save above), so the raw form wins whenever
// normalizing would move the account key. That happens for a scheme-less
// OPENOBSERVE_URL with a trailing slash — "o2.example.com:8080/" hashes whole
// because url.Parse finds no Host, while its normalized form
// "http://o2.example.com:8080" hashes to the host alone. Recording the
// normalized value there would leave the credential orphaned the moment the
// environment variable went away.
func contextBaseURL(normalized, raw string) string {
	if auth.AccountKey(normalized, auth.SchemeSession) == auth.AccountKey(raw, auth.SchemeSession) {
		return normalized
	}
	return raw
}

// contextBaseURLMismatchError reports that the session was captured and stored
// but `auth.scheme: session` was deliberately NOT written, because the active
// context on disk names a different server.
//
// The context name comes from the config file; OPENOBSERVE_URL and --base-url
// override only the server. Rewriting that context's auth block anyway would
// point its credential lookup at a server the session was never filed under —
// and discard the username it recorded — so the next command run without the
// override would fail to resolve anything. Failing loudly is the only honest
// option: the credential really is stored, so silently skipping the write
// would leave the user with a successful login they cannot use and no idea why.
func contextBaseURLMismatchError(name, ctxBaseURL, loggedInTo string) error {
	return cerrors.Newf(cerrors.CategoryConfig, "CONTEXT_BASE_URL_MISMATCH",
		"stored the captured session for %s, but the active context %q points at %s, "+
			"so `auth.scheme: session` was not recorded", loggedInTo, name, ctxBaseURL).
		WithHint("OPENOBSERVE_URL or --base-url overrode the server, but not the context name, "+
			"so the context that would have been rewritten is not the one that was signed in to.").
		WithNextSteps(
			"openobserve-cli config contexts",
			"Re-run `openobserve-cli auth login --browser` with a context whose base_url is "+
				loggedInTo+" selected (--use-context <name>, or `config use-context <name>`).",
			"openobserve-cli config init")
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
			WithHint("The underlying failure is in `message`; the browser's own output is not captured.").
			WithNextSteps("openobserve-cli auth login",
				"Set OPENOBSERVE_BROWSER=/path/to/browser.")
	}
}

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

// removeBrowserProfile deletes the persistent sign-in profile and reports
// whether one was actually there. A missing profile is not an error: most
// contexts never used browser sign-in, and `auth logout` must not claim to have
// removed a profile that never existed.
func removeBrowserProfile(cfgDir string) (bool, error) {
	dir := cdp.DefaultProfileDir(cfgDir)
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, profileRemoveFailed(err, dir)
	}
	if err := os.RemoveAll(dir); err != nil && !os.IsNotExist(err) {
		return false, profileRemoveFailed(err, dir)
	}
	return true, nil
}

func profileRemoveFailed(err error, dir string) error {
	return cerrors.Wrap(err, cerrors.CategoryConfig, "PROFILE_REMOVE_FAILED",
		"removed the stored credential but could not remove the browser profile").
		WithHint("Delete " + dir + " by hand to clear the remembered browser session.")
}
