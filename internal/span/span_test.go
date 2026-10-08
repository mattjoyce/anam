package span_test

import (
	"errors"
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

// The width Anam actually buckets at. If this changes, the ids already written to
// facet_index change shape with it — hence the ops schema version, and hence Parse
// reading the width out of the id rather than assuming this one.
func TestDefault_IsMonthly(t *testing.T) {
	t.Parallel()
	if span.Default != span.Monthly {
		t.Errorf("Default = %q, want %q", span.Default, span.Monthly)
	}
	if got, want := bucketing(t, span.Default).Of(mustParse(t, "2026-07-11T05:54:00Z")).ID(), "bucket:2026-07"; got != want {
		t.Errorf("Of(Default).ID() = %q, want %q", got, want)
	}
}

// A month is not a fixed duration, so the bounds are date arithmetic rather than an
// addition of some constant width. February and July are both exactly one bucket.
func TestMonthly_BoundsAreHalfOpenAndVariableLength(t *testing.T) {
	t.Parallel()

	cases := []struct {
		instant  string
		from, to string
		days     int
	}{
		{"2026-07-11T05:54:00Z", "2026-07-01T00:00:00Z", "2026-08-01T00:00:00Z", 31},
		{"2026-02-14T23:59:59Z", "2026-02-01T00:00:00Z", "2026-03-01T00:00:00Z", 28},
		{"2024-02-29T12:00:00Z", "2024-02-01T00:00:00Z", "2024-03-01T00:00:00Z", 29}, // leap
		{"2026-12-31T23:59:59Z", "2026-12-01T00:00:00Z", "2027-01-01T00:00:00Z", 31},
	}
	for _, tc := range cases {
		b := bucketing(t, span.Monthly).Of(mustParse(t, tc.instant))
		if got := b.From().Format(time.RFC3339); got != tc.from {
			t.Errorf("Of(%s).From() = %s, want %s", tc.instant, got, tc.from)
		}
		if got := b.To().Format(time.RFC3339); got != tc.to {
			t.Errorf("Of(%s).To() = %s, want %s", tc.instant, got, tc.to)
		}
		if got := int(b.To().Sub(b.From()).Hours() / 24); got != tc.days {
			t.Errorf("Of(%s) is %d days, want %d", tc.instant, got, tc.days)
		}
		// Half-open: From is inside, To is the next bucket.
		if bucketing(t, span.Monthly).Of(b.From()).ID() != b.ID() {
			t.Errorf("From() %s is not in its own bucket %s", b.From(), b.ID())
		}
		if bucketing(t, span.Monthly).Of(b.To()).ID() == b.ID() {
			t.Errorf("To() %s must be exclusive, but lands in %s", b.To(), b.ID())
		}
		if b.From().Location() != time.UTC || b.To().Location() != time.UTC {
			t.Error("bounds must be UTC")
		}
	}
}

// Unlike an ISO week, a calendar month never straddles its own year — which is why the
// monthly id has no week-year trap. Pinned so nobody reintroduces one.
func TestMonthly_YearNeverStraddles(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct{ instant, want string }{
		{"2026-12-31T23:59:59Z", "bucket:2026-12"},
		{"2027-01-01T00:00:00Z", "bucket:2027-01"},
	} {
		if got := bucketing(t, span.Monthly).Of(mustParse(t, tc.instant)).ID(); got != tc.want {
			t.Errorf("Of(%s).ID() = %q, want %q", tc.instant, got, tc.want)
		}
	}
}

// The ISO week-year is not the calendar year. 2026 has 53 ISO weeks, and the week
// straddling New Year belongs to 2026 — so 2026-12-31 and 2027-01-01 are the same
// bucket. Deriving the id from t.Year() instead of the year ISOWeek returns would
// split one week across two ids, and the partition would stop partitioning.
func TestWeekly_ISOWeekYearIsNotTheCalendarYear(t *testing.T) {
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
		if got := bucketing(t, span.Weekly).Of(mustParse(t, tc.instant)).ID(); got != tc.want {
			t.Errorf("Of(Weekly, %s).ID() = %q, want %q", tc.instant, got, tc.want)
		}
	}
}

