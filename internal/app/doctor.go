package app

import (
	"context"
	"net/http"
	"time"

	"github.com/angelmsger/openobserve-cli/internal/apiclient"
	"github.com/angelmsger/openobserve-cli/internal/update"
	"github.com/angelmsger/openobserve-cli/pkg/constants"
	"github.com/spf13/cobra"
)

// updateCheckTimeout caps the release-update lookup so an offline or slow
// network never stalls `doctor` for the full request timeout.
const updateCheckTimeout = 5 * time.Second

func newDoctorCmd(s *appState) *cobra.Command {
	var skipUpdate bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check configuration, credentials and connectivity",
		Long: "Runs a quick health check: is a server configured, are credentials\n" +
			"present, and can the server be reached and authenticated. Each check\n" +
			"reports ok/failed so an agent can self-diagnose environment problems.\n" +
			"It also reports whether a newer openobserve-cli release is available.",
		Example: "  openobserve-cli doctor\n" +
			"  openobserve-cli doctor --no-update-check",
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
			report := map[string]any{
				"healthy": healthy,
				"version": versionString(),
				"checks":  checks,
			}
			// Release-update check: informational only, it never affects health
			// and never fails the command — being out of date is not a fault.
			if !skipUpdate {
				ctx, cancel := updateContext(s)
				defer cancel()
				st := update.Check(ctx, &http.Client{Timeout: updateCheckTimeout}, constants.Version)
				report["update"] = st
			}
			return s.emit(report)
		},
	}
	cmd.Flags().BoolVar(&skipUpdate, "no-update-check", false,
		"skip the check for a newer openobserve-cli release")
	return cmd
}

// updateContext bounds the release-update lookup by updateCheckTimeout, or the
// configured request timeout when that is shorter.
func updateContext(s *appState) (context.Context, context.CancelFunc) {
	d := updateCheckTimeout
	if t := s.timeout(); t > 0 && t < d {
		d = t
	}
	return context.WithTimeout(context.Background(), d)
}

func containsOrg(orgs []apiclient.Org, want string) bool {
	for _, o := range orgs {
		if o.Identifier == want {
			return true
		}
	}
	return false
}
