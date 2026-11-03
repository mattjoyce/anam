package daemon

import (
	"log/slog"

	"github.com/mattjoyce/anam/internal/catalog"
	"github.com/mattjoyce/anam/internal/catchup"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/facet"
)

// structuralEffects says what stops working without each structural type. Every
// consumer answers the absence itself: the gate refuses an envelope with no
// Definition, the link form answers 404, the edge index derives nothing, and no
// document reference is checked.
var structuralEffects = map[facet.Type]string{
	catchup.EdgeType: "links cannot be entered or indexed",
	docType:          "documents cannot be referenced or served",
}

// logCatalogGaps writes the lines after "catalog admitted" that say what the catalog
// lacks (card 255, R4b): a WARN when neither catalog directory is configured, or one
// naming the directory left unset when only one is, since an empty catalog or
// component would otherwise pass for a healthy start, then one line per
// structural type the catalog holds no Definition for. None of them refuses a start.
// The test is on the configured directories rather than the source labels, so a
// directory that happens to be named "none" is not taken for an unset one.
func logCatalogGaps(cfg config.Config, cat *catalog.Catalog) {
	switch {
	case cfg.DefinitionsDir == "" && cfg.ProjectionsDir == "":
		slog.Warn("daemon: no catalog configured; set definitions_dir and projections_dir, or register definitions over the API")
	case cfg.DefinitionsDir == "":
		slog.Warn("daemon: catalog component not configured; it is empty", "unset", "definitions_dir",
			"effect", "no catalog Definitions; only registered ones load")
	case cfg.ProjectionsDir == "":
		slog.Warn("daemon: catalog component not configured; it is empty", "unset", "projections_dir",
			"effect", "no catalog projection templates; only registered ones load")
	}
	for _, t := range cat.AbsentStructural() {
		slog.Info("daemon: structural type not in the catalog", "type", string(t), "effect", structuralEffects[t])
	}
}
