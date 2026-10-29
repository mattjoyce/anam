package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/mattjoyce/ashlar"
	"github.com/mattjoyce/ashlar/yamlconfig"

	"github.com/mattjoyce/anam/internal/safepath"
	"github.com/mattjoyce/anam/internal/span"
)

// Resolved is Anam's configuration with the composition that produced it. Config is
// the usable configuration; Value is the layered Ashlar document it was decoded from;
// Provenance is where each top-level key came from, keyed by its snake_case config key.
//
// Provenance holds a key only when the key is live and was resolved. An unknown key
// is absent, a key with no default and no value (person_id) is absent, and the retired
// ingest_timeout is absent: its origin is reported under long_write_timeout instead.
type Resolved struct {
	value      Config
	provenance map[string]ashlar.Provenance
}

// Value returns the resolved Config value.
func (r Resolved) Value() Config { return r.value }

// Provenance returns the provenance for key, or false when the key is unknown,
// absent, optional without a value, or the retired ingest_timeout field.
func (r Resolved) Provenance(key string) (ashlar.Provenance, bool) {
	v, ok := r.provenance[key]
	return v, ok
}

// LoadResolved resolves configuration, reading the YAML file at path when it is
// non-empty. It is Load with the provenance Load discards.
func LoadResolved(path string) (Resolved, error) { return LoadResolvedOverriding(path, nil) }

// LoadResolvedOverriding is LoadResolved with set taking precedence over everything
// else, keyed by config key.
func LoadResolvedOverriding(path string, set map[string]string) (Resolved, error) {
	fragments, err := readConfigFragments(path)
	if err != nil {
		return Resolved{}, err
	}

	// Environment before explicit, each in key order: the explicit layer is how a
	// command-line flag wins, and a sorted layer keeps the composition deterministic.
	overrides := environmentOverrides()
	overrides = append(overrides, explicitOverrides(set)...)

	resolved, err := yamlconfig.ResolveFragments(context.Background(), fragments, yamlconfig.ResolveOptions{
		Defaults: defaultOverrides(),
		Policy:   yamlconfig.DefaultCompositionPolicy(),
		Explicit: overrides,
	})
	if err != nil {
		return Resolved{}, fmt.Errorf("decode config: %w: %w", ErrInvalid, err)
	}
	root, err := adaptIntegerOverrides(resolved.Root())
	if err != nil {
		return Resolved{}, fmt.Errorf("decode config: %w: %w", ErrInvalid, err)
	}

	var c Config
	if err := yamlconfig.DecodeValueStrict(root, &c); err != nil {
		return Resolved{}, fmt.Errorf("decode config: %w: %w", ErrInvalid, err)
	}
	if err := c.adoptLongWriteTimeout(hasKey(root, "long_write_timeout"), hasKey(root, "ingest_timeout")); err != nil {
		return Resolved{}, err
	}

	derivedLibraryDir := c.LibraryDir == ""
	if derivedLibraryDir {
		c.LibraryDir = filepath.Join(filepath.Dir(c.LedgerPath), "documents")
	}
	if err := expandPaths(&c); err != nil {
		return Resolved{}, err
	}
	if err := c.Validate(); err != nil {
		return Resolved{}, err
	}

	provenance := configProvenance(root)
	if derivedLibraryDir {
		provenance["library_dir"] = ashlar.NewProvenance(ashlar.DefaultOrigin("library_dir beside ledger_path"), false)
	}
	return Resolved{value: c, provenance: provenance}, nil
}

// readConfigFragments reads path as one strict YAML document. A missing path, an empty
// file, a comment-only file and a whole-document null are all "no file": Anam resolves
// its defaults, as it always has. A null member is dropped so the layer beneath it
// wins, which is what Viper's decode did before the composition existed.
func readConfigFragments(path string) ([]yamlconfig.Fragment, error) {
	if path == "" {
		return nil, nil
	}
	raw, err := os.ReadFile(path) // #nosec G304 -- path is Resolve-pinned or empty / caller-provided
	if err != nil {
		return nil, fmt.Errorf("read config %q: %w", path, err)
	}
	doc, err := readDocument(path, raw)
	if err != nil {
		return nil, fmt.Errorf("read config %q: %w", path, err)
	}
	if doc.IsOmitted() {
		return nil, nil
	}
	return []yamlconfig.Fragment{{Path: path, Raw: raw, Root: doc}}, nil
}

