// Package span computes a Facet's bucket: the unit Anam checkpoints (ARCHITECTURE.md
// §4, §7).
//
// A bucket is a window of fact-time. Every Facet is a member of exactly one, by
// arithmetic on its fact-time — there is no membership table and no lookup. Buckets
// tile: they partition fact-time with no overlaps and no gaps, which is what makes the
// whole-person view a plain union of bucket values rather than an interval-cover
// problem.
//
// A bucket is invisible above this package. Nobody reads one, names one, or bookmarks
// one. It exists only to bound how far a read must replay.
//
// Contrast the *derived* span (`[from, to]`, a saved bookmark, "everything"), which a
// person names and Anam never mints. And contrast **cadence** — how often a checkpoint
// is taken, which is a different knob entirely: width bounds the cost of *making* one,
// cadence the cost of *using* one.
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

// ErrInvalidWidth is returned by New for a width that is not one of the declared ones.
var ErrInvalidWidth = errors.New("invalid bucket width")

// Width is how much fact-time one bucket covers.
//
// It is a tuning knob and nothing more: it trades how many facets a checkpoint folds
// against how many values a long read must stitch. Narrow buckets make a re-mint cheap
// and a lifelong read expensive; wide ones do the reverse. Neither choice changes what
// a read returns.
type Width string

// The widths. Monthly and Weekly are the ones an operator can configure; Yearly and
// Daily exist for a Layout, which subdivides years into months and months into days.
const (
	Monthly Width = "monthly"
	Weekly  Width = "weekly"
	Yearly  Width = "yearly"
	Daily   Width = "daily"
)

// Default is the width Anam buckets at unless an operator says otherwise.
//
// Monthly, because of the density this record actually has: ~6,200 facets across 31
// months is ~200 to a bucket and 31 buckets to stitch, where weekly would be ~46 to a
// bucket and 135 to stitch. Fatter buckets win while a re-mint stays cheap.
//
// That reasoning is a measurement, not a law, and it does not survive a dense source.
// Garmin's `monitoring_*` and `stress` tables would put ~90,000 facets in a month —
// at which point one backfilled row re-mints all of them, and either the width narrows
// or re-mint becomes incremental (§7). Re-check this constant against the corpus before
// trusting it.
const Default = Monthly

// Valid reports whether w is a known width.
func (w Width) Valid() bool { return w == Monthly || w == Weekly }

var (
	weekPattern  = regexp.MustCompile(`^bucket:(\d{4})-W(\d{2})$`)
	monthPattern = regexp.MustCompile(`^bucket:(\d{4})-(\d{2})$`)
	yearPattern  = regexp.MustCompile(`^bucket:(\d{4})$`)
	dayPattern   = regexp.MustCompile(`^bucket:(\d{4})-(\d{2})-(\d{2})$`)
)

// Bucket is a half-open interval of fact-time, [From, To). It is a value: two buckets
// with the same id are the same bucket.
type Bucket struct {
	width Width
	year  int
	n     int // ISO week number, or calendar month
	day   int // day of the month, for a daily bucket
	from  time.Time
}

// Buckets tiles fact-time at one width. It is the bucketing an operator selected,
// held as a value: construct it once from configuration and pass it to whatever needs
// to place a facet.
//
// The width is checked once, in New, and the value carries the proof — so Of is total
// and has nothing to fall back to. That is the whole reason this is a type rather than
// a width handed to a package-level function. A bucket id is durable, self-describing,
// and plausible: a width that quietly defaulted would write real-looking ids that
// nothing downstream can tell from right ones.
//
// The zero Buckets is not a bucketing. New is the only constructor, and an id built
// from a zero value is one Parse refuses rather than one it misreads.
type Buckets struct{ width Width }

// New returns the bucketing of width w. It is the one place an unknown width is
// rejected; every Of afterwards is arithmetic.
func New(w Width) (Buckets, error) {
	if !w.Valid() {
		return Buckets{}, fmt.Errorf("%w: %q is neither %s nor %s", ErrInvalidWidth, w, Monthly, Weekly)
	}
	return Buckets{width: w}, nil
}

// Width reports the width this bucketing tiles at.
func (b Buckets) Width() Width { return b.width }

// Of returns the bucket containing t. The instant is normalised to UTC first, so a
// timestamp in any zone lands in the bucket its UTC instant belongs to.
func (b Buckets) Of(t time.Time) Bucket {
	utc := t.UTC()
	switch b.width {
	case Weekly:
		year, week := utc.ISOWeek()
		return Bucket{width: Weekly, year: year, n: week, from: mondayOf(utc)}
	case Monthly:
		return Bucket{
			width: Monthly, year: utc.Year(), n: int(utc.Month()),
			from: time.Date(utc.Year(), utc.Month(), 1, 0, 0, 0, 0, time.UTC),
		}
	}
	// Unreachable through New. Returning the zero bucket rather than guessing a width
	// keeps the failure loud: its id is "bucket:0000-00", which Parse rejects.
	return Bucket{}
}

