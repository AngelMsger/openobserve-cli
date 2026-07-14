package app

import (
	"context"
	"net/http"
	"time"

	"github.com/angelmsger/openobserve-cli/internal/update"
	"github.com/angelmsger/openobserve-cli/pkg/apiclient"
	"github.com/angelmsger/openobserve-cli/pkg/constants"
	cerrors "github.com/angelmsger/openobserve-cli/pkg/errors"
	"github.com/spf13/cobra"
)

// updateCheckTimeout caps the release-update lookup so an offline or slow
// network never stalls `doctor` for the full request timeout.
const updateCheckTimeout = 5 * time.Second

type doctorCheck struct {
	Check         string `json:"check"`
	OK            bool   `json:"ok"`
	Status        string `json:"status"`
	Detail        string `json:"detail"`
	RecoveryScope string `json:"recovery_scope,omitempty"`
}

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
			checks := []doctorCheck{}
			add := func(name string, ok bool, status, detail, recoveryScope string) {
				checks = append(checks, doctorCheck{
					Check: name, OK: ok, Status: status, Detail: detail, RecoveryScope: recoveryScope,
				})
			}

			add("config_dir", true, "ok", s.cfgDir, "")
			serverOK := cfg.BaseURL != ""
			add("server_configured", serverOK, statusForOK(serverOK, "missing"), cfg.BaseURL, "")

			credOK := false
			client, err := s.newClient()
			if err != nil {
				add("credentials", false, diagnosticStatus(err), err.Error(), diagnosticRecoveryScope(err))
			} else {
				credOK = true
				add("credentials", true, "ok", "scheme "+cfg.Auth.Scheme, "")
			}

			connOK := false
			if credOK {
				ctx, cancel := cmdContext(s)
				defer cancel()
				orgs, perr := client.Ping(ctx)
				if perr != nil {
					add("connectivity", false, diagnosticStatus(perr), perr.Error(), diagnosticRecoveryScope(perr))
				} else {
					connOK = true
					add("connectivity", true, "ok", "reached and authenticated", "")
					orgVisible := containsOrg(orgs, s.org())
					add("org_visible", orgVisible, statusForOK(orgVisible, "not_visible"), s.org(), "")
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

func statusForOK(ok bool, failure string) string {
	if ok {
		return "ok"
	}
	return failure
}

func diagnosticStatus(err error) string {
	if err == nil {
		return "ok"
	}
	ce := cerrors.AsCLIError(err)
	switch ce.Code {
	case "CREDENTIAL_STORE_INACCESSIBLE":
		return "inaccessible"
	case "CREDENTIAL_NOT_VISIBLE_OR_MISSING":
		return "missing_or_inaccessible"
	}
	switch ce.Category {
	case cerrors.CategoryAuth, cerrors.CategoryPermission:
		return "rejected_by_server"
	case cerrors.CategoryNetwork, cerrors.CategoryServer:
		return "unreachable"
	default:
		return "invalid"
	}
}

func diagnosticRecoveryScope(err error) string {
	if err == nil {
		return ""
	}
	if recovery := cerrors.AsCLIError(err).Recovery; recovery != nil {
		return recovery.Scope
	}
	return ""
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
