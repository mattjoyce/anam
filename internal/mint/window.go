package mint

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/span"
	"github.com/mattjoyce/anam/internal/store/ledger"
	"github.com/mattjoyce/anam/internal/store/ops"
)

// WindowLog is what a narrow read needs of the checkpoint log: a composed read's,
// plus each checkpoint's facet count.
type WindowLog interface {
	CheckpointLog
	CheckpointSizes(ctx context.Context) (map[ops.SpanVersion]int, error)
}

// Window is a read bounded by fact-time: the entries of the buckets the bounds
// overlap, at W, with what a reader is owed about the whole value, its size and hash.
type Window struct {
	W   ledger.Watermark
	Run ledger.Watermark
	// Value holds the entries of the overlapping buckets only; the caller narrows
	// them to its exact bounds.
	Value fold.Value
	// Live and Hash describe the whole value at W, as a full fold would.
	Live int
	Hash string
	// Versions is the version of each facet the tail touched and left live.
	Versions map[string]int
	// Decoded is how many buckets were decoded; the rest were only hashed.
	Decoded, Buckets int
}

// ReadWindow reads the buckets that overlap [from, toX) at w, from the latest mint run
// at or below w and the tail after it. Every other bucket is read as bytes only: its
// file is checked against its hash and streamed into the whole value's hash, never
// decoded. A bucket the tail dirtied is decoded and re-derived wherever it is, since
// its bytes have changed. from and toX are clock.Layout strings; empty is unbounded.
// ok is false when there is no mint run to start from.
//
// The whole value's bytes are its buckets' bytes in time order, joined: a value is a
// JSON array in (fact_time, facet_id) order, and buckets partition fact-time. So the
// hash streamed here is the hash the full fold computes.
func ReadWindow(ctx context.Context, l ReadLedger, log WindowLog, values Store,
	w ledger.Watermark, from, toX string,
) (Window, bool, error) {
	run, ok, err := startingRun(ctx, l, log, w)
	if err != nil || !ok {
		return Window{}, false, err
	}
	rows, buckets, ok, err := leavesAt(ctx, log, run)
	if err != nil || !ok {
		return Window{}, false, err
	}
	sizes, err := log.CheckpointSizes(ctx)
	if err != nil {
		return Window{}, false, err
	}
	tail, err := l.Replay(ctx, run, w)
	if err != nil {
		return Window{}, false, err
	}
	d, err := dirty(ctx, l, buckets, tail)
	if err != nil {
		return Window{}, false, err
	}

	order, err := bucketOrder(rows, d)
	if err != nil {
		return Window{}, false, err
	}
	win := Window{W: w, Run: run, Versions: d.versions, Buckets: len(order)}
	h := sha256.New()
	h.Write([]byte("["))
	wrote := false
	var shown []fold.Entry
	for _, b := range order {
		need := overlaps(b.bucket, from, toX)
		inner, entries, n, err := readBucket(ctx, values, b, d, need, sizes)
		if err != nil {
			return Window{}, false, declineIfMissing(err)
		}
		if entries != nil {
			win.Decoded++
		}
		win.Live += n
		if need {
			shown = append(shown, entries...)
		}
		if len(inner) > 0 {
			if wrote {
				h.Write([]byte(","))
			}
			h.Write(inner)
			wrote = true
		}
	}
	h.Write([]byte("]"))
	win.Hash = "sha256:" + hex.EncodeToString(h.Sum(nil))
	if win.Value, err = fold.NewValue(shown); err != nil {
		return Window{}, false, err
	}
	return win, true, nil
}

// dirtied is what a tail did to the buckets: the facets it touched, where each was
// before and is after, and the entries it left live.
type dirtied struct {
	touched  map[string]bool
	buckets  map[string]bool
	entries  map[string][]fold.Entry // by bucket id
	versions map[string]int
}

