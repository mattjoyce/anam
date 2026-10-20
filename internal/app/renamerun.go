package app

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/gate"
	"github.com/mattjoyce/anam/internal/store/ops"
)

// checkRenameKeying refuses the run before anything else when a type arrived at
// composes ids from another identity key than the type left: the id is the source and
// the identity values, so the ids would not meet and the run would admit duplicates
// beside the originals. The newest stored body of the type left is compared, as
// Guard compares versions, because Guard already holds every stored version of a
// type to one keying. A different fact-time rule is allowed (card 195): the gate
// places each carried facet by the rule of the type it arrives at, and the facet's
// earlier versions keep their own fact time, so a read as-of before the run is
// unchanged. It answers the Definitions in force, for the plan to use, and the rule
// each re-placing rename leaves, by its index in the table.
func checkRenameKeying(ctx context.Context, cfg config.Config, tbl renameTable) (*def.Registry, map[int]def.TimeField, error) {
	buckets, err := cfg.Buckets()
	if err != nil {
		return nil, nil, err
	}
	db, err := ops.Open(ctx, cfg.DBPath, buckets)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = db.Close() }()
	defs, err := registryOf(ctx, db)
	if err != nil {
		return nil, nil, err
	}
	newest, err := newestHeldBodies(ctx, db)
	if err != nil {
		return nil, nil, err
	}
	replaced := map[int]def.TimeField{}
	for i, p := range tbl.Renames {
		body, ok := newest[p.From]
		if !ok {
			return nil, nil, fmt.Errorf("rename: %s has no stored Definition, so nothing was admitted under it", p.From)
		}
		k, err := def.KeyingOf(body)
		if err != nil {
			return nil, nil, fmt.Errorf("rename: the stored %s: %w", p.From, err)
		}
		to, ok := defs.Lookup(p.To)
		if !ok {
			return nil, nil, fmt.Errorf("rename: %s arrives at %s, which has no Definition in force", p.From, p.To)
		}
		if err := to.SameIdentity(k); err != nil {
			return nil, nil, fmt.Errorf("rename: %s cannot arrive at %s v%d: %w; the ids would not meet", p.From, p.To, to.Version, err)
		}
		if k.FactTime != to.FactTime {
			replaced[i] = k.FactTime
		}
	}
	return defs, replaced, nil
}

// newestHeldBodies is the newest body held of every type: the binary's own
// manifests, then what the operational store keeps, bundled and registered. The
// binary's come first because a ledger no daemon has run against has an empty
// store, and a type the binary dropped is still in the store.
func newestHeldBodies(ctx context.Context, db *ops.DB) (map[facet.Type][]byte, error) {
	bundled, err := def.Manifests()
	if err != nil {
		return nil, err
	}
	stored, err := db.Definitions(ctx)
	if err != nil {
		return nil, err
	}
	registered, err := db.RegisteredDefinitions(ctx)
	if err != nil {
		return nil, err
	}
	newest := map[facet.Type][]byte{}
	version := map[facet.Type]int{}
	keep := func(t facet.Type, v int, body []byte) {
		if cur, seen := version[t]; seen && cur >= v {
			return
		}
		version[t] = v
		newest[t] = body
	}
	for _, m := range bundled {
		keep(m.Type, m.Version, m.Body)
	}
	for _, r := range append(stored, registered...) {
		keep(facet.Type(r.Type), r.Version, []byte(r.Body))
	}
	return newest, nil
}

