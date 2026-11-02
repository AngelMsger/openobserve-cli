package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/angelmsger/openobserve-cli/pkg/apiclient"
	cerrors "github.com/angelmsger/openobserve-cli/pkg/errors"
)

func TestCommandSearchNamespaces(t *testing.T) {
	for _, tc := range []struct {
		name, streamType string
		args             []string
		calls            int
		stopTail         bool
	}{
		{"default logs", "logs", []string{"search", "run", "--stream", "shared", "--since", "1h"}, 1, false},
		{"raw traces", "traces", []string{"search", "run", "--sql", `SELECT * FROM "shared"`, "--type", "traces", "--since", "1h"}, 1, false},
		{"lookahead retains metrics", "metrics", []string{"search", "run", "--stream", "shared", "--type", "metrics", "--since", "1h", "--limit", "1", "--format", "ndjson"}, 2, false},
		{"all pages retain traces", "traces", []string{"search", "run", "--stream", "shared", "--type", "traces", "--since", "1h", "--limit", "1", "--all"}, 2, false},
		{"histogram metrics", "metrics", []string{"search", "histogram", "--stream", "shared", "--type", "metrics", "--since", "1h"}, 1, false},
		{"trace get", "traces", []string{"trace", "get", "t1", "--stream", "shared", "--since", "1h"}, 1, false},
		{"tail traces", "traces", []string{"search", "tail", "--stream", "shared", "--type", "traces", "--since", "1h"}, 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			var first apiclient.SearchQuery
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.URL.Path != "/api/default/_search" || r.URL.Query().Get("type") != tc.streamType {
					t.Errorf("request went to %s; want type=%s", r.URL, tc.streamType)
				}
				if tc.stopTail {
					// Stop deterministically after the first poll; do not signal the test process.
					http.Error(w, "stop fixture", http.StatusBadRequest)
					return
				}
				var req apiclient.SearchRequest
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Error(err)
				}
				if calls == 1 {
					first = req.Query
					_, _ = w.Write([]byte(`{"hits":[{"span_id":"s1","start_time":100,"end_time":200,"zo_sql_key":100,"zo_sql_num":1}],"total":1}`))
					return
				}
				if req.Query.SQL != first.SQL || req.Query.StartTime != first.StartTime || req.Query.EndTime != first.EndTime || req.Query.From != 1 {
					t.Errorf("continuation changed query/window: %+v -> %+v", first, req.Query)
				}
				_, _ = w.Write([]byte(`{"hits":[],"total":0}`))
			}))
			defer server.Close()
			paginationEnvironment(t, server.URL)
			_, _, err := capturePaginationCommand(t, tc.args...)
			if (err != nil) != tc.stopTail || calls != tc.calls {
				t.Fatalf("error=%v calls=%d, want error=%v calls=%d", err, calls, tc.stopTail, tc.calls)
			}
		})
	}
}

func TestInvalidSearchTypeStopsBeforeRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("invalid stream type reached the server")
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()
	paginationEnvironment(t, server.URL)
	for _, verb := range []string{"run", "histogram", "tail"} {
		_, _, err := capturePaginationCommand(t, "search", verb, "--stream", "shared", "--since", "1h", "--type", "trace")
		if ce := cerrors.AsCLIError(err); err == nil || ce.Code != "BAD_STREAM_TYPE" || ce.Category != cerrors.CategoryUsage {
			t.Fatalf("%s: %v", verb, err)
		}
	}
}
