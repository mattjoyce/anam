// Package serve publishes a projection and honours a receipt — the two halves of
// R7's claim, kept in one place because they must stay each other's inverse.
//
// Serving reads: the person at the head watermark, composed from the last mint run and
// the tail after it when the checkpoint log is to hand and folded from the beginning
// when it is not, pruned through the selected template and the consent gate; the
// served value and its receipt are written to the operational store. It writes nothing to
// the ledger and does not move W: being shown is not something Anam learned (§9).
//
// Verifying re-runs exactly that derivation at the receipt's own watermark and checks
// two things independently: that the ledger still produces those bytes, and that the
// stored value is still those bytes. If serving and verifying ever stop being the
// same derivation, a receipt becomes a story about a serve rather than a proof of one,
// which is why they are neighbours in one file.
package serve

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/frame"
	"github.com/mattjoyce/anam/internal/mint"
	"github.com/mattjoyce/anam/internal/projection"
	"github.com/mattjoyce/anam/internal/query"
	"github.com/mattjoyce/anam/internal/receipt"
	"github.com/mattjoyce/anam/internal/store/ledger"
	"github.com/mattjoyce/anam/internal/store/ops"
)

// ErrTemplateMoved is returned when a receipt names a template version this record
// does not hold, or holds with another body. The template is exactly what produced
// the pruning, so a different one cannot re-derive the same value and reporting "not
// reproduced" would blame the ledger for a change in the template.
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

// Store is where this package writes the receipts and served values it publishes and
// reads them back: the operational store's content-addressed rows. It is also where
// the mint files its checkpoints, so it is the mint's Store too.
type Store interface {
	mint.Store
	Raw(ctx context.Context, kind, digest string) ([]byte, error)
	List(ctx context.Context, kind string) ([]string, error)
}

// Clock supplies the served-at time; tests freeze it.
type Clock interface{ Now() time.Time }

// Deps is everything a serve or a verify needs. It is one struct rather than seven
// parameters because both calls need the same seven, and a caller assembling them
// twice is a caller that can assemble them differently.
type Deps struct {
	Ledger    Ledger
	Templates *projection.Registry
	Published Store
	Gate      projection.Gate
	Clock     Clock

	// Checkpoints is the mint's log, when the caller has it open. With it a serve or
	// a verify composes the canonical from the last mint run at or below its
	// watermark and the events after it (mint.ReadAt), so the cost is the tail, not
	// the ledger. Nil, or before any run, it folds from the beginning. Both are the
	// fold at that watermark, so a receipt served one way verifies the other.
	Checkpoints mint.CheckpointLog

	// Series is the Definitions' word on which types close a series, which a
	// snapshot-shaped template needs and a log-shaped one never consults.
	Series projection.Series

	// DefinitionSetVersion is the receipt's provenance field, passed in rather than
	// read here so that this package needs no opinion about definitions.
	DefinitionSetVersion string

	// Definitions are the Definitions in force, which a query's base relations and
	// pointer check read (card 173). Nil means the bundled ones alone.
	Definitions query.Definitions

	// Reader is the principal a receipt filed through these Deps names (card 216):
	// the one whose read it files. Empty names nobody, which is how every receipt
	// before it was cut and how an ordinary publish still is.
	Reader string
}

// Served reports one completed serve.
type Served struct {
	Receipt     receipt.Receipt
	ReceiptHash string

	// Frame is the served value when the query named fields (card 138): what the
	// receipt's ServedHash is over, and what the reader is answered with. Nil for a
	// whole serve.
	Frame *frame.Frame

	// Answer is the served value of a query receipt (card 173). Nil otherwise.
	Answer *query.Answer

	// CanonicalHash and CanonicalLen describe what was pruned *from*, so the operator
	// can see the ratio rather than only the result.
	CanonicalHash string
	CanonicalLen  int

	// Run is the mint run the canonical was composed from, and Tail the events
	// applied after it; Run is zero when the ledger was folded from the beginning.
	Run  ledger.Watermark
	Tail int

	// Written is false for a dry run: everything was derived, nothing was published.
	Written bool
}

// canonical is the whole-person value at a watermark and how it was reached.
type canonical struct {
	W     ledger.Watermark
	Value fold.Value
	Run   ledger.Watermark
	Tail  int
}

