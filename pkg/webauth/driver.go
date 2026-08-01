package webauth

import (
	"context"
	"time"

	"github.com/angelmsger/openobserve-cli/pkg/apiclient"
	pkgauth "github.com/angelmsger/openobserve-cli/pkg/auth"
)

// Driver opens a browser at loginURL and captures the session established
// there. host scopes which cookies are kept. Implementations block until login
// is verified, the user gives up, or a timeout elapses.
//
// Two implementations exist: pkg/webauth/cdp drives a Chromium-family browser
// over the DevTools Protocol (used by the CLI, and by o3 off macOS), and o3's
// own darwin shell drives a native WKWebView.
type Driver interface {
	Capture(loginURL, host string, verify VerifyFunc) (pkgauth.Session, error)
}

// verifyTimeout bounds a single verification request. It is generous because
// the probe fires while the user is mid-login on a possibly slow instance.
const verifyTimeout = 20 * time.Second

// PingVerifier returns a VerifyFunc that confirms a session by making a real
// authenticated request to the instance. This is the sole success signal for
// capture: only an authenticated response proves the captured state works.
//
// It takes plain values rather than a config.Defaults so this package stays
// free of pkg/config (and so of yaml.v3). Callers pass d.Timeout, d.MaxRetries.
func PingVerifier(baseURL, org string, timeout time.Duration, maxRetries int) VerifyFunc {
	return func(sess pkgauth.Session) bool {
		blob, err := pkgauth.EncodeSession(sess)
		if err != nil {
			return false
		}
		cred := pkgauth.Credential{
			Scheme:   pkgauth.SchemeSession,
			Username: sess.Email,
			Secret:   blob,
		}
		if err := cred.Validate(); err != nil {
			return false
		}
		client, err := apiclient.Build(apiclient.BuildParams{
			BaseURL:       baseURL,
			Org:           org,
			AuthDecorator: cred.Decorator(),
			Timeout:       timeout,
			MaxRetries:    maxRetries,
		})
		if err != nil {
			return false
		}
		ctx, cancel := context.WithTimeout(context.Background(), verifyTimeout)
		defer cancel()
		_, err = client.Ping(ctx)
		return err == nil
	}
}
