package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/angelmsger/agent-cli-kit/credstore"
	"github.com/angelmsger/openobserve-cli/internal/auth"
	"github.com/angelmsger/openobserve-cli/internal/config"
	cerrors "github.com/angelmsger/openobserve-cli/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/zalando/go-keyring"
)

// The decision itself is agent-cli-kit/reuse's and tested there. These tests
// cover openobserve-cli's wiring: how contexts are described, how the
// destination is rewritten, and the native store, verification and command.

const reuseBase = "https://service.example.test/deploy"

// fakeStore answers lookups from keychain entries keyed like the real store.
type fakeStore map[string]string

func (f fakeStore) lookup(key string) credstore.Lookup {
	if v, ok := f[key]; ok {
		return credstore.Lookup{Secret: v, Keychain: credstore.Hit}
	}
	return credstore.Lookup{Keychain: credstore.Miss, File: credstore.Miss}
}

func (f fakeStore) resolve(cfg config.Config) (auth.Credential, credstore.Lookup, error) {
	l := f.lookup(auth.AccountKey(cfg.BaseURL, cfg.Auth.Scheme))
	if !l.Found() {
		return auth.Credential{}, l, cerrors.New(cerrors.CategoryConfig, "CREDENTIAL_NOT_VISIBLE_OR_MISSING", "missing")
	}
	cred := auth.Credential{Scheme: cfg.Auth.Scheme, Username: cfg.Auth.Username, Secret: l.Secret}
	return cred, l, cred.Validate()
}

func reuseFixture(t *testing.T, contexts ...config.NamedContext) (*appState, fakeStore, reuseServices) {
	t.Helper()
	dir := t.TempDir()
	if len(contexts) == 0 {
		contexts = []config.NamedContext{
			{Name: "personal", BaseURL: reuseBase, Auth: config.AuthConfig{Scheme: "basic", Username: "existing-user"}},
			{Name: "team", BaseURL: reuseBase, Auth: config.AuthConfig{Scheme: "basic"}},
		}
	}
	file := config.File{CurrentContext: contexts[0].Name, Contexts: contexts}
	if err := config.WriteFile(dir, file); err != nil {
		t.Fatal(err)
	}
	target := contexts[len(contexts)-1]
	s := &appState{cfgDir: dir, resolved: &config.Resolved{Config: config.StoredContext(target, file.Defaults), ActiveContext: target.Name}}
	store := fakeStore{auth.AccountKey(reuseBase, "basic"): "test-secret"}
	services := reuseServices{Read: config.ReadFile, Write: config.WriteFile, Resolve: store.resolve, Lookup: store.lookup,
		Verify: func(_ context.Context, cfg config.Config, cred auth.Credential) error {
			if cred.Secret == "test-secret" && cred.Username == "existing-user" {
				return nil
			}
			return cerrors.New(cerrors.CategoryAuth, "HTTP_UNAUTHORIZED", "rejected").WithHTTPStatus(401)
		}}
	return s, store, services
}

func targetOf(t *testing.T, s *appState) config.NamedContext {
	t.Helper()
	file, _, err := config.ReadFile(s.cfgDir)
	if err != nil {
		t.Fatal(err)
	}
	c, _ := file.Context(s.resolved.ActiveContext)
	return c
}

func TestAuthReusePreviewApplyAndIdempotence(t *testing.T) {
	s, _, services := reuseFixture(t)
	before, _, _ := config.ReadFile(s.cfgDir)
	preview, err := reuseAuthentication(s, reuseOptions{DryRun: true}, services)
	if err != nil || !preview.Changed || preview.State != "available" || !preview.Verified || preview.Scheme != "basic" {
		t.Fatalf("preview: %+v %v", preview, err)
	}
	if after, _, _ := config.ReadFile(s.cfgDir); !reflect.DeepEqual(before, after) {
		t.Fatal("preview changed config")
	}
	applied, err := reuseAuthentication(s, reuseOptions{}, services)
	if err != nil || applied.State != "reused" || applied.SourceContext != "personal" || applied.Username != "existing-user" {
		t.Fatalf("apply: %+v %v", applied, err)
	}
	after, _, _ := config.ReadFile(s.cfgDir)
	if got := targetOf(t, s); got.Auth.Username != "existing-user" || after.CurrentContext != before.CurrentContext || !reflect.DeepEqual(after.Contexts[0], before.Contexts[0]) {
		t.Fatalf("did not preserve the source or the current context: %+v", after)
	}
	services.Write = func(string, config.File) error { t.Fatal("repeated write"); return nil }
	repeated, err := reuseAuthentication(s, reuseOptions{}, services)
	if err != nil || repeated.Changed || repeated.State != "unchanged" {
		t.Fatalf("repeat: %+v %v", repeated, err)
	}
	raw, _ := json.Marshal(applied)
	if strings.Contains(string(raw), "test-secret") {
		t.Fatal("secret reached output")
	}
	if len(applied.NextSteps) != 1 || !strings.HasSuffix(applied.NextSteps[0], "auth status") {
		t.Fatalf("next steps: %v", applied.NextSteps)
	}
}

