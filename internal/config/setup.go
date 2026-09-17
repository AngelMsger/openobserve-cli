package config

import (
	"fmt"
	"net/url"
	"reflect"
	"strings"

	"github.com/angelmsger/openobserve-cli/pkg/constants"
	cerrors "github.com/angelmsger/openobserve-cli/pkg/errors"
)

// serviceField excludes personal identity and secrets before environment inference.
func serviceField(field string) bool {
	switch field {
	case fieldFormat, fieldReadOnly, fieldServer, fieldAuthScheme, fieldCredentialURL, fieldOrg:
		return true
	default:
		return false
	}
}

func resolveAuthDefaults(values, sources map[string]string) {

}

// NormalizeServiceURL retains the deployment path and rejects credential-bearing URLs.
// It is used at setup and persistence boundaries, never to derive an API route.
func NormalizeServiceURL(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", cerrors.New(cerrors.CategoryConfig, "NO_BASE_URL", "no service URL configured").WithNextSteps(constants.AppName + " config set-context <name> --base-url <url>")
	}
	if !strings.Contains(s, "://") {
		s = "http://" + s
	}
	u, err := url.Parse(s)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || strings.ContainsAny(s, "\r\n\t") {
		return "", cerrors.New(cerrors.CategoryConfig, "BAD_BASE_URL", "service URL must be an HTTP(S) URL without credentials, query or fragment")
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	if (u.Scheme == "https" && u.Port() == "443") || (u.Scheme == "http" && u.Port() == "80") {
		host := u.Hostname()
		if strings.Contains(host, ":") {
			host = "[" + host + "]"
		}
		u.Host = host
	}
	return strings.TrimRight(u.String(), "/"), nil
}

func validateCredentialURL(raw string) error {
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || strings.ContainsAny(raw, "\r\n\t") {
		return cerrors.New(cerrors.CategoryConfig, "BAD_CREDENTIAL_URL", "credential URL must be an absolute HTTP(S) page URL without embedded credentials")
	}
	return nil
}

// ValidateService checks only public setup fields and never probes a server.
func ValidateService(cfg Config) error {
	if _, err := NormalizeServiceURL(cfg.BaseURL); err != nil {
		return err
	}
	if err := validateCredentialURL(cfg.Auth.CredentialURL); err != nil {
		return err
	}
	switch cfg.Auth.Scheme {
	case "token", "basic", "session":
	default:
		return cerrors.New(cerrors.CategoryConfig, "AUTH_BAD_SCHEME", "unsupported authentication scheme").WithHint("Use one of: token, basic, session.")
	}

	return nil
}

// FieldChange describes a public configuration change; credentials are never included.
type FieldChange struct {
	Before string `json:"before"`
	After  string `json:"after"`
}

// ContextPlan is shared by preview and execution. File is the exact proposed file.
type ContextPlan struct {
	Context        string                 `json:"context"`
	Changed        bool                   `json:"changed"`
	CurrentContext string                 `json:"current_context"`
	Changes        map[string]FieldChange `json:"changes"`
	NextSteps      []string               `json:"next_steps"`
	File           File                   `json:"-"`
}

