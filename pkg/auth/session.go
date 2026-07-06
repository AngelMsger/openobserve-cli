package auth

import (
	"encoding/json"
	"strings"
	"time"
)

// Session is the decoded form of a SchemeSession credential Secret. It is
// captured when a user signs in through their instance's own web login page
// (o3's browser sign-in) and replayed on every request. Cookies is the primary
// authenticator; Authorization is a fallback for instances whose REST API
// rejects the browser cookie and expects the header the SPA sends. Email and
// ExpiresAt are display-only metadata for the connection UI.
type Session struct {
	Cookies       string    `json:"cookies"`                 // "k1=v1; k2=v2"
	Authorization string    `json:"authorization,omitempty"` // e.g. "Basic ..." / "Bearer ..."
	Email         string    `json:"email,omitempty"`
	ExpiresAt     time.Time `json:"expiresAt,omitempty"`
}

// EncodeSession serializes a Session to the JSON envelope stored as
// Credential.Secret (and, via the shared keychain, reused by the CLI).
func EncodeSession(s Session) (string, error) {
	b, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// DecodeSession parses a SchemeSession Secret. It accepts either the JSON
// envelope produced by EncodeSession or a bare "k1=v1; k2=v2" cookie string (a
// leading '{' selects JSON), so the simplest capture — cookies only — needs no
// envelope. An unparseable envelope falls back to treating the input as cookies.
func DecodeSession(secret string) Session {
	trimmed := strings.TrimSpace(secret)
	if strings.HasPrefix(trimmed, "{") {
		var s Session
		if err := json.Unmarshal([]byte(trimmed), &s); err == nil {
			return s
		}
	}
	return Session{Cookies: secret}
}
