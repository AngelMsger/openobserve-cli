package app

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentIDs(t *testing.T) {
	t.Parallel()
	want := []string{
		"claude-code", "codex", "cursor", "agents", "gemini", "github-copilot",
		"opencode", "continue", "windsurf", "grok", "pi", "kilo", "roo",
	}
	ids := agentIDs()
	if len(ids) != len(want) {
		t.Fatalf("agentIDs() = %v (%d), want %d entries", ids, len(ids), len(want))
	}
	got := map[string]bool{}
	for _, id := range ids {
		got[id] = true
	}
	for _, id := range want {
		if !got[id] {
			t.Errorf("missing agent id %q", id)
		}
	}
}

func TestAgentDests(t *testing.T) {
	t.Parallel()
	cases := []struct {
		id              string
		wantHomeSuffix  string
		wantProjectPath string
	}{
		{"claude-code", filepath.Join(".claude", "skills", "openobserve"), filepath.Join(".claude", "skills", "openobserve")},
		{"codex", filepath.Join(".codex", "skills", "openobserve"), filepath.Join(".agents", "skills", "openobserve")},
		{"cursor", filepath.Join(".cursor", "skills", "openobserve"), filepath.Join(".cursor", "skills", "openobserve")},
		{"agents", filepath.Join(".agents", "skills", "openobserve"), filepath.Join(".agents", "skills", "openobserve")},
		{"gemini", filepath.Join(".gemini", "skills", "openobserve"), filepath.Join(".gemini", "skills", "openobserve")},
		{"github-copilot", filepath.Join(".copilot", "skills", "openobserve"), filepath.Join(".agents", "skills", "openobserve")},
		{"opencode", filepath.Join(".config", "opencode", "skills", "openobserve"), filepath.Join(".opencode", "skills", "openobserve")},
		{"continue", filepath.Join(".continue", "skills", "openobserve"), filepath.Join(".continue", "skills", "openobserve")},
		{"windsurf", filepath.Join(".codeium", "windsurf", "skills", "openobserve"), filepath.Join(".windsurf", "skills", "openobserve")},
		{"grok", filepath.Join(".grok", "skills", "openobserve"), filepath.Join(".grok", "skills", "openobserve")},
		{"pi", filepath.Join(".pi", "agent", "skills", "openobserve"), filepath.Join(".pi", "skills", "openobserve")},
		{"kilo", filepath.Join(".kilocode", "skills", "openobserve"), filepath.Join(".kilocode", "skills", "openobserve")},
		{"roo", filepath.Join(".roo", "skills", "openobserve"), filepath.Join(".roo", "skills", "openobserve")},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.id, func(t *testing.T) {
			t.Parallel()
			spec, ok := agentByID(tc.id)
			if !ok {
				t.Fatalf("agentSpec %q missing", tc.id)
			}
			projectPath, err := agentDest(spec, true)
			if err != nil {
				t.Fatal(err)
			}
			if projectPath != tc.wantProjectPath {
				t.Fatalf("project dest = %q, want %q", projectPath, tc.wantProjectPath)
			}
			homePath, err := agentDest(spec, false)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasSuffix(homePath, tc.wantHomeSuffix) {
				t.Fatalf("home dest %q does not end with %q", homePath, tc.wantHomeSuffix)
			}
		})
	}
}
