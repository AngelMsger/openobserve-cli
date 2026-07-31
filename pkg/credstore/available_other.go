//go:build !darwin

package credstore

// probeKeychain is macOS-specific: only the macOS Security framework answers a
// missing keychain with a modal dialog rather than an error (see
// available_darwin.go). The Windows Credential Manager and the Linux Secret
// Service both fail cleanly, so the store attempts them directly and falls back
// on error.
// A var, not a func, so tests can substitute it (matching available_darwin.go).
var probeKeychain = func() bool { return true }