// readDocument parses one strict document, or reports it omitted when the file holds
// nothing to read. Both readers here (config and importers) use it, so both refuse
// aliases and a second document in the same way.
func readDocument(path string, raw []byte) (ashlar.Value, error) {
	if isBlankYAML(raw) {
		return ashlar.OmittedValue(), nil
	}
	doc, err := yamlconfig.Read(path, raw)
	if err != nil {
		return ashlar.Value{}, err
	}
	if doc.IsNull() {
		return ashlar.OmittedValue(), nil
	}
	return withoutNullMembers(doc), nil
}

// isBlankYAML reports whether raw holds no content: only whitespace and comments.
// yamlconfig.Read calls an empty document an error, but an operator's empty file has
// always meant defaults, so it is answered before the strict reader sees it.
func isBlankYAML(raw []byte) bool {
	for _, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			return false
		}
	}
	return true
}

// withoutNullMembers drops top-level nulls so they do not replace a default. A null
// nested in a value still reaches the decoder, where it means what its type says.
func withoutNullMembers(doc ashlar.Value) ashlar.Value {
	if doc.Kind() != ashlar.KindObject {
		return doc
	}
	members := doc.Members()
	kept := make([]ashlar.Member, 0, len(members))
	for _, m := range members {
		if !m.Value.IsNull() {
			kept = append(kept, m)
		}
	}
	if len(kept) == len(members) {
		return doc
	}
	stripped, err := ashlar.ObjectValue(kept, doc.Provenance())
	if err != nil {
		// Duplicate keys are impossible here: the members came from an object whose
		// keys ObjectValue already checked. Returning the document unchanged keeps the
		// original diagnostic rather than inventing one.
		return doc
	}
	return stripped
}

// defaultSetting is one key's default, before it is stamped with a default origin.
type defaultSetting struct {
	key   string
	value ashlar.Value
}

func textDefault(key, text string) defaultSetting {
	return defaultSetting{key: key, value: ashlar.StringValue(text, ashlar.Provenance{})}
}

func numberDefault(key string, n int) defaultSetting {
	value, _ := ashlar.NumberValue(strconv.Itoa(n), ashlar.Provenance{}) // Itoa is always a number lexeme.
	return defaultSetting{key: key, value: value}
}

// defaultSettings are Anam's shipped defaults. person_id has none on purpose: it is
// pinned into the ledger, so a default would name a person nobody chose. The two names
// of the long-write timeout also have none, so their presence says whether they were
// given; adoptLongWriteTimeout settles the value after both are decoded.
func defaultSettings() []defaultSetting {
	settings := []defaultSetting{
		textDefault("ledger_path", DefaultLedgerPath()),
		textDefault("db_path", DefaultDBPath()),
		textDefault("timeout", DefaultTimeout.String()),
		textDefault("bucket_width", string(span.Default)),
		textDefault("library_dir", ""),
		textDefault("zone", "Local"),
		textDefault("listen", DefaultListen),
		textDefault("token_file", ""),
		textDefault("importers_file", ""),
		textDefault("server", ""),
		numberDefault("checkpoint_budget", DefaultCheckpointBudget),
		textDefault("backup_dir", ""),
		numberDefault("backup_keep", DefaultBackupKeep),
		numberDefault("max_read_bytes", DefaultMaxReadBytes),
		textDefault("default_projection", DefaultDefaultProjection),
		numberDefault("library_folder_limit", DefaultLibraryFolderLimit),
	}
	return append(settings, limitDefaults()...)
}

func defaultOverrides() []yamlconfig.Override {
	settings := defaultSettings()
	out := make([]yamlconfig.Override, 0, len(settings))
	for _, setting := range settings {
		out = append(out, yamlconfig.DefaultSetting(ashlar.NewPointer(setting.key), setting.key, setting.value))
	}
	return out
}

// environmentOverrides binds every live key to its ANAM_ variable. An empty variable is
// not set, as Viper treated it: it leaves the layer beneath it in place.
func environmentOverrides() []yamlconfig.Override {
	keys := configKeys()
	sort.Strings(keys)
	out := make([]yamlconfig.Override, 0, len(keys))
	for _, key := range keys {
		name := "ANAM_" + strings.ToUpper(key)
		raw, ok := os.LookupEnv(name)
		if !ok || raw == "" {
			continue
		}
		out = append(out, yamlconfig.Override{
			Pointer: ashlar.NewPointer(key),
			Value:   ashlar.StringValue(raw, ashlar.NewProvenance(ashlar.EnvOrigin(name), false)),
		})
	}
	return out
}

