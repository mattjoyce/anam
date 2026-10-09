package def_test

import (
	"bytes"
	"io/fs"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
)

// ieeeBase is where the pinned IEEE 1752 schemas say they live; their $refs are
// relative to it, so they are registered there and nothing is fetched.
const ieeeBase = "https://w3id.org/ieee/ieee-1752-schema/"

// A Definition whose source_schema names a pinned standard schema admits only what
// that schema admits: each example payload passes both the Definition and the pinned
// schema. The Definition is closed and the standard is not, so the Definition is the
// stricter of the two, and this test is what keeps it inside the standard.
func TestConformance_ExamplesPassTheDefinitionAndThePinnedSchema(t *testing.T) {
	t.Parallel()
	reg, err := def.Bundled()
	if err != nil {
		t.Fatalf("Bundled: %v", err)
	}
	testdata := os.DirFS("testdata/conformance")
	examples, err := fs.Glob(testdata, "*.json")
	if err != nil || len(examples) == 0 {
		t.Fatalf("no conformance examples: %v", err)
	}
	for _, name := range examples {
		// <facet type>.<example>.json: the type is the first two dotted parts.
		parts := strings.SplitN(name, ".", 3)
		typ := facet.Type(parts[0] + "." + parts[1])
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			d, ok := reg.Lookup(typ)
			if !ok {
				t.Fatalf("no bundled definition for %s", typ)
			}
			raw, err := fs.ReadFile(testdata, name)
			if err != nil {
				t.Fatal(err)
			}
			doc, err := def.Decode(raw)
			if err != nil {
				t.Fatal(err)
			}
			if err := d.Check(doc); err != nil {
				t.Errorf("the Definition refuses its own example: %v", err)
			}
			pinned := pinnedSchema(t, d.SourceSchema)
			if err := pinned.Validate(doc); err != nil {
				t.Errorf("%s refuses the example: %v", d.SourceSchema, err)
			}
		})
	}
}

// pinnedSchema compiles conformance/<standard>/<name>.json with its sibling files
// registered under the standard's own base URL.
func pinnedSchema(t *testing.T, sourceSchema string) *jsonschema.Schema {
	t.Helper()
	standard, name, ok := strings.Cut(sourceSchema, "/")
	if !ok || standard != "ieee-1752" {
		t.Fatalf("source_schema %q names no pinned standard this test knows", sourceSchema)
	}
	dir := os.DirFS(path.Join("conformance", standard))
	files, err := fs.Glob(dir, "*.json")
	if err != nil || len(files) == 0 {
		t.Fatalf("no pinned schemas for %s: %v", standard, err)
	}
	c := jsonschema.NewCompiler()
	c.UseLoader(jsonschema.SchemeURLLoader{})
	for _, f := range files {
		raw, err := fs.ReadFile(dir, f)
		if err != nil {
			t.Fatal(err)
		}
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		if err := c.AddResource(ieeeBase+f, doc); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
	}
	s, err := c.Compile(ieeeBase + name + ".json")
	if err != nil {
		t.Fatalf("compile %s: %v", sourceSchema, err)
	}
	return s
}
