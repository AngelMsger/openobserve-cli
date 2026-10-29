package app

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNormalizeAlertBody(t *testing.T) {
	t.Run("definition passes through", func(t *testing.T) {
		out, err := normalizeAlertBody(json.RawMessage(
			`{"name":"cpu-high","stream_name":"app","stream_type":"logs","is_real_time":false}`))
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		_ = json.Unmarshal(out, &m)
		if m["name"] != "cpu-high" || m["stream_name"] != "app" {
			t.Errorf("unexpected body: %s", out)
		}
	})

	t.Run("server-managed fields are stripped", func(t *testing.T) {
		out, err := normalizeAlertBody(json.RawMessage(
			`{"id":"a1","name":"cpu-high","owner":"root@example.com","folder_id":"f1","folder_name":"default",` +
				`"last_triggered_at":1700000000000000,"stream_name":"app"}`))
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		_ = json.Unmarshal(out, &m)
		for _, k := range []string{"id", "owner", "folder_id", "folder_name", "last_triggered_at"} {
			if _, ok := m[k]; ok {
				t.Errorf("server-managed field %q must be dropped: %s", k, out)
			}
		}
		if m["name"] != "cpu-high" {
			t.Errorf("definition fields must survive: %s", out)
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		if _, err := normalizeAlertBody(json.RawMessage(`{oops`)); err == nil {
			t.Error("expected a BAD_BODY error")
		}
	})

	t.Run("non-object", func(t *testing.T) {
		if _, err := normalizeAlertBody(json.RawMessage(`[1,2]`)); err == nil {
			t.Error("expected a BAD_BODY error for a non-object body")
		}
	})
}

func TestReadAlertBody(t *testing.T) {
	if _, err := readAlertBody(""); err == nil {
		t.Error("expected BODY_FILE_REQUIRED for an empty --file")
	}
	if _, err := readAlertBody("/no/such/file.json"); err == nil {
		t.Error("expected FILE_READ for a missing file")
	}
	file := t.TempDir() + "/alert.json"
	if err := writeTestFile(file, `{"name":"cpu-high"}`); err != nil {
		t.Fatal(err)
	}
	body, err := readAlertBody(file)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"name"`) {
		t.Errorf("unexpected body: %s", body)
	}
	// A leading "@" is tolerated for consistency with --sql @file.
	if _, err := readAlertBody("@" + file); err != nil {
		t.Errorf("expected @path to read the file: %v", err)
	}
}

func TestAlertPlanQuery(t *testing.T) {
	q := alertPlanQuery("", "")
	if q.Get("folder") != "default" {
		t.Errorf("folder = %q, want default", q.Get("folder"))
	}
	q = alertPlanQuery("prod-alerts", "true")
	if q.Get("folder") != "prod-alerts" || q.Get("value") != "true" {
		t.Errorf("unexpected query: %s", q.Encode())
	}
}
