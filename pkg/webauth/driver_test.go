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

// An empty session is rejected by Credential.Validate before any request is
// attempted; this pins that short circuit.
func TestPingVerifierRejectsInvalidSessionWithoutRequesting(t *testing.T) {
	verify := PingVerifier("http://127.0.0.1:1", "default", 5*time.Second, 0)
	if verify(pkgauth.Session{}) {
		t.Fatal("an empty session must never verify")
	}
}

// A well-formed session against a host that refuses the connection must fail
// closed and promptly. PingVerifier runs in a polling loop while the user is
// mid-login, so a hang here would wedge capture rather than retry it.
func TestPingVerifierFailsClosedOnUnreachableHost(t *testing.T) {
	verify := PingVerifier("http://127.0.0.1:1", "default", 2*time.Second, 0)
	done := make(chan bool, 1)
	go func() { done <- verify(pkgauth.Session{Cookies: "sid=x"}) }()
	select {
	case ok := <-done:
		if ok {
			t.Fatal("an unreachable host must not verify")
		}
	case <-time.After(30 * time.Second):
		t.Fatal("PingVerifier hung on an unreachable host")
	}
}
