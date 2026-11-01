package app

import (
	"os"
	"strings"
	"testing"
)

func TestReadAlertBody(t *testing.T) {
	if _, err := readAlertBody(""); err == nil {
		t.Error("expected BODY_FILE_REQUIRED for an empty --file")
	}
	if _, err := readAlertBody("/no/such/file.json"); err == nil {
		t.Error("expected FILE_READ for a missing file")
	}
	file := t.TempDir() + "/alert.json"
	if err := os.WriteFile(file, []byte(`{"name":"cpu-high"}`), 0600); err != nil {
		t.Fatal(err)
	}
	body, err := readAlertBody(file)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"name"`) {
		t.Errorf("unexpected body: %s", body)
	}
	// A leading "@" is tolerated for consistency with --sql @file.
	if _, err := readAlertBody("@" + file); err != nil {
		t.Errorf("expected @path to read the file: %v", err)
	}
}
