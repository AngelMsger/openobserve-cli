package auth

import (
	"testing"

	"github.com/angelmsger/openobserve-cli/internal/config"
	pkgauth "github.com/angelmsger/openobserve-cli/pkg/auth"
)

func TestResolveBrowserSessionFromSharedStore(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	baseURL := "https://session-resolve-test.invalid"
	blob, err := pkgauth.EncodeSession(pkgauth.Session{Cookies: "sid=abc"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.fileSave(AccountKey(baseURL, SchemeSession), blob); err != nil {
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
	store := NewStore(dir)
	baseURL := "https://invalid-session-resolve-test.invalid"
	if err := store.fileSave(AccountKey(baseURL, SchemeSession), "{}"); err != nil {
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
