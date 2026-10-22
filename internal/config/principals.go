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
	// Definitions as its one source, for its types only, or with any_type any
	// type and no Definition.
	RoleImporter Role = "importer"
	// RoleOperator runs the daemon: every read, minting, backups, projection
	// templates and the inspector. It submits nothing, since it is no source.
	RoleOperator Role = "operator"
)

// Receipts says whether a principal's reads each leave a receipt. It is a choice
// someone running Anam makes per principal, so it is data in the importers file beside
// the role, and off unless the file turns it on.
type Receipts string

// The receipt settings. Off is the empty string, so a principal that says nothing
// keeps today's behaviour. They are an invariant, not a default: a value this build
// does not know would silently leave a reader unrecorded, so Load refuses it.
const (
	// ReceiptsOff is the default: a read leaves a receipt only when the reader files
	// one, by POST /v1/receipts or a query that is not a dry run.
	ReceiptsOff Receipts = ""
	// ReceiptsAlways files a receipt, naming the principal, for every read the
	// principal makes that serves facets (card 216).
	ReceiptsAlways Receipts = "always"
)

// OperatorName is the name of the principal whose token is in token_file. It is
// reserved, so a log or a refusal that names the operator means that token.
const OperatorName = "operator"

// WorkbenchSource is the source of facts entered by hand in the Workbench (card 188).
// It is reserved, and an invariant: a fact's identity names its source, so changing
// it would orphan every entry already made. The importers file may declare a
// principal with this source to say which types the Workbench may enter; that
// principal has no token, since the daemon holds it itself and no client authenticates
// as it.
const WorkbenchSource = "workbench"

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
	// printf %s "$token" | sha256sum. The workbench principal has none, and a hash on
	// it is an error.
	TokenSHA256 string `mapstructure:"token_sha256"`

	// Types are the facet types an importer may submit and register Definitions
	// for: the types it owns. Only an importer has them. For the workbench they are
	// the allow-list of what may be entered by hand, and it owns none of them: it
	// registers nothing, so another importer may own a type the workbench lists.
	Types []facet.Type `mapstructure:"types"`

	// AnyType lets an importer submit facets of every type the daemon knows, as
	// its source, while owning none: it registers no Definition. It is for a
	// source that is the person speaking (card 177), whose facets are kept apart
	// from every other source's by the source in their identity. An importer has
	// Types or AnyType, not both.
	AnyType bool `mapstructure:"any_type"`

	// Receipts is "always" for a principal whose every read is filed as a receipt
	// naming it, or empty (or "off") for one whose reads are not. Only a reader and
	// an importer have it: the operator reads the record unprojected, which a
	// receipt cannot describe, and the workbench principal has no token to read with.
	Receipts Receipts `mapstructure:"receipts"`
}

// Records reports whether every read p makes is filed as a receipt.
func (p Principal) Records() bool { return p.Receipts == ReceiptsAlways }

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
	for i := range all {
		if all[i].Receipts == "off" {
			all[i].Receipts = ReceiptsOff
		}
	}
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
		case p.TokenSHA256 != "" && hashes[p.TokenSHA256]:
			return bad("token shared with another principal")
		}
		for _, t := range ownedTypes(p) {
			if bundled != nil && bundled(t) {
				continue
			}
			if owner, taken := owners[t]; taken {
				return bad("type %s is %s's", t, owner)
			}
			owners[t] = p.Source
		}
		names[p.Name] = true
		if p.TokenSHA256 != "" {
			hashes[p.TokenSHA256] = true
		}
		if p.Source != "" {
			sources[p.Source] = true
		}
	}
	return nil
}

// validPrincipal checks what one principal must hold: a name, a role, a token's hash,
// and a source and types (or any_type) exactly when it is an importer. The workbench
// is the exception on the hash: it has none, and a types list that names what may be
// entered by hand.
func validPrincipal(p Principal) error {
	if p.Source == WorkbenchSource {
		return validWorkbench(p)
	}
	switch b, err := hex.DecodeString(p.TokenSHA256); {
	case p.Name == "":
		return errors.New("no name")
	case p.Name == OperatorName:
		return fmt.Errorf("name %s is the token_file's", OperatorName)
	case p.Role != RoleReader && p.Role != RoleImporter && p.Role != RoleOperator:
		return fmt.Errorf("role %q is not reader, importer or operator", p.Role)
	case err != nil || len(b) != 32 || p.TokenSHA256 != hex.EncodeToString(b):
		return errors.New("token_sha256 is not 64 lower-case hex digits")
	case p.Role != RoleImporter && (p.Source != "" || len(p.Types) > 0 || p.AnyType):
		return fmt.Errorf("a %s has no source and no types: only an importer submits", p.Role)
	case p.Role == RoleImporter && p.Source == "":
		return errors.New("no source")
	case p.Role == RoleImporter && len(p.Types) == 0 && !p.AnyType:
		return errors.New("no types")
	case p.AnyType && len(p.Types) > 0:
		return errors.New("any_type with types: list the types it owns, or any_type, not both")
	case p.Receipts != ReceiptsOff && p.Receipts != ReceiptsAlways:
		return fmt.Errorf("receipts %q is not always or off", p.Receipts)
	case p.Receipts != ReceiptsOff && p.Role == RoleOperator:
		return errors.New("receipts: an operator reads the whole record unprojected, which no receipt describes")
	}
	for _, t := range p.Types {
		if !t.Valid() {
			return fmt.Errorf("type %q is not a valid namespaced facet type", t)
		}
	}
	return nil
}

// ownedTypes are the types p owns: its types, except the workbench's, which are an
// allow-list and not an ownership, so another importer may own the same type.
func ownedTypes(p Principal) []facet.Type {
	if p.Source == WorkbenchSource {
		return nil
	}
	return p.Types
}

// validWorkbench checks the workbench principal: an importer with the reserved source,
// no token, and an explicit list of the types that may be entered by hand. any_type is
// refused, since the list is the allow-list and names types one by one.
func validWorkbench(p Principal) error {
	switch {
	case p.Name == "" || p.Name == OperatorName:
		return fmt.Errorf("source %s needs a name other than %q", WorkbenchSource, OperatorName)
	case p.Role != RoleImporter:
		return fmt.Errorf("source %s is an importer's, not a %s's", WorkbenchSource, p.Role)
	case p.TokenSHA256 != "":
		return fmt.Errorf("source %s has no token_sha256: the daemon holds this principal itself", WorkbenchSource)
	case p.AnyType:
		return fmt.Errorf("source %s lists the types it may enter, and takes no any_type", WorkbenchSource)
	case p.Receipts != ReceiptsOff:
		return fmt.Errorf("source %s has no token to read with, so receipts do not apply", WorkbenchSource)
	case len(p.Types) == 0:
		return errors.New("no types")
	}
	for _, t := range p.Types {
		if !t.Valid() {
			return fmt.Errorf("type %q is not a valid namespaced facet type", t)
		}
	}
	return nil
}
