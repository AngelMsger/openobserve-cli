package app

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	assets "github.com/angelmsger/openobserve-cli"
	"github.com/spf13/cobra"
)

// Walk the real tree so a future command cannot silently be absent from the
// agent-facing index. Check paths and flags in that index without running them.
func TestSkillCommandIndexMatchesTree(t *testing.T) {
	data, err := assets.SkillFS.ReadFile(assets.SkillRoot + "/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	_, commands, ok := strings.Cut(string(data), "## Commands\n")
	if !ok {
		t.Fatal("missing Commands index")
	}
	commands, _, _ = strings.Cut(commands, "\n## ")
	root := NewRootCmd()
	var leaves []*cobra.Command
	var walk func(*cobra.Command)
	walk = func(cmd *cobra.Command) {
		if cmd.Hidden {
			return
		}
		if !cmd.HasSubCommands() {
			leaves = append(leaves, cmd)
			if !regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(cmd.CommandPath()) + `(?:\s|$)`).MatchString(commands) {
				t.Errorf("Skill index omits %s", cmd.CommandPath())
			}
		}
		for _, child := range cmd.Commands() {
			walk(child)
		}
	}
	walk(root)
	for _, line := range strings.Split(commands, "\n") {
		if !strings.HasPrefix(line, "openobserve-cli ") || strings.HasPrefix(line, "openobserve-cli completion ") {
			continue // Cobra adds the standard completion commands at execution time.
		}
		var found *cobra.Command
		for _, cmd := range leaves {
			if line == cmd.CommandPath() || strings.HasPrefix(line, cmd.CommandPath()+" ") {
				found = cmd
				break
			}
		}
		if found == nil {
			t.Errorf("unknown command in Skill index: %s", line)
			continue
		}
		for _, flag := range regexp.MustCompile(`--([a-z][a-z-]*)`).FindAllStringSubmatch(line, -1) {
			if found.Flags().Lookup(flag[1]) == nil && found.InheritedFlags().Lookup(flag[1]) == nil {
				t.Errorf("unknown flag --%s in %s", flag[1], line)
			}
		}
	}
}

func TestSkillMutationsRejectUnexpectedArguments(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "openobserve")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "SKILL.md")
	const existing = "existing Skill must be preserved"
	if err := os.WriteFile(file, []byte(existing), 0600); err != nil {
		t.Fatal(err)
	}
	for _, verb := range []string{"install", "uninstall"} {
		if err := runRoot(t, "skill", verb, "unexpected", "--dir", base); err == nil {
			t.Fatalf("skill %s silently accepted an unexpected argument", verb)
		}
		data, err := os.ReadFile(file)
		if err != nil || string(data) != existing {
			t.Fatalf("invalid skill %s modified the install: %s / %v", verb, data, err)
		}
	}
	for _, args := range [][]string{{"skill", "status"}, {"skill", "path"}, {"skill", "show"}, {"version"}} {
		if err := runRoot(t, append(args, "unexpected")...); err == nil {
			t.Fatalf("unexpected argument accepted: %v", args)
		}
	}
}
