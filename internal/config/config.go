// Package config loads Anam's runtime configuration from defaults, an optional YAML
// file, and ANAM_-prefixed environment variables, in increasing order of precedence.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"
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

	// LibraryDir is the default library: the folder of primary source documents the
	// facts were extracted from. Anam alone writes it, each document's bytes named by
	// their sha256 (card 150), and a record.document finds them by that name. It
	// defaults to `documents` beside the ledger, so the ledger's directory is the whole
	// record and the backup unit. A folder that does not exist is an empty library,
	// not an error.
	LibraryDir string `mapstructure:"library_dir"`

	// LibraryFolderLimit is how many files one library folder holds before a new
	// document goes a level deeper, into a subfolder named by the start of its sha256
	// (card 150). It places new files only: a stored file never moves, and a lookup
	// finds a document by its name wherever it is, so changing it strands nothing.
	LibraryFolderLimit int `mapstructure:"library_folder_limit"`

	// Zone is the reader's zone: the one a bare date on the command line is taken in
	// and fact-times are shown in. It is not a fact-time rule — each Definition
	// carries the zone its source's clock is read in — it is where the person reading
	// is standing. Defaults to Local.
	Zone string `mapstructure:"zone"`

	// Listen is the address `anam daemon` serves on. It defaults to loopback, and the
	// daemon refuses anything else until its requests carry a token.
	Listen string `mapstructure:"listen"`

	// TokenFile names the file holding the daemon's bearer token. With one, every
	// request but /healthz must present it, and the daemon may listen beyond
	// loopback. Empty means no token, and loopback only. The CLI presents the token in
	// the same file when Server is set.
	TokenFile string `mapstructure:"token_file"`

	// ImportersFile names the YAML file of importers the daemon accepts envelopes
	// from (docs/IMPORTERS.md): each one's source, token hash and facet types. Empty
	// means none, and POST /v1/envelopes refuses everything. It is read at start.
	ImportersFile string `mapstructure:"importers_file"`

	// Server is the URL of a running daemon, such as http://192.168.20.4:8787. With it
	// set, the CLI asks the daemon and opens no local file: the daemon owns the
	// stores. Empty means the CLI opens the files itself, which is maintenance mode
	// and wants the daemon stopped.
	Server string `mapstructure:"server"`

	// BackupDir is where the daemon writes backups of the record (POST /v1/backups),
	// one directory each. Empty means the daemon takes none. It should be on another
	// disk from the record: a backup on the same disk shares its failure.
	BackupDir string `mapstructure:"backup_dir"`

	// BackupKeep is how many backups the daemon keeps; each new one removes the
	// oldest beyond it.
	BackupKeep int `mapstructure:"backup_keep"`

	// LongWriteTimeout bounds one long write inside the daemon: a replay of the
	// ledger into the index, a backup, the mint at start. It also bounds each phase
	// of anam migrate (card 200): the fold of the old ledger, the carry. Timeout
	// still bounds each call within it. Until card 156 the key was ingest_timeout, from when it bounded
	// an ingest run; Load still reads that name, and ANAM_INGEST_TIMEOUT, as an alias.
	LongWriteTimeout time.Duration `mapstructure:"long_write_timeout"`

	// IngestTimeout is the retired name of LongWriteTimeout. It is decoded so a
	// config or environment that still sets it is honoured, with a warning, rather
	// than refused as an unknown key. Load moves its value across and leaves this
	// zero; nothing reads it after that.
	IngestTimeout time.Duration `mapstructure:"ingest_timeout"`

	// Timeout bounds every call to a store. Anam fails fast rather than hanging (§8).
	Timeout time.Duration `mapstructure:"timeout"`

	// MaxReadBytes bounds what one read over the daemon answers with: the entries as
	// the API encodes them, whole facets only, never cut to fit (card 88). A read over
	// it is refused with the size, the count and which narrowings are left. Zero is no
	// bound. A local read, the operator at their own files, is never bounded.
	MaxReadBytes int `mapstructure:"max_read_bytes"`

	// BucketWidth is how much fact-time one checkpoint bucket covers (§4, §7). It is a
	// tuning knob and changes nothing a read returns — but it is stamped into every
	// bucket id the operational index persists, so the ops store pins it and refuses a
	// database that was built at a different width.
	BucketWidth span.Width `mapstructure:"bucket_width"`

	// CheckpointBudget is the most bytes one checkpoint's value may hold before the
	// mint splits its bucket along the calendar: a year into months, a month into days
	// (span.Layout). It prices a read: a bucket a late correction dirties is decoded
	// and re-derived whole. Zero mints at the fixed bucket_width instead. The default,
	// 1 MiB, re-derives in about 50 ms on the NAS (card 59's bench).
	CheckpointBudget int `mapstructure:"checkpoint_budget"`

	// Limits are the daemon's and the client's bounds and waits, each a key with a
	// default (limits.go). Squashed, so their keys sit at the top level beside these.
	Limits `mapstructure:",squash"`
}

