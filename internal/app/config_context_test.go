package app

import (
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/angelmsger/openobserve-cli/internal/config"
	cerrors "github.com/angelmsger/openobserve-cli/pkg/errors"
	"github.com/zalando/go-keyring"
)

// isolateConfig points the command tree at a mocked keychain, a clean
// OPENOBSERVE_* environment and an empty working directory (no .env), and
// returns a fresh config directory.
func isolateConfig(t *testing.T) string {
	t.Helper()
	keyring.MockInit()
	for _, name := range []string{
		"OPENOBSERVE_URL", "OPENOBSERVE_ORG", "OPENOBSERVE_AUTH_SCHEME", "OPENOBSERVE_CREDENTIAL_URL",
		"OPENOBSERVE_EMAIL", "OPENOBSERVE_PASSWORD", "OPENOBSERVE_TOKEN", "OPENOBSERVE_CONTEXT",
		"OPENOBSERVE_FORMAT", "OPENOBSERVE_CLI_READ_ONLY",
	} {
		t.Setenv(name, "")
	}
	t.Setenv("OPENOBSERVE_CLI_NO_SKILL_HINT", "1")
	t.Setenv("OPENOBSERVE_CLI_NO_UPDATE_NOTIFIER", "1")
	t.Chdir(t.TempDir())
	return t.TempDir()
}

// runConfig runs the command tree against cfgDir and returns what the command
// wrote to stdout.
func runConfig(t *testing.T, cfgDir string, args ...string) (string, error) {
	t.Helper()
	out, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatal(err)
	}
	sink, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	oldOut, oldErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = out, sink
	defer func() {
		os.Stdout, os.Stderr = oldOut, oldErr
		_ = out.Close()
		_ = sink.Close()
	}()
	cmd := NewRootCmd()
	cmd.SetArgs(append([]string{"--config", cfgDir}, args...))
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	runErr := cmd.Execute()
	raw, err := os.ReadFile(out.Name())
	if err != nil {
		t.Fatal(err)
	}
	return string(raw), runErr
}

func writeContexts(t *testing.T, cfgDir, current string, contexts ...config.NamedContext) {
	t.Helper()
	if err := config.WriteFile(cfgDir, config.File{CurrentContext: current, Contexts: contexts}); err != nil {
		t.Fatal(err)
	}
}

func basicContext(name, server string) config.NamedContext {
	return config.NamedContext{Name: name, BaseURL: server, Org: "default",
		Auth: config.AuthConfig{Scheme: config.SchemeBasic, Username: "alice@example.test"}}
}

func wantCode(t *testing.T, err error, code string) *cerrors.CLIError {
	t.Helper()
	if err == nil {
		t.Fatalf("expected %s, got success", code)
	}
	ce := cerrors.AsCLIError(err)
	if ce.Code != code {
		t.Fatalf("got %s (%v), want %s", ce.Code, err, code)
	}
	return ce
}

