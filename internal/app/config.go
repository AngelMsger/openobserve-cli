package app

import (
	"fmt"
	"os"
	"strings"

	"github.com/angelmsger/openobserve-cli/internal/auth"
	"github.com/angelmsger/openobserve-cli/internal/config"
	"github.com/angelmsger/openobserve-cli/pkg/apiclient"
	cerrors "github.com/angelmsger/openobserve-cli/pkg/errors"
	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
)

func newConfigCmd(s *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Set up and inspect configuration and contexts",
	}
	cmd.AddCommand(
		newConfigInitCmd(s),
		newConfigShowCmd(s),
		newConfigContextsCmd(s),
		newConfigUseContextCmd(s),
	)
	return cmd
}

func newConfigInitCmd(s *appState) *cobra.Command {
	var ctxName string
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Interactively configure a context and store credentials",
		Long: "Collects a server URL, organization and credentials, verifies them,\n" +
			"then writes a named context plus the secret. With --pretty it runs an\n" +
			"interactive TUI (requires a terminal); without it, a plain line-by-line\n" +
			"wizard that also works over a pipe. When a configuration already exists,\n" +
			"it first asks whether to edit a context, add a new one, or replace it all\n" +
			"(skip the question with --context <name>). For fully headless setup, set\n" +
			"OPENOBSERVE_URL / OPENOBSERVE_ORG / OPENOBSERVE_EMAIL /\n" +
			"OPENOBSERVE_PASSWORD (or OPENOBSERVE_TOKEN) instead.",
		Example: "  openobserve-cli config init --pretty   # interactive TUI (recommended)\n" +
			"  openobserve-cli config init             # plain line-by-line wizard (scripts, non-TTY)\n" +
			"  openobserve-cli config init --context prod   # add/update a named context directly",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// --pretty drives the interactive TUI and requires a terminal;
			// without it, plain line prompts that also work over a pipe. Gate
			// first so the edit/add/replace question never tries to render a TUI
			// without a terminal.
			if s.gflags.pretty && !stdinIsTTY() {
				return cerrors.New(cerrors.CategoryUsage, "PRETTY_NEEDS_TTY",
					"--pretty requires an interactive terminal for `config init`").
					WithHint("Drop --pretty or run from a terminal.")
			}

			file, _, err := config.ReadFile(s.cfgDir)
			if err != nil {
				return cerrors.Wrap(err, cerrors.CategoryConfig, "CONFIG_READ", "failed to read config")
			}

			// Decide which context to write, what to prefill, and whether to
			// drop the others — asking edit / add / replace when a config already
			// exists, mirroring confluence-cli / bitbucket-cli.
			target, prefill, replaceAll, err := s.resolveInitTarget(cmd, file, ctxName)
			if err != nil {
				return err
			}
			if prefill != nil && prefill.Auth.Scheme == auth.SchemeSession {
				return browserManagedSessionError(target)
			}

			def := initValues{}
			if prefill != nil {
				def = initValues{
					baseURL: prefill.BaseURL,
					org:     prefill.Org,
					scheme:  prefill.Auth.Scheme,
					email:   prefill.Auth.Username,
				}
			}

			collect := runInitPrompts
			if s.gflags.pretty {
				collect = runInitForm
			}
			vals, err := collect(def)
			if err != nil {
				return err
			}
			if vals.scheme != auth.SchemeBasic && vals.scheme != auth.SchemeToken {
				return cerrors.Newf(cerrors.CategoryUsage, "BAD_SCHEME",
					"unknown auth scheme %q (want basic or token)", vals.scheme)
			}
			normURL, err := apiclient.NormalizeBaseURL(vals.baseURL)
			if err != nil {
				return err
			}
			org := vals.org
			cred := auth.Credential{Scheme: vals.scheme, Username: vals.email, Secret: vals.secret}

			backend, err := verifyAndSave(s, normURL, org, cred)
			if err != nil {
				return err
			}

			if replaceAll {
				file.Contexts = nil
			}
			file.Upsert(config.NamedContext{
				Name:    target,
				BaseURL: normURL,
				Org:     org,
				Auth:    config.AuthConfig{Scheme: cred.Scheme, Username: cred.Username},
			})
			file.CurrentContext = target
			if err := config.WriteFile(s.cfgDir, file); err != nil {
				return cerrors.Wrap(err, cerrors.CategoryConfig, "CONFIG_WRITE", "failed to write config")
			}
			return s.emit(map[string]any{
				"configured": true,
				"context":    target,
				"base_url":   normURL,
				"org":        org,
				"scheme":     cred.Scheme,
				"stored_in":  backend,
			})
		},
	}
	cmd.Flags().StringVar(&ctxName, "context", config.DefaultContextName,
		"name for the context to create or update (skips the edit/add/replace prompt)")
	return cmd
}