// A browser-session team context adopts the personal basic login on the same
// service once its own session is gone.
func TestAuthReuseSessionContextAdoptsLongLivedLogin(t *testing.T) {
	s, _, services := reuseFixture(t,
		config.NamedContext{Name: "personal", BaseURL: reuseBase, Auth: config.AuthConfig{Scheme: "basic", Username: "existing-user"}},
		config.NamedContext{Name: "browser", BaseURL: reuseBase, Auth: config.AuthConfig{Scheme: "session"}},
		config.NamedContext{Name: "team", BaseURL: reuseBase, Auth: config.AuthConfig{Scheme: "session"}},
	)
	r, err := reuseAuthentication(s, reuseOptions{}, services)
	if err != nil || r.State != "reused" || r.SourceContext != "personal" || r.Scheme != "basic" {
		t.Fatalf("got %+v %v", r, err)
	}
	if got := targetOf(t, s); got.Auth.Scheme != "basic" || got.Auth.Username != "existing-user" {
		t.Fatalf("destination %+v", got)
	}
}

// A session context whose username is only a label adopts the basic secret
// stored for its service, which no context points at any more.
func TestAuthReuseLabelledSessionAdoptsStoredSecret(t *testing.T) {
	s, _, services := reuseFixture(t,
		config.NamedContext{Name: "team", BaseURL: reuseBase, Auth: config.AuthConfig{Scheme: "session", Username: "existing-user"}},
	)
	r, err := reuseAuthentication(s, reuseOptions{}, services)
	if err != nil || r.State != "reused" || r.SourceContext != "" || r.Username != "existing-user" {
		t.Fatalf("got %+v %v", r, err)
	}
	if got := targetOf(t, s); got.Auth.Scheme != "basic" || got.Auth.Username != "existing-user" {
		t.Fatalf("destination %+v", got)
	}
}

// Two session contexts on one key whose shared session is gone used to fail
// the command; now the stored basic secret pairs with the recorded label.
func TestAuthReuseSharedSessionKeyDoesNotFail(t *testing.T) {
	s, _, services := reuseFixture(t,
		config.NamedContext{Name: "test", BaseURL: reuseBase, Auth: config.AuthConfig{Scheme: "session", Username: "existing-user"}},
		config.NamedContext{Name: "dev", BaseURL: reuseBase, Auth: config.AuthConfig{Scheme: "session"}},
	)
	r, err := reuseAuthentication(s, reuseOptions{}, services)
	if err != nil || r.State != "reused" || r.Username != "existing-user" {
		t.Fatalf("got %+v %v", r, err)
	}
}

func TestAuthReuseKeepsAWorkingSessionUnlessSelected(t *testing.T) {
	s, store, services := reuseFixture(t,
		config.NamedContext{Name: "team", BaseURL: reuseBase, Auth: config.AuthConfig{Scheme: "session", Username: "existing-user"}},
	)
	store[auth.AccountKey(reuseBase, "session")] = `{"cookies":"auth_tokens=ok"}`
	verify := services.Verify
	services.Verify = func(ctx context.Context, cfg config.Config, cred auth.Credential) error {
		if cred.Scheme == "session" {
			return nil
		}
		return verify(ctx, cfg, cred)
	}
	r, err := reuseAuthentication(s, reuseOptions{}, services)
	if err != nil || r.State != "unchanged" || !r.Verified {
		t.Fatalf("automatic: %+v %v", r, err)
	}
	r, err = reuseAuthentication(s, reuseOptions{FromScheme: "basic"}, services)
	if err != nil || r.State != "reused" || r.Scheme != "basic" {
		t.Fatalf("explicit: %+v %v", r, err)
	}
}

