package inspect

import (
	"io/fs"
	"strings"
	"testing/fstest"

	"github.com/mattjoyce/anam/internal/catalog/catalogtest"
	"github.com/mattjoyce/anam/internal/catalog/fixture"
)

// The inspector's tests read the engine fixture (catalogtest.Engine). A few pages need
// a feature the engine fixture does not carry, so they read a copy of it with that
// feature added, built once per process like Engine itself.

// describedManifest is a type whose fields say what they mean, as the engine
// fixture's do not: two of them carry a description, one figure is written as text
// (the schema's numeric pattern on a string, which the Definition page names "number
// as text"), and two figures are nested objects of a value and a unit, so a facet page
// lists a changed leaf by its pointer.
const describedManifest = `{
  "meta": {
    "facet_type": "test.described",
    "definition_version": 1,
    "kind": "reading",
    "family": "readings",
    "classification": "Normal",
    "validation_mode": "strict",
    "source_schema": "fixture/test.described",
    "time_semantics": {"fact_time": {"pointer": "/at", "layout": "2006-01-02 15:04", "zone": "UTC"}},
    "identity_key": ["/at"]
  },
  "schema": {
    "type": "object",
    "required": ["at", "figure", "high", "low"],
    "properties": {
      "at": {"type": "string", "minLength": 1},
      "figure": {"type": "string", "minLength": 1, "pattern": "` + numericTextJSON + `", "description": "The figure as the source wrote it."},
      "high": {"type": "object", "required": ["value", "unit"], "additionalProperties": false, "properties": {
        "value": {"type": "number", "description": "The higher of the two figures."},
        "unit": {"type": "string", "minLength": 1}}},
      "low": {"type": "object", "required": ["value", "unit"], "additionalProperties": false, "properties": {
        "value": {"type": "number"},
        "unit": {"type": "string", "minLength": 1}}},
      "comment": {"type": "string", "minLength": 1}
    },
    "additionalProperties": false
  }
}`

// attendedManifest is a type entered by hand whose form shows a bounded whole number:
// its fact time reads the zone the fact names, the gate composes its id, intensity is
// a whole number from 1 to 5, the topic is required text and the note optional.
const attendedManifest = `{
  "meta": {
    "facet_type": "test.attended",
    "definition_version": 1,
    "kind": "event",
    "family": "entered",
    "classification": "Normal",
    "validation_mode": "strict",
    "source_schema": "fixture/test.attended",
    "time_semantics": {"fact_time": {"pointer": "/at", "layout": "2006-01-02 15:04", "zone": "Europe/Berlin", "zone_pointer": "/source_time/offset"}},
    "identity_key": ["/id"],
    "identity_template": "{source}:attended/{/topic|slug}/{/at|2006-01-02T15:04}{/source_time/offset?}"
  },
  "schema": {
    "type": "object",
    "required": ["id", "at", "topic", "intensity", "source_time"],
    "properties": {
      "source_time": {
        "type": "object", "required": ["value", "zone", "offset", "basis"], "additionalProperties": false,
        "properties": {
          "value": {"type": "string", "minLength": 1},
          "zone": {"type": "string", "minLength": 1, "pattern": "^([A-Za-z][A-Za-z0-9_+-]*(/[A-Za-z0-9_+-]+)*|[+-]([01][0-9]|2[0-3]):[0-5][0-9])$"},
          "offset": {"type": "string", "pattern": "^[+-]([01][0-9]|2[0-3]):[0-5][0-9]$"},
          "basis": {"type": "string", "enum": ["source", "configuration", "assumption"]}}},
      "id": {"type": "string", "minLength": 1},
      "at": {"type": "string", "minLength": 1},
      "topic": {"type": "string", "minLength": 1},
      "intensity": {"type": "integer", "minimum": 1, "maximum": 5},
      "note": {"type": "string", "minLength": 1}
    },
    "additionalProperties": false
  }
}`

