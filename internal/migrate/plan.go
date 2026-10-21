// Package migrate carries the live facets of one ledger into a fresh one (card 200).
//
// The old ledger and its operational store are opened read-only and never written.
// The selection is the old ledger's live facets at their head, folded by the one fold,
// or the ones a Datalog program's pick/1 selects over that fold. Each is offered to a
// new ledger through the gate (gate.CarryBatch), under the Definition version it was
// admitted under, so its id, content hash and fact time come out as they were. Nothing
// here writes a ledger table: the new ledger is made by ledger.Create, its facets are
// admitted by the gate, and a run that stops partway removes the files it made.
//
// The package is disposable and holds no facts: a failed run is run again. The error
// kernel is the gate and the ledger's triggers, which a migration meets like any
// writer.
package migrate

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/mattjoyce/anam/internal/catchup"
	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/gate"
	"github.com/mattjoyce/anam/internal/read"
	"github.com/mattjoyce/anam/internal/selection"
	"github.com/mattjoyce/anam/internal/store/ledger"
	"github.com/mattjoyce/anam/internal/store/ops"
)

// Params are the choices a run is given by whoever loaded the config: the person the
// ledgers belong to, the gate's two limits, and the two time bounds.
type Params struct {
	Person string
	Limits gate.Limits

	// Timeout bounds each call to a store or to the gate.
	Timeout time.Duration
	// Phase bounds a whole phase: the fold of the old ledger, the carry.
	Phase time.Duration
}

// Source is the old ledger and operational store, which a run only reads.
type Source struct {
	Ledger string
	Store  string
}

// Fate is what the gate would make of one selected facet.
type Fate int

// The two fates. A facet the gate would refuse stops a real run before it writes.
const (
	FateCarry   Fate = iota // the gate would admit it
	FateRefused             // a pillar refuses it, or the carry would not give the same facet
)

// Item is one selected facet: the current version of a live facet of the old ledger,
// its folded status, and what the gate would make of it.
type Item struct {
	Facet  facet.Facet
	Status facet.Status
	Fate   Fate
	Reason string
}

// Link is a record.edge in the selection that names a facet the selection does not
// carry. The edge is carried as it is; the facet's id derives from its identity, so
// carrying that facet later reconnects it.
type Link struct {
	Edge facet.Facet
	Side string
	// FacetID is the end that is not carried, and Standing says what the old ledger
	// holds of it: live but not selected, tombstoned, or nothing.
	FacetID  string
	Standing string
}

// Plan is the dry run as a value: the old ledger's head, the selection with what the
// gate would do to each facet, the links that would dangle, and what a real run needs
// to carry.
type Plan struct {
	// W is the old ledger's head, and Live how many facets are live there.
	W    ledger.Watermark
	Live int

	// Items is the selection in timeline order.
	Items []Item
	// NotLive are ids a program picked that no live facet holds.
	NotLive []string
	Links   []Link

	// Registered is what the old operational store holds that the ledger cannot give
	// back, to be written into the new one before any facet.
	Registered ops.Registered

	current *def.Registry
	held    *def.Versions
}

// Refused is how many selected facets the gate would refuse.
func (p Plan) Refused() int {
	n := 0
	for _, it := range p.Items {
		if it.Fate == FateRefused {
			n++
		}
	}
	return n
}

// Read reads the old ledger and store read-only, selects, and asks the gate what it
// would do with every facet selected. It writes nothing. program is Datalog text that
// defines pick/1 over the query surface, or empty for every live facet.
func Read(ctx context.Context, src Source, program string, p Params) (Plan, error) {
	l, db, err := openSource(ctx, src, p)
	if err != nil {
		return Plan{}, err
	}
	defer func() { _ = l.Close() }()
	defer func() { _ = db.Close() }()

	rctx, cancel := context.WithTimeout(ctx, p.Phase)
	defer cancel()
	registered, err := db.ReadRegistered(rctx)
	if err != nil {
		return Plan{}, err
	}
	current, held, err := definitionsOf(registered)
	if err != nil {
		return Plan{}, err
	}
	res, err := fold.Read(rctx, l)
	if err != nil {
		return Plan{}, fmt.Errorf("migrate: fold %s: %w", src.Ledger, err)
	}
	picked, notLive, err := selectFacets(rctx, res, current, program)
	if err != nil {
		return Plan{}, err
	}

	plan := Plan{W: res.W, Live: len(res.Live), NotLive: notLive, Registered: registered, current: current, held: held}
	// The dry run reads no target ledger: into one with nothing in it every carry is a
	// first admission, and the pillars VetCarry runs are all that can refuse it.
	g := carryGate(plan, picked, p, nil)
	for _, f := range picked {
		it := Item{Facet: f, Status: res.States[f.ID].Status}
		it.Fate, it.Reason = vet(g, held, it)
		plan.Items = append(plan.Items, it)
	}
	if plan.Links, err = danglingLinks(plan.Items, res.States); err != nil {
		return Plan{}, err
	}
	return plan, nil
}

