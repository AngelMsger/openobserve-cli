package app

import (
	"io"
	"os"
	"strings"

	cerrors "github.com/angelmsger/openobserve-cli/internal/errors"
)

// readInlineOrFile resolves a flag value that may reference a file. A long SQL
// query or PromQL expression is awkward (and error-prone) to pass on the command
// line — escaping quotes, newlines and shell metacharacters trips up agents and
// humans alike — so any value starting with "@" is read from a file instead:
//
//	--sql @query.sql     read the query from query.sql
//	--sql @-             read the query from stdin
//
// A literal leading "@" can be escaped as "\@". Values without a leading "@" are
// returned unchanged.
func readInlineOrFile(value string) (string, error) {
	if strings.HasPrefix(value, `\@`) {
		return value[1:], nil
	}
	if !strings.HasPrefix(value, "@") {
		return value, nil
	}
	ref := value[1:]
	if ref == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", cerrors.Wrap(err, cerrors.CategoryUsage, "STDIN_READ",
				"could not read the query from stdin")
		}
		return strings.TrimSpace(string(data)), nil
	}
	data, err := os.ReadFile(ref)
	if err != nil {
		return "", cerrors.Wrap(err, cerrors.CategoryUsage, "FILE_READ",
			"could not read the query file "+ref).
			WithHint("--sql/--query accept @<path> to read from a file, or @- for stdin.")
	}
	return strings.TrimSpace(string(data)), nil
}