func browserManagedSessionError(contextName string) error {
	return cerrors.Newf(cerrors.CategoryUsage, "SESSION_BROWSER_MANAGED",
		"context %q uses a browser-captured session whose credentials are managed by o3", contextName).
		WithHint("Browser sessions are managed by o3. Sign in there again to update this context.").
		WithNextSteps("openobserve-cli auth status", "openobserve-cli config contexts")
}

// resolveInitTarget decides which context `config init` will write: the target
// name, the existing context to prefill the wizard from (nil = a fresh setup),
// and whether to drop the other contexts (replace). With no existing config it
// is a plain fresh setup into the default (or --context) name. With existing
// config it asks edit / add / replace — unless --context was given explicitly,
// which is a non-interactive shortcut targeting that name directly.
func (s *appState) resolveInitTarget(cmd *cobra.Command, file config.File, ctxFlag string) (target string, prefill *config.NamedContext, replaceAll bool, err error) {
	if cmd.Flags().Changed("context") {
		if c, ok := file.Context(ctxFlag); ok {
			return c.Name, &c, false, nil
		}
		return ctxFlag, nil, false, nil
	}
	if len(file.Contexts) == 0 {
		return config.DefaultContextName, nil, false, nil
	}

	announceExisting(file)
	action, err := s.askInitAction()
	if err != nil {
		return "", nil, false, err
	}
	switch action {
	case "add":
		name, nerr := s.promptNewContextName(file)
		if nerr != nil {
			return "", nil, false, nerr
		}
		return name, nil, false, nil
	case "replace":
		return config.DefaultContextName, nil, true, nil
	default: // edit
		name := file.CurrentContext
		if name == "" {
			name = file.Contexts[0].Name
		}
		if len(file.Contexts) > 1 {
			name, err = s.selectContext(file)
			if err != nil {
				return "", nil, false, err
			}
		}
		c, _ := file.Context(name)
		return c.Name, &c, false, nil
	}
}

// announceExisting prints the contexts already in the config file to stderr, so
// the user sees what `config init` is about to edit, add to, or replace.
func announceExisting(file config.File) {
	fmt.Fprintln(os.Stderr, "Existing configuration:")
	for _, c := range file.Contexts {
		mark := "  "
		if strings.EqualFold(c.Name, file.CurrentContext) {
			mark = "* "
		}
		server := c.BaseURL
		if server == "" {
			server = "(no server)"
		}
		fmt.Fprintf(os.Stderr, "%s%s — %s\n", mark, c.Name, server)
	}
}

// askInitAction asks whether to edit a context, add a new one, or replace the
// configuration — a huh select under --pretty, a plain prompt otherwise.
func (s *appState) askInitAction() (string, error) {
	if s.gflags.pretty {
		return formSelect("What would you like to do?", []huh.Option[string]{
			huh.NewOption("Edit an existing context", "edit"),
			huh.NewOption("Add a new context", "add"),
			huh.NewOption("Replace the configuration", "replace"),
		}, "edit")
	}
	return promptChoice("What would you like to do (edit/add/replace)",
		[]string{"edit", "add", "replace"}, "edit")
}

