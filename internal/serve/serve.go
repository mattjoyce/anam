// Package serve publishes a projection and honours a receipt — the two halves of
// R7's claim, kept in one place because they must stay each other's inverse.
//
// Serving reads: fold the person at the head watermark, prune through the selected
// template and the consent gate, write the served value and its receipt to the
// published-artifact tier. It writes nothing to the ledger and does not move W —
// being shown is not something Anam learned (§9).
//
// Verifying re-runs exactly that derivation at the receipt's own watermark and checks
// two things independently: that the ledger still produces those bytes, and that the
// artifact on disk is still those bytes. If serving and verifying ever stop being the
// same derivation, a receipt becomes a story about a serve rather than a proof of one,
// which is why they are neighbours in one file.
package serve

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/projection"
	"github.com/mattjoyce/anam/internal/receipt"
	"github.com/mattjoyce/anam/internal/store/artifact"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// ErrTemplateMoved is returned when a receipt names a template version this binary no
// longer carries. The template is exactly what produced the pruning, so a different
// version cannot re-derive the same value and reporting "not reproduced" would blame
// the ledger for a change in the code.
var ErrTemplateMoved = errors.New("serve: template version has moved since this receipt")

// ErrOtherLedger is returned when a receipt names a ledger other than the one open.
// Its watermark counts in that ledger, so re-deriving here would prove nothing
// either way, and reporting "not reproduced" would blame this ledger for being a
// different one.
var ErrOtherLedger = errors.New("serve: receipt was served from another ledger")

// Ledger is the read surface a serve needs: the fold's, plus the ledger's identity
// for the receipt. *ledger.Ledger satisfies it.
type Ledger interface {
	fold.Ledger
	ID(ctx context.Context) (string, error)
}

// Store is the published-artifact tier this package writes to and reads back.
type Store interface {
	Put(kind string, b []byte) (string, error)
	Get(kind, digest string) ([]byte, error)
	Raw(kind, digest string) ([]byte, error)
	List(kind string) ([]string, error)
}

// Clock supplies the served-at time; tests freeze it.
type Clock interface{ Now() time.Time }

// Deps is everything a serve or a verify needs. It is one struct rather than seven
// parameters because both calls need the same seven, and a caller assembling them
// twice is a caller that can assemble them differently.
type Deps struct {
	Ledger    Ledger
	Templates *projection.Registry
	Artifacts Store
	Gate      projection.Gate
	Clock     Clock

	// Series is the Definitions' word on which types close a series, which a
	// snapshot-shaped template needs and a log-shaped one never consults.
	Series projection.Series

	// DefinitionSetVersion is the receipt's provenance field, passed in rather than
	// read here so that this package needs no opinion about definitions.
	DefinitionSetVersion string
}

// Served reports one completed serve.
type Served struct {
	Receipt     receipt.Receipt
	ReceiptHash string

	// CanonicalHash and CanonicalLen describe what was pruned *from*, so the operator
	// can see the ratio rather than only the result.
	CanonicalHash string
	CanonicalLen  int

	// Written is false for a dry run: everything was derived, nothing was published.
	Written bool
}

// Run serves one projection at the ledger's head watermark. With dryRun set it derives
// everything and writes nothing — the same code path, so a dry run cannot flatter a
// real one.
func Run(ctx context.Context, d Deps, name string, dryRun bool) (Served, error) {
	t, err := d.Templates.Lookup(name)
	if err != nil {
		return Served{}, err
	}

	ledgerID, err := d.Ledger.ID(ctx)
	if err != nil {
		return Served{}, err
	}
	res, err := fold.Read(ctx, d.Ledger)
	if err != nil {
		return Served{}, err
	}
	value, err := projection.Apply(res.Value, t, d.Gate, d.Series)
	if err != nil {
		return Served{}, err
	}

	r := receipt.Receipt{
		LedgerID:             ledgerID,
		ServedHash:           value.Hash(),
		FacetIDs:             idsOf(value),
		Watermark:            int64(res.W),
		Template:             t.Name,
		TemplateVersion:      t.Version,
		DefinitionSetVersion: d.DefinitionSetVersion,
		ServedAt:             clock.Format(d.Clock.Now()),
		Span:                 t.Span(),
	}
	if err := r.Validate(); err != nil {
		return Served{}, err
	}
	receiptBytes, err := r.Bytes()
	if err != nil {
		return Served{}, err
	}

	out := Served{
		Receipt:       r,
		ReceiptHash:   artifact.Digest(receiptBytes),
		CanonicalHash: res.Value.Hash(),
		CanonicalLen:  res.Value.Len(),
	}
	if dryRun {
		return out, nil
	}

	// The value first, then the receipt: a receipt is a pointer to a served artifact,
	// and a crash between the two should leave an unreferenced value rather than a
	// receipt naming a file that was never written. Both are content-addressed, so the
	// re-run overwrites neither and completes the pair.
	if _, err := d.Artifacts.Put(receipt.ServedKind, value.Bytes()); err != nil {
		return Served{}, err
	}
	if _, err := d.Artifacts.Put(receipt.Kind, receiptBytes); err != nil {
		return Served{}, err
	}
	out.Written = true
	return out, nil
}

