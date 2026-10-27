// Package api is the contract between the daemon and its clients: the values the /v1
// routes answer with. It holds data and nothing that computes it.
//
// A CLI command renders one of these values whichever side produced it. Run against
// local files, the command computes the value itself; pointed at a daemon, it decodes
// the same value from the response body. There is no second renderer for remote.
package api

// Error is the body of every response that is not a success. Error says what went
// wrong in words. Code, when set, names it for a program: result_too_large, with its
// measure in TooLarge, or cursor_invalid.
type Error struct {
	Error    string    `json:"error"`
	Code     string    `json:"code,omitempty"`
	TooLarge *TooLarge `json:"too_large,omitempty"`
}

// CodeTooLarge is the Code of a read refused for its size (413).
const CodeTooLarge = "result_too_large"

// CodeCursorInvalid is the Code of a page asked for with a cursor that does not name
// a page of the receipt it was presented to (400, card 92).
const CodeCursorInvalid = "cursor_invalid"

// TooLarge measures a read the daemon refused to answer whole: how many entries
// matched, what they encode to (counted up to the bound only), the bound, and the
// narrowings the query has not used yet, each a query parameter name.
//
// A published read pages rather than refuse (card 92), so there it is refused only
// for one facet over the bound alone; FacetID names it, and Matched is 1. A page
// refused so carries NextCursor, the page after that facet, so a reader can open the
// facet alone (GET /v1/receipts/{hash}/facets/{facet_id}) and walk on.
type TooLarge struct {
	Matched  int `json:"matched"`
	Bytes    int `json:"bytes"`
	MaxBytes int `json:"max_bytes"`
	// Steps and MaxSteps are set when a query was refused for its work, not its size.
	Steps      int      `json:"steps,omitempty"`
	MaxSteps   int      `json:"max_steps,omitempty"`
	Narrow     []string `json:"narrow,omitempty"`
	FacetID    string   `json:"facet_id,omitempty"`
	NextCursor string   `json:"next_cursor,omitempty"`
}

// Status is the answer to "what state is the record in".
type Status struct {
	// Version is the build that answered: the daemon's, or the CLI's own when it read
	// local files.
	Version  string `json:"version"`
	Person   string `json:"person"`
	LedgerID string `json:"ledger_id"`
	W        int64  `json:"w"`
	// Since is when the daemon started; Sources covers only what it has seen since.
	// Both are empty for a local read, which has no history of its own.
	Since   string         `json:"since,omitempty"`
	Sources []SourceStatus `json:"sources"`
	// Held are the sources the operator has held: their admission is refused until
	// the hold is released. "*" is the global hold, and it holds every source but the
	// workbench, which is never held.
	Held []string `json:"held,omitempty"`
	// DefaultProjection is the projection the operator advises a client to read first
	// (default_projection, card 180). It is advertised, never applied: a read that
	// names no projection is the read it always was. Absent when none is set.
	DefaultProjection string `json:"default_projection,omitempty"`
}

// SourceStatus is what the daemon has seen of one importer's source since it started.
type SourceStatus struct {
	Source string `json:"source"`
	// LastScanAt is when the source's last batch arrived. The name is from when Anam
	// read sources itself; the wire keeps it so clients need not change.
	LastScanAt string `json:"last_scan_at"`
	// LastAdmissionAt is when the last batch that admitted or superseded anything
	// arrived. Empty if none has since the daemon started.
	LastAdmissionAt string `json:"last_admission_at,omitempty"`
}

// Backup is one backup of the record the daemon took: where it is, the W its ledger
// holds, its size, and the older backups removed to keep the count.
type Backup struct {
	Path   string   `json:"path"`
	W      int64    `json:"w"`
	Bytes  int64    `json:"bytes"`
	Files  int      `json:"files"`
	Pruned []string `json:"pruned,omitempty"`
}

// HoldRequest is the body of POST /v1/hold: the sources to hold, or every source when
// All is set. A source that is held already keeps the hold it has.
type HoldRequest struct {
	Sources []string `json:"sources,omitempty"`
	All     bool     `json:"all,omitempty"`
}

// HoldReleaseRequest is the body of DELETE /v1/hold: the sources to release, or every
// hold when none is named.
type HoldReleaseRequest struct {
	Sources []string `json:"sources,omitempty"`
	All     bool     `json:"all,omitempty"`
}

// Holds is the answer to a hold: the sources held now.
type Holds struct {
	Held []string `json:"held"`
}

// RetractRequest is the body of POST /v1/facets/{id}/retract: the source the facet must
// belong to. It is a guard, not a selector: the facet is named by the path, and a
// facet of another source is refused.
type RetractRequest struct {
	Source string `json:"source"`
}

// Retraction is a facet tombstoned by hand: which version the tombstone marks, the
// anam-event's position and the watermark after it. Outcome is tombstoned, or no-op
// when another writer tombstoned the facet between the plan and the write, in which
// case there is no Seq of this request's.
type Retraction struct {
	FacetID         string `json:"facet_id"`
	Source          string `json:"source"`
	Type            string `json:"type"`
	ArtifactVersion int    `json:"artifact_version"`
	Outcome         string `json:"outcome"`
	Seq             int64  `json:"seq,omitempty"`
	W               int64  `json:"w"`
}

// MintRun is a mint the daemon ran on its own account, once batches stopped arriving.
type MintRun struct {
	W       int64 `json:"w"`
	Full    bool  `json:"full"`
	From    int64 `json:"from,omitempty"`
	Minted  int   `json:"minted"`
	Buckets int   `json:"buckets"`
	// Error is set when the mint failed; the checkpoints stand where they were, and
	// the next mint (after the next changing batch, or `anam checkpoint`) catches up.
	Error string `json:"error,omitempty"`
}
