package config

import (
	kit "github.com/angelmsger/agent-cli-kit/config"
	"github.com/angelmsger/openobserve-cli/pkg/constants"
)

// NormalizeServiceURL retains the deployment path and rejects credential-bearing
// URLs; a bare host gets http://. It is used at setup and persistence
// boundaries, never to derive an API route.
func NormalizeServiceURL(raw string) (string, error) { return kit.NormalizeServiceURL(raw) }

// ValidateService checks only public setup fields and never probes a server.
func ValidateService(cfg Config) error { return kit.ValidateService(kit.Encode(cfg)) }

// FieldChange describes a public configuration change; credentials are never included.
type FieldChange = kit.FieldChange

// ContextPlan is shared by preview and execution. File is the exact proposed file.
type ContextPlan = kit.ContextPlan[NamedContext, Defaults]

// PlanServiceContext builds a non-secret, target-specific merge without side
// effects: the service URL, organization, auth scheme and credential page from
// resolved. An existing browser-session context refuses a new service or
// scheme (BROWSER_MANAGED_SESSION) before any conflict is reported.
func PlanServiceContext(file File, name string, resolved *Resolved, overwrite, activate bool) (ContextPlan, error) {
	return kit.PlanServiceContext(file, name, kit.Encode(resolved.Config), resolved.Sources, overwrite, activate)
}

// AuthGuide is an offline acquisition guide, not a capability or authentication probe.
type AuthGuide = kit.AuthGuide

// Guide derives display-only links. No request is ever sent to CredentialURL.
func Guide(cfg Config, sources map[string]string) (AuthGuide, error) {
	if err := ValidateService(cfg); err != nil {
		return AuthGuide{}, err
	}
	base, _ := NormalizeServiceURL(cfg.BaseURL)
	g := AuthGuide{Server: base, Scheme: cfg.Auth.Scheme, CredentialURL: base, Source: "fallback",
		NextSteps: []string{constants.AppName + " auth login"}}
	g.DocumentationURL = "https://openobserve.ai/docs/user-guide/account-administration/identity-and-access-management/service-accounts/"
	switch cfg.Auth.Scheme {
	case "session":
		g.CredentialURL = base + "/web/login"
		g.Source = "builtin"
		g.Instructions = []string{"Use auth login --browser to complete SSO in a browser; do not paste browser cookies into a password prompt."}
		g.NextSteps = []string{constants.AppName + " auth login --browser"}
	case "token":
		g.Instructions = []string{"Obtain a credential authorized for query APIs from your administrator. Service-account availability depends on the edition (IAM > Service Accounts).", "Supply base64(email:token) or a full Basic/Bearer Authorization value, not a raw service-account secret. Ingestion-only and RUM tokens do not authorize queries."}
	default:
		g.Instructions = []string{"Use your account email and password for local authentication. For SSO use auth login --browser."}
	}
	if cfg.Auth.CredentialURL != "" {
		g.CredentialURL = cfg.Auth.CredentialURL
		g.Source = sources[fieldCredentialURL]
		if g.Source == "" {
			g.Source = "config"
		}
	}
	return g, nil
}

// WithCredentialGuide preserves host-store recovery and appends acquisition guidance
// only to absence errors, never to inaccessible-store errors.
func WithCredentialGuide(err error, cfg Config) error {
	return kit.WithCredentialGuide(err, func() (AuthGuide, error) { return Guide(cfg, nil) })
}
