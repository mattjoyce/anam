package read

import (
	"fmt"
	"time"

	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/receipt"
)

// parseAsOf reads the --as-of anam-time. A bare date means "as Anam knew it at the
// end of that day", which is deliberately NOT --to's rule: --to feeds an exclusive
// comparison and so rolls to the next midnight, while --as-of names one instant to read
// at. Rolling it forward too would silently read a day further than was asked for.
func parseAsOf(asOf string, loc *time.Location) (time.Time, error) {
	if asOf == "" {
		return time.Time{}, nil
	}
	t, dateOnly, err := parseInstant(asOf, loc)
	if err != nil {
		return time.Time{}, fmt.Errorf("--as-of: %w", err)
	}
	if dateOnly {
		t = t.AddDate(0, 0, 1).Add(-time.Nanosecond)
	}
	return t, nil
}

// bounds is a derived span over fact-time: an inclusive [from, to] the person
// names (ARCHITECTURE.md §4), held as clock.Layout strings so admission is the
// same lexical comparison the entries were sorted by.
type bounds struct {
	from string // inclusive lower, "" for unbounded
	toX  string // exclusive upper, "" for unbounded
}

// queryOf is the narrowing a read applies, as the receipt that would certify it
// records it: b's window, the snapshot at at, the type and the fields. Every read
// builds its query here and narrows through receipt.Query.Admits, so the timeline
// and a published read cannot narrow two ways (card 92). Nil when nothing narrows.
func queryOf(b bounds, at, typ string, fields []string) *receipt.Query {
	q := &receipt.Query{From: b.from, Before: b.toX, At: at, Type: typ, Fields: fields}
	if q.IsZero() {
		return nil
	}
	return q
}

// parseBoundsAt reads the fact-time flags. --at is a bound like --to (a bare date
// includes its whole day) and also asks for the snapshot shape, so the two exclude
// each other: one upper bound, and it says which read it wants.
func parseBoundsAt(from, to, at string, explain bool, loc *time.Location) (bounds, string, error) {
	if at != "" && to != "" {
		return bounds{}, "", fmt.Errorf("--at and --to both bound fact-time: give one")
	}
	if at != "" && explain {
		return bounds{}, "", fmt.Errorf("--explain shows the log's verdicts; to explain a snapshot, name a snapshot-shaped projection (e.g. --projection meds --explain)")
	}
	b, err := parseBounds(from, to, loc)
	if err != nil {
		return bounds{}, "", err
	}
	if at == "" {
		return b, "", nil
	}
	t, dateOnly, err := parseInstant(at, loc)
	if err != nil {
		return bounds{}, "", fmt.Errorf("--at: %w", err)
	}
	if dateOnly {
		t = t.AddDate(0, 0, 1).Add(-time.Nanosecond)
	}
	if b.from != "" && clock.Format(t) < b.from {
		return bounds{}, "", fmt.Errorf("--at %s is before --from", at)
	}
	return b, clock.Format(t), nil
}

func parseBounds(from, to string, loc *time.Location) (bounds, error) {
	var b bounds
	if from != "" {
		t, _, err := parseInstant(from, loc)
		if err != nil {
			return bounds{}, fmt.Errorf("--from: %w", err)
		}
		b.from = clock.Format(t)
	}
	if to != "" {
		t, dateOnly, err := parseInstant(to, loc)
		if err != nil {
			return bounds{}, fmt.Errorf("--to: %w", err)
		}
		// The interval is inclusive; the comparison is exclusive. A bare date rolls
		// to the next midnight so it includes its whole day; an instant nudges one
		// nanosecond — the wire format's full precision — so ≤t becomes <t'.
		if dateOnly {
			t = t.AddDate(0, 0, 1)
		} else {
			t = t.Add(time.Nanosecond)
		}
		b.toX = clock.Format(t)
	}
	return b, nil
}

// parseInstant reads a bound as a bare date in the reader's zone or an RFC 3339
// instant. A bare date is a day where the person stands, so "2025-12-23" is that day
// in loc, not the UTC day that overlaps it. The rule is clock.ParseBound's.
func parseInstant(s string, loc *time.Location) (t time.Time, dateOnly bool, err error) {
	return clock.ParseBound(s, loc)
}
