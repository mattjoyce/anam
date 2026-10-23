package api

// Projections is the published list of projection templates (card 125).
type Projections struct {
	// Default names the entry the operator advises a client to read first, the same
	// name GET /v1/status gives as default_projection. It is a top-level field rather
	// than a flag on each entry so there is one place to look and no entry can
	// disagree with another. Absent when none is set; never applied to a read.
	Default     string              `json:"default,omitempty"`
	Projections []ProjectionSummary `json:"projections"`
}

// ProjectionSummary is one template as a consumer selects it: its newest version and
// every version held, each readable at /v1/projections/{name}/{version}.
type ProjectionSummary struct {
	Name string `json:"name"`
	// Origin is bundled when the newest version ships with this build, registered
	// when the operator registered it.
	Origin      string   `json:"origin"`
	Current     int      `json:"current"`
	Versions    []int    `json:"versions"`
	Description string   `json:"description"`
	Types       []string `json:"types"`
	Shape       string   `json:"shape"`
	Hash        string   `json:"hash"`
}

// RegisteredProjection is the answer to a template registration.
type RegisteredProjection struct {
	Name    string `json:"name"`
	Version int    `json:"version"`
	Hash    string `json:"hash"`
	// New is false when the same body was already held at this version.
	New bool `json:"new"`
}
