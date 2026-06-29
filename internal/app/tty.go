package app

import (
	"os"

	"golang.org/x/term"
)

// stdinIsTTY reports whether standard input is an interactive terminal. Used to
// gate interactive prompts so non-TTY (agent / CI) callers fail structurally
// rather than hang waiting for input.
func stdinIsTTY() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// stderrIsTTY reports whether standard error is an interactive terminal. Used to
// keep agent-only nudges (skill / multi-context hints) off a human's screen.
func stderrIsTTY() bool {
	return term.IsTerminal(int(os.Stderr.Fd()))
}
