package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeYAML(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestReadFileMissing(t *testing.T) {
	f, ok, err := ReadFile(t.TempDir())
	if err != nil {
		t.Fatalf("ReadFile missing: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false for missing file")
	}
	if len(f.Contexts) != 0 {
		t.Fatalf("expected empty File, got %+v", f)
	}
}

func TestReadFileNoDefaults(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, `current_context: default
contexts:
  - name: default
    server: https://o.example.com
    org: default
    auth:
      scheme: basic
      username: a@b.com
`)
	f, ok, err := ReadFile(dir)
	if err != nil || !ok {
		t.Fatalf("ReadFile: ok=%v err=%v", ok, err)
	}
	if f.CurrentContext != "default" || len(f.Contexts) != 1 {
		t.Fatalf("unexpected file: %+v", f)
	}
	c := f.Contexts[0]
	if c.Name != "default" || c.BaseURL != "https://o.example.com" || c.Org != "default" ||
		c.Auth.Scheme != "basic" || c.Auth.Username != "a@b.com" {
		t.Fatalf("context mismatch: %+v", c)
	}
	if f.Defaults.Timeout != 0 || f.Defaults.MaxRetries != 0 {
		t.Fatalf("expected zero defaults, got %+v", f.Defaults)
	}
}

func TestWriteThenReadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	in := File{
		CurrentContext: "prod",
		Contexts: []NamedContext{
			{Name: "prod", BaseURL: "https://p", Org: "default", Auth: AuthConfig{Scheme: "basic", Username: "u@x"}},
			{Name: "stg", BaseURL: "https://s", Org: "dev", Auth: AuthConfig{Scheme: "token"}},
		},
		Defaults: Defaults{Timeout: 30 * time.Second, MaxRetries: 3},
	}
	if err := WriteFile(dir, in); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	out, ok, err := ReadFile(dir)
	if err != nil || !ok {
		t.Fatalf("ReadFile: ok=%v err=%v", ok, err)
	}
	if out.CurrentContext != "prod" || len(out.Contexts) != 2 {
		t.Fatalf("round-trip mismatch: %+v", out)
	}
	if out.Contexts[1].Auth.Scheme != "token" {
		t.Fatalf("scheme lost: %+v", out.Contexts[1])
	}
	if out.Defaults.Timeout != 30*time.Second || out.Defaults.MaxRetries != 3 {
		t.Fatalf("defaults lost: %+v", out.Defaults)
	}
}

func TestWriteFilePermissions(t *testing.T) {
	dir := t.TempDir()
	if err := WriteFile(dir, File{CurrentContext: "x", Contexts: []NamedContext{{Name: "x"}}}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("perm = %o, want 600", perm)
	}
}

func TestContextHelpers(t *testing.T) {
	f := File{Contexts: []NamedContext{{Name: "a"}, {Name: "b"}}}
	if _, ok := f.Context("A"); !ok {
		t.Fatal("Context should be case-insensitive")
	}
	f.Upsert(NamedContext{Name: "b", BaseURL: "https://new"})
	if c, _ := f.Context("b"); c.BaseURL != "https://new" {
		t.Fatalf("Upsert replace failed: %+v", c)
	}
	f.Upsert(NamedContext{Name: "c"})
	if len(f.Contexts) != 3 {
		t.Fatalf("Upsert append failed: %d", len(f.Contexts))
	}
	if names := f.ContextNames(); len(names) != 3 || names[0] != "a" {
		t.Fatalf("ContextNames: %v", names)
	}
	if !f.Remove("A") || len(f.Contexts) != 2 {
		t.Fatalf("Remove failed: %d", len(f.Contexts))
	}
	if f.Remove("nope") {
		t.Fatal("Remove of missing should return false")
	}
}
