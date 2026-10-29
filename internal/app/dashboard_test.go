package app

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNormalizeDashboardBody(t *testing.T) {
	t.Run("plain body passes through", func(t *testing.T) {
		out, err := normalizeDashboardBody(json.RawMessage(`{"title":"T","version":5,"tabs":[]}`))
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		_ = json.Unmarshal(out, &m)
		if m["title"] != "T" {
			t.Errorf("unexpected body: %s", out)
		}
	})

	t.Run("get envelope is unwrapped", func(t *testing.T) {
		out, err := normalizeDashboardBody(json.RawMessage(
			`{"v5":{"dashboardId":"d1","title":"T","version":5},"version":5,"hash":"abc123"}`))
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		_ = json.Unmarshal(out, &m)
		if m["dashboardId"] != "d1" {
			t.Errorf("expected the inner v5 definition, got %s", out)
		}
		if _, ok := m["hash"]; ok {
			t.Errorf("hash envelope must be dropped: %s", out)
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		if _, err := normalizeDashboardBody(json.RawMessage(`{oops`)); err == nil {
			t.Error("expected a BAD_BODY error")
		}
	})

	t.Run("non-object", func(t *testing.T) {
		if _, err := normalizeDashboardBody(json.RawMessage(`[1,2]`)); err == nil {
			t.Error("expected a BAD_BODY error for a non-object body")
		}
	})
}

func TestReadDashboardBody(t *testing.T) {
	if _, err := readDashboardBody(""); err == nil {
		t.Error("expected BODY_FILE_REQUIRED for an empty --file")
	}
	if _, err := readDashboardBody("/no/such/file.json"); err == nil {
		t.Error("expected FILE_READ for a missing file")
	}
	file := t.TempDir() + "/dash.json"
	if err := writeTestFile(file, `{"title":"T"}`); err != nil {
		t.Fatal(err)
	}
	body, err := readDashboardBody(file)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"title"`) {
		t.Errorf("unexpected body: %s", body)
	}
	// A leading "@" is tolerated for consistency with --sql @file.
	if _, err := readDashboardBody("@" + file); err != nil {
		t.Errorf("expected @path to read the file: %v", err)
	}
}

func TestDashboardPlanQuery(t *testing.T) {
	q := dashboardPlanQuery("", "")
	if q.Get("folder") != "default" {
		t.Errorf("folder = %q, want default", q.Get("folder"))
	}
	q = dashboardPlanQuery("ops", "h1")
	if q.Get("folder") != "ops" || q.Get("hash") != "h1" {
		t.Errorf("unexpected query: %s", q.Encode())
	}
}