// Parse reads an id written by ID, inferring the width from its shape.
//
// The id is self-describing on purpose. Bucket ids are stored — in the operational
// index today, in checkpoint identities tomorrow — and they outlive the setting that
// produced them. If Parse took the current width instead of reading it, changing
// Default would silently re-interpret every id already written.
func Parse(id string) (Bucket, error) {
	if m := weekPattern.FindStringSubmatch(id); m != nil {
		// The pattern guarantees four and two digits, so neither Atoi can fail.
		year, _ := strconv.Atoi(m[1])
		week, _ := strconv.Atoi(m[2])
		if week < 1 || week > weeksIn(year) {
			return Bucket{}, fmt.Errorf("%w: %q — ISO year %d has %d weeks", ErrInvalidID, id, year, weeksIn(year))
		}
		// 4 January is always in ISO week 1, whatever weekday it falls on.
		firstMonday := mondayOf(time.Date(year, time.January, 4, 0, 0, 0, 0, time.UTC))
		return Bucket{width: Weekly, year: year, n: week,
			from: firstMonday.AddDate(0, 0, 7*(week-1))}, nil
	}
	if m := monthPattern.FindStringSubmatch(id); m != nil {
		year, _ := strconv.Atoi(m[1])
		month, _ := strconv.Atoi(m[2])
		if month < 1 || month > 12 {
			return Bucket{}, fmt.Errorf("%w: %q — month %d is not 1..12", ErrInvalidID, id, month)
		}
		return Bucket{width: Monthly, year: year, n: month,
			from: time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)}, nil
	}
	if m := yearPattern.FindStringSubmatch(id); m != nil {
		year, _ := strconv.Atoi(m[1])
		return yearOf(time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC)), nil
	}
	if m := dayPattern.FindStringSubmatch(id); m != nil {
		year, _ := strconv.Atoi(m[1])
		month, _ := strconv.Atoi(m[2])
		day, _ := strconv.Atoi(m[3])
		t := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
		if month < 1 || month > 12 || t.Day() != day {
			return Bucket{}, fmt.Errorf("%w: %q is not a calendar day", ErrInvalidID, id)
		}
		return dayOf(t), nil
	}
	return Bucket{}, fmt.Errorf("%w: %q is none of bucket:YYYY, bucket:YYYY-MM, bucket:YYYY-MM-DD, bucket:YYYY-Www",
		ErrInvalidID, id)
}

// ID is the bucket's identity: "bucket:2026-07" monthly, "bucket:2026-W28" weekly.
//
// For a weekly bucket the year is the **ISO week-year**, which is not the calendar
// year: 2026-12-31 and 2027-01-01 are both in bucket:2026-W53. Building the id from the
// calendar year would split one week across two ids, and the partition would stop
// partitioning. A monthly bucket has no such trap — a calendar month never straddles
// its own year.
//
// The number is zero-padded because ids are compared lexically wherever they are stored
// or sorted. Unpadded, 7 sorts after 10.
func (b Bucket) ID() string {
	switch b.width {
	case Weekly:
		return fmt.Sprintf("bucket:%04d-W%02d", b.year, b.n)
	case Yearly:
		return fmt.Sprintf("bucket:%04d", b.year)
	case Daily:
		return fmt.Sprintf("bucket:%04d-%02d-%02d", b.year, b.n, b.day)
	}
	return fmt.Sprintf("bucket:%04d-%02d", b.year, b.n)
}

// Width reports the bucket's width.
func (b Bucket) Width() Width { return b.width }

// From is the first instant in the bucket, 00:00:00 UTC. Inclusive.
func (b Bucket) From() time.Time { return b.from }

// To is the first instant after the bucket. Exclusive, so every instant belongs to
// exactly one bucket.
//
// A month is not a fixed duration, which is why this is arithmetic on the date rather
// than an addition of some constant width. February and July are both one bucket.
func (b Bucket) To() time.Time {
	switch b.width {
	case Weekly:
		return b.from.AddDate(0, 0, 7)
	case Yearly:
		return b.from.AddDate(1, 0, 0)
	case Daily:
		return b.from.AddDate(0, 0, 1)
	}
	return b.from.AddDate(0, 1, 0)
}

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
