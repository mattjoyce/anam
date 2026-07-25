package def_test

import (
	"regexp"
	"testing"

	"github.com/mattjoyce/anam/internal/def"
)

// The definition-set version is what a receipt records as the interpretation in force
// when its facets were captured (§9): the same binary must always answer the same
// thing, and the answer must be a digest rather than a lucky constant.
func TestSetVersion_DeterministicDigest(t *testing.T) {
	t.Parallel()

	first, err := def.SetVersion()
	if err != nil {
		t.Fatalf("SetVersion: %v", err)
	}
	second, err := def.SetVersion()
	if err != nil {
		t.Fatalf("SetVersion (again): %v", err)
	}
	if first != second {
		t.Errorf("SetVersion is not deterministic: %s then %s", first, second)
	}
	if !regexp.MustCompile(`^sha256:[0-9a-f]{64}$`).MatchString(first) {
		t.Errorf("SetVersion = %q, want sha256:<64 hex>", first)
	}
	if empty := def.SetVersionOf(map[string][]byte{}); first == empty {
		t.Error("the bundled set hashes the same as an empty one — the manifests are not being read")
	}
}

// Every way a definition set can change must move the digest. A version that stayed
// put through a rename, or through bytes sliding from one manifest into the next,
// would let a receipt claim an interpretation that was never in force.
func TestSetVersion_MovesOnEveryChange(t *testing.T) {
	t.Parallel()

	base := map[string][]byte{
		"health.sleep.json":          []byte(`{"meta":{"definition_version":1}}`),
		"health.blood-pressure.json": []byte(`{"meta":{"definition_version":1}}`),
	}
	baseline := def.SetVersionOf(base)

	cases := map[string]map[string][]byte{
		"a byte changes": {
			"health.sleep.json":          []byte(`{"meta":{"definition_version":2}}`),
			"health.blood-pressure.json": []byte(`{"meta":{"definition_version":1}}`),
		},
		"a manifest is renamed": {
			"health.slumber.json":        []byte(`{"meta":{"definition_version":1}}`),
			"health.blood-pressure.json": []byte(`{"meta":{"definition_version":1}}`),
		},
		"a manifest is added": {
			"health.sleep.json":          []byte(`{"meta":{"definition_version":1}}`),
			"health.blood-pressure.json": []byte(`{"meta":{"definition_version":1}}`),
			"health.heart-rate.json":     []byte(`{"meta":{"definition_version":1}}`),
		},
		"a manifest is removed": {
			"health.sleep.json": []byte(`{"meta":{"definition_version":1}}`),
		},
		"bytes move between manifests": {
			"health.sleep.json":          []byte(`{"meta":{"definition_version":1}}{"meta":`),
			"health.blood-pressure.json": []byte(`{"definition_version":1}}`),
		},
	}
	for name, set := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := def.SetVersionOf(set); got == baseline {
				t.Errorf("%s left the digest at %s", name, got)
			}
		})
	}
}

// Map iteration order is random in Go; the digest must not be.
func TestSetVersionOf_IndependentOfMapOrder(t *testing.T) {
	t.Parallel()

	set := map[string][]byte{
		"a.json": []byte(`{"a":1}`),
		"b.json": []byte(`{"b":2}`),
		"c.json": []byte(`{"c":3}`),
	}
	want := def.SetVersionOf(set)
	for range 20 {
		if got := def.SetVersionOf(set); got != want {
			t.Fatalf("digest varied across calls: %s then %s", want, got)
		}
	}
}
