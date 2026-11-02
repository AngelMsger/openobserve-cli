package openobservecli

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func skillBashExample(t *testing.T, reference string) string {
	t.Helper()
	data, err := SkillFS.ReadFile(SkillRoot + "/references/" + reference + ".md")
	if err != nil {
		t.Fatal(err)
	}
	markdown := strings.ReplaceAll(string(data), "\r\n", "\n")
	_, rest, ok := strings.Cut(markdown, "```bash\n")
	if !ok {
		t.Fatal("no bash example in " + reference)
	}
	script, _, ok := strings.Cut(rest, "```")
	if !ok {
		t.Fatal("unterminated bash example")
	}
	return script
}

func TestSkillBackupExamplesPropagateFailures(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash unavailable")
	}
	for _, reference := range []string{"alerts", "dashboards"} {
		script := skillBashExample(t, reference)
		for _, failure := range []string{"none", "list", "jq", "get"} {
			t.Run(reference+"/"+failure, func(t *testing.T) {
				// The same folder must survive listing and individual reads. Stub
				// jq too, so this shell contract runs without an installed jq binary.
				stub := `openobserve-cli() {
  printf '%s\n' "$*" >> calls
  if [ "$2" = "$FAILURE" ]; then return 5; fi
  printf '{"ok":true}\n'
}
jq() {
  if [ "$FAILURE" = jq ]; then return 4; fi
  printf 'resource1\n'
}
`
				dir := t.TempDir()
				cmd := exec.Command(bash, "-c", stub+strings.ReplaceAll(script, "folder=default", "folder=team-ops"))
				cmd.Dir = dir
				cmd.Env = append(os.Environ(), "FAILURE="+failure)
				out, err := cmd.CombinedOutput()
				want := 0
				if failure != "none" {
					want = 5
					if failure == "jq" {
						want = 4
					}
				}
				if cmd.ProcessState == nil || cmd.ProcessState.ExitCode() != want {
					t.Fatalf("exit=%v, want %d: %s", err, want, out)
				}
				calls, err := os.ReadFile(dir + "/calls")
				if err != nil {
					t.Fatal(err)
				}
				if failure == "list" && strings.Contains(string(calls), " get ") {
					t.Fatal("failed discovery still fetched resources")
				}
				if failure == "none" && !strings.Contains(string(calls), "get resource1 --folder team-ops") {
					t.Fatalf("folder lost in backup: %s", calls)
				}
			})
		}
	}
}

func TestSkillErrorExamplePreservesExitCode(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash unavailable")
	}
	script := skillBashExample(t, "errors-and-exit-codes")
	for _, code := range []int{0, 3, 4, 5, 6, 7, 8, 9, 11} {
		cmd := exec.Command(bash, "-c", fmt.Sprintf("openobserve-cli() { return %d; }\n", code)+script)
		cmd.Dir = t.TempDir()
		out, err := cmd.CombinedOutput()
		if cmd.ProcessState == nil || cmd.ProcessState.ExitCode() != code {
			t.Fatalf("exit=%v want %d: %s", err, code, out)
		}
	}
}
