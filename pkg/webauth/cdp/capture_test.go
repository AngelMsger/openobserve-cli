package cdp

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/angelmsger/openobserve-cli/pkg/webauth"
)

func TestConvertCookies(t *testing.T) {
	raw := []byte(`[
		{"name":"auth_tokens","value":"abc","domain":"o2.example.com","path":"/","expires":1785000000,"secure":true,"httpOnly":true},
		{"name":"sid","value":"x","domain":"o2.example.com","path":"/","expires":-1}
	]`)
	var cdpCookies []cdpCookie
	if err := json.Unmarshal(raw, &cdpCookies); err != nil {
		t.Fatal(err)
	}
	got := convertCookies(cdpCookies)
	if len(got) != 2 {
		t.Fatalf("got %d cookies, want 2", len(got))
	}
	if got[0].Name != "auth_tokens" || got[0].Expires.IsZero() {
		t.Fatalf("first cookie wrong: %+v", got[0])
	}
	// CDP uses -1, not 0, for a session cookie. Treating -1 as a real timestamp
	// would date the session to 1969 and make it look already expired.
	if !got[1].Expires.IsZero() {
		t.Fatalf("session cookie must have a zero expiry, got %v", got[1].Expires)
	}
}

func TestParseBindingPayload(t *testing.T) {
	authz, email := parseBindingPayload(`{"authorization":"Basic xyz"}`)
	if authz != "Basic xyz" || email != "" {
		t.Fatalf("authz=%q email=%q", authz, email)
	}
	authz, email = parseBindingPayload(`{"email":"ops@example.com"}`)
	if authz != "" || email != "ops@example.com" {
		t.Fatalf("authz=%q email=%q", authz, email)
	}
	if a, e := parseBindingPayload("not json"); a != "" || e != "" {
		t.Fatal("malformed payload must be ignored, not panic")
	}
}

func TestDriverSatisfiesWebauthDriver(t *testing.T) {
	var _ webauth.Driver = New(Options{})
}

func TestOptionsDefaultTimeout(t *testing.T) {
	d := New(Options{})
	if d.timeout != defaultTimeout {
		t.Fatalf("timeout = %v, want the %v default", d.timeout, defaultTimeout)
	}
	d = New(Options{Timeout: time.Minute})
	if d.timeout != time.Minute {
		t.Fatalf("explicit timeout was not honoured: %v", d.timeout)
	}
}