func TestAuthReuseMatchesCompleteServiceBeforeCredentialAccess(t *testing.T) {
	for _, kind := range []string{"host", "path", "protocol", "port", "org"} {
		t.Run(kind, func(t *testing.T) {
			personal := config.NamedContext{Name: "personal", BaseURL: reuseBase, Auth: config.AuthConfig{Scheme: "basic", Username: "existing-user"}}
			switch kind {
			case "host":
				personal.BaseURL = "https://other.example.test/deploy"
			case "path":
				personal.BaseURL = "https://service.example.test/other"
			case "protocol":
				personal.BaseURL = "http://service.example.test/deploy"
			case "port":
				personal.BaseURL = "https://service.example.test:8443/deploy"
			case "org":
				personal.Org = "other-org"
			}
			s, _, services := reuseFixture(t, personal, config.NamedContext{Name: "team", BaseURL: reuseBase, Auth: config.AuthConfig{Scheme: "basic"}})
			resolve := services.Resolve
			services.Resolve = func(cfg config.Config) (auth.Credential, credstore.Lookup, error) {
				if cfg.Auth.Username == "existing-user" {
					t.Fatal("read another service's credential")
				}
				return resolve(cfg)
			}
			result, err := reuseAuthentication(s, reuseOptions{DryRun: true}, services)
			if err != nil || result.Changed || result.State != "unavailable" {
				t.Fatalf("mismatch: %+v %v", result, err)
			}
			_, err = reuseAuthentication(s, reuseOptions{From: "personal", DryRun: true}, services)
			if err == nil || cerrors.AsCLIError(err).Code != "AUTH_REUSE_SOURCE_MISMATCH" {
				t.Fatalf("explicit mismatch: %v", err)
			}
		})
	}
}

// A scheme the invocation supplies keeps reuse within that scheme.
func TestAuthReuseSuppliedSchemeStaysWithinIt(t *testing.T) {
	s, _, services := reuseFixture(t,
		config.NamedContext{Name: "personal", BaseURL: reuseBase, Auth: config.AuthConfig{Scheme: "basic", Username: "existing-user"}},
		config.NamedContext{Name: "team", BaseURL: reuseBase, Auth: config.AuthConfig{Scheme: "session"}},
	)
	s.resolved.Sources = map[string]string{config.FieldAuthScheme: "flag"}
	r, err := reuseAuthentication(s, reuseOptions{}, services)
	if err != nil || r.State != "unavailable" || r.Changed {
		t.Fatalf("pinned: %+v %v", r, err)
	}
	_, err = reuseAuthentication(s, reuseOptions{From: "personal"}, services)
	if err == nil || cerrors.AsCLIError(err).Code != "AUTH_REUSE_SOURCE_MISMATCH" {
		t.Fatalf("pinned explicit: %v", err)
	}
}

func TestAuthReusePreservesDestinationIdentity(t *testing.T) {
	s, _, services := reuseFixture(t,
		config.NamedContext{Name: "personal", BaseURL: reuseBase, Auth: config.AuthConfig{Scheme: "basic", Username: "existing-user"}},
		config.NamedContext{Name: "team", BaseURL: reuseBase, Auth: config.AuthConfig{Scheme: "basic", Username: "other-user"}},
	)
	services.Resolve = func(config.Config) (auth.Credential, credstore.Lookup, error) {
		t.Fatal("must not replace an existing identity")
		return auth.Credential{}, credstore.Lookup{}, nil
	}
	r, err := reuseAuthentication(s, reuseOptions{}, services)
	if err != nil || r.State != "unchanged" || r.Changed {
		t.Fatalf("existing identity: %+v %v", r, err)
	}
}

func TestAuthReuseAmbiguityAndExplicitSelection(t *testing.T) {
	s, store, services := reuseFixture(t,
		config.NamedContext{Name: "personal", BaseURL: reuseBase, Auth: config.AuthConfig{Scheme: "token"}},
		config.NamedContext{Name: "second", BaseURL: "https://SERVICE.example.test:443/deploy", Auth: config.AuthConfig{Scheme: "token"}},
		config.NamedContext{Name: "team", BaseURL: reuseBase, Auth: config.AuthConfig{Scheme: "session"}},
	)
	store[auth.AccountKey(reuseBase, "token")] = "first"
	store[auth.AccountKey("https://SERVICE.example.test:443/deploy", "token")] = "second"
	delete(store, auth.AccountKey(reuseBase, "basic"))
	services.Verify = func(context.Context, config.Config, auth.Credential) error { return nil }
	_, err := reuseAuthentication(s, reuseOptions{DryRun: true}, services)
	if err == nil || cerrors.AsCLIError(err).Code != "AUTH_REUSE_AMBIGUOUS" {
		t.Fatalf("ambiguity: %v", err)
	}
	r, err := reuseAuthentication(s, reuseOptions{From: "SECOND"}, services)
	if err != nil || r.SourceContext != "second" || r.State != "reused" {
		t.Fatalf("explicit choice: %+v %v", r, err)
	}
	if got := targetOf(t, s); got.BaseURL != "https://SERVICE.example.test:443/deploy" {
		t.Fatalf("destination does not resolve the chosen key: %+v", got)
	}
}

