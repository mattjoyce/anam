// Package api is the contract between the daemon and its clients: the values the /v1
// routes answer with. It holds data and nothing that computes it.
//
// A CLI command renders one of these values whichever side produced it. Run against
// local files, the command computes the value itself; pointed at a daemon, it decodes
// the same value from the response body. There is no second renderer for remote.
package api

import "fmt"

// Error is the body of every response that is not a success. Error says what went
// wrong in words. Code, when set, names it for a program: today result_too_large
// alone, with its measure in TooLarge.
type Error struct {
	Error    string    `json:"error"`
	Code     string    `json:"code,omitempty"`
	TooLarge *TooLarge `json:"too_large,omitempty"`
}

// CodeTooLarge is the Code of a read refused for its size (413).
const CodeTooLarge = "result_too_large"

// TooLarge measures a read the daemon refused to answer whole: how many entries
// matched, what they encode to (counted up to the bound only), the bound, and the
// narrowings the query has not used yet, each a query parameter name.
type TooLarge struct {
	Matched  int      `json:"matched"`
	Bytes    int      `json:"bytes"`
	MaxBytes int      `json:"max_bytes"`
	Narrow   []string `json:"narrow,omitempty"`
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
	Ingest  IngestState    `json:"ingest"`
}

// IngestState names the daemon's running and pending ingest runs, if any.
type IngestState struct {
	Running string `json:"running,omitempty"`
	Pending string `json:"pending,omitempty"`
}

// SourceStatus is what the daemon has seen of one source since it started.
type SourceStatus struct {
	Source string `json:"source"`
	// LastScanAt is when the last run that read this source began.
	LastScanAt string `json:"last_scan_at"`
	// LastAdmissionAt is when the last run that admitted or superseded anything
	// from this source began. Empty if none has since the daemon started.
	LastAdmissionAt string `json:"last_admission_at,omitempty"`
}

// SourceResult is one declared import's part of an ingest run.
type SourceResult struct {
	Source     string `json:"source"`
	Offered    int    `json:"offered"`
	Admitted   int    `json:"admitted"`
	Superseded int    `json:"superseded"`
	Unchanged  int    `json:"unchanged"`
	// Committed is how many of Offered have an outcome. Less than Offered means the
	// run stopped inside this source.
	Committed int `json:"committed"`
	// W is the watermark this source's last committed block left behind. It is nil
	// when no block committed: the zero watermark is a real value meaning "empty
	// ledger", and a run that committed nothing never observed it.
	W *int64 `json:"w,omitempty"`
}

// Operation states.
const (
	StatePending   = "pending"
	StateRunning   = "running"
	StateSucceeded = "succeeded"
	StateFailed    = "failed"
)

// Operation is one ingest run. The daemon holds it in memory and forgets it on
// restart; the durable answer to "what landed" is the range WBefore..WAfter, which
// the ledger can always be asked about again.
type Operation struct {
	ID          string         `json:"operation_id"`
	Kind        string         `json:"kind"`
	State       string         `json:"state"`
	RequestedAt string         `json:"requested_at"`
	StartedAt   string         `json:"started_at,omitempty"`
	FinishedAt  string         `json:"finished_at,omitempty"`
	WBefore     *int64         `json:"w_before,omitempty"`
	WAfter      *int64         `json:"w_after,omitempty"`
	Sources     []SourceResult `json:"sources,omitempty"`
	// Changed is how many facets the run admitted or superseded, failed or not.
	Changed int `json:"changed"`
	// Summary is Verdict's line for a run that succeeded.
	Summary string `json:"summary,omitempty"`
	Error   string `json:"error,omitempty"`
	// Mint is the mint the daemon ran after the ingest, present when the ingest moved
	// W. Its failure is its own: the ingest's state does not depend on it.
	Mint *MintRun `json:"mint,omitempty"`
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

// MintRun is a mint the daemon ran on its own account, after an ingest.
type MintRun struct {
	W       int64 `json:"w"`
	Full    bool  `json:"full"`
	From    int64 `json:"from,omitempty"`
	Minted  int   `json:"minted"`
	Buckets int   `json:"buckets"`
	// Error is set when the mint failed; the checkpoints stand where they were, and
	// the next mint (after the next changing ingest, or `anam checkpoint`) catches up.
	Error string `json:"error,omitempty"`
}

// Finished reports whether the operation has reached a final state.
func (op Operation) Finished() bool { return op.State == StateSucceeded || op.State == StateFailed }

// NothingNew is the summary of a complete run that wrote nothing. It is a fixed
// string because a scheduler matches on it to stay quiet about such a run.
const NothingNew = "ledger unchanged: nothing new in any source"

// Verdict is the one-line summary of a complete run: whether the ledger moved, in a
// line that reads the same to a person and to a string match.
func Verdict(changed int) string {
	if changed == 0 {
		return NothingNew
	}
	return fmt.Sprintf("ledger changed: %d facets admitted or superseded", changed)
}
