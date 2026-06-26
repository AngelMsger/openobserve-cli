// Package auth models OpenObserve credentials and applies them to outgoing
// HTTP requests. It is the pure, dependency-light core shared by the CLI and
// the desktop GUI; configuration and keychain wiring live in their callers.
package auth

import (
	"net/url"
	"strings"

	cerrors "github.com/angelmsger/openobserve-cli/pkg/errors"
)

// Scheme identifies an authentication scheme.
const (
	// SchemeBasic is HTTP Basic auth: email (username) + password.
	SchemeBasic = "basic"
	// SchemeToken is a pre-generated credential sent verbatim in the
	// Authorization header (the base64 portion of a Basic token, or a full
	// "Basic ..." / "Bearer ..." value).
	SchemeToken = "token"
)

// Credential is a fully resolved credential ready to authenticate requests.
type Credential struct {
	Scheme   string
	Username string // basic only (the account email)
	Secret   string // password (basic) or token value (token)
}

// Validate reports whether the credential is internally consistent.
func (c Credential) Validate() error {
	switch c.Scheme {
	case SchemeToken:
		if c.Secret == "" {
			return cerrors.New(cerrors.CategoryConfig, "AUTH_NO_TOKEN",
				"no token configured")
		}
	case SchemeBasic:
		if c.Username == "" || c.Secret == "" {
			return cerrors.New(cerrors.CategoryConfig, "AUTH_NO_BASIC",
				"basic auth requires both an email and a password")
		}
	default:
		return cerrors.Newf(cerrors.CategoryConfig, "AUTH_BAD_SCHEME",
			"unknown auth scheme %q (want basic or token)", c.Scheme)
	}
	return nil
}

// Redacted returns a copy safe for logging: the secret is masked.
func (c Credential) Redacted() Credential {
	c.Secret = maskSecret(c.Secret)
	return c
}

func maskSecret(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 4 {
		return "****"
	}
	return strings.Repeat("*", len(s)-4) + s[len(s)-4:]
}

// AccountKey derives the keychain account identifier for a base URL and scheme.
// It is stable across runs so credentials can be located later.
func AccountKey(baseURL, scheme string) string {
	host := baseURL
	if u, err := url.Parse(baseURL); err == nil && u.Host != "" {
		host = u.Host
	}
	return host + ":" + scheme
}
