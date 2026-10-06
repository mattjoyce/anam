// Package regimen holds the soft derivations over medication facts: computed when an
// agent asks, used, and dropped (SELF-LOG.md §6).
//
// A compliance estimate is a fact — Matt said it, on a date, about a period and a
// course — and so is a regimen's start, change and stop. An expected-dose schedule is
// neither: every number in it follows from those facts and carries nothing of its own,
// so it is never stored. Seeding a `took-it` facet per day at p = 0.95 would put 365
// rows in the ledger that say less than the two they derive from. Nothing in this
// package takes a ledger, writes a facet, or returns one; a test holds that the
// watermark does not move when a schedule is computed.
package regimen

import (
	"errors"
	"fmt"
	"time"
)

// ErrBadSchedule is returned when the inputs cannot form a schedule.
var ErrBadSchedule = errors.New("regimen: bad schedule")

// Schedule is the expected-dose view of one course over one period, from the
// regimen's cadence and the person's own estimate of adherence. Probable is a
// number, not a list of probable days: which days were missed is unknown, and a
// per-day probability would invent a precision the estimate does not have.
type Schedule struct {
	From, To time.Time // the period, inclusive of both days
	Days     int
	PerDay   int
	Expected int     // doses the regimen called for
	Estimate float64 // the person's adherence estimate, in (0, 1]
	Probable float64 // Expected × Estimate
}

// Expected derives the schedule. The period is in whole days, from and to inclusive,
// as the self-log records them; perDay is the regimen's cadence; estimate is the
// person's own figure.
func Expected(from, to time.Time, perDay int, estimate float64) (Schedule, error) {
	from, to = dayOf(from), dayOf(to)
	switch {
	case to.Before(from):
		return Schedule{}, fmt.Errorf("%w: period ends %s before it starts %s", ErrBadSchedule, to.Format(time.DateOnly), from.Format(time.DateOnly))
	case perDay < 1:
		return Schedule{}, fmt.Errorf("%w: %d doses per day", ErrBadSchedule, perDay)
	case estimate <= 0 || estimate > 1:
		return Schedule{}, fmt.Errorf("%w: estimate %v is outside (0, 1]", ErrBadSchedule, estimate)
	}
	days := int(to.Sub(from).Hours()/24) + 1
	expected := days * perDay
	return Schedule{
		From: from, To: to, Days: days, PerDay: perDay,
		Expected: expected, Estimate: estimate, Probable: float64(expected) * estimate,
	}, nil
}

func dayOf(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}
