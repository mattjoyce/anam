package fold

import "github.com/mattjoyce/anam/internal/facet"

// The last-known-value read (ARCHITECTURE.md §4; SELF-LOG.md §2): what was true at a
// fact-time for every series the value knows.
//
// Of the entries in hand, the latest in each series is kept, and a series whose
// latest entry is of a type that closes it is dropped whole, as `closes` reports from
// the Definitions. Entries without a series are not grouped: each is kept, which is
// the log shape they already had. The relation is decided once, here, and both the
// value-shaped read (Snapshot) and the projection's snapshot shape are built on it,
// so an explanation cannot disagree with a result.

// Fate is what the snapshot decides for one entry. Kept is the zero value.
type Fate struct {
	// Displaced: a later entry of the same series exists, and By names it.
	Displaced bool
	// Closed: the entry is the latest of its series and its type closes the series.
	Closed bool
	By     string
}

// Kept reports whether the entry survives the snapshot.
func (f Fate) Kept() bool { return !f.Displaced && !f.Closed }

// Fates decides the snapshot for entries in value order, positionally. Later in the
// value is later in time, or the same instant and a higher id: the value's own total
// order breaks the tie, so the answer is a function of the value and not of the order
// the entries happened to arrive in.
func Fates(entries []Entry, closes func(facet.Type) bool) []Fate {
	fates := make([]Fate, len(entries))
	last := map[string]int{} // series → index of its latest entry so far
	for i, e := range entries {
		if e.Series == "" {
			continue
		}
		if j, seen := last[e.Series]; seen {
			fates[j] = Fate{Displaced: true, By: e.FacetID}
		}
		last[e.Series] = i
	}
	if closes != nil {
		for _, i := range last {
			if closes(entries[i].Type) {
				fates[i] = Fate{Closed: true, By: entries[i].FacetID}
			}
		}
	}
	return fates
}

// Snapshot is the read as a value: the entries at or before fact-time `at` (a
// clock.Layout string, "" for no bound) that Fates keeps. The result is a Value, so
// it hashes, and it is the same value whether taken over the canonical, over an
// as-of read (bitemporal), or over the composed prefix of hard checkpoints up to
// at's bucket, because every version of a facet lives in one bucket and the prefix
// holds every series' history (§7).
func Snapshot(v Value, at string, closes func(facet.Type) bool) (Value, error) {
	var upTo []Entry
	for _, e := range v.Entries() {
		if at != "" && e.FactTime > at {
			break // entries are in (fact_time, facet_id) order
		}
		upTo = append(upTo, e)
	}
	var kept []Entry
	for i, f := range Fates(upTo, closes) {
		if f.Kept() {
			kept = append(kept, upTo[i])
		}
	}
	return NewValue(kept)
}
