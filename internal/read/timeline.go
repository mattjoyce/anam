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
	"github.com/mattjoyce/anam/internal/frame"
	"github.com/mattjoyce/anam/internal/mint"
	"github.com/mattjoyce/anam/internal/projection"
	"github.com/mattjoyce/anam/internal/query"
	"github.com/mattjoyce/anam/internal/serve"
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

// NamedLocation is Location for a zone another machine named: an IANA name. Local is
// refused, since on this machine it would mean the daemon's zone, and the reader's is
// the one that counts.
func NamedLocation(zone string) (*time.Location, error) {
	if zone == "Local" {
		return nil, badQuery(errors.New("zone Local names the daemon's zone, not the reader's: send an IANA name"))
	}
	return Location(zone)
}

// Ledger is the ledger surface the reads need: the fold's, its identity, and the
// newest version of each facet for the rows a composed read shows.
type Ledger interface {
	fold.Ledger
	ID(ctx context.Context) (string, error)
	LatestVersions(ctx context.Context, ids []string) (map[string]int, error)
}

// Source is the record a read is taken from: the ledger, and when they are to hand,
// the checkpoint log and the values its checkpoints name. With them a read
// starts from the last mint run and replays only what came after; without them, or
// before any run, it folds the ledger from the beginning. The answer is the same.
type Source struct {
	Ledger      Ledger
	Checkpoints mint.WindowLog
	Values      mint.Store
	// Definitions are the ones the record's facets were admitted against: the
	// bundled ones and those importers registered. Nil means the bundled ones alone.
	Definitions *def.Registry
	// Projections are the templates a read may select: the bundled ones and those
	// the operator registered (card 125). Nil means the bundled ones alone.
	Projections *projection.Registry
	// Held is every Definition body by type and version, for a query's introspection
	// and its per-version warnings (card 173). Nil means the bundled ones and their
	// history alone.
	Held query.Held
}

// projections is src's templates, or the bundled ones when it names none.
func (src Source) projections() (*projection.Registry, error) {
	if src.Projections != nil {
		return src.Projections, nil
	}
	return projection.Bundled()
}

// definitions is src's Definitions, or the bundled ones when it names none.
func (src Source) definitions() (*def.Registry, error) {
	if src.Definitions != nil {
		return src.Definitions, nil
	}
	return def.Bundled()
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
	if err := checkType(q.Type); err != nil {
		return api.Timeline{}, err
	}
	know, err := knowledgeOf(q, loc)
	if err != nil {
		return api.Timeline{}, err
	}

	// Explain shows every version ever in force, the displaced ones included, so it
	// needs every facet's history: the full fold, always.
	if q.Explain {
		src.Checkpoints = nil
	}
	var v view
	switch {
	case narrow(src, q, b):
		v, err = readWindow(ctx, src, know, b)
	case snapshotPrefix(src, q):
		v, err = readSnapshotPrefix(ctx, src, know, at)
	default:
		v, err = readView(ctx, src, know, q.Projection, at)
	}
	if err != nil {
		return api.Timeline{}, err
	}
	if err := v.checkFields(q); err != nil {
		return api.Timeline{}, err
	}
	t := v.describe()
	rq := queryOf(b, at, q.Type, q.Fields)
	if q.Explain {
		return explain(ctx, src.Ledger, v, rq, t)
	}

	// The snapshot at at was taken by the read above; what is left is the window and
	// the type, through the predicate a receipt is verified with.
	var ids []string
	for _, e := range v.shown.Entries() {
		if rq.Admits(e.FactTime, string(e.Type)) {
			t.Entries = append(t.Entries, e)
			ids = append(ids, e.FacetID)
		}
	}
	if len(q.Fields) > 0 {
		// Fields only take away: the cut runs over the entries every lens above
		// admitted, and the answer is their frame (card 138).
		fr, err := frame.Of(t.Entries, q.Fields)
		if err != nil {
			return api.Timeline{}, badQuery(err)
		}
		t.Frame, t.Entries, t.Shown = &fr, nil, fr.Len()
		return t, nil
	}
	if t.Versions, err = v.versionsOf(ctx, src.Ledger, ids); err != nil {
		return api.Timeline{}, err
	}
	t.Shown = len(t.Entries)
	return t, nil
}

// checkFields refuses a query whose fields the read cannot cut: with explain, which
// shows verdicts and no payloads, or naming a field that the schema of a type the
// read can serve does not declare. The types the read can serve are the one it
// named, else the projection's allow-list, else every type with a Definition: the
// check depends on the query alone, never on what the record happens to hold.
func (v view) checkFields(q api.TimelineQuery) error {
	if len(q.Fields) == 0 {
		return nil
	}
	if q.Explain {
		return badQuery(errors.New("explain shows verdicts, not payloads; drop it to name fields"))
	}
	var types []facet.Type
	switch {
	case q.Type != "":
		types = coveredBy(q.Type, v.defs.All())
	case v.tmpl != nil:
		types = v.tmpl.Covers(v.defs.All())
	default:
		for _, d := range v.defs.All() {
			types = append(types, d.Type)
		}
	}
	for _, t := range types {
		d, ok := v.defs.Lookup(t)
		if !ok {
			return badQuery(fmt.Errorf("fields: no definition for type %s, so its fields cannot be named", t))
		}
		for _, p := range q.Fields {
			if err := d.HasPointer(p); err != nil {
				return badQuery(fmt.Errorf("fields: %w", err))
			}
		}
	}
	return nil
}

// checkType refuses a type parameter that is neither a facet type nor a prefix of one.
func checkType(pattern string) error {
	if pattern != "" && !facet.ValidPattern(pattern) {
		return badQuery(fmt.Errorf("type %q: want a facet type, or a prefix ending in a dot (activity.exercise-session.)", pattern))
	}
	return nil
}

