// Package catalog builds a validated, frozen snapshot of one catalog: the
// provisioned definitions registry, definition bodies including history, projection
// registry and bodies including history, definition set version, and the two source
// labels. It contains no store handles, configuration resolver, environment reader,
// or mutable global selector.
//
// Build it once per daemon startup or local command invocation, after configuration
// resolution and before opening writable stores. Pass that same value down.
// Registered overlays remain separate runtime state; they do not change the
// provisioned catalog's set version.
package catalog

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"sort"

	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/projection"
)

// ErrInvalid is returned by Load for any catalog validation failure.
var ErrInvalid = errors.New("catalog: invalid catalog")

// Catalog is a fully loaded, validated catalog snapshot. Obtain one through Load.
// Accessors return prebuilt values for read-only use under their existing API
// contracts. There are no mutation methods.
type Catalog struct {
	definitions         *def.Registry
	projections         *projection.Registry
	manifests           []def.Stored
	history             []def.Stored
	projectionManifests []projection.Stored
	setVersion          string
	definitionsSource   string
	projectionsSource   string
}

// Definitions returns the prebuilt definition registry.
func (c *Catalog) Definitions() *def.Registry { return c.definitions }

// Projections returns the prebuilt projection registry.
func (c *Catalog) Projections() *projection.Registry { return c.projections }

// Manifests returns every current and historical definition Stored, preserving
// def.ManifestsFS order. Returns fresh slices; body bytes are copied.
func (c *Catalog) Manifests() []def.Stored {
	out := make([]def.Stored, len(c.manifests))
	for i, s := range c.manifests {
		out[i] = def.Stored{Type: s.Type, Version: s.Version, Body: append([]byte(nil), s.Body...)}
	}
	return out
}

// History returns history-only definition Stored values. Returns fresh slices.
func (c *Catalog) History() []def.Stored {
	out := make([]def.Stored, len(c.history))
	for i, s := range c.history {
		out[i] = def.Stored{Type: s.Type, Version: s.Version, Body: append([]byte(nil), s.Body...)}
	}
	return out
}

// ProjectionManifests returns original current and historical projection bodies
// in current-filename order followed by historical-filename order.
func (c *Catalog) ProjectionManifests() []projection.Stored {
	out := make([]projection.Stored, len(c.projectionManifests))
	for i, s := range c.projectionManifests {
		out[i] = projection.Stored{Name: s.Name, Version: s.Version, Body: append([]byte(nil), s.Body...)}
	}
	return out
}

// SetVersion returns the precomputed definition set version.
func (c *Catalog) SetVersion() string { return c.setVersion }

// DefinitionsSource returns the source label for definitions:
// "embedded" or the resolved directory path.
func (c *Catalog) DefinitionsSource() string { return c.definitionsSource }

// ProjectionsSource returns the source label for projections:
// "embedded" or the resolved directory path.
func (c *Catalog) ProjectionsSource() string { return c.projectionsSource }

// CheckRegistered refuses a registered Definition whose body disagrees with the
// catalog's at the same type and version, with def.ErrBadManifest. The catalog's
// Definition shadows a registered type at other versions, which is intended (a release
// took the type back); at the same version two bodies are two answers to one question,
// and the shadowing would hide the disagreement. Bodies compare as canonical JSON, so a
// reformatted body agrees. WithRegistered runs it, so every registry built over a
// store's registered rows refuses the same store.
func (c *Catalog) CheckRegistered(registered []def.Stored) error {
	held := make(map[string][]byte, len(c.manifests))
	for _, m := range c.manifests {
		canon, err := facet.Canonicalize(m.Body)
		if err != nil {
			return fmt.Errorf("%w: catalog definition %s v%d: %w", def.ErrBadManifest, m.Type, m.Version, err)
		}
		held[fmt.Sprintf("%s v%d", m.Type, m.Version)] = canon.Bytes()
	}
	for _, r := range registered {
		key := fmt.Sprintf("%s v%d", r.Type, r.Version)
		want, ok := held[key]
		if !ok {
			continue
		}
		canon, err := facet.Canonicalize(r.Body)
		if err != nil {
			return fmt.Errorf("%w: registered definition %s: %w", def.ErrBadManifest, key, err)
		}
		if !bytes.Equal(want, canon.Bytes()) {
			return fmt.Errorf("%w: %s is registered with a body the catalog contradicts", def.ErrBadManifest, key)
		}
	}
	return nil
}

// WithRegistered is the catalog's Definitions with the registered ones beside them
// (def.WithRegistered), after CheckRegistered: the registry a store's facets were
// admitted against. The daemon at start, a local command and a migration all build it
// here, so none of them can let the catalog's shadow hide a contradicting row.
func (c *Catalog) WithRegistered(registered []def.Stored) (*def.Registry, error) {
	if err := c.CheckRegistered(registered); err != nil {
		return nil, err
	}
	return def.WithRegistered(c.definitions, registered)
}

