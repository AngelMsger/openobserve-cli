// Package auth resolves OpenObserve credentials from configuration or secure
// storage. The pure credential model (header construction, validation, account
// keying) lives in the public pkg/auth; this package keeps the
// config/keychain-coupled resolution and re-exports the moved symbols so
// existing callers keep working unchanged.
package auth

import (
	pkgauth "github.com/angelmsger/openobserve-cli/pkg/auth"
)

// Credential is re-exported from pkg/auth. Its methods (Header, Decorator,
// Validate, Redacted) come with the aliased type.
type Credential = pkgauth.Credential

// Auth scheme identifiers, re-exported from pkg/auth.
const (
	SchemeBasic = pkgauth.SchemeBasic
	SchemeToken = pkgauth.SchemeToken
)

// AccountKey is re-exported from pkg/auth.
func AccountKey(baseURL, scheme string) string {
	return pkgauth.AccountKey(baseURL, scheme)
}
