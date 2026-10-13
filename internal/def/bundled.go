package def

import (
	"embed"
	"fmt"
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

// Bundled parses every embedded manifest into a Registry. It fails on any bad
// manifest rather than loading the rest: a definition set is one contract, and a
// binary that silently dropped a type would reject that type's every envelope with
// a misleading "no definition".
func Bundled() (*Registry, error) {
	entries, err := manifestFS.ReadDir("manifests")
	if err != nil {
		return nil, fmt.Errorf("def: read bundled manifests: %w", err)
	}

	defs := make([]Definition, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue // history: words, not contracts
		}
		raw, err := manifestFS.ReadFile("manifests/" + e.Name())
		if err != nil {
			return nil, fmt.Errorf("def: read bundled manifest %s: %w", e.Name(), err)
		}
		d, err := Parse(raw)
		if err != nil {
			return nil, fmt.Errorf("def: bundled manifest %s: %w", e.Name(), err)
		}
		if want := string(d.Type) + ".json"; e.Name() != want {
			return nil, fmt.Errorf("%w: manifest %s declares type %q (file must be named %s)",
				ErrBadManifest, e.Name(), d.Type, want)
		}
		defs = append(defs, d)
	}
	return NewRegistry(defs...)
}
