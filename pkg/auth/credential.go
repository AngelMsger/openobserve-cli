// Package auth models OpenObserve credentials and applies them to outgoing
// HTTP requests. It is the pure, dependency-light core shared by the CLI and
// the desktop GUI; configuration and keychain wiring live in their callers.
//
// The credential model, header encoding and recovery errors come from
// agent-cli-kit/auth. Importing this package registers OpenObserve's schemes
// and recovery wording with it, so a Credential validates, encodes and
// decorates requests the same way in every caller.
package auth

import (
	"net/http"
	"strings"

	kit "github.com/angelmsger/agent-cli-kit/auth"
	"github.com/angelmsger/openobserve-cli/pkg/constants"
	cerrors "github.com/angelmsger/openobserve-cli/pkg/errors"
	"github.com/angelmsger/openobserve-cli/pkg/transport"
)

// Scheme identifies an authentication scheme.
const (
	// SchemeBasic is HTTP Basic auth: email (username) + password.
	SchemeBasic = "basic"
	// SchemeToken is a pre-generated credential sent verbatim in the
	// Authorization header (the base64 portion of a Basic token, or a full
	// "Basic ..." / "Bearer ..." value).
	SchemeToken = "token"
	// SchemeSession is a browser-captured session: the Secret is a session
	// envelope (see session.go) carrying the instance cookies replayed on every
	// request, plus an optional Authorization fallback. Established by
	// `auth login --browser`; no service account required.
	SchemeSession = "session"
)

// MissingStep is the last recovery step when no stored secret is visible.
const MissingStep = "Only if the host retry also reports missing credentials, run `" + constants.AppName + " config init` in the user's terminal or set OPENOBSERVE_* environment variables."

func init() {
	kit.Register(&kit.Product{
		Schemes: []kit.Scheme{
			{Name: SchemeBasic, Kind: kit.Basic,
				Missing: "basic auth requires both an email and a password"},
			{Name: SchemeToken, Kind: kit.Custom, MissingCode: "AUTH_NO_TOKEN",
				Missing: "no token configured", Header: tokenHeader},
			{Name: SchemeSession, Kind: kit.Custom, Session: true,
				Validate: validateSession, Decorate: decorateSession},
		},
		DefaultScheme:    SchemeBasic,
		Name:             "OpenObserve",
		Doctor:           constants.AppName + " doctor",
		MissingStep:      MissingStep,
		InaccessibleStep: "Do not run `config init` unless the same check also fails in the host environment.",
		ListSchemes:      true,
	})
}

// Credential is a fully resolved credential ready to authenticate requests.
// Username is the account email (basic only); Secret is the password (basic),
// the token value (token), or the session envelope or cookies (session). Its
// Validate, Header, Decorator and Redacted methods follow the schemes
// registered above.
type Credential = kit.Credential

// AccountKey derives the keychain account identifier for a base URL and scheme.
// It is stable across runs so credentials can be located later.
func AccountKey(baseURL, scheme string) string { return kit.AccountKey(baseURL, scheme) }

// tokenHeader builds the token scheme's Authorization value. OpenObserve
// authenticates API requests with HTTP Basic auth; the user supplies a
// pre-generated credential — either the already-base64-encoded basic token, or
// a full "Basic ..." / "Bearer ..." header value, which passes through verbatim.
func tokenHeader(c Credential) string {
	s := strings.TrimSpace(c.Secret)
	if hasAuthPrefix(s) {
		return s
	}
	return "Basic " + s
}

func hasAuthPrefix(s string) bool {
	lower := strings.ToLower(s)
	return strings.HasPrefix(lower, "basic ") || strings.HasPrefix(lower, "bearer ")
}

func validateSession(c Credential) error {
	if _, err := ParseSession(c.Secret); err != nil {
		return cerrors.Wrap(err, cerrors.CategoryConfig, "AUTH_BAD_SESSION",
			"captured browser session is invalid; sign in through the browser again")
	}
	return nil
}

// decorateSession replays a browser-captured session: it sets the Cookie
// header (the primary authenticator) and, when the envelope carries one, an
// Authorization fallback for instances whose REST API expects the header the
// web SPA sends. An invalid session sets nothing.
func decorateSession(c Credential) transport.Decorator {
	s, err := ParseSession(c.Secret)
	return func(req *http.Request) {
		if err != nil {
			return
		}
		if s.Cookies != "" {
			req.Header.Set("Cookie", s.Cookies)
		}
		if s.Authorization != "" {
			req.Header.Set("Authorization", s.Authorization)
		}
	}
}
