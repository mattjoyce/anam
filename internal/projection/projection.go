// Package projection publishes governed variants of the canonical
// (ARCHITECTURE.md §9).
//
// A projection is a versioned template that **prunes**. It has five knobs, all of them
// metadata: a facet-type allow-list, a fact-time window, an endorsement-status
// filter, a maximum classification, and a shape. It never reshapes a facet, never
// generates one, and never reads a payload — a facet is included or excluded whole.
//
// The shape is the one knob whose verdict depends on other entries. A log serves
// every surviving facet; a snapshot serves the latest per series, the last-known
// value at the window's end. Shape lives in the template and not in
// a read-time flag, because a consumer selects a projection and never defines one.
//
// Two things here are structural rather than conventional, because a rule that lives
// only in a doc comment is a rule the next change can break silently:
//
//   - Apply consumes a fold.Value and returns a fold.Value, rebuilt from a subset of
//     the entries it was given. There is no code path that could emit an entry the
//     canonical did not contain, and the served serialisation is the canonical's own
//     rather than a second one that could drift from it.
//   - The consent gate (§10) is a parameter of Apply, so a serve path cannot be
//     written that forgets to pass through it.
//
// Templates are bundled with the binary, the floor, or registered by the operator over
// the API (card 125, 2026-10-13) and kept in the operational store. A consumer selects
// one by name and never defines one; defining is the operator's act, not a release.
// Every version ever held stays readable, because a receipt names a version and
// verify re-prunes through exactly that one.
package projection

import (
	"errors"
	"fmt"
	"sort"

	"github.com/mattjoyce/anam/internal/consent"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/fold"
)

// ErrBadTemplate is returned for a manifest that cannot form a Template.
var ErrBadTemplate = errors.New("projection: bad template")

// ErrUnknown is returned when a name matches no bundled template.
var ErrUnknown = errors.New("projection: unknown projection")

// Gate is the consent seam, declared where it is used. consent.Open satisfies it.
type Gate interface {
	Evaluate(consent.Request) consent.Decision
}

// Series is the Definitions' word a snapshot shape needs: which types close a
// series. *def.Registry satisfies it. A log-shaped template never consults it.
type Series interface {
	Closes(facet.Type) bool
}

// Shape says what a projection returns: every surviving facet, or one per series.
type Shape string

// The shapes.
const (
	ShapeLog      Shape = "log"
	ShapeSnapshot Shape = "snapshot"
)

// Valid reports whether s is a known shape.
func (s Shape) Valid() bool { return s == ShapeLog || s == ShapeSnapshot }

// Template is one versioned projection: a name, a version, and four metadata filters.
//
// Every filter is required in the manifest, and the two list filters deny by default —
// an empty Types or Statuses serves nothing. §9 says deny-by-default of the type
// allow-list; the same reasoning is applied to statuses here, because a governance
// template that can be *under*-specified into over-serving is the failure governance
// exists to prevent. The fact-time window is the exception: it only ever narrows, and
// the canonical is already the ceiling, so unbounded is a safe default.
type Template struct {
	Name        string
	Version     int
	Description string

	// Types is the facet-type allow-list: the exact types the manifest names.
	Types []facet.Type

	// Prefixes are the manifest's `types` entries that end in ".", each admitting every
	// type under it at a segment boundary (card 193). A type created later is served
	// without a new version of the template. Types and Prefixes together are the
	// allow-list; Covers resolves it against the Definitions held.
	Prefixes []string

	// Statuses is the endorsement-status filter. Only live statuses can appear in a
	// canonical, so in practice this chooses between "admitted facts" and "facts a
	// person has endorsed" — the distinction R9's proposed insights will need.
	Statuses []facet.Status

	// MaxClassification is the ceiling, compared as an ordinal. Anam is not judging
	// how sensitive a facet is: the Definition set the level at capture, and the
	// template sets the ceiling. This is a comparison, not an assessment.
	MaxClassification facet.Classification

	// From and To bound fact-time, inclusive and exclusive respectively, as
	// fixed-width clock.Layout strings so the comparison is the lexical one the
	// canonical is already ordered by. Empty means unbounded.
	From string
	To   string

	// Shape is log or snapshot. A snapshot with no To is the last-known value now;
	// with one, the last-known value at or before then — the coordinate was already
	// there (journal: two-clocks).
	Shape Shape

	// Hash is the content hash of the manifest as canonical JSON, "sha256:…": what a
	// receipt pins beside the version, so a body swapped under the same version
	// cannot pass as it. Body is that canonical JSON, what the API publishes.
	Hash string
	Body []byte

	// Inline is true for a template a query carried in its request (card 211), which
	// no registry holds and no operator named. Its body is filed by hash when its
	// receipt is, and a receipt for it says so.
	Inline bool

	// typeSet and statusSet are the allow-lists as sets, built once at parse.
	typeSet   map[facet.Type]bool
	statusSet map[facet.Status]bool
}

