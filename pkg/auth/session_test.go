package auth

import (
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

func TestParseSessionRequiresCookies(t *testing.T) {
	for _, secret := range []string{"", "{}", `{"authorization":"Bearer tok"}`} {
		if _, err := ParseSession(secret); err == nil {
			t.Fatalf("ParseSession(%q) error = nil, want missing-cookie error", secret)
		}
	}
}
