// Package receipt records what Anam served, so that a serve can be proved rather than
// remembered (ARCHITECTURE.md §9).
//
// A receipt is not an anam-event. Serving a facet changes nothing Anam *knows* about
// it, so the ledger says nothing about publication and the watermark does not move;
// what Anam has *shown* is a separate fact, and this is where it lives
// (journal: published-is-a-receipt-not-a-anam-event). A facet's published time is
// the earliest ServedAt over the receipts naming it.
//
// It holds no payload — only hashes, identity and the coordinates needed to re-derive.
// That is what makes it small enough to keep forever and useless to anyone who steals
// one without the ledger.
package receipt

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// ErrMalformed is returned by Parse for bytes that are not a whole receipt.
var ErrMalformed = errors.New("receipt: malformed")

// Kind is the artifact-store subdirectory receipts are filed under.
const Kind = "receipt"

// ServedKind is the artifact-store subdirectory served projection values are filed
// under. It lives here because a receipt is the only thing that names one.
const ServedKind = "served"

// Receipt is the immutable record of one serve. Its field set is §9's, and each field
// earns its place by being something the re-derivation needs or something a reader
// years later could not otherwise recover:
//
//   - Watermark and TemplateVersion are the two coordinates the re-derivation runs at.
//   - ServedHash and FacetIDs are what the re-derivation is checked against — the
//     bytes and, redundantly but usefully, the identity set, so a mismatch can say
//     *which* facets differ rather than only that the hashes do.
//   - DefinitionSetVersion is provenance: the interpretation in force when these
//     facets were captured. It gates nothing (definitions govern capture, not the
//     fold), and a mismatch is worth reporting, never a failed reproduction.
//   - Span and ServedAt make the receipt self-describing without the binary that
//     wrote it.
//   - LedgerID names the ledger the watermark counts in. A watermark means nothing
//     against another ledger: after a rebuild the same number lands somewhere else,
//     and a verify there must say "other ledger", never "failed reproduction".
//     Receipts served before 2026-10-07 have none, and they parse; the verify notes
//     it. It is last, so their bytes are unchanged.
//   - Query is the narrowing a read applied to what the template served: a window,
//     a snapshot, a type (card 87). Nil when the whole projection was served, which
//     every receipt before 2026-10-10 did; it is after LedgerID and omitted when nil,
//     so their bytes are unchanged too.
type Receipt struct {
	ServedHash           string   `json:"served_hash"`
	FacetIDs             []string `json:"facet_ids"`
	Watermark            int64    `json:"watermark"`
	Template             string   `json:"template"`
	TemplateVersion      int      `json:"template_version"`
	DefinitionSetVersion string   `json:"definition_set_version"`
	ServedAt             string   `json:"served_at"`
	Span                 string   `json:"span"`
	LedgerID             string   `json:"ledger_id,omitempty"`
	Query                *Query   `json:"query,omitempty"`
	// TemplateHash is the content hash of the template version that pruned the
	// value (card 125), so verify can tell a body swapped under the same version
	// from the one that served. Receipts before 2026-10-13 have none; it is last
	// and omitted when empty, so their bytes are unchanged.
	TemplateHash string `json:"template_hash,omitempty"`
	// Program is the query a receipt answers (card 173): the rule set it stood on,
	// the text it ran and the relation answered. The template is the projection the
	// query ran through, and ServedHash is the answer's. Nil for a timeline receipt;
	// last and omitted when nil, so every earlier receipt keeps its bytes.
	Program *Program `json:"program,omitempty"`
}

// Program is the query half of a query receipt. RuleSetHash pins the bundled rules
// the query built on, so a binary whose rules moved cannot pass as the one that
// answered; Text is the query as written, and Relation the one it was answered with.
type Program struct {
	RuleSet        string `json:"rule_set"`
	RuleSetVersion int    `json:"rule_set_version"`
	RuleSetHash    string `json:"rule_set_hash"`
	Text           string `json:"text"`
	Relation       string `json:"relation"`
}

