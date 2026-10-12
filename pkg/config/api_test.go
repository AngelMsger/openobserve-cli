package config_test

import (
	"path/filepath"
	"reflect"
	"testing"
	"time"

	kit "github.com/angelmsger/agent-cli-kit/config"
	"github.com/angelmsger/openobserve-cli/pkg/config"
)

// The exported API of pkg/config is a contract with importers outside this
// repository (the desktop GUI). This test names every exported identifier
// with its type, so removing or changing one fails to compile here first.
var (
	_ func() (string, error)                                = config.DefaultConfigDir
	_ func() (string, error)                                = config.ResolveConfigDir
	_ func(string) string                                   = config.ConfigFilePath
	_ func(string) (config.File, bool, error)               = config.ReadFile
	_ func(string, config.File) error                       = config.WriteFile
	_ func(config.File, string) (config.NamedContext, bool) = config.File.Context
	_ func(config.File) []string                            = config.File.ContextNames
	_ func(*config.File, config.NamedContext)               = (*config.File).Upsert
	_ func(*config.File, string) bool                       = (*config.File).Remove
)

func TestPublicAPI(t *testing.T) {
	// File is the kit's file of openobserve contexts, under its old name.
	var _ kit.File[config.NamedContext, config.Defaults] = config.File{}

	f := config.File{
		CurrentContext: "prod",
		Contexts: []config.NamedContext{{
			Name:    "prod",
			BaseURL: "https://o2.example.com",
			Org:     "default",
			Auth:    config.AuthConfig{CredentialURL: "https://o2.example.com/web/login", Scheme: "basic", Username: "a@example.com"},
		}},
		Defaults: config.Defaults{Format: "json", Timeout: 30 * time.Second, MaxRetries: 3, ReadOnly: true},
	}
	f.Upsert(config.NamedContext{Name: "Staging", BaseURL: "https://stg.example.com", Auth: config.AuthConfig{Scheme: "token"}})
	f.Upsert(config.NamedContext{Name: "PROD", BaseURL: "https://o2.example.com", Org: "team", Auth: config.AuthConfig{Scheme: "basic"}})
	if got := f.ContextNames(); !reflect.DeepEqual(got, []string{"PROD", "Staging"}) {
		t.Fatalf("ContextNames = %v", got)
	}
	if c, ok := f.Context("staging"); !ok || c.Name != "Staging" {
		t.Fatalf("Context(staging) = %+v, %v", c, ok)
	}

	dir := filepath.Join(t.TempDir(), "openobserve")
	if err := config.WriteFile(dir, f); err != nil {
		t.Fatal(err)
	}
	if got := config.ConfigFilePath(dir); got != filepath.Join(dir, "config.yaml") {
		t.Errorf("ConfigFilePath = %q", got)
	}
	got, ok, err := config.ReadFile(dir)
	if err != nil || !ok || !reflect.DeepEqual(got, f) {
		t.Fatalf("ReadFile = %+v, %v, %v", got, ok, err)
	}
	if !got.Remove("STAGING") || got.Remove("staging") || len(got.Contexts) != 1 {
		t.Fatalf("Remove: %+v", got)
	}

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	want := filepath.Join(home, ".angelmsger", "openobserve")
	if d, err := config.DefaultConfigDir(); err != nil || d != want {
		t.Errorf("DefaultConfigDir = %q, %v", d, err)
	}
	if d, err := config.ResolveConfigDir(); err != nil || d != want {
		t.Errorf("ResolveConfigDir = %q, %v", d, err)
	}
}
