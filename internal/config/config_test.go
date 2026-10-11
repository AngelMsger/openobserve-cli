package config

import (
	"path/filepath"
	"reflect"
	"testing"
	"time"

	cerrors "github.com/angelmsger/openobserve-cli/pkg/errors"
)

// TestSchema pins openobserve-cli's environment variables, defaults and scheme
// inference, which pkg/config registers with the shared engine.
func TestSchema(t *testing.T) {
	cleanSetupEnv(t)
	dir := t.TempDir()
	load := func() *Resolved {
		t.Helper()
		r, err := Load(LoadOptions{ConfigDir: dir, DotenvPath: filepath.Join(dir, "absent.env")})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	r := load()
	want := Config{Org: "default", Auth: AuthConfig{Scheme: SchemeBasic},
		Defaults: Defaults{Format: "json", Timeout: 30 * time.Second, MaxRetries: 3}}
	if !reflect.DeepEqual(r.Config, want) || r.Sources[FieldAuthScheme] != "default" || r.Sources[FieldOrg] != "default" {
		t.Fatalf("defaults = %+v (%v)", r.Config, r.Sources)
	}

	t.Setenv("OPENOBSERVE_URL", "o2.example.com:5080")
	t.Setenv("OPENOBSERVE_ORG", "team")
	t.Setenv("OPENOBSERVE_EMAIL", "alice@example.com")
	t.Setenv("OPENOBSERVE_CREDENTIAL_URL", "https://help.example.com/token")
	t.Setenv("OPENOBSERVE_FORMAT", "table")
	t.Setenv("OPENOBSERVE_CLI_READ_ONLY", "yes")
	t.Setenv("OPENOBSERVE_PASSWORD", "pw")
	r = load()
	if r.Config.BaseURL != "o2.example.com:5080" || r.Config.Org != "team" || r.Config.Auth.Username != "alice@example.com" ||
		r.Config.Auth.Scheme != SchemeBasic || r.Config.Auth.CredentialURL != "https://help.example.com/token" ||
		r.Config.Defaults.Format != "table" || !r.Config.Defaults.ReadOnly || r.Secrets != (Secrets{Password: "pw"}) {
		t.Fatalf("environment = %+v %+v", r.Config, r.Secrets)
	}
	t.Setenv("OPENOBSERVE_TOKEN", "tok")
	if r = load(); r.Config.Auth.Scheme != SchemeToken {
		t.Errorf("a token wins over a password: %q", r.Config.Auth.Scheme)
	}
	t.Setenv("OPENOBSERVE_AUTH_SCHEME", SchemeBasic)
	if r = load(); r.Config.Auth.Scheme != SchemeBasic {
		t.Errorf("an explicit scheme wins: %q", r.Config.Auth.Scheme)
	}
	if base, err := NormalizeServiceURL(r.Config.BaseURL); err != nil || base != "http://o2.example.com:5080" {
		t.Errorf("a bare host gets http://: %q %v", base, err)
	}
	if err := ValidateService(Config{BaseURL: "https://o2.example.com", Auth: AuthConfig{Scheme: "session"}}); err != nil {
		t.Errorf("session is an accepted scheme: %v", err)
	}
	ce := cerrors.AsCLIError(ValidateService(Config{BaseURL: "https://o2.example.com", Auth: AuthConfig{Scheme: "bearer"}}))
	if ce == nil || ce.Code != "AUTH_BAD_SCHEME" || ce.Hint != "Use one of: token, basic, session." {
		t.Errorf("bad scheme = %+v", ce)
	}
}

// A team preset may not repoint a browser-session context at another server
// or scheme, and that refusal comes before the conflict error, so --overwrite
// does not get past it either.
func TestPlanRefusesToRepointABrowserSession(t *testing.T) {
	file := File{CurrentContext: "sso", Contexts: []NamedContext{
		{Name: "sso", BaseURL: "https://o2.example.com", Org: "default", Auth: AuthConfig{Scheme: "session", Username: "alice@example.com"}},
	}}
	plan := func(server, scheme string, overwrite bool) error {
		resolved := &Resolved{
			Config:  Config{BaseURL: server, Org: "default", Auth: AuthConfig{Scheme: scheme}},
			Sources: map[string]string{FieldServer: "flag", FieldAuthScheme: "flag", FieldOrg: "default"},
		}
		_, err := PlanServiceContext(file, "SSO", resolved, overwrite, false)
		return err
	}
	for _, tc := range []struct {
		server, scheme string
		overwrite      bool
	}{
		{"https://other.example.com", "session", false},
		{"https://other.example.com", "session", true},
		{"https://o2.example.com", "basic", false},
		{"https://o2.example.com", "basic", true},
	} {
		ce := cerrors.AsCLIError(plan(tc.server, tc.scheme, tc.overwrite))
		if ce == nil || ce.Code != "BROWSER_MANAGED_SESSION" || ce.Category != cerrors.CategoryConfig ||
			ce.Hint != "Create a separate context, then sign in using auth login --browser." {
			t.Errorf("%+v: got %+v", tc, ce)
		}
	}
	if err := plan("https://o2.example.com", "session", false); err != nil {
		t.Errorf("an unchanged session context is no conflict: %v", err)
	}
	// A new context may use any service.
	resolved := &Resolved{Config: Config{BaseURL: "https://other.example.com", Auth: AuthConfig{Scheme: "basic"}}}
	if _, err := PlanServiceContext(file, "team", resolved, false, false); err != nil {
		t.Errorf("a new context beside a session: %v", err)
	}
}

// The notice names the opt-out variable the CLI checks.
func TestContextNoticeNamesTheOptOut(t *testing.T) {
	notice := ContextNotice("alpha", ContextSourceCurrent, []string{"alpha", "beta"})
	ctx := notice["_notice"].(map[string]any)["context"].(map[string]any)
	if ctx["silence"] != "set "+EnvNoContextHint+"=1 to suppress" ||
		ctx["override"] != "--use-context <name> or OPENOBSERVE_CONTEXT=<name>" {
		t.Errorf("notice = %v", ctx)
	}
}
