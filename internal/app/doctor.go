package app

import (
	"github.com/angelmsger/openobserve-cli/internal/apiclient"
	"github.com/spf13/cobra"
)

func newDoctorCmd(s *appState) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check configuration, credentials and connectivity",
		Long: "Runs a quick health check: is a server configured, are credentials\n" +
			"present, and can the server be reached and authenticated. Each check\n" +
			"reports ok/failed so an agent can self-diagnose environment problems.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg := s.cfg()
			checks := []map[string]any{}
			add := func(name string, ok bool, detail string) {
				checks = append(checks, map[string]any{"check": name, "ok": ok, "detail": detail})
			}

			add("config_dir", true, s.cfgDir)
			serverOK := cfg.BaseURL != ""
			add("server_configured", serverOK, cfg.BaseURL)

			credOK := false
			client, err := s.newClient()
			if err != nil {
				add("credentials", false, err.Error())
			} else {
				credOK = true
				add("credentials", true, "scheme "+cfg.Auth.Scheme)
			}

			connOK := false
			if credOK {
				ctx, cancel := cmdContext(s)
				defer cancel()
				orgs, perr := client.Ping(ctx)
				if perr != nil {
					add("connectivity", false, perr.Error())
				} else {
					connOK = true
					add("connectivity", true, "reached and authenticated")
					add("org_visible", containsOrg(orgs, s.org()), s.org())
				}
			}

			healthy := serverOK && credOK && connOK
			return s.emit(map[string]any{
				"healthy": healthy,
				"version": versionString(),
				"checks":  checks,
			})
		},
	}
}

func containsOrg(orgs []apiclient.Org, want string) bool {
	for _, o := range orgs {
		if o.Identifier == want {
			return true
		}
	}
	return false
}
