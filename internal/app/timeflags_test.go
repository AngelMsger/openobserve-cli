package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	cerrors "github.com/angelmsger/openobserve-cli/pkg/errors"
)

// Every command that takes a time window shares one resolver, so the family
// contract holds for all of them: --since excludes --from and --to, and --to
// requires --from. A conflicting window is a usage error raised before any
// request, never a silent preference for one flag.
func TestTimeWindowFlagsRejectAmbiguousCombinations(t *testing.T) {
	const from, to = "2026-09-01T00:00:00Z", "2026-09-01T01:00:00Z"
	commands := map[string][]string{
		"search run":          {"search", "run", "--stream", "app"},
		"search histogram":    {"search", "histogram", "--stream", "app"},
		"trace search":        {"trace", "search", "--stream", "traces"},
		"trace get":           {"trace", "get", "abc123", "--stream", "traces"},
		"metrics query-range": {"metrics", "query-range", "--query", "up", "--step", "1m"},
	}
	windows := []struct {
		name  string
		flags []string
		want  string
	}{
		{"since with from", []string{"--since", "1h", "--from", from}, "--since cannot be combined with --from or --to"},
		{"since with to", []string{"--since", "1h", "--to", to}, "--since cannot be combined with --from or --to"},
		{"since with from and to", []string{"--since", "1h", "--from", from, "--to", to}, "--since cannot be combined with --from or --to"},
		{"to without from", []string{"--to", to}, "--to requires --from"},
	}
	for name, args := range commands {
		for _, window := range windows {
			t.Run(name+"/"+window.name, func(t *testing.T) {
				var requests atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					requests.Add(1)
					w.WriteHeader(http.StatusInternalServerError)
				}))
				defer server.Close()
				paginationEnvironment(t, server.URL)
				out, _, err := capturePaginationCommand(t, append(append([]string{}, args...), window.flags...)...)
				if err == nil {
					t.Fatalf("accepted an ambiguous window: stdout=%s", out)
				}
				ce := cerrors.AsCLIError(err)
				if ce.Category != cerrors.CategoryUsage || ce.Code != "BAD_TIME_RANGE" || cerrors.ExitCode(ce) != cerrors.ExitUsage {
					t.Fatalf("got category=%q code=%q exit=%d, want usage/BAD_TIME_RANGE/%d", ce.Category, ce.Code, cerrors.ExitCode(ce), cerrors.ExitUsage)
				}
				if ce.Message != window.want {
					t.Fatalf("message = %q, want %q", ce.Message, window.want)
				}
				if !strings.Contains(ce.Hint, "--to requires --from") || len(ce.NextSteps) == 0 {
					t.Fatalf("error does not restate the contract: hint=%q next_steps=%v", ce.Hint, ce.NextSteps)
				}
				if out != "" || requests.Load() != 0 {
					t.Fatalf("rejected window still produced output or a request: stdout=%q requests=%d", out, requests.Load())
				}
			})
		}
	}
}

// The supported forms keep resolving: a look-back, a fixed window, and a lower
// bound whose upper bound defaults to now.
func TestTimeWindowFlagsAcceptSupportedForms(t *testing.T) {
	for _, flags := range [][]string{
		{"--since", "1h"},
		{"--from", "2026-09-01T00:00:00Z", "--to", "2026-09-01T01:00:00Z"},
		{"--from", "now-1h"},
	} {
		tf := timeFlags{}
		for i := 0; i < len(flags); i += 2 {
			switch flags[i] {
			case "--since":
				tf.since = flags[i+1]
			case "--from":
				tf.from = flags[i+1]
			case "--to":
				tf.to = flags[i+1]
			}
		}
		start, end, err := tf.resolve()
		if err != nil || end <= start {
			t.Fatalf("%v: start=%d end=%d err=%v", flags, start, end, err)
		}
	}
}
