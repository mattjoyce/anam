package api

import (
	"fmt"
	"net/url"
	"strconv"
)

// CoverageQuery asks what the record holds, type by type (card 90). W stands the
// answer on that watermark, or AsOf on the one an anam-time names, read in Zone;
// neither is the head. Projection, when named, counts what that projection serves
// instead of the whole person.
type CoverageQuery struct {
	W          int64  `json:"w,omitempty"`
	AsOf       string `json:"as_of,omitempty"`
	Projection string `json:"projection,omitempty"`
	// Zone is the reader's zone, an IANA name; only as_of needs it.
	Zone string `json:"zone,omitempty"`
}

// Values encodes q for a GET.
func (q CoverageQuery) Values() url.Values {
	v := url.Values{}
	if q.W != 0 {
		v.Set("w", strconv.FormatInt(q.W, 10))
	}
	set(v, "as_of", q.AsOf)
	set(v, "projection", q.Projection)
	set(v, "zone", q.Zone)
	return v
}

// CoverageQueryOf decodes a coverage query from a GET's parameters.
func CoverageQueryOf(v url.Values) (CoverageQuery, error) {
	var w int64
	if s := v.Get("w"); s != "" {
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return CoverageQuery{}, fmt.Errorf("w %q: %w", s, err)
		}
		w = n
	}
	return CoverageQuery{W: w, AsOf: v.Get("as_of"), Projection: v.Get("projection"), Zone: v.Get("zone")}, nil
}

// Coverage is what the record holds at W: for each type, how many live facets, the
// earliest and latest fact-time among them, and the sources they came from. It says
// what is recorded, never that a source is complete: a quiet source may be current,
// and a gap in fact-time may be a gap in the world.
//
// The same W gives the same answer: Canonical is the whole-person value's hash at W,
// which the coverage is counted from (or the projection's served hash, when one was
// named).
type Coverage struct {
	W        int64  `json:"w"`
	LedgerID string `json:"ledger_id"`
	// Live is how many facets the counts cover: the whole person's live facets, or
	// what the projection served.
	Live       int             `json:"live"`
	Canonical  string          `json:"canonical"`
	Projection *ProjectionRead `json:"projection,omitempty"`
	Types      []TypeCoverage  `json:"types"`
}

// TypeCoverage is one type's share of the record, and each source's share of it.
// Earliest and Latest are fact-times in the clock's fixed-width layout. Projections
// are the sorted names of the projections, the newest version of each name the daemon
// holds, that serve the type: an empty list is a type no projection reaches, so the
// query route cannot answer it and only a whole-history read can (card 231). It does
// not depend on the W or the projection the coverage was counted through.
type TypeCoverage struct {
	Type        string           `json:"type"`
	Count       int              `json:"count"`
	Earliest    string           `json:"earliest"`
	Latest      string           `json:"latest"`
	Sources     []SourceCoverage `json:"sources"`
	Projections []string         `json:"projections"`
}

// SourceCoverage is what one source contributed to one type.
type SourceCoverage struct {
	Source   string `json:"source"`
	Count    int    `json:"count"`
	Earliest string `json:"earliest"`
	Latest   string `json:"latest"`
}
