package errors

import "testing"

func TestExitCodeMapping(t *testing.T) {
	cases := map[Category]int{
		CategoryUsage:      ExitUsage,
		CategoryConfig:     ExitConfig,
		CategoryAuth:       ExitAuth,
		CategoryPermission: ExitPermission,
		CategoryNotFound:   ExitNotFound,
		CategoryConflict:   ExitConflict,
		CategoryRateLimit:  ExitRateLimit,
		CategoryNetwork:    ExitNetwork,
		CategoryServer:     ExitServer,
		CategoryParse:      ExitParse,
		CategoryInternal:   ExitInternal,
	}
	for cat, want := range cases {
		if got := ExitCode(New(cat, "X", "msg")); got != want {
			t.Errorf("ExitCode(%s) = %d, want %d", cat, got, want)
		}
	}
	if ExitCode(nil) != ExitSuccess {
		t.Error("nil error should map to ExitSuccess")
	}
}

func TestFromHTTPStatus(t *testing.T) {
	cases := map[int]Category{
		401: CategoryAuth,
		403: CategoryPermission,
		404: CategoryNotFound,
		409: CategoryConflict,
		429: CategoryRateLimit,
		500: CategoryServer,
		503: CategoryServer,
		400: CategoryUsage,
		422: CategoryUsage,
	}
	for status, want := range cases {
		if got := FromHTTPStatus(status); got != want {
			t.Errorf("FromHTTPStatus(%d) = %s, want %s", status, got, want)
		}
	}
}

func TestRetryableDefaults(t *testing.T) {
	if !New(CategoryNetwork, "N", "m").Retryable {
		t.Error("network errors should be retryable")
	}
	if New(CategoryUsage, "U", "m").Retryable {
		t.Error("usage errors should not be retryable")
	}
}

func TestPayloadShape(t *testing.T) {
	e := New(CategoryNotFound, "STREAM_NOT_FOUND", "no such stream").
		WithNextSteps("openobserve-cli stream list").
		WithHTTPStatus(404).
		WithRecovery(Recovery{Action: "retry_current_command", Scope: "host", Requires: []string{"os_keychain"}})
	p := e.Payload()
	if p.Error.Code != "STREAM_NOT_FOUND" || p.Error.Category != CategoryNotFound {
		t.Errorf("unexpected payload: %+v", p.Error)
	}
	if len(p.Error.NextSteps) != 1 || p.Error.HTTPStatus != 404 {
		t.Errorf("payload missing next_steps/http_status: %+v", p.Error)
	}
	if p.Error.Recovery == nil || p.Error.Recovery.Scope != "host" {
		t.Errorf("payload missing recovery: %+v", p.Error)
	}
}
