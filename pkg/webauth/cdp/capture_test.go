package cdp

import (
	"encoding/json"
	"os"
	"os/exec"
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

// Closing the sign-in window leaves the browser process alive on macOS, so
// Target.detachedFromTarget is the only signal Capture gets. Every later
// sample() then fails permanently ("Session with given id not found") and,
// without this event, the poll loop spins until the ten-minute timeout —
// making BROWSER_SIGNIN_CANCELLED unreachable there.
func TestDetachedIsOurs(t *testing.T) {
	cases := []struct {
		name      string
		params    string
		sessionID string
		want      bool
	}{
		{"our session", `{"sessionId":"S1","targetId":"T1"}`, "S1", true},
		{"another target's session", `{"sessionId":"S2","targetId":"T2"}`, "S1", false},
		{"no session id in the event", `{"targetId":"T1"}`, "S1", false},
		{"malformed params", `not json`, "S1", false},
		{"no session of our own yet", `{"sessionId":""}`, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := detachedIsOurs(json.RawMessage(tc.params), tc.sessionID)
			if got != tc.want {
				t.Fatalf("detachedIsOurs(%s, %q) = %v, want %v",
					tc.params, tc.sessionID, got, tc.want)
			}
		})
	}
}

// TestHelperSleep is not a test: it is the child process
// TestKillAndReapWaitsForExit re-executes. It exits at once unless the marker
// is set. Re-executing the test binary keeps the helper portable — a browser-
// free stand-in for the browser process on every OS, Windows included.
func TestHelperSleep(t *testing.T) {
	if os.Getenv("GO_CDP_HELPER_SLEEP") != "1" {
		t.Skip("helper process for TestKillAndReapWaitsForExit")
	}
	time.Sleep(60 * time.Second)
}

// killAndReap must not return until the process is really gone. Capture's
// deferred removal of a throwaway profile directory runs immediately after it
// (deferred calls are LIFO and the removal is registered first), and on Windows
// that removal fails with a sharing violation while the browser still holds
// handles inside the profile — leaving a complete profile of live
// identity-provider cookies in %TEMP%, which is exactly what --fresh-profile
// exists to prevent.
func TestKillAndReapWaitsForExit(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestHelperSleep$")
	cmd.Env = append(os.Environ(), "GO_CDP_HELPER_SLEEP=1")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	exited := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(exited)
	}()

	start := time.Now()
	killAndReap(cmd, exited, 30*time.Second)
	select {
	case <-exited:
	default:
		t.Fatal("killAndReap returned before the process was reaped; " +
			"a profile directory removed after this would race live browser handles")
	}
	if d := time.Since(start); d > 30*time.Second {
		t.Fatalf("killAndReap took %v", d)
	}
}

// The wait is bounded, so a process that refuses to die delays the command
// rather than hanging it.
func TestKillAndReapGivesUpAfterGrace(t *testing.T) {
	never := make(chan struct{})
	start := time.Now()
	killAndReap(&exec.Cmd{}, never, 100*time.Millisecond)
	d := time.Since(start)
	if d < 100*time.Millisecond {
		t.Fatalf("returned after %v, before the grace period elapsed", d)
	}
	if d > 10*time.Second {
		t.Fatalf("bounded wait did not give up: took %v", d)
	}
}
