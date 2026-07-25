// Package receipt records what Anam served, so that a serve can be proved rather than
// remembered (ARCHITECTURE.md §9).
//
// A receipt is not a record-event. Serving a facet changes nothing Anam *knows* about
// it, so the ledger says nothing about publication and the watermark does not move;
// what Anam has *shown* is a separate fact, and this is where it lives
// (journal: published-is-a-receipt-not-a-record-event). A facet's published time is
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
type Receipt struct {
	ServedHash           string   `json:"served_hash"`
	FacetIDs             []string `json:"facet_ids"`
	Watermark            int64    `json:"watermark"`
	Template             string   `json:"template"`
	TemplateVersion      int      `json:"template_version"`
	DefinitionSetVersion string   `json:"definition_set_version"`
	ServedAt             string   `json:"served_at"`
	Span                 string   `json:"span"`
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
	Reproduced bool
	Intact     bool

	// StoredHash is what the served file on disk actually hashes to; empty when the
	// file is absent.
	StoredHash string

	// DerivedHash is what re-deriving at the receipt's watermark produced.
	DerivedHash string

	// Missing and Unexpected name the facet-id differences, so a mismatch can be read
	// rather than merely noticed.
	Missing    []string
	Unexpected []string

	// Notes carries non-fatal observations, e.g. a moved definition set.
	Notes []string
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
