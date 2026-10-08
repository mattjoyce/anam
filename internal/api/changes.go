package api

// ChangesQuery asks what Anam learned between two anam-times, as the reader typed
// them, with the zone a bare date is a day in.
type ChangesQuery struct {
	Since string `json:"since"`
	Until string `json:"until,omitempty"`
	Zone  string `json:"zone"`
}

// Changes is what was admitted, superseded or tombstoned in a window of anam-time,
// read from the operational store's index of anam-events.
type Changes struct {
	// IndexSeq is the last anam-event the index holds. CaughtUpFrom is where it stood
	// before this read caught it up; equal when the read did not need to.
	IndexSeq     int64 `json:"index_seq"`
	CaughtUpFrom int64 `json:"caught_up_from"`
	// W is the ledger's head when it was read. Above IndexSeq means the index is
	// behind and the newest changes are missing from this answer.
	W int64 `json:"w"`
	// Since and Until are the window's ends, resolved; Until is empty for "to now".
	Since   string   `json:"since"`
	Until   string   `json:"until,omitempty"`
	Changes []Change `json:"changes"`
}

// Change is one anam-event.
type Change struct {
	Seq             int64  `json:"seq"`
	AnamTime        string `json:"anam_time"`
	Kind            string `json:"kind"`
	FacetID         string `json:"facet_id"`
	ArtifactVersion int    `json:"artifact_version"`
}