// Load resolves configuration, reading the YAML file at path when it is non-empty.
// Environment variables override the file, which overrides the defaults.
func Load(path string) (Config, error) { return LoadOverriding(path, nil) }

// LoadOverriding is Load with set taking precedence over everything else, keyed by
// config key. It is how a command-line flag such as --server wins over the
// environment while still passing the same validation.
func LoadOverriding(path string, set map[string]string) (Config, error) {
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
	// long_write_timeout and its retired name have no viper default either, so that
	// IsSet says whether each was actually given; the default is applied after both
	// are read (longWriteTimeout).
	for _, key := range []string{"long_write_timeout", "ingest_timeout"} {
		if err := v.BindEnv(key); err != nil {
			return Config{}, fmt.Errorf("bind %s: %w", key, err)
		}
	}

	v.SetDefault("ledger_path", DefaultLedgerPath())
	v.SetDefault("db_path", DefaultDBPath())
	v.SetDefault("timeout", DefaultTimeout)
	v.SetDefault("bucket_width", string(span.Default))
	v.SetDefault("library_dir", "")
	v.SetDefault("zone", "Local")
	v.SetDefault("listen", DefaultListen)
	v.SetDefault("token_file", "")
	v.SetDefault("importers_file", "")
	v.SetDefault("server", "")
	v.SetDefault("checkpoint_budget", DefaultCheckpointBudget)
	v.SetDefault("backup_dir", "")
	v.SetDefault("backup_keep", DefaultBackupKeep)
	v.SetDefault("max_read_bytes", DefaultMaxReadBytes)
	v.SetDefault("library_folder_limit", DefaultLibraryFolderLimit)
	setLimitDefaults(v)

	if path != "" {
		v.SetConfigFile(path)
		if err := v.ReadInConfig(); err != nil {
			return Config{}, fmt.Errorf("read config %q: %w", path, err)
		}
	}

	for k, val := range set {
		v.Set(k, val)
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
	if err := c.adoptLongWriteTimeout(v.IsSet("long_write_timeout"), v.IsSet("ingest_timeout")); err != nil {
		return Config{}, err
	}

	// Filesystem paths are expanded once, here, so every consumer sees the same
	// absolute path the operator meant — a "~/" that reached SQLite verbatim would
	// quietly create a directory literally named "~".
	if c.LibraryDir == "" {
		c.LibraryDir = filepath.Join(filepath.Dir(c.LedgerPath), "documents")
	}
	paths := map[string]*string{
		"ledger_path": &c.LedgerPath, "db_path": &c.DBPath, "library_dir": &c.LibraryDir,
	}
	if c.TokenFile != "" {
		paths["token_file"] = &c.TokenFile
	}
	if c.ImportersFile != "" {
		paths["importers_file"] = &c.ImportersFile
	}
	for name, p := range paths {
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

// adoptLongWriteTimeout settles long_write_timeout from the two keys that may name
// it (card 156): the new key when given; its retired name ingest_timeout when only
// that is given, with a warning so the deployment moves; the default when neither
// is. The two given with different values is a config that contradicts itself, so
// it is refused rather than one of them being picked. The retired field is left
// zero either way.
func (c *Config) adoptLongWriteTimeout(newSet, oldSet bool) error {
	switch {
	case newSet && oldSet && c.LongWriteTimeout != c.IngestTimeout:
		return fmt.Errorf("%w: long_write_timeout (%s) and its old name ingest_timeout (%s) differ; set long_write_timeout only",
			ErrInvalid, c.LongWriteTimeout, c.IngestTimeout)
	case oldSet:
		slog.Warn("config: ingest_timeout is the old name of long_write_timeout; set long_write_timeout (ANAM_LONG_WRITE_TIMEOUT) instead",
			"value", c.IngestTimeout)
		c.LongWriteTimeout = c.IngestTimeout
	case !newSet:
		c.LongWriteTimeout = DefaultLongWriteTimeout
	}
	c.IngestTimeout = 0
	return nil
}

// Validate reports whether c is usable.
func (c Config) Validate() error {
	switch {
	case c.PersonID == "" && c.Server == "":
		// A client of a daemon names no person: the daemon serves exactly one.
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
	case c.LibraryDir == "":
		return fmt.Errorf("%w: library_dir is required", ErrInvalid)
	case c.Timeout <= 0:
		return fmt.Errorf("%w: timeout must be positive, got %s", ErrInvalid, c.Timeout)
	case c.LongWriteTimeout <= 0:
		return fmt.Errorf("%w: long_write_timeout must be positive, got %s", ErrInvalid, c.LongWriteTimeout)
	case c.BackupDir != "" && c.BackupKeep < 1:
		return fmt.Errorf("%w: backup_keep must be at least 1, got %d", ErrInvalid, c.BackupKeep)
	case c.CheckpointBudget < 0:
		return fmt.Errorf("%w: checkpoint_budget must be 0 or more bytes, got %d", ErrInvalid, c.CheckpointBudget)
	case c.MaxReadBytes < 0:
		return fmt.Errorf("%w: max_read_bytes must be 0 or more bytes, got %d", ErrInvalid, c.MaxReadBytes)
	case c.LibraryFolderLimit < 1:
		return fmt.Errorf("%w: library_folder_limit must be at least 1, got %d", ErrInvalid, c.LibraryFolderLimit)
	}
	if err := c.Limits.Validate(); err != nil {
		return err
	}
	if c.Server != "" {
		if err := checkServer(c.Server); err != nil {
			return fmt.Errorf("%w: server: %v", ErrInvalid, err)
		}
	}
	// A typo'd width dies here, at startup, where it costs a re-run — not on the ingest
	// path, where falling back would write months of plausible, wrongly-named buckets.
	if _, err := span.New(c.BucketWidth); err != nil {
		return fmt.Errorf("%w: bucket_width: %v", ErrInvalid, err)
	}
	if _, err := time.LoadLocation(c.Zone); err != nil {
		return fmt.Errorf("%w: zone %q: %v", ErrInvalid, c.Zone, err)
	}
	return nil
}

// checkServer accepts an http or https URL naming a host, and nothing else: no path,
// query or user, which the client would otherwise drop or send somewhere unexpected.
func checkServer(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	switch {
	case u.Scheme != "http" && u.Scheme != "https":
		return fmt.Errorf("%q: want http:// or https://", raw)
	case u.Host == "":
		return fmt.Errorf("%q: names no host", raw)
	case u.User != nil, u.RawQuery != "", u.Fragment != "", u.Path != "" && u.Path != "/":
		return fmt.Errorf("%q: want scheme and host only, such as http://192.168.20.4:8787", raw)
	}
	return nil
}

// Location is the reader's zone, loaded. Validate has already accepted the name.
func (c Config) Location() (*time.Location, error) { return time.LoadLocation(c.Zone) }

// ZoneName is the reader's zone as an IANA name, for a read another machine runs.
// "Local" means this machine's zone, which the other machine cannot know, so it is
// named here the way Go's own Local is found: $TZ if it is set, else /etc/localtime.
// A zone that cannot be named is an error that says to set one, rather than a guess
// that would resolve dates in one zone while the CLI shows them in another.
func (c Config) ZoneName() (string, error) {
	if c.Zone != "Local" {
		return c.Zone, nil
	}
	unnamed := fmt.Errorf("%w: zone is Local and this machine's zone has no name to send; set zone (ANAM_ZONE)",
		ErrInvalid)
	if tz, set := os.LookupEnv("TZ"); set {
		// Go reads an empty TZ as UTC, and a path or POSIX rule as itself; only an
		// IANA name can travel.
		tz = strings.TrimPrefix(tz, ":")
		if tz == "" {
			return "UTC", nil
		}
		if strings.HasPrefix(tz, "/") || tz == "Local" {
			return "", unnamed
		}
		if _, err := time.LoadLocation(tz); err != nil {
			return "", unnamed
		}
		return tz, nil
	}
	target, err := os.Readlink("/etc/localtime")
	if err != nil {
		return "", unnamed
	}
	_, name, ok := strings.Cut(target, "zoneinfo/")
	if !ok {
		return "", unnamed
	}
	if _, err := time.LoadLocation(name); err != nil {
		return "", unnamed
	}
	return name, nil
}

// Buckets is the bucketing this configuration selects. Validate has already rejected
// an unknown width, so a config that loaded yields one here.
func (c Config) Buckets() (span.Buckets, error) { return span.New(c.BucketWidth) }

// DefaultTimeout is timeout's default: a call to a store on local disk takes
// milliseconds, so five seconds is a store that is stuck, not slow.
const DefaultTimeout = 5 * time.Second

// DefaultLongWriteTimeout is long_write_timeout's default: a full replay of the
// record into the index takes a few minutes on the NAS.
const DefaultLongWriteTimeout = 15 * time.Minute

// DefaultBackupKeep is backup_keep's default: two weeks of nightly backups.
const DefaultBackupKeep = 14

// DefaultCheckpointBudget is checkpoint_budget's default: 1 MiB.
const DefaultCheckpointBudget = 1 << 20

// DefaultMaxReadBytes is max_read_bytes's default: 512 KiB, the response budget
// docs/API.md §3 proposes. A month of every type is under it; the whole record, at
// 12.8 MB on 2026-10-11, is not.
const DefaultMaxReadBytes = 512 << 10

// DefaultLibraryFolderLimit is library_folder_limit's default: 1000 files, few
// enough that a person listing a folder by hand still reads it.
const DefaultLibraryFolderLimit = 1000

// DefaultListen is the daemon's address when none is configured: loopback, on the
// port the inspector has always used.
const DefaultListen = "127.0.0.1:8787"

// DefaultLedgerPath is the ledger: <data dir>/anam/ledger.db.
func DefaultLedgerPath() string { return dataPath("ledger.db") }

// DefaultDBPath is the tracking database: <data dir>/anam/tracking.db.
func DefaultDBPath() string { return dataPath("tracking.db") }

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