// PlanServiceContext builds a non-secret, target-specific merge without side effects.
func PlanServiceContext(file File, name string, resolved *Resolved, overwrite, activate bool) (ContextPlan, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" || strings.ContainsAny(name, "\r\n\t") {
		return ContextPlan{}, cerrors.New(cerrors.CategoryUsage, "CTX_NAME_EMPTY", "provide a non-empty context name")
	}
	old, exists := file.Context(name)
	if exists {
		name = old.Name
	}
	cfg := resolved.Config
	normalized, err := NormalizeServiceURL(cfg.BaseURL)
	if err != nil {
		return ContextPlan{}, err
	}
	cfg.BaseURL = normalized
	if err := ValidateService(cfg); err != nil {
		return ContextPlan{}, err
	}
	next := old
	next.Name = name
	changes := map[string]FieldChange{}
	conflicts := map[string]FieldChange{}
	merge := func(field string, dst *string, value string) {
		source := resolved.Sources[field]
		if exists && *dst != "" && source != "flag" && source != "env" && source != "dotenv" {
			return
		}
		before := *dst
		if field == fieldServer && before != "" {
			if normal, e := NormalizeServiceURL(before); e == nil && normal == value {
				return
			}
		}
		if before == value {
			return
		}
		change := FieldChange{Before: before, After: value}
		changes[field] = change
		if exists && before != "" {
			conflicts[field] = change
		}
		*dst = value
	}
	merge(fieldServer, &next.BaseURL, cfg.BaseURL)
	merge(fieldAuthScheme, &next.Auth.Scheme, cfg.Auth.Scheme)
	merge(fieldCredentialURL, &next.Auth.CredentialURL, cfg.Auth.CredentialURL)
	merge(fieldOrg, &next.Org, cfg.Org)
	if exists && old.Auth.Scheme == "session" && (next.Auth.Scheme != old.Auth.Scheme || next.BaseURL != old.BaseURL) {
		return ContextPlan{}, cerrors.New(cerrors.CategoryConfig, "BROWSER_MANAGED_SESSION", "service presets cannot replace an existing browser session's service or authentication scheme").WithHint("Create a separate context, then sign in using auth login --browser.")
	}
	if len(conflicts) > 0 && !overwrite {
		return ContextPlan{}, cerrors.New(cerrors.CategoryConflict, "CONFIG_CONTEXT_CONFLICT", "team presets conflict with the existing context").WithDetails(conflicts).WithHint("Inspect the field differences; use --overwrite to update the supplied service fields, or choose another context name.").WithNextSteps(constants.AppName + " config set-context --help")
	}
	result := file
	result.Contexts = append([]NamedContext(nil), file.Contexts...)
	replaced := false
	for i, c := range result.Contexts {
		if c.Name == name {
			result.Contexts[i] = next
			replaced = true
			break
		}
	}
	if !replaced {
		result.Contexts = append(result.Contexts, next)
	}
	if len(file.Contexts) == 0 || activate {
		result.CurrentContext = name
	}
	if result.CurrentContext != file.CurrentContext {
		changes["current_context"] = FieldChange{Before: file.CurrentContext, After: result.CurrentContext}
	}
	quotedName := "'" + strings.ReplaceAll(name, "'", "'\"'\"'") + "'"
	return ContextPlan{Context: name, Changed: !reflect.DeepEqual(file, result), CurrentContext: result.CurrentContext, Changes: changes,
		NextSteps: []string{constants.AppName + " --use-context " + quotedName + " auth guide", constants.AppName + " --use-context " + quotedName + " auth login"}, File: result}, nil
}

// AuthGuide is an offline acquisition guide, not a capability or authentication probe.
type AuthGuide struct {
	Server           string   `json:"server"`
	Flavor           string   `json:"flavor,omitempty"`
	Scheme           string   `json:"scheme"`
	CredentialURL    string   `json:"credential_url"`
	Source           string   `json:"source"`
	Instructions     []string `json:"instructions"`
	DocumentationURL string   `json:"documentation_url"`
	NextSteps        []string `json:"next_steps"`
}

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

// Lines returns the same guidance for plain prompts and terminal forms.
func (g AuthGuide) Lines() []string {
	lines := []string{fmt.Sprintf("Service: %s (%s)", g.Server, g.Scheme), "Credential page: " + g.CredentialURL}
	return append(lines, g.Instructions...)
}

// WithCredentialGuide preserves host-store recovery and appends acquisition guidance
// only to absence errors, never to inaccessible-store errors.
func WithCredentialGuide(err error, cfg Config) error {
	ce := cerrors.AsCLIError(err)
	switch ce.Code {
	case "AUTH_NO_TOKEN", "AUTH_NO_BASIC", "CREDENTIAL_NOT_VISIBLE_OR_MISSING":
		if g, e := Guide(cfg, nil); e == nil {
			ce.NextSteps = append(ce.NextSteps, constants.AppName+" auth guide", "Credential page: "+g.CredentialURL)
		}
	}
	return ce
}
