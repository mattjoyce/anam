package def_test

import (
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/def"
)

// Card 200: Versions holds every version of a Definition that parses as a contract, and
// says why a version it cannot hold is missing.
func TestVersions_HoldsWhatParsesAndSaysWhyNotTheRest(t *testing.T) {
	t.Parallel()
	current := mustParse(t, versioned(importedManifest, 3))
	reg, err := def.NewRegistry(current)
	if err != nil {
		t.Fatal(err)
	}
	words := strings.Replace(versioned(importedManifest, 1), `"strict"`, `"lenient"`, 1)
	v := def.NewVersions(reg,
		[]def.Stored{held(importedManifest, 2), {Type: "test.reading", Version: 1, Body: []byte(words)}},
		[]def.Stored{held(importedManifest, 2), {Type: "test.reading", Version: 3, Body: []byte(`{}`)}},
	)

	for _, version := range []int{2, 3} {
		if d, ok := v.At("test.reading", version); !ok || d.Version != version {
			t.Errorf("At(test.reading, %d) = v%d, %v; want it held", version, d.Version, ok)
		}
	}
	if d, _ := v.At("test.reading", 3); d.Version != current.Version || d.Type != current.Type {
		t.Error("the Definition in force must win over a body stored at its version")
	}
	if _, ok := v.At("test.reading", 1); ok {
		t.Error("v1 is words that do not parse and must not be held")
	}
	if why := v.Why("test.reading", 1); !strings.Contains(why, "do not parse as a contract") || !strings.Contains(why, "lenient") {
		t.Errorf("Why(v1) = %q, want the parse failure", why)
	}
	if why := v.Why("test.reading", 7); !strings.Contains(why, "no Definition of test.reading at v7 is held") {
		t.Errorf("Why(v7) = %q", why)
	}
	// A body filed under the wrong version is not that version.
	wrong := def.NewVersions(reg, []def.Stored{{Type: "test.reading", Version: 5, Body: []byte(versioned(importedManifest, 4))}})
	if _, ok := wrong.At("test.reading", 5); ok {
		t.Error("a v4 body filed as v5 was held as v5")
	}
}
