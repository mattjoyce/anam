package span

import (
	"fmt"
	"time"
)

// Layout is a subdivision of fact-time into buckets: years, a split year into its
// months, a split month into its days. Performance decides when a bucket splits;
// calendar time decides where. A sparse year stays one bucket, a dense month becomes
// days, and a fact-time is still in exactly one bucket, found by walking down from
// its year. The checkpoint log carries the layout: a bucket is split once a checkpoint
// of one of its children exists, so the layout is derived from the log, never stored
// beside it.
//
// The zero Layout splits nothing: every bucket is a year.
type Layout struct{ split map[string]bool }

// LayoutOf is the layout a set of checkpointed bucket ids implies: every year with a
// month or a day among them is split, and every month with a day. A weekly id does
// not subdivide a year and is refused.
func LayoutOf(ids []string) (Layout, error) {
	l := Layout{split: map[string]bool{}}
	for _, id := range ids {
		b, err := Parse(id)
		if err != nil {
			return Layout{}, err
		}
		switch b.width {
		case Weekly:
			return Layout{}, fmt.Errorf("%w: %s is a week, which does not subdivide a year", ErrInvalidID, id)
		case Monthly:
			l.split[yearOf(b.from).ID()] = true
		case Daily:
			l.split[yearOf(b.from).ID()] = true
			l.split[monthOf(b.from).ID()] = true
		}
	}
	return l, nil
}

// Of is the bucket t falls in.
func (l Layout) Of(t time.Time) Bucket {
	y := yearOf(t)
	if !l.split[y.ID()] {
		return y
	}
	m := monthOf(t)
	if !l.split[m.ID()] {
		return m
	}
	return dayOf(t)
}

// Leaf reports whether b is a bucket of this layout: not split, and its parents are.
func (l Layout) Leaf(b Bucket) bool { return l.Of(b.from) == b }

// Split returns the layout with b divided into its children. A day cannot be divided,
// and ok is false for one.
func (l Layout) Split(b Bucket) (next Layout, ok bool) {
	if b.width != Yearly && b.width != Monthly {
		return l, false
	}
	next = Layout{split: make(map[string]bool, len(l.split)+1)}
	for k := range l.split {
		next.split[k] = true
	}
	next.split[b.ID()] = true
	return next, true
}

func yearOf(t time.Time) Bucket {
	utc := t.UTC()
	return Bucket{width: Yearly, year: utc.Year(), from: time.Date(utc.Year(), time.January, 1, 0, 0, 0, 0, time.UTC)}
}

func monthOf(t time.Time) Bucket {
	utc := t.UTC()
	return Bucket{width: Monthly, year: utc.Year(), n: int(utc.Month()),
		from: time.Date(utc.Year(), utc.Month(), 1, 0, 0, 0, 0, time.UTC)}
}

func dayOf(t time.Time) Bucket {
	utc := t.UTC()
	return Bucket{width: Daily, year: utc.Year(), n: int(utc.Month()), day: utc.Day(),
		from: time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC)}
}
