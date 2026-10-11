package credstore

import (
	"testing"

	kit "github.com/angelmsger/agent-cli-kit/credstore"
	"github.com/angelmsger/openobserve-cli/pkg/constants"
)

// recordingBackend remembers the service name of every keychain call.
type recordingBackend struct{ services []string }

func (r *recordingBackend) Get(service, _ string) (string, error) {
	r.services = append(r.services, service)
	return "secret", nil
}

func (r *recordingBackend) Set(service, _, _ string) error {
	r.services = append(r.services, service)
	return nil
}

func (r *recordingBackend) Delete(service, _ string) error {
	r.services = append(r.services, service)
	return nil
}

// The store is tested in agent-cli-kit; this checks openobserve-cli's default:
// its secrets live under the openobserve-cli keychain service.
func TestStoreUsesOpenObserveKeychainService(t *testing.T) {
	b := &recordingBackend{}
	s := NewStoreWithBackend(t.TempDir(), b)
	if _, err := s.Save("host:basic", "hunter2"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load("host:basic"); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete("host:basic"); err != nil {
		t.Fatal(err)
	}
	if len(b.services) != 3 {
		t.Fatalf("keychain calls = %d, want 3", len(b.services))
	}
	for _, got := range b.services {
		if got != constants.KeychainService {
			t.Fatalf("keychain service = %q, want %q", got, constants.KeychainService)
		}
	}
}

// The fallback file the kit writes must be the one constants names, so a
// credential saved before the move to agent-cli-kit is still found.
func TestFallbackFileNameMatchesConstants(t *testing.T) {
	if kit.CredentialsFileName != constants.CredentialsFileName {
		t.Fatalf("kit fallback file = %q, want %q", kit.CredentialsFileName, constants.CredentialsFileName)
	}
}
