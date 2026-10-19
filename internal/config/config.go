// Package config resolves CLI configuration from layered sources: CLI flags,
// environment variables, a .env file, a YAML config file and built-in
// defaults, in that precedence order (highest first).
//
// The engine — layering, provenance, context selection, the atomic file
// write and team presets — comes from agent-cli-kit/config. The public
// pkg/config registers openobserve-cli's fields, environment variables, file
// location and command wording with it and keeps the YAML shape of the file;
// this package keeps the names the rest of the repo uses.
//
// Secrets (passwords, tokens) are never stored in the YAML config file. They
// are surfaced through Resolved.Secrets when supplied via flags/env/.env, or
// loaded from the OS keychain by the auth package.
package config

import kit "github.com/angelmsger/agent-cli-kit/config"

// Auth scheme values.
const (
	// SchemeBasic is email+password HTTP Basic auth.
	SchemeBasic = "basic"
	// SchemeToken is a pre-generated credential sent as-is in the
	// Authorization header (the base64 portion of a Basic token, or a full
	// "Basic …"/"Bearer …" value).
	SchemeToken = "token"
)

// DefaultContextName is the name given to an unnamed context.
const DefaultContextName = kit.DefaultContextName

// Context selection sources, reported on Resolved.ContextSource. The first two
// are explicit (the caller named a context for this invocation); the rest are
// implicit (the CLI fell back to a stored or sole context).
const (
	ContextSourceFlag    = kit.ContextSourceFlag    // --use-context
	ContextSourceEnv     = kit.ContextSourceEnv     // OPENOBSERVE_CONTEXT
	ContextSourceCurrent = kit.ContextSourceCurrent // the file's current_context
	ContextSourceSingle  = kit.ContextSourceSingle  // the sole defined context
	ContextSourceDefault = kit.ContextSourceDefault // a context literally named "default"
	ContextSourceNone    = kit.ContextSourceNone    // nothing selected (no/empty config)
)

// NamedContext, AuthConfig, and Defaults are defined (and re-exported) via
// type aliases in file.go from pkg/config.

// Config holds the resolved, non-secret configuration.
type Config struct {
	// CredentialBaseURL preserves an equivalent stored URL's native lookup key.
	// It is runtime-only and never changes the request destination.
	CredentialBaseURL string `yaml:"-" json:"-"`
	// ReuseStep is the `auth reuse` command that goes before any sign-in step
	// for this context, or "" when the file offers nothing to reuse. It is
	// runtime-only, computed offline from the configuration file.
	ReuseStep string     `yaml:"-" json:"-"`
	BaseURL   string     `yaml:"server" config:"server"`
	Org       string     `yaml:"org" config:"org"`
	Auth      AuthConfig `yaml:"auth"`
	Defaults  Defaults   `yaml:"defaults"`
}

// Secrets holds credentials observed in non-file layers. Empty fields mean the
// secret was not supplied via flags/env/.env and must come from the keychain.
type Secrets struct {
	Password string `config:"secret.password"`
	Token    string `config:"secret.token"`
}

// Resolved is the outcome of Load: the merged Config plus provenance and any
// transient secrets.
type Resolved struct {
	Config  Config
	Secrets Secrets
	// Sources maps a field key to the layer name that supplied its final
	// value: "flag", "env", "dotenv", "file", "default".
	Sources map[string]string
	// ActiveContext is the name of the context whose fields were applied.
	// Empty when no config file (or no contexts) exists — pure-env usage.
	ActiveContext string
	// ContextSource records which precedence rule chose ActiveContext (one of
	// the ContextSource* constants), so callers can tell an explicit choice
	// (flag/env) from an implicit fallback (current_context, sole, default).
	ContextSource string
	// ContextNames lists every context defined in the file, in file order.
	ContextNames []string
}

// ContextSelectedExplicitly reports whether the active context was chosen for
// this invocation (via --use-context or OPENOBSERVE_CONTEXT) rather than fallen
// back to from the file's current_context, the sole context, or a "default"
// context. It is the signal used to decide whether to nudge an agent that may
// not realise which of several contexts it is hitting.
func (r *Resolved) ContextSelectedExplicitly() bool {
	return r.ContextSource == ContextSourceFlag || r.ContextSource == ContextSourceEnv
}

// Field keys used for layer maps and provenance tracking.
const (
	fieldCredentialURL = kit.KeyCredentialURL
	fieldServer        = kit.KeyServer
	fieldOrg           = "org"
	fieldAuthScheme    = kit.KeyAuthScheme
	fieldAuthUsername  = "auth.username"
	fieldFormat        = "defaults.format"
	fieldTimeout       = "defaults.timeout"
	fieldMaxRetries    = "defaults.max_retries"
	fieldReadOnly      = "defaults.read_only"
)

// Field key accessors for callers outside this package (e.g. config show).
const (
	FieldServer        = fieldServer
	FieldOrg           = fieldOrg
	FieldCredentialURL = fieldCredentialURL
	FieldAuthScheme    = fieldAuthScheme
	FieldAuthUser      = fieldAuthUsername
	FieldFormat        = fieldFormat
	FieldTimeout       = fieldTimeout
	FieldMaxRetries    = fieldMaxRetries
	FieldReadOnly      = fieldReadOnly
)
