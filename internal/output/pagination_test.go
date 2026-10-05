package output

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestNDJSONPagination(t *testing.T) {
	for _, tc := range []struct {
		name string
		rows []map[string]any
	}{
		{"projected page", []map[string]any{{"name": "a", "private": "drop"}}},
		{"filtered empty page", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var data, notices bytes.Buffer
			err := EmitList(tc.rows, "opaque-next", true, Options{
				Format: FormatNDJSON, Writer: &data, NoticeWriter: &notices, Fields: []string{"name"},
			})
			if err != nil {
				t.Fatal(err)
			}
			wantRows := ""
			if len(tc.rows) > 0 {
				wantRows = "{\"name\":\"a\"}\n"
			}
			if data.String() != wantRows {
				t.Fatalf("stdout = %q, want %q", data.String(), wantRows)
			}
			var notice struct {
				Notice struct {
					Pagination struct {
						Next    string `json:"next"`
						HasMore bool   `json:"has_more"`
					} `json:"pagination"`
					NextSteps []string `json:"next_steps"`
				} `json:"_notice"`
			}
			if err := json.Unmarshal(notices.Bytes(), &notice); err != nil {
				t.Fatalf("invalid notice: %v: %s", err, notices.String())
			}
			if notice.Notice.Pagination.Next != "opaque-next" || !notice.Notice.Pagination.HasMore ||
				len(notice.Notice.NextSteps) != 1 || !strings.Contains(notice.Notice.NextSteps[0], "--cursor") {
				t.Fatalf("missing pagination: %s", notices.String())
			}
		})
	}
}

func TestPaginationCompletedPage(t *testing.T) {
	for _, rows := range [][]map[string]any{nil, {{"name": "done"}}} {
		var data, notices bytes.Buffer
		if err := EmitList(rows, "", false, Options{Format: FormatNDJSON, Writer: &data, NoticeWriter: &notices}); err != nil {
			t.Fatal(err)
		}
		if notices.Len() != 0 {
			t.Fatalf("completed page has a notice: %s", notices.String())
		}
	}
}

type failedPaginationWriter struct{}

func (failedPaginationWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestPaginationWriteFailures(t *testing.T) {
	rows := []map[string]any{{"name": "a"}}
	var notices bytes.Buffer
	err := EmitList(rows, "next", true, Options{Format: FormatNDJSON, Writer: failedPaginationWriter{}, NoticeWriter: &notices})
	if !errors.Is(err, io.ErrClosedPipe) || notices.Len() != 0 {
		t.Fatalf("failed rows must not announce continuation: error=%v notice=%s", err, notices.String())
	}
	var data bytes.Buffer
	err = EmitList(rows, "next", true, Options{Format: FormatNDJSON, Writer: &data, NoticeWriter: failedPaginationWriter{}})
	if err != nil || data.String() != "{\"name\":\"a\"}\n" {
		t.Fatalf("notice failure changed successful rows: error=%v stdout=%s", err, data.String())
	}
}

func TestPaginationContinuationFlag(t *testing.T) {
	for _, format := range []string{FormatNDJSON, FormatTable, FormatJSON} {
		t.Run(format, func(t *testing.T) {
			var data, notices bytes.Buffer
			if err := EmitList([]map[string]any{{"name": "a"}}, "20", true, Options{
				Format: format, Writer: &data, NoticeWriter: &notices, NextFlag: "--offset",
			}); err != nil {
				t.Fatal(err)
			}
			switch format {
			case FormatNDJSON:
				if !strings.Contains(notices.String(), "Pass next as --offset") || strings.Contains(data.String(), "20") {
					t.Fatalf("wrong NDJSON channels: stdout=%s stderr=%s", data.String(), notices.String())
				}
			case FormatTable:
				if !strings.Contains(data.String(), "--offset 20") || strings.Contains(data.String(), "--cursor") || notices.Len() != 0 {
					t.Fatalf("wrong table footer: stdout=%s stderr=%s", data.String(), notices.String())
				}
			case FormatJSON:
				var env map[string]any
				if err := json.Unmarshal(data.Bytes(), &env); err != nil {
					t.Fatal(err)
				}
				if env["next"] != "20" || env["has_more"] != true || len(env) != 3 || notices.Len() != 0 {
					t.Fatalf("JSON contract changed: stdout=%s stderr=%s", data.String(), notices.String())
				}
			}
		})
	}
}
