package app

import (
	"errors"
	"os"
	"path/filepath"
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
