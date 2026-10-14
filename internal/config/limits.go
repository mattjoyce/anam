package config

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
)

// Limits are the daemon's and the client's opinionated defaults (card 151): each is a
// choice someone could reasonably make differently, so each is a key, and the
// Default… constant below is the value Anam ships and stands behind. A limit reaches
// the code as a parameter taken from the one Config loaded at start; the function
// that uses it does not know where the number came from.
//
// A value that is not here and is still a constant in the code is an invariant, and
// says beside it why it is not configurable (AGENTS.md, "Defaults and invariants").
type Limits struct {
	// HTTPReadHeaderTimeout bounds reading a request's headers, against a client that
	// opens a connection and sends nothing.
	HTTPReadHeaderTimeout time.Duration `mapstructure:"http_read_header_timeout"`

	// HTTPReadTimeout bounds reading a whole request, its body included. The largest
	// body is a posted document (max_document_bytes).
	HTTPReadTimeout time.Duration `mapstructure:"http_read_timeout"`

	// HTTPWriteTimeout bounds writing one response. The inspector's whole-record page
	// is the largest.
	HTTPWriteTimeout time.Duration `mapstructure:"http_write_timeout"`

	// HTTPIdleTimeout is how long a kept-alive connection waits for its next request.
	HTTPIdleTimeout time.Duration `mapstructure:"http_idle_timeout"`

	// ShutdownGrace bounds how long shutdown waits for in-flight HTTP requests.
	ShutdownGrace time.Duration `mapstructure:"shutdown_grace"`

	// AcceptWait is how long an operator's write (a mint, POST /v1/checkpoints) waits
	// for the write queue before it is refused as busy.
	AcceptWait time.Duration `mapstructure:"accept_wait"`

	// ImporterWait is how long an importer's write waits for the write queue before it
	// is refused as busy: long enough to wait out a mint after the batch before it.
	ImporterWait time.Duration `mapstructure:"importer_wait"`

	// RetryAfter is what a busy answer (503) asks its caller to wait. It is sent in
	// whole seconds, so it must be at least one.
	RetryAfter time.Duration `mapstructure:"retry_after"`

	// MintQuiet is how long the mint worker waits for batches to stop arriving before
	// it mints, so an importer's hundred batches get one mint and not a hundred.
	MintQuiet time.Duration `mapstructure:"mint_quiet"`

	// MaxPayloadBytes is the most one envelope's payload may hold. A batch's body is
	// bounded by it, at the gate's batch size, with room for the envelopes around them.
	MaxPayloadBytes int `mapstructure:"max_payload_bytes"`

	// CommitRetries is how many times the gate retries a batch that lost the race for
	// the watermark before it reports contention.
	CommitRetries int `mapstructure:"commit_retries"`

	// MaxManifestBytes is the most a registered Definition may hold.
	MaxManifestBytes int `mapstructure:"max_manifest_bytes"`

	// MaxDocumentBytes is the most one posted document may hold.
	MaxDocumentBytes int `mapstructure:"max_document_bytes"`

	// SnapshotTTL is how long an incomplete snapshot is kept before it is dropped.
	SnapshotTTL time.Duration `mapstructure:"snapshot_ttl"`

	// SnapshotKeep is how many complete snapshots are kept per source. The newest is
	// the one retraction plans from.
	SnapshotKeep int `mapstructure:"snapshot_keep"`

	// MaxSnapshotParts is the most parts one snapshot may declare.
	MaxSnapshotParts int `mapstructure:"max_snapshot_parts"`

	// MaxAbsentIDs is the most absent facet ids a snapshot report lists; it still
	// counts them all.
	MaxAbsentIDs int `mapstructure:"max_absent_ids"`

	// MaxAbsentJudged is the most uncarried live facets one snapshot report judges,
	// each a query for that facet's events. Past it the report is 413 and the CLI,
	// local and unbounded, lists them (card 100).
	MaxAbsentJudged int `mapstructure:"max_absent_judged"`

	// MaxResponseBytes is the most the CLI reads of one answer from a daemon.
	MaxResponseBytes int `mapstructure:"max_response_bytes"`

	// NetworkMargin is how much longer the CLI waits for an answer than it asks the
	// daemon to take: the trip there and back.
	NetworkMargin time.Duration `mapstructure:"network_margin"`

	// InspectCachedReads is how many historical (as-of) folds the inspector keeps,
	// bounding its memory however many instants a person clicks through. Zero keeps
	// none.
	InspectCachedReads int `mapstructure:"inspect_cached_reads"`
}