// coveredBy returns the types a query's type pattern names: the type itself when it is
// exact, so a type with no Definition is still refused by name, else every Definition
// type under the prefix.
func coveredBy(pattern string, defs []def.Definition) []facet.Type {
	if !facet.IsPrefixPattern(pattern) {
		return []facet.Type{facet.Type(pattern)}
	}
	var out []facet.Type
	for _, d := range defs {
		if facet.Matches(d.Type, pattern) {
			out = append(out, d.Type)
		}
	}
	return out
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

	// whole is set when res.Value holds only the buckets a bounded read needed: the
	// size and hash of the whole value, for the header.
	whole *mint.Window
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
	if v.whole != nil {
		t.Live, t.Canonical = v.whole.Live, v.whole.Hash
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
func readView(ctx context.Context, src Source, know knowledge, projName, at string) (view, error) {
	defs, err := src.definitions()
	if err != nil {
		return view{}, err
	}
	v := view{defs: defs, at: at}

	res, err := readAtOrHead(ctx, src, know, &v)
	if err != nil {
		return view{}, err
	}
	v.res, v.shown = res, res.Value

	if projName != "" {
		templates, err := src.projections()
		if err != nil {
			return view{}, err
		}
		t, err := templates.Lookup(projName)
		if err != nil {
			return view{}, badQuery(err)
		}
		served, err := serve.Derive(res.Value, t, consent.Open{}, defs, at)
		if err != nil {
			return view{}, err
		}
		v.tmpl, v.shown = &t, served
	}

	if at != "" {
		// The snapshot is taken over whatever the lenses above produced: the
		// canonical, or the served variant. A served variant was derived from the
		// canonical cut at at, so over a snapshot shape this is the same value
		// again, not a double count.
		snap, err := fold.Snapshot(v.shown, at, defs.Closes)
		if err != nil {
			return view{}, err
		}
		v.shown = snap
	}
	return v, nil
}

// narrow reports whether q can be read from only the buckets its fact-time bounds
// overlap. A projection or a snapshot reads the whole value (a projection's served
// hash is over all of it, a snapshot's series can start anywhere earlier), and so does
// explain, which needs every facet's history.
func narrow(src Source, q api.TimelineQuery, b bounds) bool {
	return src.Checkpoints != nil && src.Values != nil &&
		q.Projection == "" && q.At == "" && !q.Explain && (b.from != "" || b.toX != "")
}

// snapshotPrefix reports whether q is a snapshot that can be read from the buckets up
// to its fact-time alone: one with no projection (a projection's served hash is over
// its whole value). A snapshot at D looks only at entries at or before D, and the
// prefix from the first bucket to D's holds every series' history up to D, the start
// of a series begun years earlier and any closing event included (card 64).
func snapshotPrefix(src Source, q api.TimelineQuery) bool {
	return src.Checkpoints != nil && src.Values != nil && q.At != "" && q.Projection == "" && !q.Explain
}

// readSnapshotPrefix reads the snapshot at fact-time at from the buckets up to at's,
// composed from checkpoints, with the whole value's size and hash for the header. It
// is fold.Snapshot over the prefix, which is fold.Snapshot over the whole value,
// because nothing after at can change what the snapshot keeps.
func readSnapshotPrefix(ctx context.Context, src Source, know knowledge, at string) (view, error) {
	t, err := clock.Parse(at)
	if err != nil {
		return view{}, err
	}
	v, err := readWindow(ctx, src, know, bounds{toX: clock.Format(t.Add(time.Nanosecond))})
	if err != nil {
		return view{}, err
	}
	snap, err := fold.Snapshot(v.shown, at, v.defs.Closes)
	if err != nil {
		return view{}, err
	}
	v.at, v.shown = at, snap
	return v, nil
}

// readWindow reads the buckets b overlaps, and the whole value's size and hash, at the
// head or the watermark an anam-time names. With no mint run to start from it is the
// full read.
func readWindow(ctx context.Context, src Source, know knowledge, b bounds) (view, error) {
	defs, err := src.definitions()
	if err != nil {
		return view{}, err
	}
	v := view{defs: defs}
	w, err := watermarkOf(ctx, src.Ledger, know, &v)
	if err != nil {
		return view{}, err
	}
	win, ok, err := mint.ReadWindow(ctx, src.Ledger, src.Checkpoints, src.Values, w, b.from, b.toX)
	if err != nil {
		return view{}, err
	}
	if !ok {
		return readView(ctx, src, know, "", "")
	}
	v.res = fold.Result{W: w, Value: win.Value}
	v.shown = win.Value
	v.whole = &win
	v.composed = &mint.Composed{W: w, Value: win.Value, Run: win.Run, Versions: win.Versions}
	return v, nil
}

// Canonical is the whole-person value at the ledger's head: composed from the last
// mint run and the tail after it when src has the checkpoint log, folded from the
// beginning when it does not. The result's States are empty when it was composed; a
// caller that needs a facet's history folds the ledger itself.
func Canonical(ctx context.Context, src Source) (fold.Result, error) {
	var v view
	return readAtOrHead(ctx, src, knowledge{}, &v)
}

// readAtOrHead reads the canonical at the head, or at the watermark an anam-time names,
// composed from checkpoints when src has them and a mint run is at or below it.
func readAtOrHead(ctx context.Context, src Source, know knowledge, v *view) (fold.Result, error) {
	l := src.Ledger
	w, err := watermarkOf(ctx, l, know, v)
	if err != nil {
		return fold.Result{}, err
	}
	if src.Checkpoints != nil && src.Values != nil {
		c, ok, err := mint.ReadAt(ctx, l, src.Checkpoints, src.Values, w)
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
