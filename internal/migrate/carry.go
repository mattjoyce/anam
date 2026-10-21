package migrate

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/gate"
	"github.com/mattjoyce/anam/internal/span"
	"github.com/mattjoyce/anam/internal/store/ledger"
	"github.com/mattjoyce/anam/internal/store/ops"
)

// Target is the new ledger and operational store, which must not exist.
type Target struct {
	Ledger  string
	Store   string
	Buckets span.Buckets
}

// ErrExists is returned for a target file that is already there: a migration makes its
// files and never writes into one that exists.
var ErrExists = errors.New("migrate: the target already exists")

// CheckAbsent refuses a target file that exists, and the same path given twice. The
// ledger's and the store's sidecar files count: a leftover -wal beside a missing
// database would be read into the new one.
func (t Target) CheckAbsent() error {
	if t.Ledger == t.Store {
		return fmt.Errorf("%w: the ledger and the operational store are both %s", ErrExists, t.Ledger)
	}
	for _, p := range t.files() {
		if _, err := os.Lstat(p); err == nil {
			return fmt.Errorf("%w: %s", ErrExists, p)
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("migrate: look for %s: %w", p, err)
		}
	}
	return nil
}

// files is every file the two stores can leave on disk.
func (t Target) files() []string {
	var out []string
	for _, p := range []string{t.Ledger, t.Store} {
		out = append(out, p, p+"-wal", p+"-shm", p+"-journal")
	}
	return out
}

