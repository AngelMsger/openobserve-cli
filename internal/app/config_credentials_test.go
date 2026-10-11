package app

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/angelmsger/openobserve-cli/internal/auth"
	"github.com/angelmsger/openobserve-cli/internal/config"
	"github.com/zalando/go-keyring"
)

// A stored secret is keyed by the server's host and the scheme, so every
// context on one server shares it. Two defects found in a sibling's setup code
// deleted such a secret: re-running the wizard across a spelling-only change of
// the server URL removed the credential it had just saved, and removing one
// context removed the secret another still used. `config init` here only ever
// saves. `config delete-context` removes a secret only when no remaining
// context resolves it, and only after the config write. These tests pin that.

func credentialServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"identifier":"default","name":"Default"}]}`))
	}))
	t.Cleanup(server.Close)
	return server
}

// runConfigInit drives the plain `config init` wizard over a non-terminal
// stdin, the way a piped setup does, against a mocked keychain.
func runConfigInit(t *testing.T, cfgDir, contextName string, answers ...string) {
	t.Helper()
	for _, name := range []string{
		"OPENOBSERVE_URL", "OPENOBSERVE_ORG", "OPENOBSERVE_AUTH_SCHEME", "OPENOBSERVE_CREDENTIAL_URL",
		"OPENOBSERVE_EMAIL", "OPENOBSERVE_PASSWORD", "OPENOBSERVE_TOKEN", "OPENOBSERVE_CONTEXT",
		"OPENOBSERVE_FORMAT", "OPENOBSERVE_CLI_READ_ONLY",
	} {
		t.Setenv(name, "")
	}
	t.Setenv("OPENOBSERVE_CLI_NO_SKILL_HINT", "1")
	t.Setenv("OPENOBSERVE_CLI_NO_UPDATE_NOTIFIER", "1")
	scratch := t.TempDir()
	input, err := os.Create(filepath.Join(scratch, "stdin"))
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	if _, err := input.WriteString(strings.Join(answers, "\n") + "\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := input.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	sink, err := os.Create(filepath.Join(scratch, "output"))
	if err != nil {
		t.Fatal(err)
	}
	defer sink.Close()
	oldIn, oldOut, oldErr := os.Stdin, os.Stdout, os.Stderr
	os.Stdin, os.Stdout, os.Stderr = input, sink, sink
	defer func() { os.Stdin, os.Stdout, os.Stderr = oldIn, oldOut, oldErr }()
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"--config", cfgDir, "config", "init", "--context", contextName})
	cmd.SetOut(sink)
	cmd.SetErr(sink)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("config init --context %s: %v", contextName, err)
	}
}

// resolveStoredSecret loads a context the way a fresh invocation does and
// returns the secret it resolves from the credential store.
func resolveStoredSecret(t *testing.T, cfgDir, contextName string) (string, error) {
	t.Helper()
	file, _, err := config.ReadFile(cfgDir)
	if err != nil {
		t.Fatal(err)
	}
	nc, ok := file.Context(contextName)
	if !ok {
		t.Fatalf("context %q is missing: %+v", contextName, file)
	}
	cred, err := auth.Resolve(config.StoredContext(nc, file.Defaults), config.Secrets{}, auth.NewStore(cfgDir))
	return cred.Secret, err
}

// The wizard default, a preset and a pasted address can spell one server
// differently. Editing a context across such a difference must leave the
// credential that the edit has just saved resolvable.
func TestConfigInitKeepsTheCredentialWhenOnlyTheURLSpellingChanges(t *testing.T) {
	keyring.MockInit()
	server := credentialServer(t)
	host := strings.TrimPrefix(server.URL, "http://")
	for _, spelling := range []string{
		server.URL + "/",  // trailing slash
		server.URL + "//", // doubled slash
		host,              // bare host, no scheme
		"HTTP://" + host,  // scheme case
	} {
		t.Run(spelling, func(t *testing.T) {
			dir := t.TempDir()
			runConfigInit(t, dir, "default", server.URL, "default", "basic", "member@example.test", "first-password")
			if got, err := resolveStoredSecret(t, dir, "default"); err != nil || got != "first-password" {
				t.Fatalf("initial setup did not store a resolvable credential: %q %v", got, err)
			}
			runConfigInit(t, dir, "default", spelling, "default", "basic", "member@example.test", "second-password")
			if got, err := resolveStoredSecret(t, dir, "default"); err != nil || got != "second-password" {
				t.Fatalf("the wizard lost the credential it had just saved: %q %v", got, err)
			}
		})
	}
}

// Moving one context to another server must not remove the secret that a
// second context on the old server still resolves.
func TestConfigInitKeepsACredentialAnotherContextUses(t *testing.T) {
	keyring.MockInit()
	shared, elsewhere := credentialServer(t), credentialServer(t)
	dir := t.TempDir()
	runConfigInit(t, dir, "default", shared.URL, "default", "basic", "member@example.test", "shared-password")
	// A second context on the same server, as a team preset would add it.
	file, _, err := config.ReadFile(dir)
	if err != nil {
		t.Fatal(err)
	}
	file.Upsert(config.NamedContext{Name: "team", BaseURL: shared.URL + "/", Org: "team-a", Auth: config.AuthConfig{Scheme: "basic", Username: "member@example.test"}})
	if err := config.WriteFile(dir, file); err != nil {
		t.Fatal(err)
	}
	if got, err := resolveStoredSecret(t, dir, "team"); err != nil || got != "shared-password" {
		t.Fatalf("contexts on one server do not share its credential: %q %v", got, err)
	}

	runConfigInit(t, dir, "default", elsewhere.URL, "default", "basic", "member@example.test", "new-password")
	if got, err := resolveStoredSecret(t, dir, "default"); err != nil || got != "new-password" {
		t.Fatalf("the moved context cannot resolve its new credential: %q %v", got, err)
	}
	if got, err := resolveStoredSecret(t, dir, "team"); err != nil || got != "shared-password" {
		t.Fatalf("moving one context removed the credential another still uses: %q %v", got, err)
	}
}

// storedSecret reads the secret stored for a server and scheme.
func storedSecret(t *testing.T, cfgDir, server, scheme string) (string, error) {
	t.Helper()
	return auth.NewStore(cfgDir).Load(auth.AccountKey(server, scheme))
}

// saveSecret stores a secret the way login does, without validating it, so a
// browser-session envelope can be stood in for by any string.
func saveSecret(t *testing.T, cfgDir, server, scheme, secret string) {
	t.Helper()
	if _, err := auth.NewStore(cfgDir).Save(auth.AccountKey(server, scheme), secret); err != nil {
		t.Fatal(err)
	}
}

// Deleting a context removes its secret only when no remaining context
// resolves the same account: another spelling of the server, another
// organization on it, or a preset with no scheme of its own (which resolves to
// basic) keeps it.
func TestDeleteContextKeepsACredentialAnotherContextUses(t *testing.T) {
	cfgDir := isolateConfig(t)
	other := basicContext("other-org", "https://o2.example.test/")
	other.Org = "team-a"
	writeContexts(t, cfgDir, "default",
		basicContext("default", "https://o2.example.test/"),
		config.NamedContext{Name: "team", BaseURL: "https://o2.example.test"},
		other,
		basicContext("elsewhere", "https://elsewhere.example.test/o2"))
	saveSecret(t, cfgDir, "https://o2.example.test/", auth.SchemeBasic, "shared-password")
	saveSecret(t, cfgDir, "https://elsewhere.example.test/o2", auth.SchemeBasic, "elsewhere-password")

	for _, name := range []string{"team", "default"} {
		if _, err := runConfig(t, cfgDir, "config", "delete-context", name); err != nil {
			t.Fatal(err)
		}
		if got, err := storedSecret(t, cfgDir, "https://o2.example.test", auth.SchemeBasic); err != nil || got != "shared-password" {
			t.Fatalf("deleting %s removed the credential another context uses: %q %v", name, got, err)
		}
	}
	if _, err := runConfig(t, cfgDir, "config", "delete-context", "elsewhere"); err != nil {
		t.Fatal(err)
	}
	if got, err := storedSecret(t, cfgDir, "https://elsewhere.example.test/o2", auth.SchemeBasic); !errors.Is(err, auth.ErrSecretNotFound) {
		t.Fatalf("an unused credential was left behind: %q %v", got, err)
	}
	if file, _, _ := config.ReadFile(cfgDir); len(file.Contexts) != 1 || file.Contexts[0].Name != "other-org" {
		t.Fatalf("unexpected contexts: %+v", file)
	}
}

// A browser session is stored under the session scheme. Deleting a session
// context keeps the session while another session context on the server
// resolves it, removes it with the last one, and never touches a password
// stored for the same server.
func TestDeleteContextHandlesBrowserSessions(t *testing.T) {
	cfgDir := isolateConfig(t)
	session := func(name, org string) config.NamedContext {
		return config.NamedContext{Name: name, BaseURL: "https://o2.example.test", Org: org,
			Auth: config.AuthConfig{Scheme: auth.SchemeSession, Username: "alice@example.test"}}
	}
	writeContexts(t, cfgDir, "sso", session("sso", "default"), session("sso-team", "team-a"),
		basicContext("password", "https://o2.example.test"))
	saveSecret(t, cfgDir, "https://o2.example.test", auth.SchemeSession, "session-envelope")
	saveSecret(t, cfgDir, "https://o2.example.test", auth.SchemeBasic, "the-password")

	if _, err := runConfig(t, cfgDir, "config", "delete-context", "sso"); err != nil {
		t.Fatal(err)
	}
	if got, err := storedSecret(t, cfgDir, "https://o2.example.test", auth.SchemeSession); err != nil || got != "session-envelope" {
		t.Fatalf("deleting one session context removed the session another uses: %q %v", got, err)
	}
	if _, err := runConfig(t, cfgDir, "config", "delete-context", "sso-team"); err != nil {
		t.Fatal(err)
	}
	if got, err := storedSecret(t, cfgDir, "https://o2.example.test", auth.SchemeSession); !errors.Is(err, auth.ErrSecretNotFound) {
		t.Fatalf("the last session context left its session behind: %q %v", got, err)
	}
	if got, err := storedSecret(t, cfgDir, "https://o2.example.test", auth.SchemeBasic); err != nil || got != "the-password" {
		t.Fatalf("deleting a session context removed a password: %q %v", got, err)
	}

	// The other way round: deleting the password context keeps a session.
	writeContexts(t, cfgDir, "sso", session("sso", "default"), basicContext("password", "https://o2.example.test"))
	saveSecret(t, cfgDir, "https://o2.example.test", auth.SchemeSession, "session-envelope")
	if _, err := runConfig(t, cfgDir, "config", "delete-context", "password"); err != nil {
		t.Fatal(err)
	}
	if got, err := storedSecret(t, cfgDir, "https://o2.example.test", auth.SchemeSession); err != nil || got != "session-envelope" {
		t.Fatalf("deleting a password context removed a session: %q %v", got, err)
	}
	if got, err := storedSecret(t, cfgDir, "https://o2.example.test", auth.SchemeBasic); !errors.Is(err, auth.ErrSecretNotFound) {
		t.Fatalf("the deleted context's password was left behind: %q %v", got, err)
	}
}

// A context on the same server under the other scheme keeps its own secret
// and does not keep the deleted context's secret alive.
func TestDeleteContextScopesTheSharedCredentialByScheme(t *testing.T) {
	cfgDir := isolateConfig(t)
	token := config.NamedContext{Name: "token", BaseURL: "https://o2.example.test", Auth: config.AuthConfig{Scheme: auth.SchemeToken}}
	writeContexts(t, cfgDir, "basic", basicContext("basic", "https://o2.example.test"), token)
	saveSecret(t, cfgDir, token.BaseURL, auth.SchemeToken, "the-token")
	saveSecret(t, cfgDir, token.BaseURL, auth.SchemeBasic, "the-password")

	if _, err := runConfig(t, cfgDir, "config", "delete-context", "token"); err != nil {
		t.Fatal(err)
	}
	if got, err := storedSecret(t, cfgDir, token.BaseURL, auth.SchemeToken); !errors.Is(err, auth.ErrSecretNotFound) {
		t.Fatalf("the deleted context's secret was left behind: %q %v", got, err)
	}
	if got, err := storedSecret(t, cfgDir, token.BaseURL, auth.SchemeBasic); err != nil || got != "the-password" {
		t.Fatalf("a secret under the other scheme was deleted: %q %v", got, err)
	}
}

// A failed config write must not cost the credential: the context is still in
// the file and still resolves it.
func TestDeleteContextKeepsTheCredentialWhenTheConfigWriteFails(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("a read-only directory does not block file creation for this user")
	}
	cfgDir := isolateConfig(t)
	writeContexts(t, cfgDir, "default", basicContext("default", "https://o2.example.test"), basicContext("target", "https://target.example.test"))
	saveSecret(t, cfgDir, "https://target.example.test", auth.SchemeBasic, "target-password")
	// The config is replaced through a temporary file in the same directory,
	// so a read-only directory fails the write while the read still succeeds.
	if err := os.Chmod(cfgDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(cfgDir, 0o700) })
	if _, err := runConfig(t, cfgDir, "config", "delete-context", "target"); err == nil {
		t.Fatal("a failed config write was reported as success")
	}
	if got, err := storedSecret(t, cfgDir, "https://target.example.test", auth.SchemeBasic); err != nil || got != "target-password" {
		t.Fatalf("a failed delete removed a credential the context still names: %q %v", got, err)
	}
}
