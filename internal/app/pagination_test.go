package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/angelmsger/openobserve-cli/pkg/apiclient"
)

func paginationEnvironment(t *testing.T, url string) {
	t.Helper()
	for key, value := range map[string]string{
		"OPENOBSERVE_URL": url, "OPENOBSERVE_ORG": "default",
		"OPENOBSERVE_AUTH_SCHEME": "basic", "OPENOBSERVE_EMAIL": "test@example.com",
		"OPENOBSERVE_PASSWORD": "fixture", "OPENOBSERVE_TOKEN": "", "OPENOBSERVE_CONTEXT": "",
		"OPENOBSERVE_CLI_NO_SKILL_HINT": "1", "OPENOBSERVE_CLI_NO_UPDATE_NOTIFIER": "1",
	} {
		t.Setenv(key, value)
	}
}

// Command rendering uses the process streams. Temporary files avoid pipe
// buffering and preserve stdout/stderr separately without parallel tests.
func capturePaginationCommand(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	dir := t.TempDir()
	data, err := os.Create(filepath.Join(dir, "stdout"))
	if err != nil {
		t.Fatal(err)
	}
	defer data.Close()
	notices, err := os.Create(filepath.Join(dir, "stderr"))
	if err != nil {
		t.Fatal(err)
	}
	defer notices.Close()
	oldOut, oldErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = data, notices
	defer func() { os.Stdout, os.Stderr = oldOut, oldErr }()
	cmd := NewRootCmd()
	cmd.SetArgs(append([]string{"--config", dir}, args...))
	cmd.SetOut(data)
	cmd.SetErr(notices)
	runErr := cmd.Execute()
	stdout, err := os.ReadFile(data.Name())
	if err != nil {
		t.Fatal(err)
	}
	stderr, err := os.ReadFile(notices.Name())
	if err != nil {
		t.Fatal(err)
	}
	return string(stdout), string(stderr), runErr
}

func paginationOffset(t *testing.T, stderr string) string {
	t.Helper()
	var record struct {
		Notice struct {
			Pagination struct {
				Next    string `json:"next"`
				HasMore bool   `json:"has_more"`
			} `json:"pagination"`
			NextSteps []string `json:"next_steps"`
		} `json:"_notice"`
	}
	if err := json.Unmarshal([]byte(stderr), &record); err != nil {
		t.Fatalf("invalid notice: %v: %s", err, stderr)
	}
	if !record.Notice.Pagination.HasMore || record.Notice.Pagination.Next == "" ||
		!strings.Contains(strings.Join(record.Notice.NextSteps, " "), "--offset") {
		t.Fatalf("missing usable offset: %s", stderr)
	}
	return record.Notice.Pagination.Next
}

func TestSearchNDJSONResume(t *testing.T) {
	for _, globalTotal := range []bool{true, false} {
		t.Run(strconv.FormatBool(globalTotal), func(t *testing.T) {
			var requests []apiclient.SearchQuery
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req apiclient.SearchRequest
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Error(err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				requests = append(requests, req.Query)
				rows := []map[string]any{}
				for i := req.Query.From; i < 3 && i < req.Query.From+req.Query.Size; i++ {
					rows = append(rows, map[string]any{"id": i, "drop": true})
				}
				total := len(rows)
				if globalTotal {
					total = 3
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"total": total, "hits": rows})
			}))
			defer server.Close()
			paginationEnvironment(t, server.URL)
			args := []string{"--format", "ndjson", "--fields", "id", "search", "run", "--stream", "app", "--limit", "1", "--from", "2026-09-01T00:00:00Z", "--to", "2026-09-01T01:00:00Z", "--offset"}
			out, notice, err := capturePaginationCommand(t, append(args, "1")...)
			if err != nil || out != "{\"id\":1}\n" {
				t.Fatalf("first page: error=%v stdout=%s", err, out)
			}
			next := paginationOffset(t, notice)
			if next != "2" {
				t.Fatalf("next = %s", next)
			}
			out, notice, err = capturePaginationCommand(t, append(args, next)...)
			if err != nil || out != "{\"id\":2}\n" || notice != "" {
				t.Fatalf("terminal page: error=%v stdout=%s stderr=%s", err, out, notice)
			}
			wantOffsets := []int{1, 2, 3}
			if !globalTotal {
				wantOffsets = []int{1, 2, 2, 3}
			}
			var offsets []int
			for _, req := range requests {
				offsets = append(offsets, req.From)
				if req.SQL != requests[0].SQL || req.StartTime != requests[0].StartTime || req.EndTime != requests[0].EndTime || req.Size != 1 {
					t.Fatalf("lookahead changed query/window/size: %+v", requests)
				}
			}
			if !reflect.DeepEqual(offsets, wantOffsets) {
				t.Fatalf("offsets = %v, want %v", offsets, wantOffsets)
			}
		})
	}
}

func TestSearchPaginationBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, format string
		rows, total  int
		probeFails   bool
		wantCalls    int
	}{
		{"short page", "ndjson", 1, 1, false, 1},
		{"empty page with stale total", "ndjson", 0, 100, false, 1},
		{"full terminal page", "ndjson", 2, 2, false, 2},
		{"lookahead fails", "ndjson", 2, 2, true, 2},
		{"JSON stays one request", "json", 2, 2, false, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			var first apiclient.SearchQuery
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var req apiclient.SearchRequest
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Error(err)
				}
				rows := make([]map[string]any, 0, tc.rows)
				if calls == 1 {
					first = req.Query
					for i := 0; i < tc.rows; i++ {
						rows = append(rows, map[string]any{"id": i})
					}
				} else {
					if req.Query.From != tc.rows || req.Query.Size != 1 || req.Query.SQL != first.SQL || req.Query.StartTime != first.StartTime || req.Query.EndTime != first.EndTime {
						t.Errorf("invalid lookahead: %+v after %+v", req.Query, first)
					}
					if tc.probeFails {
						w.WriteHeader(http.StatusBadRequest)
						_, _ = w.Write([]byte(`{"message":"probe failed"}`))
						return
					}
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"total": tc.total, "hits": rows})
			}))
			defer server.Close()
			paginationEnvironment(t, server.URL)
			out, notice, err := capturePaginationCommand(t, "--format", tc.format, "search", "run", "--stream", "app", "--since", "1h", "--limit", "2")
			if (err != nil) != tc.probeFails || calls != tc.wantCalls || notice != "" {
				t.Fatalf("error=%v calls=%d stderr=%s", err, calls, notice)
			}
			if tc.probeFails && out != "" {
				t.Fatalf("failed lookahead must not report successful completion: %s", out)
			}
			if tc.format == "json" {
				var result map[string]any
				if err := json.Unmarshal([]byte(out), &result); err != nil {
					t.Fatal(err)
				}
				if len(result) != 8 || result["returned"] != float64(2) || result["hits"] == nil {
					t.Fatalf("JSON summary changed: %s", out)
				}
			}
		})
	}
}

func TestTraceNDJSONResumeAndEmptyPage(t *testing.T) {
	var offsets []int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		from, _ := strconv.Atoi(r.URL.Query().Get("from"))
		offsets = append(offsets, from)
		rows := []map[string]any{}
		if from < 3 {
			rows = append(rows, map[string]any{"trace_id": strconv.Itoa(from), "duration": 50})
		}
		total := 3
		if from == 3 {
			total = 10 // A stale total cannot justify repeating the same empty page.
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"total": total, "hits": rows})
	}))
	defer server.Close()
	paginationEnvironment(t, server.URL)
	args := []string{"--format", "ndjson", "--fields", "trace_id", "trace", "search", "--stream", "traces", "--limit", "1", "--from", "2026-09-01T00:00:00Z", "--to", "2026-09-01T01:00:00Z", "--offset"}
	out, notice, err := capturePaginationCommand(t, append(args, "1")...)
	if err != nil || out != "{\"trace_id\":\"1\"}\n" {
		t.Fatalf("first trace page: error=%v stdout=%s", err, out)
	}
	next := paginationOffset(t, notice)
	out, notice, err = capturePaginationCommand(t, append(args, next)...)
	if err != nil || out != "{\"trace_id\":\"2\"}\n" || notice != "" {
		t.Fatalf("last trace page: error=%v stdout=%s stderr=%s", err, out, notice)
	}
	out, notice, err = capturePaginationCommand(t, append(args, "3")...)
	if err != nil || out != "" || notice != "" || !reflect.DeepEqual(offsets, []int{1, 2, 3}) {
		t.Fatalf("empty trace page: error=%v stdout=%s stderr=%s offsets=%v", err, out, notice, offsets)
	}
}

func TestSearchAllKeepsStreamingAndCapNotice(t *testing.T) {
	for _, cap := range []string{"0", "3"} {
		t.Run(cap, func(t *testing.T) {
			var offsets []int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req apiclient.SearchRequest
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Error(err)
				}
				offsets = append(offsets, req.Query.From)
				rows := []map[string]any{}
				for i := req.Query.From; i < 4 && i < req.Query.From+req.Query.Size; i++ {
					rows = append(rows, map[string]any{"id": i})
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"total": len(rows), "hits": rows})
			}))
			defer server.Close()
			paginationEnvironment(t, server.URL)
			out, notice, err := capturePaginationCommand(t, "--format", "json", "search", "run", "--stream", "app", "--since", "1h", "--all", "--limit", "2", "--max", cap)
			want := "{\"id\":0}\n{\"id\":1}\n{\"id\":2}\n"
			wantOffsets := []int{0, 2}
			if cap == "0" {
				want += "{\"id\":3}\n"
				wantOffsets = append(wantOffsets, 4)
			}
			if err != nil || out != want || !reflect.DeepEqual(offsets, wantOffsets) || strings.Contains(notice, "pagination") {
				t.Fatalf("--all behavior changed: error=%v stdout=%s stderr=%s offsets=%v", err, out, notice, offsets)
			}
			if (cap == "0" && notice != "") || (cap == "3" && (!strings.Contains(notice, "truncated") || !strings.Contains(notice, "--max reached"))) {
				t.Fatalf("--all cap notice changed: %s", notice)
			}
		})
	}
}

