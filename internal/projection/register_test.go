package projection_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/projection"
)

const heartV1 = `{"name":"heart","version":1,"description":"heart","types":["health.heart-rate-daily","health.heart-rate-alert"],` +
	`"statuses":["admitted","endorsed"],"max_classification":"Sensitive","from":"","to":"","shape":"log"}`

func allDefined(facet.Type) bool { return true }

func TestRegister_SameBodyIsUnchangedAndAChangeIsANewVersion(t *testing.T) {
	t.Parallel()
	held, err := projection.Bundled()
	if err != nil {
		t.Fatal(err)
	}

	reg, err := projection.Register([]byte(heartV1), held, allDefined)
	if err != nil || !reg.New || reg.Template.Name != "heart" || reg.Template.Hash == "" {
		t.Fatalf("first registration: %+v, %v", reg, err)
	}
	held, err = projection.WithRegistered(held, []projection.Stored{{Name: "heart", Version: 1, Body: []byte(heartV1)}})
	if err != nil {
		t.Fatal(err)
	}

	// The same body, reformatted, is the same template.
	reg, err = projection.Register([]byte(strings.ReplaceAll(heartV1, ",", ", ")), held, allDefined)
	if err != nil || reg.New {
		t.Errorf("the same body again: %+v, %v", reg, err)
	}
	// A different body at the held version is refused.
	if _, err := projection.Register([]byte(strings.Replace(heartV1, `"endorsed"`, `"admitted"`, 1)), held, allDefined); !errors.Is(err, projection.ErrRegistered) {
		t.Errorf("a changed body at v1: %v, want ErrRegistered", err)
	}
	// v2 is new, and the registry then answers v2 by name and v1 by version.
	v2 := strings.Replace(heartV1, `"version":1`, `"version":2`, 1)
	if reg, err = projection.Register([]byte(v2), held, allDefined); err != nil || !reg.New {
		t.Fatalf("v2: %+v, %v", reg, err)
	}
	held, err = projection.WithRegistered(held, []projection.Stored{
		{Name: "heart", Version: 1, Body: []byte(heartV1)}, {Name: "heart", Version: 2, Body: []byte(v2)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if cur, err := held.Lookup("heart"); err != nil || cur.Version != 2 {
		t.Errorf("Lookup(heart) = v%d, %v; want v2", cur.Version, err)
	}
	if old, err := held.Version("heart", 1); err != nil || old.Version != 1 {
		t.Errorf("Version(heart, 1) = v%d, %v", old.Version, err)
	}
	if _, err := held.Version("heart", 3); !errors.Is(err, projection.ErrUnknown) {
		t.Errorf("Version(heart, 3): %v, want ErrUnknown", err)
	}
	// A version behind the newest is refused, even one never held.
	v2only, err := projection.NewRegistry(reg.Template)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projection.Register([]byte(heartV1), v2only, allDefined); !errors.Is(err, projection.ErrVersionBehind) {
		t.Errorf("v1 after v2: %v, want ErrVersionBehind", err)
	}
}

func TestRegister_ABundledTemplateIsTheFloor(t *testing.T) {
	t.Parallel()
	held, err := projection.Bundled()
	if err != nil {
		t.Fatal(err)
	}
	bp, _ := held.Lookup("bp")
	// The bundled body again: unchanged, not an error.
	if reg, err := projection.Register(bp.Body, held, allDefined); err != nil || reg.New {
		t.Errorf("the bundled bp body again: %+v, %v", reg, err)
	}
	// bp v2 registered over the bundled v1: allowed, and v1 stays held for receipts.
	v2 := strings.Replace(string(bp.Body), `"version":1`, `"version":2`, 1)
	if reg, err := projection.Register([]byte(v2), held, allDefined); err != nil || !reg.New {
		t.Fatalf("bp v2: %+v, %v", reg, err)
	}
	merged, err := projection.WithRegistered(held, []projection.Stored{{Name: "bp", Version: 2, Body: []byte(v2)}})
	if err != nil {
		t.Fatal(err)
	}
	if cur, _ := merged.Lookup("bp"); cur.Version != 2 {
		t.Errorf("bp current = v%d, want v2", cur.Version)
	}
	if old, err := merged.Version("bp", 1); err != nil || old.Hash != bp.Hash {
		t.Errorf("bp v1 is no longer the bundled body: %v", err)
	}
	if got := len(merged.Versions("bp")); got != 2 {
		t.Errorf("bp holds %d versions, want 2", got)
	}
	// A version below the bundled one is behind.
	v0 := strings.Replace(string(bp.Body), `"version":1`, `"version":0`, 1)
	if _, err := projection.Register([]byte(v0), held, allDefined); !errors.Is(err, projection.ErrBadTemplate) {
		t.Errorf("v0: %v, want ErrBadTemplate (version must be positive)", err)
	}
}

func TestRegister_RefusesATypeWithNoDefinition(t *testing.T) {
	t.Parallel()
	held, _ := projection.Bundled()
	defined := func(typ facet.Type) bool { return typ == "health.heart-rate-daily" }
	if _, err := projection.Register([]byte(heartV1), held, defined); !errors.Is(err, projection.ErrUndefinedType) ||
		!strings.Contains(err.Error(), "health.heart-rate-alert") {
		t.Errorf("a template naming an undefined type: %v", err)
	}
}

func TestWithRegistered_RefusesTwoBodiesAtOneVersion(t *testing.T) {
	t.Parallel()
	held, _ := projection.Bundled()
	bp, _ := held.Lookup("bp")
	other := strings.Replace(string(bp.Body), `"endorsed"`, `"admitted"`, 1)
	if _, err := projection.WithRegistered(held, []projection.Stored{{Name: "bp", Version: 1, Body: []byte(other)}}); !errors.Is(err, projection.ErrBadTemplate) {
		t.Errorf("a registered bp v1 with another body: %v, want ErrBadTemplate", err)
	}
	// A row whose body names another template is refused too.
	if _, err := projection.WithRegistered(held, []projection.Stored{{Name: "sleep", Version: 9, Body: bp.Body}}); !errors.Is(err, projection.ErrBadTemplate) {
		t.Errorf("a mislabelled row: %v", err)
	}
}
