package auth

import (
	"net/http"
	"testing"

	cerrors "github.com/angelmsger/openobserve-cli/pkg/errors"
)

func TestHeader(t *testing.T) {
	tests := []struct {
		name string
		cred Credential
		want string
	}{
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
			name: "token prefix is case-insensitive and trimmed",
			cred: Credential{Scheme: SchemeToken, Secret: "  bearer xyz "},
			want: "bearer xyz",
		},
		{
			name: "session sends no Authorization header of its own",
			cred: Credential{Scheme: SchemeSession, Secret: `{"cookies":"sid=abc","authorization":"Bearer tok"}`},
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
		name     string
		cred     Credential
		wantCode string
	}{
		{"basic ok", Credential{Scheme: SchemeBasic, Username: "u", Secret: "p"}, ""},
		{"basic missing secret", Credential{Scheme: SchemeBasic, Username: "u"}, "AUTH_NO_BASIC"},
		{"token ok", Credential{Scheme: SchemeToken, Secret: "t"}, ""},
		{"token missing secret", Credential{Scheme: SchemeToken}, "AUTH_NO_TOKEN"},
		{"session ok", Credential{Scheme: SchemeSession, Secret: "sid=abc"}, ""},
		{"session missing secret", Credential{Scheme: SchemeSession}, "AUTH_BAD_SESSION"},
		{"session empty envelope", Credential{Scheme: SchemeSession, Secret: "{}"}, "AUTH_BAD_SESSION"},
		{"session metadata only", Credential{Scheme: SchemeSession, Secret: `{"email":"ops@example.com"}`}, "AUTH_BAD_SESSION"},
		{"session malformed envelope", Credential{Scheme: SchemeSession, Secret: "{not json"}, "AUTH_BAD_SESSION"},
		{"session header injection", Credential{Scheme: SchemeSession, Secret: "sid=abc\r\nX-Bad: yes"}, "AUTH_BAD_SESSION"},
		{"unknown scheme", Credential{Scheme: "x", Secret: "t"}, "AUTH_BAD_SCHEME"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cred.Validate()
			if tt.wantCode == "" {
				if err != nil {
					t.Fatalf("Validate() err = %v, want nil", err)
				}
				return
			}
			if ce := cerrors.AsCLIError(err); err == nil || ce.Code != tt.wantCode {
				t.Fatalf("Validate() err = %v, want code %s", err, tt.wantCode)
			}
		})
	}
}

func TestValidateMessages(t *testing.T) {
	tests := []struct {
		cred Credential
		want string
	}{
		{Credential{Scheme: SchemeBasic}, "basic auth requires both an email and a password"},
		{Credential{Scheme: SchemeToken}, "no token configured"},
		{Credential{Scheme: SchemeSession}, "captured browser session is invalid; sign in through the browser again"},
		{Credential{Scheme: "kerberos"}, `unknown auth scheme "kerberos" (want basic, token or session)`},
	}
	for _, tt := range tests {
		if ce := cerrors.AsCLIError(tt.cred.Validate()); ce == nil || ce.Message != tt.want {
			t.Fatalf("Validate(%q) = %+v, want message %q", tt.cred.Scheme, ce, tt.want)
		}
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

// OpenObserve keys stored secrets by host alone, even for a path-routed URL;
// keying by path would orphan every secret already stored.
func TestAccountKey(t *testing.T) {
	tests := []struct {
		baseURL, scheme, want string
	}{
		{"http://localhost:5080", "basic", "localhost:5080:basic"},
		{"https://o2.example.com/deploy/", "session", "o2.example.com:session"},
	}
	for _, tt := range tests {
		if got := AccountKey(tt.baseURL, tt.scheme); got != tt.want {
			t.Fatalf("AccountKey(%q,%q) = %q, want %q", tt.baseURL, tt.scheme, got, tt.want)
		}
	}
}
