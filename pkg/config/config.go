// Package config is the OpenObserve config-file model shared by the CLI and the
// desktop GUI: the on-disk YAML schema (named contexts + current context +
// shared defaults), file IO, and context helpers. Secrets are never stored
// here; they live in the OS keychain. The CLI's layered loader (flags/env/file)
// builds on this package.
package config

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/angelmsger/openobserve-cli/pkg/constants"
	"gopkg.in/yaml.v3"
)

// --- on-disk YAML shapes ---

type authShape struct {
	Scheme   string `yaml:"scheme,omitempty"`
	Username string `yaml:"username,omitempty"`
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
	Scheme   string `yaml:"scheme"`
	Username string `yaml:"username,omitempty"`
}

// Defaults are runtime defaults shared across contexts.
type Defaults struct {
	Format     string        `yaml:"format"`
	Timeout    time.Duration `yaml:"timeout"`
	MaxRetries int           `yaml:"max_retries"`
	ReadOnly   bool          `yaml:"read_only,omitempty"`
}

// NamedContext is one named OpenObserve server profile inside the config file.
type NamedContext struct {
	Name    string
	BaseURL string
	Org     string
	Auth    AuthConfig
}

// File is the parsed config file: named contexts plus shared defaults and the
// name of the current context.
type File struct {
	CurrentContext string
	Contexts       []NamedContext
	Defaults       Defaults
}

// Context returns the context whose name matches, case-insensitively.
func (f File) Context(name string) (NamedContext, bool) {
	for _, c := range f.Contexts {
		if strings.EqualFold(c.Name, name) {
			return c, true
		}
	}
	return NamedContext{}, false
}

// ContextNames returns every context name, in file order.
func (f File) ContextNames() []string {
	names := make([]string, len(f.Contexts))
	for i, c := range f.Contexts {
		names[i] = c.Name
	}
	return names
}

// Upsert inserts or replaces a context by case-insensitive name, preserving
// file order for existing entries.
func (f *File) Upsert(nc NamedContext) {
	for i, c := range f.Contexts {
		if strings.EqualFold(c.Name, nc.Name) {
			f.Contexts[i] = nc
			return
		}
	}
	f.Contexts = append(f.Contexts, nc)
}

// Remove deletes the context with the given (case-insensitive) name, preserving
// order. It reports whether a context was removed.
func (f *File) Remove(name string) bool {
	for i, c := range f.Contexts {
		if strings.EqualFold(c.Name, name) {
			f.Contexts = append(f.Contexts[:i], f.Contexts[i+1:]...)
			return true
		}
	}
	return false
}

// --- paths ---

// DefaultConfigDir returns the per-user config directory
// (~/.angelmsger/openobserve).
func DefaultConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, constants.ConfigParentDirName, constants.ConfigDirName), nil
}

// ResolveConfigDir picks the config directory to use when --config was not
// supplied.
func ResolveConfigDir() (string, error) {
	return DefaultConfigDir()
}

// ConfigFilePath returns the YAML config file path inside dir.
func ConfigFilePath(dir string) string {
	return filepath.Join(dir, constants.ConfigFileName)
}

// --- file IO ---

// ReadFile reads and parses the config file in dir. The bool return is false
// when the file does not exist.
func ReadFile(dir string) (File, bool, error) {
	raw, err := os.ReadFile(ConfigFilePath(dir))
	if err != nil {
		if os.IsNotExist(err) {
			return File{}, false, nil
		}
		return File{}, false, err
	}
	var fs fileShape
	if err := yaml.Unmarshal(raw, &fs); err != nil {
		return File{}, false, err
	}
	f := File{
		CurrentContext: fs.CurrentContext,
		Defaults:       defaultsFromShape(fs.Defaults),
	}
	for _, cs := range fs.Contexts {
		f.Contexts = append(f.Contexts, NamedContext{
			Name:    cs.Name,
			BaseURL: cs.Server,
			Org:     cs.Org,
			Auth:    AuthConfig{Scheme: cs.Auth.Scheme, Username: cs.Auth.Username},
		})
	}
	return f, true, nil
}

// WriteFile persists a File to dir/config.yaml, creating dir with 0700
// permissions. Secrets are never written here.
func WriteFile(dir string, f File) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	var fs fileShape
	fs.CurrentContext = f.CurrentContext
	for _, c := range f.Contexts {
		fs.Contexts = append(fs.Contexts, contextShape{
			Name:   c.Name,
			Server: c.BaseURL,
			Org:    c.Org,
			Auth:   authShape{Scheme: c.Auth.Scheme, Username: c.Auth.Username},
		})
	}
	fs.Defaults.Format = f.Defaults.Format
	if f.Defaults.Timeout > 0 {
		fs.Defaults.Timeout = f.Defaults.Timeout.String()
	}
	fs.Defaults.MaxRetries = f.Defaults.MaxRetries
	fs.Defaults.ReadOnly = f.Defaults.ReadOnly

	out, err := yaml.Marshal(&fs)
	if err != nil {
		return err
	}
	return os.WriteFile(ConfigFilePath(dir), out, 0o600)
}

func defaultsFromShape(ds defaultsShape) Defaults {
	return Defaults{
		Format:     ds.Format,
		Timeout:    durationOr(ds.Timeout, 0),
		MaxRetries: ds.MaxRetries,
		ReadOnly:   ds.ReadOnly,
	}
}

func durationOr(s string, fallback time.Duration) time.Duration {
	if s == "" {
		return fallback
	}
	if d, err := time.ParseDuration(s); err == nil {
		return d
	}
	return fallback
}