// check refuses a program a receipt could not replay.
func (p Program) check() error {
	switch {
	case p.RuleSet == "" || p.RuleSetVersion < 1 || p.RuleSetHash == "":
		return fmt.Errorf("%w: program names no rule set", ErrMalformed)
	case strings.TrimSpace(p.Text) == "":
		return fmt.Errorf("%w: program has no text", ErrMalformed)
	case p.Relation == "":
		return fmt.Errorf("%w: program names no relation", ErrMalformed)
	}
	return nil
}

// Query is how a read narrowed the template's value before it was served, resolved to
// instants so that replaying it needs no zone: the snapshot at At first, then the
// fact-time window [From, Before) and the type. Each part is optional; at least one is
// set, or the receipt carries no Query at all.
type Query struct {
	From   string `json:"from,omitempty"`
	Before string `json:"before,omitempty"`
	At     string `json:"at,omitempty"`
	Type   string `json:"type,omitempty"`
	// Fields are the payload pointers the read was cut to (card 138). With any, the
	// served value is a frame, and ServedHash is the frame's. Last, and omitted
	// when empty, so receipts without fields keep their bytes.
	Fields []string `json:"fields,omitempty"`
}

// IsZero reports whether q narrows nothing.
func (q Query) IsZero() bool {
	return q.From == "" && q.Before == "" && q.At == "" && q.Type == "" && len(q.Fields) == 0
}

// checkFields refuses a field that is not a JSON pointer: a receipt naming one
// could not be replayed.
func (q Query) checkFields() error {
	for _, f := range q.Fields {
		if f == "" || !strings.HasPrefix(f, "/") {
			return fmt.Errorf("%w: query field %q is not a JSON pointer", ErrMalformed, f)
		}
	}
	return nil
}

// Bytes is the receipt's one serialisation, and the bytes its own content address is
// taken over. Field order is the struct's, which is fixed, so the same receipt always
// produces the same bytes.
func (r Receipt) Bytes() ([]byte, error) {
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("receipt: serialise: %w", err)
	}
	return append(b, '\n'), nil
}

// Validate reports whether every field a receipt needs is present. A receipt missing
// one is not a partial record to be tolerated — it is a claim that cannot be checked.
func (r Receipt) Validate() error {
	switch {
	case r.ServedHash == "":
		return fmt.Errorf("%w: served_hash is required", ErrMalformed)
	case r.FacetIDs == nil:
		return fmt.Errorf("%w: facet_ids is required (an empty serve records an empty list)", ErrMalformed)
	case r.Watermark < 0:
		return fmt.Errorf("%w: watermark %d", ErrMalformed, r.Watermark)
	case r.Template == "":
		return fmt.Errorf("%w: template is required", ErrMalformed)
	case r.TemplateVersion < 1:
		return fmt.Errorf("%w: template_version %d", ErrMalformed, r.TemplateVersion)
	case r.DefinitionSetVersion == "":
		return fmt.Errorf("%w: definition_set_version is required", ErrMalformed)
	case r.ServedAt == "":
		return fmt.Errorf("%w: served_at is required", ErrMalformed)
	case r.Span == "":
		return fmt.Errorf("%w: span is required", ErrMalformed)
	case r.Query != nil && r.Query.IsZero():
		return fmt.Errorf("%w: query narrows nothing; omit it", ErrMalformed)
	case r.Query != nil && r.Query.At != "" && r.Query.Before != "":
		return fmt.Errorf("%w: query has both at and before; a snapshot has one upper bound", ErrMalformed)
	case r.Query != nil && r.Program != nil:
		return fmt.Errorf("%w: a query receipt is not narrowed; the program names its instants", ErrMalformed)
	case r.Query != nil:
		return r.Query.checkFields()
	case r.Program != nil:
		return r.Program.check()
	}
	return nil
}

// Parse reads a receipt written by Bytes. Unknown fields are refused: a receipt from a
// future Anam carrying a field this build cannot check is not something to verify
// partially and report as sound.
func Parse(raw []byte) (Receipt, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var r Receipt
	if err := dec.Decode(&r); err != nil {
		return Receipt{}, fmt.Errorf("%w: %v", ErrMalformed, err)
	}
	if err := r.Validate(); err != nil {
		return Receipt{}, err
	}
	return r, nil
}