// writeRenameReport says what the run would do, per rename, and names every facet the
// gate would refuse and every facet no one rename places, with the reason: a run
// carries thousands, so the shape is the count per pair and a line per refusal. A facet already carried sits under its new
// type, so the fold never lists it again: a rerun or a resumed run plans only what is
// left, and a no-op from the gate is an oddity worth one line, not a column.
func writeRenameReport(cmd *cobra.Command, plan renamePlan) {
	out := cmd.OutOrStdout()
	type tally struct{ carry, refused int }
	by := make([]tally, len(plan.table.Renames))
	for _, it := range plan.items {
		switch it.fate {
		case renameCarry:
			by[it.row()].carry++
		case renameRefused:
			by[it.row()].refused++
		case renameDone, renameUnplaced:
		}
	}
	w := tabwriter.NewWriter(out, 2, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "FROM\tTO\tCARRY\tREFUSED\tSELECT")
	for i, p := range plan.table.Renames {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%d\t%d\t%s\n", p.From, p.To, by[i].carry, by[i].refused, p.Select)
	}
	_ = w.Flush()
	writeReplaced(out, plan)
	if n := plan.count(renameDone); n > 0 {
		_, _ = fmt.Fprintf(out, "%d facets are already under their new type\n", n)
	}
	for _, it := range plan.items {
		switch {
		case it.fate == renameRefused:
			_, _ = fmt.Fprintf(out, "refused %s: %s\n", it.facet.Provenance.SourceRef, it.reason)
		case it.fate == renameUnplaced && len(it.rows) == 0:
			_, _ = fmt.Fprintf(out, "unmatched %s (%s): %s\n", it.facet.Provenance.SourceRef, it.facet.Type, it.reason)
		case it.fate == renameUnplaced:
			_, _ = fmt.Fprintf(out, "doubly matched %s (%s): %s\n", it.facet.Provenance.SourceRef, it.facet.Type, it.reason)
		}
	}
}

// writeReplaced names each rename that places its facets by another fact-time rule
// than the type they leave, and how many of the facets it would carry the new rule
// moves in fact time (card 195).
func writeReplaced(out io.Writer, plan renamePlan) {
	for i, p := range plan.table.Renames {
		was, ok := plan.replaced[i]
		if !ok {
			continue
		}
		to, _ := plan.defs.Lookup(p.To)
		moved, carried := 0, 0
		for _, it := range plan.items {
			if it.fate == renameCarry && it.row() == i {
				carried++
				if it.moved {
					moved++
				}
			}
		}
		_, _ = fmt.Fprintf(out, "%s to %s places facets by %s (%s, %s), where %s read %s (%s, %s): %d of %d to carry move in fact time\n",
			p.From, p.To, to.FactTime.Pointer, to.FactTime.Layout, to.FactTime.Zone,
			p.From, was.Pointer, was.Layout, was.Zone, moved, carried)
	}
}

// renameAll writes the plan: every facet the gate would carry, in blocks of MaxBatch,
// each block one transaction. A block that fails names how far the run got; the run
// again is all no-op up to there and carries on.
func renameAll(cmd *cobra.Command, cfg config.Config, plan renamePlan) error {
	var carry []renameItem
	for _, it := range plan.items {
		if it.fate == renameCarry {
			carry = append(carry, it)
		}
	}
	l, err := openLedgerForWrite(cmd.Context(), cfg)
	if err != nil {
		return err
	}
	defer func() { _ = l.Close() }()
	g := gate.New(l, plan.defs, renameScopes(plan.table, carry), clock.Real{}, cfg.PersonID, renameLimits(cfg))

	done := 0
	for start := 0; start < len(carry); start += gate.MaxBatch {
		end := min(start+gate.MaxBatch, len(carry))
		batch := make([]gate.Rename, 0, end-start)
		for _, it := range carry[start:end] {
			batch = append(batch, renameOf(it.facet, plan.table.Renames[it.row()].To))
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), cfg.Timeout)
		results, _, err := g.RenameBatch(ctx, batch)
		cancel()
		if err != nil {
			return fmt.Errorf("after %d of %d carried: %w (re-run to finish)", done, len(carry), err)
		}
		for _, r := range results {
			if r.Outcome == gate.OutcomeSuperseded {
				done++
			}
		}
	}
	w, err := l.Head(cmd.Context())
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(cmd.OutOrStdout(), "carried %d facets into their new types; the ledger is at W %d\n", done, w)
	return err
}
