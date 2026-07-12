package auth

import (
	"encoding/base64"
	"net/http"
	"testing"
)

func TestHeader(t *testing.T) {
	tests := []struct {
		name string
		cred Credential
		want string
	}{
		{
			name: "basic encodes email:password",
			cred: Credential{Scheme: SchemeBasic, Username: "ops@x.com", Secret: "pw"},
			want: "Basic " + base64.StdEncoding.EncodeToString([]byte("ops@x.com:pw")),
		},
		{
			name: "token without prefix gets Basic",
			cred: Credential{Scheme: SchemeToken, Secret: "abc123"},
			want: "Basic abc123",
		},
		{
			name: "token with Basic prefix passes through",
			cred: Credential{Scheme: SchemeToken, Secret: "Basic abc123"},
			want: "Basic abc123",
		},
		{
			name: "token with Bearer prefix passes through",
			cred: Credential{Scheme: SchemeToken, Secret: "Bearer xyz"},
			want: "Bearer xyz",
		},
		{
			name: "unknown scheme yields empty",
			cred: Credential{Scheme: "nope", Secret: "x"},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cred.Header(); got != tt.want {
				t.Fatalf("Header() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		cred    Credential
		wantErr bool
	}{
		{"basic ok", Credential{Scheme: SchemeBasic, Username: "u", Secret: "p"}, false},
		{"basic missing secret", Credential{Scheme: SchemeBasic, Username: "u"}, true},
		{"token ok", Credential{Scheme: SchemeToken, Secret: "t"}, false},
		{"token missing secret", Credential{Scheme: SchemeToken}, true},
		{"session ok", Credential{Scheme: SchemeSession, Secret: "sid=abc"}, false},
		{"session missing secret", Credential{Scheme: SchemeSession}, true},
		{"session empty envelope", Credential{Scheme: SchemeSession, Secret: "{}"}, true},
		{"session metadata only", Credential{Scheme: SchemeSession, Secret: `{"email":"ops@example.com"}`}, true},
		{"session malformed envelope", Credential{Scheme: SchemeSession, Secret: "{not json"}, true},
		{"session header injection", Credential{Scheme: SchemeSession, Secret: "sid=abc\r\nX-Bad: yes"}, true},
		{"unknown scheme", Credential{Scheme: "x", Secret: "t"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cred.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() err = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestDecoratorSetsAuthorization(t *testing.T) {
	cred := Credential{Scheme: SchemeToken, Secret: "Bearer tok"}
	req, _ := http.NewRequest(http.MethodGet, "http://x", nil)
	cred.Decorator()(req)
	if got := req.Header.Get("Authorization"); got != "Bearer tok" {
		t.Fatalf("Authorization = %q, want %q", got, "Bearer tok")
	}
}

func TestDecoratorSetsCookie(t *testing.T) {
	// Bare cookie string: only the Cookie header is set.
	t.Run("cookies only", func(t *testing.T) {
		cred := Credential{Scheme: SchemeSession, Secret: "sid=abc; auth_ext=xyz"}
		req, _ := http.NewRequest(http.MethodGet, "http://x", nil)
		cred.Decorator()(req)
		if got := req.Header.Get("Cookie"); got != "sid=abc; auth_ext=xyz" {
			t.Fatalf("Cookie = %q, want %q", got, "sid=abc; auth_ext=xyz")
		}
		if got := req.Header.Get("Authorization"); got != "" {
			t.Fatalf("Authorization = %q, want empty", got)
		}
	})
	// JSON envelope carrying an Authorization fallback: both headers are set.
	t.Run("envelope with authorization fallback", func(t *testing.T) {
		blob, err := EncodeSession(Session{Cookies: "sid=abc", Authorization: "Bearer tok"})
		if err != nil {
			t.Fatalf("EncodeSession: %v", err)
		}
		cred := Credential{Scheme: SchemeSession, Secret: blob}
		req, _ := http.NewRequest(http.MethodGet, "http://x", nil)
		cred.Decorator()(req)
		if got := req.Header.Get("Cookie"); got != "sid=abc" {
			t.Fatalf("Cookie = %q, want %q", got, "sid=abc")
		}
		if got := req.Header.Get("Authorization"); got != "Bearer tok" {
			t.Fatalf("Authorization = %q, want %q", got, "Bearer tok")
		}
	})
	t.Run("invalid session sets no headers", func(t *testing.T) {
		cred := Credential{Scheme: SchemeSession, Secret: "{}"}
		req, _ := http.NewRequest(http.MethodGet, "http://x", nil)
		cred.Decorator()(req)
		if got := req.Header.Get("Cookie"); got != "" {
			t.Fatalf("Cookie = %q, want empty", got)
		}
		if got := req.Header.Get("Authorization"); got != "" {
			t.Fatalf("Authorization = %q, want empty", got)
		}
	})
}

func TestAccountKey(t *testing.T) {
	tests := []struct {
		baseURL, scheme, want string
	}{
		{"http://localhost:5080", "basic", "localhost:5080:basic"},
		{"https://api.openobserve.ai", "token", "api.openobserve.ai:token"},
		{"not-a-url", "basic", "not-a-url:basic"},
	}
	for _, tt := range tests {
		if got := AccountKey(tt.baseURL, tt.scheme); got != tt.want {
			t.Fatalf("AccountKey(%q,%q) = %q, want %q", tt.baseURL, tt.scheme, got, tt.want)
		}
	}
}
