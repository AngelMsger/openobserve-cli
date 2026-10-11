// Package auth resolves OpenObserve credentials from configuration or secure
// storage. The credential model, header encoding and recovery errors come from
// agent-cli-kit/auth; the public pkg/auth registers OpenObserve's schemes and
// wording with it. This package keeps the config/keychain-coupled resolution
// and re-exports pkg/auth's symbols so existing callers keep working unchanged.
package auth

import (
	pkgauth "github.com/angelmsger/openobserve-cli/pkg/auth"
)

// Credential is re-exported from pkg/auth. Its methods (Header, Decorator,
// Validate, Redacted) come with the aliased type.
type Credential = pkgauth.Credential

// Auth scheme identifiers, re-exported from pkg/auth.
const (
	SchemeBasic   = pkgauth.SchemeBasic
	SchemeToken   = pkgauth.SchemeToken
	SchemeSession = pkgauth.SchemeSession
)

// AccountKey is re-exported from pkg/auth.
func AccountKey(baseURL, scheme string) string {
	return pkgauth.AccountKey(baseURL, scheme)
}
