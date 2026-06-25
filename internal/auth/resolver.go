package auth

import (
	"errors"

	"github.com/angelmsger/openobserve-cli/internal/config"
	cerrors "github.com/angelmsger/openobserve-cli/pkg/errors"
)

// Resolve produces a Credential from configuration. A secret supplied via
// flags/env/.env (carried in secrets) takes precedence; otherwise the secret
// is loaded from the Store. The returned credential is validated.
func Resolve(cfg config.Config, secrets config.Secrets, store *Store) (Credential, error) {
	scheme := cfg.Auth.Scheme
	if scheme == "" {
		scheme = SchemeBasic
	}
	cred := Credential{Scheme: scheme, Username: cfg.Auth.Username}

	switch scheme {
	case SchemeBasic:
		cred.Secret = secrets.Password
	case SchemeToken:
		cred.Secret = secrets.Token
	}

	if cred.Secret == "" && store != nil && cfg.BaseURL != "" {
		loaded, err := store.Load(AccountKey(cfg.BaseURL, scheme))
		if err != nil && !errors.Is(err, ErrSecretNotFound) {
			return Credential{}, cerrors.Wrap(err, cerrors.CategoryConfig,
				"AUTH_STORE_READ", "failed to read stored credentials")
		}
		cred.Secret = loaded
	}

	if err := cred.Validate(); err != nil {
		return Credential{}, err
	}
	return cred, nil
}

// Save stores a credential's secret for later resolution and returns the
// backend ("keychain" or "file") that accepted it.
func Save(baseURL string, cred Credential, store *Store) (string, error) {
	if err := cred.Validate(); err != nil {
		return "", err
	}
	return store.Save(AccountKey(baseURL, cred.Scheme), cred.Secret)
}

// Forget removes any stored secret for the base URL and scheme.
func Forget(baseURL, scheme string, store *Store) error {
	return store.Delete(AccountKey(baseURL, scheme))
}
