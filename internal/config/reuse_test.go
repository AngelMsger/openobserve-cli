package config

import (
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/angelmsger/openobserve-cli/pkg/auth" // registers the auth schemes
)

func TestLoadOffersReuseBeforeSignIn(t *testing.T) {
	dir := t.TempDir()
	for _, k := range []string{"OPENOBSERVE_URL", "OPENOBSERVE_ORG", "OPENOBSERVE_EMAIL", "OPENOBSERVE_PASSWORD", "OPENOBSERVE_TOKEN", "OPENOBSERVE_CONTEXT", "OPENOBSERVE_AUTH_SCHEME"} {
		t.Setenv(k, "")
	}
	file := File{CurrentContext: "team", Contexts: []NamedContext{
		{Name: "personal", BaseURL: "https://o2.example.test", Org: "default", Auth: AuthConfig{Scheme: "basic", Username: "me@example.test"}},
		{Name: "team", BaseURL: "https://o2.example.test", Org: "default", Auth: AuthConfig{Scheme: "session"}},
	}}
	if err := WriteFile(dir, file); err != nil {
		t.Fatal(err)
	}
	load := func(opt LoadOptions) *Resolved {
		t.Helper()
		opt.ConfigDir, opt.DotenvPath = dir, filepath.Join(t.TempDir(), "absent.env")
		r, err := Load(opt)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	r := load(LoadOptions{})
	if r.Config.ReuseStep != "openobserve-cli --use-context 'team' auth reuse" {
		t.Fatalf("reuse step %q", r.Config.ReuseStep)
	}
	g, err := Guide(r.Config, r.Sources)
	if err != nil || g.NextSteps[0] != r.Config.ReuseStep || !strings.Contains(g.Instructions[0], "auth reuse") {
		t.Fatalf("guide %+v %v", g, err)
	}
	if r = load(LoadOptions{Context: "personal"}); r.Config.ReuseStep != "" {
		t.Fatalf("a context with an identity was offered reuse: %q", r.Config.ReuseStep)
	}
	if r = load(LoadOptions{Flags: FlagValues{AuthScheme: "session"}}); r.Config.ReuseStep != "" {
		t.Fatalf("a supplied scheme without same-scheme sources was offered reuse: %q", r.Config.ReuseStep)
	}
	if r = load(LoadOptions{Flags: FlagValues{BaseURL: "https://other.example.test"}}); r.Config.ReuseStep != "" {
		t.Fatalf("an override was offered reuse: %q", r.Config.ReuseStep)
	}
}
