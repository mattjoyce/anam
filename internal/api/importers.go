package api

import "encoding/json"

// Submission is what an importer posts to /v1/envelopes: one batch, admitted whole or
// not at all.
type Submission struct {
	Envelopes []SubmittedEnvelope `json:"envelopes"`
}

// SubmittedEnvelope is one observation, shaped to a Definition. It names no source:
// the token it arrives under does.
type SubmittedEnvelope struct {
	Type string `json:"type"`
	// DefinitionVersion is the version the payload was shaped to. It must be the
	// type's current version.
	DefinitionVersion int `json:"definition_version"`
	// SourceRef is optional: the gate composes it from the Definition's identity_key,
	// and checks it against that when it is sent.
	SourceRef      string          `json:"source_ref,omitempty"`
	Payload        json.RawMessage `json:"payload"`
	AssertedPerson string          `json:"asserted_person,omitempty"`
	SpanHint       string          `json:"span_hint,omitempty"`
}

// Admission is the answer to a batch the gate took: an outcome for every envelope,
// by its index, and W after it.
type Admission struct {
	W int64 `json:"w"`
	// Changed is how many envelopes were admitted or superseded.
	Changed int              `json:"changed"`
	Results []EnvelopeResult `json:"results"`
}

// EnvelopeResult is what became of one envelope.
type EnvelopeResult struct {
	Index int `json:"index"`
	// Outcome is admitted, superseded or no-op.
	Outcome         string `json:"outcome"`
	FacetID         string `json:"facet_id"`
	ArtifactVersion int    `json:"artifact_version"`
}

// Refusal is the answer to a batch the gate refused: nothing of it was written, and
// Refused names every envelope that failed and why. Error says the same in one line.
type Refusal struct {
	Error   string            `json:"error"`
	Refused []RefusedEnvelope `json:"refused"`
}

// RefusedEnvelope is one envelope of a refused batch.
type RefusedEnvelope struct {
	Index  int    `json:"index"`
	Type   string `json:"type"`
	Reason string `json:"reason"`
}

// Definitions is the published list of Definitions.
type Definitions struct {
	Definitions []DefinitionSummary `json:"definitions"`
}

// The origins of a Definition.
const (
	// OriginBundled is a Definition this build of Anam carries.
	OriginBundled = "bundled"
	// OriginRegistered is a Definition an importer registered.
	OriginRegistered = "registered"
)

// DefinitionSummary is one type's Definitions.
type DefinitionSummary struct {
	Type   string `json:"type"`
	Origin string `json:"origin"`
	// Current is the version envelopes must be shaped to.
	Current int `json:"current"`
	// Versions are every version held, oldest first, each readable at
	// /v1/definitions/{type}/{version}.
	Versions []int `json:"versions"`
}

// Registered is the answer to a Definition registration.
type Registered struct {
	Type    string `json:"type"`
	Version int    `json:"version"`
	// New is false when the same body was already registered at this version.
	New bool `json:"new"`
}
