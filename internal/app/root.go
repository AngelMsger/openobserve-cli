// Package app wires the cobra command tree and runs the CLI.
package app

import (
	"fmt"
	"os"

	"github.com/angelmsger/openobserve-cli/internal/cliflags"
	cerrors "github.com/angelmsger/openobserve-cli/internal/errors"
	"github.com/angelmsger/openobserve-cli/internal/output"
	"github.com/angelmsger/openobserve-cli/pkg/constants"
	"github.com/spf13/cobra"
)

// NewRootCmd builds the full cobra command tree. It exists so tooling — the
// docs generator (cmd/gen-docs) — can walk the same tree the CLI runs.
func NewRootCmd() *cobra.Command { return newRootCmd() }

// Execute builds and runs the root command, returning a process exit code.
func Execute() int {
	root := newRootCmd()
	// Absorb common LLM argv slips (--streamName -> --stream-name, --limit100
	// -> --limit 100) before cobra parses, echoing each fix to stderr so the
	// data on stdout is untouched and the agent learns the canonical form.
	if corrected, corrections := cliflags.Normalize(os.Args[1:], cliflags.Collect(root)); len(corrections) > 0 {
		root.SetArgs(corrected)
		output.EmitNotice(os.Stderr, map[string]any{"_notice": map[string]any{"corrections": corrections}})
	}
	if err := root.Execute(); err != nil {
		ce := cerrors.AsCLIError(err)
		if ce.Category == cerrors.CategoryInternal && !isCLIError(err) {
			ce = cerrors.Wrap(err, cerrors.CategoryUsage, "USAGE", err.Error())
		}
		output.EmitError(ce, os.Stderr)
		return cerrors.ExitCode(ce)
	}
	return 0
}

func isCLIError(err error) bool {
	_, ok := err.(*cerrors.CLIError)
	return ok
}

// newRootCmd assembles the full command tree.
func newRootCmd() *cobra.Command {
	state := &appState{}

	root := &cobra.Command{
		Use:   constants.AppName,
		Short: "Query an OpenObserve (O2) backend as a coding agent",
		Long: "openobserve-cli discovers streams and runs SQL searches over logs,\n" +
			"metrics and traces in OpenObserve (O2). It emits agent-friendly JSON\n" +
			"with structured errors, and works with self-hosted and Cloud instances.\n\n" +
			"AGENT NOTE: a companion Skill (\"openobserve\") carries the canonical usage,\n" +
			"safety modes, and env setup and is the source of truth for driving this CLI.\n" +
			"If you are an agent, load that Skill before composing commands. Check status\n" +
			"with `openobserve-cli skill status`; install it with `openobserve-cli skill install`.",
		Version:       versionString(),
		SilenceErrors: true,
		SilenceUsage:  true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			output.SetErrorPretty(state.gflags.pretty)
			// Nudge agents that shell out without the companion Skill loaded.
			maybeSkillHint(cmd)
			return state.load()
		},
		// After a command succeeds, surface a one-line update notice on stderr
		// when a newer release is available (cached 24h; never fails the command).
		PersistentPostRunE: func(cmd *cobra.Command, _ []string) error {
			maybeNotifyUpdate(state, cmd)
			return nil
		},
	}

	pf := root.PersistentFlags()
	pf.StringVar(&state.gflags.baseURL, "base-url", "", "OpenObserve server URL (overrides config), e.g. http://localhost:5080")
	pf.StringVar(&state.gflags.org, "org", "", "organization identifier (overrides config)")
	pf.StringVarP(&state.gflags.format, "format", "f", "", "output format: json, table or ndjson")
	pf.StringVar(&state.gflags.fields, "fields", "", "comma-separated dot-path fields to keep")
	pf.StringVar(&state.gflags.timeout, "timeout", "", "request timeout, e.g. 30s")
	pf.StringVar(&state.gflags.configPath, "config", "", "config directory (default ~/.angelmsger/openobserve)")
	pf.StringVar(&state.gflags.useContext, "use-context", "", "use a named context for this invocation")
	pf.BoolVarP(&state.gflags.verbose, "verbose", "v", false, "log request lines on stderr")
	pf.BoolVar(&state.gflags.pretty, "pretty", false,
		"human-friendly mode for interactive terminal use only (agents/scripts should omit): TUI in `config init`, colorized JSON elsewhere; errors without a TTY")
	pf.BoolVar(&state.gflags.allowWrites, "allow-writes", false,
		"override read-only mode (defaults.read_only / OPENOBSERVE_CLI_READ_ONLY) for this invocation")

	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return cerrors.Wrap(err, cerrors.CategoryUsage, "BAD_FLAG", err.Error())
	})
	root.SetVersionTemplate("{{.Name}} {{.Version}}\n")

	enumComplete(root, "format", "json", "table", "ndjson")

	root.AddCommand(
		newOrgCmd(state),
		newStreamCmd(state),
		newSearchCmd(state),
		newMetricsCmd(state),
		newTraceCmd(state),
		newAuthCmd(state),
		newConfigCmd(state),
		newDoctorCmd(state),
		newSkillCmd(state),
		newVersionCmd(),
	)
	return root
}

// versionString renders the version, commit and build time as one line.
func versionString() string {
	return fmt.Sprintf("%s (commit %s, built %s)",
		constants.Version, constants.Commit, constants.BuildTime)
}

// newVersionCmd prints build metadata. It mirrors the `--version` flag.
func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		RunE: func(cmd *cobra.Command, _ []string) error {
			fmt.Fprintf(os.Stdout, "%s %s\n", constants.AppName, versionString())
			return nil
		},
	}
}
