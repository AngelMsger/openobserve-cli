package webauth

import (
	"sync"

	pkgauth "github.com/angelmsger/openobserve-cli/pkg/auth"
)

// Tracker decides when an observed browser state counts as a completed login.
// It is shared by every transport so the CLI and o3 cannot drift on the
// question of when sign-in has actually finished.
//
// The policy, in order:
//
//   - Assemble the observed state into a session and drop it unless something
//     is replayable. An instance using native login sets no cookies at all, so
//     "has cookies" is not the test — "has cookies OR an Authorization header"
//     is.
//   - An authenticated API probe is the SOLE success signal. A benign cookie
//     set on the login page, or an in-progress redirect to an external identity
//     provider, must never be mistaken for a completed login, and only a real
//     authenticated request can tell the difference.
//   - Verify each distinct state at most once. Without this a static page is
//     re-probed on every tick, sending an authenticated request per second.
//
// A nil verifier disables the probe and falls back to the pure cookie/URL
// heuristic in LoginSucceeded. That path exists for unit tests and for any
// caller with no API client to hand; production callers always pass a verifier.
type Tracker struct {
	host   string
	verify VerifyFunc

	mu   sync.Mutex
	seen map[string]bool // signature -> already verified
}

// NewTracker returns a Tracker scoping cookies to host and confirming captures
// with verify. A nil verify selects the heuristic fallback.
func NewTracker(host string, verify VerifyFunc) *Tracker {
	return &Tracker{host: host, verify: verify, seen: map[string]bool{}}
}

// Observe feeds one sampled browser state to the policy. It returns the
// captured session and true exactly when sign-in is complete.
//
// Observe blocks for the duration of the verification request, so callers
// should not hold a UI lock across it.
func (t *Tracker) Observe(cookies []Cookie, currentURL, authz, email string) (pkgauth.Session, bool) {
	sess := AssembleSession(cookies, t.host, authz, email)

	if t.verify == nil {
		if !LoginSucceeded(currentURL, t.host, cookies) {
			return pkgauth.Session{}, false
		}
		return sess, true
	}

	if !Replayable(sess) {
		return pkgauth.Session{}, false
	}

	sig := sess.Cookies + "\n" + sess.Authorization
	t.mu.Lock()
	already := t.seen[sig]
	t.seen[sig] = true
	t.mu.Unlock()
	if already {
		return pkgauth.Session{}, false
	}

	if !t.verify(sess) {
		return pkgauth.Session{}, false
	}
	return sess, true
}
