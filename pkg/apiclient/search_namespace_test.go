package apiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	cerrors "github.com/angelmsger/openobserve-cli/pkg/errors"
)

// OpenObserve selects the namespace from ?type=, independently of the JSON
// search_type (the UI/API query origin). An absent type defaults to logs.
func TestSearchStreamNamespace(t *testing.T) {
	for _, streamType := range []string{"", "logs", "metrics", "traces"} {
		t.Run("type="+streamType, func(t *testing.T) {
			client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("type") != streamType {
					t.Errorf("query = %s, want type %q", r.URL.RawQuery, streamType)
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if _, ok := body["stream_type"]; ok {
					t.Error("stream type leaked into JSON body")
				}
				if body["search_type"] != "ui" {
					t.Errorf("search_type changed: %v", body)
				}
				_, _ = w.Write([]byte(`{"hits":[],"total":0}`))
			})
			_, err := client.Search(context.Background(), "default", SearchRequest{
				StreamType: streamType, Query: SearchQuery{SQL: `SELECT * FROM "shared"`, Size: 10},
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestStreamInspectionRequiresUnambiguousType(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("type") == "traces" {
			_, _ = w.Write([]byte(`{"list":[{"name":"shared","stream_type":"traces"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"list":[{"name":"shared","stream_type":"logs"},{"name":"shared","stream_type":"traces"}]}`))
	})
	_, err := client.GetStream(context.Background(), "default", "shared", "")
	if ce := cerrors.AsCLIError(err); err == nil || ce.Code != "STREAM_AMBIGUOUS" || ce.Category != cerrors.CategoryUsage {
		t.Fatalf("ambiguous lookup: %v", err)
	}
	stream, err := client.GetStream(context.Background(), "default", "shared", "traces")
	if err != nil || stream.StreamType != "traces" {
		t.Fatalf("typed lookup: %+v / %v", stream, err)
	}
}
