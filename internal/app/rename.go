package app

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/gate"
	"github.com/mattjoyce/anam/internal/read"
)

// A rename run carries every live facet of a type into another type as the next
// version of itself, through the gate (card 140): the same id, source, identity
// values, fact-time and payload, under the new type, with the new Definition's
// classification and series. The old version keeps its seq, so anam-time is kept: a
// read as-of a moment before the run is unchanged, and a read after shows the new
// type. The table is data (renametable.go), the dry run is the default, a rerun is
// all no-op, and a run that stops partway is re-run to finish. The daemon is stopped
// while it runs, as for a retraction: the run opens the ledger itself.

func renameCmd(cfgPath *string) *cobra.Command {
	var table string
	var yes bool
	cmd := &cobra.Command{
		Use:   "rename",
		Short: "carry the facets of one type into another, as new versions of themselves",
		Long: `Rename reads a table of types to leave and types to arrive at, and for every
live facet of a type left, offers the gate the same facet under the new type:
the same source, identity values and payload, so the same id, as its next
version. The old version stays, so a read --as-of a moment before the run is
unchanged, and a read after shows the new type. Nothing is written unless
--yes is given.

A rename may carry a select, to split one type into several: the source a
facet came from, and a string in its payload that starts with a prefix. Every
live facet of a type split this way must match exactly one rename.

The dry run is the report: per rename, how many facets would be carried, how
many already were, and how many the gate would refuse, with the reasons, and
every facet of a split type that matches no rename or more than one. Either
stops the run before anything is written. The two types must
key and place facets the same way, or the ids would not meet; the run checks
that against the stored Definitions before it asks the gate anything.

Stop the daemon first. The run opens the ledger itself, like a retraction.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadConfig(*cfgPath, cmd)
			if err != nil {
				return err
			}
			if cfg.Server != "" {
				return errors.New("rename runs against the files: unset the server and stop the daemon first")
			}
			tbl, err := loadRenameTable(table)
			if err != nil {
				return err
			}
			plan, err := planRename(cmd.Context(), cfg, tbl)
			if err != nil {
				return err
			}
			writeRenameReport(cmd, plan)
			if n := plan.count(renameUnplaced); n > 0 {
				return fmt.Errorf("%d facets match no rename or more than one; nothing written", n)
			}
			if n := plan.count(renameRefused); n > 0 {
				return fmt.Errorf("%d facets the gate would refuse; nothing written", n)
			}
			if plan.count(renameCarry) == 0 {
				return nil
			}
			if !yes {
				_, err = fmt.Fprintln(cmd.OutOrStdout(), "dry run: nothing written. Re-run with --yes to carry them.")
				return err
			}
			return renameAll(cmd, cfg, plan)
		},
	}
	cmd.Flags().StringVar(&table, "table", "",
		`JSON file naming the renames: {"renames": [{"from": "old.type", "to": "new.type", "select": {"source": "s", "pointer": "/id", "prefix": "p:"}}]}, select optional`)
	cmd.Flags().BoolVar(&yes, "yes", false, "write the new versions; without it the command only reports")
	_ = cmd.MarkFlagRequired("table")
	return cmd
}

// renameFate is what the gate would make of one facet's rename.
type renameFate int

const (
	renameCarry    renameFate = iota // the gate would supersede: carry it
	renameDone                       // already under the new type: a no-op
	renameRefused                    // the gate refuses, or the ledger does not hold it
	renameUnplaced                   // no rename matches it, or more than one does
)

// renameItem is one live facet of a type left, the renames that match it, and its
// fate. A facet is placed when exactly one rename matches; only then is the gate
// asked about it, and row is that rename's index in the table.
type renameItem struct {
	rows   []int
	facet  facet.Facet
	fate   renameFate
	reason string
}

// row is the index of the one rename that carries the item; placed items only.
func (it renameItem) row() int { return it.rows[0] }

// renamePlan is the dry run as a value: every live facet the table reaches, with
// what the gate would do to it, against the Definitions in force.
type renamePlan struct {
	table renameTable
	defs  *def.Registry
	items []renameItem
}

func (p renamePlan) count(f renameFate) int {
	n := 0
	for _, it := range p.items {
		if it.fate == f {
			n++
		}
	}
	return n
}

// planRename checks the keying, reads the fold, places every live facet of a type
// left on the one rename that matches it, and asks the gate what it would do with
// each facet placed. It writes nothing.
func planRename(ctx context.Context, cfg config.Config, tbl renameTable) (renamePlan, error) {
	defs, err := checkRenameKeying(ctx, cfg, tbl)
	if err != nil {
		return renamePlan{}, err
	}
	facets, err := liveFacetsLeaving(ctx, cfg, tbl)
	if err != nil {
		return renamePlan{}, err
	}
	l, err := openLedgerReadOnly(ctx, cfg)
	if err != nil {
		return renamePlan{}, err
	}
	defer func() { _ = l.Close() }()

	plan := renamePlan{table: tbl, defs: defs, items: placeAll(tbl, facets)}
	g := gate.New(l, defs, renameScopes(tbl, plan.items), clock.Real{}, cfg.PersonID, renameLimits(cfg))
	for i, it := range plan.items {
		if it.fate == renameUnplaced {
			continue
		}
		fctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
		res, err := g.Rehearse(fctx, renameOf(it.facet, tbl.Renames[it.row()].To))
		cancel()
		plan.items[i].fate, plan.items[i].reason = fateOf(res, err)
	}
	return plan, nil
}

// placeAll matches every facet against the table. A facet that matches no rename,
// or more than one, is unplaced, with the reason; the rest wait for the gate.
func placeAll(tbl renameTable, facets []facet.Facet) []renameItem {
	items := make([]renameItem, len(facets))
	for i, f := range facets {
		rows := tbl.place(f)
		items[i] = renameItem{rows: rows, facet: f}
		switch {
		case len(rows) == 0:
			items[i].fate, items[i].reason = renameUnplaced, "no rename's select matches it"
		case len(rows) > 1:
			nums := make([]string, len(rows))
			for j, r := range rows {
				nums[j] = strconv.Itoa(r + 1)
			}
			items[i].fate, items[i].reason = renameUnplaced, "renames "+strings.Join(nums, ", ")+" all match it"
		}
	}
	return items
}

// liveFacetsLeaving is every live facet of a type the table leaves, at the version
// in force, in timeline order.
func liveFacetsLeaving(ctx context.Context, cfg config.Config, tbl renameTable) ([]facet.Facet, error) {
	rctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	src, closeAll, err := readSource(rctx, cfg)
	if err != nil {
		return nil, err
	}
	defer closeAll()
	res, err := read.Canonical(rctx, src)
	if err != nil {
		return nil, err
	}
	leaving := map[facet.Type]bool{}
	for _, p := range tbl.Renames {
		leaving[p.From] = true
	}
	var entries []fold.Entry
	for _, e := range res.Value.Entries() {
		if leaving[e.Type] {
			entries = append(entries, e)
		}
	}
	return facetsOf(rctx, src.Ledger, entries)
}

// renameOf is a stored facet offered under another type: its source, binding and
// payload as they are, leaving the type it holds. The gate composes the same id.
func renameOf(f facet.Facet, to facet.Type) gate.Rename {
	return gate.Rename{
		Env:     facet.Envelope{Type: to, Source: f.Provenance.Source, Injector: "rename", Payload: f.Payload.Bytes()},
		Binding: f.Binding,
		From:    f.Type,
	}
}

// fateOf reads the gate's answer. A facet the ledger does not hold would be admitted
// as new, which a run that carries only facets the fold listed treats as a refusal.
func fateOf(res gate.Result, err error) (renameFate, string) {
	switch {
	case err != nil:
		return renameRefused, err.Error()
	case res.Outcome == gate.OutcomeSuperseded:
		return renameCarry, ""
	case res.Outcome == gate.OutcomeNoOp:
		return renameDone, ""
	default:
		return renameRefused, "the ledger does not hold this facet; it would be admitted as new"
	}
}

// renameScopes lets each placed facet's source submit the type its rename arrives
// at, and nothing else: the gate's authenticated pillar still runs, scoped to the run.
func renameScopes(tbl renameTable, items []renameItem) gate.StaticScopes {
	scopes := gate.StaticScopes{}
	seen := map[string]bool{}
	for _, it := range items {
		if it.fate == renameUnplaced {
			continue
		}
		src, to := it.facet.Provenance.Source, tbl.Renames[it.row()].To
		if key := src + " " + string(to); !seen[key] {
			seen[key] = true
			scopes[src] = append(scopes[src], to)
		}
	}
	return scopes
}

func renameLimits(cfg config.Config) gate.Limits {
	return gate.Limits{MaxPayloadBytes: cfg.MaxPayloadBytes, MaxRetries: cfg.CommitRetries}
}
