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
