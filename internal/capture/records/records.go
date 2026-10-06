// Package records is the capture worker for facts written down as records: YAML files
// in a folder, one file per source document, each holding the records extracted from
// it (docs/SELF-LOG.md §3, §5). It is how a pathology PDF, a hand-kept log, or a note
// an agent wrote from email becomes envelopes.
//
// A records file is a source, so it is read in its native format and never written
// here. Its shape:
//
//	document:  {name: ..., sha256: ...}      # the file the records came from
//	extracted: {method: ..., version: ...}   # how; a parser, or a model reading
//	records:
//	  - type: lab.result                     # a namespaced facet type
//	    id: acl:39986323/sodium              # the row's identity, chosen once
//	    ...fields                            # the payload, verbatim
//
// The payload is the record's fields and nothing else. `document` and `extracted` are
// the file's provenance, not the fact's: the same measurement reported by two documents
// (a lab prints last year's column beside this year's) is one fact with one identity,
// and carrying the document into the payload would make the two reports fight over it.
// The records file stays on disk, so which document said what is a grep away.
//
// Identity is definition-owned (§6a). The envelope's source_ref is "records:<id>", and
// the gate recomposes it from the type's identity_key, so a record whose id is not what
// its definition composes is refused rather than admitted under the wrong identity.
package records

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/safepath"
)

// Source is this worker's identity, as the gate's token map and provenance name it.
const Source = "records"

// ErrBadRecords is returned for a records file that cannot be used.
var ErrBadRecords = errors.New("records: bad records file")

// file is one records file as written. Records are kept as yaml nodes so that each
// scalar can be rendered the way it was typed: YAML would otherwise read an id of
// 20240916 as a number and a collected time of 2026-01-30 08:53 as a timestamp, and the
// payload must carry what the record says, not what YAML guessed.
type file struct {
	Document  yaml.Node   `yaml:"document"`
	Extracted yaml.Node   `yaml:"extracted"`
	Records   []yaml.Node `yaml:"records"`
}

// Scan reads every records file under path (a directory, or one file) and returns
// their envelopes in file-name, then record, order. The whole set is read before any
// envelope is returned, and a duplicate id across the set is an error: two records
// claiming one identity in one scan is a mistake in the files, not a supersession.
func Scan(path, assertedPerson string) ([]facet.Envelope, error) {
	paths, err := recordFiles(path)
	if err != nil {
		return nil, err
	}

	var out []facet.Envelope
	seen := map[string]string{}
	for _, p := range paths {
		envs, err := scanFile(p, assertedPerson)
		if err != nil {
			return nil, err
		}
		for _, env := range envs {
			if prev, dup := seen[env.SourceRef]; dup {
				return nil, fmt.Errorf("%w: %s and %s both declare id %q",
					ErrBadRecords, prev, p, strings.TrimPrefix(env.SourceRef, Source+":"))
			}
			seen[env.SourceRef] = p
		}
		out = append(out, envs...)
	}
	return out, nil
}

// recordFiles lists the .yaml and .json files to read, sorted. A directory with none
// is a state (nothing extracted yet), not a fault.
func recordFiles(path string) ([]string, error) {
	expanded, err := safepath.Expand(path)
	if err != nil {
		return nil, fmt.Errorf("records: %w", err)
	}
	info, err := os.Stat(expanded)
	if err != nil {
		return nil, fmt.Errorf("records: %w", err)
	}
	if !info.IsDir() {
		return []string{expanded}, nil
	}
	entries, err := os.ReadDir(expanded)
	if err != nil {
		return nil, fmt.Errorf("records: %w", err)
	}
	var paths []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		switch strings.ToLower(filepath.Ext(e.Name())) {
		case ".yaml", ".yml", ".json":
			paths = append(paths, filepath.Join(expanded, e.Name()))
		}
	}
	sort.Strings(paths)
	return paths, nil
}

