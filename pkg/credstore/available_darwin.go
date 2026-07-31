//go:build darwin

package credstore

import (
	"os/exec"
	"strings"
	"sync"
)

// keychainUsable reports whether macOS has a default keychain this process can
// write to.
//
// This probe exists to keep a destructive system modal off the user's screen.
// go-keyring stores secrets by running `security add-generic-password`. When the
// process's HOME has no login keychain, the Security framework does not simply
// return an error — it puts up a SecurityAgent dialog reading "A keychain cannot
// be found to store <account>", whose default button is "Reset To Defaults".
// That button resets the user's default keychain, and an app must never be able
// to provoke it. The command then exits 154, which surfaced as the useless
// "exit status 154".
//
// `security default-keychain` asks the same framework the same question without
// any UI: it prints the keychain path and exits 0, or exits non-zero with
// "A default keychain could not be found." Probing with it first lets the store
// route straight to its file fallback and never reach the dialog.
//
// The answer is cached: a default keychain does not appear or disappear during a
// run, and this sits in front of every secret read and write. A keychain created
// while the app is running is picked up on the next launch. Note this reports
// EXISTENCE, not unlockedness — a locked keychain still exists, and go-keyring's
// own error handling covers that case (macOS prompts to unlock, which is
// expected and non-destructive).
var keychainUsable = sync.OnceValue(func() bool {
	out, err := exec.Command("/usr/bin/security", "default-keychain").Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) != ""
})