// get-contexts is the listing command; contexts is its earlier name, kept
// working with the same output but left out of help.
func TestGetContextsAndItsHiddenName(t *testing.T) {
	cfgDir := isolateConfig(t)
	alpha, beta := basicContext("alpha", "http://a.example.test"), basicContext("beta", "http://b.example.test")
	beta.Org = "team-a"
	writeContexts(t, cfgDir, "beta", alpha, beta)
	listed, err := runConfig(t, cfgDir, "config", "get-contexts")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"name": "alpha"`, `"base_url": "http://b.example.test"`, `"org": "team-a"`, `"current": true`} {
		if !strings.Contains(listed, want) {
			t.Errorf("get-contexts output lacks %s:\n%s", want, listed)
		}
	}
	old, err := runConfig(t, cfgDir, "config", "contexts")
	if err != nil || old != listed {
		t.Errorf("contexts differs from get-contexts (%v):\n%s", err, old)
	}

	root := NewRootCmd()
	hidden, _, err := root.Find([]string{"config", "contexts"})
	if err != nil || hidden.Name() != "contexts" || !hidden.Hidden || hidden.Deprecated != "" || len(hidden.Aliases) != 0 {
		t.Errorf("contexts must be a hidden command of its own: %+v %v", hidden, err)
	}
	cfgCmd, _, _ := root.Find([]string{"config"})
	if help := cfgCmd.UsageString(); !strings.Contains(help, "get-contexts") || strings.Contains(help, "  contexts ") ||
		!strings.Contains(help, "delete-context") {
		t.Errorf("config help must list get-contexts and delete-context, not contexts:\n%s", help)
	}
}

func TestUseContextNamesTheListingCommand(t *testing.T) {
	cfgDir := isolateConfig(t)
	writeContexts(t, cfgDir, "alpha", basicContext("alpha", "http://a.example.test"), basicContext("Beta", "http://b.example.test"))
	_, err := runConfig(t, cfgDir, "config", "use-context", "ghost")
	ce := wantCode(t, err, "UNKNOWN_CONTEXT")
	if !reflect.DeepEqual(ce.NextSteps, []string{"openobserve-cli config get-contexts"}) || ce.Hint != "Available contexts: alpha, Beta." {
		t.Errorf("hint = %q, next steps = %v", ce.Hint, ce.NextSteps)
	}
	// A lookup ignores case and keeps the stored spelling.
	if out, err := runConfig(t, cfgDir, "config", "use-context", "BETA"); err != nil || !strings.Contains(out, `"current_context": "Beta"`) {
		t.Fatalf("use-context BETA: %v\n%s", err, out)
	}
	if file, _, _ := config.ReadFile(cfgDir); file.CurrentContext != "Beta" {
		t.Errorf("current_context = %q", file.CurrentContext)
	}
}

// Every other pointer at the listing command names get-contexts.
func TestListingStepsNameGetContexts(t *testing.T) {
	cfgDir := isolateConfig(t)
	writeContexts(t, cfgDir, "alpha", basicContext("alpha", "http://a.example.test"), basicContext("beta", "http://b.example.test"))
	_, err := runConfig(t, cfgDir, "--use-context", "ghost", "config", "show")
	if ce := wantCode(t, err, "UNKNOWN_CONTEXT"); !reflect.DeepEqual(ce.NextSteps, []string{"openobserve-cli config get-contexts"}) {
		t.Errorf("loader next steps = %v", ce.NextSteps)
	}
	if got := contextBaseURLMismatchError("alpha", "http://a.example.test", "http://b.example.test"); !strings.Contains(
		strings.Join(cerrors.AsCLIError(got).NextSteps, "\n"), "openobserve-cli config get-contexts") {
		t.Errorf("CONTEXT_BASE_URL_MISMATCH next steps = %v", cerrors.AsCLIError(got).NextSteps)
	}
}

func TestDeleteContext(t *testing.T) {
	cfgDir := isolateConfig(t)
	_, err := runConfig(t, cfgDir, "config", "delete-context", "alpha")
	wantCode(t, err, "NO_CONFIG")

	writeContexts(t, cfgDir, "alpha", basicContext("alpha", "http://a.example.test"),
		basicContext("beta", "http://b.example.test"), basicContext("gamma", "http://c.example.test"))
	_, err = runConfig(t, cfgDir, "config", "delete-context", "ghost")
	if ce := wantCode(t, err, "UNKNOWN_CONTEXT"); !reflect.DeepEqual(ce.NextSteps, []string{"openobserve-cli config get-contexts"}) {
		t.Errorf("next steps = %v", ce.NextSteps)
	}

	// Deleting the current context, named in another case, makes the first
	// remaining context current.
	out, err := runConfig(t, cfgDir, "config", "delete-context", "ALPHA")
	if err != nil || !strings.Contains(out, `"context": "alpha"`) || !strings.Contains(out, `"status": "deleted"`) {
		t.Fatalf("delete-context ALPHA: %v\n%s", err, out)
	}
	file, _, _ := config.ReadFile(cfgDir)
	if file.CurrentContext != "beta" || !reflect.DeepEqual(file.ContextNames(), []string{"beta", "gamma"}) {
		t.Fatalf("after deleting the current context: %+v", file)
	}
	// Deleting another context leaves the current one alone.
	if _, err := runConfig(t, cfgDir, "config", "delete-context", "gamma"); err != nil {
		t.Fatal(err)
	}
	if file, _, _ = config.ReadFile(cfgDir); file.CurrentContext != "beta" || len(file.Contexts) != 1 {
		t.Fatalf("after deleting gamma: %+v", file)
	}
	_, err = runConfig(t, cfgDir, "config", "delete-context", "beta")
	if ce := wantCode(t, err, "LAST_CONTEXT"); ce.Category != cerrors.CategoryUsage {
		t.Errorf("LAST_CONTEXT category = %s", ce.Category)
	}
}
