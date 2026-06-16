package output

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestEmitListEnvelope(t *testing.T) {
	var buf bytes.Buffer
	items := []map[string]any{{"name": "app"}, {"name": "web"}}
	if err := EmitList(items, "", false, Options{Format: FormatJSON, Writer: &buf}); err != nil {
		t.Fatal(err)
	}
	var env struct {
		Items   []map[string]any `json:"items"`
		HasMore bool             `json:"has_more"`
	}
	if err := json.Unmarshal(buf.Bytes(), &env); err != nil {
		t.Fatalf("output is not the list envelope: %v\n%s", err, buf.String())
	}
	if len(env.Items) != 2 || env.HasMore {
		t.Errorf("unexpected envelope: %+v", env)
	}
}

func TestFieldProjection(t *testing.T) {
	var buf bytes.Buffer
	v := map[string]any{"name": "app", "secret": "x", "nested": map[string]any{"keep": 1}}
	if err := Emit(v, Options{Format: FormatJSON, Writer: &buf, Fields: []string{"name", "nested.keep"}}); err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if _, ok := out["secret"]; ok {
		t.Error("projection should have dropped 'secret'")
	}
	if out["name"] != "app" {
		t.Errorf("projection dropped 'name': %+v", out)
	}
	if _, ok := out["nested.keep"]; !ok {
		t.Errorf("projection missing flattened dot-path: %+v", out)
	}
}

func TestNDJSONStreamsRows(t *testing.T) {
	var buf bytes.Buffer
	items := []map[string]any{{"a": 1}, {"a": 2}}
	if err := EmitList(items, "", false, Options{Format: FormatNDJSON, Writer: &buf}); err != nil {
		t.Fatal(err)
	}
	lines := bytes.Count(buf.Bytes(), []byte("\n"))
	if lines != 2 {
		t.Errorf("ndjson should emit one line per row, got %d lines:\n%s", lines, buf.String())
	}
}

func TestBadFormat(t *testing.T) {
	var buf bytes.Buffer
	if err := Emit(map[string]any{}, Options{Format: "yaml", Writer: &buf}); err == nil {
		t.Error("expected error for unknown format")
	}
}
