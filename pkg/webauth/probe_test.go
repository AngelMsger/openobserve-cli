package webauth

import (
	"strings"
	"testing"
)

func TestProbeJSBindsDeliverName(t *testing.T) {
	js := ProbeJS("__oo_probe")
	if !strings.Contains(js, "__oo_probe") {
		t.Fatal("script does not call the delivery function it was given")
	}
	if strings.Contains(js, "webkit.messageHandlers") {
		t.Fatal("script must not hardcode a transport; the caller binds the delivery name")
	}
}

// Both hooks are load-bearing: OpenObserve's web app issues requests through
// axios (XHR), so a fetch-only hook captures nothing on real instances and
// leaves only the short-lived session cookie. Guard against a well-meaning
// simplification deleting one.
func TestProbeJSHooksBothFetchAndXHR(t *testing.T) {
	js := ProbeJS("d")
	for _, want := range []string{
		"window.fetch",
		"XMLHttpRequest.prototype.setRequestHeader",
		"localStorage.getItem('user_info')",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("script is missing %q", want)
		}
	}
}

func TestProbeJSIsSelfContainedExpression(t *testing.T) {
	js := strings.TrimSpace(ProbeJS("d"))
	if !strings.HasPrefix(js, "(function()") || !strings.HasSuffix(js, ")();") {
		t.Fatalf("script must be one self-invoking expression, got prefix/suffix of %q", js)
	}
}