// The defaults Anam ships. Each says why it is the number it is.
const (
	// DefaultHTTPReadHeaderTimeout: headers are a few hundred bytes.
	DefaultHTTPReadHeaderTimeout = 5 * time.Second
	// DefaultHTTPReadTimeout: a 64 MiB document over the home network in well under it.
	DefaultHTTPReadTimeout = 30 * time.Second
	// DefaultHTTPWriteTimeout: the inspector's whole-record page is about 5 MB and
	// renders in about a second, so this leaves a slow link plenty.
	DefaultHTTPWriteTimeout = 2 * time.Minute
	// DefaultHTTPIdleTimeout: an importer's batches arrive seconds apart.
	DefaultHTTPIdleTimeout = 2 * time.Minute
	// DefaultShutdownGrace: each request is already bounded by timeout, so this only
	// has to cover one.
	DefaultShutdownGrace = 10 * time.Second
	// DefaultAcceptWait: the worker is back at the queue microseconds after a write
	// ends, so a write still running after this long is one worth refusing for.
	DefaultAcceptWait = 250 * time.Millisecond
	// DefaultImporterWait: long enough to wait out a mint, short of a long write.
	DefaultImporterWait = 5 * time.Second
	// DefaultRetryAfter: what holds the queue past importer_wait is a long write (a
	// backup, a full mint, a replay into the index), which takes tens of seconds.
	DefaultRetryAfter = 15 * time.Second
	// DefaultMintQuiet: an importer's batches arrive well inside it.
	DefaultMintQuiet = 2 * time.Second
	// DefaultMaxPayloadBytes: 300 KiB, far above the largest payload any source sends.
	DefaultMaxPayloadBytes = 300 << 10
	// DefaultCommitRetries: the daemon is the only writer, so a lost race is rare and
	// a second is rarer.
	DefaultCommitRetries = 5
	// DefaultMaxManifestBytes: the largest bundled Definition is a few KB.
	DefaultMaxManifestBytes = 1 << 20
	// DefaultMaxDocumentBytes: a scanned letter or a lab report is a few MB; a bound
	// keeps one request from filling the disk.
	DefaultMaxDocumentBytes = 64 << 20
	// DefaultSnapshotTTL: a run that has not finished in a day has stopped.
	DefaultSnapshotTTL = 24 * time.Hour
	// DefaultSnapshotKeep: the newest, which retraction plans from, and the one
	// before it to compare against.
	DefaultSnapshotKeep = 2
	// DefaultMaxSnapshotParts: at 49 envelopes a part, far beyond any source.
	DefaultMaxSnapshotParts = 100000
	// DefaultMaxAbsentIDs: a report a person can still read.
	DefaultMaxAbsentIDs = 1000
	// DefaultMaxAbsentJudged: a few seconds of queries on the NAS.
	DefaultMaxAbsentJudged = 5000
	// DefaultMaxResponseBytes: the whole-person timeline is about 5 MB today; this
	// leaves it room to grow tenfold.
	DefaultMaxResponseBytes = 64 << 20
	// DefaultNetworkMargin: the home network's round trip, many times over.
	DefaultNetworkMargin = 10 * time.Second
	// DefaultInspectCachedReads: enough to flick between a couple of instants without
	// folding again.
	DefaultInspectCachedReads = 4
)

func setLimitDefaults(v *viper.Viper) {
	v.SetDefault("http_read_header_timeout", DefaultHTTPReadHeaderTimeout)
	v.SetDefault("http_read_timeout", DefaultHTTPReadTimeout)
	v.SetDefault("http_write_timeout", DefaultHTTPWriteTimeout)
	v.SetDefault("http_idle_timeout", DefaultHTTPIdleTimeout)
	v.SetDefault("shutdown_grace", DefaultShutdownGrace)
	v.SetDefault("accept_wait", DefaultAcceptWait)
	v.SetDefault("importer_wait", DefaultImporterWait)
	v.SetDefault("retry_after", DefaultRetryAfter)
	v.SetDefault("mint_quiet", DefaultMintQuiet)
	v.SetDefault("max_payload_bytes", DefaultMaxPayloadBytes)
	v.SetDefault("commit_retries", DefaultCommitRetries)
	v.SetDefault("max_manifest_bytes", DefaultMaxManifestBytes)
	v.SetDefault("max_document_bytes", DefaultMaxDocumentBytes)
	v.SetDefault("snapshot_ttl", DefaultSnapshotTTL)
	v.SetDefault("snapshot_keep", DefaultSnapshotKeep)
	v.SetDefault("max_snapshot_parts", DefaultMaxSnapshotParts)
	v.SetDefault("max_absent_ids", DefaultMaxAbsentIDs)
	v.SetDefault("max_absent_judged", DefaultMaxAbsentJudged)
	v.SetDefault("max_response_bytes", DefaultMaxResponseBytes)
	v.SetDefault("network_margin", DefaultNetworkMargin)
	v.SetDefault("inspect_cached_reads", DefaultInspectCachedReads)
}

