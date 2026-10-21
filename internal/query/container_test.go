package query_test

import (
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/def"
)

// A pointer that names a container is refused at the plan check, and the refusal
// names the leaves to ask for instead. attr holds the leaves of a payload and a
// container holds none, so a program that asked for one would answer empty forever
// and say nothing.

// cuffNoteManifest declares "cuff" as a plain string, where the blood-pressure type
// declares it as an object.
const cuffNoteManifest = `{
  "meta": {
    "facet_type": "health.cuff-note", "definition_version": 1, "classification": "Normal",
    "validation_mode": "strict", "source_schema": "note/cuff",
    "time_semantics": {"fact_time": {"pointer": "/measured_at", "layout": "2006-01-02T15:04:05Z07:00", "zone": "UTC"}},
    "identity_key": ["/measured_at"]
  },
  "schema": {"type": "object", "required": ["measured_at"],
    "properties": {"measured_at": {"type": "string", "minLength": 1}, "cuff": {"type": "string", "minLength": 1}},
    "additionalProperties": false}
}`

// cuffObjectManifest declares "cuff" as an object with a member the blood-pressure
// type does not have.
const cuffObjectManifest = `{
  "meta": {
    "facet_type": "health.cuff-object", "definition_version": 1, "classification": "Normal",
    "validation_mode": "strict", "source_schema": "note/cuff",
    "time_semantics": {"fact_time": {"pointer": "/measured_at", "layout": "2006-01-02T15:04:05Z07:00", "zone": "UTC"}},
    "identity_key": ["/measured_at"]
  },
  "schema": {"type": "object", "required": ["measured_at"],
    "properties": {"measured_at": {"type": "string", "minLength": 1},
      "cuff": {"type": "object", "properties": {"side": {"type": "string"}}, "additionalProperties": false}},
    "additionalProperties": false}
}`

// registryOf is a registry over the given manifests.
func registryOf(t *testing.T, manifests ...string) *def.Registry {
	t.Helper()
	var defs []def.Definition
	for _, m := range manifests {
		d, err := def.Parse([]byte(m))
		if err != nil {
			t.Fatalf("def.Parse: %v", err)
		}
		defs = append(defs, d)
	}
	reg, err := def.NewRegistry(defs...)
	if err != nil {
		t.Fatalf("def.NewRegistry: %v", err)
	}
	return reg
}

// A pointer that names a container in every Definition that declares it is refused,
// and the refusal names the leaves to ask for instead.
func TestPointer_AContainerPointerIsRefusedWithItsLeaves(t *testing.T) {
	t.Parallel()
	v, versions := value(t)
	_, err := tryRun(v, versions, registryOf(t, bpManifest),
		`cuff(E, A) :- entry(E, "health.blood-pressure", _, _, _, _, _), attr(E, "/cuff", A).`, "cuff")
	if err == nil {
		t.Fatal("the engine accepted a pointer that names a container")
	}
	if !strings.Contains(err.Error(), "/cuff/arm") {
		t.Errorf("the refusal must name the leaves to ask for: %v", err)
	}
}

// When every Definition that declares the pointer declares it as a container, the
// refusal names the leaves of them all.
func TestPointer_TheRefusalNamesTheLeavesOfEveryType(t *testing.T) {
	t.Parallel()
	v, versions := value(t)
	_, err := tryRun(v, versions, registryOf(t, bpManifest, cuffObjectManifest),
		`cuff(E, A) :- entry(E, "health.blood-pressure", _, _, _, _, _), attr(E, "/cuff", A).`, "cuff")
	if err == nil {
		t.Fatal("the engine accepted a pointer that names a container")
	}
	for _, leaf := range []string{"/cuff/arm", "/cuff/side"} {
		if !strings.Contains(err.Error(), leaf) {
			t.Errorf("the refusal must name %s: %v", leaf, err)
		}
	}
}

// A pointer that names a container in one type and a field in another is not
// refused: the field binds for the type that has one, and the answer comes from
// that type alone.
func TestPointer_AFieldInOneTypeAndAContainerInAnotherIsNotRefused(t *testing.T) {
	t.Parallel()
	v, versions := value(t, entry("f1", "health.cuff-note", "2026-06-15T00:00:00.000000000Z", "",
		`{"measured_at": "2026-06-15T07:00:00Z", "cuff": "left"}`))
	ans, err := tryRun(v, versions, registryOf(t, bpManifest, cuffNoteManifest),
		`cuff(E, C) :- entry(E, "health.cuff-note", _, _, _, _, _), attr(E, "/cuff", C).`, "cuff")
	if err != nil {
		t.Fatalf("a pointer that is a field in one type must not be refused: %v", err)
	}
	if ans.Len() != 1 {
		t.Errorf("the query answered %d rows, want 1", ans.Len())
	}
}
