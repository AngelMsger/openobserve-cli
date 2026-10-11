package transport

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/angelmsger/openobserve-cli/pkg/constants"
)

// The client is tested in agent-cli-kit; this checks openobserve-cli's default.
func TestNewSendsOpenObserveUserAgent(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("User-Agent")
	}))
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	resp, err := New(Options{}).Do(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if got != constants.UserAgent() {
		t.Fatalf("User-Agent = %q, want %q", got, constants.UserAgent())
	}
}
