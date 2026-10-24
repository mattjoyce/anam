package query_test

import (
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/query"
)

// Each text a receipt may name is found by its hash, with the version it was served
// under; the bundled text is v2 and the three v1 texts are told apart by hash alone.
func TestHeldRuleSet_FindsEveryTextByItsHash(t *testing.T) {
	t.Parallel()
	bundled, err := query.Bundled()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		hash    string
		version int
		defines string // a relation the text has, or "" for none to check
		lacks   string // one it does not
	}{
		{"the bundled text", bundled.Hash, 2, "in_force", ""},
		{"v1 as first served", "sha256:bd62040288c0e5b4e6de576c13618b065714736ed6c628106b73b0769c32727d", 1, "current", "linked"},
		{"v1 with the edge rules over record.note", "sha256:e29634877441262c765923739d4d2cb49ae7e15d93b537dd972ebc68bb7c026d", 1, "linked", "in_force"},
		{"v1 with the edge rules over record.edge", "sha256:f8a1a46bce8701e37c18e92340e5ce7ab082d3f791ec98c0dbacbc61802a2220", 1, "linked", "in_force"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rs, ok, err := query.HeldRuleSet(tt.hash)
			if err != nil || !ok {
				t.Fatalf("HeldRuleSet = ok %v, %v", ok, err)
			}
			if rs.Hash != tt.hash || rs.Version != tt.version || rs.Name != "anam" {
				t.Errorf("found %s v%d %s", rs.Name, rs.Version, rs.Hash)
			}
			heads := map[string]bool{}
			for _, r := range rs.Program.Rules {
				heads[r.Head.Name] = true
			}
			if !heads[tt.defines] {
				t.Errorf("the text defines no %s", tt.defines)
			}
			if tt.lacks != "" && heads[tt.lacks] {
				t.Errorf("the text defines %s, which it should not", tt.lacks)
			}
		})
	}

	// The record.note text and the record.edge text differ in the type they read,
	// and nothing else: the version number could not tell them apart, the hash does.
	note, _, _ := query.HeldRuleSet(tests[2].hash)
	edge, _, _ := query.HeldRuleSet(tests[3].hash)
	if !strings.Contains(note.Program.Text, `"record.note"`) || strings.Contains(note.Program.Text, `"record.edge"`) {
		t.Error("the record.note text is not the one over record.note")
	}
	if !strings.Contains(edge.Program.Text, `"record.edge"`) || strings.Contains(edge.Program.Text, `"record.note"`) {
		t.Error("the record.edge text is not the one over record.edge")
	}
}

func TestHeldRuleSet_AHashNoTextHasIsNotHeld(t *testing.T) {
	t.Parallel()
	for _, hash := range []string{"", "sha256:" + strings.Repeat("0", 64), "bd62040288c0e5b4e6de576c13618b065714736ed6c628106b73b0769c32727d"} {
		if _, ok, err := query.HeldRuleSet(hash); ok || err != nil {
			t.Errorf("HeldRuleSet(%q) = ok %v, %v; want not held, no error", hash, ok, err)
		}
	}
}

// Every held text still compiles with the evaluator as it is now, so a receipt cut
// under it replays: the same program over each runs and answers current.
func TestHeldRuleSet_EveryTextRuns(t *testing.T) {
	t.Parallel()
	v, versions := value(t, entry("w", "health.weight", noon, "", `{}`))
	for _, hash := range []string{
		"sha256:bd62040288c0e5b4e6de576c13618b065714736ed6c628106b73b0769c32727d",
		"sha256:e29634877441262c765923739d4d2cb49ae7e15d93b537dd972ebc68bb7c026d",
		"sha256:f8a1a46bce8701e37c18e92340e5ce7ab082d3f791ec98c0dbacbc61802a2220",
	} {
		rs, ok, err := query.HeldRuleSet(hash)
		if err != nil || !ok {
			t.Fatalf("%s: ok %v, %v", hash, ok, err)
		}
		ans, err := query.RunWith(t.Context(), rs, v, versions, registry(t), "instant(At) :- entry(_, _, At, _, _, _, _).", "current", 0)
		if err != nil {
			t.Errorf("%s: %v", hash, err)
			continue
		}
		if got := rows(ans); !same(got, []string{"w " + noon}) {
			t.Errorf("%s: current = %v", hash, got)
		}
	}
}
