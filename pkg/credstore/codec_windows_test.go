//go:build windows

package credstore

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsFallbackFileUsesDPAPI(t *testing.T) {
	withKeychain(t, false)
	dir := t.TempDir()
	s := NewStoreWithBackend(dir, newStub(nil))
	const secret = "windows-fallback-secret"
	backend, err := s.Save("host:basic", secret)
	if err != nil {
		t.Fatal(err)
	}
	if backend != BackendFile {
		t.Fatalf("Save() backend = %q, want %q", backend, BackendFile)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "credentials"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(raw, []byte(windowsCredentialFilePrefix)) || bytes.Contains(raw, []byte(secret)) {
		t.Fatal("fallback file must contain DPAPI ciphertext without the plaintext secret")
	}
	got, err := s.Load("host:basic")
	if err != nil || got != secret {
		t.Fatalf("Load() = %q, %v; want the stored secret", got, err)
	}
}

func TestWindowsCredentialFileCodec(t *testing.T) {
	plain := []byte(`{"account":"fallback-secret"}`)
	encoded, err := encodeCredentialFile(plain)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(encoded), windowsCredentialFilePrefix) {
		t.Fatalf("encoded fallback does not have %q prefix", windowsCredentialFilePrefix)
	}
	if bytes.Contains(encoded, []byte("fallback-secret")) {
		t.Fatal("encoded fallback contains the plaintext secret")
	}
	decoded, err := decodeCredentialFile(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decoded, plain) {
		t.Fatalf("decoded fallback = %q, want %q", decoded, plain)
	}
}

func TestWindowsCredentialFileCodecReadsLegacyPlaintext(t *testing.T) {
	legacy := []byte(`{"account":"legacy-secret"}`)
	decoded, err := decodeCredentialFile(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decoded, legacy) {
		t.Fatalf("decoded legacy fallback = %q, want %q", decoded, legacy)
	}
}
