package def_test

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
)

// importedManifest is bpManifest under a type no binary bundles, as an importer's
// repo would carry it.
var importedManifest = strings.Replace(bpManifest, `"health.blood-pressure"`, `"test.reading"`, 1)

func versioned(manifest string, v int) string {
	return strings.Replace(manifest, `"definition_version": 1`, `"definition_version": `+strconv.Itoa(v), 1)
}

func held(manifest string, v int) def.Stored {
	return def.Stored{Type: "test.reading", Version: v, Body: []byte(versioned(manifest, v))}
}

// Card 53: a registered version is immutable, versions only go forward, a new version
// keys facets as the newest did, and a type lives in the binary or in an importer,
// never both.
func TestRegister(t *testing.T) {
	t.Parallel()
	bundled, err := def.Bundled()
	if err != nil {
		t.Fatal(err)
	}
	reformatted := strings.ReplaceAll(importedManifest, "\n", "")
	rekeyed := strings.Replace(importedManifest, `"identity_key": ["/measured_at"]`, `"identity_key": ["/systolic"]`, 1)
	widened := strings.Replace(importedManifest, `"comment": {"type": "string", "minLength": 1}`,
		`"comment": {"type": "string"}`, 1)

	cases := []struct {
		name       string
		body       string
		stored     []def.Stored
		registered []def.Stored
		wantNew    bool
		want       error
	}{
		{"a new type", importedManifest, nil, nil, true, nil},
		{"the same body again", importedManifest, nil, []def.Stored{held(importedManifest, 1)}, false, nil},
		{"the same body reformatted", reformatted, nil, []def.Stored{held(importedManifest, 1)}, false, nil},
		{"another body at a registered version", widened, nil, []def.Stored{held(importedManifest, 1)}, false, def.ErrRegistered},
		{"a newer version, same keying", versioned(widened, 2), nil, []def.Stored{held(importedManifest, 1)}, true, nil},
		{"a newer version, re-keyed", versioned(rekeyed, 2), nil, []def.Stored{held(importedManifest, 1)}, false, def.ErrKeyed},
		{"an older version", importedManifest, nil, []def.Stored{held(importedManifest, 2)}, false, def.ErrVersionBehind},
		{"taken over from the binary, same body", importedManifest, []def.Stored{held(importedManifest, 1)}, nil, true, nil},
		{"taken over from the binary, re-keyed", versioned(rekeyed, 2), []def.Stored{held(importedManifest, 1)}, nil, false, def.ErrKeyed},
		{"a type this binary bundles", bpManifestBundled(t, bundled), nil, nil, false, def.ErrBundled},
		{"not a manifest", `{"meta": {}}`, nil, nil, false, def.ErrBadManifest},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got, err := def.Register([]byte(c.body), c.stored, c.registered, bundled)
			if !errors.Is(err, c.want) {
				t.Fatalf("Register: got %v, want %v", err, c.want)
			}
			if err == nil && got.New != c.wantNew {
				t.Errorf("New = %v, want %v", got.New, c.wantNew)
			}
		})
	}
}

// bpManifestBundled is a manifest for a type the binary ships, at a version past it.
func bpManifestBundled(t *testing.T, bundled *def.Registry) string {
	t.Helper()
	d, ok := bundled.Lookup("health.blood-pressure")
	if !ok {
		t.Fatal("health.blood-pressure is not bundled")
	}
	return versioned(bpManifest, d.Version+1)
}

func TestWithRegistered(t *testing.T) {
	t.Parallel()
	bundled, err := def.Bundled()
	if err != nil {
		t.Fatal(err)
	}

	t.Run("the newest registered version is the one admitted against", func(t *testing.T) {
		t.Parallel()
		r, err := def.WithRegistered(bundled, []def.Stored{held(importedManifest, 2), held(importedManifest, 1)})
		if err != nil {
			t.Fatal(err)
		}
		d, ok := r.Lookup("test.reading")
		if !ok || d.Version != 2 {
			t.Fatalf("Lookup = v%d %v, want v2", d.Version, ok)
		}
		if _, ok := r.Lookup("health.weight"); !ok {
			t.Error("bundled types are missing")
		}
	})

	t.Run("a type the binary took back is admitted against the bundled one", func(t *testing.T) {
		t.Parallel()
		r, err := def.WithRegistered(bundled, []def.Stored{{
			Type: facet.Type("health.blood-pressure"), Version: 9, Body: []byte(versioned(bpManifest, 9)),
		}})
		if err != nil {
			t.Fatal(err)
		}
		want, _ := bundled.Lookup("health.blood-pressure")
		if got, _ := r.Lookup("health.blood-pressure"); got.Version != want.Version {
			t.Fatalf("health.blood-pressure is v%d, want the bundled v%d", got.Version, want.Version)
		}
	})
}
