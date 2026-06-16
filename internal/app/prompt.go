package app

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

// promptLine writes a prompt to stderr and reads a line from stdin. When the
// user enters nothing, def is returned. Prompts go to stderr so stdout stays a
// clean data channel.
func promptLine(label, def string) (string, error) {
	if def != "" {
		fmt.Fprintf(os.Stderr, "%s [%s]: ", label, def)
	} else {
		fmt.Fprintf(os.Stderr, "%s: ", label)
	}
	line, err := readLine()
	if err != nil {
		return "", err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return def, nil
	}
	return line, nil
}

// promptSecret reads a secret from stdin. On an interactive terminal it reads
// without echoing; without a TTY (a pipe / script) it falls back to a plain
// line read so non-interactive setup still works — at the cost of the secret
// being echoed by whatever is feeding stdin.
func promptSecret(label string) (string, error) {
	fmt.Fprintf(os.Stderr, "%s: ", label)
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		line, err := readLine()
		return strings.TrimSpace(line), err
	}
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

// readLine reads one line from stdin a byte at a time, so it never buffers past
// the newline — leaving the descriptor intact for a subsequent
// term.ReadPassword call.
func readLine() (string, error) {
	var b strings.Builder
	buf := make([]byte, 1)
	for {
		n, err := os.Stdin.Read(buf)
		if n > 0 {
			if buf[0] == '\n' {
				break
			}
			if buf[0] != '\r' {
				b.WriteByte(buf[0])
			}
		}
		if err != nil {
			if b.Len() > 0 {
				return b.String(), nil
			}
			return "", err
		}
	}
	return b.String(), nil
}
