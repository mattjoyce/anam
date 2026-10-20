package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"

	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/gate"
	"github.com/mattjoyce/anam/internal/store/ops"
)

// maySubmit reports whether imp may submit facets of type t: one of its types, or
// any with any_type.
func maySubmit(imp config.Principal, t facet.Type) bool {
	return imp.AnyType || lists(imp, t)
}

// mayRegister reports whether imp owns type t, and so may register its
// Definitions. An any_type importer owns nothing (card 177).
func mayRegister(imp config.Principal, t facet.Type) bool {
	return lists(imp, t)
}

// mayStore reports whether imp may post a document's bytes to the library: it lists
// record.document. any_type reaches every facet type and not the library, which takes
// bytes only from a source that owns the references to them (card 177's audit).
func mayStore(imp config.Principal) bool {
	return lists(imp, docType)
}

// lists reports whether t is one of imp's types.
func lists(imp config.Principal, t facet.Type) bool {
	for _, allowed := range imp.Types {
		if allowed == t {
			return true
		}
	}
	return false
}

// scopesOf is the gate's authenticated pillar for imp: its source for its types, or
// for every type with any_type.
func scopesOf(imp config.Principal) gate.TokenScopes {
	if imp.AnyType {
		return gate.OpenScopes(imp.Source)
	}
	return gate.StaticScopes{imp.Source: imp.Types}
}

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
func loadDefinitions(ctx context.Context, db *ops.DB, bundled *def.Registry) (*definitions, error) {
	rows, err := db.RegisteredDefinitions(ctx)
	if err != nil {
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

// guardBundled checks this build's bundled Definitions against the ones the
// operational store last saw (def.Guard), then stores the current set. It runs at
// Open, before the gate exists: a Definition that would re-key or re-place the
// ledger's facets stops the daemon starting, and the error is the remedy.
//
// It stores by upsert, never a sweep: a bundled type the build has dropped stays as a
// provisioned row at its last version. Its facets were admitted under that body and
// the inspector still reads them through it, and an importer that takes the type over
// keeps the version.
func guardBundled(ctx context.Context, db *ops.DB, bundled *def.Registry) error {
	rows, err := db.Definitions(ctx)
	if err != nil {
		return err
	}
	if err := def.Guard(storedOf(rows), bundled); err != nil {
		return err
	}
	current, err := def.Manifests()
	if err != nil {
		return err
	}
	put := make([]ops.DefinitionRow, len(current))
	for i, m := range current {
		put[i] = ops.DefinitionRow{Type: string(m.Type), Version: m.Version, Body: string(m.Body)}
	}
	return db.PutDefinitions(ctx, put)
}

func storedOf(rows []ops.DefinitionRow) []def.Stored {
	out := make([]def.Stored, len(rows))
	for i, r := range rows {
		out[i] = def.Stored{Type: facet.Type(r.Type), Version: r.Version, Body: []byte(r.Body)}
	}
	return out
}