// Discovery endpoints are unpaginated: their NDJSON output is rows only, with
// no fabricated continuation.
func TestUnpaginatedListingsEmitNoContinuation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/streams"):
			_ = json.NewEncoder(w).Encode(map[string]any{"list": []map[string]any{
				{"name": "app", "stream_type": "logs"}, {"name": "web", "stream_type": "logs"},
			}})
		case r.URL.Path == "/api/organizations":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{
				{"identifier": "default", "name": "Default"}, {"identifier": "team-a", "name": "Team A"},
			}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	paginationEnvironment(t, server.URL)
	for _, args := range [][]string{{"stream", "list"}, {"org", "list"}} {
		out, notice, err := capturePaginationCommand(t, append([]string{"--format", "ndjson"}, args...)...)
		if err != nil || strings.Count(out, "\n") != 2 || notice != "" {
			t.Fatalf("%v: error=%v stdout=%s stderr=%s", args, err, out, notice)
		}
	}
}

// Trace search computes continuation independently of the output format, so
// JSON carries it in the envelope and the table footer names --offset.
func TestTraceContinuationInJSONAndTable(t *testing.T) {
	var windows []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		windows = append(windows, r.URL.Query().Get("start_time")+"/"+r.URL.Query().Get("end_time"))
		_ = json.NewEncoder(w).Encode(map[string]any{"total": 3, "hits": []map[string]any{{"trace_id": "a", "duration": 50}}})
	}))
	defer server.Close()
	paginationEnvironment(t, server.URL)
	args := []string{"trace", "search", "--stream", "traces", "--limit", "1", "--from", "2026-09-01T00:00:00Z", "--to", "2026-09-01T01:00:00Z", "--offset", "1"}
	out, notice, err := capturePaginationCommand(t, append([]string{"--format", "json"}, args...)...)
	var envelope struct {
		Items   []map[string]any `json:"items"`
		Next    string           `json:"next"`
		HasMore bool             `json:"has_more"`
	}
	if err != nil || notice != "" || json.Unmarshal([]byte(out), &envelope) != nil ||
		len(envelope.Items) != 1 || envelope.Next != "2" || !envelope.HasMore {
		t.Fatalf("JSON envelope: error=%v stdout=%s stderr=%s", err, out, notice)
	}
	out, notice, err = capturePaginationCommand(t, append([]string{"--format", "table"}, args...)...)
	if err != nil || notice != "" || !strings.Contains(out, "--offset 2") || strings.Contains(out, "--cursor") {
		t.Fatalf("table footer: error=%v stdout=%s stderr=%s", err, out, notice)
	}
	// Fixed absolute bounds resolve to the same window on every invocation.
	if len(windows) != 2 || windows[0] != windows[1] {
		t.Fatalf("fixed bounds moved between pages: %v", windows)
	}
}

// --all resolves a relative window once, so every page of one traversal reads
// the same bounds even though --since is measured from the current time.
func TestSearchAllResolvesARelativeWindowOnce(t *testing.T) {
	var requests []apiclient.SearchQuery
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req apiclient.SearchRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		requests = append(requests, req.Query)
		rows := []map[string]any{}
		for i := req.Query.From; i < 5 && i < req.Query.From+req.Query.Size; i++ {
			rows = append(rows, map[string]any{"id": i})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"total": len(rows), "hits": rows})
	}))
	defer server.Close()
	paginationEnvironment(t, server.URL)
	out, notice, err := capturePaginationCommand(t, "search", "run", "--stream", "app", "--since", "1h", "--all", "--limit", "2")
	if err != nil || strings.Count(out, "\n") != 5 || notice != "" || len(requests) != 3 {
		t.Fatalf("error=%v stdout=%s stderr=%s requests=%d", err, out, notice, len(requests))
	}
	for _, req := range requests {
		if req.StartTime != requests[0].StartTime || req.EndTime != requests[0].EndTime || req.EndTime <= req.StartTime {
			t.Fatalf("--all re-resolved its window between pages: %+v", requests)
		}
	}
}