// Span describes the derived span a template serves, for the receipt to record (§9).
// It is the template's fact-time window rendered as one deterministic string, so a
// receipt stays self-describing when read years later without the binary that made it.
func (t Template) Span() string {
	if t.From == "" && t.To == "" {
		return "whole-person"
	}
	from, to := t.From, t.To
	if from == "" {
		from = "-inf"
	}
	if to == "" {
		to = "+inf"
	}
	return fmt.Sprintf("fact-time[%s,%s)", from, to)
}

// Reason says why one entry is or is not served. It exists so that a projection can
// be *explained* — "why is this reading not in the bp view?" is the question a
// governed system has to be able to answer, and answering it from a second, parallel
// re-implementation of the filter would produce an explanation that could disagree
// with the filter. Apply and Explain call the same verdict, so they cannot.
type Reason string

// The verdicts. Served is the only one that includes an entry.
const (
	ReasonServed         Reason = "served"
	ReasonType           Reason = "type not in the allow-list"
	ReasonStatus         Reason = "status not in the filter"
	ReasonClassification Reason = "above the classification ceiling"
	ReasonBeforeWindow   Reason = "fact-time before the window"
	ReasonAfterWindow    Reason = "fact-time at or after the window"
	ReasonConsent        Reason = "denied by the consent gate"
	ReasonDisplaced      Reason = "a later statement in the series exists"
	ReasonClosed         Reason = "the series is closed"
)

// Served reports whether this verdict includes the entry.
func (r Reason) Served() bool { return r == ReasonServed }

// admitsType reports whether typ is on the allow-list: named exactly, or under a prefix.
func (t Template) admitsType(typ facet.Type) bool {
	if t.typeSet[typ] {
		return true
	}
	for _, p := range t.Prefixes {
		if facet.Matches(typ, p) {
			return true
		}
	}
	return false
}

