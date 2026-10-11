package config

import (
	"path/filepath"
	"strconv"

	kit "github.com/angelmsger/agent-cli-kit/config"
	"github.com/angelmsger/openobserve-cli/pkg/constants"
	cerrors "github.com/angelmsger/openobserve-cli/pkg/errors"
)

// The schemes a context may store. A session is captured by
// `auth login --browser` and never typed in.
const (
	schemeBasic   = "basic"
	schemeToken   = "token"
	schemeSession = "session"
)

// The registration lives here rather than in internal/config so that a
// program importing only this package (the desktop GUI) reads and writes the
// file exactly as the CLI does.
func init() {
	kit.Register(&kit.Schema{
		// Each row names a field's environment variable and built-in default.
		// Service fields are the team presets a setup load keeps; the email
		// and secrets are personal.
		Fields: []kit.Field{
			{Key: kit.KeyServer, Env: []string{"OPENOBSERVE_URL"}, Service: true},
			{Key: "org", Env: []string{"OPENOBSERVE_ORG"}, Default: constants.DefaultOrg, Service: true},
			{Key: kit.KeyCredentialURL, Env: []string{"OPENOBSERVE_CREDENTIAL_URL"}, Service: true},
			{Key: kit.KeyAuthScheme, Env: []string{"OPENOBSERVE_AUTH_SCHEME"}, Default: schemeBasic, Service: true},
			{Key: "auth.username", Env: []string{"OPENOBSERVE_EMAIL"}},
			{Key: "defaults.format", Env: []string{"OPENOBSERVE_FORMAT"}, Default: constants.DefaultFormat, Service: true},
			{Key: "defaults.timeout", Default: constants.DefaultTimeout.String()},
			{Key: "defaults.max_retries", Default: strconv.Itoa(constants.DefaultMaxRetries)},
			{Key: "defaults.read_only", Env: []string{"OPENOBSERVE_CLI_READ_ONLY"}, Service: true},
			// A secret in the environment selects its scheme; a token wins
			// over a password.
			{Key: "secret.token", Env: []string{"OPENOBSERVE_TOKEN"}, Implies: schemeToken},
			{Key: "secret.password", Env: []string{"OPENOBSERVE_PASSWORD"}, Implies: schemeBasic},
		},
		Paths: kit.Paths{
			Dir:  filepath.Join(constants.ConfigParentDirName, constants.ConfigDirName),
			File: constants.ConfigFileName,
		},
		Words: kit.Words{
			App:          constants.AppName,
			ContextFlag:  "--use-context",
			ContextEnv:   "OPENOBSERVE_CONTEXT",
			NoHintEnv:    "OPENOBSERVE_CLI_NO_CONTEXT_HINT",
			ListContexts: "config get-contexts",
			UseContext:   "config use-context",
			SetContext:   "config set-context",
			AuthGuide:    "auth guide",
			AuthLogin:    "auth login",
		},
		Schemes: []string{schemeToken, schemeBasic, schemeSession},
		// A bare host means a local http:// install, as `config init` assumes.
		URL:       kit.URLRules{AssumeScheme: "http"},
		CheckPlan: checkSessionPlan,
	})
}

// checkSessionPlan refuses a team preset that would repoint a browser-session
// context at another server or scheme: the session can only be captured again
// by signing in through a browser. It runs before the conflict check, so the
// refusal holds even with --overwrite.
func checkSessionPlan(old, next kit.Values, exists bool) error {
	if exists && old[kit.KeyAuthScheme] == schemeSession &&
		(next[kit.KeyAuthScheme] != old[kit.KeyAuthScheme] || next[kit.KeyServer] != old[kit.KeyServer]) {
		return cerrors.New(cerrors.CategoryConfig, "BROWSER_MANAGED_SESSION",
			"service presets cannot replace an existing browser session's service or authentication scheme").
			WithHint("Create a separate context, then sign in using auth login --browser.")
	}
	return nil
}
