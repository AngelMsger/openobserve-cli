package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMicrosToSeconds(t *testing.T) {
	cases := map[int64]float64{
		1_700_000_000_000_000: 1_700_000_000,   // whole second
		1_700_000_000_500_000: 1_700_000_000.5, // sub-second precision preserved
		0:                     0,
	}
	for micros, want := range cases {
		if got := microsToSeconds(micros); got != want {
			t.Errorf("microsToSeconds(%d) = %v, want %v", micros, got, want)
		}
	}
}

func TestInstantSecondsDefaultsToNow(t *testing.T) {
	// An empty --time resolves to roughly now (within a generous window).
	got, err := instantSeconds("")
	if err != nil {
		t.Fatal(err)
	}
	if got < 1_700_000_000 {
		t.Fatalf("instantSeconds(\"\") = %v, expected a recent unix-seconds value", got)
	}
}

func TestInstantSecondsParsesRFC3339(t *testing.T) {
	got, err := instantSeconds("2023-11-14T22:13:20Z")
	if err != nil {
		t.Fatal(err)
	}
	if got != 1_700_000_000 {
		t.Fatalf("instantSeconds(rfc3339) = %v, want 1700000000", got)
	}
}

// TestMetricsAdvisories: the server's warnings travel with the JSON result,
// and reach stderr when ndjson writes only the series, as in prometheus-cli.
func TestMetricsAdvisories(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"success","warnings":["results truncated"],` +
			`"data":{"resultType":"vector","result":[{"metric":{"service":"web"},"value":[1,"1"]}]}}`))
	}))
	t.Cleanup(server.Close)
	paginationEnvironment(t, server.URL)

	stdout, stderr, err := capturePaginationCommand(t, "metrics", "query", "--query", "up", "--format", "json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		ResultType string   `json:"result_type"`
		Warnings   []string `json:"warnings"`
	}
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("invalid JSON: %v: %s", err, stdout)
	}
	if doc.ResultType != "vector" || len(doc.Warnings) != 1 {
		t.Fatalf("want the warning in the result, got %s", stdout)
	}
	if strings.Contains(stderr, "query_advisories") {
		t.Fatalf("a document that carries its warnings needs no notice: %s", stderr)
	}

	stdout, stderr, err = capturePaginationCommand(t, "metrics", "query", "--query", "up", "--format", "ndjson")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stdout, "results truncated") || !strings.Contains(stdout, `"service":"web"`) {
		t.Fatalf("want only the series on stdout, got %s", stdout)
	}
	if !strings.Contains(stderr, "query_advisories") || !strings.Contains(stderr, "results truncated") {
		t.Fatalf("want the warning on stderr, got %s", stderr)
	}
}
