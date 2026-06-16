package apiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/angelmsger/openobserve-cli/internal/transport"
)

// newTestClient wires a Client to an httptest server, recording the last seen
// Authorization header so auth wiring can be asserted.
func newTestClient(t *testing.T, h http.HandlerFunc) (Client, *string) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	var gotAuth string
	dec := func(req *http.Request) { req.Header.Set("Authorization", "Basic test") }
	tc := transport.New(transport.Options{Decorators: []transport.Decorator{
		dec,
		func(req *http.Request) { gotAuth = req.Header.Get("Authorization") },
	}})
	return New(Config{BaseURL: srv.URL, Org: "default", Transport: tc}), &gotAuth
}

func TestListOrgs(t *testing.T) {
	client, auth := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/organizations" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"data":[{"identifier":"default","name":"Default"},{"identifier":"team-a"}]}`))
	})
	orgs, err := client.ListOrgs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(orgs) != 2 || orgs[0].Identifier != "default" || orgs[1].Identifier != "team-a" {
		t.Errorf("unexpected orgs: %+v", orgs)
	}
	if *auth != "Basic test" {
		t.Errorf("auth header not applied, got %q", *auth)
	}
}

// Regression: real self-hosted builds return an org object with many extra
// fields and types that drift across versions (e.g. `plan` as a number, not a
// string). Decoding must not fail, and Identifier must still be extracted.
func TestListOrgsToleratesRichPayload(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":1,"identifier":"default","name":"default",` +
			`"user_email":"a@b.com","ingest_threshold":9383939382,"search_threshold":9383939382,` +
			`"type":"default","plan":0,"UserObj":{"first_name":"","last_name":""}}]}`))
	})
	orgs, err := client.ListOrgs(context.Background())
	if err != nil {
		t.Fatalf("rich org payload should decode, got: %v", err)
	}
	if len(orgs) != 1 || orgs[0].Identifier != "default" || orgs[0].Name != "default" {
		t.Errorf("unexpected orgs: %+v", orgs)
	}
	// Rendered output keeps only the curated keys, dropping noise like UserObj.
	out, _ := json.Marshal(orgs[0])
	var m map[string]any
	_ = json.Unmarshal(out, &m)
	if _, ok := m["UserObj"]; ok {
		t.Errorf("curated output should drop UserObj: %s", out)
	}
	if m["identifier"] != "default" {
		t.Errorf("curated output missing identifier: %s", out)
	}
}

func TestListStreamsParams(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/default/streams" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if got := r.URL.Query().Get("type"); got != "logs" {
			t.Errorf("type param = %q, want logs", got)
		}
		if got := r.URL.Query().Get("fetchSchema"); got != "true" {
			t.Errorf("fetchSchema param = %q, want true", got)
		}
		_, _ = w.Write([]byte(`{"list":[{"name":"app","stream_type":"logs","schema":[{"name":"level","type":"Utf8"}]}]}`))
	})
	streams, err := client.ListStreams(context.Background(), "default", "logs", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(streams) != 1 || streams[0].Name != "app" || len(streams[0].Schema) != 1 {
		t.Errorf("unexpected streams: %+v", streams)
	}
}

func TestGetStreamFilterAndNotFound(t *testing.T) {
	h := func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"list":[{"name":"app","stream_type":"logs"},{"name":"web","stream_type":"logs"}]}`))
	}
	client, _ := newTestClient(t, h)
	st, err := client.GetStream(context.Background(), "default", "web", "")
	if err != nil {
		t.Fatal(err)
	}
	if st.Name != "web" {
		t.Errorf("got %q, want web", st.Name)
	}
	if _, err := client.GetStream(context.Background(), "default", "missing", ""); err == nil {
		t.Error("expected not-found error for missing stream")
	}
}

func TestSearchRequestAndResponse(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/default/_search" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		var body SearchRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body.Query.SQL == "" || body.Query.StartTime <= 0 || body.Query.EndTime <= body.Query.StartTime {
			t.Errorf("bad query block: %+v", body.Query)
		}
		if body.SearchType != "ui" {
			t.Errorf("search_type = %q, want ui (defaulted)", body.SearchType)
		}
		_, _ = w.Write([]byte(`{"took":7,"total":1,"scan_size":1.5,"hits":[{"_timestamp":1700000000000000,"level":"ERROR"}]}`))
	})
	resp, err := client.Search(context.Background(), "default", SearchRequest{
		Query: SearchQuery{SQL: `SELECT * FROM "app"`, StartTime: 1, EndTime: 2, Size: 10},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Took != 7 || resp.Total != 1 || len(resp.Hits) != 1 {
		t.Errorf("unexpected response: %+v", resp)
	}
	if resp.Hits[0]["level"] != "ERROR" {
		t.Errorf("unexpected hit: %+v", resp.Hits[0])
	}
}

func TestHTTPErrorClassification(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"stream not found"}`))
	})
	_, err := client.ListStreams(context.Background(), "default", "", false)
	if err == nil {
		t.Fatal("expected error for 404")
	}
	if got := err.Error(); got == "" {
		t.Error("error message should be populated")
	}
}
