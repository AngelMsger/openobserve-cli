package apiclient

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestNormalizeDashboardBody(t *testing.T) {
	t.Run("definition passes through", func(t *testing.T) {
		out, err := NormalizeDashboardBody(json.RawMessage(`{"title":"T","version":5,"tabs":[]}`))
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
		out, err := NormalizeDashboardBody(json.RawMessage(
			`{"v5":{"dashboardId":"d1","title":"T","version":5},"version":5,"hash":"abc123"}`))
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]json.RawMessage
		_ = json.Unmarshal(out, &m)
		if string(m["dashboardId"]) != `"d1"` {
			t.Errorf("expected the inner v5 definition, got %s", out)
		}
		if _, ok := m["hash"]; ok {
			t.Errorf("hash envelope must be dropped: %s", out)
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		if _, err := NormalizeDashboardBody(json.RawMessage(`{oops`)); err == nil {
			t.Error("expected a BAD_BODY error")
		}
	})

	t.Run("non-object", func(t *testing.T) {
		if _, err := NormalizeDashboardBody(json.RawMessage(`[1,2]`)); err == nil {
			t.Error("expected a BAD_BODY error for a non-object body")
		}
	})
}

// Values above 2^53 must survive backup/edit/restore without float64 rounding.
func TestDashboardBackupPreservesExactNumbers(t *testing.T) {
	backup := `{"v5":{"dashboardId":"d1","title":"exact","variables":{"list":[]},"created":"2026-01-01T00:00:00Z",` +
		`"epoch":9007199254740993},"version":5,"hash":"abc123","updatedAt":1760000000000000}`
	body, err := NormalizeDashboardBody(json.RawMessage(backup))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `{"dashboardId":"d1","title":"exact","variables":{"list":[]},"created":"2026-01-01T00:00:00Z","epoch":9007199254740993}` {
		t.Fatalf("definition lost precision: %s", body)
	}
	// WriteBody returns the same raw definition for library callers.
	var d Dashboard
	if err := json.Unmarshal([]byte(backup), &d); err != nil {
		t.Fatal(err)
	}
	got, err := d.WriteBody()
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(body) {
		t.Fatalf("WriteBody re-encoded the definition, want raw passthrough: %s", got)
	}
	// Bare definitions keep their numbers too (no float64 round-trip).
	out, err := NormalizeDashboardBody(json.RawMessage(`{"title":"t","epoch":9007199254740995}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"epoch":9007199254740995`) {
		t.Fatalf("bare body lost precision: %s", out)
	}
}

func TestDescribeDashboardWrite(t *testing.T) {
	// Nil transport makes any accidental I/O fail; read-only planning must work.
	c := NewReadOnly(New(Config{BaseURL: "https://example.test"}))
	cases := []struct {
		op      any
		wantURL string
	}{
		{CreateDashboardReq{Org: "org", Body: json.RawMessage(`{"title":"T","version":5}`)},
			"https://example.test/api/org/dashboards?folder=default"},
		{UpdateDashboardReq{Org: "org", ID: "d1", Folder: "ops", Hash: "abc",
			Body: json.RawMessage(`{"title":"T","version":5}`)},
			"https://example.test/api/org/dashboards/d1?folder=ops&hash=abc"},
		{DeleteDashboardReq{Org: "org", ID: "d1"},
			"https://example.test/api/org/dashboards/d1?folder=default"},
	}
	for _, tc := range cases {
		plan, err := c.DescribeWrite(context.Background(), tc.op)
		if err != nil || plan.URL != tc.wantURL {
			t.Errorf("offline plan for %T: %+v %v, want URL %q", tc.op, plan, err, tc.wantURL)
		}
	}
}

func TestDescribeDashboardWriteValidation(t *testing.T) {
	c := NewReadOnly(New(Config{BaseURL: "https://example.test"}))
	for _, op := range []any{
		DeleteDashboardReq{Org: "org"},
		UpdateDashboardReq{Org: "org", ID: "d1", Body: json.RawMessage(`null`)},
		CreateDashboardReq{Org: "org", Body: json.RawMessage(`[]`)},
		"unsupported",
	} {
		if _, err := c.DescribeWrite(context.Background(), op); err == nil {
			t.Fatalf("accepted invalid operation: %#v", op)
		}
	}
	bad := New(Config{BaseURL: "http://[invalid"})
	if _, err := bad.DescribeWrite(context.Background(), DeleteDashboardReq{Org: "org", ID: "d1"}); err == nil {
		t.Fatal("invalid base URL was hidden")
	}
}
