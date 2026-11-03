// Package fixture holds the catalogs the engine's tests read (card 255, R4a). A test
// that needs a catalog names one here instead of falling back on the catalog
// embedded in the binary.
//
// Each catalog is two roots laid out as the daemon reads them: definitions (*.json,
// history/*.json) and projections (*.json, history/*.json). The package imports
// nothing from the engine, so the def and projection packages' own tests can use it
// without an import cycle; catalogtest builds registries and catalogs over it.
//
// Nothing outside a test imports it. internal/catalog/fixture/boundary_test.go and
// scripts/check.sh both fail on a production file that does.
package fixture

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

//go:embed testdata/mattjoyce/definitions testdata/mattjoyce/projections
//go:embed testdata/minimal/definitions testdata/minimal/projections
//go:embed testdata/engine/definitions testdata/engine/projections
var files embed.FS

// Mattjoyce is the catalog the engine embedded at de2981c, byte for byte: a copy of
// internal/def/manifests and internal/projection/manifests. It is the personal
// catalog, kept while tests move off it, and its definition set version is the one
// the binary reports. Do not reformat, sort or prune it.
func Mattjoyce() (definitions, projections fs.FS) {
	return pair("mattjoyce")
}

// Minimal is the smallest catalog the engine's own consumers need: record.edge and
// record.document read in UTC, one measure type (test.measure) and one projection
// over it (measures). It is nobody's record.
func Minimal() (definitions, projections fs.FS) {
	return pair("minimal")
}

// Engine is the catalog the engine's behaviour tests read (card 255, R4b): one type
// or template per engine feature, each with the history its tests need, under the
// test. head and record.*. It is nobody's record. What each file is there for is in
// engine_test.go, which pins the layout and the features; catalogtest's tests pin its
// definition set version.
//
// Definitions: record.edge and record.document, Minimal's UTC copies; test.reading,
// a reading placed by the offset source_time names (v2) or in Europe/Berlin (v1);
// test.measure, keyed by two fields, source_time required from v2; test.templated,
// an identity template that gains an optional offset in v2; test.labelled, a label
// whose fields may be absent; test.course.started, .changed and .stopped, one series
// the stop closes; and test.retired, only in history, the stop's earlier name, which
// closes the series too. Projections: measures (a log of one type), course (a
// snapshot over the series, whose v1 named test.retired), everything (a log of every
// type, the series by its prefix) and normal (the same types under a Normal ceiling).
func Engine() (definitions, projections fs.FS) {
	return pair("engine")
}

// pair returns a named catalog's two roots.
func pair(name string) (definitions, projections fs.FS) {
	return sub("testdata/" + name + "/definitions"), sub("testdata/" + name + "/projections")
}

// sub is files rooted at dir, which the embed directives above name.
func sub(dir string) fs.FS {
	root, err := fs.Sub(files, dir)
	if err != nil {
		panic("fixture: cannot sub " + dir + ": " + err.Error())
	}
	return root
}

// Dirs writes a catalog under t.TempDir() and returns its two directories, for
// anything that takes the definitions_dir and projections_dir paths.
func Dirs(t testing.TB, definitions, projections fs.FS) (defsDir, projsDir string) {
	t.Helper()
	defsDir, projsDir, err := Write(t.TempDir(), definitions, projections)
	if err != nil {
		t.Fatal(err)
	}
	return defsDir, projsDir
}

// Write copies a catalog into dir/definitions and dir/projections and returns the
// two directories. Dirs is Write into a test's own directory; Write is for a catalog
// built once per process, outside any one test.
func Write(dir string, definitions, projections fs.FS) (defsDir, projsDir string, err error) {
	defsDir, projsDir = filepath.Join(dir, "definitions"), filepath.Join(dir, "projections")
	if err := copyFS(defsDir, definitions); err != nil {
		return "", "", err
	}
	if err := copyFS(projsDir, projections); err != nil {
		return "", "", err
	}
	return defsDir, projsDir, nil
}

// copyFS writes every directory and regular file of src under dst.
func copyFS(dst string, src fs.FS) error {
	err := fs.WalkDir(src, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(dst, filepath.FromSlash(name))
		if d.IsDir() {
			return os.MkdirAll(target, 0o750)
		}
		body, err := fs.ReadFile(src, name)
		if err != nil {
			return err
		}
		return os.WriteFile(target, body, 0o600)
	})
	if err != nil {
		return fmt.Errorf("fixture: write %s: %w", dst, err)
	}
	return nil
}
