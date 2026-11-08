package config

import (
	"context"
	"errors"
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
// the usable configuration. Provenance reports each top-level origin by its
// snake_case key; ProvenanceAt also exposes nested origins in the composed tree.
//
// Provenance holds a key only when the key is live and was resolved. An unknown key
// is absent, a key with no default and no value (subject_id) is absent, and the old
// names ingest_timeout and person_id are absent: their origins are reported under
// long_write_timeout and subject_id instead.
type Resolved struct {
	value      Config
	root       ashlar.Value
	provenance map[string]ashlar.Provenance
}

// Value returns the resolved Config value.
func (r Resolved) Value() Config { return r.value }

// Provenance returns the provenance for key, or false when the key is unknown,
// absent, optional without a value, or one of the old names ingest_timeout and
// person_id.
func (r Resolved) Provenance(key string) (ashlar.Provenance, bool) {
	v, ok := r.provenance[key]
	return v, ok
}

// ProvenanceAt returns the provenance at an effective JSON pointer, including nested
// fields. It retains source coordinates and the include chain through validation.
func (r Resolved) ProvenanceAt(pointer ashlar.Pointer) (ashlar.Provenance, bool) {
	value := r.root.At(pointer)
	if value.IsOmitted() {
		return ashlar.Provenance{}, false
	}
	return value.Provenance(), true
}

func (r Resolved) validationError(err error) error {
	var diagnostic *ashlar.Error
	if !errors.As(err, &diagnostic) {
		return err
	}
	located := *diagnostic
	if p, ok := r.ProvenanceAt(diagnostic.Pointer); ok {
		located.Origin = p.Effective
	} else if p, ok := r.provenance[strings.TrimPrefix(string(diagnostic.Pointer), "/")]; ok {
		// Retired keys have been adopted under their canonical names.
		located.Origin = p.Effective
	} else if p, ok := r.containingProvenance(diagnostic.Pointer); ok {
		located.Origin = p.Effective
	} else if diagnostic.Origin.String() == "" {
		// No file set this key (a default), so a bare pointer prefix would point at
		// nothing. Keep the error as it reads unlocated.
		return err
	}
	return &located
}

// LoadResolved resolves configuration, reading the YAML include graph at path when
// non-empty. It is Load with the provenance Load discards.
func LoadResolved(path string) (Resolved, error) { return LoadResolvedOverriding(path, nil) }

// LoadResolvedOverriding is LoadResolved with set taking precedence over everything
// else, keyed by config key.
func LoadResolvedOverriding(path string, set map[string]string) (Resolved, error) {
	r, err := resolve(path, set)
	if err != nil {
		return Resolved{}, err
	}
	if err := r.value.Validate(); err != nil {
		return Resolved{}, r.validationError(err)
	}
	return r, nil
}

// CatalogDirs is the definitions_dir and projections_dir the configuration at path
// resolves to, by the same layers, strict decoding and path expansion as
// LoadResolved, without validating the settings a caller that wants only the catalog
// never reads. It is for anam-bench, which takes the catalog from the anam config and
// everything else from its own flags.
func CatalogDirs(path string) (definitionsDir, projectionsDir string, err error) {
	r, err := resolve(path, nil)
	if err != nil {
		return "", "", err
	}
	return r.value.DefinitionsDir, r.value.ProjectionsDir, nil
}

// resolve composes and decodes the configuration at path with set on top, and expands
// its paths: LoadResolvedOverriding but for Validate.
func resolve(path string, set map[string]string) (Resolved, error) {
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

	r := Resolved{root: root, provenance: configProvenance(root)}
	if err := validateInspectorNodes(root); err != nil {
		return Resolved{}, r.validationError(err)
	}
	var c Config
	if err := yamlconfig.DecodeValueStrict(root, &c); err != nil {
		return Resolved{}, fmt.Errorf("decode config: %w: %w", ErrInvalid, err)
	}
	if err := c.CompilePresentation(); err != nil {
		return Resolved{}, r.validationError(err)
	}
	if err := c.adoptLongWriteTimeout(hasKey(root, "long_write_timeout"), hasKey(root, "ingest_timeout")); err != nil {
		return Resolved{}, r.validationError(err)
	}
	if err := c.adoptSubjectID(hasKey(root, "subject_id"), hasKey(root, "person_id")); err != nil {
		return Resolved{}, r.validationError(err)
	}

	derivedLibraryDir := c.LibraryDir == ""
	if derivedLibraryDir {
		c.LibraryDir = filepath.Join(filepath.Dir(c.LedgerPath), "documents")
	}
	if err := expandPaths(&c); err != nil {
		return Resolved{}, r.validationError(err)
	}

	provenance := r.provenance
	if derivedLibraryDir {
		provenance["library_dir"] = ashlar.NewProvenance(ashlar.DefaultOrigin("library_dir beside ledger_path"), false)
	}
	return Resolved{value: c, root: root, provenance: provenance}, nil
}

// readDocument parses one strict document, or reports it omitted when the file holds
// nothing to read. Both readers here (config and importers) use it, so both refuse
// aliases and a second document in the same way.
func readDocument(path string, raw []byte, via ...ashlar.SourceSpan) (ashlar.Value, error) {
	if isBlankYAML(raw) {
		return ashlar.OmittedValue(), nil
	}
	doc, err := yamlconfig.Read(path, raw, via...)
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

// defaultSettings are Anam's shipped defaults. subject_id has none on purpose: it is
// pinned into the ledger, so a default would name a subject nobody chose. Neither has
// its old name person_id, so its presence says whether it was given. The two names of
// the long-write timeout also have none, for the same reason; adoptSubjectID and
// adoptLongWriteTimeout settle the values after both names are decoded.
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
		textDefault("definitions_dir", DefaultDefinitionsDir),
		textDefault("projections_dir", DefaultProjectionsDir),
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
	keys := make([]string, 0, len(settings)+4)
	for _, setting := range settings {
		keys = append(keys, setting.key)
	}
	return append(keys, "subject_id", "person_id", "long_write_timeout", "ingest_timeout")
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

// configProvenance reports where each live top-level key came from. An old name is
// never a key of its own: when ingest_timeout supplied the long-write timeout, its
// origin is the timeout's origin, and when person_id supplied the subject, its origin
// is subject_id's.
func configProvenance(root ashlar.Value) map[string]ashlar.Provenance {
	provenance := make(map[string]ashlar.Provenance, root.Len())
	for _, member := range root.Members() {
		if member.Key == "ingest_timeout" || member.Key == "person_id" {
			continue
		}
		provenance[member.Key] = member.Value.Provenance()
	}
	if !hasKey(root, "subject_id") {
		if value, ok := root.Lookup("person_id"); ok {
			provenance["subject_id"] = value.Provenance()
		}
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
	if c.DefinitionsDir != "" {
		paths["definitions_dir"] = &c.DefinitionsDir
	}
	if c.ProjectionsDir != "" {
		paths["projections_dir"] = &c.ProjectionsDir
	}
	for name, p := range paths {
		expanded, err := safepath.Expand(*p)
		if err != nil {
			return invalidSetting(name, "%s: %v", name, err)
		}
		*p = expanded
	}
	return nil
}

func (r Resolved) containingProvenance(pointer ashlar.Pointer) (ashlar.Provenance, bool) {
	// Missing inspector fields are located at their containing sequence entry.
	// Existing scalar setting diagnostics keep their established fallback behavior.
	if !strings.HasPrefix(string(pointer), "/inspector/") {
		return ashlar.Provenance{}, false
	}
	path := string(pointer)
	for strings.Contains(path, "/") {
		path = path[:strings.LastIndex(path, "/")]
		if p, ok := r.ProvenanceAt(ashlar.Pointer(path)); ok {
			return p, true
		}
	}
	return ashlar.Provenance{}, false
}