// Load builds and validates a catalog. An empty directory selects the embedded
// catalog for that component, labelled "embedded"; a nonempty one is a resolved
// directory path, which must exist and is used as its label. Load does not expand
// paths, read configuration, inspect the environment, or log, and it writes nothing.
//
// Each component is read once into memory and every parser, and the set version,
// runs over that snapshot. Load returns a catalog only when both components and the
// duplicate check pass; every failure returns nil and an error wrapping ErrInvalid
// together with the underlying cause.
func Load(ctx context.Context, definitionsDir, projectionsDir string) (*Catalog, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalid, err)
	}

	defSrc, defLabel, err := source(definitionsDir, def.EmbeddedFS)
	if err != nil {
		return nil, fmt.Errorf("%w: definitions_dir: %s: %w", ErrInvalid, defLabel, err)
	}
	defSnap, err := takeSnapshot(ctx, defSrc)
	if err != nil {
		return nil, fmt.Errorf("%w: definitions_dir: %s: %w", ErrInvalid, defLabel, err)
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalid, err)
	}

	projSrc, projLabel, err := source(projectionsDir, projection.EmbeddedFS)
	if err != nil {
		return nil, fmt.Errorf("%w: projections_dir: %s: %w", ErrInvalid, projLabel, err)
	}
	projSnap, err := takeSnapshot(ctx, projSrc)
	if err != nil {
		return nil, fmt.Errorf("%w: projections_dir: %s: %w", ErrInvalid, projLabel, err)
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalid, err)
	}

	c := &Catalog{definitionsSource: defLabel, projectionsSource: projLabel}
	if err := c.loadDefinitions(defSnap); err != nil {
		return nil, fmt.Errorf("%w: definitions_dir: %s: %w", ErrInvalid, defLabel, err)
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	if err := c.loadProjections(projSnap); err != nil {
		return nil, fmt.Errorf("%w: projections_dir: %s: %w", ErrInvalid, projLabel, err)
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	return c, nil
}

// source selects a component's root and its label.
func source(dir string, embedded func() fs.FS) (fs.FS, string, error) {
	if dir == "" {
		return embedded(), "embedded", nil
	}
	root, err := dirRoot(dir)
	return root, dir, err
}

// loadDefinitions parses the definition snapshot with the def package's own readers
// and checks that no type and version is given two different bodies.
func (c *Catalog) loadDefinitions(snap *snapshot) error {
	reg, err := def.BundledFS(snap)
	if err != nil {
		return err
	}
	manifests, err := def.ManifestsFS(snap)
	if err != nil {
		return err
	}
	history, err := def.HistoryFS(snap)
	if err != nil {
		return err
	}
	setVersion, err := def.SetVersionFS(snap)
	if err != nil {
		return err
	}
	if err := checkDuplicateDefs(manifests[:len(manifests)-len(history)], history); err != nil {
		return err
	}
	c.definitions, c.manifests, c.history, c.setVersion = reg, manifests, history, setVersion
	return nil
}

// loadProjections parses the projection snapshot with the projection package's own
// reader, which already refuses a history version that is not earlier than its
// current template, and keeps each original body with its name and version.
func (c *Catalog) loadProjections(snap *snapshot) error {
	reg, err := projection.BundledFS(snap)
	if err != nil {
		return err
	}
	stored := make([]projection.Stored, 0, len(snap.root)+len(snap.history))
	for _, files := range []map[string][]byte{snap.root, snap.history} {
		for _, name := range sortedNames(files) {
			t, err := projection.Parse(files[name])
			if err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
			stored = append(stored, projection.Stored{Name: t.Name, Version: t.Version, Body: files[name]})
		}
	}
	c.projections, c.projectionManifests = reg, stored
	return nil
}

// checkDuplicateDefs refuses a type and version given two bodies that differ once
// canonicalized; equal canonical bodies agree. History file names are unique per
// type and version, so a conflict pairs a current manifest with a history entry.
func checkDuplicateDefs(current, history []def.Stored) error {
	type entry struct {
		body []byte
		file string
	}
	seen := make(map[string]entry, len(current)+len(history))
	add := func(s def.Stored, file string) error {
		key := fmt.Sprintf("%s v%d", s.Type, s.Version)
		canon, err := facet.Canonicalize(s.Body)
		if err != nil {
			return fmt.Errorf("%w: %s: %w", def.ErrBadManifest, file, err)
		}
		if prev, ok := seen[key]; ok {
			if !bytes.Equal(prev.body, canon.Bytes()) {
				return fmt.Errorf("%w: %s is given two different bodies, by %s and %s",
					def.ErrBadManifest, key, prev.file, file)
			}
			return nil
		}
		seen[key] = entry{body: canon.Bytes(), file: file}
		return nil
	}
	for _, s := range current {
		if err := add(s, string(s.Type)+".json"); err != nil {
			return err
		}
	}
	for _, s := range history {
		if err := add(s, fmt.Sprintf("history/%s.v%d.json", s.Type, s.Version)); err != nil {
			return err
		}
	}
	return nil
}

func sortedNames(files map[string][]byte) []string {
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
