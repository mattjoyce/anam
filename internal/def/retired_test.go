package def_test

import (
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/mattjoyce/anam/internal/def"
)

// retiredHistory is a history body for a type no current manifest names. A history
// body is words, not a contract: only its meta is read.
func retiredHistory(typ string, closes bool) []byte {
	c := "false"
	if closes {
		c = "true"
	}
	return []byte(`{"meta": {"facet_type": "` + typ + `", "definition_version": 1,
  "series": {"name": "test.course", "key": ["/drug"], "closes": ` + c + `}}}`)
}

// copyOf copies a catalog root into a map the test can change.
func copyOf(t *testing.T, root fs.FS) fstest.MapFS {
	t.Helper()
	out := fstest.MapFS{}
	err := fs.WalkDir(root, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		body, err := fs.ReadFile(root, name)
		if err != nil {
			return err
		}
		out[name] = &fstest.MapFile{Data: body}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// Card 255 (R4b): a registry answers Closes for a retired type from the history of the
// catalog it was built over, not from a history the binary carries. A catalog may hold
// a retired type the binary never heard of, and may lack one the binary has.
func TestRegistry_RetiredTypesCloseFromTheirOwnCatalog(t *testing.T) {
	t.Parallel()

	t.Run("a retired type only this catalog's history names still closes", func(t *testing.T) {
		t.Parallel()
		root := copyOf(t, fixtureDefs())
		root["history/test.gone.v1.json"] = &fstest.MapFile{Data: retiredHistory("test.gone", true)}
		root["history/test.gone-open.v1.json"] = &fstest.MapFile{Data: retiredHistory("test.gone-open", false)}
		reg, err := def.BundledFS(root)
		if err != nil {
			t.Fatal(err)
		}
		if !reg.Closes("test.gone") {
			t.Error("test.gone closes = false, want true: its newest history version closes the series")
		}
		if reg.Closes("test.gone-open") {
			t.Error("test.gone-open closes = true, want false")
		}
		full, err := def.BundledFS(fixtureDefs())
		if err != nil {
			t.Fatal(err)
		}
		if full.Closes("test.gone") {
			t.Error("test.gone closes = true over the fixture, whose history does not name it")
		}
	})

	t.Run("a retired type this catalog's history lacks closes nothing", func(t *testing.T) {
		t.Parallel()
		root := copyOf(t, fixtureDefs())
		var removed int
		for name := range root {
			if strings.HasPrefix(name, "history/test.retired.v") {
				delete(root, name)
				removed++
			}
		}
		if removed == 0 {
			t.Fatal("the fixture has no test.retired history to remove")
		}
		reg, err := def.BundledFS(root)
		if err != nil {
			t.Fatal(err)
		}
		if reg.Closes("test.retired") {
			t.Error("test.retired closes = true over a catalog whose history does not name it")
		}
		full, err := def.BundledFS(fixtureDefs())
		if err != nil {
			t.Fatal(err)
		}
		if !full.Closes("test.retired") {
			t.Error("test.retired closes = false over the fixture, whose history closes it")
		}
	})

	t.Run("a registry built with no history has no retired types", func(t *testing.T) {
		t.Parallel()
		reg, err := def.NewRegistry()
		if err != nil {
			t.Fatal(err)
		}
		if reg.Closes("test.retired") {
			t.Error("test.retired closes = true over no history")
		}
	})

	t.Run("a registry with registered Definitions keeps its catalog's retired types", func(t *testing.T) {
		t.Parallel()
		root := copyOf(t, fixtureDefs())
		root["history/test.gone.v1.json"] = &fstest.MapFile{Data: retiredHistory("test.gone", true)}
		bundled, err := def.BundledFS(root)
		if err != nil {
			t.Fatal(err)
		}
		reg, err := def.WithRegistered(bundled, []def.Stored{held(importedManifest, 1)})
		if err != nil {
			t.Fatal(err)
		}
		if !reg.Closes("test.gone") || !reg.Closes("test.retired") {
			t.Error("WithRegistered dropped the catalog's retired types")
		}
	})
}
