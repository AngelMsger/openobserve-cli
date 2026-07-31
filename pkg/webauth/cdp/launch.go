package cdp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// errNoBrowser is returned when no Chromium-family browser can be located. The
// CLI layer translates it into a BROWSER_NOT_FOUND cerrors value with next
// steps; keeping it a sentinel here keeps this package free of cerrors.
var errNoBrowser = errors.New("no Chromium-family browser found")

// browserCandidates lists the executables to try, most-preferred first. Bare
// names are resolved through PATH; absolute paths are probed directly.
func browserCandidates() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
			"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
		}
	case "windows":
		var out []string
		for _, base := range []string{
			os.Getenv("ProgramFiles"),
			os.Getenv("ProgramFiles(x86)"),
			os.Getenv("LOCALAPPDATA"),
		} {
			if base == "" {
				continue
			}
			out = append(out,
				filepath.Join(base, `Google\Chrome\Application\chrome.exe`),
				filepath.Join(base, `Microsoft\Edge\Application\msedge.exe`),
			)
		}
		return out
	default:
		return []string{
			"google-chrome", "google-chrome-stable", "chromium",
			"chromium-browser", "microsoft-edge", "brave-browser",
		}
	}
}

// findBrowser locates a usable browser. OPENOBSERVE_BROWSER overrides the
// search entirely; when it is set but unusable that is an error rather than a
// silent fallback, because silently ignoring an explicit choice is worse than
// failing.
func findBrowser() (string, error) {
	if override := strings.TrimSpace(os.Getenv("OPENOBSERVE_BROWSER")); override != "" {
		if _, err := os.Stat(override); err != nil {
			if resolved, lookErr := exec.LookPath(override); lookErr == nil {
				return resolved, nil
			}
			return "", fmt.Errorf("OPENOBSERVE_BROWSER is set to %q, which cannot be used: %w", override, err)
		}
		return override, nil
	}
	for _, c := range browserCandidates() {
		if filepath.IsAbs(c) {
			if _, err := os.Stat(c); err == nil {
				return c, nil
			}
			continue
		}
		if resolved, err := exec.LookPath(c); err == nil {
			return resolved, nil
		}
	}
	return "", errNoBrowser
}

// launch starts the browser on about:blank with remote debugging enabled and
// returns the process, the browser-level WebSocket URL, and a channel closed
// when the browser process exits.
//
// launch owns the process lifecycle: the goroutine started here is the ONLY
// cmd.Wait() on this Cmd. Callers learn about exit from the returned channel
// and must not call Wait themselves — a second Wait returns an error and races
// this one.
//
// The browser starts on about:blank rather than the login URL so the capture
// script can be installed before any instance page loads. Injecting after the
// SPA has already booted would race its first request, which is exactly the
// request carrying the Authorization header worth capturing.
//
// Port 0 makes the browser choose a free port and write it, with the WebSocket
// path, to DevToolsActivePort inside the profile directory. Reading that file
// is the only reliable way to learn the port.
func launch(ctx context.Context, exe, profileDir string) (*exec.Cmd, string, <-chan struct{}, error) {
	if err := os.MkdirAll(profileDir, 0o700); err != nil {
		return nil, "", nil, fmt.Errorf("create browser profile directory: %w", err)
	}
	// A stale port file from a previous run would be read as if it were ours.
	_ = os.Remove(filepath.Join(profileDir, "DevToolsActivePort"))

	cmd := exec.CommandContext(ctx, exe,
		"--user-data-dir="+profileDir,
		"--remote-debugging-port=0",
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-background-networking",
		"about:blank",
	)
	if err := cmd.Start(); err != nil {
		return nil, "", nil, fmt.Errorf("start browser: %w", err)
	}

	exited := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(exited)
	}()

	alive := func() bool {
		select {
		case <-exited:
			return false
		default:
			return true
		}
	}

	wsURL, err := readDevToolsURL(profileDir, 30*time.Second, alive)
	if err != nil {
		_ = cmd.Process.Kill()
		<-exited // let the reaper finish so the process is not left behind
		return nil, "", nil, err
	}
	return cmd, wsURL, exited, nil
}

// readDevToolsURL polls for the DevToolsActivePort file the browser writes on
// startup. Its first line is the port; its second is the browser WebSocket
// path. alive reports whether the browser is still running, so a browser that
// exits immediately fails fast instead of burning the full timeout.
func readDevToolsURL(profileDir string, timeout time.Duration, alive func() bool) (string, error) {
	path := filepath.Join(profileDir, "DevToolsActivePort")
	deadline := time.Now().Add(timeout)
	for {
		data, err := os.ReadFile(path)
		if err == nil {
			lines := strings.Split(strings.TrimSpace(string(data)), "\n")
			if len(lines) >= 2 {
				port := strings.TrimSpace(lines[0])
				wsPath := strings.TrimSpace(lines[1])
				if port != "" && wsPath != "" {
					return "ws://127.0.0.1:" + port + wsPath, nil
				}
			}
		}
		if !alive() {
			return "", errors.New("the browser exited before remote debugging became available")
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("the browser did not report a debugging port within %s", timeout)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
