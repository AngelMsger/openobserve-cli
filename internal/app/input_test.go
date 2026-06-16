package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadInlineOrFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "q.sql")
	if err := os.WriteFile(file, []byte("  SELECT * FROM \"app\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Run("literal passes through", func(t *testing.T) {
		got, err := readInlineOrFile("SELECT 1")
		if err != nil || got != "SELECT 1" {
			t.Fatalf("got %q, err %v", got, err)
		}
	})

	t.Run("@file is read and trimmed", func(t *testing.T) {
		got, err := readInlineOrFile("@" + file)
		if err != nil {
			t.Fatal(err)
		}
		if got != `SELECT * FROM "app"` {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("escaped leading at is literal", func(t *testing.T) {
		got, err := readInlineOrFile(`\@notafile`)
		if err != nil || got != "@notafile" {
			t.Fatalf("got %q, err %v", got, err)
		}
	})

	t.Run("missing file is a usage error", func(t *testing.T) {
		if _, err := readInlineOrFile("@/no/such/file.sql"); err == nil {
			t.Fatal("expected an error for a missing file")
		}
	})
}
