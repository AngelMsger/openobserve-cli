package config

import (
	pkgcfg "github.com/angelmsger/openobserve-cli/pkg/config"
)

// The persistent config-file model moved to the public pkg/config so the
// desktop GUI can read/write the same file. These aliases keep the existing
// internal callers (loader.go, internal/app) compiling unchanged. The layered
// loader (flags/env/file) stays in this package.
type (
	File         = pkgcfg.File
	NamedContext = pkgcfg.NamedContext
	Defaults     = pkgcfg.Defaults
	AuthConfig   = pkgcfg.AuthConfig
)

func DefaultConfigDir() (string, error)       { return pkgcfg.DefaultConfigDir() }
func ResolveConfigDir() (string, error)       { return pkgcfg.ResolveConfigDir() }
func ConfigFilePath(dir string) string        { return pkgcfg.ConfigFilePath(dir) }
func ReadFile(dir string) (File, bool, error) { return pkgcfg.ReadFile(dir) }
func WriteFile(dir string, f File) error      { return pkgcfg.WriteFile(dir, f) }
