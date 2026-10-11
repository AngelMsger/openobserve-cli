package auth

import (
	"errors"
	"testing"

	"github.com/angelmsger/openobserve-cli/internal/config"
	pkgauth "github.com/angelmsger/openobserve-cli/pkg/auth"
	cerrors "github.com/angelmsger/openobserve-cli/pkg/errors"
	"github.com/zalando/go-keyring"
)

// failingKeyring is a keychain backend whose every call fails with err. The
// store's own behavior is tested in agent-cli-kit/credstore.
type failingKeyring struct{ err error }

func (f failingKeyring) Get(string, string) (string, error) { return "", f.err }
func (f failingKeyring) Set(string, string, string) error   { return f.err }
func (f failingKeyring) Delete(string, string) error        { return f.err }

func TestResolveBrowserSessionFromSharedStore(t *testing.T) {
	dir := t.TempDir()
	// A refusing keychain routes Save to the file fallback — the same path a
	// host with no usable keychain takes.
	store := NewStoreWithBackend(dir, failingKeyring{errors.New("no keychain here")})
	baseURL := "https://session-resolve-test.invalid"
	blob, err := pkgauth.EncodeSession(pkgauth.Session{Cookies: "sid=abc"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Save(AccountKey(baseURL, SchemeSession), blob); err != nil {
		t.Fatal(err)
	}

	cred, err := Resolve(config.Config{
		BaseURL: baseURL,
		Auth:    config.AuthConfig{Scheme: SchemeSession},
	}, config.Secrets{}, store)
	if err != nil {
		t.Fatal(err)
	}
	if cred.Scheme != SchemeSession || cred.Secret != blob {
		t.Fatalf("Resolve() = %+v, want stored session credential", cred.Redacted())
	}
}

func TestResolveRejectsInvalidStoredBrowserSession(t *testing.T) {
	dir := t.TempDir()
	store := NewStoreWithBackend(dir, failingKeyring{errors.New("no keychain here")})
	baseURL := "https://invalid-session-resolve-test.invalid"
	if _, err := store.Save(AccountKey(baseURL, SchemeSession), "{}"); err != nil {
		t.Fatal(err)
	}

	_, err := Resolve(config.Config{
		BaseURL: baseURL,
		Auth:    config.AuthConfig{Scheme: SchemeSession},
	}, config.Secrets{}, store)
	if err == nil {
		t.Fatal("Resolve() error = nil, want invalid-session error")
	}
}

func TestResolveDefaultsToBasic(t *testing.T) {
	cred, err := Resolve(config.Config{
		BaseURL: "https://o2.example.com",
		Auth:    config.AuthConfig{Username: "alice@example.com"},
	}, config.Secrets{Password: "pw", Token: "tok"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cred.Scheme != SchemeBasic || cred.Secret != "pw" {
		t.Fatalf("Resolve() = %+v, want the basic scheme with the password", cred.Redacted())
	}
}

func TestResolveCredentialRecovery(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		scheme   string
		err      error
		code     string
		message  string
		lastStep string
	}{
		{"missing", SchemeBasic, keyring.ErrNotFound, "CREDENTIAL_NOT_VISIBLE_OR_MISSING",
			"stored OpenObserve credentials are missing or not visible in this execution environment",
			"Only if the host retry also reports missing credentials, run `openobserve-cli config init` in the user's terminal or set OPENOBSERVE_* environment variables."},
		{"missing session", SchemeSession, keyring.ErrNotFound, "CREDENTIAL_NOT_VISIBLE_OR_MISSING",
			"stored OpenObserve credentials are missing or not visible in this execution environment",
			"Only if the host retry also reports a missing browser session, run `openobserve-cli auth login --browser` in the user's graphical session; browser sessions are not created by `config init`."},
		{"inaccessible", SchemeBasic, errors.New("sandbox denied keychain"), "CREDENTIAL_STORE_INACCESSIBLE",
			"stored OpenObserve credentials cannot be read in this execution environment",
			"Do not run `config init` unless the same check also fails in the host environment."},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Config{
				BaseURL: "https://o2.example.com",
				Auth:    config.AuthConfig{Scheme: tc.scheme, Username: "alice@example.com"},
			}
			s := NewStoreWithBackend(t.TempDir(), failingKeyring{err: tc.err})
			_, err := Resolve(cfg, config.Secrets{}, s)
			ce := cerrors.AsCLIError(err)
			if ce == nil || ce.Code != tc.code || ce.Message != tc.message {
				t.Fatalf("Resolve() error = %+v, want code %s and message %q", ce, tc.code, tc.message)
			}
			if ce.Recovery == nil || ce.Recovery.Scope != "host" || ce.Retryable {
				t.Fatalf("Resolve() recovery = %+v retryable=%v, want host and false", ce.Recovery, ce.Retryable)
			}
			if len(ce.NextSteps) < 3 || ce.NextSteps[1] != "openobserve-cli doctor" || ce.NextSteps[2] != tc.lastStep {
				t.Fatalf("Resolve() next steps = %q, want doctor then %q", ce.NextSteps, tc.lastStep)
			}
		})
	}
}
