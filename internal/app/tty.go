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
