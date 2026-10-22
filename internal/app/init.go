package app

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/store/ledger"
	"github.com/mattjoyce/anam/internal/store/ops"
)

func initCmd(cfgPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "idempotently create the ledger file and the operational store",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadConfig(*cfgPath, cmd)
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), cfg.Timeout)
			defer cancel()

			l, err := ledger.Create(ctx, cfg.LedgerPath, cfg.PersonID)
			if err != nil {
				return err
			}
			defer func() { _ = l.Close() }()
			buckets, err := cfg.Buckets()
			if err != nil {
				return err
			}
			db, err := ops.Open(ctx, cfg.DBPath, buckets)
			if err != nil {
				return err
			}
			defer func() { _ = db.Close() }()

			_, err = fmt.Fprintf(cmd.OutOrStdout(),
				"ledger %s ready (person %s); operational store %s ready\n",
				cfg.LedgerPath, cfg.PersonID, cfg.DBPath)
			return err
		},
	}
}

// openLedgerForWrite opens an existing ledger for writing. It creates nothing: a
// write against a path with no ledger fails and names `anam init`.
func openLedgerForWrite(ctx context.Context, cfg config.Config) (*ledger.Ledger, error) {
	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	return ledger.Open(ctx, cfg.LedgerPath, cfg.PersonID)
}

// registryOf is the bundled Definitions with those importers registered, as db holds
// them: the Definitions the record's facets were admitted against.
func registryOf(ctx context.Context, db *ops.DB) (*def.Registry, error) {
	bundled, err := def.Bundled()
	if err != nil {
		return nil, err
	}
	rows, err := db.RegisteredDefinitions(ctx)
	if err != nil {
		return nil, err
	}
	stored := make([]def.Stored, len(rows))
	for i, r := range rows {
		stored[i] = def.Stored{Type: facet.Type(r.Type), Version: r.Version, Body: []byte(r.Body)}
	}
	return def.WithRegistered(bundled, stored)
}

// openStore opens the operational store at cfg's path, which a command that reads
// only it (receipt list, receipt show) needs and no ledger. The schema and the bucket
// width are checked as every opener checks them.
func openStore(ctx context.Context, cfg config.Config) (*ops.DB, error) {
	buckets, err := cfg.Buckets()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	return ops.Open(ctx, cfg.DBPath, buckets)
}
