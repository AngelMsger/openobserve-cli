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
	removed, err := removeBrowserProfile(cfgDir)
	if err != nil {
		t.Fatalf("removeBrowserProfile: %v", err)
	}
	if !removed {
		t.Fatal("a profile that existed and was deleted must be reported as removed")
	}
	if _, err := os.Stat(profile); !os.IsNotExist(err) {
		t.Fatal("browser profile survived logout; the remembered session is still on disk")
	}
}

func TestRemoveBrowserProfileToleratesNoProfile(t *testing.T) {
	removed, err := removeBrowserProfile(t.TempDir())
	if err != nil {
		t.Fatalf("a context that never used browser sign-in must log out cleanly: %v", err)
	}
	// `auth logout` reports this verbatim; most contexts never used browser
	// sign-in and must not be told a profile was deleted.
	if removed {
		t.Fatal("reported removing a browser profile that never existed")
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
// base URL and starting auth scheme, a temp config dir, and a file-backed
// credential store so auth.Save/Resolve/Forget work without touching the real
// OS keychain.
//
// scheme is whatever the context started with — "" (never configured) and
// "basic" (a password context switching to SSO) are the two states a real user
// runs `auth login --browser` from. Neither may be pre-set to `session`: the
// account key mixes the scheme in, so a test that hands itself the answer would
// pass even if the login never recorded `scheme: session` anywhere.
func newTestAppState(t *testing.T, baseURL, scheme string) *appState {
	t.Helper()
	dir := t.TempDir()
	return &appState{
		resolved: &config.Resolved{
			Config: config.Config{
				BaseURL: baseURL,
				Org:     "default",
				Auth:    config.AuthConfig{Scheme: scheme},
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

// loadFromDisk re-resolves configuration from a config directory the way a
// fresh CLI invocation would, with the environment neutralised so only the
// file under test decides the outcome.
func loadFromDisk(t *testing.T, cfgDir string) *config.Resolved {
	t.Helper()
	for _, k := range []string{
		"OPENOBSERVE_URL", "OPENOBSERVE_ORG", "OPENOBSERVE_EMAIL",
		"OPENOBSERVE_PASSWORD", "OPENOBSERVE_TOKEN", "OPENOBSERVE_CONTEXT",
	} {
		t.Setenv(k, "")
	}
	resolved, err := config.Load(config.LoadOptions{
		ConfigDir:  cfgDir,
		DotenvPath: filepath.Join(t.TempDir(), "absent.env"),
	})
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	return resolved
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
		s := newTestAppState(t, tc.raw, "")
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
	s := newTestAppState(t, raw, "")
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

// TestRunBrowserLoginPersistsSessionScheme is the regression test for the
// release blocker: --browser stored the captured session under the `session`
// account key but never wrote `auth.scheme: session` anywhere, and nothing else
// could. auth.Resolve defaults an unset scheme to `basic` and `config init`
// refuses to write `session`, so the login reported success and every later
// command then either failed to find a credential or silently carried on with
// the context's stale password.
//
// It therefore starts from the two states a real user is in — never configured,
// and a working password context moving to SSO — and asserts on the CONFIG FILE
// plus a fresh reload, not on the in-memory state the login just mutated.
func TestRunBrowserLoginPersistsSessionScheme(t *testing.T) {
	cases := []struct {
		name   string
		scheme string
	}{
		{"scheme never configured", ""},
		{"context previously used a password", auth.SchemeBasic},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestAppState(t, "https://o2.example.com", tc.scheme)
			if tc.scheme != "" {
				// The context exists on disk, as it would after `config init`.
				writeContext(t, s.cfgDir, config.NamedContext{
					Name:    config.DefaultContextName,
					BaseURL: "https://o2.example.com",
					Org:     "default",
					Auth:    config.AuthConfig{Scheme: tc.scheme, Username: "old@example.com"},
				})
			}
			d := &fakeDriver{sess: pkgauth.Session{
				Cookies: "auth_tokens=captured", Email: "ops@example.com",
			}}
			if err := runBrowserLoginWith(s, d); err != nil {
				t.Fatalf("runBrowserLoginWith: %v", err)
			}

			// 1. The file itself now says `session`.
			file, ok, err := config.ReadFile(s.cfgDir)
			if err != nil || !ok {
				t.Fatalf("config file not written (ok=%v): %v", ok, err)
			}
			nc, found := file.Context(config.DefaultContextName)
			if !found {
				t.Fatalf("no %q context in the config file: %+v", config.DefaultContextName, file)
			}
			if nc.Auth.Scheme != auth.SchemeSession {
				t.Fatalf("config file records scheme %q, want %q — nothing else writes it, "+
					"so the captured session is unreachable", nc.Auth.Scheme, auth.SchemeSession)
			}
			if nc.Auth.Username != "ops@example.com" {
				t.Errorf("username = %q, want the captured email", nc.Auth.Username)
			}

			// 2. A fresh invocation resolves the captured session from it.
			reloaded := loadFromDisk(t, s.cfgDir)
			cred, err := auth.Resolve(reloaded.Config, reloaded.Secrets, s.store)
			if err != nil {
				t.Fatalf("a fresh invocation cannot resolve the captured session: %v", err)
			}
			if cred.Scheme != auth.SchemeSession {
				t.Fatalf("reloaded scheme = %q, want %q", cred.Scheme, auth.SchemeSession)
			}
			sess, err := pkgauth.ParseSession(cred.Secret)
			if err != nil {
				t.Fatalf("resolved secret is not the captured session: %v", err)
			}
			if sess.Cookies != "auth_tokens=captured" {
				t.Fatalf("resolved cookies = %q, want the captured ones", sess.Cookies)
			}
		})
	}
}

// Writing the scheme must not cost the user the rest of their config file.
func TestRunBrowserLoginPreservesTheRestOfTheConfigFile(t *testing.T) {
	s := newTestAppState(t, "https://prod.example.com", auth.SchemeBasic)
	s.resolved.ActiveContext = "prod"
	before := config.File{
		CurrentContext: "prod",
		Contexts: []config.NamedContext{
			{Name: "staging", BaseURL: "https://staging.example.com", Org: "stg",
				Auth: config.AuthConfig{Scheme: auth.SchemeToken, Username: "svc@example.com"}},
			{Name: "prod", BaseURL: "https://prod.example.com", Org: "default",
				Auth: config.AuthConfig{Scheme: auth.SchemeBasic, Username: "old@example.com"}},
		},
		Defaults: config.Defaults{Format: "table", Timeout: 42 * time.Second, MaxRetries: 7, ReadOnly: true},
	}
	if err := config.WriteFile(s.cfgDir, before); err != nil {
		t.Fatal(err)
	}

	d := &fakeDriver{sess: pkgauth.Session{Cookies: "auth_tokens=x", Email: "ops@example.com"}}
	if err := runBrowserLoginWith(s, d); err != nil {
		t.Fatal(err)
	}

	after, _, err := config.ReadFile(s.cfgDir)
	if err != nil {
		t.Fatal(err)
	}
	if after.CurrentContext != "prod" {
		t.Errorf("current_context = %q, want prod", after.CurrentContext)
	}
	if after.Defaults != before.Defaults {
		t.Errorf("shared defaults were clobbered: %+v, want %+v", after.Defaults, before.Defaults)
	}
	stg, _ := after.Context("staging")
	if stg != before.Contexts[0] {
		t.Errorf("the unrelated context changed: %+v, want %+v", stg, before.Contexts[0])
	}
	prod, _ := after.Context("prod")
	want := config.NamedContext{
		Name: "prod", BaseURL: "https://prod.example.com", Org: "default",
		Auth: config.AuthConfig{Scheme: auth.SchemeSession, Username: "ops@example.com"},
	}
	if prod != want {
		t.Errorf("prod context = %+v, want %+v", prod, want)
	}
}

// TestRunBrowserLoginRefusesToRewriteADifferentServersContext is the
// regression test for the corruption the session-scheme fix introduced. The
// context NAME comes from the config file and is unaffected by OPENOBSERVE_URL
// or --base-url, which override only the server. So a browser login against an
// overridden server used to rewrite the auth block of a context still pointing
// at the ORIGINAL server: that context ended up claiming `scheme: session`
// (with the newly captured email replacing its recorded username) while its
// credential was filed under the other server's account key. Nothing surfaced
// until the override went away, at which point every command failed to resolve
// a credential.
func TestRunBrowserLoginRefusesToRewriteADifferentServersContext(t *testing.T) {
	// The config file describes prod; this invocation was pointed at a local
	// instance, exactly as `OPENOBSERVE_URL=http://localhost:5080 ... --browser`
	// would leave things.
	s := newTestAppState(t, "http://localhost:5080", auth.SchemeBasic)
	s.resolved.ActiveContext = config.DefaultContextName
	before := config.NamedContext{
		Name:    config.DefaultContextName,
		BaseURL: "https://prod.example.com",
		Org:     "default",
		Auth:    config.AuthConfig{Scheme: auth.SchemeBasic, Username: "old@example.com"},
	}
	writeContext(t, s.cfgDir, before)
	raw, err := os.ReadFile(config.ConfigFilePath(s.cfgDir))
	if err != nil {
		t.Fatal(err)
	}

	d := &fakeDriver{sess: pkgauth.Session{Cookies: "auth_tokens=x", Email: "ops@example.com"}}
	err = runBrowserLoginWith(s, d)
	if err == nil {
		t.Fatal("browser login silently rewrote a context that points at a different server")
	}
	ce := cerrors.AsCLIError(err)
	if ce.Code != "CONTEXT_BASE_URL_MISMATCH" {
		t.Fatalf("code = %q, want CONTEXT_BASE_URL_MISMATCH", ce.Code)
	}
	if len(ce.NextSteps) == 0 {
		t.Error("the error carries no next steps; the user cannot act on it")
	}

	// The whole context must survive untouched — not just the scheme. The
	// recorded username was collateral damage of the same write.
	after, ok, err := config.ReadFile(s.cfgDir)
	if err != nil || !ok {
		t.Fatalf("config file unreadable (ok=%v): %v", ok, err)
	}
	got, found := after.Context(config.DefaultContextName)
	if !found {
		t.Fatalf("the context disappeared: %+v", after)
	}
	if got != before {
		t.Errorf("context = %+v, want it byte-identical: %+v", got, before)
	}
	nowRaw, err := os.ReadFile(config.ConfigFilePath(s.cfgDir))
	if err != nil {
		t.Fatal(err)
	}
	if string(nowRaw) != string(raw) {
		t.Errorf("the config file was rewritten:\n--- before ---\n%s\n--- after ---\n%s", raw, nowRaw)
	}

	// Reject the target before browser capture or credential persistence.
	if d.loginURL != "" {
		t.Error("browser capture started before checking the configured target")
	}
	if _, err := s.store.Load(auth.AccountKey(s.cfg().BaseURL, auth.SchemeSession)); err == nil {
		t.Error("a mismatched login stored a session")
	}
}

// TestRunBrowserLoginFreshContextWithTrailingSlashResolvesLater covers the
// other half of the same guard. A scheme-less OPENOBSERVE_URL with a trailing
// slash makes the raw and normalized base URLs hash to different account keys:
// url.Parse finds no Host in "o2.example.com:8080/" so AccountKey hashes the
// whole string, while the normalized "http://o2.example.com:8080" hashes to the
// host alone. Materializing the fresh context from the normalized value while
// auth.Save used the raw one therefore orphaned the credential the moment the
// environment variable went away. A brand-new context for the server that was
// just signed in to is legitimate, so this must keep working end to end rather
// than erroring.
func TestRunBrowserLoginFreshContextWithTrailingSlashResolvesLater(t *testing.T) {
	s := newTestAppState(t, "o2.example.com:8080/", "")
	d := &fakeDriver{sess: pkgauth.Session{
		Cookies: "auth_tokens=captured", Email: "ops@example.com",
	}}
	if err := runBrowserLoginWith(s, d); err != nil {
		t.Fatalf("a fresh context for the server just signed in to must be created: %v", err)
	}

	// The env var is gone: only the config file decides now.
	reloaded := loadFromDisk(t, s.cfgDir)
	cred, err := auth.Resolve(reloaded.Config, reloaded.Secrets, s.store)
	if err != nil {
		t.Fatalf("the captured session is unresolvable once OPENOBSERVE_URL is gone: %v", err)
	}
	if cred.Scheme != auth.SchemeSession {
		t.Fatalf("reloaded scheme = %q, want %q", cred.Scheme, auth.SchemeSession)
	}
	sess, err := pkgauth.ParseSession(cred.Secret)
	if err != nil {
		t.Fatalf("resolved secret is not the captured session: %v", err)
	}
	if sess.Cookies != "auth_tokens=captured" {
		t.Fatalf("resolved cookies = %q, want the captured ones", sess.Cookies)
	}
}

// `auth logout` forgets the scheme the context actually uses. Keying the
// deletion off `basic` would report logged_out: true and leave the captured
// session sitting in the store.
func TestAuthLogoutRemovesTheSessionCredential(t *testing.T) {
	s := newTestAppState(t, "https://o2.example.com", "")
	d := &fakeDriver{sess: pkgauth.Session{Cookies: "auth_tokens=x", Email: "ops@example.com"}}
	if err := runBrowserLoginWith(s, d); err != nil {
		t.Fatal(err)
	}
	key := auth.AccountKey(s.cfg().BaseURL, auth.SchemeSession)
	if _, err := s.store.Load(key); err != nil {
		t.Fatalf("precondition: session credential was not stored: %v", err)
	}

	cmd := newAuthLogoutCmd(s)
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("auth logout: %v", err)
	}
	// The test store's keychain backend always fails, so a missing secret
	// surfaces as a store-access error rather than ErrSecretNotFound; what
	// matters is that the secret is no longer loadable when it was a moment ago.
	if secret, err := s.store.Load(key); err == nil {
		t.Fatalf("the captured session survived logout (still loads %d bytes); "+
			"logout reported success while leaving the credential in the store", len(secret))
	}
}

// --fresh-profile selects a throwaway browser profile; nothing but --browser
// reads it. Accepting it silently would hide a typo'd invocation behind an
// apparently successful password login.
func TestFreshProfileWithoutBrowserIsAUsageError(t *testing.T) {
	s := newTestAppState(t, "https://o2.example.com", "")
	cmd := newAuthLoginCmd(s)
	if err := cmd.Flags().Set("fresh-profile", "true"); err != nil {
		t.Fatal(err)
	}
	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("--fresh-profile without --browser was accepted")
	}
	ce := cerrors.AsCLIError(err)
	if ce.Code != "FRESH_PROFILE_NEEDS_BROWSER" {
		t.Fatalf("code = %q, want FRESH_PROFILE_NEEDS_BROWSER", ce.Code)
	}
	if len(ce.NextSteps) == 0 {
		t.Error("the error carries no next steps")
	}
}

// TestRequireDisplayOnHeadlessLinux pins the actual decision the display
// check makes, including the branch that only fires on Linux. This CLI's
// dev machines are macOS, where checkDisplay is always a no-op because
// runtime.GOOS is fixed at compile time — a test running here has no way to
// observe the Linux behavior except by calling the parameterized decision
// directly. This is what release CI (headless Linux) actually hits.
func TestRequireDisplayOnHeadlessLinux(t *testing.T) {
	cases := []struct {
		name             string
		goos             string
		display, wayland string
		wantErr          bool
	}{
		{"linux with neither set", "linux", "", "", true},
		{"linux with DISPLAY set", "linux", ":0", "", false},
		{"linux with WAYLAND_DISPLAY set", "linux", "", "wayland-0", false},
		{"darwin never needs a display", "darwin", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := requireDisplayOn(tc.goos, tc.display, tc.wayland)
			if (err != nil) != tc.wantErr {
				t.Fatalf("requireDisplayOn(%q, %q, %q) = %v, want error: %v",
					tc.goos, tc.display, tc.wayland, err, tc.wantErr)
			}
			if err != nil {
				if ce := cerrors.AsCLIError(err); ce.Code != "BROWSER_NO_DISPLAY" {
					t.Fatalf("code = %q, want BROWSER_NO_DISPLAY", ce.Code)
				}
			}
		})
	}
}

// TestRunBrowserLoginWithNeverConsultsTheDisplayCheck is the regression test
// for the release blocker itself: requireDisplay used to be called inside
// runBrowserLoginWith, so every test that drove it through a fake driver —
// the six listed in the v0.10.0 CI failure — demanded a graphical session
// they had no use for, and only passed on macOS because checkDisplay is a
// no-op there. The check belongs in runBrowserLogin, which is the only
// caller that ever constructs a real (drawing) browser driver.
//
// This substitutes a spy for the package-level requireDisplay var and proves
// runBrowserLoginWith completes an entire successful login — including the
// filesystem/config-file work persistSessionScheme does — without calling it
// even once, regardless of the host OS or environment the test happens to
// run under.
func TestRunBrowserLoginWithNeverConsultsTheDisplayCheck(t *testing.T) {
	calls := 0
	orig := requireDisplay
	requireDisplay = func() error {
		calls++
		return errors.New("the display check must not run on the injected-driver path")
	}
	t.Cleanup(func() { requireDisplay = orig })

	s := newTestAppState(t, "https://o2.example.com", "")
	d := &fakeDriver{sess: pkgauth.Session{Cookies: "auth_tokens=x", Email: "ops@example.com"}}
	if err := runBrowserLoginWith(s, d); err != nil {
		t.Fatalf("runBrowserLoginWith failed (display check should never have run): %v", err)
	}
	if calls != 0 {
		t.Fatalf("runBrowserLoginWith consulted the display check %d time(s); "+
			"it must stay reachable with an injected driver regardless of display state", calls)
	}
}

// writeContext seeds a config file holding exactly one context.
func writeContext(t *testing.T, dir string, nc config.NamedContext) {
	t.Helper()
	if err := config.WriteFile(dir, config.File{
		CurrentContext: nc.Name,
		Contexts:       []config.NamedContext{nc},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestBrowserLoginEquivalentURLKeepsCredentialKey(t *testing.T) {
	s := newTestAppState(t, "https://SERVICE.example.test:443/deploy/", auth.SchemeBasic)
	s.resolved.ActiveContext = config.DefaultContextName
	writeContext(t, s.cfgDir, config.NamedContext{Name: config.DefaultContextName, BaseURL: "https://service.example.test/deploy", Auth: config.AuthConfig{Scheme: auth.SchemeBasic}})
	driver := &fakeDriver{sess: pkgauth.Session{Cookies: "auth_tokens=x", Email: "ops@example.com"}}
	if err := runBrowserLoginWith(s, driver); err != nil {
		t.Fatal(err)
	}
	file, _, err := config.ReadFile(s.cfgDir)
	if err != nil {
		t.Fatal(err)
	}
	nc, _ := file.Context(config.DefaultContextName)
	fresh := s.cfg()
	fresh.BaseURL = nc.BaseURL
	fresh.Auth = nc.Auth
	if _, err := auth.Resolve(fresh, config.Secrets{}, s.store); err != nil {
		t.Fatalf("equivalent URL orphaned browser session: %v", err)
	}
}
