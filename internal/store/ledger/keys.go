package ledger

import "fmt"

// The ledger's keys are identity-first and fixed-width (ARCHITECTURE.md §8).
//
// Identity-first: an artifact's sort key carries its facet_id, never its fact_time.
// Fact-time is a value read out of the payload; keying on it would let a corrected
// row land at a fresh sort key, where the prior-version lookup cannot see it, and
// supersession would silently become duplication.
//
// Fixed-width: sort keys compare lexically, so every embedded integer is zero-padded.
// Unpadded, anam#10 < anam#9 and v10 < v9 — replay returns the wrong window and the
// prior-version lookup reads the ninth artifact as current from the tenth onward.
// Verified against DynamoDB Local before this package was written, not assumed.

// seqSK is the sort key of the watermark counter item, seeded at 0 by Init.
const seqSK = "seq"

// facetPrefix is the sort-key prefix of every artifact version of one logical facet.
// The prior-version lookup queries it descending, limit 1.
func facetPrefix(facetID string) string {
	return fmt.Sprintf("facet#%s#v", facetID)
}

// facetSK is an artifact version's sort key.
func facetSK(facetID string, version int) string {
	return fmt.Sprintf("%s%06d", facetPrefix(facetID), version)
}

// anamSK is an anam-event's sort key. int64 needs at most 19 digits; 20 keeps the
// width fixed for every value the counter can hold.
func anamSK(seq int64) string {
	return fmt.Sprintf("anam#%020d", seq)
}

// personPK is the partition key: one person, one timeline.
func personPK(personID string) string {
	return fmt.Sprintf("person#%s", personID)
}
