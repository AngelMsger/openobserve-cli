// Package webauth holds the browser sign-in capture core shared by
// openobserve-cli and the o3 desktop app: cookie shaping, the login-success
// heuristic, the injected capture script, and the policy deciding when a
// captured state counts as a completed login.
//
// It is pure and dependency-light — it imports only the standard library and
// pkg/auth — so both a CLI driving a browser over the DevTools Protocol and a
// desktop app driving a native WebView can share it. Transport-specific code
// lives with its client (see pkg/webauth/cdp for the CLI's).
package webauth

import (
	"net"
	"net/url"
	"sort"
	"strings"
	"time"
)

// Cookie is a single captured cookie, mirroring the fields the native
// WKHTTPCookieStore hands back.
type Cookie struct {
	Name     string
	Value    string
	Domain   string
	Path     string
	Expires  time.Time // zero for session cookies
	Secure   bool
	HTTPOnly bool
}

// authCookieNames are OpenObserve cookies set only after a successful login.
// Their presence is a strong signal the user is authenticated, even if the SPA
// stayed on the same URL path.
var authCookieNames = map[string]bool{
	"auth_ext":    true,
	"auth_tokens": true,
}

// SerializeCookies renders cookies into a Cookie header value ("k1=v1; k2=v2"),
// sorted by name (then value) for a stable, testable result.
func SerializeCookies(cs []Cookie) string {
	sorted := append([]Cookie(nil), cs...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Name != sorted[j].Name {
			return sorted[i].Name < sorted[j].Name
		}
		return sorted[i].Value < sorted[j].Value
	})
	parts := make([]string, 0, len(sorted))
	for _, c := range sorted {
		if c.Name == "" {
			continue
		}
		parts = append(parts, c.Name+"="+c.Value)
	}
	return strings.Join(parts, "; ")
}

// hostname strips any port/brackets and lowercases a host. net.SplitHostPort is
// required here: splitting at the first colon truncates IPv6 hosts to "[".
func hostname(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	if h, _, err := net.SplitHostPort(host); err == nil {
		return strings.Trim(h, "[]")
	}
	return strings.Trim(host, "[]")
}

// HostMatches reports whether a cookie's Domain applies to host. An empty
// domain is a host-only cookie and matches (the caller already scoped it). A
// leading dot is ignored; host equal to, or a subdomain of, the domain matches.
func HostMatches(cookieDomain, host string) bool {
	h := hostname(host)
	d := strings.ToLower(strings.TrimSpace(cookieDomain))
	if d == "" {
		return true
	}
	d = strings.TrimPrefix(d, ".")
	return h == d || strings.HasSuffix(h, "."+d)
}

// FilterForHost keeps only cookies whose domain applies to host.
func FilterForHost(cs []Cookie, host string) []Cookie {
	out := make([]Cookie, 0, len(cs))
	for _, c := range cs {
		if HostMatches(c.Domain, host) {
			out = append(out, c)
		}
	}
	return out
}

// EarliestExpiry returns the soonest non-zero expiry among cookies (drives the
// "expires in N days" UI). Zero when every cookie is a session cookie.
func EarliestExpiry(cs []Cookie) time.Time {
	var earliest time.Time
	for _, c := range cs {
		if c.Expires.IsZero() {
			continue
		}
		if earliest.IsZero() || c.Expires.Before(earliest) {
			earliest = c.Expires
		}
	}
	return earliest
}

// hasAuthCookie reports whether the cookies include a known post-login
// OpenObserve auth cookie with a non-empty value.
func hasAuthCookie(cs []Cookie) bool {
	for _, c := range cs {
		if c.Value != "" && authCookieNames[strings.ToLower(c.Name)] {
			return true
		}
	}
	return false
}

// LoginSucceeded is the capture heuristic, kept pure so it can be unit-tested in
// isolation. Login is considered complete when either:
//   - a known post-login OpenObserve auth cookie for the host is present, or
//   - the WebView has navigated off the login page AND at least one host cookie
//     exists (OpenObserve lands on /web/ or /web/logs after login).
func LoginSucceeded(currentURL, host string, cookies []Cookie) bool {
	scoped := FilterForHost(cookies, host)
	if hasAuthCookie(scoped) {
		return true
	}
	if len(scoped) == 0 {
		return false
	}
	u, err := url.Parse(currentURL)
	if err != nil {
		return false
	}
	return !strings.Contains(strings.ToLower(u.Path), "/login")
}
