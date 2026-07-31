package webauth

import (
	"sync"
	"testing"

	pkgauth "github.com/angelmsger/openobserve-cli/pkg/auth"
)

func hostCookies() []Cookie {
	return []Cookie{{Name: "auth_tokens", Value: "abc", Domain: "o2.example.com", Path: "/"}}
}

func TestTrackerIgnoresNothingToReplay(t *testing.T) {
	tr := NewTracker("o2.example.com", func(pkgauth.Session) bool { return true })
	if _, ok := tr.Observe(nil, "https://o2.example.com/web/login", "", ""); ok {
		t.Fatal("no cookies and no authorization is not replayable; must not report success")
	}
}

func TestTrackerRequiresVerification(t *testing.T) {
	tr := NewTracker("o2.example.com", func(pkgauth.Session) bool { return false })
	if _, ok := tr.Observe(hostCookies(), "https://o2.example.com/web/logs", "", ""); ok {
		t.Fatal("verifier rejected the session; must not report success")
	}
}

func TestTrackerReportsVerifiedSession(t *testing.T) {
	tr := NewTracker("o2.example.com", func(pkgauth.Session) bool { return true })
	sess, ok := tr.Observe(hostCookies(), "https://o2.example.com/web/logs", "Basic xyz", "ops@example.com")
	if !ok {
		t.Fatal("verified session must report success")
	}
	if sess.Authorization != "Basic xyz" || sess.Email != "ops@example.com" {
		t.Fatalf("session did not carry the captured fields: %+v", sess)
	}
	if sess.Cookies == "" {
		t.Fatal("session did not carry the host cookies")
	}
}

// A static page must not be re-verified on every tick; that would hammer the
// instance with an authenticated request once a second.
func TestTrackerVerifiesEachDistinctStateOnce(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	tr := NewTracker("o2.example.com", func(pkgauth.Session) bool {
		mu.Lock()
		defer mu.Unlock()
		calls++
		return false
	})
	for i := 0; i < 5; i++ {
		tr.Observe(hostCookies(), "https://o2.example.com/web/logs", "", "")
	}
	mu.Lock()
	defer mu.Unlock()
	if calls != 1 {
		t.Fatalf("verifier called %d times for an unchanged state, want 1", calls)
	}
}

func TestTrackerRetriesWhenStateChanges(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	tr := NewTracker("o2.example.com", func(s pkgauth.Session) bool {
		mu.Lock()
		defer mu.Unlock()
		calls++
		return s.Authorization != ""
	})
	tr.Observe(hostCookies(), "https://o2.example.com/web/logs", "", "")
	_, ok := tr.Observe(hostCookies(), "https://o2.example.com/web/logs", "Basic xyz", "")
	mu.Lock()
	defer mu.Unlock()
	if calls != 2 {
		t.Fatalf("verifier called %d times, want 2 (state changed between observations)", calls)
	}
	if !ok {
		t.Fatal("second observation carried a working credential and must succeed")
	}
}

// Off-darwin builds and older tests run without a verifier; the pure heuristic
// is the fallback so behaviour is preserved.
func TestTrackerFallsBackToHeuristicWithoutVerifier(t *testing.T) {
	// A generic session cookie, NOT one of the known post-login auth cookies —
	// those succeed on their own and would bypass the URL check below.
	plain := []Cookie{{Name: "sid", Value: "x", Domain: "o2.example.com", Path: "/"}}

	tr := NewTracker("o2.example.com", nil)
	if _, ok := tr.Observe(plain, "https://o2.example.com/web/login", "", ""); ok {
		t.Fatal("still on the login page with only a generic cookie; heuristic must not report success")
	}
	if _, ok := tr.Observe(plain, "https://o2.example.com/web/logs", "", ""); !ok {
		t.Fatal("navigated off the login page with a host cookie; heuristic must report success")
	}
}

// A known post-login auth cookie is sufficient on its own: OpenObserve sets it
// only after a successful login, so capture must not wait for a navigation that
// a single-page app may never make.
func TestTrackerHeuristicAcceptsAuthCookieOnLoginPage(t *testing.T) {
	tr := NewTracker("o2.example.com", nil)
	if _, ok := tr.Observe(hostCookies(), "https://o2.example.com/web/login", "", ""); !ok {
		t.Fatal("auth_tokens present; heuristic must report success even on the login path")
	}
}
