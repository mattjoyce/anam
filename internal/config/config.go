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
)

// ErrInvalid is returned when the resolved configuration is unusable.
var ErrInvalid = errors.New("invalid config")

// Config is Anam's resolved runtime configuration.
type Config struct {
	// PersonID is the storage substrate: every facet is partitioned under
	// person#<id>. Anam serves exactly one person (§4).
	PersonID string `mapstructure:"person_id"`

	// DBPath is the SQLite ledger — a file, not a service (§8). Losing it costs a
	// re-ingest, not a fact.
	DBPath string `mapstructure:"db_path"`

	// Timeout bounds every call to the store. SQLite can block on a write lock, and a
	// hung call is a hung call whatever its cause (§8).
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

	v.SetDefault("db_path", DefaultDBPath())
	v.SetDefault("timeout", 5*time.Second)

	if path != "" {
		v.SetConfigFile(path)
		if err := v.ReadInConfig(); err != nil {
			return Config{}, fmt.Errorf("read config %q: %w", path, err)
		}
	}

	// An unrecognised key is an error, not a shrug. Config is external input, and a
	// silently-ignored `db_pth` would leave Anam writing to the default ledger while
	// the operator believes it is writing to theirs — the failure you never notice.
	// Fail fast at startup instead.
	var c Config
	errorOnUnknownKeys := func(dc *mapstructure.DecoderConfig) { dc.ErrorUnused = true }
	if err := v.Unmarshal(&c, errorOnUnknownKeys); err != nil {
		return Config{}, fmt.Errorf("decode config: %w: %v", ErrInvalid, err)
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
	case c.DBPath == "":
		return fmt.Errorf("%w: db_path is required", ErrInvalid)
	case c.Timeout <= 0:
		return fmt.Errorf("%w: timeout must be positive, got %s", ErrInvalid, c.Timeout)
	}
	return nil
}

// DefaultDBPath is the ledger's home: $XDG_DATA_HOME/anam/anam.db, falling back to
// ~/.local/share/anam/anam.db, and finally to a bare relative name when neither is
// discoverable.
//
// The ledger is data, not config, so it belongs under the data directory rather than
// beside config.yaml. Go's standard library has no UserDataDir, so this is spelled out.
func DefaultDBPath() string {
	const rel = "anam.db"

	if dir := os.Getenv("XDG_DATA_HOME"); dir != "" {
		return filepath.Join(dir, "anam", rel)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return rel
	}
	return filepath.Join(home, ".local", "share", "anam", rel)
}
