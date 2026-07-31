package webauth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	pkgauth "github.com/angelmsger/openobserve-cli/pkg/auth"
)

// An instance using native login sets no cookies; the SPA authenticates with an
// Authorization header alone. Such a session must verify.
func TestPingVerifierAcceptsHeaderOnlySession(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Basic good" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"identifier":"default","name":"default"}]}`))
	}))
	defer srv.Close()

	verify := PingVerifier(srv.URL, "default", 5*time.Second, 0)
	if !verify(pkgauth.Session{Authorization: "Basic good"}) {
		t.Fatal("header-only session that authenticates must verify")
	}
	if verify(pkgauth.Session{Authorization: "Basic bad"}) {
		t.Fatal("session that does not authenticate must not verify")
	}
}

func TestPingVerifierAcceptsCookieSession(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "auth_tokens=good" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"identifier":"default","name":"default"}]}`))
	}))
	defer srv.Close()

	verify := PingVerifier(srv.URL, "default", 5*time.Second, 0)
	if !verify(pkgauth.Session{Cookies: "auth_tokens=good"}) {
		t.Fatal("cookie session that authenticates must verify")
	}
}

func TestPingVerifierRejectsUnusableSession(t *testing.T) {
	verify := PingVerifier("http://127.0.0.1:1", "default", 5*time.Second, 0)
	if verify(pkgauth.Session{}) {
		t.Fatal("an empty session must never verify")
	}
}
