package serve

import (
	"context"
	"errors"
	"fmt"

	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/projection"
	"github.com/mattjoyce/anam/internal/query"
	"github.com/mattjoyce/anam/internal/receipt"
	"github.com/mattjoyce/anam/internal/store/ledger"
	"github.com/mattjoyce/anam/internal/store/ops"
)

// A query receipt (card 173) is a receipt whose served value is a query's answer: the
// template is the projection the query ran through, FacetIDs are the facets it
// served, and Program is the rule set, the text and the relation. Verify replays the
// derivation; the rule set is the binary's, so one whose rules moved cannot replay.

// ErrRuleSetMoved is returned when a receipt names a bundled rule set other than this
// binary's. The rules are what derived the answer, so another set cannot re-derive
// it, and reporting "not reproduced" would blame the ledger for a change in the rules.
var ErrRuleSetMoved = errors.New("serve: the bundled rule set has moved since this receipt")

// definitions is d's Definitions, or the bundled ones when it names none.
func (d Deps) definitions() (query.Definitions, error) {
	if d.Definitions != nil {
		return d.Definitions, nil
	}
	return def.Bundled()
}

// PublishQuery files a query's answer as what template t served at w, with a receipt
// that names the facets the projection served, the program, and the answer's hash.
// The answer is filed first and the receipt second, as Publish does; with dryRun set
// nothing is written.
func PublishQuery(ctx context.Context, d Deps, t projection.Template, w ledger.Watermark, prog receipt.Program,
	served fold.Value, ans query.Answer, dryRun bool,
) (Served, error) {
	ledgerID, err := d.Ledger.ID(ctx)
	if err != nil {
		return Served{}, err
	}
	r := receipt.Receipt{
		LedgerID:             ledgerID,
		ServedHash:           ans.Hash(),
		FacetIDs:             idsOf(served),
		Watermark:            int64(w),
		Template:             t.Name,
		TemplateVersion:      t.Version,
		TemplateHash:         t.Hash,
		DefinitionSetVersion: d.DefinitionSetVersion,
		ServedAt:             clock.Format(d.Clock.Now()),
		Span:                 t.Span(),
		Program:              &prog,
		Principal:            d.Reader,
	}
	if err := r.Validate(); err != nil {
		return Served{}, err
	}
	receiptBytes, err := r.Bytes()
	if err != nil {
		return Served{}, err
	}
	out := Served{Receipt: r, ReceiptHash: ops.Digest(receiptBytes), Answer: &ans}
	if dryRun {
		return out, nil
	}
	if _, err := d.Published.Put(ctx, receipt.ServedKind, ans.Bytes()); err != nil {
		return Served{}, err
	}
	if _, err := d.Published.Put(ctx, receipt.Kind, receiptBytes); err != nil {
		return Served{}, err
	}
	out.Written = true
	return out, nil
}

// verifyQuery is Verify's second half for a query receipt: the full fold at the
// receipt's watermark (the base relations need each facet's Definition version,
// which only the fold's live facets carry), the projection, the same program over
// the served value, and the answer's hash against the receipt's.
func verifyQuery(ctx context.Context, d Deps, r receipt.Receipt, t projection.Template, notes []string) (receipt.Result, error) {
	rules, err := query.Bundled()
	if err != nil {
		return receipt.Result{}, err
	}
	if p := r.Program; p.RuleSetHash != rules.Hash {
		return receipt.Result{}, fmt.Errorf("%w: %s v%d is %s here, the receipt was answered under %s v%d %s",
			ErrRuleSetMoved, rules.Name, rules.Version, rules.Hash, p.RuleSet, p.RuleSetVersion, p.RuleSetHash)
	}
	defs, err := d.definitions()
	if err != nil {
		return receipt.Result{}, err
	}
	res, err := fold.ReadAt(ctx, d.Ledger, ledger.Watermark(r.Watermark))
	if err != nil {
		return receipt.Result{}, err
	}
	served, err := Derive(res.Value, t, d.Gate, d.Series, "")
	if err != nil {
		return receipt.Result{}, err
	}
	ans, _, err := query.Run(ctx, served, query.Versions(res.Live), defs, r.Program.Text, r.Program.Relation, 0)
	if err != nil {
		return receipt.Result{}, err
	}
	stored, err := d.Published.Raw(ctx, receipt.ServedKind, r.ServedHash)
	if err != nil && !errors.Is(err, ops.ErrNotFound) {
		return receipt.Result{}, err
	}
	out := receipt.Compare(r, ans.Hash(), idsOf(served), stored, ops.Digest)
	out.Notes = append(out.Notes, notes...)
	if r.DefinitionSetVersion != d.DefinitionSetVersion {
		out.Notes = append(out.Notes, fmt.Sprintf(
			"definition set has moved (receipt %s, this build %s) — a query reads which types close a series and which fields a type declares from the definitions, so if reproduction failed, look here first",
			r.DefinitionSetVersion, d.DefinitionSetVersion))
	}
	return out, nil
}