// Verify honours a receipt: re-derive at its watermark, and re-hash the artifact on
// disk. The two proofs are independent (see receipt.Result).
func Verify(ctx context.Context, d Deps, receiptHash string) (receipt.Receipt, receipt.Result, error) {
	raw, err := d.Artifacts.Get(receipt.Kind, receiptHash)
	if err != nil {
		return receipt.Receipt{}, receipt.Result{}, err
	}
	r, err := receipt.Parse(raw)
	if err != nil {
		return receipt.Receipt{}, receipt.Result{}, err
	}

	t, err := d.Templates.Lookup(r.Template)
	if err != nil {
		return r, receipt.Result{}, err
	}
	if t.Version != r.TemplateVersion {
		return r, receipt.Result{}, fmt.Errorf("%w: %s is v%d, the receipt was served under v%d",
			ErrTemplateMoved, t.Name, t.Version, r.TemplateVersion)
	}

	ledgerID, err := d.Ledger.ID(ctx)
	if err != nil {
		return r, receipt.Result{}, err
	}
	var notes []string
	switch r.LedgerID {
	case ledgerID:
	case "":
		notes = append(notes, "receipt names no ledger (served before ledger ids were recorded); its watermark is taken to be this ledger's")
	default:
		return r, receipt.Result{}, fmt.Errorf("%w: receipt names %s, this is %s; its watermark %d counts there, not here",
			ErrOtherLedger, r.LedgerID, ledgerID, r.Watermark)
	}

	res, err := fold.ReadAt(ctx, d.Ledger, ledger.Watermark(r.Watermark))
	if err != nil {
		return r, receipt.Result{}, err
	}
	value, err := projection.Apply(res.Value, t, d.Gate, d.Series)
	if err != nil {
		return r, receipt.Result{}, err
	}

	// Raw, not Get: the verifier wants the bytes as they are so it can say what they
	// hash to. Get would refuse a tampered artifact and report only that it refused.
	stored, err := d.Artifacts.Raw(receipt.ServedKind, r.ServedHash)
	if err != nil && !errors.Is(err, artifact.ErrNotFound) {
		return r, receipt.Result{}, err
	}

	out := receipt.Compare(r, value.Hash(), idsOf(value), stored, artifact.Digest)
	out.Notes = append(out.Notes, notes...)
	if r.DefinitionSetVersion != d.DefinitionSetVersion {
		note := "definitions govern capture, not the fold, so this does not affect reproduction"
		if t.Shape == projection.ShapeSnapshot {
			note = "a snapshot shape reads which types close a series from the definitions, so if reproduction failed, look here first"
		}
		out.Notes = append(out.Notes, fmt.Sprintf(
			"definition set has moved (receipt %s, this build %s) — %s",
			r.DefinitionSetVersion, d.DefinitionSetVersion, note))
	}
	return r, out, nil
}

// Show returns a published receipt and the served value it names, read from the
// artifact tier alone — no ledger, no fold, no re-derivation.
//
// That restraint is the point. Verify asks whether the ledger still honours a receipt;
// Show asks what the artifact actually says, which is a question you must be able to
// answer when the ledger is gone, unreachable, or exactly what you doubt. The bytes are
// checked against their own content address on the way out, so a tampered artifact is
// reported rather than displayed as though it were what was served.
func Show(d Deps, receiptHash string) (receipt.Receipt, []fold.Entry, error) {
	raw, err := d.Artifacts.Get(receipt.Kind, receiptHash)
	if err != nil {
		return receipt.Receipt{}, nil, err
	}
	r, err := receipt.Parse(raw)
	if err != nil {
		return receipt.Receipt{}, nil, err
	}

	served, err := d.Artifacts.Get(receipt.ServedKind, r.ServedHash)
	if err != nil {
		return r, nil, err
	}
	var entries []fold.Entry
	if err := json.Unmarshal(served, &entries); err != nil {
		return r, nil, fmt.Errorf("serve: parse served value %s: %w", r.ServedHash, err)
	}
	return r, entries, nil
}

// List returns the receipts in the store, newest serve first.
func List(d Deps) ([]receipt.Receipt, []string, error) {
	digests, err := d.Artifacts.List(receipt.Kind)
	if err != nil {
		return nil, nil, err
	}

	receipts := make([]receipt.Receipt, 0, len(digests))
	hashes := make([]string, 0, len(digests))
	for _, digest := range digests {
		raw, err := d.Artifacts.Get(receipt.Kind, digest)
		if err != nil {
			return nil, nil, err
		}
		r, err := receipt.Parse(raw)
		if err != nil {
			return nil, nil, fmt.Errorf("serve: receipt %s: %w", digest, err)
		}
		receipts = append(receipts, r)
		hashes = append(hashes, digest)
	}

	// served_at is a fixed-width clock.Layout string, so the lexical sort is the
	// chronological one; the hash breaks ties on a same-instant pair.
	for i := range receipts {
		for j := i + 1; j < len(receipts); j++ {
			ri, rj := receipts[i], receipts[j]
			if rj.ServedAt > ri.ServedAt || (rj.ServedAt == ri.ServedAt && hashes[j] < hashes[i]) {
				receipts[i], receipts[j] = receipts[j], receipts[i]
				hashes[i], hashes[j] = hashes[j], hashes[i]
			}
		}
	}
	return receipts, hashes, nil
}

// idsOf lists a served value's facet ids in served order — the canonical's own
// (fact-time, id) order, which is what makes the list reproducible.
func idsOf(v fold.Value) []string {
	ids := make([]string, 0, v.Len())
	for _, e := range v.Entries() {
		ids = append(ids, e.FacetID)
	}
	return ids
}
