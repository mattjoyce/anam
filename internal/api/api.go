// Package api is the contract between the daemon and its clients: the values the /v1
// routes answer with. It holds data and nothing that computes it.
//
// A CLI command renders one of these values whichever side produced it. Run against
// local files, the command computes the value itself; pointed at a daemon, it decodes
// the same value from the response body. There is no second renderer for remote.
package api

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
