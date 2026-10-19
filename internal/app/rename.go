package app

import (
	"context"
	"errors"
	"fmt"

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

The dry run is the report: per rename, how many facets would be carried, how
many already were, and how many the gate would refuse, with the reasons. A
refusal anywhere stops the run before anything is written. The two types must
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
		`JSON file naming the renames: {"renames": [{"from": "old.type", "to": "new.type"}]}`)
	cmd.Flags().BoolVar(&yes, "yes", false, "write the new versions; without it the command only reports")
	_ = cmd.MarkFlagRequired("table")
	return cmd
}

// renameFate is what the gate would make of one facet's rename.
type renameFate int

const (
	renameCarry   renameFate = iota // the gate would supersede: carry it
	renameDone                      // already under the new type: a no-op
	renameRefused                   // the gate refuses, or the ledger does not hold it
)

// renameItem is one live facet of a type left, and its fate.
type renameItem struct {
	pair   renamePair
	facet  facet.Facet
	fate   renameFate
	reason string
}

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

// toOf is the table as a map from the type left to the type arrived at.
func (t renameTable) toOf() map[facet.Type]facet.Type {
	to := make(map[facet.Type]facet.Type, len(t.Renames))
	for _, p := range t.Renames {
		to[p.From] = p.To
	}
	return to
}

// planRename checks the keying, reads the fold, and asks the gate what it would do
// with every live facet of a type left. It writes nothing.
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
	g := gate.New(l, defs, renameScopes(tbl, facets), clock.Real{}, cfg.PersonID, renameLimits(cfg))

	to := tbl.toOf()
	plan := renamePlan{table: tbl, defs: defs}
	for _, f := range facets {
		item := renameItem{pair: renamePair{From: f.Type, To: to[f.Type]}, facet: f}
		fctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
		res, err := g.Rehearse(fctx, renameOf(f, to[f.Type]))
		cancel()
		item.fate, item.reason = fateOf(res, err)
		plan.items = append(plan.items, item)
	}
	return plan, nil
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

// renameScopes lets each facet's source submit the type it arrives at, and nothing
// else: the gate's authenticated pillar still runs, scoped to the run.
func renameScopes(tbl renameTable, facets []facet.Facet) gate.StaticScopes {
	to := tbl.toOf()
	scopes := gate.StaticScopes{}
	seen := map[string]bool{}
	for _, f := range facets {
		key := f.Provenance.Source + " " + string(to[f.Type])
		if seen[key] {
			continue
		}
		seen[key] = true
		scopes[f.Provenance.Source] = append(scopes[f.Provenance.Source], to[f.Type])
	}
	return scopes
}

func renameLimits(cfg config.Config) gate.Limits {
	return gate.Limits{MaxPayloadBytes: cfg.MaxPayloadBytes, MaxRetries: cfg.CommitRetries}
}