// valueAt is the canonical at w: composed from the checkpoint log and the tail when d
// has the log and a mint run is at or below w, else the full fold. The two are the
// same value (mint's oracle test holds ReadAt to the fold at every watermark), which
// is what lets serve and verify use either and still be each other's inverse.
func valueAt(ctx context.Context, d Deps, w ledger.Watermark) (canonical, error) {
	if d.Checkpoints != nil {
		c, ok, err := mint.ReadAt(ctx, d.Ledger, d.Checkpoints, d.Published, w)
		if err != nil {
			return canonical{}, err
		}
		if ok {
			return canonical{W: c.W, Value: c.Value, Run: c.Run, Tail: c.Tail}, nil
		}
	}
	res, err := fold.ReadAt(ctx, d.Ledger, w)
	if err != nil {
		return canonical{}, err
	}
	return canonical{W: res.W, Value: res.Value}, nil
}

// Run serves one projection at the ledger's head watermark. With dryRun set it derives
// everything and writes nothing — the same code path, so a dry run cannot flatter a
// real one.
func Run(ctx context.Context, d Deps, name string, dryRun bool) (Served, error) {
	t, err := d.Templates.Lookup(name)
	if err != nil {
		return Served{}, err
	}
	head, err := d.Ledger.Head(ctx)
	if err != nil {
		return Served{}, err
	}
	res, err := valueAt(ctx, d, head)
	if err != nil {
		return Served{}, err
	}
	value, err := Derive(res.Value, t, d.Gate, d.Series, "")
	if err != nil {
		return Served{}, err
	}
	out, err := Publish(ctx, d, t, res.W, nil, value, dryRun)
	if err != nil {
		return Served{}, err
	}
	out.CanonicalHash, out.CanonicalLen = res.Value.Hash(), res.Value.Len()
	out.Run, out.Tail = res.Run, res.Tail
	return out, nil
}

// Publish files value as what template t served at w, narrowed by q (nil when the
// whole projection was served), with the receipt that says so. The caller derived
// value; Verify derives it again from the receipt alone, through Narrow, so a caller
// that narrowed any other way publishes a receipt that will not reproduce. The served
// value is filed first and the receipt second: a crash between them leaves a value
// no receipt names, never a receipt without its value. With dryRun set nothing is
// written.
func Publish(ctx context.Context, d Deps, t projection.Template, w ledger.Watermark, q *receipt.Query,
	value fold.Value, dryRun bool) (Served, error) {
	ledgerID, err := d.Ledger.ID(ctx)
	if err != nil {
		return Served{}, err
	}
	fr, err := Cut(value, q)
	if err != nil {
		return Served{}, err
	}
	servedHash, servedBytes := value.Hash(), value.Bytes()
	if fr != nil {
		servedHash, servedBytes = fr.Hash(), fr.Bytes()
	}
	r := receipt.Receipt{
		LedgerID:             ledgerID,
		ServedHash:           servedHash,
		FacetIDs:             idsOf(value),
		Watermark:            int64(w),
		Template:             t.Name,
		TemplateVersion:      t.Version,
		TemplateHash:         t.Hash,
		DefinitionSetVersion: d.DefinitionSetVersion,
		ServedAt:             clock.Format(d.Clock.Now()),
		Span:                 t.Span(),
		Query:                q,
		Principal:            d.Reader,
	}
	if err := r.Validate(); err != nil {
		return Served{}, err
	}
	receiptBytes, err := r.Bytes()
	if err != nil {
		return Served{}, err
	}

	out := Served{Receipt: r, ReceiptHash: ops.Digest(receiptBytes), Frame: fr}
	if dryRun {
		return out, nil
	}
	if _, err := d.Published.Put(ctx, receipt.ServedKind, servedBytes); err != nil {
		return Served{}, err
	}
	if _, err := d.Published.Put(ctx, receipt.Kind, receiptBytes); err != nil {
		return Served{}, err
	}
	out.Written = true
	return out, nil
}

// Narrow is how a read narrows a template's value before publishing it, and how
// Verify narrows it again: the snapshot at q.At first, then the fact-time window
// [q.From, q.Before) and the type. The order is the timeline read's, so a published
// read is the one the reader saw. A nil q narrows nothing; an empty or contradictory
// one is malformed, the same refusal a receipt carrying it would meet.
func Narrow(v fold.Value, q *receipt.Query, s projection.Series) (fold.Value, error) {
	if q == nil {
		return v, nil
	}
	if q.IsZero() {
		return fold.Value{}, fmt.Errorf("%w: query narrows nothing; omit it", receipt.ErrMalformed)
	}
	if q.At != "" && q.Before != "" {
		return fold.Value{}, fmt.Errorf("%w: query has both at and before; a snapshot has one upper bound", receipt.ErrMalformed)
	}
	if q.At != "" {
		closes := func(facet.Type) bool { return false }
		if s != nil {
			closes = s.Closes
		}
		snap, err := fold.Snapshot(v, q.At, closes)
		if err != nil {
			return fold.Value{}, err
		}
		v = snap
	}
	if q.From == "" && q.Before == "" && q.Type == "" {
		return v, nil
	}
	var kept []fold.Entry
	for _, e := range v.Entries() {
		if q.Admits(e.FactTime, string(e.Type)) {
			kept = append(kept, e)
		}
	}
	return fold.NewValue(kept)
}

