package update

import "testing"

func TestCompare(t *testing.T) {
	cases := []struct {
		current, latest string
		wantAvailable   bool
	}{
		{"0.1.0", "0.2.0", true},
		{"0.2.0", "0.2.0", false},
		{"0.2.0", "0.1.9", false},
		{"1.0.0", "1.0.1", true},
		{"v0.1.0", "v0.2.0", true}, // leading v tolerated
		{"dev", "0.2.0", false},    // non-release build → comparison skipped
		{"0.2.0", "garbage", false},
	}
	for _, c := range cases {
		st := compare(c.current, c.latest)
		if st.Available != c.wantAvailable {
			t.Errorf("compare(%q,%q).Available = %v, want %v (detail: %s)",
				c.current, c.latest, st.Available, c.wantAvailable, st.Detail)
		}
	}
}

func TestParse(t *testing.T) {
	if v, ok := parse("1.2.3"); !ok || v != [3]int{1, 2, 3} {
		t.Fatalf("parse(1.2.3) = %v, %v", v, ok)
	}
	if v, ok := parse("v0.2.0-rc1+meta"); !ok || v != [3]int{0, 2, 0} {
		t.Fatalf("parse with suffix = %v, %v", v, ok)
	}
	if _, ok := parse("dev"); ok {
		t.Fatal("parse(dev) should not be ok")
	}
}
