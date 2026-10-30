package app

import (
	"context"

	"github.com/mattjoyce/anam/internal/catalog"
	"github.com/mattjoyce/anam/internal/config"
)

// loadCatalog loads the provisioned catalog a local command reads, once per
// invocation: the Definitions and projections every registry it builds starts from,
// and the set version its receipts name. A command that asks a daemon does not call
// it; the daemon loaded its own.
func loadCatalog(ctx context.Context, _ config.Config) (*catalog.Catalog, error) {
	return catalog.Load(ctx, "", "")
}
