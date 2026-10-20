package api

import "encoding/json"

// EntryRequest is the body of POST /v1/entry: one fact entered by hand in the
// Workbench (card 188). It names no source, injector or id: the daemon submits as the
// workbench principal, and the gate fills the identity field from the Definition's
// identity_template, or refuses a payload that lacks it.
type EntryRequest struct {
	Type string `json:"type"`
	// DefinitionVersion is the version the form was drawn from. It must be the type's
	// version in force; a form drawn from another is refused (409).
	DefinitionVersion int             `json:"definition_version"`
	Payload           json.RawMessage `json:"payload"`
}

// Entry is the answer to an entry the gate took. Outcome is admitted (a new facet),
// superseded (a new version of a facet already held) or no-op (the same facet, with
// the same content, is already held under FacetID). Seq is the anam-event the entry
// wrote, absent for a no-op; W is the watermark after it.
type Entry struct {
	Outcome         string `json:"outcome"`
	FacetID         string `json:"facet_id"`
	ArtifactVersion int    `json:"artifact_version"`
	Seq             int64  `json:"seq,omitempty"`
	W               int64  `json:"w"`
}
