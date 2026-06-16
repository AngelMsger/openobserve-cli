package app

import (
	"github.com/angelmsger/openobserve-cli/internal/apiclient"
	"github.com/angelmsger/openobserve-cli/internal/auth"
	"github.com/angelmsger/openobserve-cli/internal/config"
	cerrors "github.com/angelmsger/openobserve-cli/internal/errors"
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
		Long: "Walks through server URL, organization and credentials, verifies them,\n" +
			"then writes a named context plus the secret. Requires an interactive\n" +
			"terminal; for headless use set OPENOBSERVE_URL / OPENOBSERVE_ORG /\n" +
			"OPENOBSERVE_EMAIL / OPENOBSERVE_PASSWORD (or OPENOBSERVE_TOKEN).",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !stdinIsTTY() {
				return cerrors.New(cerrors.CategoryConfig, "INIT_NEEDS_TTY",
					"config init requires an interactive terminal").
					WithHint("In CI / agent sandboxes use environment variables instead: " +
						"OPENOBSERVE_URL, OPENOBSERVE_ORG, OPENOBSERVE_EMAIL, OPENOBSERVE_PASSWORD (or OPENOBSERVE_TOKEN).").
					WithNextSteps("openobserve-cli auth status")
			}
			cur := s.cfg()
			def := initValues{
				baseURL: cur.BaseURL,
				org:     cur.Org,
				scheme:  cur.Auth.Scheme,
				email:   cur.Auth.Username,
			}
			// --pretty drives the interactive TUI; otherwise plain line prompts.
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

			file, _, err := config.ReadFile(s.cfgDir)
			if err != nil {
				return cerrors.Wrap(err, cerrors.CategoryConfig, "CONFIG_READ", "failed to read config")
			}
			file.Upsert(config.NamedContext{
				Name:    ctxName,
				BaseURL: normURL,
				Org:     org,
				Auth:    config.AuthConfig{Scheme: cred.Scheme, Username: cred.Username},
			})
			file.CurrentContext = ctxName
			if err := config.WriteFile(s.cfgDir, file); err != nil {
				return cerrors.Wrap(err, cerrors.CategoryConfig, "CONFIG_WRITE", "failed to write config")
			}
			return s.emit(map[string]any{
				"configured": true,
				"context":    ctxName,
				"base_url":   normURL,
				"org":        org,
				"scheme":     cred.Scheme,
				"stored_in":  backend,
			})
		},
	}
	cmd.Flags().StringVar(&ctxName, "context", config.DefaultContextName, "name for the context to create or update")
	return cmd
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
