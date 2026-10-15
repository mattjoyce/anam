package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/facet"
)

const hashA = "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"
const hashB = "60303ae22b998861bce3b28f33eec1be758a213c86c93c076dbe9f558c11c752"
const hashC = "fd61a03af4f77d870fc21e05e7e80678095c92d808cfb3b5c279ee04c74aca13"

func writeImporters(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "importers.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadPrincipals(t *testing.T) {
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
		got, err := config.LoadPrincipals(writeImporters(t, valid), nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 || got[1].Source != "garmin" || got[1].Types[1] != facet.Type("exercise.session") ||
			got[0].TokenSHA256 != hashA {
			t.Fatalf("loaded %+v", got)
		}
		// The file's first shape: every importer is a principal named for its source.
		if got[0].Name != "withings" || got[0].Role != config.RoleImporter {
			t.Fatalf("an importer reads as %+v", got[0])
		}
	})

	cases := []struct{ name, body string }{
		{"an unknown key", strings.Replace(valid, "types: [health.weight]", "types: [health.weight]\n    tokn: x", 1)},
		{"no source", strings.Replace(valid, "source: withings", "source: ''", 1)},
		{"a short hash", strings.Replace(valid, hashA, hashA[:62], 1)},
		{"an upper-case hash", strings.Replace(valid, hashA, strings.ToUpper(hashA), 1)},
		{"an importer with another role", strings.Replace(valid, "source: withings", "source: withings\n    role: reader", 1)},
		{"a source named twice", strings.Replace(valid, "source: garmin", "source: withings", 1)},
		{"a shared token", strings.Replace(valid, hashB, hashA, 1)},
		{"a name used twice", strings.Replace(valid, "source: garmin", "source: garmin\n    name: withings", 1)},
		{"a type two importers name", strings.Replace(valid, "types: [health.sleep, exercise.session]", "types: [health.weight]", 1)},
		{"no types", strings.Replace(valid, "types: [health.weight]", "types: []", 1)},
		{"a type that is not namespaced", strings.Replace(valid, "types: [health.weight]", "types: [weight]", 1)},
	}
	t.Run("a bundled type two importers name", func(t *testing.T) {
		t.Parallel()
		body := strings.Replace(valid, "types: [health.sleep, exercise.session]", "types: [health.weight]", 1)
		bundled := func(ty facet.Type) bool { return ty == "health.weight" }
		got, err := config.LoadPrincipals(writeImporters(t, body), bundled)
		if err != nil || len(got) != 2 {
			t.Fatalf("got %v, %v; want both importers", got, err)
		}
	})

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			_, err := config.LoadPrincipals(writeImporters(t, c.body), nil)
			if !errors.Is(err, config.ErrInvalidImporters) {
				t.Fatalf("got %v, want ErrInvalidImporters", err)
			}
		})
	}
}

func TestLoadPrincipalsWithRoles(t *testing.T) {
	t.Parallel()
	valid := `
principals:
  - name: assistant
    role: reader
    token_sha256: ` + hashC + `
  - name: conversation
    role: importer
    source: conversation
    token_sha256: ` + hashB + `
    types: [note.annotation]
importers:
  - source: withings
    token_sha256: ` + hashA + `
    types: [health.weight]
`
	t.Run("a valid file", func(t *testing.T) {
		t.Parallel()
		got, err := config.LoadPrincipals(writeImporters(t, valid), nil)
		if err != nil {
			t.Fatal(err)
		}
		roles := map[string]config.Role{}
		for _, p := range got {
			roles[p.Name] = p.Role
		}
		want := map[string]config.Role{"assistant": config.RoleReader, "conversation": config.RoleImporter, "withings": config.RoleImporter}
		if len(roles) != len(want) {
			t.Fatalf("loaded %+v", got)
		}
		for name, role := range want {
			if roles[name] != role {
				t.Errorf("%s: role %q, want %q", name, roles[name], role)
			}
		}
	})

	cases := []struct{ name, body string }{
		{"no name", strings.Replace(valid, "name: assistant", "name: ''", 1)},
		{"the operator's name", strings.Replace(valid, "name: assistant", "name: operator", 1)},
		{"no role", strings.Replace(valid, "role: reader", "role: ''", 1)},
		{"an unknown role", strings.Replace(valid, "role: reader", "role: admin", 1)},
		{"a reader with a source", strings.Replace(valid, "role: reader", "role: reader\n    source: chat", 1)},
		{"a reader with types", strings.Replace(valid, "role: reader", "role: reader\n    types: [note.annotation]", 1)},
		{"an importer with no source", strings.Replace(valid, "source: conversation", "source: ''", 1)},
		{"a reader sharing an importer's token", strings.Replace(valid, hashC, hashA, 1)},
		{"a name an importer takes by its source", strings.Replace(valid, "name: assistant", "name: withings", 1)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			_, err := config.LoadPrincipals(writeImporters(t, c.body), nil)
			if !errors.Is(err, config.ErrInvalidImporters) {
				t.Fatalf("got %v, want ErrInvalidImporters", err)
			}
		})
	}
}
