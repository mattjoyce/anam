package api

import (
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/frame"
)

// TimelineQuery is a timeline read as the reader asked for it: the flags as typed,
// and the zone the reader stands in. Resolving a bare date to an instant is a
// function of the two, and it runs where the read runs, so there is one parser.
type TimelineQuery struct {
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
	At   string `json:"at,omitempty"`
	AsOf string `json:"as_of,omitempty"`
	// W reads at this watermark instead of the head; it excludes AsOf. Zero is the head.
	W          int64  `json:"w,omitempty"`
	Type       string `json:"type,omitempty"`
	Projection string `json:"projection,omitempty"`
	Explain    bool   `json:"explain,omitempty"`
	// Fields names the payload fields the read wants, as JSON pointers into the
	// type's schema (card 138): a temporary subset of the schema. With any, the
	// read answers with a Frame in place of Entries. Each must be declared by the
	// schema of every type the read can serve, or the read is refused.
	Fields []string `json:"fields,omitempty"`
	// Zone is the reader's zone, an IANA name. A bare date is a day in it.
	Zone string `json:"zone"`
}

// Timeline is one read of the person: what it was read at and through, the hashes
// that identify it, and the entries that survived the narrowing. With Explain it
// carries Verdicts instead of Entries.
type Timeline struct {
	W    int64 `json:"w"`
	Live int   `json:"live"`
	// Shown is how many entries or verdicts survived the narrowing.
	Shown int `json:"shown"`
	// AsOf is the anam-time the read was wound back to, resolved; empty at head.
	AsOf string `json:"as_of,omitempty"`
	// At is the snapshot's fact-time bound, resolved, and AtHash the snapshot's
	// hash; both empty for a log read.
	At     string `json:"at,omitempty"`
	AtHash string `json:"at_hash,omitempty"`
	// Canonical is the whole-person value's hash at W, whatever the narrowing.
	Canonical  string          `json:"canonical"`
	Projection *ProjectionRead `json:"projection,omitempty"`

	Entries []fold.Entry `json:"entries,omitempty"`
	// Versions is each shown entry's artifact version, by facet id.
	Versions map[string]int `json:"versions,omitempty"`
	// Frame is the answer when the query named Fields: the entries cut to those
	// fields, as columns and rows. Entries and Versions are then empty.
	Frame *frame.Frame `json:"frame,omitempty"`

	Verdicts []VerdictRow  `json:"verdicts,omitempty"`
	Counts   VerdictCounts `json:"counts,omitzero"`
}

// ProjectionRead names the projection a read went through and what came out of it.
type ProjectionRead struct {
	Name    string `json:"name"`
	Version int    `json:"version"`
	// Served is how many facets the projection served, and Hash their value's hash:
	// the one a receipt would record.
	Served int    `json:"served"`
	Hash   string `json:"hash"`
}

// VerdictRow is one artifact version and its standing in an explained read: served,
// excluded with the reason, or superseded.
type VerdictRow struct {
	FactTime  string `json:"fact_time"`
	Type      string `json:"type"`
	Version   int    `json:"version"`
	Standing  string `json:"standing"`
	Reason    string `json:"reason,omitempty"`
	SourceRef string `json:"source_ref"`
	FacetID   string `json:"facet_id"`
}

// VerdictCounts counts an explained read's verdicts by standing, before narrowing.
type VerdictCounts struct {
	Served     int `json:"served"`
	Excluded   int `json:"excluded"`
	Superseded int `json:"superseded"`
}
