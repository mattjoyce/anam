package clock_test

import (
	"sort"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/clock"
)

func TestFormatIsFixedWidthUTC(t *testing.T) {
	t.Parallel()

	sydney, err := time.LoadLocation("Australia/Sydney")
	if err != nil {
		t.Skipf("tzdata unavailable: %v", err)
	}

	tests := []struct {
		name string
		in   time.Time
		want string
	}{
		{"whole second", time.Date(2026, 1, 24, 6, 30, 0, 0, time.UTC), "2026-01-24T06:30:00.000000000Z"},
		{"half second", time.Date(2026, 1, 24, 6, 30, 0, 5e8, time.UTC), "2026-01-24T06:30:00.500000000Z"},
		{"full nanos", time.Date(2026, 1, 24, 6, 30, 0, 123456789, time.UTC), "2026-01-24T06:30:00.123456789Z"},
		{"one nano", time.Date(2026, 1, 24, 6, 30, 0, 1, time.UTC), "2026-01-24T06:30:00.000000001Z"},
		{"zero value", time.Time{}, "0001-01-01T00:00:00.000000000Z"},
		{"non-UTC zone is converted", time.Date(2026, 1, 24, 17, 30, 0, 0, sydney), "2026-01-24T06:30:00.000000000Z"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := clock.Format(tt.in)
			if got != tt.want {
				t.Errorf("Format() = %q, want %q", got, tt.want)
			}
			if len(got) != clock.Width {
				t.Errorf("Format() width = %d, want %d (fixed width)", len(got), clock.Width)
			}
		})
	}
}

// TestFormatLexicalOrderMatchesChronologicalOrder guards ARCHITECTURE.md §8: the
// ledger's range queries assume sorting the formatted strings sorts the instants.
// time.RFC3339Nano fails this test; that is precisely why Layout exists.
func TestFormatLexicalOrderMatchesChronologicalOrder(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 1, 24, 6, 30, 0, 0, time.UTC)
	instants := []time.Time{
		base.Add(-24 * time.Hour),
		base,
		base.Add(1),                      // +1ns
		base.Add(time.Millisecond),       // the case RFC3339Nano inverts
		base.Add(500 * time.Millisecond), // ditto
		base.Add(time.Second),
		base.Add(time.Minute),
		base.AddDate(1, 0, 0),
	}

	for i := range instants {
		for j := range instants {
			a, b := instants[i], instants[j]
			gotLexical := clock.Format(a) < clock.Format(b)
			wantChrono := a.Before(b)
			if gotLexical != wantChrono {
				t.Errorf("ordering disagrees for %s vs %s: lexical(<)=%v, chronological(Before)=%v",
					clock.Format(a), clock.Format(b), gotLexical, wantChrono)
			}
		}
	}

	// Sorting the strings must reproduce sorting the instants.
	formatted := make([]string, len(instants))
	for i, in := range instants {
		formatted[i] = clock.Format(in)
	}
	shuffled := append([]string(nil), formatted...)
	sort.Sort(sort.Reverse(sort.StringSlice(shuffled)))
	sort.Strings(shuffled)
	for i := range formatted {
		if shuffled[i] != formatted[i] {
			t.Fatalf("sorted formatted times diverge at %d: got %q, want %q", i, shuffled[i], formatted[i])
		}
	}
}

func TestParseRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   time.Time
	}{
		{"whole second", time.Date(2026, 1, 24, 6, 30, 0, 0, time.UTC)},
		{"full nanos", time.Date(2026, 1, 24, 6, 30, 0, 123456789, time.UTC)},
		{"epoch", time.Unix(0, 0).UTC()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := clock.Parse(clock.Format(tt.in))
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if !got.Equal(tt.in) {
				t.Errorf("round trip = %v, want %v", got, tt.in)
			}
			if got.Location() != time.UTC {
				t.Errorf("Parse() location = %v, want UTC", got.Location())
			}
		})
	}
}

func TestParseRejectsForeignFormats(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
	}{
		{"empty", ""},
		{"garbage", "not-a-time"},
		{"RFC3339 without fraction", "2026-01-24T06:30:00Z"},
		{"RFC3339Nano truncated fraction", "2026-01-24T06:30:00.5Z"},
		{"date only", "2026-01-24"},
		{"trailing text", "2026-01-24T06:30:00.000000000Z and more"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := clock.Parse(tt.in); err == nil {
				t.Errorf("Parse(%q) succeeded, want error", tt.in)
			}
		})
	}
}

func TestFixedClockIsFrozenAndUTC(t *testing.T) {
	t.Parallel()

	sydney, err := time.LoadLocation("Australia/Sydney")
	if err != nil {
		t.Skipf("tzdata unavailable: %v", err)
	}
	instant := time.Date(2026, 1, 24, 17, 30, 0, 0, sydney)
	c := clock.Fixed{Instant: instant}

	first, second := c.Now(), c.Now()
	if !first.Equal(second) {
		t.Errorf("Fixed.Now() moved: %v then %v", first, second)
	}
	if first.Location() != time.UTC {
		t.Errorf("Fixed.Now() location = %v, want UTC", first.Location())
	}
	if !first.Equal(instant) {
		t.Errorf("Fixed.Now() = %v, want the same instant as %v", first, instant)
	}
}

func TestRealClockIsUTCAndAdvances(t *testing.T) {
	t.Parallel()

	c := clock.Real{}
	got := c.Now()
	if got.Location() != time.UTC {
		t.Errorf("Real.Now() location = %v, want UTC", got.Location())
	}
	if time.Since(got) > time.Minute {
		t.Errorf("Real.Now() = %v, implausibly far from now", got)
	}
}

// A bare date is a day where the reader stands (card 236): the start of that day in
// the zone, not the UTC day that overlaps it. An instant is the instant it names.
func TestParseBound(t *testing.T) {
	t.Parallel()
	sydney, err := time.LoadLocation("Australia/Sydney")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		in       string
		loc      *time.Location
		want     time.Time
		dateOnly bool
		wantErr  bool
	}{
		{in: "2026-10-12", loc: sydney, want: time.Date(2026, 10, 11, 13, 0, 0, 0, time.UTC), dateOnly: true},
		{in: "2026-10-12", loc: time.UTC, want: time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC), dateOnly: true},
		{in: "2026-10-12", loc: nil, want: time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC), dateOnly: true},
		{in: "2026-10-12T05:00:00Z", loc: sydney, want: time.Date(2026, 10, 12, 5, 0, 0, 0, time.UTC)},
		{in: "12 Oct", loc: sydney, wantErr: true},
	}
	for _, c := range cases {
		got, dateOnly, err := clock.ParseBound(c.in, c.loc)
		if c.wantErr {
			if err == nil {
				t.Errorf("ParseBound(%q) was accepted", c.in)
			}
			continue
		}
		if err != nil || !got.Equal(c.want) || dateOnly != c.dateOnly {
			t.Errorf("ParseBound(%q) = %s %v %v, want %s %v", c.in, got, dateOnly, err, c.want, c.dateOnly)
		}
	}
	if _, err := clock.Day("2026-10-12T05:00:00Z", sydney); err == nil {
		t.Error("Day accepted an instant")
	}
}