// Result is the verdict on one receipt: two independent proofs, kept apart.
//
// Reproduced asks whether the ledger still honours what the receipt promised — re-fold
// at the watermark, re-prune through the template, compare. Intact asks whether the
// artifact on disk is still the one that was served — re-hash the file against its own
// content address. Either can fail without the other: a tampered file under a sound
// ledger, or a corrupted ledger under an untouched file. Collapsing them into one
// boolean would let each hide behind the other.
type Result struct {
	Reproduced bool `json:"reproduced"`
	Intact     bool `json:"intact"`

	// StoredHash is what the served file on disk actually hashes to; empty when the
	// file is absent.
	StoredHash string `json:"stored_hash,omitempty"`

	// DerivedHash is what re-deriving at the receipt's watermark produced.
	DerivedHash string `json:"derived_hash"`

	// Missing and Unexpected name the facet-id differences, so a mismatch can be read
	// rather than merely noticed.
	Missing    []string `json:"missing,omitempty"`
	Unexpected []string `json:"unexpected,omitempty"`

	// Notes carries non-fatal observations, e.g. a moved definition set.
	Notes []string `json:"notes,omitempty"`
}

// OK reports whether both proofs held.
func (r Result) OK() bool { return r.Reproduced && r.Intact }

// Compare checks a receipt against a freshly derived served value and, when the served
// file was found, the bytes actually on disk. storedBytes may be nil, which records the
// artifact as absent rather than as altered — those are different failures.
func Compare(r Receipt, derivedHash string, derivedIDs []string, storedBytes []byte, digest func([]byte) string) Result {
	res := Result{DerivedHash: derivedHash}

	res.Missing, res.Unexpected = diffIDs(r.FacetIDs, derivedIDs)
	res.Reproduced = derivedHash == r.ServedHash && len(res.Missing) == 0 && len(res.Unexpected) == 0

	if storedBytes != nil {
		res.StoredHash = digest(storedBytes)
		res.Intact = res.StoredHash == r.ServedHash
	}
	return res
}

// diffIDs reports which ids the receipt claimed but the re-derivation did not produce,
// and which it produced but the receipt did not claim.
func diffIDs(claimed, derived []string) (missing, unexpected []string) {
	inClaimed := make(map[string]bool, len(claimed))
	for _, id := range claimed {
		inClaimed[id] = true
	}
	inDerived := make(map[string]bool, len(derived))
	for _, id := range derived {
		inDerived[id] = true
	}
	for _, id := range claimed {
		if !inDerived[id] {
			missing = append(missing, id)
		}
	}
	for _, id := range derived {
		if !inClaimed[id] {
			unexpected = append(unexpected, id)
		}
	}
	sort.Strings(missing)
	sort.Strings(unexpected)
	return missing, unexpected
}

// Summary renders the verdict in one line per proof, for the CLI.
func (r Result) Summary() string {
	var b strings.Builder
	write := func(ok bool, label, detail string) {
		mark := "FAIL"
		if ok {
			mark = "OK"
		}
		fmt.Fprintf(&b, "%-4s %s%s\n", mark, label, detail)
	}

	write(r.Reproduced, "reproduced", fmt.Sprintf("  re-derived %s", r.DerivedHash))
	// Absent is neither pass nor fail: the ledger can still honour a receipt whose
	// artifact was deleted, and calling that "FAIL intact" would read as tampering.
	if r.StoredHash == "" {
		b.WriteString("--   intact      served artifact not present in this store\n")
	} else {
		write(r.Intact, "intact", fmt.Sprintf("      on disk    %s", r.StoredHash))
	}
	if len(r.Missing) > 0 {
		fmt.Fprintf(&b, "     missing     %d facet(s): %s\n", len(r.Missing), strings.Join(clip(r.Missing), ", "))
	}
	if len(r.Unexpected) > 0 {
		fmt.Fprintf(&b, "     unexpected  %d facet(s): %s\n", len(r.Unexpected), strings.Join(clip(r.Unexpected), ", "))
	}
	for _, n := range r.Notes {
		fmt.Fprintf(&b, "note %s\n", n)
	}
	return b.String()
}

// clip bounds a list printed to a terminal; the receipt holds the whole set.
func clip(ids []string) []string {
	const max = 5
	if len(ids) <= max {
		return ids
	}
	return append(append([]string{}, ids[:max]...), fmt.Sprintf("… and %d more", len(ids)-max))
}
