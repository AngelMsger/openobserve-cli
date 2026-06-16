// Package constants holds project-wide constants and build-time metadata.
package constants

import "time"

// Build-time metadata, injected via -ldflags. See Makefile.
var (
	Version   = "dev"
	Commit    = "none"
	BuildTime = "unknown"
)

const (
	// AppName is the binary / command name.
	AppName = "openobserve-cli"

	// EnvPrefix is the environment variable prefix for all settings.
	EnvPrefix = "OPENOBSERVE_"

	// ConfigParentDirName groups every angelmsger CLI's per-user config under
	// one shared $HOME-relative directory (~/.angelmsger).
	ConfigParentDirName = ".angelmsger"

	// ConfigDirName is the per-CLI config directory under ConfigParentDirName,
	// i.e. ~/.angelmsger/openobserve.
	ConfigDirName = "openobserve"

	// ConfigFileName is the YAML config file within ConfigDirName.
	ConfigFileName = "config.yaml"

	// CredentialsFileName is the fallback secret store when no keychain is available.
	CredentialsFileName = "credentials"

	// KeychainService is the service name used for OS keychain entries.
	KeychainService = "openobserve-cli"
)

// Defaults for runtime behaviour.
const (
	DefaultFormat     = "json"
	DefaultTimeout    = 30 * time.Second
	DefaultMaxRetries = 3

	// DefaultOrg is OpenObserve's built-in organization identifier.
	DefaultOrg = "default"

	// DefaultSearchLimit is the number of hits returned by `search run` when
	// --limit is not given. Kept modest so a stray query does not flood an
	// agent's context window.
	DefaultSearchLimit = 100

	// MaxSearchLimit caps a single search request's size.
	MaxSearchLimit = 10000

	// SelfHostedBaseURL is the default URL of a local OpenObserve install.
	SelfHostedBaseURL = "http://localhost:5080"

	// CloudBaseURL is the OpenObserve Cloud API root.
	CloudBaseURL = "https://api.openobserve.ai"
)

// UserAgent identifies the CLI to the OpenObserve server.
func UserAgent() string {
	return AppName + "/" + Version
}
