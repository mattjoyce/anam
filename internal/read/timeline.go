// Package read computes the values Anam's reads answer with (internal/api) from the
// stores. The CLI calls it on local files and the daemon calls it on its own handles,
// so a read means the same thing whichever process runs it.
package read

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/consent"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/projection"
)

// ErrQuery marks a query that cannot be read as asked: a malformed instant, flags
// that exclude each other, an unknown zone or projection. Asking again unchanged
// will fail again.
var ErrQuery = errors.New("query")

// queryError is an error the reader caused. It reads as the error it wraps, and is
// ErrQuery.
type queryError struct{ err error }

func (e queryError) Error() string        { return e.err.Error() }
func (e queryError) Unwrap() error        { return e.err }
func (e queryError) Is(target error) bool { return target == ErrQuery }

func badQuery(err error) error { return queryError{err} }

// Location loads the reader's zone a query names. It is an IANA name, or Local for
// a read on the reader's own machine.
func Location(zone string) (*time.Location, error) {
	if zone == "" {
		return nil, badQuery(errors.New("no zone: the reader's zone travels with the read"))
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return nil, badQuery(fmt.Errorf("zone %q: %w", zone, err))
	}
	return loc, nil
}

// Timeline performs the read q describes over l: at head or at an anam-time, through
// a projection or not, as a log or as the snapshot at a fact-time, narrowed by
// fact-time and type, with or without the verdicts.
func Timeline(ctx context.Context, l fold.Ledger, q api.TimelineQuery) (api.Timeline, error) {
	loc, err := Location(q.Zone)
	if err != nil {
		return api.Timeline{}, err
	}
	b, at, err := parseBoundsAt(q.From, q.To, q.At, q.Explain, loc)
	if err != nil {
		return api.Timeline{}, badQuery(err)
	}
	asOf, err := parseAsOf(q.AsOf, loc)
	if err != nil {
		return api.Timeline{}, badQuery(err)
	}

	v, err := readView(ctx, l, asOf, q.Projection, at)
	if err != nil {
		return api.Timeline{}, err
	}
	t := v.describe()
	if q.Explain {
		return explain(ctx, l, v, b, q.Type, t)
	}

	t.Versions = map[string]int{}
	for _, e := range v.shown.Entries() {
		if b.admits(e) && (q.Type == "" || string(e.Type) == q.Type) {
			t.Entries = append(t.Entries, e)
			t.Versions[e.FacetID] = v.res.States[e.FacetID].Version
		}
	}
	t.Shown = len(t.Entries)
	return t, nil
}

// view is one read of the person, optionally wound back, optionally pruned, and
// optionally taken as a snapshot.
type view struct {
	res fold.Result

	// tmpl is set only when a projection was named. shown is whichever value the
	// caller should render — the canonical, the served variant, or the snapshot of
	// either.
	tmpl   *projection.Template
	shown  fold.Value
	asOfAt string

	// at is the snapshot's fact-time bound when --at was given, else "".
	at string

	// defs is the bundled Definition set: which types close a series.
	defs *def.Registry
}

// describe is the read's header as a value: which watermark, which lens, and the
// hash that identifies what is being shown. When a projection is in force the served
// hash is the one a receipt would carry, so the viewer and the publisher can be
// checked against each other.
func (v view) describe() api.Timeline {
	t := api.Timeline{
		W: int64(v.res.W), Live: v.res.Value.Len(), AsOf: v.asOfAt, At: v.at,
		Canonical: v.res.Value.Hash(),
	}
	if v.at != "" {
		t.AtHash = v.shown.Hash()
	}
	if v.tmpl != nil {
		t.Projection = &api.ProjectionRead{
			Name: v.tmpl.Name, Version: v.tmpl.Version, Served: v.shown.Len(), Hash: v.shown.Hash(),
		}
	}
	return t
}

// readView performs the read the flags describe: at head or at an anam-time, through
// a projection or not, as a log or as the snapshot at a fact-time.
func readView(ctx context.Context, l fold.Ledger, asOfTime time.Time, projName, at string) (view, error) {
	defs, err := def.Bundled()
	if err != nil {
		return view{}, err
	}
	v := view{defs: defs, at: at}

	res, err := readAtOrHead(ctx, l, asOfTime, &v)
	if err != nil {
		return view{}, err
	}
	v.res, v.shown = res, res.Value

	if projName != "" {
		templates, err := projection.Bundled()
		if err != nil {
			return view{}, err
		}
		t, err := templates.Lookup(projName)
		if err != nil {
			return view{}, badQuery(err)
		}
		served, err := projection.Apply(res.Value, t, consent.Open{}, defs)
		if err != nil {
			return view{}, err
		}
		v.tmpl, v.shown = &t, served
	}

	if at != "" {
		// The snapshot is taken over whatever the lenses above produced: the
		// canonical, or the served variant. Over a served snapshot shape it is the
		// same value again, which is the right answer, not a double count.
		snap, err := fold.Snapshot(v.shown, at, defs.Closes)
		if err != nil {
			return view{}, err
		}
		v.shown = snap
	}
	return v, nil
}

func readAtOrHead(ctx context.Context, l fold.Ledger, asOfTime time.Time, v *view) (fold.Result, error) {
	if asOfTime.IsZero() {
		return fold.Read(ctx, l)
	}
	w, err := fold.WatermarkAt(ctx, l, asOfTime)
	if err != nil {
		return fold.Result{}, err
	}
	v.asOfAt = clock.Format(asOfTime)
	return fold.ReadAt(ctx, l, w)
}
