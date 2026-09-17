package config

import (
	"os"

	"github.com/joho/godotenv"
)

// envBindings maps environment variable names to layer field keys.
var envBindings = map[string]string{
	"OPENOBSERVE_AUTH_SCHEME":    fieldAuthScheme,
	"OPENOBSERVE_CREDENTIAL_URL": fieldCredentialURL,
	"OPENOBSERVE_URL":            fieldServer,
	"OPENOBSERVE_ORG":            fieldOrg,
	"OPENOBSERVE_EMAIL":          fieldAuthUsername,
	"OPENOBSERVE_FORMAT":         fieldFormat,
	"OPENOBSERVE_PASSWORD":       fieldPassword,
	"OPENOBSERVE_TOKEN":          fieldToken,
	"OPENOBSERVE_CLI_READ_ONLY":  fieldReadOnly,
}

// layerFromVars converts a name->value map into a layer map. Empty values are
// skipped so they do not shadow lower-precedence layers.
func layerFromVars(vars map[string]string, serviceOnly ...bool) map[string]string {
	m := map[string]string{}
	for name, field := range envBindings {
		if v := vars[name]; v != "" {
			if len(serviceOnly) > 0 && serviceOnly[0] && !serviceField(field) {
				continue
			}
			m[field] = v
		}
	}
	// Infer the auth scheme from whichever secret is present, so the user need
	// not also set the scheme explicitly. A token wins over a password.
	if _, ok := m[fieldToken]; ok {
		m[fieldAuthScheme] = SchemeToken
	} else if _, ok := m[fieldPassword]; ok {
		m[fieldAuthScheme] = SchemeBasic
	}
	if v := vars["OPENOBSERVE_AUTH_SCHEME"]; v != "" {
		m[fieldAuthScheme] = v
	}
	return m
}

// envLayer reads configuration from the process environment.
func envLayer(serviceOnly ...bool) map[string]string {
	vars := map[string]string{}
	for name := range envBindings {
		if v, ok := os.LookupEnv(name); ok {
			vars[name] = v
		}
	}
	return layerFromVars(vars, serviceOnly...)
}

// dotenvLayer reads configuration from a .env file without mutating the
// process environment. A missing file yields an empty layer.
func dotenvLayer(path string, serviceOnly ...bool) (map[string]string, error) {
	if path == "" {
		return map[string]string{}, nil
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, err
	}
	vars, err := godotenv.Read(path)
	if err != nil {
		return nil, err
	}
	return layerFromVars(vars, serviceOnly...), nil
}
