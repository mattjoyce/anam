package read

import (
	"strings"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/clock"
)

// The CLI's date handling is the part a person touches most and the part with the most
// room for an off-by-one, so the pure functions are pinned here. No ledger is involved:
// these are string-and-time functions, and testing them against a real ledger would
// be slower and prove less.

func TestParseInstant(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in       string
		want     time.Time
		dateOnly bool
		wantErr  bool
	}{
		{in: "2026-01-24", want: time.Date(2026, 1, 24, 0, 0, 0, 0, time.UTC), dateOnly: true},
		{in: "2026-01-24T06:36:00Z", want: time.Date(2026, 1, 24, 6, 36, 0, 0, time.UTC)},
		{in: "2026-01-24T06:36:00+10:00", want: time.Date(2026, 1, 24, 6, 36, 0, 0, time.FixedZone("", 10*3600))},
		{in: "24 January 2026", wantErr: true},
		{in: "2026-1-24", wantErr: true},
		{in: "", wantErr: true},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			t.Parallel()
			got, dateOnly, err := parseInstant(c.in, time.UTC)
			if c.wantErr {
				if err == nil {
					t.Fatalf("parseInstant(%q) was accepted", c.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseInstant(%q): %v", c.in, err)
			}
			if !got.Equal(c.want) {
				t.Errorf("parseInstant(%q) = %s, want %s", c.in, got, c.want)
			}
			if dateOnly != c.dateOnly {
				t.Errorf("parseInstant(%q) dateOnly = %v, want %v", c.in, dateOnly, c.dateOnly)
			}
		})
	}
}

// --to is inclusive to the operator and exclusive in the comparison. A bare date must
// include its whole day; an instant must include itself.
func TestParseBounds_UpperIsInclusiveToTheOperator(t *testing.T) {
	t.Parallel()

	// The bounds narrow through the receipt's own predicate, as every read does.
	admits := func(b bounds, s string) bool { return queryOf(b, "", "", nil).Admits(s, "") }

	t.Run("a bare --to includes its whole day", func(t *testing.T) {
		t.Parallel()
		b, err := parseBounds("", "2026-01-24", time.UTC)
		if err != nil {
			t.Fatalf("parseBounds: %v", err)
		}
		for _, in := range []string{
			"2026-01-24T00:00:00.000000000Z",
			"2026-01-24T23:59:59.999999999Z",
		} {
			if !admits(b, in) {
				t.Errorf("--to 2026-01-24 excluded %s, which is in that day", in)
			}
		}
		if admits(b, "2026-01-25T00:00:00.000000000Z") {
			t.Error("--to 2026-01-24 admitted the next midnight")
		}
	})

	t.Run("an instant --to includes itself", func(t *testing.T) {
		t.Parallel()
		b, err := parseBounds("", "2026-01-24T06:36:00Z", time.UTC)
		if err != nil {
			t.Fatalf("parseBounds: %v", err)
		}
		if !admits(b, "2026-01-24T06:36:00.000000000Z") {
			t.Error("--to at an instant excluded that very instant")
		}
		if admits(b, "2026-01-24T06:36:00.000000001Z") {
			t.Error("--to at an instant admitted one nanosecond later")
		}
	})

	t.Run("--from is inclusive", func(t *testing.T) {
		t.Parallel()
		b, err := parseBounds("2026-01-24T06:36:00Z", "", time.UTC)
		if err != nil {
			t.Fatalf("parseBounds: %v", err)
		}
		if !admits(b, "2026-01-24T06:36:00.000000000Z") {
			t.Error("--from excluded its own instant")
		}
		if admits(b, "2026-01-24T06:35:59.999999999Z") {
			t.Error("--from admitted one nanosecond earlier")
		}
	})

	t.Run("unbounded admits everything", func(t *testing.T) {
		t.Parallel()
		b, err := parseBounds("", "", time.UTC)
		if err != nil {
			t.Fatalf("parseBounds: %v", err)
		}
		for _, in := range []string{"1970-01-01T00:00:00.000000000Z", "2999-12-31T23:59:59.999999999Z"} {
			if !admits(b, in) {
				t.Errorf("unbounded excluded %s", in)
			}
		}
	})

	for _, bad := range []string{"last tuesday", "2026-1-24"} {
		if _, err := parseBounds(bad, "", time.UTC); err == nil {
			t.Errorf("parseBounds(--from %q, time.UTC) was accepted", bad)
		}
		if _, err := parseBounds("", bad, time.UTC); err == nil {
			t.Errorf("parseBounds(--to %q, time.UTC) was accepted", bad)
		}
	}
}

