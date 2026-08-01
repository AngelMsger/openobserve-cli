package app

import (
	"errors"
	"net/url"
	"os"
	"runtime"
	"time"

	"github.com/angelmsger/openobserve-cli/internal/auth"
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

	out := map[string]any{
		"logged_in": true,
		"base_url":  baseURL,
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
