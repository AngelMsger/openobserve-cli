package webauth

import (
	"testing"
	"time"
)

func TestSerializeCookies(t *testing.T) {
	tests := []struct {
		name string
		in   []Cookie
		want string
	}{
		{"empty", nil, ""},
		{
			"sorted by name",
			[]Cookie{{Name: "b", Value: "2"}, {Name: "a", Value: "1"}},
			"a=1; b=2",
		},
		{
			"skips nameless",
			[]Cookie{{Name: "", Value: "x"}, {Name: "a", Value: "1"}},
			"a=1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SerializeCookies(tt.in); got != tt.want {
				t.Fatalf("SerializeCookies() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHostMatches(t *testing.T) {
	tests := []struct {
		name         string
		cookieDomain string
		host         string
		want         bool
	}{
		{"exact", "observe.example.com", "observe.example.com", true},
		{"leading dot", ".example.com", "observe.example.com", true},
		{"subdomain", "example.com", "observe.example.com", true},
		{"host with port", "observe.example.com", "observe.example.com:5080", true},
		{"ipv6 with port", "::1", "[::1]:5080", true},
		{"ipv6 without port", "::1", "[::1]", true},
		{"empty domain matches", "", "observe.example.com", true},
		{"mismatch", "other.com", "observe.example.com", false},
		{"suffix but not subdomain", "ample.com", "example.com", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HostMatches(tt.cookieDomain, tt.host); got != tt.want {
				t.Fatalf("HostMatches(%q,%q) = %v, want %v", tt.cookieDomain, tt.host, got, tt.want)
			}
		})
	}
}

func TestFilterForHost(t *testing.T) {
	cs := []Cookie{
		{Name: "keep", Domain: "example.com"},
		{Name: "drop", Domain: "evil.com"},
		{Name: "hostonly", Domain: ""},
	}
	got := FilterForHost(cs, "observe.example.com")
	if len(got) != 2 {
		t.Fatalf("FilterForHost kept %d cookies, want 2: %+v", len(got), got)
	}
}

func TestEarliestExpiry(t *testing.T) {
	soon := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	late := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	t.Run("mix picks soonest non-zero", func(t *testing.T) {
		got := EarliestExpiry([]Cookie{{Expires: late}, {}, {Expires: soon}})
		if !got.Equal(soon) {
			t.Fatalf("EarliestExpiry = %v, want %v", got, soon)
		}
	})
	t.Run("all session cookies -> zero", func(t *testing.T) {
		if got := EarliestExpiry([]Cookie{{}, {}}); !got.IsZero() {
			t.Fatalf("EarliestExpiry = %v, want zero", got)
		}
	})
}

func TestLoginSucceeded(t *testing.T) {
	host := "observe.example.com"
	authCookie := Cookie{Name: "auth_ext", Value: "abc", Domain: host}
	plainCookie := Cookie{Name: "sid", Value: "x", Domain: host}
	tests := []struct {
		name    string
		url     string
		cookies []Cookie
		want    bool
	}{
		{"on login page, no cookies", "https://observe.example.com/web/login", nil, false},
		{"on login page, plain cookie only", "https://observe.example.com/web/login", []Cookie{plainCookie}, false},
		{"auth cookie present even on login path", "https://observe.example.com/web/login", []Cookie{authCookie}, true},
		{"navigated off login with cookie", "https://observe.example.com/web/logs", []Cookie{plainCookie}, true},
		{"navigated off login, no host cookie", "https://observe.example.com/web/logs", nil, false},
		{"cookie for other host is ignored", "https://observe.example.com/web/logs", []Cookie{{Name: "sid", Value: "x", Domain: "evil.com"}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := LoginSucceeded(tt.url, host, tt.cookies); got != tt.want {
				t.Fatalf("LoginSucceeded = %v, want %v", got, tt.want)
			}
		})
	}
}
