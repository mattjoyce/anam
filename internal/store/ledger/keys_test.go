package ledger

import (
	"strings"
	"testing"
)

// Sort keys compare lexically, so lexical order must equal numeric order across every
// digit-count boundary a counter will actually cross. Unpadded keys invert at each
// power of ten: rec#10 < rec#9. This is the same defect family as the fixed-width
// timestamp rule in internal/clock, and it was verified live against DynamoDB Local
// (an unpadded v1..v12 keyspace queried descending returns v9).
func TestKeys_LexicalOrderIsNumericOrder(t *testing.T) {
	t.Parallel()

	boundaries := []struct{ lo, hi int64 }{
		{9, 10}, {99, 100}, {999, 1000}, {999999, 1000000},
	}
	for _, b := range boundaries {
		if recSK(b.lo) >= recSK(b.hi) {
			t.Errorf("recSK: %q must sort before %q", recSK(b.lo), recSK(b.hi))
		}
	}
	for _, b := range []struct{ lo, hi int }{{9, 10}, {99, 100}, {9999, 10000}} {
		if facetSK("f", b.lo) >= facetSK("f", b.hi) {
			t.Errorf("facetSK: %q must sort before %q", facetSK("f", b.lo), facetSK("f", b.hi))
		}
	}
}

// Every artifact version of a facet must share the prefix the prior-version lookup
// queries, and the prefix must not capture a different facet whose id extends this
// one's (the #v terminator is what prevents it).
func TestFacetPrefix_CoversOwnVersionsOnly(t *testing.T) {
	t.Parallel()

	if !strings.HasPrefix(facetSK("abc", 1), facetPrefix("abc")) {
		t.Errorf("facetSK(abc, 1) = %q does not extend facetPrefix(abc) = %q",
			facetSK("abc", 1), facetPrefix("abc"))
	}
	if strings.HasPrefix(facetSK("abcd", 1), facetPrefix("abc")) {
		t.Errorf("facetPrefix(abc) = %q wrongly captures facetSK(abcd, 1) = %q",
			facetPrefix("abc"), facetSK("abcd", 1))
	}
}
