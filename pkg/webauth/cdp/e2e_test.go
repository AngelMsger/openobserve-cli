//go:build browser_e2e

package cdp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	pkgauth "github.com/angelmsger/openobserve-cli/pkg/auth"
	"github.com/angelmsger/openobserve-cli/pkg/webauth"
)

// fakeInstance imitates the parts of OpenObserve that capture depends on: a
// login page whose submit handler sets a cookie AND sends an Authorization
// header over XHR (as the real SPA's axios client does), and an API that
// answers only when one of the two is presented.
func fakeInstance(t *testing.T) *httptest.Server {
	t.Helper()
	// The test cannot click, so the page also signs itself in shortly after
	// load. That delay is deliberate: it forces capture to observe a
	// pre-login state first, exercising the Tracker's "not yet" path.
	const loginHTML = `<!doctype html><html><body>
<button id="go">Sign in</button>
<script>
function signIn(){
  document.cookie = 'auth_tokens=secret-token; path=/';
  localStorage.setItem('user_info', JSON.stringify({email:'ops@example.com'}));
  var x = new XMLHttpRequest();
  x.open('GET', '/api/organizations', true);
  x.setRequestHeader('Authorization', 'Basic ops-durable-token');
  x.onload = function(){ location.href = '/web/logs'; };
  x.send();
}
document.getElementById('go').onclick = signIn;
setTimeout(signIn, 1500);
</script></body></html>`

	mux := http.NewServeMux()
	mux.HandleFunc("/web/login", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(loginHTML))
	})
	mux.HandleFunc("/web/logs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<!doctype html><html><body>signed in</body></html>`))
	})
	mux.HandleFunc("/api/organizations", func(w http.ResponseWriter, r *http.Request) {
		authed := r.Header.Get("Authorization") == "Basic ops-durable-token"
		if c, err := r.Cookie("auth_tokens"); err == nil && c.Value == "secret-token" {
			authed = true
		}
		if !authed {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"identifier":"default","name":"default"}]}`))
	})
	return httptest.NewServer(mux)
}

func TestCaptureAgainstRealBrowser(t *testing.T) {
	if _, err := findBrowser(); err != nil {
		t.Skipf("no browser available: %v", err)
	}
	srv := fakeInstance(t)
	defer srv.Close()

	host := mustHost(t, srv.URL)
	// The real verifier, not a stub: apiclient.Ping issues
	// GET /api/organizations, which is exactly what the fake instance serves
	// and gates on credentials. So this exercises the whole chain — injected
	// script, binding, cookie sampling, Tracker, authenticated probe.
	verify := webauth.PingVerifier(srv.URL, "default", 5*time.Second, 0)

	d := New(Options{Timeout: 90 * time.Second})

	done := make(chan struct{})
	var sess pkgauth.Session
	var captureErr error
	go func() {
		defer close(done)
		sess, captureErr = d.Capture(srv.URL+"/web/login", host, verify)
	}()

	select {
	case <-done:
	case <-time.After(100 * time.Second):
		t.Fatal("capture never returned")
	}

	if captureErr != nil {
		t.Fatalf("Capture: %v", captureErr)
	}
	if !strings.Contains(sess.Cookies, "auth_tokens=secret-token") {
		t.Errorf("cookies not captured: %q", sess.Cookies)
	}
	if sess.Authorization != "Basic ops-durable-token" {
		t.Errorf("Authorization not captured from XHR: %q", sess.Authorization)
	}
	if sess.Email != "ops@example.com" {
		t.Errorf("email not captured from localStorage: %q", sess.Email)
	}
}

func mustHost(t *testing.T, raw string) string {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u.Host
}

// TestCaptureIgnoresForeignOriginAuthorization is the regression test for the
// cross-origin leak: the capture script is installed with
// Page.addScriptToEvaluateOnNewDocument, which evaluates it in EVERY document
// the target loads — including a full-page identity provider on another origin.
// An IdP that sends its own Authorization header over XHR therefore had that
// header captured, written to the user's keychain, and replayed on every
// request to the OpenObserve instance.
//
// The instance here authenticates purely with a cookie and never sends an
// Authorization header of its own, so the assertion is unambiguous: any
// Authorization in the captured session can only have come from the foreign
// origin. The two origins are `localhost` and `127.0.0.1`, which are distinct
// hostnames on the same loopback interface — the gate compares hostnames, so a
// second port would not do.
func TestCaptureIgnoresForeignOriginAuthorization(t *testing.T) {
	if _, err := findBrowser(); err != nil {
		t.Skipf("no browser available: %v", err)
	}

	var instanceBase, idpBase string

	idpMux := http.NewServeMux()
	// The IdP keeps sending its header until the browser leaves the page, so a
	// leak cannot be masked by the instance happening to write last.
	idpMux.HandleFunc("/idp", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<!doctype html><html><body>identity provider
<script>
function shout(){
  var x = new XMLHttpRequest();
  x.open('GET', '/token', true);
  x.setRequestHeader('Authorization', 'Basic idp-secret');
  x.send();
}
shout();
setInterval(shout, 200);
setTimeout(function(){ location.href = '/back'; }, 900);
</script></body></html>`))
	})
	idpMux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	})
	// Bouncing back through the IdP's own origin means the IdP page needs no
	// knowledge of the instance URL, which is only known after both servers
	// have been started.
	idpMux.HandleFunc("/back", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, instanceBase+"/web/login?done=1", http.StatusFound)
	})
	idp := httptest.NewServer(idpMux)
	defer idp.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/web/login", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		if r.URL.Query().Get("done") != "" {
			// Back from the IdP: sign in with a cookie only. This instance
			// never sends an Authorization header.
			_, _ = w.Write([]byte(`<!doctype html><html><body>
<script>
document.cookie = 'auth_tokens=secret-token; path=/';
localStorage.setItem('user_info', JSON.stringify({email:'ops@example.com'}));
setTimeout(function(){ location.href = '/web/logs'; }, 200);
</script></body></html>`))
			return
		}
		_, _ = w.Write([]byte(`<!doctype html><html><body>sign in
<script>setTimeout(function(){ location.href = '` + idpBase + `/idp'; }, 400);</script>
</body></html>`))
	})
	mux.HandleFunc("/web/logs", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<!doctype html><html><body>signed in</body></html>`))
	})
	mux.HandleFunc("/api/organizations", func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie("auth_tokens")
		if err != nil || c.Value != "secret-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"identifier":"default","name":"default"}]}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// httptest binds 127.0.0.1; reach the instance through `localhost` so the
	// two servers differ by hostname, not just by port.
	port := mustPort(t, srv.URL)
	instanceBase = "http://localhost:" + port
	idpBase = idp.URL

	verify := webauth.PingVerifier(instanceBase, "default", 5*time.Second, 0)
	d := New(Options{Timeout: 90 * time.Second})

	done := make(chan struct{})
	var sess pkgauth.Session
	var captureErr error
	go func() {
		defer close(done)
		sess, captureErr = d.Capture(instanceBase+"/web/login", "localhost:"+port, verify)
	}()

	select {
	case <-done:
	case <-time.After(100 * time.Second):
		t.Fatal("capture never returned")
	}
	if captureErr != nil {
		t.Fatalf("Capture: %v", captureErr)
	}
	if !strings.Contains(sess.Cookies, "auth_tokens=secret-token") {
		t.Errorf("the instance's own cookie was not captured: %q", sess.Cookies)
	}
	if sess.Authorization != "" {
		t.Fatalf("captured an Authorization header from a foreign origin: %q — "+
			"it would be written to the keychain and sent to the OpenObserve instance",
			sess.Authorization)
	}
}

// TestCaptureReturnsCancelledWhenThePageTargetCloses pins the fix for the
// ten-minute hang: closing the sign-in window leaves the browser process alive
// on macOS, so `exited` never fires; every later sample() then fails with
// "Session with given id not found" and the poll loop spun until the timeout,
// making BROWSER_SIGNIN_CANCELLED unreachable. Closing the page target is what
// the user's window close does at the protocol level, so drive that directly
// through a second debugger connection to the same browser.
func TestCaptureReturnsCancelledWhenThePageTargetCloses(t *testing.T) {
	if _, err := findBrowser(); err != nil {
		t.Skipf("no browser available: %v", err)
	}
	mux := http.NewServeMux()
	// A page that never signs in: only the close can end this capture.
	mux.HandleFunc("/web/login", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<!doctype html><html><body>waiting for sign-in</body></html>`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// A known profile directory is what lets the test find the browser's
	// debugging endpoint and act as a second client.
	profileDir := t.TempDir()
	d := New(Options{ProfileDir: profileDir, Timeout: 90 * time.Second})

	done := make(chan struct{})
	var captureErr error
	go func() {
		defer close(done)
		_, captureErr = d.Capture(srv.URL+"/web/login", mustHost(t, srv.URL),
			func(pkgauth.Session) bool { return false })
	}()

	closePageTarget(t, profileDir, "/web/login")

	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("Capture did not return within 30s of the window closing; " +
			"it is spinning on a dead session until the sign-in timeout")
	}
	if !errors.Is(captureErr, ErrCancelled) {
		t.Fatalf("Capture returned %v, want ErrCancelled", captureErr)
	}
}

