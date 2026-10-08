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
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/mint"
	"github.com/mattjoyce/anam/internal/projection"
	"github.com/mattjoyce/anam/internal/store/ledger"
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

// Ledger is the ledger surface the reads need: the fold's, its identity, and the
// newest version of each facet for the rows a composed read shows.
type Ledger interface {
	fold.Ledger
	ID(ctx context.Context) (string, error)
	LatestVersions(ctx context.Context, ids []string) (map[string]int, error)
}

// Source is the record a read is taken from: the ledger, and when they are to hand,
// the checkpoint log and the artifacts its checkpoints are filed in. With them a read
// starts from the last mint run and replays only what came after; without them, or
// before any run, it folds the ledger from the beginning. The answer is the same.
type Source struct {
	Ledger      Ledger
	Checkpoints mint.CheckpointLog
	Artifacts   mint.Store
}

// Timeline performs the read q describes over src: at head or at an anam-time, through
// a projection or not, as a log or as the snapshot at a fact-time, narrowed by
// fact-time and type, with or without the verdicts.
func Timeline(ctx context.Context, src Source, q api.TimelineQuery) (api.Timeline, error) {
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

	// Explain shows every version ever in force, the displaced ones included, so it
	// needs every facet's history: the full fold, always.
	if q.Explain {
		src.Checkpoints = nil
	}
	v, err := readView(ctx, src, asOf, q.Projection, at)
	if err != nil {
		return api.Timeline{}, err
	}
	t := v.describe()
	if q.Explain {
		return explain(ctx, src.Ledger, v, b, q.Type, t)
	}

	var ids []string
	for _, e := range v.shown.Entries() {
		if b.admits(e) && (q.Type == "" || string(e.Type) == q.Type) {
			t.Entries = append(t.Entries, e)
			ids = append(ids, e.FacetID)
		}
	}
	if t.Versions, err = v.versionsOf(ctx, src.Ledger, ids); err != nil {
		return api.Timeline{}, err
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

	// composed is set when the read was composed from checkpoints. res.States is then
	// empty, and a shown row's version comes from versionsOf.
	composed *mint.Composed
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
func readView(ctx context.Context, src Source, asOfTime time.Time, projName, at string) (view, error) {
	defs, err := def.Bundled()
	if err != nil {
		return view{}, err
	}
	v := view{defs: defs, at: at}

	res, err := readAtOrHead(ctx, src, asOfTime, &v)
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

// readAtOrHead reads the canonical at the head, or at the watermark an anam-time names,
// composed from checkpoints when src has them and a mint run is at or below it.
func readAtOrHead(ctx context.Context, src Source, asOfTime time.Time, v *view) (fold.Result, error) {
	l := src.Ledger
	var w ledger.Watermark
	var err error
	if asOfTime.IsZero() {
		w, err = l.Head(ctx)
	} else {
		w, err = fold.WatermarkAt(ctx, l, asOfTime)
		v.asOfAt = clock.Format(asOfTime)
	}
	if err != nil {
		return fold.Result{}, err
	}
	if src.Checkpoints != nil && src.Artifacts != nil {
		c, ok, err := mint.ReadAt(ctx, l, src.Checkpoints, src.Artifacts, w)
		if err != nil {
			return fold.Result{}, err
		}
		if ok {
			v.composed = &c
			return fold.Result{W: c.W, Value: c.Value}, nil
		}
	}
	return fold.ReadAt(ctx, l, w)
}

// versionsOf is the artifact version in force at the read's W of each facet in ids.
// A full fold knows every one. A composed read knows those its tail touched; the rest
// were untouched since the mint run, so their version is the newest the ledger holds,
// unless an event after W moved it, and then the first such event names it.
func (v view) versionsOf(ctx context.Context, l Ledger, ids []string) (map[string]int, error) {
	out := make(map[string]int, len(ids))
	if v.composed == nil {
		for _, id := range ids {
			out[id] = v.res.States[id].Version
		}
		return out, nil
	}
	var rest []string
	for _, id := range ids {
		if ver, ok := v.composed.Versions[id]; ok {
			out[id] = ver
		} else {
			rest = append(rest, id)
		}
	}
	latest, err := l.LatestVersions(ctx, rest)
	if err != nil {
		return nil, err
	}
	head, err := l.Head(ctx)
	if err != nil {
		return nil, err
	}
	later, err := l.Replay(ctx, v.res.W, head)
	if err != nil {
		return nil, err
	}
	atW := map[string]int{}
	for _, ev := range later {
		if _, seen := atW[ev.FacetID]; seen {
			continue
		}
		atW[ev.FacetID] = ev.ArtifactVersion
		if ev.Kind == facet.EventSuperseded {
			atW[ev.FacetID] = ev.PriorArtifactVersion
		}
	}
	for _, id := range rest {
		out[id] = latest[id]
		if ver, ok := atW[id]; ok {
			out[id] = ver
		}
	}
	return out, nil
}
