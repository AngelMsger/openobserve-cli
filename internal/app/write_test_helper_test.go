package app

import "os"

// writeTestFile writes content to path and fails anything silently less often
// than sprinkling os.WriteFile across tests.
func writeTestFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o600)
}
