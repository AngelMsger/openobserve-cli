package app

import (
	"testing"

	"github.com/angelmsger/openobserve-cli/internal/config"
	cerrors "github.com/angelmsger/openobserve-cli/pkg/errors"
	"github.com/spf13/cobra"
)

func initCmdForTest() *cobra.Command {
	cmd := &cobra.Command{Use: "init"}
	var ctx string
	cmd.Flags().StringVar(&ctx, "context", config.DefaultContextName, "")
	return cmd
}

// The non-interactive branches of resolveInitTarget are pure decisions and must
// hold without any prompting; the edit/add/replace prompts are exercised by the
// e2e suite (scripts/e2e.sh) which drives stdin.

func TestResolveInitTarget_NoExistingConfig(t *testing.T) {
	s := &appState{}
	target, prefill, replaceAll, err := s.resolveInitTarget(initCmdForTest(), config.File{}, config.DefaultContextName)
	if err != nil {
		t.Fatal(err)
	}
	if target != config.DefaultContextName || prefill != nil || replaceAll {
		t.Fatalf("fresh setup: got (%q, %v, %v)", target, prefill, replaceAll)
	}
}

func TestResolveInitTarget_ExplicitContextSkipsPrompt(t *testing.T) {
	file := config.File{
		CurrentContext: "default",
		Contexts: []config.NamedContext{
			{Name: "default", BaseURL: "http://a", Org: "o1"},
		},
	}

	t.Run("new name", func(t *testing.T) {
		s := &appState{}
		cmd := initCmdForTest()
		_ = cmd.Flags().Set("context", "prod") // marks the flag Changed
		target, prefill, replaceAll, err := s.resolveInitTarget(cmd, file, "prod")
		if err != nil {
			t.Fatal(err)
		}
		if target != "prod" || prefill != nil || replaceAll {
			t.Fatalf("explicit new: got (%q, %v, %v)", target, prefill, replaceAll)
		}
	})

	t.Run("existing name prefills, case-insensitive", func(t *testing.T) {
		s := &appState{}
		cmd := initCmdForTest()
		_ = cmd.Flags().Set("context", "DEFAULT")
		target, prefill, replaceAll, err := s.resolveInitTarget(cmd, file, "DEFAULT")
		if err != nil {
			t.Fatal(err)
		}
		if target != "default" || replaceAll {
			t.Fatalf("explicit existing: got (%q, replaceAll=%v)", target, replaceAll)
		}
		if prefill == nil || prefill.BaseURL != "http://a" || prefill.Org != "o1" {
			t.Fatalf("expected prefill from the stored context, got %+v", prefill)
		}
	})
}

func TestBrowserManagedSessionError(t *testing.T) {
	err := browserManagedSessionError("prod")
	ce := cerrors.AsCLIError(err)
	if ce.Code != "SESSION_BROWSER_MANAGED" {
		t.Fatalf("code = %q, want SESSION_BROWSER_MANAGED", ce.Code)
	}
	if len(ce.NextSteps) == 0 {
		t.Fatal("expected actionable next steps")
	}
}

func TestConfigInitRejectsBrowserManagedSessionBeforePrompting(t *testing.T) {
	dir := t.TempDir()
	file := config.File{
		CurrentContext: "prod",
		Contexts: []config.NamedContext{{
			Name:    "prod",
			BaseURL: "https://o2.example.com",
			Org:     "default",
			Auth:    config.AuthConfig{Scheme: "session"},
		}},
	}
	if err := config.WriteFile(dir, file); err != nil {
		t.Fatal(err)
	}

	cmd := NewRootCmd()
	cmd.SetArgs([]string{"--config", dir, "config", "init", "--context", "prod"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("config init error = nil, want browser-managed session error")
	}
	ce := cerrors.AsCLIError(err)
	if ce.Code != "SESSION_BROWSER_MANAGED" {
		t.Fatalf("code = %q, want SESSION_BROWSER_MANAGED", ce.Code)
	}
}
