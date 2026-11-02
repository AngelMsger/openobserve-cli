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

	"github.com/angelmsger/openobserve-cli/pkg/apiclient"
	cerrors "github.com/angelmsger/openobserve-cli/pkg/errors"
)

func TestDashboardCommandsPreserveRevisionAndPlan(t *testing.T) {
	const backup = `{"version":5,"hash":"old","v1":null,"v2":null,"v3":null,"v4":null,"v5":{"version":5,"dashboardId":"d1","title":"exact","epoch":9007199254740993},"v6":null,"v7":null,"v8":null}`
	type sentRequest struct{ method, url, body string }
	var requests []sentRequest
	current := "old"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		requests = append(requests, sentRequest{r.Method, "http://" + r.Host + r.URL.RequestURI(), string(body)})
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "PUT" && r.URL.Query().Get("hash") != current {
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"message":"revision changed"}`))
			return
		}
		_, _ = w.Write([]byte(strings.Replace(backup, `"hash":"old"`, `"hash":"`+current+`"`, 1)))
	}))
	defer server.Close()
	paginationEnvironment(t, server.URL)
	t.Setenv("OPENOBSERVE_CLI_READ_ONLY", "1")
	dir := t.TempDir()
	saved := filepath.Join(dir, "backup.json")
	bare := filepath.Join(dir, "body.json")
	if err := os.WriteFile(saved, []byte(backup), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bare, []byte(`{"version":5,"title":"exact","epoch":9007199254740993}`), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name     string
		args     []string
		hash     string
		prefetch bool
		conflict bool
	}{
		{"create", []string{"create", "--file", saved}, "old", false, false},
		{"backup update", []string{"update", "d1", "--file", saved}, "old", false, false},
		{"explicit revision", []string{"update", "d1", "--file", saved, "--hash", "explicit"}, "explicit", false, false},
		{"bare definition", []string{"update", "d1", "--file", bare}, "fresh", true, false},
		{"stale backup", []string{"update", "d1", "--file", saved}, "newer", false, true},
		{"delete", []string{"delete", "d1", "--yes"}, "old", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			current = tc.hash
			requests = nil
			args := append([]string{"dashboard"}, tc.args...)
			args = append(args, "--folder", "ops")
			stdout, _, err := capturePaginationCommand(t, append(args, "--dry-run")...)
			if err != nil {
				t.Fatal(err)
			}
			if len(requests) != 0 {
				t.Fatal("dry-run contacted server")
			}
			var doc struct {
				Request apiclient.WriteRequestPlan `json:"request"`
			}
			if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
				t.Fatal(err)
			}
			plan := doc.Request
			if (plan.Prefetch != nil) != tc.prefetch {
				t.Fatalf("missing/extra prefetch: %s", stdout)
			}
			if _, _, err := capturePaginationCommand(t, args...); err == nil {
				t.Fatal("read-only write succeeded")
			}
			if len(requests) != 0 {
				t.Fatalf("read-only mode performed HTTP: %+v", requests)
			}
			_, _, err = capturePaginationCommand(t, append(args, "--allow-writes")...)
			if tc.conflict {
				if err == nil || cerrors.AsCLIError(err).Category != cerrors.CategoryConflict {
					t.Fatalf("stale backup not rejected: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			count := 1
			if tc.prefetch {
				count++
			}
			if len(requests) != count {
				t.Fatalf("unexpected requests: %+v", requests)
			}
			actual := requests[len(requests)-1]
			wantURL := plan.URL
			if tc.prefetch {
				if requests[0].method != "GET" || requests[0].url != plan.Prefetch.URL {
					t.Fatalf("prefetch differs: %+v", requests)
				}
				wantURL += "&hash=" + current
			}
			if actual.method != plan.Method || actual.url != wantURL {
				t.Fatalf("plan differs from execution: %s / %+v", stdout, actual)
			}
			if plan.Method != "DELETE" {
				if !strings.Contains(actual.body, `"epoch":9007199254740993`) || strings.Contains(actual.body, `"v1"`) {
					t.Fatalf("bad write body: %s", actual.body)
				}
			} else if actual.body != "" {
				t.Fatalf("DELETE has body: %s", actual.body)
			}
		})
	}
}

func TestDashboardDryRunWithoutCredentials(t *testing.T) {
	paginationEnvironment(t, "https://example.test")
	t.Setenv("OPENOBSERVE_EMAIL", "")
	t.Setenv("OPENOBSERVE_PASSWORD", "")
	t.Setenv("OPENOBSERVE_CLI_READ_ONLY", "1")
	if _, _, err := capturePaginationCommand(t, "dashboard", "delete", "d1", "--dry-run"); err != nil {
		t.Fatal(err)
	}
}