// selectContext asks which existing context to edit (only reached when more than
// one exists).
func (s *appState) selectContext(file config.File) (string, error) {
	if s.gflags.pretty {
		opts := make([]huh.Option[string], 0, len(file.Contexts))
		for _, c := range file.Contexts {
			label := c.Name
			if c.BaseURL != "" {
				label += " — " + c.BaseURL
			}
			opts = append(opts, huh.NewOption(label, c.Name))
		}
		return formSelect("Edit which context", opts, file.CurrentContext)
	}
	return promptChoice("Edit which context", file.ContextNames(), file.CurrentContext)
}

// promptNewContextName asks for a new context name, rejecting names already in
// use until a fresh one is given.
func (s *appState) promptNewContextName(file config.File) (string, error) {
	used := map[string]bool{}
	for _, c := range file.Contexts {
		used[strings.ToLower(c.Name)] = true
	}
	for {
		var name string
		var err error
		if s.gflags.pretty {
			name, err = formInput("New context name", "production")
		} else {
			name, err = promptLine("New context name", "")
		}
		if err != nil {
			return "", err
		}
		name = strings.TrimSpace(name)
		if name == "" {
			fmt.Fprintln(os.Stderr, "  a context name is required")
			continue
		}
		if used[strings.ToLower(name)] {
			fmt.Fprintf(os.Stderr, "  context %q already exists; choose another name\n", name)
			continue
		}
		return name, nil
	}
}

func newConfigShowCmd(s *appState) *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Show the resolved configuration with field provenance",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg := s.cfg()
			src := s.resolved.Sources
			return s.emit(map[string]any{
				"base_url":       cfg.BaseURL,
				"org":            cfg.Org,
				"auth_scheme":    cfg.Auth.Scheme,
				"username":       cfg.Auth.Username,
				"format":         cfg.Defaults.Format,
				"timeout":        cfg.Defaults.Timeout.String(),
				"read_only":      cfg.Defaults.ReadOnly,
				"active_context": s.resolved.ActiveContext,
				"config_dir":     s.cfgDir,
				"sources": map[string]any{
					"base_url": config.ExplainField(src, config.FieldServer),
					"org":      config.ExplainField(src, config.FieldOrg),
					"format":   config.ExplainField(src, config.FieldFormat),
				},
			})
		},
	}
}

func newConfigContextsCmd(s *appState) *cobra.Command {
	return &cobra.Command{
		Use:   "contexts",
		Short: "List configured contexts and which one is current",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			file, ok, err := config.ReadFile(s.cfgDir)
			if err != nil {
				return cerrors.Wrap(err, cerrors.CategoryConfig, "CONFIG_READ", "failed to read config")
			}
			if !ok {
				return s.emitList([]any{}, pageInfo{})
			}
			items := make([]map[string]any, 0, len(file.Contexts))
			for _, c := range file.Contexts {
				items = append(items, map[string]any{
					"name":     c.Name,
					"base_url": c.BaseURL,
					"org":      c.Org,
					"current":  c.Name == file.CurrentContext,
				})
			}
			return s.emitList(items, pageInfo{})
		},
	}
}

func newConfigUseContextCmd(s *appState) *cobra.Command {
	return &cobra.Command{
		Use:   "use-context <name>",
		Short: "Set the current context",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			file, ok, err := config.ReadFile(s.cfgDir)
			if err != nil {
				return cerrors.Wrap(err, cerrors.CategoryConfig, "CONFIG_READ", "failed to read config")
			}
			if !ok {
				return cerrors.New(cerrors.CategoryConfig, "NO_CONFIG",
					"no config file yet").WithNextSteps("openobserve-cli config init")
			}
			nc, found := file.Context(name)
			if !found {
				return cerrors.Newf(cerrors.CategoryConfig, "UNKNOWN_CONTEXT",
					"context %q is not defined", name).
					WithHint(config.UnknownContextHint(name, file.ContextNames())).
					WithNextSteps("openobserve-cli config contexts")
			}
			file.CurrentContext = nc.Name
			if err := config.WriteFile(s.cfgDir, file); err != nil {
				return cerrors.Wrap(err, cerrors.CategoryConfig, "CONFIG_WRITE", "failed to write config")
			}
			return s.emit(map[string]any{"current_context": nc.Name})
		},
	}
}
