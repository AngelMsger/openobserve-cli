package app

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
// saves, and no command removes a context, so neither defect exists; these
// tests pin that, so a cleanup step added later has to keep credentials that
// are still in use.

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