// Covers returns the types the template serves among the Definitions held: every
// exact entry, and every Definition type under a prefix. Exact entries are returned
// whether or not a Definition holds them, as before. The result is sorted and free of
// duplicates. Nothing but a Definition can bring a type into being, so this is the set
// a reader can be served.
func (t Template) Covers(defs []def.Definition) []facet.Type {
	seen := make(map[facet.Type]bool, len(t.Types))
	out := make([]facet.Type, 0, len(t.Types))
	for _, ft := range t.Types {
		if !seen[ft] {
			seen[ft] = true
			out = append(out, ft)
		}
	}
	for _, d := range defs {
		if !seen[d.Type] && t.admitsType(d.Type) {
			seen[d.Type] = true
			out = append(out, d.Type)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// Patterns returns the `types` entries as the manifest wrote them, exact types then
// prefixes, for display: a prefix is shown as a prefix, not expanded.
func (t Template) Patterns() []string {
	out := make([]string, 0, len(t.Types)+len(t.Prefixes))
	for _, ft := range t.Types {
		out = append(out, string(ft))
	}
	return append(out, t.Prefixes...)
}

// verdict applies the template's four filters to one entry and says which, if any,
// excluded it. It reads only governing metadata; Entry.Payload is never consulted (§9).
//
// The order is the order the knobs are documented in, and it is the order the reason
// is reported in: an entry excluded by two filters is attributed to the first, which
// keeps the answer stable rather than dependent on evaluation order.
func (t Template) verdict(e fold.Entry) Reason {
	switch {
	case !t.admitsType(e.Type):
		return ReasonType
	case !t.statusSet[e.Status]:
		return ReasonStatus
	case classificationRank(e.Classification) > classificationRank(t.MaxClassification):
		return ReasonClassification
	case t.From != "" && e.FactTime < t.From:
		return ReasonBeforeWindow
	case t.To != "" && e.FactTime >= t.To:
		return ReasonAfterWindow
	}
	return ReasonServed
}

// Apply prunes v through the template and the consent gate, returning the served
// value: the same kind of value, built from a subset of the same entries.
//
// The gate runs only on entries the template already admitted. That order is
// deliberate — consent is asked about what a consumer would otherwise see, not about
// every fact Anam holds, so a future policy's audit trail is about a real disclosure
// rather than about the whole ledger.
func Apply(v fold.Value, t Template, g Gate, s Series) (fold.Value, error) {
	verdicts, err := Explain(v, t, g, s)
	if err != nil {
		return fold.Value{}, err
	}

	kept := make([]fold.Entry, 0, len(verdicts))
	for _, x := range verdicts {
		if x.Reason.Served() {
			kept = append(kept, x.Entry)
		}
	}
	return fold.NewValue(kept)
}

// Verdict is one entry and what the template and the gate decided about it.
type Verdict struct {
	Entry  fold.Entry
	Reason Reason

	// Detail carries the consent gate's own words when it denied; empty otherwise.
	Detail string
}

// Explain runs the template and the gate over every entry and reports the decision for
// each, served or not, in the canonical's order.
//
// This is the function Apply is built from, not a companion to it. A projection that
// could not say why it excluded something would be governance by assertion; one whose
// explanation came from separately-written code would be governance by coincidence.
func Explain(v fold.Value, t Template, g Gate, s Series) ([]Verdict, error) {
	if g == nil {
		return nil, fmt.Errorf("projection: %s: a consent gate is required (§10)", t.Name)
	}
	if t.Shape == ShapeSnapshot && s == nil {
		return nil, fmt.Errorf("projection: %s: a snapshot shape needs the Definitions' series rules", t.Name)
	}

	out := make([]Verdict, 0, v.Len())
	for _, e := range v.Entries() {
		x := Verdict{Entry: e, Reason: t.verdict(e)}
		if x.Reason.Served() {
			// The gate is asked only about what a consumer would otherwise see, not
			// about every fact Anam holds, so a future policy's audit trail is about a
			// real disclosure rather than about the whole ledger.
			d := g.Evaluate(consent.Request{
				Type:           e.Type,
				Classification: e.Classification,
				Status:         e.Status,
				FactTime:       e.FactTime,
			})
			if !d.Allow {
				x.Reason, x.Detail = ReasonConsent, d.Reason
			}
		}
		out = append(out, x)
	}
	if t.Shape == ShapeSnapshot {
		snapshot(out, s)
	}
	return out, nil
}

// snapshot demotes the served verdicts the last-known-value relation excludes. It
// runs after the four per-entry knobs and the gate, over what they admitted, so the
// window's To is the snapshot's "at" and the type allow-list bounds which series are
// in play. The relation itself is fold.Fates: one decision procedure, shared with
// the value-shaped read, so this explanation cannot disagree with that result.
func snapshot(verdicts []Verdict, s Series) {
	var served []fold.Entry
	var at []int
	for i, x := range verdicts {
		if x.Reason.Served() {
			served = append(served, x.Entry)
			at = append(at, i)
		}
	}
	for j, f := range fold.Fates(served, s.Closes) {
		switch {
		case f.Displaced:
			verdicts[at[j]].Reason, verdicts[at[j]].Detail = ReasonDisplaced, "by "+f.By
		case f.Closed:
			verdicts[at[j]].Reason, verdicts[at[j]].Detail = ReasonClosed, "by "+f.By
		}
	}
}

// classificationRank orders the levels by restriction, so a ceiling comparison is an
// integer one. An unknown level ranks above every known one: a facet Anam cannot place
// is not served under a ceiling it cannot prove it is under.
func classificationRank(c facet.Classification) int {
	switch c {
	case facet.ClassificationNormal:
		return 1
	case facet.ClassificationConfidential:
		return 2
	case facet.ClassificationSensitive:
		return 3
	default:
		return 4
	}
}
