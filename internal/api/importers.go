package api

import "encoding/json"

// Submission is what an importer posts to /v1/envelopes: one batch, admitted whole or
// not at all.
type Submission struct {
	Envelopes []SubmittedEnvelope `json:"envelopes"`
	// Snapshot, when set, marks the batch as one part of a complete resend: a run
	// that says it is everything the source holds of some types (card 78). Admission
	// is the same either way.
	Snapshot *SnapshotPart `json:"snapshot,omitempty"`
}

// SnapshotPart labels one batch of a complete resend. ID is the importer's, fresh for
// each run; Part counts from 1 to Parts; Types are the types the run speaks for, all
// within the importer's, and every envelope in the batch is one of them.
type SnapshotPart struct {
	ID    string   `json:"id"`
	Part  int      `json:"part"`
	Parts int      `json:"parts"`
	Types []string `json:"types"`
}

// SnapshotProgress is how far a snapshot has got, returned with each part admitted.
type SnapshotProgress struct {
	ID            string `json:"id"`
	Parts         int    `json:"parts"`
	PartsAdmitted int    `json:"parts_admitted"`
	Complete      bool   `json:"complete"`
}

// SnapshotReport is what Anam makes of a snapshot. Live and Absent are set once it is
// complete: the source's live facets of its types, and those it did not carry and
// that nothing has touched since it began (started_w). Nothing is tombstoned by it;
// that is an operator's anam retract --yes.
type SnapshotReport struct {
	SnapshotProgress
	Source      string   `json:"source"`
	Types       []string `json:"types"`
	Carried     int      `json:"carried"`
	StartedAt   string   `json:"started_at"`
	StartedW    int64    `json:"started_w"`
	CompletedAt string   `json:"completed_at,omitempty"`
	CompletedW  int64    `json:"completed_w,omitempty"`
	Live        *int     `json:"live,omitempty"`
	Absent      *int     `json:"absent,omitempty"`
	// AbsentIDs lists the absent facets, at most AbsentIDsMax of them.
	AbsentIDs []string `json:"absent_ids,omitempty"`
}

// AbsentIDsMax bounds the absent ids a report lists; Absent still counts them all.
const AbsentIDsMax = 1000

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
	// Snapshot is the snapshot's progress when the batch was one of its parts.
	Snapshot *SnapshotProgress `json:"snapshot,omitempty"`
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
