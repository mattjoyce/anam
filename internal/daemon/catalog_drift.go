package daemon

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/mattjoyce/anam/internal/catalog"
)

// catalogDrift compares the admitted catalog with the embedded one. It runs before any
// store opens, so a failure to load the embedded catalog refuses the start with the
// store untouched. It returns nil when both components are embedded: there is nothing
// to compare and nothing to report. The test is on the configured directories rather
// than on the source labels, so a directory that happens to be named "embedded" is
// still compared.
func catalogDrift(ctx context.Context, definitionsDir, projectionsDir string, cat *catalog.Catalog) (*catalog.Drift, error) {
	if definitionsDir == "" && projectionsDir == "" {
		return nil, nil
	}
	embedded, err := catalog.Load(ctx, "", "")
	if err != nil {
		return nil, fmt.Errorf("daemon: load embedded catalog for drift comparison: %w", err)
	}
	d := cat.DriftFrom(embedded)
	return &d, nil
}

// logCatalogDrift writes the one drift line after "catalog admitted". Drift never
// refuses a start.
func logCatalogDrift(d *catalog.Drift) {
	if d == nil {
		return
	}
	if d.None() {
		slog.Info("daemon: catalog matches embedded")
		return
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
}