func TestAuthReuseRetainsOperationalFailures(t *testing.T) {
	for _, kind := range []string{"store", "network", "permission"} {
		t.Run(kind, func(t *testing.T) {
			s, _, services := reuseFixture(t)
			before, _, _ := config.ReadFile(s.cfgDir)
			failure := cerrors.New(cerrors.CategoryNetwork, "NETWORK_ERROR", "network unavailable")
			switch kind {
			case "store":
				failure = cerrors.New(cerrors.CategoryConfig, "CREDENTIAL_STORE_INACCESSIBLE", "host keychain unavailable")
				services.Resolve = func(config.Config) (auth.Credential, credstore.Lookup, error) {
					return auth.Credential{}, credstore.Lookup{Keychain: credstore.Failed}, failure
				}
			case "permission":
				failure = cerrors.New(cerrors.CategoryPermission, "FORBIDDEN", "access denied")
			}
			if kind != "store" {
				services.Verify = func(context.Context, config.Config, auth.Credential) error { return failure }
			}
			_, err := reuseAuthentication(s, reuseOptions{}, services)
			if !errors.Is(err, failure) {
				t.Fatalf("lost diagnostic: %v", err)
			}
			if after, _, _ := config.ReadFile(s.cfgDir); !reflect.DeepEqual(before, after) {
				t.Fatal("failure mutated config")
			}
		})
	}
}

func TestAuthReuseConcurrentChangeAndRotation(t *testing.T) {
	s, _, services := reuseFixture(t)
	verify := services.Verify
	services.Verify = func(ctx context.Context, cfg config.Config, cred auth.Credential) error {
		f, _, _ := config.ReadFile(s.cfgDir)
		f.CurrentContext = "changed-by-user"
		if err := config.WriteFile(s.cfgDir, f); err != nil {
			return err
		}
		return verify(ctx, cfg, cred)
	}
	_, err := reuseAuthentication(s, reuseOptions{}, services)
	if err == nil || cerrors.AsCLIError(err).Code != "AUTH_REUSE_CONFIG_CHANGED" {
		t.Fatalf("concurrent change: %v", err)
	}

	s, store, services := reuseFixture(t)
	before, _, _ := config.ReadFile(s.cfgDir)
	resolve := services.Resolve
	calls := 0
	services.Resolve = func(cfg config.Config) (auth.Credential, credstore.Lookup, error) {
		calls++
		if calls > 1 {
			store[auth.AccountKey(reuseBase, "basic")] = "rotated-secret"
		}
		return resolve(cfg)
	}
	_, err = reuseAuthentication(s, reuseOptions{}, services)
	if err == nil || cerrors.AsCLIError(err).Code != "AUTH_REUSE_CREDENTIAL_CHANGED" {
		t.Fatalf("credential rotation: %v", err)
	}
	if after, _, _ := config.ReadFile(s.cfgDir); !reflect.DeepEqual(before, after) {
		t.Fatal("rotation changed destination")
	}
}

func TestAuthReuseRejectsTargetOverridesAndUnknownSource(t *testing.T) {
	s, _, services := reuseFixture(t)
	s.resolved.Config.BaseURL = "https://service.example.test/other"
	_, err := reuseAuthentication(s, reuseOptions{DryRun: true}, services)
	if err == nil || cerrors.AsCLIError(err).Code != "AUTH_REUSE_TARGET_MISMATCH" {
		t.Fatal(err)
	}
	s.resolved.Config.BaseURL = reuseBase
	_, err = reuseAuthentication(s, reuseOptions{From: "missing", DryRun: true}, services)
	if err == nil || cerrors.AsCLIError(err).Code != "AUTH_REUSE_SOURCE_NOT_FOUND" {
		t.Fatal(err)
	}
}

