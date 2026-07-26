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
)

// ErrInvalid is returned when the resolved configuration is unusable.
var ErrInvalid = errors.New("invalid config")

// Config is Anam's resolved runtime configuration.
type Config struct {
	// PersonID is the storage substrate: every facet is partitioned under
	// person#<id>. Anam serves exactly one person (§4).
	PersonID string `mapstructure:"person_id"`

	// DDBEndpoint addresses DynamoDB Local, which holds the ledger: facets,
	// anam-events and the watermark (§8).
	DDBEndpoint string `mapstructure:"ddb_endpoint"`

	// DDBRegion is nominal against DynamoDB Local, which does not authenticate, but the
	// AWS SDK requires one.
	DDBRegion string `mapstructure:"ddb_region"`

	// TableName is the single DynamoDB table holding every ledger item kind (§8).
	TableName string `mapstructure:"table_name"`

	// DBPath is the SQLite tracking database. It holds nothing that cannot be rebuilt
	// by replaying the ledger, so losing it costs a replay, not a fact (§8).
	DBPath string `mapstructure:"db_path"`

	// ArtifactDir holds content-addressed checkpoint values and served receipts —
	// published artifacts, immutable, backed up rather than rebuilt (§8).
	ArtifactDir string `mapstructure:"artifact_dir"`

	// Timeout bounds every call to a store. Anam fails fast on an unreachable endpoint
	// rather than hanging (§8).
	Timeout time.Duration `mapstructure:"timeout"`
}

// Load resolves configuration, reading the YAML file at path when it is non-empty.
// Environment variables override the file, which overrides the defaults.
func Load(path string) (Config, error) {
	v := viper.New()
	v.SetEnvPrefix("ANAM")
	v.AutomaticEnv()

	// person_id has no default on purpose. It is the ledger's partition key
	// (person#<id>), so a defaulted or mistyped value would not fail — it would quietly
	// write facets into a partition nobody chose. Validate rejects it when empty.
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

	v.SetDefault("ddb_endpoint", "http://localhost:8000")
	v.SetDefault("ddb_region", "us-east-1")
	v.SetDefault("table_name", "anam")
	v.SetDefault("db_path", DefaultDBPath())
	v.SetDefault("artifact_dir", DefaultArtifactDir())
	v.SetDefault("timeout", 5*time.Second)

	if path != "" {
		v.SetConfigFile(path)
		if err := v.ReadInConfig(); err != nil {
			return Config{}, fmt.Errorf("read config %q: %w", path, err)
		}
	}

	// An unrecognised key is an error, not a shrug. Config is external input, and a
	// silently-ignored `ddb_endpont` would leave Anam talking to the default endpoint
	// while the operator believes it is talking to theirs — the failure you never
	// notice. Fail fast at startup instead.
	var c Config
	errorOnUnknownKeys := func(dc *mapstructure.DecoderConfig) { dc.ErrorUnused = true }
	if err := v.Unmarshal(&c, errorOnUnknownKeys); err != nil {
		return Config{}, fmt.Errorf("decode config: %w: %v", ErrInvalid, err)
	}

	// Filesystem paths are expanded once, here, so every consumer sees the same
	// absolute path the operator meant — a "~/" that reached SQLite verbatim would
	// quietly create a directory literally named "~".
	for name, p := range map[string]*string{"db_path": &c.DBPath, "artifact_dir": &c.ArtifactDir} {
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
	case c.DDBEndpoint == "":
		return fmt.Errorf("%w: ddb_endpoint is required", ErrInvalid)
	case c.DDBRegion == "":
		return fmt.Errorf("%w: ddb_region is required", ErrInvalid)
	case c.TableName == "":
		return fmt.Errorf("%w: table_name is required", ErrInvalid)
	case c.DBPath == "":
		return fmt.Errorf("%w: db_path is required", ErrInvalid)
	case c.ArtifactDir == "":
		return fmt.Errorf("%w: artifact_dir is required", ErrInvalid)
	case c.Timeout <= 0:
		return fmt.Errorf("%w: timeout must be positive, got %s", ErrInvalid, c.Timeout)
	}
	return nil
}

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
