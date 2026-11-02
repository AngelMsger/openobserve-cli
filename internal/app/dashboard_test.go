package app

import (
	"os"
	"strings"
	"testing"
)

func TestReadDashboardBody(t *testing.T) {
	if _, err := readDashboardBody(""); err == nil {
		t.Error("expected BODY_FILE_REQUIRED for an empty --file")
	}
	if _, err := readDashboardBody("/no/such/file.json"); err == nil {
		t.Error("expected FILE_READ for a missing file")
	}
	file := t.TempDir() + "/dash.json"
	if err := os.WriteFile(file, []byte(`{"title":"T"}`), 0600); err != nil {
		t.Fatal(err)
	}
	body, err := readDashboardBody(file)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"title"`) {
		t.Errorf("unexpected body: %s", body)
	}
	// A leading "@" is tolerated for consistency with --sql @file.
	if _, err := readDashboardBody("@" + file); err != nil {
		t.Errorf("expected @path to read the file: %v", err)
	}
}
