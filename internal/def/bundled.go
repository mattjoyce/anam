package def

import (
	"embed"
	"fmt"
	"io/fs"
	"strings"
)

// The bundled manifests: small, static, versioned with the binary (AGENTS.md,
// Embedded assets). A definition is not something Anam knew — it is how Anam
// interprets — so it ships with the interpreter, not in the ledger.
//
// manifests/<type>.json is each type's current Definition, the contract envelopes are
// admitted against. manifests/history/<type>.v<n>.json is every earlier version a
// facet in the ledger may have been admitted under (card 111): provisioned into the
// operational store at start so an old facet still reads against its own words after
// a rebuild, and so an importer registering an older version finds it held. History
// is words, not a contract: it is not parsed as one, and Bundled never loads it.
//
//go:embed manifests/*.json manifests/history/*.json
var manifestFS embed.FS

// embedded returns the embedded catalog root, laid out as a catalog FS: *.json at
// the root, history/*.json for earlier versions.
func embedded() fs.FS {
	root, err := fs.Sub(manifestFS, "manifests")
	if err != nil {
		panic("def: cannot sub manifests: " + err.Error())
	}
	return root
}

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
// earlier versions under history/. Bundled is BundledFS over the embedded catalog.
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
	return NewRegistry(defs...)
}

// EmbeddedFS returns the read-only embedded catalog root: *.json at the root,
// history/*.json for earlier versions. It is the fallback when no external
// definitions directory is configured.
func EmbeddedFS() fs.FS {
	return embedded()
}

// Bundled parses every embedded manifest into a Registry. It fails on any bad
// manifest rather than loading the rest: a definition set is one contract, and a
// binary that silently dropped a type would reject that type's every envelope with
// a misleading "no definition".
func Bundled() (*Registry, error) {
	return BundledFS(embedded())
}
