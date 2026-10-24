// Package clock owns Anam's two time concerns: a source of "now" that tests can
// control, and the single wire format for every timestamp Anam stores.
//
// Timestamps are formatted fixed-width, in UTC, at nanosecond precision, so that
// lexical order equals chronological order (ARCHITECTURE.md §8). Fact-time and
// anam-time are stored as text and range-queried with BETWEEN on that equivalence;
// nothing else guarantees it. (The rule outlived two substrate changes: any ordered
// text timestamp needs it, whatever stores it.)
//
// Consumers needing a clock declare their own single-method interface at the point of
// use (AGENTS.md); this package returns concrete types.
package clock

import (
	"fmt"
	"time"
)

// Layout is Anam's timestamp wire format: RFC 3339 with a fixed nine-digit fractional
// second.
//
// time.RFC3339Nano must never be substituted here. It elides trailing zeros, so
// "2026-01-24T06:30:00Z" sorts after "2026-01-24T06:30:00.5Z" ('Z' > '.'), inverting
// the order of any two facets whose fact-times differ only in sub-second precision.
const Layout = "2006-01-02T15:04:05.000000000Z07:00"

// Width is the number of bytes Format always returns. Fixed width is what makes the
// lexical comparison total: a shorter string can never sort against a longer one on a
// prefix. It follows from Layout and orders the record, so it is never configurable.
const Width = 30

// Format renders t as a fixed-width UTC timestamp.
func Format(t time.Time) string { return t.UTC().Format(Layout) }

// Parse reads a timestamp written by Format and returns it in UTC. It is strict: a
// timestamp missing its nine fractional digits is not one Anam wrote, and is rejected
// rather than silently accepted at a different width.
func Parse(s string) (time.Time, error) {
	t, err := time.Parse(Layout, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse timestamp %q: %w", s, err)
	}
	return t.UTC(), nil
}

// ParseBound reads a bound a person types: a bare date, or an RFC 3339 instant. A bare
// date is a day where the reader stands, so "2025-12-23" is the start of that day in
// loc (UTC when loc is nil), not the UTC day that overlaps it, and dateOnly says it
// was one so a caller can take the whole day. This is the one place a bare date gets
// its meaning: the API, the CLI and every Workbench page read a bound through it.
func ParseBound(s string, loc *time.Location) (t time.Time, dateOnly bool, err error) {
	if loc == nil {
		loc = time.UTC
	}
	if t, err := time.ParseInLocation(time.DateOnly, s, loc); err == nil {
		return t, true, nil
	}
	t, err = time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("%q is neither 2006-01-02 nor RFC 3339", s)
	}
	return t, false, nil
}

// Day reads a bare date and answers the start of that day in loc (UTC when loc is
// nil). It refuses an instant, which ParseBound accepts.
func Day(s string, loc *time.Location) (time.Time, error) {
	if loc == nil {
		loc = time.UTC
	}
	t, err := time.ParseInLocation(time.DateOnly, s, loc)
	if err != nil {
		return time.Time{}, fmt.Errorf("%q is not YYYY-MM-DD", s)
	}
	return t, nil
}

// Real reads the system clock.
type Real struct{}

// Now returns the current time in UTC.
func (Real) Now() time.Time { return time.Now().UTC() }

// Fixed is a clock frozen at an instant, for tests.
type Fixed struct{ Instant time.Time }

// Now returns the frozen instant in UTC.
func (f Fixed) Now() time.Time { return f.Instant.UTC() }
