package app

import (
	"strings"
	"testing"
)

func TestBuildRunSQL(t *testing.T) {
	t.Run("stream only", func(t *testing.T) {
		got, err := buildRunSQL("", "default", "", "desc")
		if err != nil {
			t.Fatal(err)
		}
		want := `SELECT * FROM "default" ORDER BY _timestamp DESC`
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
	t.Run("with where and asc", func(t *testing.T) {
		got, err := buildRunSQL("", "app", "level = 'ERROR'", "asc")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(got, `FROM "app"`) || !strings.Contains(got, "WHERE level = 'ERROR'") ||
			!strings.HasSuffix(got, "ORDER BY _timestamp ASC") {
			t.Errorf("unexpected sql: %q", got)
		}
	})
	t.Run("explicit sql wins", func(t *testing.T) {
		got, err := buildRunSQL("SELECT count(*) FROM \"x\"", "ignored", "ignored", "desc")
		if err != nil {
			t.Fatal(err)
		}
		if got != `SELECT count(*) FROM "x"` {
			t.Errorf("explicit --sql should pass through, got %q", got)
		}
	})
	t.Run("no stream no sql errors", func(t *testing.T) {
		if _, err := buildRunSQL("", "", "", "desc"); err == nil {
			t.Error("expected NO_QUERY error")
		}
	})
	t.Run("bad order errors", func(t *testing.T) {
		if _, err := buildRunSQL("", "s", "", "sideways"); err == nil {
			t.Error("expected BAD_ORDER error")
		}
	})
}

func TestBuildHistogramSQL(t *testing.T) {
	got := buildHistogramSQL("default", "level = 'ERROR'", "5 minute")
	for _, frag := range []string{
		"histogram(_timestamp, '5 minute')",
		`FROM "default"`,
		"WHERE level = 'ERROR'",
		"GROUP BY zo_sql_key",
		"ORDER BY zo_sql_key",
	} {
		if !strings.Contains(got, frag) {
			t.Errorf("histogram SQL missing %q in %q", frag, got)
		}
	}
}

func TestConvertInterval(t *testing.T) {
	cases := map[string]string{
		"30s": "30 second",
		"1m":  "1 minute",
		"5m":  "5 minute",
		"1h":  "1 hour",
		"1d":  "1 day",
		"":    "1 minute",
	}
	for in, want := range cases {
		got, err := convertInterval(in)
		if err != nil {
			t.Fatalf("convertInterval(%q) error: %v", in, err)
		}
		if got != want {
			t.Errorf("convertInterval(%q) = %q, want %q", in, got, want)
		}
	}
	if got, _ := convertInterval("10 second"); got != "10 second" {
		t.Errorf("worded interval should pass through, got %q", got)
	}
	if _, err := convertInterval("bogus"); err == nil {
		t.Error("expected error for bogus interval")
	}
}
