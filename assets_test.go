package openobservecli

import (
	"regexp"
	"strings"
	"testing"
)

func TestSkillHandshakeMatchesVersion(t *testing.T) {
	data, err := SkillFS.ReadFile(SkillRoot + "/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	lf := strings.ReplaceAll(string(data), "\r\n", "\n")
	for _, newline := range []string{"\n", "\r\n"} {
		t.Run("newline="+strings.ReplaceAll(newline, "\n", "LF"), func(t *testing.T) {
			checkSkillHandshake(t, strings.ReplaceAll(lf, "\n", newline))
		})
	}
}

func checkSkillHandshake(t *testing.T, markdown string) {
	t.Helper()
	data := []byte(strings.ReplaceAll(markdown, "\r\n", "\n"))
	version := regexp.MustCompile(`(?m)^version: ([0-9]+\.[0-9]+\.[0-9]+)$`).FindSubmatch(data)
	handshakes := regexp.MustCompile(`OPENOBSERVE_CLI_SKILL=([0-9]+\.[0-9]+\.[0-9]+)`).FindAllSubmatch(data, -1)
	if len(version) != 2 || len(handshakes) == 0 {
		t.Fatal("Skill must declare its version and a versioned handshake")
	}
	for _, handshake := range handshakes {
		if string(handshake[1]) != string(version[1]) {
			t.Errorf("handshake %s differs from Skill version %s", handshake[1], version[1])
		}
	}
}

// codexDescriptionLimit is the maximum Skill `description` length Codex accepts;
// a longer one fails to load with "invalid description: exceeds maximum length
// of 1024 characters". Keep the embedded Skill under it.
const codexDescriptionLimit = 1024

func TestEmbeddedSkillDescriptionWithinCodexLimit(t *testing.T) {
	data, err := SkillFS.ReadFile(SkillRoot + "/SKILL.md")
	if err != nil {
		t.Fatalf("reading embedded SKILL.md: %v", err)
	}
	desc := frontmatterDescription(string(data))
	if desc == "" {
		t.Fatal("no description found in SKILL.md frontmatter")
	}
	if n := len(desc); n > codexDescriptionLimit {
		t.Errorf("SKILL.md description is %d chars, exceeds the %d-char Codex limit; shorten it",
			n, codexDescriptionLimit)
	}
}

// frontmatterDescription returns the single-line YAML `description:` value
// (quotes stripped) from a Skill's frontmatter, or "" if absent.
func frontmatterDescription(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if !strings.HasPrefix(line, "description:") {
			continue
		}
		v := strings.TrimSpace(strings.TrimPrefix(line, "description:"))
		v = strings.TrimPrefix(v, `"`)
		v = strings.TrimSuffix(v, `"`)
		return v
	}
	return ""
}
