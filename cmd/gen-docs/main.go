// Command gen-docs walks the cobra command tree and writes a Markdown CLI
// reference under docs/cli/, keeping the docs in lock-step with --help.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/angelmsger/openobserve-cli/internal/app"
	"github.com/spf13/cobra"
)

func main() {
	root := app.NewRootCmd()
	root.DisableAutoGenTag = true
	outDir := "docs/cli"
	if len(os.Args) > 1 {
		outDir = os.Args[1]
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := walk(root, outDir); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// walk renders one Markdown file per command, recursing into subcommands.
func walk(cmd *cobra.Command, dir string) error {
	if !cmd.IsAvailableCommand() && cmd.Name() != cmd.Root().Name() {
		return nil
	}
	name := strings.ReplaceAll(cmd.CommandPath(), " ", "_")
	path := filepath.Join(dir, name+".md")
	var b strings.Builder
	fmt.Fprintf(&b, "# `%s`\n\n%s\n\n", cmd.CommandPath(), cmd.Short)
	if cmd.Long != "" {
		fmt.Fprintf(&b, "%s\n\n", cmd.Long)
	}
	if cmd.Runnable() {
		fmt.Fprintf(&b, "```\n%s\n```\n\n", cmd.UseLine())
	}
	if cmd.Example != "" {
		fmt.Fprintf(&b, "## Examples\n\n```\n%s\n```\n\n", cmd.Example)
	}
	if cmd.HasAvailableLocalFlags() {
		fmt.Fprintf(&b, "## Flags\n\n```\n%s```\n\n", cmd.LocalFlags().FlagUsages())
	}
	if cmd.HasAvailableSubCommands() {
		fmt.Fprintf(&b, "## Subcommands\n\n")
		for _, sub := range cmd.Commands() {
			if sub.IsAvailableCommand() {
				fmt.Fprintf(&b, "- `%s` — %s\n", sub.CommandPath(), sub.Short)
			}
		}
		b.WriteString("\n")
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return err
	}
	for _, sub := range cmd.Commands() {
		if err := walk(sub, dir); err != nil {
			return err
		}
	}
	return nil
}