func TestWeekly_BoundsAreHalfOpenAndSevenDaysWide(t *testing.T) {
	t.Parallel()

	b := bucketing(t, span.Weekly).Of(mustParse(t, "2026-07-05T14:30:00Z")) // a Sunday

	if got, want := b.From().Format(time.RFC3339), "2026-06-29T00:00:00Z"; got != want {
		t.Errorf("From() = %s, want %s (the Monday)", got, want)
	}
	if got, want := b.To().Sub(b.From()), 7*24*time.Hour; got != want {
		t.Errorf("width = %s, want %s", got, want)
	}
	if bucketing(t, span.Weekly).Of(b.From()).ID() != b.ID() {
		t.Errorf("From() %s is not in its own bucket %s", b.From(), b.ID())
	}
	if bucketing(t, span.Weekly).Of(b.To()).ID() == b.ID() {
		t.Errorf("To() %s must be exclusive, but lands in %s", b.To(), b.ID())
	}
	if b.From().Location() != time.UTC || b.To().Location() != time.UTC {
		t.Error("bounds must be UTC: a week is exactly 7 days only without DST")
	}
}

// Bucket ids are compared lexically wherever they are stored or sorted, so the number
// is zero-padded. Unpadded, 7 sorts after 10.
func TestID_LexicalOrderIsChronological(t *testing.T) {
	t.Parallel()

	for _, w := range []span.Width{span.Monthly, span.Weekly} {
		early := bucketing(t, w).Of(mustParse(t, "2026-02-16T00:00:00Z"))
		late := bucketing(t, w).Of(mustParse(t, "2026-11-09T00:00:00Z"))
		if early.ID() >= late.ID() {
			t.Errorf("%s: lexical order broken: %q should sort before %q", w, early.ID(), late.ID())
		}
	}
	if got, want := bucketing(t, span.Monthly).Of(mustParse(t, "2026-02-16T00:00:00Z")).ID(), "bucket:2026-02"; got != want {
		t.Errorf("month not zero-padded: got %q, want %q", got, want)
	}
	if got, want := bucketing(t, span.Weekly).Of(mustParse(t, "2026-02-16T00:00:00Z")).ID(), "bucket:2026-W08"; got != want {
		t.Errorf("week not zero-padded: got %q, want %q", got, want)
	}
}

// An instant in any zone lands in the bucket its UTC instant belongs to.
func TestOf_NormalisesToUTC(t *testing.T) {
	t.Parallel()

	// 2027-01-01T09:00:00+10:00 is 2026-12-31T23:00:00Z — the previous month, and
	// (for weekly) still 2026-W53.
	zoned := time.Date(2027, 1, 1, 9, 0, 0, 0, time.FixedZone("AEST", 10*3600))
	if got, want := bucketing(t, span.Monthly).Of(zoned).ID(), "bucket:2026-12"; got != want {
		t.Errorf("Of(Monthly, %s).ID() = %q, want %q", zoned, got, want)
	}
	if got, want := bucketing(t, span.Weekly).Of(zoned).ID(), "bucket:2026-W53"; got != want {
		t.Errorf("Of(Weekly, %s).ID() = %q, want %q", zoned, got, want)
	}
}

// An id carries its own width. Bucket ids are stored — the operational index today,
// checkpoint identities tomorrow — and they outlive the setting that produced them, so
// changing Default must not silently re-interpret ids already written.
func TestParse_ReadsTheWidthOutOfTheID(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		id    string
		width span.Width
	}{
		{"bucket:2026-07", span.Monthly},
		{"bucket:2026-W28", span.Weekly},
	} {
		b, err := span.Parse(tc.id)
		if err != nil {
			t.Fatalf("Parse(%q): %v", tc.id, err)
		}
		if b.Width() != tc.width {
			t.Errorf("Parse(%q).Width() = %q, want %q", tc.id, b.Width(), tc.width)
		}
		if b.ID() != tc.id {
			t.Errorf("Parse(%q).ID() = %q — not a round trip", tc.id, b.ID())
		}
	}
}

