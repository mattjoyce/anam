package app

import (
	"context"

	"github.com/mattjoyce/anam/internal/catalog"
	"github.com/mattjoyce/anam/internal/config"
)

// loadCatalog loads the provisioned catalog a local command reads, once per
// invocation, from cfg's definitions_dir and projections_dir (the built-in catalog
// for either left empty): the Definitions and projections every registry it builds
// starts from, and the set version its receipts name. A command that asks a daemon
// does not call it; the daemon loaded its own, and a client's directories are never
// opened.
func loadCatalog(ctx context.Context, cfg config.Config) (*catalog.Catalog, error) {
	return catalog.Load(ctx, cfg.DefinitionsDir, cfg.ProjectionsDir)
}