// --as-of's bare-date rule is deliberately NOT --to's. Both mean "include that day",
// but --to feeds an exclusive comparison (so it rolls to the next midnight) while
// --as-of names a single instant to read at (so it lands on the day's last nanosecond).
// Rolling --as-of to the next midnight would silently read a day further than asked.
func TestParseAsOf(t *testing.T) {
	t.Parallel()

	t.Run("a bare date lands on the end of that day", func(t *testing.T) {
		t.Parallel()
		got, err := parseAsOf("2026-07-10", time.UTC)
		if err != nil {
			t.Fatalf("parseAsOf: %v", err)
		}
		if want := "2026-07-10T23:59:59.999999999Z"; clock.Format(got) != want {
			t.Errorf("parseAsOf(2026-07-10) = %s, want %s", clock.Format(got), want)
		}
		// It must not reach into the next day, which is what --to's rule would do.
		if !got.Before(time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC)) {
			t.Error("the as-of instant reached into 2026-07-11")
		}
	})

	t.Run("an instant is taken as given", func(t *testing.T) {
		t.Parallel()
		got, err := parseAsOf("2026-07-10T20:00:00Z", time.UTC)
		if err != nil {
			t.Fatalf("parseAsOf: %v", err)
		}
		if want := "2026-07-10T20:00:00.000000000Z"; clock.Format(got) != want {
			t.Errorf("parseAsOf = %s, want %s", clock.Format(got), want)
		}
	})

	t.Run("empty means no as-of, not the zero instant as a bound", func(t *testing.T) {
		t.Parallel()
		got, err := parseAsOf("", time.UTC)
		if err != nil {
			t.Fatalf("parseAsOf(\"\"): %v", err)
		}
		if !got.IsZero() {
			t.Errorf("parseAsOf(\"\") = %s, want the zero time (readView treats it as 'at head')", got)
		}
	})

	t.Run("unparseable is refused, naming the flag", func(t *testing.T) {
		t.Parallel()
		_, err := parseAsOf("last tuesday", time.UTC)
		if err == nil {
			t.Fatal("parseAsOf(\"last tuesday\") was accepted")
		}
		if !strings.Contains(err.Error(), "--as-of") {
			t.Errorf("error %q does not name the flag", err)
		}
	})
}

// The rows come out of a map, so an order has to be imposed or the output flickers
// between runs. It is the canonical's order, with version breaking ties so a facet's
// history reads oldest-first in place.
func TestSortRows_ImposesTheCanonicalOrder(t *testing.T) {
	t.Parallel()

	rows := []api.VerdictRow{
		{FactTime: "2026-01-25T00:00:00.000000000Z", FacetID: "a", Version: 1},
		{FactTime: "2026-01-24T00:00:00.000000000Z", FacetID: "b", Version: 2},
		{FactTime: "2026-01-24T00:00:00.000000000Z", FacetID: "b", Version: 1},
		{FactTime: "2026-01-24T00:00:00.000000000Z", FacetID: "a", Version: 1},
	}
	sortRows(rows)

	want := []struct {
		id string
		v  int
	}{{"a", 1}, {"b", 1}, {"b", 2}, {"a", 1}}
	for i, w := range want {
		if rows[i].FacetID != w.id || rows[i].Version != w.v {
			t.Errorf("row %d = %s v%d, want %s v%d", i, rows[i].FacetID, rows[i].Version, w.id, w.v)
		}
	}
}

func TestParseBoundsAt(t *testing.T) {
	t.Parallel()

	// A bare date includes its whole day, like --to, and leaves the bounds alone.
	b, at, err := parseBoundsAt("2026-01-01", "", "2026-06-15", false, time.UTC)
	if err != nil {
		t.Fatalf("parseBoundsAt: %v", err)
	}
	if at != "2026-06-15T23:59:59.999999999Z" || b.toX != "" || b.from != "2026-01-01T00:00:00.000000000Z" {
		t.Errorf("at=%q from=%q toX=%q", at, b.from, b.toX)
	}

	// An instant is taken as given.
	_, at, err = parseBoundsAt("", "", "2026-06-15T12:00:00Z", false, time.UTC)
	if err != nil || at != "2026-06-15T12:00:00.000000000Z" {
		t.Errorf("instant: at=%q err=%v", at, err)
	}

	// Two upper bounds, --explain, a bad instant, and an --at before --from are refused.
	for name, c := range map[string]struct {
		from, to, at string
		explain      bool
	}{
		"at and to":      {"", "2026-07-01", "2026-06-15", false},
		"at and explain": {"", "", "2026-06-15", true},
		"bad at":         {"", "", "June", false},
		"at before from": {"2026-07-01", "", "2026-06-15", false},
	} {
		if _, _, err := parseBoundsAt(c.from, c.to, c.at, c.explain, time.UTC); err == nil {
			t.Errorf("%s: no error", name)
		}
	}

	// No --at: plain bounds, no snapshot.
	_, at, err = parseBoundsAt("", "2026-07-01", "", true, time.UTC)
	if err != nil || at != "" {
		t.Errorf("no at: at=%q err=%v", at, err)
	}
}
