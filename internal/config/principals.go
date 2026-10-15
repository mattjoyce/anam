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

// Role says what a principal's token opens. It is data in the importers file, and each
// route names the roles it admits; no code path switches on who a caller is.
type Role string

// The roles. They are an invariant, not a default: each is a set of routes the daemon
// is built with, and a role this build does not know would open nothing it can name.
const (
	// RoleReader reads the record and publishes reads, which write artifacts only.
	RoleReader Role = "reader"
	// RoleImporter reads as a reader does, and submits envelopes, documents and
	// Definitions as its one source, for its types only.
	RoleImporter Role = "importer"
	// RoleOperator runs the daemon: every read, minting, backups, projection
	// templates and the inspector. It submits nothing, since it is no source.
	RoleOperator Role = "operator"
)

// OperatorName is the name of the principal whose token is in token_file. It is
// reserved, so a log or a refusal that names the operator means that token.
const OperatorName = "operator"

// Principal is one client the daemon admits: its name, its role, the SHA-256 of its
// token, and for an importer the source it submits as and the facet types it may
// submit. The daemon holds only the hash, so the file can be read by whoever runs it
// and gives nobody a token.
type Principal struct {
	// Name says who the principal is, in a refusal or a log line, without naming
	// its token. Under importers it defaults to the source.
	Name string `mapstructure:"name"`

	// Role is what the token opens. Under importers it is importer.
	Role Role `mapstructure:"role"`

	// Source is the source every envelope an importer submits is admitted as. A
	// facet's identity includes its source, so an importer that takes over a
	// built-in worker's facets submits as that worker's source. Only an importer
	// has one.
	Source string `mapstructure:"source"`

	// TokenSHA256 is the hex SHA-256 of the token, without a trailing newline:
	// printf %s "$token" | sha256sum.
	TokenSHA256 string `mapstructure:"token_sha256"`

	// Types are the facet types an importer may submit and register Definitions
	// for. Only an importer has them.
	Types []facet.Type `mapstructure:"types"`
}

// importersFile holds principals under two keys. importers is the file's first shape,
// importers alone, and stays so a deployed file reads as it did; principals names
// each role.
type importersFile struct {
	Principals []Principal `mapstructure:"principals"`
	Importers  []Principal `mapstructure:"importers"`
}

// LoadPrincipals reads and validates an importers file. Unknown keys are errors, as
// in Load. A name, a source or a token used twice is refused: any would make one
// principal's requests indistinguishable from another's. So is a type named by two
// importers: a type's Definitions come from one place, and a second importer could
// register a version that refuses every envelope of the first. A type bundled
// reports true for is the exception: its Definitions come from the binary, no
// importer can register one, so any number of importers may submit it. A nil
// bundled shares nothing.
func LoadPrincipals(path string, bundled func(facet.Type) bool) ([]Principal, error) {
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
	for i, p := range f.Importers {
		if p.Role != "" && p.Role != RoleImporter {
			return nil, fmt.Errorf("%w: %s: importer %d (%s): role %s belongs under principals",
				ErrInvalidImporters, path, i+1, p.Source, p.Role)
		}
		f.Importers[i].Role = RoleImporter
		if p.Name == "" {
			f.Importers[i].Name = p.Source
		}
	}
	all := make([]Principal, 0, len(f.Principals)+len(f.Importers))
	all = append(append(all, f.Principals...), f.Importers...)
	if err := validPrincipals(all, bundled); err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrInvalidImporters, path, err)
	}
	return all, nil
}

// validPrincipals checks each principal alone, then what the list must not share.
func validPrincipals(all []Principal, bundled func(facet.Type) bool) error {
	names, sources, hashes := map[string]bool{}, map[string]bool{}, map[string]bool{}
	owners := map[facet.Type]string{}
	for _, p := range all {
		bad := func(format string, a ...any) error {
			return fmt.Errorf("%s: %s", p.Name, fmt.Sprintf(format, a...))
		}
		if err := validPrincipal(p); err != nil {
			return bad("%v", err)
		}
		switch {
		case names[p.Name]:
			return bad("name used twice")
		case sources[p.Source]:
			return bad("source named twice")
		case hashes[p.TokenSHA256]:
			return bad("token shared with another principal")
		}
		for _, t := range p.Types {
			if bundled != nil && bundled(t) {
				continue
			}
			if owner, taken := owners[t]; taken {
				return bad("type %s is %s's", t, owner)
			}
			owners[t] = p.Source
		}
		names[p.Name], hashes[p.TokenSHA256] = true, true
		if p.Source != "" {
			sources[p.Source] = true
		}
	}
	return nil
}

// validPrincipal checks what one principal must hold: a name, a role, a token's hash,
// and a source and types exactly when it is an importer.
func validPrincipal(p Principal) error {
	switch b, err := hex.DecodeString(p.TokenSHA256); {
	case p.Name == "":
		return errors.New("no name")
	case p.Name == OperatorName:
		return fmt.Errorf("name %s is the token_file's", OperatorName)
	case p.Role != RoleReader && p.Role != RoleImporter && p.Role != RoleOperator:
		return fmt.Errorf("role %q is not reader, importer or operator", p.Role)
	case err != nil || len(b) != 32 || p.TokenSHA256 != hex.EncodeToString(b):
		return errors.New("token_sha256 is not 64 lower-case hex digits")
	case p.Role != RoleImporter && (p.Source != "" || len(p.Types) > 0):
		return fmt.Errorf("a %s has no source and no types: only an importer submits", p.Role)
	case p.Role == RoleImporter && p.Source == "":
		return errors.New("no source")
	case p.Role == RoleImporter && len(p.Types) == 0:
		return errors.New("no types")
	}
	for _, t := range p.Types {
		if !t.Valid() {
			return fmt.Errorf("type %q is not a valid namespaced facet type", t)
		}
	}
	return nil
}
