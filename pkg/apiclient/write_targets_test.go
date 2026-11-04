package apiclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	cerrors "github.com/angelmsger/openobserve-cli/pkg/errors"
)

var (
	testAlertBody     = json.RawMessage(`{"name":"cpu-high","stream_name":"app","stream_type":"logs","is_real_time":false}`)
	testDashboardBody = json.RawMessage(`{"title":"Ops overview","version":5}`)
)

// A write that fails after it was sent may have been applied, so its error must
// never read as retryable: it says the outcome is unknown and names the read
// that shows the remote state. Before the kit's write rule a 5xx after a write
// carried the transport's retryable:true, inviting an agent to repeat it.
func TestWriteFailuresAreUnknownOutcomes(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name  string
		write func(Client) error
		read  string
	}{
		{"create alert", func(c Client) error {
			_, err := c.CreateAlert(ctx, "team-a", "ops", testAlertBody)
			return err
		}, "openobserve-cli --org 'team-a' alert list --folder 'ops'"},
		{"update alert", func(c Client) error {
			_, err := c.UpdateAlert(ctx, "team-a", "a1", "", testAlertBody)
			return err
		}, "openobserve-cli --org 'team-a' alert get 'a1' --folder 'default'"},
		{"delete alert", func(c Client) error {
			return c.DeleteAlert(ctx, "team-a", "a1", "ops")
		}, "openobserve-cli --org 'team-a' alert get 'a1' --folder 'ops'"},
		{"enable alert", func(c Client) error {
			return c.SetAlertEnabled(ctx, "team-a", "a1", "ops", true)
		}, "openobserve-cli --org 'team-a' alert get 'a1' --folder 'ops'"},
		{"create dashboard", func(c Client) error {
			_, err := c.CreateDashboard(ctx, "team-a", "ops", testDashboardBody)
			return err
		}, "openobserve-cli --org 'team-a' dashboard list --folder 'ops' --title 'Ops overview'"},
		{"update dashboard", func(c Client) error {
			_, err := c.UpdateDashboard(ctx, "team-a", "d1", "ops", "h1", testDashboardBody)
			return err
		}, "openobserve-cli --org 'team-a' dashboard get 'd1' --folder 'ops'"},
		{"delete dashboard", func(c Client) error {
			return c.DeleteDashboard(ctx, "team-a", "d1", "ops")
		}, "openobserve-cli --org 'team-a' dashboard get 'd1' --folder 'ops'"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusServiceUnavailable)
			})
			ce := cerrors.AsCLIError(tc.write(c))
			if ce == nil || ce.Category != cerrors.CategoryServer || ce.HTTPStatus != 503 || ce.Retryable {
				t.Fatalf("got %+v; want a non-retryable server 503", ce)
			}
			if !strings.HasPrefix(ce.Message, "Write outcome is unknown for ") || !strings.Contains(ce.Message, "of org team-a") {
				t.Fatalf("message %q does not name the write target", ce.Message)
			}
			if len(ce.NextSteps) == 0 || ce.NextSteps[0] != tc.read {
				t.Fatalf("next steps %q, want %q", ce.NextSteps, tc.read)
			}
		})
	}
}

// A definite refusal shows nothing was written, so it keeps its own code and
// guidance: a stale dashboard hash is still a conflict to merge.
func TestWriteRefusalPassesThrough(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"message":"concurrent dashboard update"}`))
	})
	_, err := c.UpdateDashboard(context.Background(), "default", "d1", "ops", "h1", testDashboardBody)
	if ce := cerrors.AsCLIError(err); ce.Category != cerrors.CategoryConflict || strings.Contains(ce.Message, "Write outcome") {
		t.Fatalf("got %s %q; want the conflict unchanged", ce.Category, ce.Message)
	}
}

// The server acknowledged these creates, so their errors say the write
// happened and must not invite a second one. A create that named no ID used
// to report success with an empty ID.
func TestCreateAcknowledgedButUnusable(t *testing.T) {
	for name, body := range map[string]string{"undecodable": `{"id":`, "no id": `{"name":"cpu-high"}`} {
		t.Run(name, func(t *testing.T) {
			c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(body))
			})
			_, err := c.CreateAlert(context.Background(), "default", "", testAlertBody)
			ce := cerrors.AsCLIError(err)
			if ce == nil || ce.Code != cerrors.CodeWriteSucceededResponseInvalid || ce.Retryable ||
				!strings.HasPrefix(ce.Message, `Write succeeded for new alert "cpu-high" in folder default of org default`) {
				t.Fatalf("got %+v; want %s", ce, cerrors.CodeWriteSucceededResponseInvalid)
			}
		})
	}
}

// A redirect after a write is followed with a GET, so the final 200 answers
// that GET (here a login page) and says nothing about the write. Before this
// rule a delete that landed on a login page read as done.
func TestRedirectedWriteIsAnUnknownOutcome(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login" {
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<html>sign in</html>"))
			return
		}
		http.Redirect(w, r, "/login", http.StatusFound)
	})
	err := c.DeleteDashboard(context.Background(), "default", "d1", "ops")
	ce := cerrors.AsCLIError(err)
	if err == nil || ce.Retryable || !strings.HasPrefix(ce.Message, "Write outcome is unknown for dashboard d1") {
		t.Fatalf("got %v; want an unknown write outcome", err)
	}
	var cause *cerrors.CLIError
	if !errors.As(errors.Unwrap(ce), &cause) || cause.Code != "HTTP_REDIRECTED" {
		t.Fatalf("cause %+v; want HTTP_REDIRECTED", cause)
	}
}
