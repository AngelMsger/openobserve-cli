package app

import (
	"testing"

	cerrors "github.com/angelmsger/openobserve-cli/pkg/errors"
)

func TestDoctorCredentialRecoveryStatus(t *testing.T) {
	t.Parallel()
	err := cerrors.New(cerrors.CategoryConfig, "CREDENTIAL_STORE_INACCESSIBLE", "hidden").
		WithRecovery(cerrors.Recovery{Action: "retry_current_command", Scope: "host"})
	if got := diagnosticStatus(err); got != "inaccessible" {
		t.Fatalf("diagnosticStatus() = %q, want inaccessible", got)
	}
	if got := diagnosticRecoveryScope(err); got != "host" {
		t.Fatalf("diagnosticRecoveryScope() = %q, want host", got)
	}
}
