package daemon

import (
	"io/fs"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/mattjoyce/anam/internal/catalog/catalogtest"
	"github.com/mattjoyce/anam/internal/catalog/fixture"
	"github.com/mattjoyce/anam/internal/config"
)

// doseType is the type the Workbench tests enter by hand. Engine's own types have no
// schema example, no bound and no optional number, so the form drawn from them would
// test less than the form does; doseFixture adds one type that has all three.
const doseType = "test.dose"

// doseManifest is test.dose: a thing taken at a moment, its id composed from the
// source, the name and the minute with its offset, its wall clock read in
// Europe/Berlin or at the offset source_time names. It is nobody's record.
const doseManifest = `{
  "meta": {
    "facet_type": "test.dose",
    "definition_version": 1,
    "kind": "event",
    "family": "doses",
    "classification": "Sensitive",
    "validation_mode": "strict",
    "source_schema": "fixture/test.dose",
    "time_semantics": {"fact_time": {"pointer": "/taken_at", "layout": "2006-01-02 15:04", "zone": "Europe/Berlin", "zone_pointer": "/source_time/offset"}},
    "identity_key": ["/id"],
    "identity_template": "{source}:dose/{/name|slug}/{/taken_at|2006-01-02T15:04}{/source_time/offset?}",
    "min_binding_assurance": "medium"
  },
  "handling": {
    "guidance": "One dose, taken at a moment: a fact that an intake happened. Leave id out and Anam fills it; send it and it must match.",
    "label": "{/name} {/amount} {/unit}"
  },
  "schema": {
    "$schema": "https://json-schema.org/draft/2020-12/schema",
    "description": "One dose of something, taken at a moment. It is nobody's record.",
    "type": "object",
    "required": ["id", "taken_at", "name", "amount", "unit", "source_time"],
    "properties": {
      "source_time": {
        "type": "object",
        "required": ["value", "zone", "offset", "basis"],
        "additionalProperties": false,
        "properties": {
          "value": {"type": "string", "minLength": 1},
          "zone": {"type": "string", "minLength": 1, "pattern": "^([A-Za-z][A-Za-z0-9_+-]*(/[A-Za-z0-9_+-]+)*|[+-]([01][0-9]|2[0-3]):[0-5][0-9])$"},
          "offset": {"type": "string", "pattern": "^[+-]([01][0-9]|2[0-3]):[0-5][0-9]$"},
          "basis": {"type": "string", "enum": ["source", "configuration", "assumption"]}
        }
      },
      "id": {"type": "string", "minLength": 1},
      "taken_at": {"type": "string", "minLength": 1},
      "name": {"type": "string", "minLength": 1, "examples": ["melatonin"]},
      "amount": {"type": "number", "exclusiveMinimum": 0, "examples": [5]},
      "unit": {"type": "string", "minLength": 1, "examples": ["mg"]},
      "course": {"type": "number", "examples": [1]},
      "reason": {"type": "string", "minLength": 1, "examples": ["for sleep"]},
      "source_note": {"type": "string", "minLength": 1}
    },
    "additionalProperties": false
  }
}`

// doseRoots is fixture.Engine with test.dose beside its definitions, built once.
var doseRoots = sync.OnceValues(func() (fs.FS, fs.FS) {
	defs, projs := fixture.Engine()
	withDose := fstest.MapFS{doseType + ".json": {Data: []byte(doseManifest)}}
	err := fs.WalkDir(defs, ".", func(path string, e fs.DirEntry, err error) error {
		if err != nil || e.IsDir() {
			return err
		}
		raw, err := fs.ReadFile(defs, path)
		withDose[path] = &fstest.MapFile{Data: raw}
		return err
	})
	if err != nil {
		panic("daemon tests: copy fixture.Engine: " + err.Error()) // the embedded fixture always reads
	}
	return withDose, projs
})

// doseFixture is Engine and test.dose, for the Workbench tests.
var doseFixture = catalogtest.Over(doseRoots)

// withDose points cfg at a copy of doseFixture under the test's own directory.
func withDose(t *testing.T, cfg *config.Config) {
	t.Helper()
	doseFixture.Config(t, cfg)
}
