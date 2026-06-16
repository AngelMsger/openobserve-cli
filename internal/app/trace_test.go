package app

import "testing"

// sampleSpans is a three-span trace: a (root) → b → c, plus distinct services.
func sampleSpans() []map[string]any {
	return []map[string]any{
		{"trace_id": "t1", "span_id": "a", "service_name": "web", "operation_name": "GET /",
			"span_status": "OK", "start_time": float64(1_000), "end_time": float64(1_500), "duration": float64(500)},
		{"trace_id": "t1", "span_id": "b", "reference_parent_span_id": "a", "service_name": "api",
			"operation_name": "query", "start_time": float64(1_100), "end_time": float64(1_300), "duration": float64(200)},
		{"trace_id": "t1", "span_id": "c", "reference_parent_span_id": "b", "service_name": "db",
			"operation_name": "SELECT", "start_time": float64(1_150), "end_time": float64(1_250), "duration": float64(100)},
	}
}

func TestAssembleTraceTree(t *testing.T) {
	tree := assembleTrace("t1", sampleSpans())

	if got := tree.summary["span_count"]; got != 3 {
		t.Fatalf("span_count = %v, want 3", got)
	}
	if got := tree.summary["duration_micros"]; got != int64(500) {
		t.Fatalf("duration_micros = %v, want 500", got)
	}
	services, _ := tree.summary["services"].([]string)
	if len(services) != 3 || services[0] != "api" { // sorted
		t.Fatalf("services = %v, want sorted [api db web]", services)
	}

	roots, _ := tree.summary["spans"].([]any)
	if len(roots) != 1 {
		t.Fatalf("want exactly one root span, got %d", len(roots))
	}
	root, _ := roots[0].(map[string]any)
	if root["span_id"] != "a" {
		t.Fatalf("root span_id = %v, want a", root["span_id"])
	}
	if root["offset_micros"] != int64(0) {
		t.Fatalf("root offset = %v, want 0", root["offset_micros"])
	}
	kids, _ := root["children"].([]any)
	if len(kids) != 1 {
		t.Fatalf("root should have one child, got %d", len(kids))
	}
	b, _ := kids[0].(map[string]any)
	if b["span_id"] != "b" || b["offset_micros"] != int64(100) {
		t.Fatalf("child b = %v, want span_id b offset 100", b)
	}
}

func TestAssembleTraceFlatAndOrphan(t *testing.T) {
	// A span whose parent is missing from the set must surface as a root, not be
	// dropped — otherwise spans vanish from the waterfall.
	spans := append(sampleSpans(), map[string]any{
		"trace_id": "t1", "span_id": "d", "reference_parent_span_id": "ghost",
		"service_name": "cache", "start_time": float64(1_120),
	})
	tree := assembleTrace("t1", spans)

	if len(tree.flatSpans) != 4 {
		t.Fatalf("flatSpans = %d, want 4", len(tree.flatSpans))
	}
	roots, _ := tree.summary["spans"].([]any)
	if len(roots) != 2 {
		t.Fatalf("want two roots (a and orphan d), got %d", len(roots))
	}
}
