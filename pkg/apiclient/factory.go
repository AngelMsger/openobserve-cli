package apiclient

import (
	"net/url"
	"strings"
	"time"

	cerrors "github.com/angelmsger/openobserve-cli/pkg/errors"
	"github.com/angelmsger/openobserve-cli/pkg/transport"
)

// BuildParams configures Build.
type BuildParams struct {
	BaseURL string
	Org     string
	// AuthDecorator authenticates every request. Required.
	AuthDecorator transport.Decorator
	Timeout       time.Duration
	MaxRetries    int
}

// Build assembles a ready-to-use Client: it normalizes the base URL and
// constructs the retrying HTTP transport carrying the auth decorator.
func Build(p BuildParams) (Client, error) {
	if p.BaseURL == "" {
		return nil, cerrors.New(cerrors.CategoryConfig, "NO_BASE_URL",
			"no OpenObserve server URL configured").
			WithNextSteps("openobserve-cli config init",
				"Set OPENOBSERVE_URL or pass --base-url (e.g. http://localhost:5080).")
	}
	base, err := NormalizeBaseURL(p.BaseURL)
	if err != nil {
		return nil, err
	}

	tc := transport.New(transport.Options{
		Timeout:    p.Timeout,
		MaxRetries: p.MaxRetries,
		Decorators: []transport.Decorator{p.AuthDecorator},
	})

	return New(Config{BaseURL: base, Org: p.Org, Transport: tc}), nil
}

// NormalizeBaseURL trims a trailing slash and supplies a scheme when the user
// gave a bare host:port (the common self-hosted case).
func NormalizeBaseURL(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", cerrors.New(cerrors.CategoryConfig, "NO_BASE_URL", "empty base URL")
	}
	if !strings.Contains(s, "://") {
		s = "http://" + s
	}
	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return "", cerrors.Newf(cerrors.CategoryConfig, "BAD_BASE_URL",
			"could not parse base URL %q", raw).
			WithHint("Use a form like http://localhost:5080 or https://api.openobserve.ai.")
	}
	return strings.TrimRight(u.String(), "/"), nil
}