// dirty works out which buckets of the run's layout the tail changed. A touched facet
// left the bucket its version at the run put it in, found from the version the first tail event names as
// prior, and joined the bucket its version at w puts it in, if the tail leaves it live.
func dirty(ctx context.Context, l ReadLedger, buckets span.Layout, tail []facet.AnamEvent) (dirtied, error) {
	d := dirtied{touched: map[string]bool{}, buckets: map[string]bool{}, entries: map[string][]fold.Entry{},
		versions: map[string]int{}}
	first := map[string]facet.AnamEvent{}
	last := map[string]fold.State{}
	for _, ev := range tail {
		if _, seen := first[ev.FacetID]; !seen {
			first[ev.FacetID] = ev
		}
		st, err := fold.StateAfter(ev)
		if err != nil {
			return dirtied{}, err
		}
		last[ev.FacetID] = st
	}
	want := map[ledger.VersionRef]bool{}
	for id, ev := range first {
		d.touched[id] = true
		if prior := priorVersion(ev); prior > 0 {
			want[ledger.VersionRef{FacetID: id, Version: prior}] = true
		}
		if st := last[id]; st.Live() {
			want[ledger.VersionRef{FacetID: id, Version: st.Version}] = true
			d.versions[id] = st.Version
		}
	}
	refs := make([]ledger.VersionRef, 0, len(want))
	for r := range want {
		refs = append(refs, r)
	}
	facets, err := l.Versions(ctx, refs)
	if err != nil {
		return dirtied{}, fmt.Errorf("%w: %v", fold.ErrCorruptStream, err)
	}
	for _, f := range facets {
		id := buckets.Of(f.FactTime).ID()
		d.buckets[id] = true
		if st := last[f.ID]; st.Live() && st.Version == f.ArtifactVersion {
			d.entries[id] = append(d.entries[id], fold.EntryOf(f, st))
		}
	}
	return d, nil
}

// priorVersion is the version a facet had at the run, by the first event after it, or
// 0 when the facet did not exist then.
func priorVersion(ev facet.AnamEvent) int {
	switch ev.Kind {
	case facet.EventAdmitted:
		return 0
	case facet.EventSuperseded:
		return ev.PriorArtifactVersion
	default:
		return ev.ArtifactVersion
	}
}

// orderedBucket is one bucket in time order, with its checkpoint if it has one.
type orderedBucket struct {
	bucket span.Bucket
	row    *ops.CheckpointRow
}

// bucketOrder is every bucket a checkpoint or the tail names, in time order.
func bucketOrder(rows []ops.CheckpointRow, d dirtied) ([]orderedBucket, error) {
	seen := map[string]bool{}
	var out []orderedBucket
	add := func(id string, row *ops.CheckpointRow) error {
		if seen[id] {
			return nil
		}
		seen[id] = true
		b, err := span.Parse(id)
		if err != nil {
			return fmt.Errorf("mint: bucket %s: %w", id, err)
		}
		out = append(out, orderedBucket{bucket: b, row: row})
		return nil
	}
	for i := range rows {
		if err := add(rows[i].SpanID, &rows[i]); err != nil {
			return nil, err
		}
	}
	for id := range d.buckets {
		if err := add(id, nil); err != nil {
			return nil, err
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].bucket.From().Before(out[j].bucket.From()) })
	return out, nil
}

// overlaps reports whether b holds any fact-time in [from, toX).
func overlaps(b span.Bucket, from, toX string) bool {
	if from != "" && clock.Format(b.To()) <= from {
		return false
	}
	if toX != "" && clock.Format(b.From()) >= toX {
		return false
	}
	return true
}

// readBucket returns one bucket's array contents, without its brackets, for the whole
// hash, and how many facets it holds. It decodes the bucket, returning its entries,
// when the read shows it or the tail dirtied it; otherwise it only checks the value's
// bytes against their hash, and counts from the recorded size.
func readBucket(ctx context.Context, values Store, b orderedBucket, d dirtied, need bool, sizes map[ops.SpanVersion]int,
) (inner []byte, entries []fold.Entry, n int, err error) {
	id := b.bucket.ID()
	var raw []byte
	if b.row != nil {
		if raw, err = values.Get(ctx, Kind, b.row.ValueHash); err != nil {
			return nil, nil, 0, fmt.Errorf("mint: checkpoint %s v%d: %w", id, b.row.V, err)
		}
	}
	size, sized := 0, false
	if b.row != nil {
		size, sized = sizes[ops.SpanVersion{SpanID: id, V: b.row.V}]
	}
	if !need && !d.buckets[id] && sized {
		return trim(raw), nil, size, nil
	}

	entries = []fold.Entry{}
	if raw != nil {
		if err := json.Unmarshal(raw, &entries); err != nil {
			return nil, nil, 0, fmt.Errorf("mint: checkpoint %s v%d: decode value: %w", id, b.row.V, err)
		}
	}
	if !d.buckets[id] {
		return trim(raw), entries, len(entries), nil
	}
	kept := entries[:0]
	for _, e := range entries {
		if !d.touched[e.FacetID] {
			kept = append(kept, e)
		}
	}
	kept = append(kept, d.entries[id]...)
	v, err := fold.NewValue(kept)
	if err != nil {
		return nil, nil, 0, err
	}
	return trim(v.Bytes()), v.Entries(), v.Len(), nil
}

// trim strips a JSON array's brackets, leaving its elements as they are serialised.
func trim(b []byte) []byte {
	b = bytes.TrimSpace(b)
	if len(b) < 2 {
		return nil
	}
	return b[1 : len(b)-1]
}
