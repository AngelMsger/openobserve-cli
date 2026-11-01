package apiclient

import (
	"context"
	"net/http"
	"testing"

	cerrors "github.com/angelmsger/openobserve-cli/pkg/errors"
)

// TestPromQLEnvelope: only a reply that says "success" and names its result
// type is an answer. Anything else used to read as an empty result.
func TestPromQLEnvelope(t *testing.T) {
	cases := []struct {
		name     string
		body     string
		wantCode string
		wantCat  cerrors.Category
	}{
		{"server error in a 200", `{"status":"error","errorType":"bad_data","error":"parse error"}`,
			"PROMQL_ERROR", cerrors.CategoryUsage},
		{"no status", `{"data":{"resultType":"vector","result":[]}}`,
			"PROMQL_RESPONSE_INVALID", cerrors.CategoryParse},
		{"no result type", `{"status":"success","data":{"result":[]}}`,
			"PROMQL_RESPONSE_INVALID", cerrors.CategoryParse},
		{"undecodable data", `{"status":"success","data":"<html>"}`,
			"PROMQL_RESPONSE_INVALID", cerrors.CategoryParse},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(tc.body))
			})
			resp, err := client.QueryMetricsInstant(context.Background(), "default", "up", 1)
			if err == nil {
				t.Fatalf("want an error, got %+v", resp)
			}
			ce := cerrors.AsCLIError(err)
			if ce.Code != tc.wantCode || ce.Category != tc.wantCat {
				t.Fatalf("want %s/%s, got %s/%s: %s", tc.wantCat, tc.wantCode, ce.Category, ce.Code, ce.Message)
			}
		})
	}

	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"success","warnings":["results truncated"],"infos":["i"],` +
			`"data":{"resultType":"vector","result":[{"metric":{},"value":[1,"1"]}]}}`))
	})
	resp, err := client.QueryMetricsRange(context.Background(), "default", "up", 1, 2, "1m")
	if err != nil {
		t.Fatal(err)
	}
	if resp.ResultType != "vector" || len(resp.Result) == 0 ||
		len(resp.Warnings) != 1 || resp.Warnings[0] != "results truncated" || len(resp.Infos) != 1 {
		t.Fatalf("want the result and its advisories, got %+v", resp)
	}
}
