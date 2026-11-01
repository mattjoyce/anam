package daemon

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/mattjoyce/anam/internal/catalog"
)

func reportCatalogDrift(ctx context.Context, cat *catalog.Catalog) error {
	if cat.DefinitionsSource() == "embedded" && cat.ProjectionsSource() == "embedded" {
		return nil
	}
	embedded, err := catalog.Load(ctx, "", "")
	if err != nil {
		return fmt.Errorf("daemon: load embedded catalog for drift comparison: %w", err)
	}
	d := cat.DriftFrom(embedded)
	if d.None() {
		slog.Info("daemon: catalog matches embedded")
		return nil
	}
	attrs := []any{"set_version_differs", d.SetVersionDiffers}
	for _, field := range []struct {
		name  string
		files []string
	}{
		{"definitions_only_here", d.DefinitionsOnlyHere},
		{"definitions_only_there", d.DefinitionsOnlyThere},
		{"definitions_differ", d.DefinitionsDiffer},
		{"projections_only_here", d.ProjectionsOnlyHere},
		{"projections_only_there", d.ProjectionsOnlyThere},
		{"projections_differ", d.ProjectionsDiffer},
	} {
		// Presentation bound keeps one startup line readable for a replaced catalog.
		shown := min(len(field.files), 20)
		attrs = append(attrs, field.name+"_count", len(field.files),
			field.name, field.files[:shown], field.name+"_more", len(field.files)-shown)
	}
	slog.Warn("daemon: catalog drifts from embedded", attrs...)
	return nil
}
