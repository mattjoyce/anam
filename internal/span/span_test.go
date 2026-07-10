package span_test

import (
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/span"
)

func mustParse(t *testing.T, s string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return ts
}

// The ISO week-year is not the calendar year. 2026 has 53 ISO weeks, and the week
// straddling New Year belongs to 2026 — so 2026-12-31 and 2027-01-01 are the same
// bucket. Deriving the id from t.Year() instead of the year ISOWeek returns would
// split one week across two ids, and the partition would stop partitioning.
func TestOf_ISOWeekYearIsNotTheCalendarYear(t *testing.T) {
	t.Parallel()

	tests := []struct {
		instant string
		want    string
	}{
		{"2026-01-01T00:00:00Z", "bucket:2026-W01"}, // a Thursday: ISO week 1 of 2026
		{"2026-12-28T00:00:00Z", "bucket:2026-W53"}, // Monday, start of the 53rd week
		{"2026-12-31T23:59:59Z", "bucket:2026-W53"},
		{"2027-01-01T00:00:00Z", "bucket:2026-W53"}, // still 2026's week, not 2027's
		{"2027-01-03T23:59:59Z", "bucket:2026-W53"}, // Sunday, last instant of it
		{"2027-01-04T00:00:00Z", "bucket:2027-W01"}, // Monday, the next week
	}
	for _, tc := range tests {
		if got := span.Of(mustParse(t, tc.instant)).ID(); got != tc.want {
			t.Errorf("Of(%s).ID() = %q, want %q", tc.instant, got, tc.want)
		}
	}
}

// Bucket ids are compared lexically wherever they are stored or sorted, so the week
// number is zero-padded. Unpadded, W7 sorts after W10.
func TestID_LexicalOrderIsChronological(t *testing.T) {
	t.Parallel()

	early := span.Of(mustParse(t, "2026-02-16T00:00:00Z")) // W08
	late := span.Of(mustParse(t, "2026-03-09T00:00:00Z"))  // W11

	if early.ID() >= late.ID() {
		t.Errorf("lexical order broken: %q should sort before %q", early.ID(), late.ID())
	}
	if got, want := early.ID(), "bucket:2026-W08"; got != want {
		t.Errorf("week not zero-padded: got %q, want %q", got, want)
	}
}

// Every instant belongs to exactly one bucket: [From, To) is half-open, and the two
// bounds land in adjacent buckets.
func TestBounds_HalfOpenAndSevenDaysWide(t *testing.T) {
	t.Parallel()

	b := span.Of(mustParse(t, "2026-07-05T14:30:00Z")) // a Sunday

	if got, want := b.From().Format(time.RFC3339), "2026-06-29T00:00:00Z"; got != want {
		t.Errorf("From() = %s, want %s (the Monday)", got, want)
	}
	if got, want := b.To().Sub(b.From()), 7*24*time.Hour; got != want {
		t.Errorf("width = %s, want %s", got, want)
	}
	if span.Of(b.From()).ID() != b.ID() {
		t.Errorf("From() %s is not in its own bucket %s", b.From(), b.ID())
	}
	if span.Of(b.To()).ID() == b.ID() {
		t.Errorf("To() %s must be exclusive, but lands in %s", b.To(), b.ID())
	}
	if b.From().Location() != time.UTC || b.To().Location() != time.UTC {
		t.Error("bounds must be UTC: a week is exactly 7 days only without DST")
	}
}

// An instant in any zone lands in the bucket its UTC instant belongs to.
func TestOf_NormalisesToUTC(t *testing.T) {
	t.Parallel()

	// 2027-01-04T09:00:00+10:00 is 2027-01-03T23:00:00Z — still 2026-W53.
	zoned := time.Date(2027, 1, 4, 9, 0, 0, 0, time.FixedZone("AEST", 10*3600))
	if got, want := span.Of(zoned).ID(), "bucket:2026-W53"; got != want {
		t.Errorf("Of(%s).ID() = %q, want %q", zoned, got, want)
	}
}

func TestParse_RoundTrips(t *testing.T) {
	t.Parallel()

	for _, instant := range []string{
		"2026-01-01T00:00:00Z",
		"2026-07-05T14:30:00Z",
		"2027-01-01T00:00:00Z",
		"2024-02-29T12:00:00Z", // leap day
	} {
		want := span.Of(mustParse(t, instant))
		got, err := span.Parse(want.ID())
		if err != nil {
			t.Fatalf("Parse(%q): %v", want.ID(), err)
		}
		if got.ID() != want.ID() || !got.From().Equal(want.From()) || !got.To().Equal(want.To()) {
			t.Errorf("Parse(%q) = {%s %s %s}, want {%s %s %s}",
				want.ID(), got.ID(), got.From(), got.To(), want.ID(), want.From(), want.To())
		}
	}
}

func TestParse_Rejects(t *testing.T) {
	t.Parallel()

	for _, id := range []string{
		"",
		"2026-W28",          // no prefix
		"bucket:2026-W00",   // ISO weeks start at 1
		"bucket:2026-W54",   // no year has 54
		"bucket:2025-W53",   // 2025 has only 52 ISO weeks
		"bucket:2026-W7",    // unpadded
		"bucket:2026-28",    // no W
		"derived:2026-W28",  // not a bucket
		"bucket:2026-W28-x", // trailing junk
		"bucket:abcd-W28",   // year not numeric
	} {
		if _, err := span.Parse(id); err == nil {
			t.Errorf("Parse(%q) succeeded, want error", id)
		}
	}
}
