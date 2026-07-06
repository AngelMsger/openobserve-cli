package auth

import (
	"encoding/base64"
	"net/http"
	"strings"

	"github.com/angelmsger/openobserve-cli/pkg/transport"
)

// Header returns the Authorization header value for the credential.
//
// OpenObserve authenticates API requests with HTTP Basic auth. For the basic
// scheme we encode email:password; for the token scheme the user supplies a
// pre-generated credential — either the already-base64-encoded basic token, or
// a full "Basic ..." / "Bearer ..." header value, which we pass through verbatim.
func (c Credential) Header() string {
	switch c.Scheme {
	case SchemeBasic:
		raw := c.Username + ":" + c.Secret
		return "Basic " + base64.StdEncoding.EncodeToString([]byte(raw))
	case SchemeToken:
		s := strings.TrimSpace(c.Secret)
		if hasAuthPrefix(s) {
			return s
		}
		return "Basic " + s
	default:
		return ""
	}
}

func hasAuthPrefix(s string) bool {
	lower := strings.ToLower(s)
	return strings.HasPrefix(lower, "basic ") || strings.HasPrefix(lower, "bearer ")
}

// Decorator returns a transport.Decorator that authenticates every request.
//
// The basic and token schemes set an Authorization header. The session scheme
// replays a browser-captured session: it sets the Cookie header (the primary
// authenticator) and, when the envelope carries one, an Authorization fallback
// for instances whose REST API expects the header the web SPA sends.
func (c Credential) Decorator() transport.Decorator {
	if c.Scheme == SchemeSession {
		s := DecodeSession(c.Secret)
		return func(req *http.Request) {
			if s.Cookies != "" {
				req.Header.Set("Cookie", s.Cookies)
			}
			if s.Authorization != "" {
				req.Header.Set("Authorization", s.Authorization)
			}
		}
	}
	header := c.Header()
	return func(req *http.Request) {
		if header != "" {
			req.Header.Set("Authorization", header)
		}
	}
}
