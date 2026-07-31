package auth

import (
	"net/http"
	"testing"
	"time"
)

func TestSessionRoundTrip(t *testing.T) {
	exp := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	in := Session{
		Cookies:       "sid=abc; auth_ext=xyz",
		Authorization: "Basic Zm9vOmJhcg==",
		Email:         "ops@example.com",
		ExpiresAt:     exp,
	}
	blob, err := EncodeSession(in)
	if err != nil {
		t.Fatalf("EncodeSession: %v", err)
	}
	got := DecodeSession(blob)
	if got.Cookies != in.Cookies || got.Authorization != in.Authorization ||
		got.Email != in.Email || !got.ExpiresAt.Equal(in.ExpiresAt) {
		t.Fatalf("round trip mismatch: got %+v, want %+v", got, in)
	}
}

func TestDecodeSessionBareCookieString(t *testing.T) {
	// A non-JSON secret is treated as a raw cookie header value.
	got := DecodeSession("sid=abc; auth_ext=xyz")
	if got.Cookies != "sid=abc; auth_ext=xyz" {
		t.Fatalf("Cookies = %q, want raw cookie string", got.Cookies)
	}
	if got.Authorization != "" || got.Email != "" {
		t.Fatalf("expected no envelope fields, got %+v", got)
	}
}

func TestDecodeSessionMalformedEnvelope(t *testing.T) {
	// A malformed envelope must never be replayed as a Cookie header.
	got := DecodeSession("{not json")
	if got != (Session{}) {
		t.Fatalf("DecodeSession() = %+v, want empty session", got)
	}
	if _, err := ParseSession("{not json"); err == nil {
		t.Fatal("ParseSession() error = nil, want malformed envelope error")
	}
}

func TestParseSessionRequiresSomethingReplayable(t *testing.T) {
	for _, secret := range []string{"", "{}", `{"email":"ops@example.com"}`} {
		if _, err := ParseSession(secret); err == nil {
			t.Fatalf("ParseSession(%q) error = nil, want empty-session error", secret)
		}
	}
}

// A session captured from an instance that authenticates its own web app with an
// Authorization header (native email + password login) carries no cookies at
// all. Rejecting it made browser sign-in impossible against such instances.
func TestParseSessionAcceptsAuthorizationWithoutCookies(t *testing.T) {
	got, err := ParseSession(`{"authorization":"Basic dXNlcjpwYXNz","email":"ops@example.com"}`)
	if err != nil {
		t.Fatalf("ParseSession() error = %v, want nil", err)
	}
	if got.Authorization != "Basic dXNlcjpwYXNz" || got.Cookies != "" {
		t.Fatalf("parsed session = %+v, want the header with no cookies", got)
	}

	// It must also authenticate a request: the header is set, no Cookie header.
	cred := Credential{Scheme: SchemeSession, Secret: `{"authorization":"Basic dXNlcjpwYXNz"}`}
	if err := cred.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
	req, err := http.NewRequest(http.MethodGet, "https://observe.example.com/api/organizations", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	cred.Decorator()(req)
	if got := req.Header.Get("Authorization"); got != "Basic dXNlcjpwYXNz" {
		t.Fatalf("Authorization = %q, want the captured header", got)
	}
	if got := req.Header.Get("Cookie"); got != "" {
		t.Fatalf("Cookie = %q, want none", got)
	}
}
