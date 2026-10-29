package apiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	cerrors "github.com/angelmsger/openobserve-cli/pkg/errors"
)

func TestListAlerts(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/acme/alerts" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if got := r.URL.Query().Get("folder"); got != "prod" {
			t.Errorf("folder = %q, want prod", got)
		}
		_, _ = w.Write([]byte(`{"list":[{
			"id":"a1","name":"cpu-high","description":"cpu over 90%",
			"stream_type":"metrics","stream_name":"node","is_real_time":false,
			"enabled":true,"owner":"ops@example.com","last_triggered_at":1700000000000000,
			"folder_id":"prod","folder_name":"prod",
			"query_condition":{"type":"promql","conditions":[]},
			"trigger_condition":{"period":10,"threshold":1}
		}]}`))
	})
	summaries, err := client.ListAlerts(context.Background(), "acme",
		AlertListOptions{Folder: "prod"})
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(summaries))
	}
	got := summaries[0]
	if got.AlertID != "a1" || got.Name != "cpu-high" || got.StreamName != "node" ||
		got.StreamType != "metrics" || !got.Enabled || got.FolderID != "prod" {
		t.Errorf("unexpected summary: %+v", got)
	}
	out, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	_ = json.Unmarshal(out, &m)
	if _, ok := m["query_condition"]; ok {
		t.Errorf("summary output must drop the embedded conditions: %s", out)
	}
	if m["alert_id"] != "a1" || m["name"] != "cpu-high" {
		t.Errorf("summary output missing identifiers: %s", out)
	}
}

func TestGetAlert(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/acme/alerts/a1" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if got := r.URL.Query().Get("folder"); got != "default" {
			t.Errorf("folder = %q, want default", got)
		}
		_, _ = w.Write([]byte(`{
			"id":"a1","name":"cpu-high","stream_name":"node","folder_id":"default",
			"query_condition":{"type":"promql"},"trigger_condition":{"threshold":1}
		}`))
	})
	a, err := client.GetAlert(context.Background(), "acme", "a1", "")
	if err != nil {
		t.Fatal(err)
	}
	if a.AlertID != "a1" || a.Name != "cpu-high" || a.StreamName != "node" {
		t.Errorf("unexpected alert: %+v", a)
	}
	// The raw envelope round-trips unchanged for `alert get` output.
	raw, _ := json.Marshal(a)
	if !strings.Contains(string(raw), `"trigger_condition"`) {
		t.Errorf("get output lost the definition: %s", raw)
	}
}

func TestCreateAlertStripsAndSendsBody(t *testing.T) {
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
		_, _ = w.Write([]byte(`{"id":"a1","name":"cpu-high","stream_name":"node"}`))
	})
	a, err := client.CreateAlert(context.Background(), "default", "",
		json.RawMessage(`{"name":"cpu-high","stream_name":"node","stream_type":"metrics"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(gotBody, `{"name":"cpu-high"`) {
		t.Errorf("body was not passed through raw, got %s", gotBody)
	}
	if a.AlertID != "a1" || a.Name != "cpu-high" {
		t.Errorf("unexpected create result: %+v", a)
	}
}

func TestUpdateAlert(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %q, want PUT", r.Method)
		}
		if got := r.URL.Query().Get("folder"); got != "prod" {
			t.Errorf("folder = %q, want prod", got)
		}
		_, _ = w.Write([]byte(`{"id":"a1","name":"cpu-high","stream_name":"node"}`))
	})
	a, err := client.UpdateAlert(context.Background(), "default", "a1", "prod",
		json.RawMessage(`{"name":"cpu-high","stream_name":"node","stream_type":"metrics"}`))
	if err != nil {
		t.Fatal(err)
	}
	if a.AlertID != "a1" {
		t.Errorf("unexpected update result: %+v", a)
	}
}

func TestDeleteAlert(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %q, want DELETE", r.Method)
		}
		_, _ = w.Write([]byte(`{"message":"Alert deleted"}`))
	})
	if err := client.DeleteAlert(context.Background(), "default", "a1", ""); err != nil {
		t.Fatal(err)
	}
}

func TestSetAlertEnabled(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("method = %q, want PATCH", r.Method)
		}
		if r.URL.Path != "/api/default/alerts/a1/enabled" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if got := r.URL.Query().Get("value"); got != "false" {
			t.Errorf("value = %q, want false", got)
		}
		_, _ = w.Write([]byte(`{"id":"a1","enabled":false}`))
	})
	if err := client.SetAlertEnabled(context.Background(), "default", "a1", "", false); err != nil {
		t.Fatal(err)
	}
}

func TestAlertGetNotFoundGuidance(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Alert not found"}`))
	})
	_, err := client.GetAlert(context.Background(), "default", "missing", "")
	if err == nil {
		t.Fatal("expected a not-found error")
	}
	ce := cerrors.AsCLIError(err)
	if ce.Category != cerrors.CategoryNotFound {
		t.Fatalf("category = %q, want not_found", ce.Category)
	}
	joined := strings.Join(ce.NextSteps, "|")
	if !strings.Contains(joined, "alert list") {
		t.Errorf("not-found guidance should point at alert list, got %q", joined)
	}
}

func TestReadOnlyBlocksAlertWrites(t *testing.T) {
	base, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("no request may leave the process for a write, got %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"list":[]}`))
	})
	ro := NewReadOnly(base)

	// Reads pass through.
	if _, err := ro.ListAlerts(context.Background(), "default", AlertListOptions{}); err != nil {
		t.Fatalf("reads should pass through the read-only wrapper: %v", err)
	}
	for name, call := range map[string]func() error{
		"create": func() error {
			_, err := ro.CreateAlert(context.Background(), "default", "", json.RawMessage(`{}`))
			return err
		},
		"update": func() error {
			_, err := ro.UpdateAlert(context.Background(), "default", "a1", "", json.RawMessage(`{}`))
			return err
		},
		"delete": func() error {
			return ro.DeleteAlert(context.Background(), "default", "a1", "")
		},
		"enable": func() error {
			return ro.SetAlertEnabled(context.Background(), "default", "a1", "", true)
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