// remove deletes what a run that failed partway made, so a rerun finds no target.
func (t Target) remove() error {
	var errs []error
	for _, p := range t.files() {
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Carried is what a real run did.
type Carried struct {
	// Admitted is how many facets the gate admitted, W the new ledger's head.
	Admitted int
	W        ledger.Watermark
}

// Carry makes the new ledger and operational store and admits the selection into them.
// The Definitions come first: every registered Definition and projection of the old
// store is written to the new one before any facet, so every version a carried facet
// was admitted under is held there. Each facet then goes through the gate in blocks of
// gate.MaxBatch. A plan with a refusal writes nothing, and a run that fails partway
// removes the files it made.
func Carry(ctx context.Context, plan Plan, to Target, p Params) (done Carried, err error) {
	if n := plan.Refused(); n > 0 {
		return Carried{}, fmt.Errorf("migrate: %d facets the gate would refuse; nothing written", n)
	}
	if err := to.CheckAbsent(); err != nil {
		return Carried{}, err
	}
	defer func() {
		if err == nil {
			return
		}
		if rmErr := to.remove(); rmErr != nil {
			err = errors.Join(err, fmt.Errorf("migrate: remove the partial target: %w", rmErr))
		}
	}()

	rctx, cancel := context.WithTimeout(ctx, p.Phase)
	defer cancel()
	l, db, err := createTarget(rctx, to, p)
	if err != nil {
		return Carried{}, err
	}
	defer func() { _ = l.Close() }()
	defer func() { _ = db.Close() }()

	cctx, ccancel := context.WithTimeout(rctx, p.Timeout)
	err = db.CarryRegistered(cctx, plan.Registered)
	ccancel()
	if err != nil {
		return Carried{}, err
	}

	facets := make([]facet.Facet, len(plan.Items))
	for i, it := range plan.Items {
		facets[i] = it.Facet
	}
	g := carryGate(plan, facets, p, l)

	for start := 0; start < len(facets); start += gate.MaxBatch {
		end := min(start+gate.MaxBatch, len(facets))
		batch := make([]gate.Carry, 0, end-start)
		for i, f := range facets[start:end] {
			batch = append(batch, carryOf(f, plan.Items[start+i].Under))
		}
		bctx, bcancel := context.WithTimeout(rctx, p.Timeout)
		results, w, berr := g.CarryBatch(bctx, batch)
		bcancel()
		if berr != nil {
			return Carried{}, fmt.Errorf("migrate: after %d of %d carried: %w", done.Admitted, len(facets), berr)
		}
		for _, r := range results {
			if r.Outcome == gate.OutcomeAdmitted {
				done.Admitted++
			}
		}
		done.W = w
	}
	if done.Admitted != len(facets) {
		return Carried{}, fmt.Errorf("migrate: the gate admitted %d of %d facets", done.Admitted, len(facets))
	}
	return done, nil
}

// createTarget makes the new ledger as `anam init` does, and the operational store
// beside it.
func createTarget(ctx context.Context, to Target, p Params) (*ledger.Ledger, *ops.DB, error) {
	l, err := ledger.Create(ctx, to.Ledger, p.Person)
	if err != nil {
		return nil, nil, fmt.Errorf("migrate: new ledger: %w", err)
	}
	db, err := ops.Open(ctx, to.Store, to.Buckets)
	if err != nil {
		_ = l.Close()
		return nil, nil, fmt.Errorf("migrate: new operational store: %w", err)
	}
	return l, db, nil
}

// scopesOf lets each facet's source submit the facet's own type and nothing else.
func scopesOf(facets []facet.Facet) gate.StaticScopes {
	scopes := gate.StaticScopes{}
	seen := map[string]bool{}
	for _, f := range facets {
		src := f.Provenance.Source
		if key := src + " " + string(f.Type); !seen[key] {
			seen[key] = true
			scopes[src] = append(scopes[src], f.Type)
		}
	}
	return scopes
}

// Proof is what a carried ledger is checked against.
type Proof struct {
	// W is the new ledger's head, Live its live facets.
	W    ledger.Watermark
	Live int
	// Moves are the definition version moves the new ledger holds, read from its own
	// facets against the ones read from the old: the facets that arrived under a
	// version other than the one they were admitted under. They are reported apart
	// from the identity check, and each must be the version the plan chose.
	Moves []Move
	// Mismatches are the ways the new ledger's live facets differ from the selection,
	// at most a screenful. Empty is equal, facet for facet.
	Mismatches []string
}

// Equal reports that the new ledger's live facets are the selection's.
func (pr Proof) Equal() bool { return len(pr.Mismatches) == 0 }

// maxMismatches bounds how many differences a proof lists.
const maxMismatches = 10

// Prove reads the new ledger read-only, folds it, and holds its live facets to the
// plan's selection, facet for facet by id, content hash, fact time and status. A live
// facet the selection lacks and a selected facet the ledger lacks both count. The
// definition version a facet arrived under is held to the one the plan chose, and the
// facets that arrived under another version than they were admitted under are
// reported as moves; the version is not part of a fact's identity.
func Prove(ctx context.Context, plan Plan, newLedger string, p Params) (Proof, error) {
	octx, cancel := context.WithTimeout(ctx, p.Timeout)
	l, err := ledger.OpenReadOnly(octx, newLedger, p.Person)
	cancel()
	if err != nil {
		return Proof{}, fmt.Errorf("migrate: new ledger: %w", err)
	}
	defer func() { _ = l.Close() }()
	rctx, rcancel := context.WithTimeout(ctx, p.Phase)
	defer rcancel()
	res, err := fold.Read(rctx, l)
	if err != nil {
		return Proof{}, fmt.Errorf("migrate: fold %s: %w", newLedger, err)
	}

	pr := Proof{W: res.W, Live: len(res.Live)}
	add := func(format string, args ...any) {
		if len(pr.Mismatches) < maxMismatches {
			pr.Mismatches = append(pr.Mismatches, fmt.Sprintf(format, args...))
		}
	}
	want := make(map[string]Item, len(plan.Items))
	for _, it := range plan.Items {
		want[it.Facet.ID] = it
	}
	var moved []movedFacet
	for _, f := range res.Live {
		it, ok := want[f.ID]
		switch {
		case !ok:
			add("%s (%s) is live in the new ledger and was not selected", f.ID, f.Type)
			continue
		case f.Descriptor.ContentHash != it.Facet.Descriptor.ContentHash:
			add("%s content hash %s, selected %s", f.ID, f.Descriptor.ContentHash, it.Facet.Descriptor.ContentHash)
		case !f.FactTime.Equal(it.Facet.FactTime):
			add("%s fact time %s, selected %s", f.ID, clock.Format(f.FactTime), clock.Format(it.Facet.FactTime))
		case res.States[f.ID].Status != it.Status:
			add("%s status %s, selected %s", f.ID, res.States[f.ID].Status, it.Status)
		case f.DefinitionVersion != it.Under:
			add("%s arrived under definition v%d, the plan chose v%d", f.ID, f.DefinitionVersion, it.Under)
		}
		if f.DefinitionVersion != it.Facet.DefinitionVersion {
			moved = append(moved, movedFacet{f.Type, f.Provenance.Source, it.Facet.DefinitionVersion, f.DefinitionVersion})
		}
		delete(want, f.ID)
	}
	for id, it := range want {
		add("%s (%s) was selected and is not live in the new ledger", id, it.Facet.Type)
	}
	pr.Moves = groupMoves(moved)
	return pr, nil
}
