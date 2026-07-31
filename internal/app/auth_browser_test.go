package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/angelmsger/openobserve-cli/pkg/apiclient"
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
