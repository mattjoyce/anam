package fold

import "github.com/mattjoyce/anam/internal/facet"

// Snapshot is the last-known-value read (ARCHITECTURE.md §4; SELF-LOG.md §2): what
// was true at fact-time `at` for every series the value knows.
//
// Of the entries whose fact-time is at or before `at`, it keeps the latest in each
// series and drops a series whose latest entry is of a type that closes it, as
// `closes` reports from the Definitions. Entries without a series are not grouped:
// each one up to `at` is kept, which is the log shape they already had. The result is
// a Value, so it hashes and can be served through a projection and pinned by a receipt.
//
// It is a function of the value it is given. Taken over the canonical it is the
// snapshot at the head watermark; over an as-of read it is bitemporal; over a
// composition of hard checkpoints up to at's bucket it is the same answer, because
// every version of a facet lives in one bucket and the prefix holds every series'
// history (§7). `at` is a clock.Layout string, compared lexically like the entries'
// fact-times.
func Snapshot(v Value, at string, closes func(facet.Type) bool) (Value, error) {
	var kept []Entry
	latest := map[string]Entry{}
	for _, e := range v.Entries() {
		if e.FactTime > at {
			break // entries are in (fact_time, facet_id) order
		}
		if e.Series == "" {
			kept = append(kept, e)
			continue
		}
		// Later in the value is later in time, or the same instant and a higher id:
		// the value's own total order breaks the tie, so the answer is a function of
		// the value and not of the order the entries happened to arrive in.
		latest[e.Series] = e
	}
	for _, e := range latest {
		if closes != nil && closes(e.Type) {
			continue
		}
		kept = append(kept, e)
	}
	return NewValue(kept)
}
