package timeutil

import (
	"testing"
	"time"
)

var fixedNow = time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)

func TestParseFlexDuration(t *testing.T) {
	cases := map[string]time.Duration{
		"15m": 15 * time.Minute,
		"2h":  2 * time.Hour,
		"1d":  24 * time.Hour,
		"1w":  7 * 24 * time.Hour,
		"30s": 30 * time.Second,
	}
	for in, want := range cases {
		got, err := ParseFlexDuration(in)
		if err != nil {
			t.Fatalf("ParseFlexDuration(%q) error: %v", in, err)
		}
		if got != want {
			t.Errorf("ParseFlexDuration(%q) = %v, want %v", in, got, want)
		}
	}
	if _, err := ParseFlexDuration("bogus"); err == nil {
		t.Error("expected error for bogus duration")
	}
}

func TestParseInstant(t *testing.T) {
	cases := []struct {
		in   string
		want time.Time
	}{
		{"now", fixedNow},
		{"now-1h", fixedNow.Add(-time.Hour)},
		{"now+30m", fixedNow.Add(30 * time.Minute)},
		{"2h", fixedNow.Add(-2 * time.Hour)},
		{"2026-06-16", time.Date(2026, 6, 16, 0, 0, 0, 0, time.UTC)},
		{"2026-06-16T10:00:00Z", time.Date(2026, 6, 16, 10, 0, 0, 0, time.UTC)},
		{"1700000000", time.Unix(1700000000, 0).UTC()},
	}
	for _, c := range cases {
		got, err := ParseInstant(c.in, fixedNow)
		if err != nil {
			t.Fatalf("ParseInstant(%q) error: %v", c.in, err)
		}
		if !got.Equal(c.want) {
			t.Errorf("ParseInstant(%q) = %v, want %v", c.in, got, c.want)
		}
	}
	if _, err := ParseInstant("definitely-not-a-time!!", fixedNow); err == nil {
		t.Error("expected error for unparseable instant")
	}
}

func TestEpochMagnitudeDetection(t *testing.T) {
	// Same wall-clock instant expressed in seconds / millis / micros must map
	// to the same time (to second precision).
	sec := int64(1700000000)
	cases := []int64{sec, sec * 1000, sec * 1_000_000}
	for _, n := range cases {
		got := epochToTime(n)
		if got.Unix() != sec {
			t.Errorf("epochToTime(%d).Unix() = %d, want %d", n, got.Unix(), sec)
		}
	}
}

func TestRangeResolve(t *testing.T) {
	t.Run("since", func(t *testing.T) {
		start, end, err := Range{Since: "1h", Now: fixedNow}.Resolve()
		if err != nil {
			t.Fatal(err)
		}
		if end != fixedNow.UnixMicro() {
			t.Errorf("end = %d, want %d", end, fixedNow.UnixMicro())
		}
		if start != fixedNow.Add(-time.Hour).UnixMicro() {
			t.Errorf("start = %d, want %d", start, fixedNow.Add(-time.Hour).UnixMicro())
		}
	})
	t.Run("from/to", func(t *testing.T) {
		start, end, err := Range{From: "now-2h", To: "now-1h", Now: fixedNow}.Resolve()
		if err != nil {
			t.Fatal(err)
		}
		if start >= end {
			t.Errorf("start %d should be < end %d", start, end)
		}
	})
	t.Run("from defaults to now", func(t *testing.T) {
		_, end, err := Range{From: "now-2h", Now: fixedNow}.Resolve()
		if err != nil {
			t.Fatal(err)
		}
		if end != fixedNow.UnixMicro() {
			t.Errorf("end = %d, want now %d", end, fixedNow.UnixMicro())
		}
	})
	t.Run("empty range errors", func(t *testing.T) {
		if _, _, err := (Range{Now: fixedNow}).Resolve(); err == nil {
			t.Error("expected error when no range given")
		}
	})
	t.Run("inverted range errors", func(t *testing.T) {
		if _, _, err := (Range{From: "now", To: "now-1h", Now: fixedNow}).Resolve(); err == nil {
			t.Error("expected error for end before start")
		}
	})
}
