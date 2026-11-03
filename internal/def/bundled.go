package def

import (
	"fmt"
	"io/fs"
	"strings"
)

// A catalog root holds each type's current Definition as <type>.json, the contract
// envelopes are admitted against, and every earlier version a facet in the ledger may
// have been admitted under as history/<type>.v<n>.json (card 111): provisioned into
// the operational store at start so an old facet still reads against its own words
// after a rebuild, and so an importer registering an older version finds it held.
// History is words, not a contract: it is never parsed as a current Definition.
// BundledFS reads it for one thing only, which retired types close their series
// (Closes). The binary carries no catalog (card 255); the deployment supplies one.

// readJSONFiles lists regular *.json files from an fs.FS root. Any
// non-regular-file entry (directories, .DS_Store, README.md) is ignored.
func readJSONFiles(fsys fs.FS, dir string) ([]string, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.Type().IsRegular() {
			continue
		}
		if !isJSON(e.Name()) {
			continue
		}
		names = append(names, e.Name())
	}
	return names, nil
}

// isJSON reports whether name ends in .json.
func isJSON(name string) bool {
	return strings.HasSuffix(name, ".json")
}

// BundledFS parses every current manifest in a catalog root: *.json at the root,
// earlier versions under history/. The registry's retired types, which Closes answers
// for a type no current manifest names, come from that root's history/ and no other.
// It fails on any bad manifest rather than loading the rest: a definition set is one
// contract, and a registry that silently dropped a type would reject that type's
// every envelope with a misleading "no definition".
func BundledFS(fsys fs.FS) (*Registry, error) {
	names, err := readJSONFiles(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("def: read bundled manifests: %w", err)
	}

	defs := make([]Definition, 0, len(names))
	for _, name := range names {
		raw, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, fmt.Errorf("def: read bundled manifest %s: %w", name, err)
		}
		d, err := Parse(raw)
		if err != nil {
			return nil, fmt.Errorf("def: bundled manifest %s: %w", name, err)
		}
		if want := string(d.Type) + ".json"; name != want {
			return nil, fmt.Errorf("%w: manifest %s declares type %q (file must be named %s)",
				ErrBadManifest, name, d.Type, want)
		}
		defs = append(defs, d)
	}
	history, err := HistoryFS(fsys)
	if err != nil {
		return nil, err
	}
	retired, err := retiredCloses(history)
	if err != nil {
		return nil, err
	}
	return newRegistry(retired, defs...)
}
