package config_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/facet"
)

const hashA = "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"
const hashB = "60303ae22b998861bce3b28f33eec1be758a213c86c93c076dbe9f558c11c752"

func TestLoadImporters(t *testing.T) {
	t.Parallel()
	valid := `
importers:
  - source: withings
    token_sha256: ` + hashA + `
    types: [health.weight]
  - source: garmin
    token_sha256: ` + hashB + `
    types: [health.sleep, exercise.session]
`
	t.Run("a valid file", func(t *testing.T) {
		t.Parallel()
		got, err := config.LoadImporters(writeImports(t, valid))
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 || got[1].Source != "garmin" || got[1].Types[1] != facet.Type("exercise.session") ||
			got[0].TokenSHA256 != hashA {
			t.Fatalf("loaded %+v", got)
		}
	})

	cases := []struct{ name, body string }{
		{"an unknown key", strings.Replace(valid, "types: [health.weight]", "types: [health.weight]\n    tokn: x", 1)},
		{"no source", strings.Replace(valid, "source: withings", "source: ''", 1)},
		{"a short hash", strings.Replace(valid, hashA, hashA[:62], 1)},
		{"an upper-case hash", strings.Replace(valid, hashA, strings.ToUpper(hashA), 1)},
		{"a source named twice", strings.Replace(valid, "source: garmin", "source: withings", 1)},
		{"a shared token", strings.Replace(valid, hashB, hashA, 1)},
		{"a type two importers name", strings.Replace(valid, "types: [health.sleep, exercise.session]", "types: [health.weight]", 1)},
		{"no types", strings.Replace(valid, "types: [health.weight]", "types: []", 1)},
		{"a type that is not namespaced", strings.Replace(valid, "types: [health.weight]", "types: [weight]", 1)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			_, err := config.LoadImporters(writeImports(t, c.body))
			if !errors.Is(err, config.ErrInvalidImporters) {
				t.Fatalf("got %v, want ErrInvalidImporters", err)
			}
		})
	}
}
