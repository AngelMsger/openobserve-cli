package auth

import (
	"encoding/json"
	"fmt"
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

// ParseSession parses and validates a SchemeSession Secret. It accepts either
// the JSON envelope produced by EncodeSession or a bare "k1=v1; k2=v2" cookie
// string. A captured session must contain cookies; Authorization is only an
// optional fallback for instances whose API expects the header sent by the SPA.
func ParseSession(secret string) (Session, error) {
	trimmed := strings.TrimSpace(secret)
	if trimmed == "" {
		return Session{}, fmt.Errorf("session is empty")
	}

	var s Session
	if strings.HasPrefix(trimmed, "{") {
		if err := json.Unmarshal([]byte(trimmed), &s); err != nil {
			return Session{}, fmt.Errorf("decode session envelope: %w", err)
		}
	} else {
		s.Cookies = trimmed
	}

	if strings.TrimSpace(s.Cookies) == "" {
		return Session{}, fmt.Errorf("session has no cookies")
	}
	if strings.ContainsAny(s.Cookies, "\r\n") || strings.ContainsAny(s.Authorization, "\r\n") {
		return Session{}, fmt.Errorf("session contains an invalid header value")
	}
	return s, nil
}

// DecodeSession parses a SchemeSession Secret. Invalid input produces an empty
// Session; callers that need the parse error should use ParseSession.
func DecodeSession(secret string) Session {
	s, _ := ParseSession(secret)
	return s
}