// Cut is how a read that named fields shapes a narrowed value before publishing it,
// and how Verify shapes it again: the frame of q's fields over v's entries (card
// 138). Nil when q names no fields: the value is served whole.
func Cut(v fold.Value, q *receipt.Query) (*frame.Frame, error) {
	if q == nil || len(q.Fields) == 0 {
		return nil, nil
	}
	f, err := frame.Of(v.Entries(), q.Fields)
	if err != nil {
		return nil, err
	}
	return &f, nil
}

// Verify honours a receipt: re-derive at its watermark, and re-hash the stored value. The two proofs are independent (see receipt.Result).
func Verify(ctx context.Context, d Deps, receiptHash string) (receipt.Receipt, receipt.Result, error) {
	raw, err := d.Published.Get(ctx, receipt.Kind, receiptHash)
	if err != nil {
		return receipt.Receipt{}, receipt.Result{}, err
	}
	r, err := receipt.Parse(raw)
	if err != nil {
		return receipt.Receipt{}, receipt.Result{}, err
	}

	// The exact version the receipt names, not the newest: every version ever held
	// stays readable (card 125), so a receipt served under v1 verifies after v2 is
	// registered. A version this record does not hold cannot re-derive the value.
	t, err := d.Templates.Version(r.Template, r.TemplateVersion)
	if err != nil {
		if errors.Is(err, projection.ErrUnknown) {
			return r, receipt.Result{}, fmt.Errorf("%w: the receipt was served under %s v%d: %v",
				ErrTemplateMoved, r.Template, r.TemplateVersion, err)
		}
		return r, receipt.Result{}, err
	}
	var notes []string
	if cur, err := d.Templates.Lookup(r.Template); err == nil && cur.Version != t.Version {
		notes = append(notes, fmt.Sprintf("replayed %s v%d from the held history; v%d is current", t.Name, t.Version, cur.Version))
	}
	switch r.TemplateHash {
	case t.Hash:
	case "":
		notes = append(notes, "receipt names no template hash (served before template hashes were recorded); the version held is taken to be the one that served")
	default:
		return r, receipt.Result{}, fmt.Errorf("%w: %s v%d is %s here, the receipt was served under %s",
			ErrTemplateMoved, t.Name, t.Version, t.Hash, r.TemplateHash)
	}

	ledgerID, err := d.Ledger.ID(ctx)
	if err != nil {
		return r, receipt.Result{}, err
	}
	switch r.LedgerID {
	case ledgerID:
	case "":
		notes = append(notes, "receipt names no ledger (served before ledger ids were recorded); its watermark is taken to be this ledger's")
	default:
		return r, receipt.Result{}, fmt.Errorf("%w: receipt names %s, this is %s; its watermark %d counts there, not here",
			ErrOtherLedger, r.LedgerID, ledgerID, r.Watermark)
	}

	if r.Program != nil {
		out, err := verifyQuery(ctx, d, r, t, notes)
		return r, out, err
	}
	res, err := valueAt(ctx, d, ledger.Watermark(r.Watermark))
	if err != nil {
		return r, receipt.Result{}, err
	}
	var at string
	if r.Query != nil {
		at = r.Query.At
	}
	value, err := Derive(res.Value, t, d.Gate, d.Series, at)
	if err != nil {
		return r, receipt.Result{}, err
	}
	if value, err = Narrow(value, r.Query, d.Series); err != nil {
		return r, receipt.Result{}, err
	}
	derivedHash := value.Hash()
	if fr, err := Cut(value, r.Query); err != nil {
		return r, receipt.Result{}, err
	} else if fr != nil {
		derivedHash = fr.Hash()
	}

	// Raw, not Get: the verifier wants the bytes as they are so it can say what they
	// hash to. Get would refuse a tampered row and report only that it refused.
	stored, err := d.Published.Raw(ctx, receipt.ServedKind, r.ServedHash)
	if err != nil && !errors.Is(err, ops.ErrNotFound) {
		return r, receipt.Result{}, err
	}

	out := receipt.Compare(r, derivedHash, idsOf(value), stored, ops.Digest)
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
// operational store alone: no ledger, no fold, no re-derivation.
//
// That restraint is the point. Verify asks whether the ledger still honours a receipt;
// Show asks what the store actually says, which is a question you must be able to
// answer when the ledger is gone, unreachable, or exactly what you doubt. The bytes are
// checked against their own content address on the way out, so a tampered row is
// reported rather than displayed as though it were what was served.
//
// A receipt whose query named fields names a frame, not a list of entries (card
// 138); Show returns it as Shown.Frame, with Entries empty.
func Show(ctx context.Context, d Deps, receiptHash string) (receipt.Receipt, Shown, error) {
	raw, err := d.Published.Get(ctx, receipt.Kind, receiptHash)
	if err != nil {
		return receipt.Receipt{}, Shown{}, err
	}
	r, err := receipt.Parse(raw)
	if err != nil {
		return receipt.Receipt{}, Shown{}, err
	}

	served, err := d.Published.Get(ctx, receipt.ServedKind, r.ServedHash)
	if err != nil {
		return r, Shown{}, err
	}
	if r.Program != nil {
		var a query.Answer
		if err := json.Unmarshal(served, &a); err != nil {
			return r, Shown{}, fmt.Errorf("serve: parse served answer %s: %w", r.ServedHash, err)
		}
		return r, Shown{Answer: &a}, nil
	}
	if r.Query != nil && len(r.Query.Fields) > 0 {
		f, err := frame.Parse(served)
		if err != nil {
			return r, Shown{}, fmt.Errorf("serve: parse served value %s: %w", r.ServedHash, err)
		}
		return r, Shown{Frame: &f}, nil
	}
	var entries []fold.Entry
	if err := json.Unmarshal(served, &entries); err != nil {
		return r, Shown{}, fmt.Errorf("serve: parse served value %s: %w", r.ServedHash, err)
	}
	return r, Shown{Entries: entries}, nil
}

// Shown is a served value read back from the store: the entries of a whole
// serve, the frame of one that named fields, or the answer of a query.
type Shown struct {
	Entries []fold.Entry
	Frame   *frame.Frame
	Answer  *query.Answer
}

// ListFilter narrows a listing of receipts. Each part is optional, and a receipt
// must pass every part that is set. Since and Until are clock.Layout instants, the
// window [Since, Until) of served time.
type ListFilter struct {
	// Principal keeps the receipts that name this reader.
	Principal string
	// Since keeps the receipts served at or after this instant.
	Since string
	// Until keeps the receipts served before this instant.
	Until string
}

// admits reports whether r passes f. served_at is a fixed-width clock.Layout string,
// so the lexical comparison is the chronological one.
func (f ListFilter) admits(r receipt.Receipt) bool {
	return (f.Principal == "" || r.Principal == f.Principal) &&
		(f.Since == "" || r.ServedAt >= f.Since) &&
		(f.Until == "" || r.ServedAt < f.Until)
}

// List returns the receipts in the store that pass f, newest serve first.
func List(ctx context.Context, d Deps, f ListFilter) ([]receipt.Receipt, []string, error) {
	digests, err := d.Published.List(ctx, receipt.Kind)
	if err != nil {
		return nil, nil, err
	}

	type listed struct {
		r    receipt.Receipt
		hash string
	}
	var kept []listed
	for _, digest := range digests {
		raw, err := d.Published.Get(ctx, receipt.Kind, digest)
		if err != nil {
			return nil, nil, err
		}
		r, err := receipt.Parse(raw)
		if err != nil {
			return nil, nil, fmt.Errorf("serve: receipt %s: %w", digest, err)
		}
		if f.admits(r) {
			kept = append(kept, listed{r, digest})
		}
	}

	// served_at is a fixed-width clock.Layout string, so the lexical sort is the
	// chronological one; the hash breaks ties on a same-instant pair.
	sort.Slice(kept, func(i, j int) bool {
		if kept[i].r.ServedAt != kept[j].r.ServedAt {
			return kept[i].r.ServedAt > kept[j].r.ServedAt
		}
		return kept[i].hash < kept[j].hash
	})
	receipts := make([]receipt.Receipt, len(kept))
	hashes := make([]string, len(kept))
	for i, k := range kept {
		receipts[i], hashes[i] = k.r, k.hash
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

// Derive is what template t serves from the canonical v as of fact-time at ("" for
// now). The canonical is cut at at before the template runs, so a snapshot-shaped
// template takes its latest-per-series at at: taken at the head and trimmed after, a
// course stopped later than at would be missing from every earlier date (card 172).
// Every read and Verify derive through here, so a receipt replays the read it names.
func Derive(v fold.Value, t projection.Template, g projection.Gate, s projection.Series, at string) (fold.Value, error) {
	v, err := fold.Until(v, at)
	if err != nil {
		return fold.Value{}, err
	}
	return projection.Apply(v, t, g, s)
}