// openSource opens the old ledger and operational store on handles SQLite will not
// accept a write from.
func openSource(ctx context.Context, src Source, p Params) (*ledger.Ledger, *ops.DB, error) {
	octx, cancel := context.WithTimeout(ctx, p.Timeout)
	defer cancel()
	l, err := ledger.OpenReadOnly(octx, src.Ledger, p.Person)
	if err != nil {
		return nil, nil, fmt.Errorf("migrate: old ledger: %w", err)
	}
	db, err := ops.OpenReadOnly(octx, src.Store)
	if err != nil {
		_ = l.Close()
		return nil, nil, fmt.Errorf("migrate: old operational store: %w", err)
	}
	return l, db, nil
}

// definitionsOf builds the Definitions in force in the old store (the binary's, with
// the registered ones beside them, as the daemon would load them) and every version it
// holds as a contract: the binary's own, the registered rows, and the store's copy of
// what the binary carried when it last started.
func definitionsOf(r ops.Registered) (*def.Registry, *def.Versions, error) {
	bundled, err := def.Bundled()
	if err != nil {
		return nil, nil, err
	}
	registered := make([]def.Stored, len(r.Definitions))
	for i, d := range r.Definitions {
		registered[i] = def.Stored{Type: facet.Type(d.Type), Version: d.Version, Body: []byte(d.Body)}
	}
	current, err := def.WithRegistered(bundled, registered)
	if err != nil {
		return nil, nil, err
	}
	manifests, err := def.Manifests()
	if err != nil {
		return nil, nil, err
	}
	provisioned := make([]def.Stored, len(r.Provisioned))
	for i, d := range r.Provisioned {
		provisioned[i] = def.Stored{Type: facet.Type(d.Type), Version: d.Version, Body: []byte(d.Body)}
	}
	return current, def.NewVersions(current, registered, manifests, provisioned), nil
}

// selectFacets is the facets the run carries: every live facet at its head, or those a
// program's pick/1 selects. The program is evaluated over the canonical as it stands,
// the fold's whole live value with no projection narrowing, which is the value the
// selection page evaluates over for its canonical, so no template is needed: the query
// reads a served value and the canonical is one. A pick of an id no live facet holds
// is reported, not carried.
func selectFacets(ctx context.Context, res fold.Result, current *def.Registry, program string) (picked []facet.Facet, notLive []string, err error) {
	if program == "" {
		return res.Live, nil, nil
	}
	// A local run is the operator at their own files: no bound on the derivation.
	ans, _, err := read.Evaluate(ctx, res.Value, res.Live, current, program, selection.Relation, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("migrate: the program: %w", err)
	}
	ids := map[string]bool{}
	for _, t := range ans.Tuples {
		ids[t[0].Text()] = true
	}
	for _, f := range res.Live {
		if ids[f.ID] {
			picked = append(picked, f)
			delete(ids, f.ID)
		}
	}
	for id := range ids {
		notLive = append(notLive, id)
	}
	sort.Strings(notLive)
	return picked, notLive, nil
}

// carryOf is a stored facet offered to a fresh ledger as itself: its type, source,
// injector, source ref and payload, its binding, and the Definition version it was
// admitted under.
func carryOf(f facet.Facet) gate.Carry {
	return gate.Carry{
		Env: facet.Envelope{
			Type: f.Type, Source: f.Provenance.Source, Injector: f.Provenance.Injector,
			SourceRef: f.Provenance.SourceRef, Payload: f.Payload.Bytes(),
		},
		Binding:           f.Binding,
		DefinitionVersion: f.DefinitionVersion,
	}
}

// carryGate is the gate a carry runs on: each facet's source scoped to the facet's own
// type and nothing else, as the rename scopes its carry, with the Definition versions
// the old store holds. The authenticated pillar still runs, scoped to the run.
func carryGate(plan Plan, facets []facet.Facet, p Params, l gate.Ledger) *gate.Gate {
	return gate.New(l, plan.current, scopesOf(facets), clock.Real{}, p.Person, p.Limits).Holding(plan.held)
}

