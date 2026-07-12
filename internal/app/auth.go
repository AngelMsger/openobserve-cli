package app

import (
	"context"

	"github.com/angelmsger/openobserve-cli/internal/auth"
	"github.com/angelmsger/openobserve-cli/pkg/apiclient"
	cerrors "github.com/angelmsger/openobserve-cli/pkg/errors"
	"github.com/spf13/cobra"
)

func newAuthCmd(s *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Log in, check identity and log out",
	}
	cmd.AddCommand(newAuthLoginCmd(s), newAuthStatusCmd(s), newAuthLogoutCmd(s))
	return cmd
}

func newAuthLoginCmd(s *appState) *cobra.Command {
	return &cobra.Command{
		Use:   "login",
		Short: "Store credentials for the active context (interactive)",
		Long: "Prompts for the password (basic) or token, verifies it against the\n" +
			"server, and stores it in the OS keychain. Requires an interactive\n" +
			"terminal; in CI / agent sandboxes set OPENOBSERVE_EMAIL +\n" +
			"OPENOBSERVE_PASSWORD or OPENOBSERVE_TOKEN instead.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !stdinIsTTY() {
				return cerrors.New(cerrors.CategoryAuth, "AUTH_LOGIN_NEEDS_TTY",
					"auth login requires an interactive terminal").
					WithHint("Set OPENOBSERVE_EMAIL + OPENOBSERVE_PASSWORD (or OPENOBSERVE_TOKEN), "+
						"or run `openobserve-cli config init` in a terminal.").
					WithNextSteps("openobserve-cli auth status", "openobserve-cli config init")
			}
			cfg := s.cfg()
			if cfg.BaseURL == "" {
				return cerrors.New(cerrors.CategoryConfig, "NO_BASE_URL",
					"no server configured yet").
					WithNextSteps("openobserve-cli config init")
			}
			scheme := cfg.Auth.Scheme
			if scheme == "" {
				scheme = auth.SchemeBasic
			}
			cred := auth.Credential{Scheme: scheme, Username: cfg.Auth.Username}
			switch scheme {
			case auth.SchemeBasic:
				if cred.Username == "" {
					u, err := promptLine("Email", "")
					if err != nil {
						return err
					}
					cred.Username = u
				}
				pw, err := promptSecret("Password")
				if err != nil {
					return err
				}
				cred.Secret = pw
			case auth.SchemeToken:
				tok, err := promptSecret("Token")
				if err != nil {
					return err
				}
				cred.Secret = tok
			case auth.SchemeSession:
				return browserManagedSessionError(s.resolved.ActiveContext)
			}
			backend, err := verifyAndSave(s, cfg.BaseURL, s.org(), cred)
			if err != nil {
				return err
			}
			return s.emit(map[string]any{
				"logged_in": true,
				"base_url":  cfg.BaseURL,
				"org":       s.org(),
				"scheme":    scheme,
				"stored_in": backend,
			})
		},
	}
}

func newAuthStatusCmd(s *appState) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the active identity and verify connectivity",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg := s.cfg()
			out := map[string]any{
				"base_url": cfg.BaseURL,
				"org":      s.org(),
				"scheme":   cfg.Auth.Scheme,
				"username": cfg.Auth.Username,
				"context":  s.resolved.ActiveContext,
			}
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient()
			if err != nil {
				out["authenticated"] = false
				out["error"] = err.Error()
				return s.emit(out)
			}
			orgs, err := client.Ping(ctx)
			if err != nil {
				out["authenticated"] = false
				out["error"] = err.Error()
				return s.emit(out)
			}
			out["authenticated"] = true
			out["visible_orgs"] = orgIdentifiers(orgs)
			return s.emit(out)
		},
	}
}

func newAuthLogoutCmd(s *appState) *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Remove the stored credential for the active context",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg := s.cfg()
			if cfg.BaseURL == "" {
				return cerrors.New(cerrors.CategoryConfig, "NO_BASE_URL",
					"no server configured").WithNextSteps("openobserve-cli config init")
			}
			scheme := cfg.Auth.Scheme
			if scheme == "" {
				scheme = auth.SchemeBasic
			}
			if err := auth.Forget(cfg.BaseURL, scheme, s.store); err != nil {
				return cerrors.Wrap(err, cerrors.CategoryConfig, "LOGOUT_FAILED",
					"failed to remove stored credential")
			}
			return s.emit(map[string]any{"logged_out": true, "base_url": cfg.BaseURL, "scheme": scheme})
		},
	}
}

// verifyAndSave builds a client from cred, pings the server to confirm the
// credential works, then persists the secret. It returns the storage backend.
func verifyAndSave(s *appState, baseURL, org string, cred auth.Credential) (string, error) {
	if err := cred.Validate(); err != nil {
		return "", err
	}
	client, err := apiclient.Build(apiclient.BuildParams{
		BaseURL:       baseURL,
		Org:           org,
		AuthDecorator: cred.Decorator(),
		Timeout:       s.timeout(),
		MaxRetries:    s.cfg().Defaults.MaxRetries,
	})
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout())
	defer cancel()
	if _, err := client.Ping(ctx); err != nil {
		return "", err
	}
	return auth.Save(baseURL, cred, s.store)
}

// orgIdentifiers extracts the identifier of each org for compact output.
func orgIdentifiers(orgs []apiclient.Org) []string {
	out := make([]string, 0, len(orgs))
	for _, o := range orgs {
		if o.Identifier != "" {
			out = append(out, o.Identifier)
		}
	}
	return out
}
