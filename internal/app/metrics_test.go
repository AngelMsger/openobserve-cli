package app

import "testing"

func TestMicrosToSeconds(t *testing.T) {
	cases := map[int64]float64{
		1_700_000_000_000_000: 1_700_000_000,   // whole second
		1_700_000_000_500_000: 1_700_000_000.5, // sub-second precision preserved
		0:                     0,
	}
	for micros, want := range cases {
		if got := microsToSeconds(micros); got != want {
			t.Errorf("microsToSeconds(%d) = %v, want %v", micros, got, want)
		}
	}
}

func TestInstantSecondsDefaultsToNow(t *testing.T) {
	// An empty --time resolves to roughly now (within a generous window).
	got, err := instantSeconds("")
	if err != nil {
		t.Fatal(err)
	}
	if got < 1_700_000_000 {
		t.Fatalf("instantSeconds(\"\") = %v, expected a recent unix-seconds value", got)
	}
}

func TestInstantSecondsParsesRFC3339(t *testing.T) {
	got, err := instantSeconds("2023-11-14T22:13:20Z")
	if err != nil {
		t.Fatal(err)
	}
	if got != 1_700_000_000 {
		t.Fatalf("instantSeconds(rfc3339) = %v, want 1700000000", got)
	}
}
