// Package config loads Anam's runtime configuration from defaults, an optional YAML
// file, and ANAM_-prefixed environment variables, in increasing order of precedence.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"

	"github.com/mattjoyce/anam/internal/safepath"
	"github.com/mattjoyce/anam/internal/span"
)

// ErrInvalid is returned when the resolved configuration is unusable.
var ErrInvalid = errors.New("invalid config")

// Config is Anam's resolved runtime configuration.
type Config struct {
	// PersonID names whose timeline this is. It is pinned inside the ledger file when
	// the ledger is initialised, and a ledger pinned to another person is refused.
	// Anam serves exactly one person (§4).
	PersonID string `mapstructure:"person_id"`

	// LedgerPath is the SQLite file holding the ledger: facet artifacts and
	// anam-events (§8). It is the record. Back it up; nothing rebuilds the anam-times
	// in it.
	LedgerPath string `mapstructure:"ledger_path"`

	// DBPath is the SQLite operational store, a separate file from the ledger. Its
	// derived tables rebuild by replaying the ledger (§8).
	DBPath string `mapstructure:"db_path"`

	// ArtifactDir holds content-addressed checkpoint values and served receipts —
	// published artifacts, immutable, backed up rather than rebuilt (§8).
	ArtifactDir string `mapstructure:"artifact_dir"`

	// Timeout bounds every call to a store. Anam fails fast rather than hanging (§8).
	Timeout time.Duration `mapstructure:"timeout"`

	// BucketWidth is how much fact-time one checkpoint bucket covers (§4, §7). It is a
	// tuning knob and changes nothing a read returns — but it is stamped into every
	// bucket id the operational index persists, so the ops store pins it and refuses a
	// database that was built at a different width.
	BucketWidth span.Width `mapstructure:"bucket_width"`
}

// Load resolves configuration, reading the YAML file at path when it is non-empty.
// Environment variables override the file, which overrides the defaults.
func Load(path string) (Config, error) {
	v := viper.New()
	v.SetEnvPrefix("ANAM")
	v.AutomaticEnv()

	// person_id has no default on purpose. It is pinned into the ledger at init, so a
	// defaulted value would name a person nobody chose for the life of the file.
	// Validate rejects it when empty.
	//
	// BindEnv is still required: Unmarshal decodes from viper's known keys, and a key
	// with neither a default nor a config entry is unknown, so ANAM_PERSON_ID would be
	// ignored. BindEnv registers the key without giving it a value.
	//
	// It must come after SetEnvPrefix, which BindEnv reads at call time to build the
	// ANAM_ name. AutomaticEnv happens to paper over the wrong order today; do not rely
	// on that.
	if err := v.BindEnv("person_id"); err != nil {
		return Config{}, fmt.Errorf("bind person_id: %w", err)
	}

	v.SetDefault("ledger_path", DefaultLedgerPath())
	v.SetDefault("db_path", DefaultDBPath())
	v.SetDefault("artifact_dir", DefaultArtifactDir())
	v.SetDefault("timeout", 5*time.Second)
	v.SetDefault("bucket_width", string(span.Default))

	if path != "" {
		v.SetConfigFile(path)
		if err := v.ReadInConfig(); err != nil {
			return Config{}, fmt.Errorf("read config %q: %w", path, err)
		}
	}

	// An unrecognised key is an error, not a shrug. Config is external input, and a
	// silently-ignored `ledger_pth` would leave Anam writing to the default ledger
	// while the operator believes it is writing to theirs. Fail fast at startup
	// instead. The same rule retires the DynamoDB keys: a config that still names
	// `ddb_endpoint` is refused, which is how its owner learns the ledger moved.
	var c Config
	errorOnUnknownKeys := func(dc *mapstructure.DecoderConfig) { dc.ErrorUnused = true }
	if err := v.Unmarshal(&c, errorOnUnknownKeys); err != nil {
		return Config{}, fmt.Errorf("decode config: %w: %v", ErrInvalid, err)
	}

	// Filesystem paths are expanded once, here, so every consumer sees the same
	// absolute path the operator meant — a "~/" that reached SQLite verbatim would
	// quietly create a directory literally named "~".
	for name, p := range map[string]*string{
		"ledger_path": &c.LedgerPath, "db_path": &c.DBPath, "artifact_dir": &c.ArtifactDir,
	} {
		expanded, err := safepath.Expand(*p)
		if err != nil {
			return Config{}, fmt.Errorf("%w: %s: %v", ErrInvalid, name, err)
		}
		*p = expanded
	}

	if err := c.Validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}

// Validate reports whether c is usable.
func (c Config) Validate() error {
	switch {
	case c.PersonID == "":
		return fmt.Errorf("%w: person_id is required", ErrInvalid)
	case c.LedgerPath == "":
		return fmt.Errorf("%w: ledger_path is required", ErrInvalid)
	case c.DBPath == "":
		return fmt.Errorf("%w: db_path is required", ErrInvalid)
	case c.LedgerPath == c.DBPath:
		// The operational store's repair is to delete its file. Sharing a file would
		// put the ledger inside that repair.
		return fmt.Errorf("%w: ledger_path and db_path are the same file (%s); the ledger must have its own",
			ErrInvalid, c.LedgerPath)
	case c.ArtifactDir == "":
		return fmt.Errorf("%w: artifact_dir is required", ErrInvalid)
	case c.Timeout <= 0:
		return fmt.Errorf("%w: timeout must be positive, got %s", ErrInvalid, c.Timeout)
	}
	// A typo'd width dies here, at startup, where it costs a re-run — not on the ingest
	// path, where falling back would write months of plausible, wrongly-named buckets.
	if _, err := span.New(c.BucketWidth); err != nil {
		return fmt.Errorf("%w: bucket_width: %v", ErrInvalid, err)
	}
	return nil
}

// Buckets is the bucketing this configuration selects. Validate has already rejected
// an unknown width, so a config that loaded yields one here.
func (c Config) Buckets() (span.Buckets, error) { return span.New(c.BucketWidth) }

// DefaultLedgerPath is the ledger: <data dir>/anam/ledger.db.
func DefaultLedgerPath() string { return dataPath("ledger.db") }

// DefaultDBPath is the tracking database: <data dir>/anam/tracking.db.
func DefaultDBPath() string { return dataPath("tracking.db") }

// DefaultArtifactDir is where checkpoint values and receipts are written:
// <data dir>/anam/artifacts.
func DefaultArtifactDir() string { return dataPath("artifacts") }

// dataPath resolves $XDG_DATA_HOME/anam/<rel>, falling back to
// ~/.local/share/anam/<rel>, and finally to a bare relative name when neither is
// discoverable.
//
// These are data, not config, so they belong under the data directory rather than
// beside config.yaml. Go's standard library has no UserDataDir, so this is spelled out.
func dataPath(rel string) string {
	if dir := os.Getenv("XDG_DATA_HOME"); dir != "" {
		return filepath.Join(dir, "anam", rel)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return rel
	}
	return filepath.Join(home, ".local", "share", "anam", rel)
}
