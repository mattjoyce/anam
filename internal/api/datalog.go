package api

import (
	"bytes"
	"encoding/json"
	"errors"

	"github.com/mattjoyce/anam/internal/query"
	"github.com/mattjoyce/anam/internal/receipt"
)

// QueryRequest is a Datalog query as the reader asks it (card 173): the projection
// it runs through, the program, the relation to answer, which knowledge to stand on,
// and whether to file the receipt. It travels as a JSON body.
type QueryRequest struct {
	// Projection is the template the query runs through; required, since the
	// projection decides which whole facets the query may see. In JSON it is a name,
	// or an inline template object (card 211).
	Projection ProjectionRef `json:"projection"`
	// Text is the program, Anam's Datalog, built on the bundled rules.
	Text string `json:"text"`
	// Relation is the derived relation answered.
	Relation string `json:"relation"`
	// AsOf and W choose what Anam knew, as a timeline read's do; at most one.
	AsOf string `json:"as_of,omitempty"`
	W    int64  `json:"w,omitempty"`
	// DryRun derives the answer and its receipt and files nothing.
	DryRun bool `json:"dry_run,omitempty"`
	// Zone is the reader's zone, for a bare date in as_of.
	Zone string `json:"zone,omitempty"`
}

// ProjectionRef is how a query says which projection it reads through: the name of a
// registered or bundled one, or, for a one-off question, the template itself (card
// 211). The JSON is a string for the first and an object for the second, with the
// fields a manifest has but its name, version and description: types, statuses,
// max_classification, from, to and shape.
type ProjectionRef struct {
	// Name is the registered or bundled projection's name.
	Name string
	// Inline is the template object as sent, which the projection loader checks. Set
	// only when Name is empty.
	Inline json.RawMessage
}

// Named is a reference to the projection called name.
func Named(name string) ProjectionRef { return ProjectionRef{Name: name} }

// IsZero reports whether r names no projection.
func (r ProjectionRef) IsZero() bool { return r.Name == "" && len(r.Inline) == 0 }

// MarshalJSON writes a name as a string and an inline template as its object, and
// nothing as null.
func (r ProjectionRef) MarshalJSON() ([]byte, error) {
	switch {
	case r.Name != "" && len(r.Inline) > 0:
		return nil, errors.New("a projection is named or inline, not both")
	case len(r.Inline) > 0:
		return r.Inline, nil
	}
	return json.Marshal(r.Name)
}

// UnmarshalJSON reads a name from a string and an inline template from an object. Any
// other JSON is the reader's error, naming what it should have been.
func (r *ProjectionRef) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	switch {
	case bytes.Equal(b, []byte("null")):
		*r = ProjectionRef{}
		return nil
	case len(b) > 0 && b[0] == '"':
		var name string
		if err := json.Unmarshal(b, &name); err != nil {
			return err
		}
		*r = ProjectionRef{Name: name}
		return nil
	case len(b) > 0 && b[0] == '{':
		*r = ProjectionRef{Inline: append(json.RawMessage(nil), b...)}
		return nil
	}
	return errors.New("projection is a name, or an object holding the template's types, statuses, max_classification, from, to and shape")
}

// RuleSetRef names the bundled rule set an answer stood on.
type RuleSetRef struct {
	Name    string `json:"name"`
	Version int    `json:"version"`
	Hash    string `json:"hash"`
}

// QueryAnswer is a query's answer with its receipt: the knowledge it stood on, the
// projection and the rules it went through, the relation's tuples and their hash,
// warnings worth reading, and the receipt that pins it all.
type QueryAnswer struct {
	W         int64  `json:"w"`
	LedgerID  string `json:"ledger_id"`
	AsOf      string `json:"as_of,omitempty"`
	Live      int    `json:"live"`
	Canonical string `json:"canonical"`

	Projection           ProjectionRead `json:"projection"`
	DefinitionSetVersion string         `json:"definition_set_version"`
	Rules                RuleSetRef     `json:"rules"`

	Answer   query.Answer `json:"answer"`
	Hash     string       `json:"hash"`
	Warnings []string     `json:"warnings,omitempty"`

	ReceiptHash string          `json:"receipt_hash"`
	Receipt     receipt.Receipt `json:"receipt"`
	// Written is false for a dry run: the receipt was derived and not filed.
	Written bool `json:"written"`
}

// QueryCatalogue is the introspection a query is written against (GET /v1/query).
type QueryCatalogue = query.Catalogue
