package app

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/gate"
	"github.com/mattjoyce/anam/internal/policy"
	"github.com/mattjoyce/anam/internal/retract"
	"github.com/mattjoyce/anam/internal/store/ledger"
	"github.com/mattjoyce/anam/internal/store/ops"
)

// A source nobody resends has no snapshot to compare with, so a wrong entry in it is
// undone by naming the one facet (card 188, slice 3). The plan is internal/retract's,
// the same one POST /v1/facets/{id}/retract applies; the tombstone is gate.Retract.

// runFacetRetraction plans, and with yes tombstones, one facet of source. The dry run
// opens the ledger read-only, so it cannot write.
func runFacetRetraction(cmd *cobra.Command, cfg config.Config, source, facetID string, yes bool) error {
	file, err := importersFile(cfg)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(cmd.Context(), cfg.Timeout)
	defer cancel()

	l, err := openFacetLedger(ctx, cfg, yes)
	if err != nil {
		return err
	}
	defer func() { _ = l.Close() }()
	db, err := ops.OpenReadOnly(ctx, cfg.DBPath)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	plan, err := retract.PlanFacet(ctx, l, db, policy.MayRetract(file, source), source, facetID)
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	f := plan.Facet
	_, _ = fmt.Fprintf(out, "%s: %s\n  type %s, v%d, fact-time %s, status %s\n",
		source, f.ID, f.Type, f.ArtifactVersion, clock.Format(f.FactTime), plan.Status)
	if !yes {
		_, err = fmt.Fprintf(out, "dry run: nothing written. --yes appends one tombstone to v%d; the artifact stays and a read --at before it still shows the facet.\n",
			f.ArtifactVersion)
		return err
	}
	reg, err := def.Bundled()
	if err != nil {
		return err
	}
	// A tombstone admits nothing, so the gate needs no scopes for it.
	g := gate.New(l, reg, gate.StaticScopes{}, clock.Real{}, cfg.PersonID,
		gate.Limits{MaxPayloadBytes: cfg.MaxPayloadBytes, MaxRetries: cfg.CommitRetries})
	res, w, err := g.Retract(ctx, f.ID)
	if err != nil {
		return err
	}
	if res.Outcome != gate.OutcomeTombstoned {
		_, err = fmt.Fprintf(out, "already tombstoned by another writer; the ledger is at W=%d\n", w)
		return err
	}
	_, err = fmt.Fprintf(out, "tombstoned %s v%d; the ledger moved to W=%d\n", f.ID, res.ArtifactVersion, w)
	return err
}

// openFacetLedger is the ledger the plan reads: writable only when the tombstone will
// be written.
func openFacetLedger(ctx context.Context, cfg config.Config, write bool) (*ledger.Ledger, error) {
	if write {
		return openLedgerForWrite(ctx, cfg)
	}
	return openLedgerReadOnly(ctx, cfg)
}

// importersFile is the importers file's principals, none when no file is configured.
// The policy reads it for the sources it declares.
func importersFile(cfg config.Config) ([]config.Principal, error) {
	if cfg.ImportersFile == "" {
		return nil, nil
	}
	reg, err := def.Bundled()
	if err != nil {
		return nil, err
	}
	return config.LoadPrincipals(cfg.ImportersFile, reg.Has)
}
