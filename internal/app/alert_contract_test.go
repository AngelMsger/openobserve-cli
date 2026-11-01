package app

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Expectations follow OpenObserve's v2 handlers and response DTOs, rather than
// being derived from the client's own URL helpers. Keep preview and execution
// consistent, including folder escaping, exact JSON numbers and absent bodies.
func TestAlertWriteContract(t *testing.T) {
	type request struct {
		Method string          `json:"method"`
		URL    string          `json:"url"`
		Body   json.RawMessage `json:"body"`
	}
	var requests []request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		requests = append(requests, request{r.Method, "http://" + r.Host + r.URL.RequestURI(), body})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"message":"Alert saved","id":"a1","name":"exact"}`))
	}))
	defer server.Close()
	paginationEnvironment(t, server.URL)
	t.Setenv("OPENOBSERVE_CLI_READ_ONLY", "1")
	file := filepath.Join(t.TempDir(), "alert.json")
	if err := os.WriteFile(file, []byte(`{"id":"old","owner":"someone","name":"exact","query_condition":{"threshold":9007199254740993}}`), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		args           []string
		method, suffix string
	}{
		{[]string{"create", "--file", file}, "POST", "/api/v2/default/alerts?folder=team+ops"},
		{[]string{"update", "a1", "--file", file}, "PUT", "/api/v2/default/alerts/a1?folder=team+ops"},
		{[]string{"delete", "a1", "--yes"}, "DELETE", "/api/v2/default/alerts/a1?folder=team+ops"},
		{[]string{"enable", "a1"}, "PATCH", "/api/v2/default/alerts/a1/enable?folder=team+ops&value=true"},
		{[]string{"disable", "a1"}, "PATCH", "/api/v2/default/alerts/a1/enable?folder=team+ops&value=false"},
	} {
		t.Run(tc.args[0], func(t *testing.T) {
			args := append([]string{"alert"}, tc.args...)
			args = append(args, "--folder", "team ops")
			before := len(requests)
			stdout, _, err := capturePaginationCommand(t, append(args, "--dry-run")...)
			if err != nil {
				t.Fatal(err)
			}
			if len(requests) != before {
				t.Fatal("dry-run contacted server")
			}
			var plan struct {
				Request request `json:"request"`
			}
			if err := json.Unmarshal([]byte(stdout), &plan); err != nil {
				t.Fatal(err)
			}
			if plan.Request.Method != tc.method || plan.Request.URL != server.URL+tc.suffix {
				t.Fatalf("wrong plan: %s", stdout)
			}
			// Every command is blocked under read-only mode, including no-body writes.
			if _, _, err := capturePaginationCommand(t, args...); err == nil {
				t.Fatal("read-only write succeeded")
			}
			if len(requests) != before {
				t.Fatal("read-only write contacted server")
			}
			if _, _, err := capturePaginationCommand(t, append(args, "--allow-writes")...); err != nil {
				t.Fatal(err)
			}
			if len(requests) != before+1 {
				t.Fatal("expected exactly one request")
			}
			actual := requests[before]
			if actual.Method != plan.Request.Method || actual.URL != plan.Request.URL {
				t.Fatalf("execution differs from plan: %+v", actual)
			}
			if tc.method == "POST" || tc.method == "PUT" {
				var expected, got map[string]json.RawMessage
				if err := json.Unmarshal(plan.Request.Body, &expected); err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(actual.Body, &got); err != nil {
					t.Fatal(err)
				}
				left, _ := json.Marshal(expected)
				right, _ := json.Marshal(got)
				if string(left) != string(right) || string(got["query_condition"]) != `{"threshold":9007199254740993}` {
					t.Fatalf("payload drift: %s / %s", left, right)
				}
				if _, ok := got["id"]; ok {
					t.Fatal("backup ID was sent")
				}
			} else if len(actual.Body) != 0 {
				t.Fatalf("body should be absent: %s", actual.Body)
			}
		})
	}
}

func TestAlertDryRunWithoutCredentials(t *testing.T) {
	paginationEnvironment(t, "https://example.test")
	t.Setenv("OPENOBSERVE_EMAIL", "")
	t.Setenv("OPENOBSERVE_PASSWORD", "")
	t.Setenv("OPENOBSERVE_CLI_READ_ONLY", "1")
	stdout, _, err := capturePaginationCommand(t, "alert", "disable", "a1", "--dry-run")
	if err != nil || !strings.Contains(stdout, "/api/v2/default/alerts/a1/enable") {
		t.Fatalf("offline plan failed: %s %v", stdout, err)
	}
}

func TestAlertDryRunRejectsInvalidBaseURL(t *testing.T) {
	paginationEnvironment(t, "http://")
	if _, _, err := capturePaginationCommand(t, "alert", "disable", "a1", "--dry-run"); err == nil {
		t.Fatal("invalid base URL must fail instead of producing a relative plan")
	}
}
