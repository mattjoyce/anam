// Package documents is the capture worker for a folder of files whose bytes stay
// where they are (docs/SELF-LOG.md §5; journal/notes/reference-payloads.md). Each
// regular file becomes one doc.reference envelope whose payload is the native
// reference: the relative path, the file's sha256, its media type and size, and the
// date its sidecar gives it. Anam holds the pointer and never the bytes.
//
// Identity is the relative path, so a renamed file is a new facet. The payload
// carries the file's hash, so a file whose bytes change supersedes. Both are the
// ordinary gate at work; this worker decides neither.
//
// Fact-time comes from outside the bytes: a sidecar `<file>.meta.yaml` beside the
// file, with `date: YYYY-MM-DD` and anything else worth saying about it (title,
// note). A file without one is refused, and the error names the sidecar to write.
package documents

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/safepath"
)

// Source is this worker's identity, as the gate's token map and provenance name it.
const Source = "documents"

// Type is the one facet type this worker emits.
const Type facet.Type = "doc.reference"

// sidecarSuffix is appended to a file's name to find its sidecar.
const sidecarSuffix = ".meta.yaml"

// ErrNoSidecar is returned for a file with no sidecar, which is a file with no
// fact-time.
var ErrNoSidecar = errors.New("documents: file has no sidecar")

// ErrBadSidecar is returned for a sidecar that cannot be used.
var ErrBadSidecar = errors.New("documents: bad sidecar")

// Scan walks the folder and returns one envelope per file, in path order. Sidecars
// are read, not emitted; dot-directories and dot-files are skipped. The whole folder
// is read before any envelope is returned, so a file that cannot be described refuses
// the scan, as a bad record refuses a records scan.
func Scan(root, assertedPerson string) ([]facet.Envelope, error) {
	dir, err := safepath.Expand(root)
	if err != nil {
		return nil, fmt.Errorf("documents: %w", err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("documents: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("documents: %s is not a directory", dir)
	}

	var paths []string
	err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if p != dir && strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(name, ".") || strings.HasSuffix(name, sidecarSuffix) || !d.Type().IsRegular() {
			return nil
		}
		paths = append(paths, p)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("documents: walk %s: %w", dir, err)
	}
	sort.Strings(paths)

	out := make([]facet.Envelope, 0, len(paths))
	for _, p := range paths {
		env, err := describe(dir, p, assertedPerson)
		if err != nil {
			return nil, err
		}
		out = append(out, env)
	}
	return out, nil
}

// describe builds the reference for one file: its sidecar's fields, then the four
// the worker measures, which the sidecar may not override.
func describe(dir, path, assertedPerson string) (facet.Envelope, error) {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return facet.Envelope{}, fmt.Errorf("documents: %w", err)
	}
	rel = filepath.ToSlash(rel)

	payload, err := readSidecar(path)
	if err != nil {
		return facet.Envelope{}, err
	}
	sum, size, err := hashFile(path)
	if err != nil {
		return facet.Envelope{}, err
	}
	payload["path"] = rel
	payload["sha256"] = sum
	payload["bytes"] = size
	payload["media_type"] = mediaType(path)

	b, err := json.Marshal(payload)
	if err != nil {
		return facet.Envelope{}, fmt.Errorf("documents: %s: %w", rel, err)
	}
	return facet.Envelope{
		Type: Type, Source: Source, Injector: Source,
		SourceRef: facet.SourceRef(Source, rel), Payload: b, AssertedPerson: assertedPerson,
	}, nil
}

// readSidecar loads `<file>.meta.yaml` as the payload's starting fields. The date
// is required here as well as at the gate, so the message can say where it goes.
func readSidecar(path string) (map[string]any, error) {
	sidecar := path + sidecarSuffix
	f, err := safepath.Open(sidecar)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s: write %s with `date: YYYY-MM-DD` (the document's own date, not today's)",
				ErrNoSidecar, filepath.Base(path), sidecar)
		}
		return nil, fmt.Errorf("documents: %w", err)
	}
	defer func() { _ = f.Close() }()

	var node yaml.Node
	if err := yaml.NewDecoder(f).Decode(&node); err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrBadSidecar, sidecar, err)
	}
	fields, ok := plain(&node).(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: %s: not a mapping", ErrBadSidecar, sidecar)
	}
	date, _ := fields["date"].(string)
	if date == "" {
		return nil, fmt.Errorf("%w: %s: `date: YYYY-MM-DD` is required", ErrBadSidecar, sidecar)
	}
	for _, k := range []string{"path", "sha256", "bytes", "media_type"} {
		if _, set := fields[k]; set {
			return nil, fmt.Errorf("%w: %s: %s is measured from the file, not declared", ErrBadSidecar, sidecar, k)
		}
	}
	return fields, nil
}

// hashFile streams the file through sha256 and reports its size.
func hashFile(path string) (string, int64, error) {
	f, err := safepath.Open(path)
	if err != nil {
		return "", 0, fmt.Errorf("documents: %w", err)
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, fmt.Errorf("documents: read %s: %w", path, err)
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

// mediaType is by extension, the way a browser or a share would name it; bytes are
// not sniffed, because the type is a label on the pointer, not a verdict on the file.
func mediaType(path string) string {
	if t := mime.TypeByExtension(strings.ToLower(filepath.Ext(path))); t != "" {
		if i := strings.IndexByte(t, ';'); i >= 0 {
			t = t[:i]
		}
		return t
	}
	return "application/octet-stream"
}

// plain converts a yaml node to plain Go values, scalars as the text that was
// written except numbers, booleans and null.
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
