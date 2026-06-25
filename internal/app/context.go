package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/angelmsger/openobserve-cli/internal/auth"
	"github.com/angelmsger/openobserve-cli/internal/config"
	"github.com/angelmsger/openobserve-cli/internal/output"
	"github.com/angelmsger/openobserve-cli/pkg/apiclient"
	cerrors "github.com/angelmsger/openobserve-cli/pkg/errors"
	"github.com/angelmsger/openobserve-cli/pkg/transport"
)

// globalFlags holds the persistent flags shared by every command.
type globalFlags struct {
	baseURL    string
	org        string
	format     string
	fields     string
	timeout    string
	configPath string
	useContext string
	verbose    bool
	// pretty opts a human user into TUI prompts (in `config init`) and
	// ANSI-colored JSON. Off by default so agent / scripted / pipe usage stays
	// byte-identical.
	pretty bool
	// allowWrites overrides read-only mode for the current invocation.
	allowWrites bool
}

// appState is the shared runtime context, built once in the root command's
// PersistentPreRunE and captured by every subcommand handler.
type appState struct {
	gflags   globalFlags
	resolved *config.Resolved
	store    *auth.Store
	cfgDir   string
}

// load resolves configuration from all sources using the current global flags.
func (s *appState) load() error {
	cfgDir := s.gflags.configPath
	if cfgDir == "" {
		d, err := config.ResolveConfigDir()
		if err != nil {
			return cerrors.Wrap(err, cerrors.CategoryConfig, "NO_HOME",
				"could not determine the home directory")
		}
		cfgDir = d
	}
	resolved, err := config.Load(config.LoadOptions{
		ConfigDir: cfgDir,
		Context:   s.gflags.useContext,
		Flags: config.FlagValues{
			BaseURL: s.gflags.baseURL,
			Org:     s.gflags.org,
			Format:  s.gflags.format,
			Timeout: s.gflags.timeout,
		},
	})
	if err != nil {
		// Pass structured CLI errors (e.g. UNKNOWN_CONTEXT) through untouched.
		var ce *cerrors.CLIError
		if errors.As(err, &ce) {
			return ce
		}
		return cerrors.Wrap(err, cerrors.CategoryConfig, "CONFIG_LOAD",
			"failed to load configuration")
	}
	s.resolved = resolved
	s.cfgDir = cfgDir
	s.store = auth.NewStore(cfgDir)
	return nil
}

// cfg returns the resolved config.
func (s *appState) cfg() config.Config { return s.resolved.Config }

// org returns the effective organization identifier for this invocation.
func (s *appState) org() string {
	if o := s.cfg().Org; o != "" {
		return o
	}
	return "default"
}

// newClient resolves credentials and builds an authenticated API client.
func (s *appState) newClient() (apiclient.Client, error) {
	cfg := s.cfg()
	cred, err := auth.Resolve(cfg, s.resolved.Secrets, s.store)
	if err != nil {
		return nil, err
	}
	client, err := apiclient.Build(apiclient.BuildParams{
		BaseURL:       cfg.BaseURL,
		Org:           s.org(),
		AuthDecorator: cred.Decorator(),
		Timeout:       cfg.Defaults.Timeout,
		MaxRetries:    cfg.Defaults.MaxRetries,
	})
	if err != nil {
		return nil, err
	}
	if s.readOnly() {
		client = apiclient.NewReadOnly(client)
	}
	return client, nil
}

// probeTransport builds an unauthenticated-or-authenticated transport for
// connectivity checks where a full client is unnecessary.
func (s *appState) probeTransport(dec transport.Decorator) *transport.Client {
	decs := []transport.Decorator{}
	if dec != nil {
		decs = append(decs, dec)
	}
	if s.gflags.verbose {
		decs = append(decs, verboseDecorator)
	}
	return transport.New(transport.Options{
		Timeout:    s.timeout(),
		MaxRetries: s.cfg().Defaults.MaxRetries,
		Decorators: decs,
	})
}

// verboseDecorator logs each outgoing request line to stderr.
func verboseDecorator(req *http.Request) {
	fmt.Fprintf(os.Stderr, "> %s %s\n", req.Method, req.URL.String())
}

// readOnly reports whether the effective posture for this invocation is
// read-only.
func (s *appState) readOnly() bool {
	return s.cfg().Defaults.ReadOnly && !s.gflags.allowWrites
}

// emit writes a successful result to stdout in the configured format.
func (s *appState) emit(v any) error {
	return output.Emit(v, output.Options{
		Format: s.cfg().Defaults.Format,
		Fields: s.fieldList(),
		Writer: os.Stdout,
		Pretty: s.gflags.pretty,
	})
}

// emitList writes a paginated list result to stdout as a {items, next,
// has_more} envelope in the configured format.
func (s *appState) emitList(items any, info pageInfo) error {
	return output.EmitList(items, info.Next, info.HasMore, output.Options{
		Format: s.cfg().Defaults.Format,
		Fields: s.fieldList(),
		Writer: os.Stdout,
		Pretty: s.gflags.pretty,
	})
}

// emitStream writes items as ndjson (one record per line) regardless of the
// configured format. It backs the streaming commands — `search run --all` and
// `search tail` — where a {items,...} envelope or a single buffered JSON blob
// would defeat the point of streaming a large or unbounded result.
func (s *appState) emitStream(items any) error {
	return output.EmitList(items, "", false, output.Options{
		Format: output.FormatNDJSON,
		Fields: s.fieldList(),
		Writer: os.Stdout,
		Pretty: s.gflags.pretty,
	})
}

// fieldList splits the --fields flag into dot paths.
func (s *appState) fieldList() []string {
	if s.gflags.fields == "" {
		return nil
	}
	parts := strings.Split(s.gflags.fields, ",")
	out := parts[:0]
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// timeout returns the resolved request timeout.
func (s *appState) timeout() time.Duration { return s.cfg().Defaults.Timeout }

// cmdContext returns a context bounded by the configured request timeout.
func cmdContext(s *appState) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), s.timeout())
}

// pageInfo carries the pagination cursor for one page of a listing. The
// OpenObserve endpoints used in v0.1 are unpaginated, so HasMore is always
// false; the type keeps emitList's envelope shape uniform with future paged
// listings.
type pageInfo struct {
	Next    string
	HasMore bool
}
