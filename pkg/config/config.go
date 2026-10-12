// Package config is the OpenObserve config-file model shared by the CLI and the
// desktop GUI: the on-disk YAML schema (named contexts + current context +
// shared defaults), file IO, and context helpers. Secrets are never stored
// here; they live in the OS keychain. The CLI's layered loader (flags/env/file)
// builds on this package.
//
// The engine behind it is agent-cli-kit/config. Importing this package
// registers openobserve-cli's settings, file location and command wording with
// it (schema.go), so the CLI and any other caller read and write the file the
// same way. File is a type alias of the kit's File[NamedContext, Defaults]: it
// keeps the fields and the Context, ContextNames, Upsert and Remove methods it
// had, and both names denote one type. The kit holds one registration per
// program, so a program that imports this package must not register another
// CLI's settings with the kit.
package config

import (
	"time"

	kit "github.com/angelmsger/agent-cli-kit/config"
)

// --- on-disk YAML shapes ---

type authShape struct {
	CredentialURL string `yaml:"credential_url,omitempty"`
	Scheme        string `yaml:"scheme,omitempty"`
	Username      string `yaml:"username,omitempty"`
}

type defaultsShape struct {
	Format     string `yaml:"format,omitempty"`
	Timeout    string `yaml:"timeout,omitempty"`
	MaxRetries int    `yaml:"max_retries,omitempty"`
	ReadOnly   bool   `yaml:"read_only,omitempty"`
}

type contextShape struct {
	Name   string    `yaml:"name"`
	Server string    `yaml:"server,omitempty"`
	Org    string    `yaml:"org,omitempty"`
	Auth   authShape `yaml:"auth,omitempty"`
}

// fileShape is the on-disk YAML representation of the config file.
type fileShape struct {
	CurrentContext string         `yaml:"current_context,omitempty"`
	Contexts       []contextShape `yaml:"contexts,omitempty"`
	Defaults       defaultsShape  `yaml:"defaults,omitempty"`
}

// --- public model ---

// AuthConfig is the per-context auth settings persisted to the file.
type AuthConfig struct {
	CredentialURL string `yaml:"credential_url,omitempty" config:"auth.credential_url"`
	Scheme        string `yaml:"scheme" config:"auth.scheme"`
	Username      string `yaml:"username,omitempty" config:"auth.username"`
}

// Defaults are runtime defaults shared across contexts.
type Defaults struct {
	Format     string        `yaml:"format" config:"defaults.format"`
	Timeout    time.Duration `yaml:"timeout" config:"defaults.timeout"`
	MaxRetries int           `yaml:"max_retries" config:"defaults.max_retries"`
	ReadOnly   bool          `yaml:"read_only,omitempty" config:"defaults.read_only"`
}

// NamedContext is one named OpenObserve server profile inside the config file.
type NamedContext struct {
	Name    string `config:"name"`
	BaseURL string `config:"server"`
	Org     string `config:"org"`
	Auth    AuthConfig
}

// File is the parsed config file: named contexts plus shared defaults and the
// name of the current context. Context looks a name up case-insensitively and
// returns the stored spelling; Upsert and Remove match names the same way.
type File = kit.File[NamedContext, Defaults]

// --- paths ---

// DefaultConfigDir returns the per-user config directory
// (~/.angelmsger/openobserve).
func DefaultConfigDir() (string, error) { return kit.DefaultDir() }

// ResolveConfigDir picks the config directory to use when --config was not
// supplied.
func ResolveConfigDir() (string, error) { return kit.ResolveDir() }

// ConfigFilePath returns the YAML config file path inside dir.
func ConfigFilePath(dir string) string { return kit.FilePath(dir) }

// --- file IO ---

// ReadFile reads and parses the config file in dir. The bool return is false
// when the file does not exist.
func ReadFile(dir string) (File, bool, error) {
	var fs fileShape
	if ok, err := kit.ReadFile(dir, &fs); !ok || err != nil {
		return File{}, false, err
	}
	// Missing or unparsable defaults stay zero; the CLI's default layer fills
	// them during a load.
	timeout, _ := time.ParseDuration(fs.Defaults.Timeout)
	f := File{
		CurrentContext: fs.CurrentContext,
		Defaults: Defaults{
			Format:     fs.Defaults.Format,
			Timeout:    timeout,
			MaxRetries: fs.Defaults.MaxRetries,
			ReadOnly:   fs.Defaults.ReadOnly,
		},
	}
	for _, cs := range fs.Contexts {
		f.Contexts = append(f.Contexts, NamedContext{
			Name:    cs.Name,
			BaseURL: cs.Server,
			Org:     cs.Org,
			Auth:    AuthConfig{Scheme: cs.Auth.Scheme, Username: cs.Auth.Username, CredentialURL: cs.Auth.CredentialURL},
		})
	}
	return f, true, nil
}

// WriteFile persists a File to dir/config.yaml, creating dir with 0700
// permissions and replacing the file atomically. Secrets are never written
// here.
func WriteFile(dir string, f File) error {
	fs := fileShape{
		CurrentContext: f.CurrentContext,
		Defaults: defaultsShape{
			Format:     f.Defaults.Format,
			MaxRetries: f.Defaults.MaxRetries,
			ReadOnly:   f.Defaults.ReadOnly,
		},
	}
	if f.Defaults.Timeout > 0 {
		fs.Defaults.Timeout = f.Defaults.Timeout.String()
	}
	for _, c := range f.Contexts {
		fs.Contexts = append(fs.Contexts, contextShape{
			Name:   c.Name,
			Server: c.BaseURL,
			Org:    c.Org,
			Auth:   authShape{Scheme: c.Auth.Scheme, Username: c.Auth.Username, CredentialURL: c.Auth.CredentialURL},
		})
	}
	return kit.WriteFile(dir, &fs)
}
