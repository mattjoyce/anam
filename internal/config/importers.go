package config

import (
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"

	"github.com/mattjoyce/anam/internal/facet"
)

// ErrInvalidImporters is returned for an importers file that cannot be used.
var ErrInvalidImporters = errors.New("invalid importers file")

// Importer is one importer the daemon accepts envelopes and Definitions from: the
// source it submits as, the SHA-256 of its token, and the facet types it may submit.
// The daemon holds only the hash, so the file can be read by whoever runs it and
// gives nobody a token.
type Importer struct {
	// Source is the source every envelope it submits is admitted as. A facet's
	// identity includes its source, so an importer that takes over a built-in
	// worker's facets submits as that worker's source.
	Source string `mapstructure:"source"`

	// TokenSHA256 is the hex SHA-256 of the token, without a trailing newline:
	// printf %s "$token" | sha256sum.
	TokenSHA256 string `mapstructure:"token_sha256"`

	// Types are the facet types it may submit and register Definitions for.
	Types []facet.Type `mapstructure:"types"`
}

type importersFile struct {
	Importers []Importer `mapstructure:"importers"`
}

// LoadImporters reads and validates an importers file. Unknown keys are errors, as
// in Load. A source or a token named twice is refused: either would make one
// importer's envelopes indistinguishable from another's. So is a type named by two
// importers: a type's Definitions come from one place, and a second importer could
// register a version that refuses every envelope of the first. A type bundled
// reports true for is the exception: its Definitions come from the binary, no
// importer can register one, so any number of importers may submit it. A nil
// bundled shares nothing.
func LoadImporters(path string, bundled func(facet.Type) bool) ([]Importer, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read importers %q: %w", path, err)
	}
	var f importersFile
	errorOnUnknownKeys := func(dc *mapstructure.DecoderConfig) { dc.ErrorUnused = true }
	if err := v.Unmarshal(&f, errorOnUnknownKeys); err != nil {
		return nil, fmt.Errorf("decode importers %q: %w: %v", path, ErrInvalidImporters, err)
	}

	sources, hashes, owners := map[string]bool{}, map[string]bool{}, map[facet.Type]string{}
	for i, imp := range f.Importers {
		bad := func(format string, a ...any) error {
			return fmt.Errorf("%w: %s: importer %d (%s): %s", ErrInvalidImporters, path, i+1, imp.Source,
				fmt.Sprintf(format, a...))
		}
		switch b, err := hex.DecodeString(imp.TokenSHA256); {
		case imp.Source == "":
			return nil, bad("no source")
		case err != nil || len(b) != 32 || imp.TokenSHA256 != hex.EncodeToString(b):
			return nil, bad("token_sha256 is not 64 lower-case hex digits")
		case sources[imp.Source]:
			return nil, bad("source named twice")
		case hashes[imp.TokenSHA256]:
			return nil, bad("token shared with another importer")
		case len(imp.Types) == 0:
			return nil, bad("no types")
		}
		for _, t := range imp.Types {
			if !t.Valid() {
				return nil, bad("type %q is not a valid namespaced facet type", t)
			}
			if bundled != nil && bundled(t) {
				continue
			}
			if owner, taken := owners[t]; taken {
				return nil, bad("type %s is %s's", t, owner)
			}
			owners[t] = imp.Source
		}
		sources[imp.Source], hashes[imp.TokenSHA256] = true, true
	}
	return f.Importers, nil
}