func scanFile(path, assertedPerson string) ([]facet.Envelope, error) {
	f, err := safepath.Open(path)
	if err != nil {
		return nil, fmt.Errorf("records: %w", err)
	}
	defer func() { _ = f.Close() }()

	var rf file
	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)
	if err := dec.Decode(&rf); err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrBadRecords, path, err)
	}
	if len(rf.Records) == 0 {
		return nil, fmt.Errorf("%w: %s declares no records", ErrBadRecords, path)
	}

	envs := make([]facet.Envelope, 0, len(rf.Records))
	for i, node := range rf.Records {
		rec, ok := plain(&node).(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%w: %s record %d: not a mapping", ErrBadRecords, path, i+1)
		}
		env, err := envelope(rec, assertedPerson)
		if err != nil {
			return nil, fmt.Errorf("%w: %s record %d: %v", ErrBadRecords, path, i+1, err)
		}
		envs = append(envs, env)
	}
	return envs, nil
}

// envelope turns one record into an envelope. `type` leaves the payload — it is the
// envelope's, not the fact's — and `id` stays in it, because identity_key points at it.
func envelope(rec map[string]any, assertedPerson string) (facet.Envelope, error) {
	typ, _ := rec["type"].(string)
	if !facet.Type(typ).Valid() {
		return facet.Envelope{}, fmt.Errorf("type %q is not a namespaced facet type", typ)
	}
	id, err := scalar(rec["id"])
	if err != nil || id == "" {
		return facet.Envelope{}, fmt.Errorf("id is required and must be a scalar")
	}

	payload := make(map[string]any, len(rec))
	for k, v := range rec {
		if k == "type" {
			continue
		}
		payload[k] = v
	}
	// The id is text, however it was typed: identity is compared as a string, and a
	// numeric id left as a number would be re-rendered by whatever decodes the payload.
	payload["id"] = id
	// Escaping in transit is fine: the gate decodes and canonicalises, so the value
	// bytes that reach the ledger are the record's.
	b, err := json.Marshal(payload)
	if err != nil {
		return facet.Envelope{}, err
	}
	return facet.Envelope{
		Type: facet.Type(typ), Source: Source, Injector: Source,
		SourceRef: Source + ":" + id, Payload: b, AssertedPerson: assertedPerson,
	}, nil
}

// plain converts a yaml node to plain Go values. Strings and timestamps come back as
// the text that was written; numbers, booleans and null as themselves; mappings and
// sequences recursively. A mapping with a repeated key is an error in YAML already.
func plain(n *yaml.Node) any {
	switch n.Kind {
	case yaml.DocumentNode:
		if len(n.Content) == 0 {
			return nil
		}
		return plain(n.Content[0])
	case yaml.MappingNode:
		m := make(map[string]any, len(n.Content)/2)
		for i := 0; i+1 < len(n.Content); i += 2 {
			m[n.Content[i].Value] = plain(n.Content[i+1])
		}
		return m
	case yaml.SequenceNode:
		s := make([]any, len(n.Content))
		for i, c := range n.Content {
			s[i] = plain(c)
		}
		return s
	case yaml.ScalarNode:
		switch n.Tag {
		case "!!int":
			var v int64
			if err := n.Decode(&v); err == nil {
				return v
			}
		case "!!float":
			var v float64
			if err := n.Decode(&v); err == nil {
				return v
			}
		case "!!bool":
			var v bool
			if err := n.Decode(&v); err == nil {
				return v
			}
		case "!!null":
			return nil
		}
		return n.Value
	}
	return nil
}

// scalar renders an id the way the gate will: as the string of a scalar. YAML reads
// 20240916 as a number, so the id is compared as text, which is what the identity
// composition does too.
func scalar(v any) (string, error) {
	switch x := v.(type) {
	case string:
		return x, nil
	case int, int64, float64, bool:
		return fmt.Sprint(x), nil
	case nil:
		return "", nil
	default:
		return "", fmt.Errorf("not a scalar")
	}
}
