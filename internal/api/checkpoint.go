package api

// Checkpoints is every checkpoint the log still holds, by bucket and version. A mint
// removes the ones nothing reads any more, so this is the newest of each bucket and
// the older ones a receipt keeps a floor for.
type Checkpoints struct {
	// DB is the operational store the log was read from, on the machine that read it.
	DB          string       `json:"db"`
	Checkpoints []Checkpoint `json:"checkpoints"`
}

// Checkpoint is one bucket's value at one version: the watermark it folded and the
// hash of the value filed in the operational store.
type Checkpoint struct {
	SpanID    string `json:"span_id"`
	V         int    `json:"v"`
	CoversW   int64  `json:"covers_w"`
	ValueHash string `json:"value_hash"`
	MintedAt  string `json:"minted_at"`
}

// Mint is one checkpoint run: the watermark every bucket was folded at, and what
// happened to each bucket.
type Mint struct {
	W int64 `json:"w"`
	// Written is false for a dry run: everything was derived, nothing was published.
	Written bool `json:"written"`
	// Full is true when the mint folded the whole ledger; false when it started from
	// the mint run at From and rebuilt only the buckets the events after it touched.
	Full     bool          `json:"full"`
	From     int64         `json:"from,omitempty"`
	Outcomes []MintOutcome `json:"outcomes"`
	// Collected is what the run removed afterwards: the checkpoints nothing reads any
	// more. CollectError is why that failed, when it did; the mint itself stands, and
	// the next one collects.
	Collected    Collected `json:"collected"`
	CollectError string    `json:"collect_error,omitempty"`
}

// Collected counts what a mint removed once it had finished: checkpoint rows, the mint
// runs only they were read from, and the values no row names, with their size.
type Collected struct {
	Checkpoints int   `json:"checkpoints"`
	Runs        int   `json:"runs"`
	Values      int   `json:"values"`
	Bytes       int64 `json:"bytes"`
}

// MintQuery is how a mint is asked for. DryRun derives and publishes nothing; Full
// folds the whole ledger instead of starting from the last run, the repair if the
// checkpoint log is ever wrong.
type MintQuery struct {
	DryRun bool
	Full   bool
}

// MintOutcome is one bucket in a mint. Minted is false when the head checkpoint's
// hash already matched and nothing was written; V and CoversW describe the row that
// stands after the run either way.
type MintOutcome struct {
	SpanID    string `json:"span_id"`
	V         int    `json:"v"`
	CoversW   int64  `json:"covers_w"`
	ValueHash string `json:"value_hash"`
	Len       int    `json:"len"`
	Minted    bool   `json:"minted"`
}

// MintedCount is how many buckets received a new version.
func (m Mint) MintedCount() int {
	n := 0
	for _, o := range m.Outcomes {
		if o.Minted {
			n++
		}
	}
	return n
}
