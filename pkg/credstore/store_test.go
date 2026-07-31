package credstore

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/zalando/go-keyring"
)

// stubKeyring records whether it was touched, so a test can prove the store did
// NOT reach the OS keychain.
type stubKeyring struct {
	err     error
	touched bool
	items   map[string]string
}

func newStub(err error) *stubKeyring {
	return &stubKeyring{err: err, items: map[string]string{}}
}

func (s *stubKeyring) Get(_, account string) (string, error) {
	s.touched = true
	if s.err != nil {
		return "", s.err
	}
	v, ok := s.items[account]
	if !ok {
		return "", keyring.ErrNotFound
	}
	return v, nil
}

func (s *stubKeyring) Set(_, account, secret string) error {
	s.touched = true
	if s.err != nil {
		return s.err
	}
	s.items[account] = secret
	return nil
}

func (s *stubKeyring) Delete(_, account string) error {
	s.touched = true
	if s.err != nil {
		return s.err
	}
	delete(s.items, account)
	return nil
}

// withKeychain forces the availability probe for the duration of a test.
func withKeychain(t *testing.T, available bool) {
	t.Helper()
	prev := probeKeychain
	probeKeychain = func() bool { return available }
	t.Cleanup(func() { probeKeychain = prev })
}

func TestSavePrefersTheKeychain(t *testing.T) {
	withKeychain(t, true)
	stub := newStub(nil)
	s := NewStoreWithBackend(t.TempDir(), stub)

	backend, err := s.Save("host:basic", "hunter2")
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if backend != BackendKeychain {
		t.Fatalf("Save() backend = %q, want %q", backend, BackendKeychain)
	}
	got, err := s.Load("host:basic")
	if err != nil || got != "hunter2" {
		t.Fatalf("Load() = %q, %v; want the stored secret", got, err)
	}
}

func TestFallsBackToFileWhenTheKeychainErrors(t *testing.T) {
	withKeychain(t, true)
	dir := t.TempDir()
	s := NewStoreWithBackend(dir, newStub(errors.New("keychain is locked")))

	backend, err := s.Save("host:token", "tok")
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if backend != BackendFile {
		t.Fatalf("Save() backend = %q, want %q", backend, BackendFile)
	}
	got, err := s.Load("host:token")
	if err != nil || got != "tok" {
		t.Fatalf("Load() = %q, %v; want the file-stored secret", got, err)
	}
}

// The point of the whole exercise: with no default keychain, the store must not
// touch the OS keychain at all. On macOS that write is what raises the
// "Keychain Not Found / Reset To Defaults" system dialog and exits 154.
func TestNeverTouchesAnUnavailableKeychain(t *testing.T) {
	withKeychain(t, false)
	stub := newStub(nil)
	s := NewStoreWithBackend(t.TempDir(), stub)

	backend, err := s.Save("host:session", "blob")
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if backend != BackendFile {
		t.Fatalf("Save() backend = %q, want %q", backend, BackendFile)
	}
	if _, err := s.Load("host:session"); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := s.Delete("host:session"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if stub.touched {
		t.Fatal("the store reached the OS keychain despite no default keychain being available")
	}
}

func TestMissingSecretIsNotAnAccessError(t *testing.T) {
	withKeychain(t, true)
	s := NewStoreWithBackend(t.TempDir(), newStub(nil))
	if _, err := s.Load("host:basic"); !errors.Is(err, ErrSecretNotFound) {
		t.Fatalf("Load() error = %v, want ErrSecretNotFound", err)
	}
}

// A keychain that is present but inaccessible must NOT be reported as "no
// credential configured" — that would send the user off reconfiguring a
// credential that is already there.
func TestInaccessibleKeychainReportsAccessError(t *testing.T) {
	withKeychain(t, true)
	s := NewStoreWithBackend(t.TempDir(), newStub(errors.New("sandboxed")))
	_, err := s.Load("host:basic")
	var access *StoreAccessError
	if !errors.As(err, &access) {
		t.Fatalf("Load() error = %v, want a StoreAccessError", err)
	}
	if access.Backend != BackendKeychain {
		t.Fatalf("access error backend = %q, want %q", access.Backend, BackendKeychain)
	}
}

func TestDeleteRemovesFromBothBackends(t *testing.T) {
	withKeychain(t, true)
	dir := t.TempDir()
	stub := newStub(nil)
	s := NewStoreWithBackend(dir, stub)
	if _, err := s.Save("host:basic", "keychain-copy"); err != nil {
		t.Fatal(err)
	}
	if err := s.fileSave("host:basic", "file-copy"); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete("host:basic"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := s.Load("host:basic"); !errors.Is(err, ErrSecretNotFound) {
		t.Fatalf("Load() after Delete = %v, want ErrSecretNotFound", err)
	}
}

func TestFallbackFileIsNotWorldReadable(t *testing.T) {
	withKeychain(t, false)
	dir := t.TempDir()
	s := NewStoreWithBackend(dir, newStub(nil))
	if _, err := s.Save("host:basic", "hunter2"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, "credentials"))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("credentials file mode = %o, want 600", perm)
	}
}
