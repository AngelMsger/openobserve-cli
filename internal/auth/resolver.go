package auth

import (
	"errors"

	"github.com/angelmsger/openobserve-cli/internal/config"
	cerrors "github.com/angelmsger/openobserve-cli/pkg/errors"
)

// Resolve produces a Credential from configuration. A secret supplied via
// flags/env/.env (carried in secrets) takes precedence; otherwise the secret
// is loaded from the Store. The returned credential is validated.
func Resolve(cfg config.Config, secrets config.Secrets, store *Store) (credResult Credential, resultErr error) {
	defer func() {
		if resultErr != nil {
			resultErr = config.WithCredentialGuide(resultErr, cfg)
		}
	}()
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
		lookupBase, err := CredentialLookupURL(cfg)
		if err != nil {
			return Credential{}, err
		}
		loaded, err := store.Load(AccountKey(lookupBase, scheme))
		switch {
		case err == nil:
			cred.Secret = loaded
		case errors.Is(err, ErrSecretNotFound):
			return Credential{}, credentialNotVisibleOrMissingError(scheme)
		default:
			return Credential{}, credentialStoreInaccessibleError(err)
		}
	}

	if err := cred.Validate(); err != nil {
		return Credential{}, err
	}
	return cred, nil
}

func credentialNotVisibleOrMissingError(scheme string) error {
	lastStep := "Only if the host retry also reports missing credentials, run `openobserve-cli config init` in the user's terminal or set OPENOBSERVE_* environment variables."
	if scheme == SchemeSession {
		lastStep = "Only if the host retry also reports a missing browser session, run `openobserve-cli auth login --browser` in the user's graphical session (or sign in through o3 — both write the same keychain entry); browser sessions are not created by `config init`."
	}
	return cerrors.New(cerrors.CategoryConfig, "CREDENTIAL_NOT_VISIBLE_OR_MISSING",
		"stored OpenObserve credentials are missing or not visible in this execution environment").
		WithHint("An agent sandbox may have a different home or keychain view even when the host is already configured.").
		WithNextSteps(
			"Retry the same command with access to the host user environment (home directory and OS keychain).",
			"openobserve-cli doctor",
			lastStep).
		WithRecovery(hostCredentialRecovery())
}

func credentialStoreInaccessibleError(err error) error {
	return cerrors.Wrap(err, cerrors.CategoryConfig, "CREDENTIAL_STORE_INACCESSIBLE",
		"stored OpenObserve credentials cannot be read in this execution environment").
		WithHint("The configured credential store is inaccessible; this commonly happens when an agent sandbox cannot access the host keychain or credential file.").
		WithNextSteps(
			"Retry the same command with access to the host user environment (home directory and OS keychain).",
			"openobserve-cli doctor",
			"Do not run `config init` unless the same check also fails in the host environment.").
		WithRecovery(hostCredentialRecovery())
}

func hostCredentialRecovery() cerrors.Recovery {
	return cerrors.Recovery{
		Action:   "retry_current_command",
		Scope:    "host",
		Requires: []string{"user_home", "os_keychain"},
	}
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

// CredentialLookupURL retains legacy store addressing only for the same complete service.
func CredentialLookupURL(cfg config.Config) (string, error) {
	if cfg.CredentialBaseURL == "" {
		return cfg.BaseURL, nil
	}
	target, e1 := config.NormalizeServiceURL(cfg.BaseURL)
	stored, e2 := config.NormalizeServiceURL(cfg.CredentialBaseURL)
	if e1 != nil || e2 != nil || target != stored {
		return "", cerrors.New(cerrors.CategoryConfig, "CREDENTIAL_SERVICE_MISMATCH", "stored credential lookup does not match the complete service URL")
	}
	return cfg.CredentialBaseURL, nil
}

// ForgetForConfig removes the same entry that configured requests resolve.
func ForgetForConfig(cfg config.Config, scheme string, store *Store) error {
	base, err := CredentialLookupURL(cfg)
	if err != nil {
		return err
	}
	return Forget(base, scheme, store)
}