// explicitOverrides is the caller's layer, a flag or a test. An empty value is still
// present here: it was chosen, unlike an empty environment variable.
func explicitOverrides(set map[string]string) []yamlconfig.Override {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]yamlconfig.Override, 0, len(keys))
	for _, key := range keys {
		out = append(out, yamlconfig.ExplicitSetting(
			ashlar.NewPointer(key), key, ashlar.StringValue(set[key], ashlar.Provenance{})))
	}
	return out
}

// configKeys is every key Anam reads: the defaults plus the keys that are given only
// by a layer above them.
func configKeys() []string {
	settings := defaultSettings()
	keys := make([]string, 0, len(settings)+3)
	for _, setting := range settings {
		keys = append(keys, setting.key)
	}
	return append(keys, "person_id", "long_write_timeout", "ingest_timeout")
}

// integerKeys are the config keys decoded as an int. A YAML value for one must be a
// number; an environment or explicit value is text, so it is converted after
// composition, once its origin says it is text the operator typed rather than YAML.
var integerKeys = map[string]bool{
	"backup_keep":          true,
	"max_read_bytes":       true,
	"checkpoint_budget":    true,
	"library_folder_limit": true,
	"max_payload_bytes":    true,
	"commit_retries":       true,
	"max_manifest_bytes":   true,
	"max_document_bytes":   true,
	"snapshot_keep":        true,
	"max_snapshot_parts":   true,
	"max_absent_ids":       true,
	"max_absent_judged":    true,
	"max_response_bytes":   true,
	"inspect_cached_reads": true,
}

// adaptIntegerOverrides turns an environment or explicit integer from its text into a
// number. A quoted integer in the file stays a string and is refused by the strict
// decoder, so the two layers mean what they say.
func adaptIntegerOverrides(root ashlar.Value) (ashlar.Value, error) {
	if root.Kind() != ashlar.KindObject {
		return root, nil
	}
	members := root.Members()
	changed := false
	for i, member := range members {
		if !integerKeys[member.Key] || member.Value.Kind() != ashlar.KindString {
			continue
		}
		switch member.Value.Provenance().Effective.Kind {
		case ashlar.OriginEnv, ashlar.OriginExplicit:
		default:
			continue
		}
		raw, _ := member.Value.StringVal()
		n, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		if err != nil {
			return ashlar.Value{}, fmt.Errorf("%s: %w", member.Key, err)
		}
		number, err := ashlar.NumberValue(strconv.FormatInt(n, 10), member.Value.Provenance())
		if err != nil {
			return ashlar.Value{}, fmt.Errorf("%s: %w", member.Key, err)
		}
		members[i].Value = number
		changed = true
	}
	if !changed {
		return root, nil
	}
	adapted, err := ashlar.ObjectValue(members, root.Provenance())
	if err != nil {
		return ashlar.Value{}, err
	}
	return adapted, nil
}

func hasKey(root ashlar.Value, key string) bool {
	_, ok := root.Lookup(key)
	return ok
}

// configProvenance reports where each live top-level key came from. The retired name is
// never a key of its own: when it supplied the long-write timeout, its origin is the
// timeout's origin.
func configProvenance(root ashlar.Value) map[string]ashlar.Provenance {
	provenance := make(map[string]ashlar.Provenance, root.Len())
	for _, member := range root.Members() {
		if member.Key == "ingest_timeout" {
			continue
		}
		provenance[member.Key] = member.Value.Provenance()
	}
	switch {
	case hasKey(root, "long_write_timeout"):
		value, _ := root.Lookup("long_write_timeout")
		provenance["long_write_timeout"] = value.Provenance()
	case hasKey(root, "ingest_timeout"):
		value, _ := root.Lookup("ingest_timeout")
		provenance["long_write_timeout"] = value.Provenance()
	default:
		provenance["long_write_timeout"] = ashlar.NewProvenance(ashlar.DefaultOrigin("long_write_timeout"), false)
	}
	return provenance
}

// expandPaths expands the operator's filesystem paths once, here, so every consumer
// sees the same absolute path the operator meant.
func expandPaths(c *Config) error {
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
			return fmt.Errorf("%w: %s: %v", ErrInvalid, name, err)
		}
		*p = expanded
	}
	return nil
}