// DefaultLimits is every limit at its default: what a test or a tool that loads no
// config passes, the same values Load resolves when nothing overrides them.
func DefaultLimits() Limits {
	return Limits{
		HTTPReadHeaderTimeout: DefaultHTTPReadHeaderTimeout,
		HTTPReadTimeout:       DefaultHTTPReadTimeout,
		HTTPWriteTimeout:      DefaultHTTPWriteTimeout,
		HTTPIdleTimeout:       DefaultHTTPIdleTimeout,
		ShutdownGrace:         DefaultShutdownGrace,
		AcceptWait:            DefaultAcceptWait,
		ImporterWait:          DefaultImporterWait,
		RetryAfter:            DefaultRetryAfter,
		MintQuiet:             DefaultMintQuiet,
		MaxPayloadBytes:       DefaultMaxPayloadBytes,
		CommitRetries:         DefaultCommitRetries,
		MaxManifestBytes:      DefaultMaxManifestBytes,
		MaxDocumentBytes:      DefaultMaxDocumentBytes,
		SnapshotTTL:           DefaultSnapshotTTL,
		SnapshotKeep:          DefaultSnapshotKeep,
		MaxSnapshotParts:      DefaultMaxSnapshotParts,
		MaxAbsentIDs:          DefaultMaxAbsentIDs,
		MaxAbsentJudged:       DefaultMaxAbsentJudged,
		MaxResponseBytes:      DefaultMaxResponseBytes,
		NetworkMargin:         DefaultNetworkMargin,
		InspectCachedReads:    DefaultInspectCachedReads,
	}
}

// maxLimitBytes caps every byte limit: past 1 GiB a single request or answer is held
// in memory whole, which no deployment of Anam wants.
const maxLimitBytes = 1 << 30

// Validate reports the first limit out of its range, so a bad override fails at
// start and not at the first request that uses it.
func (l Limits) Validate() error {
	durations := []struct {
		key string
		got time.Duration
		min time.Duration
	}{
		{"http_read_header_timeout", l.HTTPReadHeaderTimeout, time.Millisecond},
		{"http_read_timeout", l.HTTPReadTimeout, time.Millisecond},
		{"http_write_timeout", l.HTTPWriteTimeout, time.Millisecond},
		{"http_idle_timeout", l.HTTPIdleTimeout, time.Millisecond},
		{"shutdown_grace", l.ShutdownGrace, time.Millisecond},
		{"accept_wait", l.AcceptWait, time.Millisecond},
		{"importer_wait", l.ImporterWait, time.Millisecond},
		{"retry_after", l.RetryAfter, time.Second},
		{"mint_quiet", l.MintQuiet, time.Millisecond},
		{"snapshot_ttl", l.SnapshotTTL, time.Minute},
		{"network_margin", l.NetworkMargin, 0},
	}
	for _, d := range durations {
		if d.got < d.min {
			return fmt.Errorf("%w: %s must be at least %s, got %s", ErrInvalid, d.key, d.min, d.got)
		}
	}
	counts := []struct {
		key      string
		got      int
		min, max int
	}{
		{"max_payload_bytes", l.MaxPayloadBytes, 1 << 10, maxLimitBytes},
		{"commit_retries", l.CommitRetries, 1, 100},
		{"max_manifest_bytes", l.MaxManifestBytes, 1 << 10, maxLimitBytes},
		{"max_document_bytes", l.MaxDocumentBytes, 1, maxLimitBytes},
		{"snapshot_keep", l.SnapshotKeep, 1, 1000},
		{"max_snapshot_parts", l.MaxSnapshotParts, 1, 10_000_000},
		{"max_absent_ids", l.MaxAbsentIDs, 0, 10_000_000},
		{"max_absent_judged", l.MaxAbsentJudged, 1, 10_000_000},
		{"max_response_bytes", l.MaxResponseBytes, 1 << 10, maxLimitBytes},
		{"inspect_cached_reads", l.InspectCachedReads, 0, 1000},
	}
	for _, c := range counts {
		if c.got < c.min {
			return fmt.Errorf("%w: %s must be from %d to %d, got %d", ErrInvalid, c.key, c.min, c.max, c.got)
		}
		if c.got > c.max {
			return fmt.Errorf("%w: %s must be from %d to %d, got %d", ErrInvalid, c.key, c.min, c.max, c.got)
		}
	}
	return nil
}
