package webauth

import (
	"strings"
	"testing"
)

func TestProbeJSBindsDeliverName(t *testing.T) {
	js := ProbeJS("__oo_probe", "o2.example.com")
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
	js := ProbeJS("d", "o2.example.com")
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
	js := strings.TrimSpace(ProbeJS("d", "o2.example.com"))
	if !strings.HasPrefix(js, "(function()") || !strings.HasSuffix(js, ")();") {
		t.Fatalf("script must be one self-invoking expression, got prefix/suffix of %q", js)
	}
}

// The script is injected into EVERY document the browser loads, including a
// full-page or iframed identity provider. Without an origin gate, an IdP that
// sends its own Authorization header over XHR has that header captured, stored
// in the user's keychain, and replayed to the OpenObserve instance — a third
// party's credential sent to a different server. The gate is the fix, so pin
// its shape.
func TestProbeJSGatesOnHost(t *testing.T) {
	js := ProbeJS("d", "o2.example.com")
	for _, want := range []string{
		`var t="o2.example.com";`,
		"location.hostname",
		"toLowerCase()",
		`if(!t||!h||(h!==t&&h.slice(-(t.length+1))!=='.'+t)){return;}`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("script is missing the host gate fragment %q\ngot: %s", want, js)
		}
	}
	// The gate must come before anything that could capture or deliver, or a
	// foreign origin still gets hooked.
	gate := strings.Index(js, "{return;}")
	for _, after := range []string{"window.fetch", "XMLHttpRequest.prototype.setRequestHeader", "localStorage"} {
		if idx := strings.Index(js, after); idx < gate {
			t.Errorf("%q appears at %d, before the host gate at %d; a foreign origin would still be hooked",
				after, idx, gate)
		}
	}
}

// location.hostname never carries a port, so a host like "localhost:5080" must
// be compared without one — otherwise the gate would reject the very instance
// it is scoping to and capture nothing at all.
func TestProbeJSStripsPortAndLowercasesHost(t *testing.T) {
	cases := []struct{ in, want string }{
		{"localhost:5080", `var t="localhost";`},
		{"O2.Example.COM", `var t="o2.example.com";`},
		// An IPv6 literal keeps its brackets: that is what location.hostname
		// reports, and the gate compares against it directly.
		{"[::1]:5080", `var t="[::1]";`},
		{"o2.example.com", `var t="o2.example.com";`},
	}
	for _, tc := range cases {
		if js := ProbeJS("d", tc.in); !strings.Contains(js, tc.want) {
			t.Errorf("ProbeJS(_, %q) does not contain %q", tc.in, tc.want)
		}
	}
}

// An empty host must fail closed. Capturing everywhere would be exactly the
// leak the gate exists to prevent.
func TestProbeJSWithNoHostCapturesNothing(t *testing.T) {
	js := ProbeJS("d", "")
	if !strings.Contains(js, `var t="";`) {
		t.Fatalf("empty host did not produce an empty target: %s", js)
	}
	if !strings.Contains(js, "if(!t||") {
		t.Fatal("the gate does not fail closed on an empty target host")
	}
}

// A hostile host string must not be able to break out of the JS literal.
func TestProbeJSEscapesHost(t *testing.T) {
	js := ProbeJS("d", `evil";__DELIVER__("pwned`)
	if strings.Contains(js, `evil";__`) {
		t.Fatalf("host was interpolated unescaped: %s", js)
	}
	if !strings.Contains(js, `\"`) {
		t.Fatalf("expected the quote to be escaped: %s", js)
	}
}

// The gate must accept a subdomain of the configured host, matching the
// relation HostMatches applies to cookie domains, so an instance served from a
// sibling name inside the same zone still captures.
func TestProbeJSGateMatchesHostMatchesSemantics(t *testing.T) {
	// Mirror the generated JS comparison in Go and cross-check it against
	// HostMatches, which is the semantics the rest of the package uses.
	gate := func(pageHost, target string) bool {
		t2 := probeHost(target)
		h := strings.ToLower(pageHost)
		if t2 == "" || h == "" {
			return false
		}
		return h == t2 || strings.HasSuffix(h, "."+t2)
	}
	cases := []struct {
		page, target string
		want         bool
	}{
		{"o2.example.com", "o2.example.com:5080", true},
		{"eu.o2.example.com", "o2.example.com", true},
		{"o2.example.com.evil.net", "o2.example.com", false},
		{"login.microsoftonline.com", "o2.example.com", false},
		{"example.com", "o2.example.com", false},
		{"", "o2.example.com", false},
	}
	for _, tc := range cases {
		if got := gate(tc.page, tc.target); got != tc.want {
			t.Errorf("gate(%q, %q) = %v, want %v", tc.page, tc.target, got, tc.want)
		}
		// HostMatches takes a cookie domain, which never carries a port, so
		// cross-check only the port-free targets.
		if tc.page != "" && !strings.Contains(tc.target, ":") &&
			tc.want != HostMatches(tc.target, tc.page) {
			t.Errorf("gate and HostMatches disagree for page=%q target=%q", tc.page, tc.target)
		}
	}
}
