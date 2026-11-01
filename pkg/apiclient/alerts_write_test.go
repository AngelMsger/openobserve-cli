package apiclient

import (
	"context"
	"encoding/json"
	"testing"
)

func TestNormalizeAlertBody(t *testing.T) {
	t.Run("definition passes through", func(t *testing.T) {
		out, err := NormalizeAlertBody(json.RawMessage(
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
		out, err := NormalizeAlertBody(json.RawMessage(
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
		if _, err := NormalizeAlertBody(json.RawMessage(`{oops`)); err == nil {
			t.Error("expected a BAD_BODY error")
		}
	})

	t.Run("non-object", func(t *testing.T) {
		if _, err := NormalizeAlertBody(json.RawMessage(`[1,2]`)); err == nil {
			t.Error("expected a BAD_BODY error for a non-object body")
		}
	})
}

// Values above 2^53 must survive get/edit/restore without float64 rounding.
func TestAlertBackupPreservesExactNumbers(t *testing.T) {
	var a Alert
	if err := json.Unmarshal([]byte(`{"id":"a1","name":"exact","last_edited_by":"someone","query_condition":{"threshold":9007199254740993},"future_field":{"value":9007199254740995}}`), &a); err != nil {
		t.Fatal(err)
	}
	backup, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	body, err := NormalizeAlertBody(backup)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if string(got["query_condition"]) != `{"threshold":9007199254740993}` || string(got["future_field"]) != `{"value":9007199254740995}` {
		t.Fatalf("definition lost precision: %s", body)
	}
	for _, key := range []string{"id", "last_edited_by"} {
		if _, ok := got[key]; ok {
			t.Fatalf("response metadata leaked: %s", body)
		}
	}
}

func TestDescribeAlertWriteValidation(t *testing.T) {
	// Nil transport makes any accidental I/O fail; read-only planning must work.
	c := NewReadOnly(New(Config{BaseURL: "https://example.test"}))
	plan, err := c.DescribeWrite(context.Background(), DeleteAlertReq{Org: "org", ID: "a1"})
	if err != nil || plan.URL != "https://example.test/api/v2/org/alerts/a1?folder=default" {
		t.Fatalf("offline plan: %+v %v", plan, err)
	}
	for _, op := range []any{
		DeleteAlertReq{Org: "org"},
		UpdateAlertReq{Org: "org", ID: "a1", Body: json.RawMessage(`null`)},
		CreateAlertReq{Org: "org", Body: json.RawMessage(`[]`)},
		"unsupported",
	} {
		if _, err := c.DescribeWrite(context.Background(), op); err == nil {
			t.Fatalf("accepted invalid operation: %#v", op)
		}
	}
	bad := New(Config{BaseURL: "http://[invalid"})
	if _, err := bad.DescribeWrite(context.Background(), DeleteAlertReq{Org: "org", ID: "a1"}); err == nil {
		t.Fatal("invalid base URL was hidden")
	}
}
