package webauth

import (
	"testing"
)

func TestAssembleSession(t *testing.T) {
	cookies := []Cookie{
		{Name: "b", Value: "2", Domain: "observe.example.com"},
		{Name: "a", Value: "1", Domain: "observe.example.com"},
		{Name: "drop", Value: "z", Domain: "evil.com"},
	}
	sess := AssembleSession(cookies, "observe.example.com", "Bearer tok", "ops@example.com")
	if sess.Cookies != "a=1; b=2" {
		t.Fatalf("Cookies = %q, want %q (host-scoped, stable order)", sess.Cookies, "a=1; b=2")
	}
	if sess.Authorization != "Bearer tok" || sess.Email != "ops@example.com" {
		t.Fatalf("metadata wrong: %+v", sess)
	}
}

// TestReplayable pins the gate that decides when a probed state is worth
// verifying. The header-only case is the one that matters: an OpenObserve
// instance using native (email + password) login authenticates its own SPA with
// an Authorization header and sets NO cookies at all, so gating on cookies alone
// meant capture never verified — the login window sat on the web UI forever.
func TestReplayable(t *testing.T) {
	cases := []struct {
		name    string
		cookies []Cookie
		authz   string
		want    bool
	}{
		{"nothing captured yet", nil, "", false},
		{"cookies only (SSO / Dex instances)", []Cookie{{Name: "auth_ext", Value: "abc"}}, "", true},
		{"authorization only (native-login instances)", nil, "Basic dXNlcjpwYXNz", true},
		{"both", []Cookie{{Name: "auth_ext", Value: "abc"}}, "Basic dXNlcjpwYXNz", true},
		{"blank authorization is not a capture", nil, "   ", false},
	}
	for _, c := range cases {
		sess := AssembleSession(c.cookies, "observe.example.com", c.authz, "")
		if got := Replayable(sess); got != c.want {
			t.Errorf("%s: Replayable = %v, want %v", c.name, got, c.want)
		}
	}
}
