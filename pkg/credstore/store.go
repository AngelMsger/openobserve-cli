// Package credstore persists secrets for the OpenObserve tools. It prefers the
// OS keychain and transparently falls back to a protected file under the config
// directory when no keychain is available.
//
// The store itself lives in github.com/angelmsger/agent-cli-kit/credstore,
// shared by every CLI in the family; this package re-exports it under
// openobserve-cli's keychain service name (constants.KeychainService).
package credstore

import (
	kit "github.com/angelmsger/agent-cli-kit/credstore"
	"github.com/angelmsger/openobserve-cli/pkg/constants"
)

type (
	// Store persists secrets, preferring the OS keychain over its file fallback.
	// Windows encrypts the fallback with per-user DPAPI; other platforms write a
	// 0600 file.
	Store = kit.Store
	// Backend is the OS keychain a Store writes to. Exported so callers can
	// supply a stub in tests; NewStore wires the real one.
	Backend = kit.Backend
	// StoreAccessError means the credential store could not be inspected, as
	// opposed to holding no secret (ErrSecretNotFound).
	StoreAccessError = kit.StoreAccessError
)

// ErrSecretNotFound is returned by a Store when no secret exists for an account.
var ErrSecretNotFound = kit.ErrSecretNotFound

// Backend names reported by Save.
const (
	BackendKeychain = kit.BackendKeychain
	BackendFile     = kit.BackendFile
)

// NewStore returns a Store whose file fallback lives in dir. On macOS it first
// checks for a default keychain, so a sandbox without one never raises the
// "Reset To Defaults" dialog.
func NewStore(dir string) *Store { return kit.NewStore(dir, constants.KeychainService) }

// NewStoreWithBackend returns a Store using the supplied keychain backend. It
// skips the macOS default-keychain probe and always attempts b: the probe
// guards the system keychain only.
func NewStoreWithBackend(dir string, b Backend) *Store {
	return kit.NewStoreWithBackend(dir, constants.KeychainService, b)
}