func TestParse_RoundTrips(t *testing.T) {
	t.Parallel()

	for _, w := range []span.Width{span.Monthly, span.Weekly} {
		for _, instant := range []string{
			"2026-01-01T00:00:00Z",
			"2026-07-05T14:30:00Z",
			"2027-01-01T00:00:00Z",
			"2024-02-29T12:00:00Z", // leap day
		} {
			want := bucketing(t, w).Of(mustParse(t, instant))
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
}

func TestParse_Rejects(t *testing.T) {
	t.Parallel()

	for _, id := range []string{
		"",
		"2026-W28",          // no prefix
		"2026-07",           // no prefix
		"bucket:2026-W00",   // ISO weeks start at 1
		"bucket:2026-W54",   // no year has 54
		"bucket:2025-W53",   // 2025 has only 52 ISO weeks
		"bucket:2026-W7",    // unpadded
		"bucket:2026-00",    // months start at 1
		"bucket:2026-13",    // no year has 13 months
		"bucket:2026-7",     // unpadded
		"derived:2026-W28",  // not a bucket
		"bucket:2026-W28-x", // trailing junk
		"bucket:2026-02-30", // not a calendar day
		"bucket:abcd-W28",   // year not numeric
		"bucket:abcd-07",
	} {
		if _, err := span.Parse(id); err == nil {
			t.Errorf("Parse(%q) succeeded, want error", id)
		}
	}
}

// An unknown width is refused where it is named, not absorbed where it is used.
// Falling back would put a typo's facets in real-looking buckets under the wrong
// width — durable, plausible, and indistinguishable afterwards from correct ones.
func TestNew_RefusesAnUnknownWidth(t *testing.T) {
	t.Parallel()

	if _, err := span.New("fortnightly"); !errors.Is(err, span.ErrInvalidWidth) {
		t.Errorf("New(\"fortnightly\") = %v, want ErrInvalidWidth", err)
	}
	if span.Width("fortnightly").Valid() {
		t.Error("an unknown width reported itself valid")
	}
	for _, w := range []span.Width{span.Monthly, span.Weekly} {
		if !w.Valid() {
			t.Errorf("%q reported itself invalid", w)
		}
		if _, err := span.New(w); err != nil {
			t.Errorf("New(%q) = %v, want a bucketing", w, err)
		}
	}
}

// The zero Buckets is not a bucketing, and must not quietly behave like the default
// one. Its id is the one shape Parse refuses, so a Buckets that skipped New announces
// itself rather than writing plausible rows.
func TestZeroBuckets_ProducesAnIDParseRejects(t *testing.T) {
	t.Parallel()

	var b span.Buckets
	id := b.Of(mustParse(t, "2026-07-11T05:54:00Z")).ID()
	if id == bucketing(t, span.Default).Of(mustParse(t, "2026-07-11T05:54:00Z")).ID() {
		t.Fatalf("the zero Buckets produced the default's id %q", id)
	}
	if _, err := span.Parse(id); !errors.Is(err, span.ErrInvalidID) {
		t.Errorf("Parse(%q) = %v, want ErrInvalidID", id, err)
	}
}

// bucketing is New with the error asserted away — every test here uses a width it
// knows is valid, and the ones checking that New rejects call it directly.
func bucketing(t *testing.T, w span.Width) span.Buckets {
	t.Helper()
	b, err := span.New(w)
	if err != nil {
		t.Fatalf("span.New(%q): %v", w, err)
	}
	return b
}

// A layout walks year → month → day. It is derived from the checkpointed ids: a month
// splits its year, a day its month and year. Every id round-trips through Parse.
func TestLayout(t *testing.T) {
	t.Parallel()
	l, err := span.LayoutOf([]string{"bucket:2026", "bucket:2025-03", "bucket:2025-07-14", "bucket:2011"})
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		at   time.Time
		want string
	}{
		{time.Date(2011, 5, 1, 0, 0, 0, 0, time.UTC), "bucket:2011"},
		{time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), "bucket:2026"},
		{time.Date(2025, 3, 31, 23, 59, 59, 0, time.UTC), "bucket:2025-03"},
		{time.Date(2025, 8, 2, 0, 0, 0, 0, time.UTC), "bucket:2025-08"},
		{time.Date(2025, 7, 14, 12, 0, 0, 0, time.UTC), "bucket:2025-07-14"},
		{time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC), "bucket:2025-07-01"},
	} {
		b := l.Of(tt.at)
		if b.ID() != tt.want {
			t.Errorf("Of(%s) = %s, want %s", tt.at, b.ID(), tt.want)
		}
		back, err := span.Parse(b.ID())
		if err != nil || back != b || !l.Leaf(b) {
			t.Errorf("%s: Parse = %v, %v; leaf %v", b.ID(), back, err, l.Leaf(b))
		}
		if b.From().After(tt.at) || !b.To().After(tt.at) {
			t.Errorf("%s does not hold %s", b.ID(), tt.at)
		}
	}
	year, _ := span.Parse("bucket:2025")
	if l.Leaf(year) {
		t.Error("a split year is a leaf")
	}
	day, _ := span.Parse("bucket:2025-07-14")
	if _, ok := l.Split(day); ok {
		t.Error("a day was split")
	}
	if _, err := span.LayoutOf([]string{"bucket:2026-W28"}); err == nil {
		t.Error("a weekly id was accepted into a layout")
	}
}
