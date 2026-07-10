// Package span computes a Facet's bucket: the unit Anam checkpoints (ARCHITECTURE.md
// §4, §7).
//
// A bucket is one ISO week of event-time. Every Facet is a member of exactly one, by
// arithmetic on its event-time — there is no membership table and no lookup. Buckets
// tile: they partition event-time with no overlaps and no gaps, which is what makes the
// whole-person view a plain union of bucket values rather than an interval-cover
// problem.
//
// A bucket is invisible above this package. Nobody reads one, names one, or bookmarks
// one. It exists only to bound how far a read must replay, and its width is a tuning
// knob — it trades how many facets a checkpoint folds against how many values a long
// read must stitch.
//
// Contrast the *derived* span (`[from, to]`, a saved bookmark, "everything"), which a
// person names and Anam never mints.
package span

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"time"
)

// ErrInvalidID is returned by Parse for a malformed or non-existent bucket id.
var ErrInvalidID = errors.New("invalid bucket id")

// Width is a bucket's duration. Bounds are UTC, where a week is exactly seven days —
// under a DST-observing zone it would not be.
const Width = 7 * 24 * time.Hour

var idPattern = regexp.MustCompile(`^bucket:(\d{4})-W(\d{2})$`)

// Bucket is a half-open interval of event-time, [From, To), identified by its ISO
// week. It is a value: two buckets with the same id are the same bucket.
type Bucket struct {
	year int
	week int
	from time.Time
}

// Of returns the bucket containing t. The instant is normalised to UTC first, so a
// timestamp in any zone lands in the bucket its UTC instant belongs to.
func Of(t time.Time) Bucket {
	utc := t.UTC()
	year, week := utc.ISOWeek()
	return Bucket{year: year, week: week, from: mondayOf(utc)}
}

// Parse reads an id written by ID.
func Parse(id string) (Bucket, error) {
	m := idPattern.FindStringSubmatch(id)
	if m == nil {
		return Bucket{}, fmt.Errorf("%w: %q is not bucket:YYYY-Www", ErrInvalidID, id)
	}

	// The pattern guarantees four and two digits, so neither Atoi can fail.
	year, _ := strconv.Atoi(m[1])
	week, _ := strconv.Atoi(m[2])

	if week < 1 || week > weeksIn(year) {
		return Bucket{}, fmt.Errorf("%w: %q — ISO year %d has %d weeks", ErrInvalidID, id, year, weeksIn(year))
	}

	// 4 January is always in ISO week 1, whatever weekday it falls on.
	firstMonday := mondayOf(time.Date(year, time.January, 4, 0, 0, 0, 0, time.UTC))
	return Bucket{year: year, week: week, from: firstMonday.AddDate(0, 0, 7*(week-1))}, nil
}

// ID is the bucket's identity: "bucket:2026-W28".
//
// The year is the **ISO week-year**, which is not the calendar year: 2026-12-31 and
// 2027-01-01 are both in bucket:2026-W53. Building the id from the calendar year would
// split one week across two ids, and the partition would stop partitioning.
//
// The week is zero-padded because ids are compared lexically wherever they are stored
// or sorted. Unpadded, W7 sorts after W10.
func (b Bucket) ID() string { return fmt.Sprintf("bucket:%04d-W%02d", b.year, b.week) }

// From is the first instant in the bucket: Monday, 00:00:00 UTC. Inclusive.
func (b Bucket) From() time.Time { return b.from }

// To is the first instant after the bucket: the following Monday. Exclusive, so every
// instant belongs to exactly one bucket.
func (b Bucket) To() time.Time { return b.from.Add(Width) }

// mondayOf returns midnight UTC on the Monday of t's ISO week.
func mondayOf(t time.Time) time.Time {
	utc := t.UTC()
	midnight := time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC)
	// time.Weekday counts Sunday as 0; ISO counts Monday as the first day.
	offset := (int(midnight.Weekday()) + 6) % 7
	return midnight.AddDate(0, 0, -offset)
}

// weeksIn reports how many ISO weeks the ISO year has: 52, or 53 in a long year.
// 28 December always falls in the last ISO week of its ISO year, so its week number is
// the answer — no need to encode the leap rules.
func weeksIn(year int) int {
	_, week := time.Date(year, time.December, 28, 0, 0, 0, 0, time.UTC).ISOWeek()
	return week
}