func TestAuthReuseCommandVerifiesNativeStoredCredential(t *testing.T) {
	keyring.MockInit()
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		user, password, ok := r.BasicAuth()
		if r.Method != http.MethodGet || !ok || user != "existing-user" || password != "retained-secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"identifier":"default","name":"Default"}]}`))
	}))
	defer server.Close()
	base := server.URL + "/deploy"
	s, _, _ := reuseFixture(t,
		config.NamedContext{Name: "personal", BaseURL: base, Auth: config.AuthConfig{Scheme: "basic", Username: "existing-user"}},
		config.NamedContext{Name: "team", BaseURL: base, Auth: config.AuthConfig{Scheme: "session"}},
	)
	s.store = auth.NewStore(s.cfgDir)
	credential := auth.Credential{Scheme: "basic", Username: "existing-user", Secret: "retained-secret"}
	if _, err := auth.Save(base, credential, s.store); err != nil {
		t.Fatal(err)
	}
	command := newAuthReuseCmd(s)
	if err := command.RunE(command, nil); err != nil {
		t.Fatal(err)
	}
	if requests == 0 {
		t.Fatal("did not verify with the native API client")
	}
	after, _, _ := config.ReadFile(s.cfgDir)
	target, _ := after.Context("team")
	actual, err := auth.Resolve(config.StoredContext(target, after.Defaults), config.Secrets{}, auth.NewStore(s.cfgDir))
	if err != nil || actual.Username != "existing-user" || actual.Secret != credential.Secret {
		t.Fatalf("native reuse unavailable after reload: %v", err)
	}
}

func TestAuthReusePreservesLegacyStoreKeyUnderEquivalentOverride(t *testing.T) {
	keyring.MockInit()
	legacy := "https://SERVICE.example.test:443/deploy/"
	s, _, services := reuseFixture(t,
		config.NamedContext{Name: "personal", BaseURL: legacy, Auth: config.AuthConfig{Scheme: "basic", Username: "existing-user"}},
		config.NamedContext{Name: "team", BaseURL: reuseBase, Auth: config.AuthConfig{Scheme: "basic"}},
	)
	store := auth.NewStore(s.cfgDir)
	credential := auth.Credential{Scheme: "basic", Username: "existing-user", Secret: "retained-secret"}
	if _, err := auth.Save(legacy, credential, store); err != nil {
		t.Fatal(err)
	}
	services.Resolve = func(cfg config.Config) (auth.Credential, credstore.Lookup, error) {
		return auth.ResolveLookup(cfg, config.Secrets{}, store)
	}
	services.Lookup = store.Lookup
	services.Verify = func(context.Context, config.Config, auth.Credential) error { return nil }
	if _, err := reuseAuthentication(s, reuseOptions{}, services); err != nil {
		t.Fatal(err)
	}
	fresh, err := config.Load(config.LoadOptions{ConfigDir: s.cfgDir, Context: "team", DotenvPath: filepath.Join(t.TempDir(), "absent"), Flags: config.FlagValues{BaseURL: reuseBase}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := auth.Resolve(fresh.Config, config.Secrets{}, auth.NewStore(s.cfgDir))
	if err != nil || got.Secret != credential.Secret || got.Username != credential.Username {
		t.Fatalf("equivalent override lost native credential: %v", err)
	}
	if fresh.Config.BaseURL != reuseBase {
		t.Fatal("changed request destination")
	}
}

// A rejected credential leads with reuse when a stored login could replace it,
// and a session context is offered browser sign-in rather than config init.
func TestRejectedCredentialRecoveryLeadsWithReuse(t *testing.T) {
	s := &appState{resolved: &config.Resolved{Config: config.Config{BaseURL: reuseBase,
		Auth: config.AuthConfig{Scheme: "session"}, ReuseStep: "openobserve-cli --use-context 'team' auth reuse"}}}
	ce := cerrors.New(cerrors.CategoryAuth, "HTTP_UNAUTHORIZED", "rejected").WithHTTPStatus(401)
	withReuseRecovery(ce, s, &cobra.Command{Use: "search"})
	if len(ce.NextSteps) < 2 || ce.NextSteps[0] != s.cfg().ReuseStep {
		t.Fatalf("next steps %v", ce.NextSteps)
	}
	for _, step := range ce.NextSteps {
		if strings.HasSuffix(step, "config init") {
			t.Fatalf("session context offered config init: %v", ce.NextSteps)
		}
	}
	forbidden := cerrors.New(cerrors.CategoryPermission, "FORBIDDEN", "denied").WithHTTPStatus(403)
	steps := append([]string(nil), forbidden.NextSteps...)
	withReuseRecovery(forbidden, s, &cobra.Command{Use: "search"})
	if !reflect.DeepEqual(steps, forbidden.NextSteps) {
		t.Fatal("a permission error gained a reuse step")
	}
}
