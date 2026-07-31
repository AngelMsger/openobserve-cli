package auth

import (
	"github.com/angelmsger/openobserve-cli/pkg/credstore"
)

// The credential store moved to pkg/credstore so the o3 desktop app can use the
// exact same implementation: both tools share one keychain service name and one
// fallback file, and a second implementation would let them drift apart on hosts
// without a usable keychain. These aliases keep the internal call sites — and
// their error handling — unchanged.
type (
	// Store persists secrets, preferring the OS keychain over a protected file.
	Store = credstore.Store
	// StoreAccessError means the credential store could not be inspected.
	StoreAccessError = credstore.StoreAccessError
	// Backend is the OS keychain a Store writes to.
	Backend = credstore.Backend
)

// ErrSecretNotFound is returned when no secret exists for an account.
var ErrSecretNotFound = credstore.ErrSecretNotFound

// Backend names reported by Store.Save.
const (
	BackendKeychain = credstore.BackendKeychain
	BackendFile     = credstore.BackendFile
)

// NewStore returns a Store whose file fallback lives in dir.
func NewStore(dir string) *Store { return credstore.NewStore(dir) }

// NewStoreWithBackend returns a Store using the supplied keychain backend.
func NewStoreWithBackend(dir string, b Backend) *Store {
	return credstore.NewStoreWithBackend(dir, b)
}
