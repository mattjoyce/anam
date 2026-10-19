package api

import (
	"github.com/mattjoyce/anam/internal/query"
	"github.com/mattjoyce/anam/internal/receipt"
)

// QueryRequest is a Datalog query as the reader asks it (card 173): the projection
// it runs through, the program, the relation to answer, which knowledge to stand on,
// and whether to file the receipt. It travels as a JSON body.
type QueryRequest struct {
	// Projection names the template the query runs through; required, since the
	// projection decides which whole facets the query may see.
	Projection string `json:"projection"`
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
