package apiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// OpenObserve serializes inactive Option<Dashboard> fields as null:
// https://github.com/openobserve/openobserve/blob/2f010b8de363a07963d84e3e32f373309ee2e105/src/config/src/meta/dashboards/mod.rs
const dashboardBackup = `{"v1":null,"v2":null,"v3":null,"v4":null,"v5":{"dashboardId":"d1","version":5,"title":"exact","epoch":9007199254740993},"v6":null,"v7":null,"v8":null,"version":5,"hash":"old"}`

func TestDashboardRealEnvelope(t *testing.T) {
	for _, input := range []string{dashboardBackup, `{"v1":null,"v5":{"title":"exact","epoch":9007199254740993},"hash":"old"}`} {
		body, err := NormalizeDashboardBody(json.RawMessage(input))
		if err != nil || !strings.Contains(string(body), `"epoch":9007199254740993`) {
			t.Fatalf("lost active definition: %s %v", body, err)
		}
		var d Dashboard
		if err := json.Unmarshal([]byte(input), &d); err != nil {
			t.Fatal(err)
		}
		fromModel, err := d.WriteBody()
		if err != nil || string(body) != string(fromModel) {
			t.Fatalf("model and import disagree: %s %v", fromModel, err)
		}
	}
	for _, input := range []string{
		`{"version":5,"hash":"h","v1":{},"v5":null}`,
		`{"version":5,"hash":"h","v5":[]}`,
		`{"hash":"h","v1":{},"v5":{}}`,
		`{"version":5,"hash":"h"}`,
		`{"version":5,"hash":"h","v5":"text"}`,
	} {
		if _, err := NormalizeDashboardBody(json.RawMessage(input)); err == nil {
			t.Fatalf("accepted broken envelope: %s", input)
		}
	}
}

func TestDashboardModelReuseClearsMetadata(t *testing.T) {
	var d Dashboard
	if err := json.Unmarshal([]byte(dashboardBackup), &d); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(`{"v5":{"title":"new"},"version":5}`), &d); err != nil {
		t.Fatal(err)
	}
	if d.Hash != "" || d.DashboardID != "" || d.Title != "new" {
		t.Fatalf("retained stale metadata: %+v", d)
	}
}

func TestDescribeDashboardPlans(t *testing.T) {
	c := NewReadOnly(New(Config{BaseURL: "https://example.test"}))
	for _, tc := range []struct {
		body, hash, wantHash string
		prefetch             bool
	}{
		{dashboardBackup, "", "old", false},
		{dashboardBackup, "explicit", "explicit", false},
		{`{"version":5,"title":"bare"}`, "", "", true},
	} {
		op := UpdateDashboardReq{Org: "org", ID: "d1", Folder: "ops", Body: json.RawMessage(tc.body), Hash: tc.hash}
		plan, err := c.DescribeWrite(context.Background(), op)
		if err != nil {
			t.Fatal(err)
		}
		expected := "https://example.test/api/org/dashboards/d1?folder=ops"
		if tc.wantHash != "" {
			expected += "&hash=" + tc.wantHash
		}
		if plan.URL != expected || (plan.Prefetch != nil) != tc.prefetch {
			t.Fatalf("bad plan: %+v", plan)
		}
		if plan.Prefetch != nil && (plan.Prefetch.Method != "GET" || plan.Prefetch.URL != expected) {
			t.Fatalf("bad prefetch: %+v", plan.Prefetch)
		}
		compatible, err := PlanDashboardWrite("https://example.test", op)
		if err != nil {
			t.Fatal(err)
		}
		a, _ := json.Marshal(plan)
		b, _ := json.Marshal(compatible)
		if string(a) != string(b) {
			t.Fatal("legacy planning helper drifted")
		}
	}
}

func TestDashboardMissingHashPreventsWrite(t *testing.T) {
	requests := 0
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != "GET" {
			t.Errorf("unexpected mutation: %s", r.Method)
		}
		_, _ = w.Write([]byte(`{"version":5,"v5":{"title":"missing hash"}}`))
	})
	_, err := client.UpdateDashboard(context.Background(), "org", "d1", "", "", json.RawMessage(`{"version":5}`))
	if err == nil || requests != 1 {
		t.Fatalf("expected missing-hash failure after one GET: %d %v", requests, err)
	}
}

func TestDashboardListPageSizeContract(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("pageSize") != "2" || r.URL.Query().Has("page_size") {
			t.Errorf("wrong query: %s", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"dashboards":[]}`))
	})
	if _, err := client.ListDashboards(context.Background(), "org", DashboardListOptions{Title: "ops", PageSize: 2}); err != nil {
		t.Fatal(err)
	}
}
