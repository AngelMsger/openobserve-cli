package errors

import "testing"

// The model is tested in agent-cli-kit; this checks that openobserve-cli's own
// guidance is what an error built through this package carries.
func TestErrorsCarryOpenObserveGuidance(t *testing.T) {
	e := New(CategoryNotFound, "STREAM_NOT_FOUND", "no such stream")
	want := []string{"openobserve-cli org list", "openobserve-cli stream list"}
	if len(e.NextSteps) != len(want) || e.NextSteps[0] != want[0] || e.NextSteps[1] != want[1] {
		t.Fatalf("NextSteps = %v, want openobserve-cli's not-found guidance %v", e.NextSteps, want)
	}
	if ExitCode(e) != ExitNotFound {
		t.Fatalf("ExitCode = %d, want %d", ExitCode(e), ExitNotFound)
	}
}
