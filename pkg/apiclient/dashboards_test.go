package apiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	cerrors "github.com/angelmsger/openobserve-cli/pkg/errors"
)

func TestListDashboards(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/acme/dashboards" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if got := r.URL.Query().Get("folder"); got != "ops" {
			t.Errorf("folder = %q, want ops", got)
		}
		if got := r.URL.Query().Get("title"); got != "traffic" {
			t.Errorf("title = %q, want traffic", got)
		}
		_, _ = w.Write([]byte(`{"dashboards":[{
			"dashboard_id":"dash1","title":"Ops Overview","description":"ops",
			"folder_id":"default","folder_name":"default","role":"","owner":"ops@example.com",
			"version":5,"hash":"abc123","created":"2026-01-01T00:00:00Z","updatedAt":1700000000000000,
			"v5":{"dashboardId":"dash1","title":"Ops Overview","version":5,"tabs":[]}
		}]}`))
	})
	summaries, err := client.ListDashboards(context.Background(), "acme",
		DashboardListOptions{Folder: "ops", Title: "traffic"})
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(summaries))
	}
	got := summaries[0]
	if got.DashboardID != "dash1" || got.Title != "Ops Overview" || got.FolderName != "default" ||
		got.Version != 5 || got.Hash != "abc123" {
		t.Errorf("unexpected summary: %+v", got)
	}
	out, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	_ = json.Unmarshal(out, &m)
	if _, ok := m["v5"]; ok {
		t.Errorf("summary output must drop the embedded definition: %s", out)
	}
	if m["dashboard_id"] != "dash1" || m["hash"] != "abc123" {
		t.Errorf("summary output missing identifiers: %s", out)
	}
}

func TestGetDashboardExtractsHashAndBody(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/acme/dashboards/d1" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if got := r.URL.Query().Get("folder"); got != "default" {
			t.Errorf("folder = %q, want default", got)
		}
		_, _ = w.Write([]byte(`{
			"v5":{"dashboardId":"d1","title":"Traffic","version":5,"tabs":[]},
			"version":5,"hash":"abc123","updatedAt":1700000000000000
		}`))
	})
	d, err := client.GetDashboard(context.Background(), "acme", "d1", "")
	if err != nil {
		t.Fatal(err)
	}
	if d.Hash != "abc123" || d.Version != 5 || d.DashboardID != "d1" || d.Title != "Traffic" {
		t.Errorf("unexpected dashboard: %+v", d)
	}
	body, err := d.WriteBody()
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatal(err)
	}
	if m["dashboardId"] != "d1" {
		t.Errorf("write body should be the inner v5 definition, got %s", body)
	}
	// The raw envelope round-trips unchanged for `dashboard get` output.
	raw, _ := json.Marshal(d)
	if !strings.Contains(string(raw), `"hash":"abc123"`) {
		t.Errorf("get output lost the hash: %s", raw)
	}
}

func TestDashboardWriteBodyFallback(t *testing.T) {
	var d Dashboard
	if err := json.Unmarshal([]byte(`{"v8":{"dashboardId":"x","title":"Y","version":8},"version":8,"hash":"h"}`), &d); err != nil {
		t.Fatal(err)
	}
	body, err := d.WriteBody()
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	_ = json.Unmarshal(body, &m)
	if m["dashboardId"] != "x" {
		t.Errorf("expected v8 body, got %s", body)
	}
	var bare Dashboard
	if err := json.Unmarshal([]byte(`{"version":5,"hash":"h"}`), &bare); err != nil {
		t.Fatal(err)
	}
	if _, err := bare.WriteBody(); err == nil {
		t.Error("expected an error when no version-keyed body exists")
	}
}

func TestCreateDashboardSendsFolderAndBody(t *testing.T) {
	var gotBody string
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		if got := r.URL.Query().Get("folder"); got != "default" {
			t.Errorf("folder = %q, want default", got)
		}
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		gotBody = string(buf)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"v5":{"dashboardId":"d1","title":"T","version":5},"version":5,"hash":"h1"}`))
	})
	d, err := client.CreateDashboard(context.Background(), "default", "",
		json.RawMessage(`{"title":"T","version":5,"tabs":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(gotBody, `{"title":"T"`) {
		t.Errorf("body was not passed through raw, got %s", gotBody)
	}
	if d.DashboardID != "d1" || d.Hash != "h1" {
		t.Errorf("unexpected create result: %+v", d)
	}
}

func TestUpdateDashboardSendsHashAndMapsConflict(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %q, want PUT", r.Method)
		}
		if got := r.URL.Query().Get("hash"); got != "abc123" {
			t.Errorf("hash = %q, want abc123", got)
		}
		if got := r.URL.Query().Get("folder"); got != "ops" {
			t.Errorf("folder = %q, want ops", got)
		}
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"message":"concurrent dashboard update"}`))
	})
	_, err := client.UpdateDashboard(context.Background(), "default", "d1", "ops", "abc123",
		json.RawMessage(`{"title":"T2","version":5}`))
	var ce *cerrors.CLIError
	if err == nil {
		t.Fatal("expected a conflict error")
	}
	ce = cerrors.AsCLIError(err)
	if ce.Category != cerrors.CategoryConflict {
		t.Fatalf("category = %q, want conflict", ce.Category)
	}
	if !strings.Contains(ce.Hint, "hash") {
		t.Errorf("conflict hint should mention the hash, got %q", ce.Hint)
	}
}

func TestDeleteDashboard(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %q, want DELETE", r.Method)
		}
		_, _ = w.Write([]byte(`{"message":"Dashboard deleted"}`))
	})
	if err := client.DeleteDashboard(context.Background(), "default", "d1", ""); err != nil {
		t.Fatal(err)
	}
}

func TestDashboardGetNotFoundGuidance(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Dashboard not found"}`))
	})
	_, err := client.GetDashboard(context.Background(), "default", "missing", "")
	if err == nil {
		t.Fatal("expected a not-found error")
	}
	ce := cerrors.AsCLIError(err)
	if ce.Category != cerrors.CategoryNotFound {
		t.Fatalf("category = %q, want not_found", ce.Category)
	}
	joined := strings.Join(ce.NextSteps, "|")
	if !strings.Contains(joined, "dashboard list") {
		t.Errorf("not-found guidance should point at dashboard list, got %q", joined)
	}
}

func TestReadOnlyBlocksDashboardWrites(t *testing.T) {
	base, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("no request may leave the process for a write, got %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"dashboards":[]}`))
	})
	ro := NewReadOnly(base)

	// Reads pass through.
	if _, err := ro.ListDashboards(context.Background(), "default", DashboardListOptions{}); err != nil {
		t.Fatalf("reads should pass through the read-only wrapper: %v", err)
	}
	for name, call := range map[string]func() error{
		"create": func() error {
			_, err := ro.CreateDashboard(context.Background(), "default", "", json.RawMessage(`{}`))
			return err
		},
		"update": func() error {
			_, err := ro.UpdateDashboard(context.Background(), "default", "d1", "", "h", json.RawMessage(`{}`))
			return err
		},
		"delete": func() error {
			return ro.DeleteDashboard(context.Background(), "default", "d1", "")
		},
	} {
		err := call()
		if err == nil {
			t.Fatalf("%s should be blocked", name)
		}
		ce := cerrors.AsCLIError(err)
		if ce.Category != cerrors.CategoryPermission || ce.Code != "READONLY_BLOCKED" {
			t.Errorf("%s: %s blocked with %q/%q, want permission/READONLY_BLOCKED", name, name, ce.Category, ce.Code)
		}
	}
}