// closePageTarget waits for a page target whose URL contains want, then closes
// it — the protocol-level equivalent of the user closing the sign-in window.
func closePageTarget(t *testing.T, profileDir, want string) {
	t.Helper()
	wsURL, err := readDevToolsURL(profileDir, 30*time.Second, func() bool { return true })
	if err != nil {
		t.Fatalf("find the browser's debugging endpoint: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c, err := dial(ctx, wsURL)
	if err != nil {
		t.Fatalf("dial devtools: %v", err)
	}
	defer c.Close()

	deadline := time.Now().Add(30 * time.Second)
	for {
		var targets struct {
			TargetInfos []struct {
				TargetID string `json:"targetId"`
				Type     string `json:"type"`
				URL      string `json:"url"`
			} `json:"targetInfos"`
		}
		if err := c.call(ctx, "", "Target.getTargets", nil, &targets); err != nil {
			t.Fatalf("Target.getTargets: %v", err)
		}
		for _, tg := range targets.TargetInfos {
			if tg.Type != "page" || !strings.Contains(tg.URL, want) {
				continue
			}
			// Give Capture a moment to finish attaching and start polling, so
			// the close lands on an established session rather than racing it.
			time.Sleep(2 * time.Second)
			if err := c.call(ctx, "", "Target.closeTarget",
				map[string]any{"targetId": tg.TargetID}, nil); err != nil {
				t.Fatalf("Target.closeTarget: %v", err)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("no page target reached %q", want)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func mustPort(t *testing.T, raw string) string {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u.Port()
}
