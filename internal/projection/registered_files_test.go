package projection_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"testing"
)

// A registered version is immutable, so rewording a description is a new version whose
// filter is the old one's. This holds each newest file in projections/ to the file it
// replaces: the same name and every knob alike, the next version, a new description
// that does not name a kanban card.
func TestRegisteredFiles_NewestOnlyRewordsThePrevious(t *testing.T) {
	t.Parallel()
	cardRef := regexp.MustCompile(`(?i)\bcards?[ -]\d`)
	for _, tc := range []struct {
		name       string
		older, new int
	}{
		{"heart", 3, 4},
		{"sleep", 3, 4},
		{"recent-life", 5, 6},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			older := readRegistered(t, tc.name, tc.older)
			newer := readRegistered(t, tc.name, tc.new)
			if got := newer["version"]; got != float64(tc.new) {
				t.Errorf("%s v%d file says version %v", tc.name, tc.new, got)
			}
			desc, _ := newer["description"].(string)
			if desc == "" || desc == older["description"] {
				t.Errorf("%s v%d keeps v%d's description: %q", tc.name, tc.new, tc.older, desc)
			}
			if cardRef.MatchString(desc) {
				t.Errorf("%s v%d's description names a card: %q", tc.name, tc.new, desc)
			}
			for _, m := range []map[string]any{older, newer} {
				delete(m, "version")
				delete(m, "description")
			}
			if !reflect.DeepEqual(older, newer) {
				t.Errorf("%s v%d changes more than the description:\n v%d %v\n v%d %v", tc.name, tc.new, tc.older, older, tc.new, newer)
			}
		})
	}
}

func readRegistered(t *testing.T, name string, version int) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "projections", name+".v"+strconv.Itoa(version)+".json"))
	if err != nil {
		t.Fatalf("read %s v%d: %v", name, version, err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("parse %s v%d: %v", name, version, err)
	}
	return m
}
