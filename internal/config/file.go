package config

import (
	kit "github.com/angelmsger/agent-cli-kit/config"
	pkgcfg "github.com/angelmsger/openobserve-cli/pkg/config"
)

// The persistent config-file model lives in the public pkg/config so the
// desktop GUI can read/write the same file; importing it registers
// openobserve-cli's settings with agent-cli-kit/config. These aliases keep the
// existing internal callers (loader.go, internal/app) compiling unchanged.
type (
	File         = pkgcfg.File
	NamedContext = pkgcfg.NamedContext
	Defaults     = pkgcfg.Defaults
	AuthConfig   = pkgcfg.AuthConfig
)

// NormalizeContextName trims and lower-cases a context name about to be saved.
// Never apply it to a name being looked up: File.Context already ignores case.
func NormalizeContextName(name string) string { return kit.NormalizeContextName(name) }

// ListContextsStep is the command that lists contexts, as a next step.
func ListContextsStep() string { return kit.ListContextsStep() }

// UseContext makes the named context current and returns its stored name, or
// UNKNOWN_CONTEXT when there is no such context.
func UseContext(f *File, name string) (string, error) { return kit.UseContext(f, name) }

// DeleteContext removes the named context and returns it; the first remaining
// context becomes current when it was. It returns UNKNOWN_CONTEXT or
// LAST_CONTEXT instead when the name is unknown or the context is the only one.
func DeleteContext(f *File, name string) (NamedContext, error) { return kit.DeleteContext(f, name) }

func DefaultConfigDir() (string, error)       { return pkgcfg.DefaultConfigDir() }
func ResolveConfigDir() (string, error)       { return pkgcfg.ResolveConfigDir() }
func ConfigFilePath(dir string) string        { return pkgcfg.ConfigFilePath(dir) }
func ReadFile(dir string) (File, bool, error) { return pkgcfg.ReadFile(dir) }
func WriteFile(dir string, f File) error      { return pkgcfg.WriteFile(dir, f) }
