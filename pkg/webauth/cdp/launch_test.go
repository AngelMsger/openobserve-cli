package cdp

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestBrowserCandidatesAreAbsoluteOrBareNames(t *testing.T) {
	got := browserCandidates()
	if len(got) == 0 {
		t.Fatal("no browser candidates for this platform")
	}
	for _, c := range got {
		if c == "" {
			t.Fatal("empty candidate")
		}
	}
	// Sanity-check the platform actually got its own list rather than a default.
	switch runtime.GOOS {
	case "darwin":
		if !strings.Contains(strings.Join(got, "|"), "/Applications/") {
			t.Errorf("darwin candidates should include /Applications paths: %v", got)
		}
	case "windows":
		if !strings.Contains(strings.ToLower(strings.Join(got, "|")), "chrome.exe") {
			t.Errorf("windows candidates should include chrome.exe: %v", got)
		}
	default:
		if !strings.Contains(strings.Join(got, "|"), "google-chrome") {
			t.Errorf("linux candidates should include google-chrome: %v", got)
		}
	}
}

func TestFindBrowserPrefersEnvOverride(t *testing.T) {
	fake := filepath.Join(t.TempDir(), "my-browser")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OPENOBSERVE_BROWSER", fake)
	got, err := findBrowser()
	if err != nil {
		t.Fatalf("findBrowser: %v", err)
	}
	if got != fake {
		t.Fatalf("findBrowser = %q, want the override %q", got, fake)
	}
}

func TestFindBrowserRejectsMissingOverride(t *testing.T) {
	t.Setenv("OPENOBSERVE_BROWSER", filepath.Join(t.TempDir(), "nope"))
	if _, err := findBrowser(); err == nil {
		t.Fatal("a set-but-missing override must be an error, not a silent fallback")
	}
}

func TestReadDevToolsURLWaitsForTheFile(t *testing.T) {
	dir := t.TempDir()
	go func() {
		time.Sleep(150 * time.Millisecond)
		_ = os.WriteFile(filepath.Join(dir, "DevToolsActivePort"),
			[]byte("54321\n/devtools/browser/abc-123\n"), 0o600)
	}()
	got, err := readDevToolsURL(dir, 3*time.Second, func() bool { return true })
	if err != nil {
		t.Fatalf("readDevToolsURL: %v", err)
	}
	want := "ws://127.0.0.1:54321/devtools/browser/abc-123"
	if got != want {
		t.Fatalf("url = %q, want %q", got, want)
	}
}

func TestReadDevToolsURLFailsWhenProcessDies(t *testing.T) {
	_, err := readDevToolsURL(t.TempDir(), 3*time.Second, func() bool { return false })
	if err == nil {
		t.Fatal("must fail fast when the browser process is gone, not wait for the timeout")
	}
}

// The fast-fail path must be driven by launch's OWN liveness callback, not a
// stub. /bin/sh rejects Chrome's flags and exits immediately, standing in for a
// browser that dies on startup (missing library, bad flag, sandbox refusal).
func TestLaunchFailsFastWhenBrowserExitsImmediately(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs a POSIX shell")
	}
	start := time.Now()
	_, _, _, err := launch(context.Background(), "/bin/sh", t.TempDir())
	if err == nil {
		t.Fatal("launch must fail when the browser exits immediately")
	}
	if elapsed := time.Since(start); elapsed > 20*time.Second {
		t.Fatalf("launch took %s — the liveness fast-fail never fired", elapsed)
	}
}