// takenManifest is a type entered by hand whose form shows choices: the kind and the
// part of the day are closed lists, two answers are yes or no, the count a whole
// number from 1, and every field but the kind and the time is optional.
const takenManifest = `{
  "meta": {
    "facet_type": "test.taken",
    "definition_version": 1,
    "kind": "event",
    "family": "entered",
    "classification": "Normal",
    "validation_mode": "strict",
    "source_schema": "fixture/test.taken",
    "time_semantics": {"fact_time": {"pointer": "/at", "layout": "2006-01-02 15:04", "zone": "Europe/Berlin", "zone_pointer": "/source_time/offset"}},
    "identity_key": ["/id"],
    "identity_template": "{source}:taken/{/kind}/{/at|2006-01-02T15:04}{/source_time/offset?}"
  },
  "schema": {
    "type": "object",
    "required": ["id", "at", "kind", "source_time"],
    "properties": {
      "source_time": {
        "type": "object", "required": ["value", "zone", "offset", "basis"], "additionalProperties": false,
        "properties": {
          "value": {"type": "string", "minLength": 1},
          "zone": {"type": "string", "minLength": 1, "pattern": "^([A-Za-z][A-Za-z0-9_+-]*(/[A-Za-z0-9_+-]+)*|[+-]([01][0-9]|2[0-3]):[0-5][0-9])$"},
          "offset": {"type": "string", "pattern": "^[+-]([01][0-9]|2[0-3]):[0-5][0-9]$"},
          "basis": {"type": "string", "enum": ["source", "configuration", "assumption"]}}},
      "id": {"type": "string", "minLength": 1},
      "at": {"type": "string", "minLength": 1},
      "kind": {"type": "string", "enum": ["drink", "meal"]},
      "part_of_day": {"type": "string", "enum": ["morning", "evening"]},
      "drink": {"type": "string", "minLength": 1},
      "count": {"type": "integer", "minimum": 1},
      "abv": {"type": "number"},
      "text": {"type": "string", "minLength": 1},
      "rich": {"type": "boolean"},
      "out": {"type": "boolean"}
    },
    "additionalProperties": false
  }
}`

// numericTextJSON is numericText as a JSON string body: its backslash doubled.
const numericTextJSON = `^-?[0-9]+(\\.[0-9]+)?$`

// ExtraCatalog is the engine fixture with test.described, test.attended and
// test.taken added.
var ExtraCatalog = catalogtest.Over(engineWith(addExtras))

func addExtras(defs fstest.MapFS) {
	for name, body := range map[string]string{
		"test.described.json": describedManifest,
		"test.attended.json":  attendedManifest,
		"test.taken.json":     takenManifest,
	} {
		defs[name] = &fstest.MapFile{Data: []byte(body)}
	}
}

// berlinCatalog is the engine fixture with record.edge read in Europe/Berlin rather
// than UTC: the structural type as a deployment outside UTC supplies it, so the link
// form is held to a zone with clocks that skip.
var berlinCatalog = catalogtest.Over(engineWith(func(defs fstest.MapFS) {
	const name = "record.edge.json"
	body := string(defs[name].Data)
	berlin := strings.Replace(body, `"zone": "UTC"`, `"zone": "Europe/Berlin"`, 1)
	if berlin == body {
		panic("inspect: " + name + " in the engine fixture names no UTC zone to replace")
	}
	defs[name] = &fstest.MapFile{Data: []byte(berlin)}
}))

// engineWith returns the roots of a copy of the engine fixture whose definitions edit
// has changed. The fixture is embedded, so a read that fails is a bug in the fixture,
// not data, and panics.
func engineWith(edit func(defs fstest.MapFS)) func() (definitions, projections fs.FS) {
	return func() (fs.FS, fs.FS) {
		defs, projs := fixture.Engine()
		out := fstest.MapFS{}
		err := fs.WalkDir(defs, ".", func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			b, err := fs.ReadFile(defs, path)
			if err != nil {
				return err
			}
			out[path] = &fstest.MapFile{Data: b}
			return nil
		})
		if err != nil {
			panic("inspect: copy the engine fixture: " + err.Error())
		}
		edit(out)
		return out, projs
	}
}