// vet asks the gate what it would make of a facet, and holds the answer to the facet
// that was read: a carry that would give another id, hash, fact time or classification
// is a refusal, because the new ledger would not hold the same fact.
func vet(g *gate.Gate, held *def.Versions, it Item) (Fate, string) {
	f := it.Facet
	if it.Status != facet.StatusAdmitted {
		return FateRefused, fmt.Sprintf("its status is %q, and the gate admits only a first sight", it.Status)
	}
	if _, ok := held.At(f.Type, f.DefinitionVersion); !ok {
		return FateRefused, held.Why(f.Type, f.DefinitionVersion)
	}
	got, err := g.VetCarry(carryOf(f))
	if err != nil {
		return FateRefused, err.Error()
	}
	if why := differs(f, got); why != "" {
		return FateRefused, "the gate would make another facet of it: " + why
	}
	return FateCarry, ""
}

// differs names the first way the facet the gate would build is not the facet read.
// Version and the moment of admission are the carry's own; everything else is the
// fact.
func differs(old, got facet.Facet) string {
	switch {
	case got.ID != old.ID:
		return fmt.Sprintf("id %s, not %s", got.ID, old.ID)
	case got.Descriptor.ContentHash != old.Descriptor.ContentHash:
		return fmt.Sprintf("content hash %s, not %s", got.Descriptor.ContentHash, old.Descriptor.ContentHash)
	case !got.FactTime.Equal(old.FactTime):
		return fmt.Sprintf("fact time %s, not %s", clock.Format(got.FactTime), clock.Format(old.FactTime))
	case got.Series != old.Series:
		return fmt.Sprintf("series %q, not %q", got.Series, old.Series)
	case got.Classification != old.Classification:
		return fmt.Sprintf("classification %s, not %s", got.Classification, old.Classification)
	case got.Descriptor != old.Descriptor:
		return fmt.Sprintf("descriptor %+v, not %+v", got.Descriptor, old.Descriptor)
	case got.DefinitionVersion != old.DefinitionVersion:
		return fmt.Sprintf("definition v%d, not v%d", got.DefinitionVersion, old.DefinitionVersion)
	case got.Binding != old.Binding:
		return fmt.Sprintf("binding %+v, not %+v", got.Binding, old.Binding)
	}
	return ""
}

// danglingLinks is every end of a selected record.edge that the selection does not
// carry, in the order of the edges.
func danglingLinks(items []Item, states map[string]fold.State) ([]Link, error) {
	selected := make(map[string]bool, len(items))
	for _, it := range items {
		selected[it.Facet.ID] = true
	}
	var links []Link
	for _, it := range items {
		if it.Facet.Type != catchup.EdgeType {
			continue
		}
		ends, err := catchup.EdgeEnds(it.Facet)
		if err != nil {
			return nil, fmt.Errorf("migrate: edge %s: %w", it.Facet.Provenance.SourceRef, err)
		}
		for _, e := range ends {
			if selected[e.FacetID] {
				continue
			}
			links = append(links, Link{Edge: it.Facet, Side: e.Side, FacetID: e.FacetID, Standing: standing(states, e.FacetID)})
		}
	}
	return links, nil
}

// standing says what the old ledger holds of a facet a link names but the selection
// does not carry.
func standing(states map[string]fold.State, id string) string {
	st, ok := states[id]
	switch {
	case !ok:
		return "not in the old ledger"
	case st.Live():
		return "live, not selected"
	default:
		return string(st.Status)
	}
}

// Same reports whether two plans select the same facets: the same ids, content hashes,
// types and sources, and the gate would carry all of them. It is the proof that a carry
// is idempotent: the plan read from the new ledger is the plan read from the old.
func Same(a, b Plan) error {
	if len(a.Items) != len(b.Items) {
		return fmt.Errorf("%d facets selected, then %d", len(a.Items), len(b.Items))
	}
	byID := make(map[string]Item, len(a.Items))
	for _, it := range a.Items {
		byID[it.Facet.ID] = it
	}
	var errs []error
	for _, it := range b.Items {
		was, ok := byID[it.Facet.ID]
		switch {
		case !ok:
			errs = append(errs, fmt.Errorf("%s is selected now and was not", it.Facet.ID))
		case was.Facet.Descriptor.ContentHash != it.Facet.Descriptor.ContentHash:
			errs = append(errs, fmt.Errorf("%s has content hash %s, was %s", it.Facet.ID, it.Facet.Descriptor.ContentHash, was.Facet.Descriptor.ContentHash))
		case was.Facet.Type != it.Facet.Type || was.Facet.Provenance.Source != it.Facet.Provenance.Source:
			errs = append(errs, fmt.Errorf("%s is %s from %s, was %s from %s", it.Facet.ID, it.Facet.Type, it.Facet.Provenance.Source, was.Facet.Type, was.Facet.Provenance.Source))
		case it.Fate != FateCarry:
			errs = append(errs, fmt.Errorf("%s would be refused: %s", it.Facet.ID, it.Reason))
		}
		if len(errs) >= 5 {
			errs = append(errs, errors.New("and more"))
			break
		}
	}
	return errors.Join(errs...)
}
