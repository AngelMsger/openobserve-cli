package webauth

import (
	"strings"

	pkgauth "github.com/angelmsger/openobserve-cli/pkg/auth"
)

// VerifyFunc confirms that a captured session actually authenticates against the
// instance's API. When a verifier is supplied, Capture reports success only once
// it returns true, so a login that merely set a benign preference/language cookie
// — or an in-progress SSO redirect that briefly leaves the login path — is never
// mistaken for a completed sign-in. A nil verifier disables the probe and falls
// back to the pure cookie/URL heuristic (used off-darwin and in unit tests).
type VerifyFunc func(pkgauth.Session) bool

// Replayable reports whether a captured state carries anything o3 could replay,
// and so is worth handing to the verifier.
//
// Cookies are the usual authenticator, but they are NOT universal: an instance
// using native (email + password) login authenticates its own SPA with an
// Authorization header the browser builds locally and sets no cookies at all.
// Gating capture on cookies alone therefore never verified such a login — the
// window stayed open on the instance's home page after the user had signed in.
func Replayable(s pkgauth.Session) bool {
	return strings.TrimSpace(s.Cookies) != "" || strings.TrimSpace(s.Authorization) != ""
}

// AssembleSession builds the storable/replayable session from captured cookies.
// Only cookies scoped to host are kept; the Cookie header is serialized stably,
// and the soonest cookie expiry (if any) drives the connection UI. Authorization
// and email ride along as the header fallback and display metadata.
func AssembleSession(cookies []Cookie, host, authorization, email string) pkgauth.Session {
	scoped := FilterForHost(cookies, host)
	return pkgauth.Session{
		Cookies:       SerializeCookies(scoped),
		Authorization: authorization,
		Email:         email,
		ExpiresAt:     EarliestExpiry(scoped),
	}
}
