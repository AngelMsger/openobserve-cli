// Package timeutil converts human-friendly time expressions into the
// microsecond Unix timestamps OpenObserve's search API expects.
//
// This is deliberately the CLI's job, not the agent's: hand-computing
// microsecond epochs is the single most error-prone part of calling the search
// API, so the CLI owns it and accepts forgiving inputs (--since 1h, RFC3339,
// bare dates, epoch seconds/millis/micros, and now±duration expressions).
package timeutil

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Range is an unresolved time window described by flags. Exactly one of Since
// or (From/To) is normally provided. Now, when zero, defaults to time.Now() —
// tests set it for determinism.
type Range struct {
	Since string
	From  string
	To    string
	Now   time.Time
}

// Resolve turns the Range into start/end microsecond timestamps. The window is
// validated to be non-empty and correctly ordered.
func (r Range) Resolve() (startMicros, endMicros int64, err error) {
	now := r.Now
	if now.IsZero() {
		now = time.Now()
	}

	var start, end time.Time
	switch {
	case r.Since != "":
		d, derr := ParseFlexDuration(r.Since)
		if derr != nil {
			return 0, 0, fmt.Errorf("invalid --since %q: %w", r.Since, derr)
		}
		if d <= 0 {
			return 0, 0, fmt.Errorf("invalid --since %q: must be a positive duration", r.Since)
		}
		end = now
		start = now.Add(-d)
	case r.From != "":
		start, err = ParseInstant(r.From, now)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid --from %q: %w", r.From, err)
		}
		if r.To != "" {
			end, err = ParseInstant(r.To, now)
			if err != nil {
				return 0, 0, fmt.Errorf("invalid --to %q: %w", r.To, err)
			}
		} else {
			end = now
		}
	default:
		return 0, 0, fmt.Errorf("no time range given: pass --since (e.g. 1h) or --from/--to")
	}

	if !end.After(start) {
		return 0, 0, fmt.Errorf("empty time range: end (%s) must be after start (%s)",
			end.Format(time.RFC3339), start.Format(time.RFC3339))
	}
	return ToMicros(start), ToMicros(end), nil
}

// ToMicros converts a time.Time to Unix microseconds.
func ToMicros(t time.Time) int64 { return t.UnixMicro() }

// FromMicros converts Unix microseconds back to a UTC time.Time.
func FromMicros(us int64) time.Time { return time.UnixMicro(us).UTC() }

var flexDurationRe = regexp.MustCompile(`^(\d+)\s*([smhdw])$`)

// ParseFlexDuration parses a duration string. In addition to Go's native units
// (s, m, h) it understands d (days) and w (weeks), e.g. "15m", "2h", "7d".
func ParseFlexDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if m := flexDurationRe.FindStringSubmatch(s); m != nil {
		n, _ := strconv.Atoi(m[1])
		switch m[2] {
		case "d":
			return time.Duration(n) * 24 * time.Hour, nil
		case "w":
			return time.Duration(n) * 7 * 24 * time.Hour, nil
		}
	}
	return time.ParseDuration(s)
}

var nowExprRe = regexp.MustCompile(`^now\s*([+-])\s*(.+)$`)

// ParseInstant parses a single point in time relative to now. It accepts:
//   - "now"                       → now
//   - "now-1h" / "now+30m"        → now ± duration (supports d/w units)
//   - a bare duration "1h" / "2d" → that long ago (now - duration)
//   - RFC3339 / RFC3339 with zone → as written
//   - "2006-01-02" (UTC midnight) and "2006-01-02 15:04:05" (UTC)
//   - an integer epoch in seconds, milliseconds or microseconds (auto-detected)
func ParseInstant(s string, now time.Time) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, fmt.Errorf("empty time")
	}
	if strings.EqualFold(s, "now") {
		return now, nil
	}
	if m := nowExprRe.FindStringSubmatch(s); m != nil {
		d, err := ParseFlexDuration(m[2])
		if err != nil {
			return time.Time{}, err
		}
		if m[1] == "-" {
			return now.Add(-d), nil
		}
		return now.Add(d), nil
	}
	// Bare duration → that long ago.
	if d, err := ParseFlexDuration(s); err == nil {
		return now.Add(-d), nil
	}
	// Absolute layouts.
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	if t, err := time.ParseInLocation("2006-01-02 15:04:05", s, time.UTC); err == nil {
		return t, nil
	}
	if t, err := time.ParseInLocation("2006-01-02", s, time.UTC); err == nil {
		return t, nil
	}
	// Integer epoch, magnitude-detected.
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return epochToTime(n), nil
	}
	return time.Time{}, fmt.Errorf("unrecognized time format (try RFC3339, 2006-01-02, an epoch, or now-1h)")
}

// epochToTime interprets an integer epoch whose unit is inferred from its
// magnitude: seconds (~1e9), milliseconds (~1e12), microseconds (~1e15) or
// nanoseconds (~1e18).
func epochToTime(n int64) time.Time {
	switch {
	case n < 1e12: // seconds
		return time.Unix(n, 0).UTC()
	case n < 1e15: // milliseconds
		return time.UnixMilli(n).UTC()
	case n < 1e18: // microseconds
		return time.UnixMicro(n).UTC()
	default: // nanoseconds
		return time.Unix(0, n).UTC()
	}
}
