package daemon

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"

	"github.com/mattjoyce/anam/internal/catalog"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/store/ops"
)

// definitions is the registry importers' envelopes are admitted against: the bundled
// Definitions and the newest registered version of every other type. It is replaced
// whole when a registration lands, so a reader always holds one consistent set.
type definitions struct {
	bundled *def.Registry
	current atomic.Pointer[def.Registry]
}

// loadDefinitions builds the registry from the bundled Definitions and those the
// operational store holds as registered. A registered type this build bundles is
// admitted against the bundled Definition, and said so in the log.
func loadDefinitions(ctx context.Context, db *ops.DB, cat *catalog.Catalog) (*definitions, error) {
	bundled := cat.Definitions()
	rows, err := db.RegisteredDefinitions(ctx)
	if err != nil {
		return nil, err
	}
	if err := checkRegisteredAgainst(cat.Manifests(), rows); err != nil {
		return nil, err
	}
	for _, r := range rows {
		if _, taken := bundled.Lookup(facet.Type(r.Type)); taken {
			slog.Warn("daemon: a registered definition is shadowed by the bundled one",
				"type", r.Type, "version", r.Version)
		}
	}
	reg, err := def.WithRegistered(bundled, storedOf(rows))
	if err != nil {
		return nil, fmt.Errorf("daemon: registered definitions: %w", err)
	}
	d := &definitions{bundled: bundled}
	d.current.Store(reg)
	return d, nil
}

// checkRegisteredAgainst refuses a registered Definition whose body disagrees with the
// catalog's at the same type and version. The catalog's Definition shadows a registered
// type at other versions, which is intended (a release took the type back); at the same
// version two bodies are two answers to one question, and shadowing would hide the
// disagreement. Bodies compare as canonical JSON, so a reformatted file agrees.
func checkRegisteredAgainst(catalogDefs []def.Stored, registered []ops.DefinitionRow) error {
	type key struct {
		typ     facet.Type
		version int
	}
	held := make(map[key][]byte, len(catalogDefs))
	for _, m := range catalogDefs {
		c, err := facet.Canonicalize(m.Body)
		if err != nil {
			return fmt.Errorf("daemon: catalog definition %s v%d: %w", m.Type, m.Version, err)
		}
		held[key{m.Type, m.Version}] = c.Bytes()
	}
	for _, r := range registered {
		want, ok := held[key{facet.Type(r.Type), r.Version}]
		if !ok {
			continue
		}
		c, err := facet.Canonicalize([]byte(r.Body))
		if err != nil {
			return fmt.Errorf("daemon: registered definition %s v%d: %w", r.Type, r.Version, err)
		}
		if !bytes.Equal(want, c.Bytes()) {
			return fmt.Errorf("daemon: %w: %s v%d is registered with a body the catalog contradicts",
				def.ErrBadManifest, r.Type, r.Version)
		}
	}
	return nil
}

// guardCatalog checks the catalog's Definitions against the ones the operational
// store last saw (def.Guard). It runs at Open, before the gate exists and before any
// catalog write: a Definition that would re-key or re-place the ledger's facets stops
// the daemon starting, and the error is the remedy.
func guardCatalog(ctx context.Context, db *ops.DB, cat *catalog.Catalog) error {
	rows, err := db.Definitions(ctx)
	if err != nil {
		return err
	}
	return def.Guard(storedOf(rows), cat.Definitions())
}

// persistCatalog stores the catalog the daemon admitted: its Definition manifests and
// its projection templates, current and history. What is persisted is what was
// guarded and loaded. It runs last, after every check that can refuse the start, so a
// catalog the daemon would not run on is never written.
//
// It stores by upsert, never a sweep: a provisioned type or template the catalog has
// dropped stays as a provisioned row at its last version. Its facets were admitted
// under that body, the inspector still reads them through it, and a receipt that names
// it still verifies. An importer that takes the type over keeps the version. The two
// tables are separate files-worth of state with no transaction across them; a crash
// between the writes leaves the second to the next start, which writes it again.
func persistCatalog(ctx context.Context, db *ops.DB, cat *catalog.Catalog) error {
	current := cat.Manifests()
	put := make([]ops.DefinitionRow, len(current))
	for i, m := range current {
		put[i] = ops.DefinitionRow{Type: string(m.Type), Version: m.Version, Body: string(m.Body)}
	}
	if err := db.PutDefinitions(ctx, put); err != nil {
		return err
	}
	templates := cat.ProjectionManifests()
	rows := make([]ops.ProjectionRow, len(templates))
	for i, t := range templates {
		rows[i] = ops.ProjectionRow{Name: t.Name, Version: t.Version, Body: string(t.Body)}
	}
	return db.PutProjections(ctx, rows)
}

func storedOf(rows []ops.DefinitionRow) []def.Stored {
	out := make([]def.Stored, len(rows))
	for i, r := range rows {
		out[i] = def.Stored{Type: facet.Type(r.Type), Version: r.Version, Body: []byte(r.Body)}
	}
	return out
}
