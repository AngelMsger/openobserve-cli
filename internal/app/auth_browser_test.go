package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/angelmsger/openobserve-cli/internal/auth"
	"github.com/angelmsger/openobserve-cli/internal/config"
	"github.com/angelmsger/openobserve-cli/pkg/apiclient"
	pkgauth "github.com/angelmsger/openobserve-cli/pkg/auth"
	cerrors "github.com/angelmsger/openobserve-cli/pkg/errors"
	"github.com/angelmsger/openobserve-cli/pkg/webauth"
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

// TestBrowserLoginNormalizesBaseURL pins the normalization boundary
// runBrowserLogin relies on. OPENOBSERVE_URL never passes through
// `config init`, so a bare host:port or a trailing slash can reach
// runBrowserLogin verbatim; hostOfBaseURL and the login URL it builds must be
// derived from the *normalized* base URL, not the raw config value.
func TestBrowserLoginNormalizesBaseURL(t *testing.T) {
	cases := []struct{ raw, wantHost, wantLogin string }{
		{"localhost:5080", "localhost:5080", "http://localhost:5080/web/login"},
		{"http://o2.example.com/", "o2.example.com", "http://o2.example.com/web/login"},
		{"https://o2.example.com", "o2.example.com", "https://o2.example.com/web/login"},
	}
	for _, tc := range cases {
		base, err := apiclient.NormalizeBaseURL(tc.raw)
		if err != nil {
			t.Fatalf("%q: normalize: %v", tc.raw, err)
		}
		host, err := hostOfBaseURL(base)
		if err != nil {
			t.Fatalf("%q: host: %v", tc.raw, err)
		}
		if host != tc.wantHost {
			t.Errorf("%q: host = %q, want %q", tc.raw, host, tc.wantHost)
		}
		if got := base + "/web/login"; got != tc.wantLogin {
			t.Errorf("%q: login URL = %q, want %q", tc.raw, got, tc.wantLogin)
		}
	}
}

// failingKeyring refuses every operation, routing Store.Save/Load/Delete to
// the file fallback so tests never touch the real OS keychain. Mirrors the
// unexported type of the same name in internal/auth's own tests, redefined
// here because that one is package-private.
type failingKeyring struct{}

func (failingKeyring) Get(string, string) (string, error) {
	return "", errors.New("no keychain in tests")
}
func (failingKeyring) Set(string, string, string) error {
	return errors.New("no keychain in tests")
}
func (failingKeyring) Delete(string, string) error {
	return errors.New("no keychain in tests")
}

// newTestAppState builds an appState with the given (possibly un-normalized)
// base URL, a temp config dir, and a file-backed credential store so
// auth.Save/Resolve/Forget work without touching the real OS keychain.
func newTestAppState(t *testing.T, baseURL string) *appState {
	t.Helper()
	dir := t.TempDir()
	return &appState{
		resolved: &config.Resolved{
			Config: config.Config{
				BaseURL: baseURL,
				Org:     "default",
				// Auth.Scheme must be SchemeSession: Resolve defaults to
				// SchemeBasic when this is unset, which would look the
				// credential up under a different account key than the one
				// runBrowserLoginWith saves under (AccountKey mixes the
				// scheme in), and the round trip below would fail even with
				// a correctly file-backed store.
				Auth: config.AuthConfig{Scheme: auth.SchemeSession},
				Defaults: config.Defaults{
					Format:     "json",
					Timeout:    5 * time.Second,
					MaxRetries: 1,
				},
			},
		},
		store:  auth.NewStoreWithBackend(dir, failingKeyring{}),
		cfgDir: dir,
	}
}

// fakeDriver stands in for cdp.Driver in tests: it records the arguments
// Capture was handed instead of driving a real browser.
type fakeDriver struct {
	loginURL, host string
	sess           pkgauth.Session
}

func (f *fakeDriver) Capture(loginURL, host string, _ webauth.VerifyFunc) (pkgauth.Session, error) {
	f.loginURL, f.host = loginURL, host
	return f.sess, nil
}

// TestRunBrowserLoginNormalizesBeforeCapture pins that runBrowserLogin
// normalizes the base URL before deriving the host and login URL handed to
// the driver. This must exercise runBrowserLoginWith itself — a test that
// only calls apiclient.NormalizeBaseURL and hostOfBaseURL directly (as
// TestBrowserLoginNormalizesBaseURL above does) stays green even if
// runBrowserLoginWith is reverted to pass cfg.BaseURL straight through,
// since neither of those helper functions would have changed. A bare
// host:port reaches here whenever OPENOBSERVE_URL is used, since that path
// never goes through `config init`.
func TestRunBrowserLoginNormalizesBeforeCapture(t *testing.T) {
	cases := []struct{ raw, wantLogin, wantHost string }{
		{"localhost:5080", "http://localhost:5080/web/login", "localhost:5080"},
		{"http://o2.example.com/", "http://o2.example.com/web/login", "o2.example.com"},
	}
	for _, tc := range cases {
		s := newTestAppState(t, tc.raw)
		d := &fakeDriver{sess: pkgauth.Session{Cookies: "auth_tokens=x", Email: "ops@example.com"}}
		if err := runBrowserLoginWith(s, d); err != nil {
			t.Fatalf("%q: %v", tc.raw, err)
		}
		if d.loginURL != tc.wantLogin {
			t.Errorf("%q: login URL = %q, want %q", tc.raw, d.loginURL, tc.wantLogin)
		}
		if d.host != tc.wantHost {
			t.Errorf("%q: host = %q, want %q", tc.raw, d.host, tc.wantHost)
		}
	}
}

// TestRunBrowserLoginSavesUnderRawBaseURL pins the other half of the
// normalization fix: the keychain key must match what auth.Resolve and
// auth.Forget compute from cfg.BaseURL (the raw value), not the normalized
// one, or a credential saved by --browser would be unfindable by every other
// command. The raw form below has no scheme, so url.Parse cannot split off a
// Host and AccountKey falls back to hashing the whole raw string including
// the trailing slash ("o2.example.com:8080/:session"); NormalizeBaseURL
// supplies the scheme and strips the slash first, giving a different key
// ("o2.example.com:8080:session") — exactly the asymmetry that would make a
// --browser-saved credential invisible to every other command if auth.Save
// used the normalized value instead of the raw one.
func TestRunBrowserLoginSavesUnderRawBaseURL(t *testing.T) {
	raw := "o2.example.com:8080/"
	s := newTestAppState(t, raw)
	d := &fakeDriver{sess: pkgauth.Session{Cookies: "auth_tokens=x", Email: "ops@example.com"}}
	if err := runBrowserLoginWith(s, d); err != nil {
		t.Fatal(err)
	}

	// auth.Resolve is what every other command (auth status, search, ...)
	// uses to find the credential; it keys off cfg.BaseURL verbatim.
	cred, err := auth.Resolve(s.resolved.Config, config.Secrets{}, s.store)
	if err != nil {
		t.Fatalf("credential saved by --browser is not resolvable via the normal lookup path: %v", err)
	}
	if cred.Scheme != auth.SchemeSession {
		t.Fatalf("resolved scheme = %q, want %q", cred.Scheme, auth.SchemeSession)
	}
}
