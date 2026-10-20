package query_test

import (
	"context"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/gate"
	"github.com/mattjoyce/anam/internal/projection"
	"github.com/mattjoyce/anam/internal/query"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// The edge rules (card 104) over a real ledger: a note about a reading is an edge
// touching it, read from the served payloads with no table; a v2 note names its
// sides, and the kind is its own attr.

// edgeHarness is newHarness with the bundled record.note beside the test types, a
// records source allowed to write it, and the log projection serving it.
func edgeHarness(t *testing.T) *harness {
	t.Helper()
	l, err := ledger.Create(context.Background(), filepath.Join(t.TempDir(), "ledger.db"), "matt")
	if err != nil {
		t.Fatalf("ledger.Create: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })
	bundled, err := def.Bundled()
	if err != nil {
		t.Fatal(err)
	}
	note, ok := bundled.Lookup("record.note")
	if !ok {
		t.Fatal("no bundled record.note")
	}
	defs, err := def.NewRegistry(append(registry(t).All(), note)...)
	if err != nil {
		t.Fatalf("def.NewRegistry: %v", err)
	}
	scopes := gate.StaticScopes{
		"bp-csv":  {"health.blood-pressure"},
		"records": {"record.note"},
	}
	tmpl, err := projection.Parse([]byte(strings.Replace(logTemplate, `"health.blood-pressure",`, `"health.blood-pressure", "record.note",`, 1)))
	if err != nil {
		t.Fatalf("projection.Parse: %v", err)
	}
	return &harness{
		l: l, defs: defs, tmpl: tmpl,
		gate: gate.New(l, defs, scopes, clock.Real{}, "matt",
			gate.Limits{MaxPayloadBytes: config.DefaultMaxPayloadBytes, MaxRetries: config.DefaultCommitRetries}),
	}
}

func pairs(ans query.Answer) []string {
	var out []string
	for _, tup := range ans.Tuples {
		out = append(out, tup[0].Text()+" "+tup[1].Text())
	}
	sort.Strings(out)
	return out
}

func TestEdges_LinkedFromTheServedPayloads(t *testing.T) {
	t.Parallel()
	h := edgeHarness(t)
	h.bp(t, "2026-02-15T07:00:00Z", 121, 79)
	h.bp(t, "2026-04-10T07:00:00Z", 146, 92)
	v, _, _ := h.served(t)
	var ids []string
	for _, e := range v.Entries() {
		ids = append(ids, e.FacetID)
	}
	sort.Strings(ids)
	a, b := ids[0], ids[1]

	// A v1 note about a; a v2 edge from b to a, kind said; a note that names nothing.
	h.admit(t, "records", "record.note", "note:1", `{"id": "note:1", "at": "2026-04-10 18:00", "about": ["`+a+`"], "text": "Before the dentist."}`)
	h.admit(t, "records", "record.note", "interp:1", `{"id": "interp:1", "at": "2026-04-11 09:00", "kind": "interpreted_from", "from": ["`+b+`"], "to": ["`+a+`"]}`)
	h.admit(t, "records", "record.note", "note:2", `{"id": "note:2", "at": "2026-04-12 09:00", "text": "A free account."}`)
	v, versions, _ := h.served(t)
	byRef := map[string]string{}
	for _, e := range v.Entries() {
		if e.Type == "record.note" {
			for _, ref := range []string{"note:1", "interp:1", "note:2"} {
				if strings.HasSuffix(e.SourceRef, ":"+ref) {
					byRef[ref] = e.FacetID
				}
			}
		}
	}
	note, interp := byRef["note:1"], byRef["interp:1"]
	if note == "" || interp == "" || byRef["note:2"] == "" {
		t.Fatalf("notes not served: %v", byRef)
	}

	ans, err := tryRun(v, versions, h.defs, "", "linked")
	if err != nil {
		t.Fatalf("linked: %v", err)
	}
	want := []string{interp + " " + a, interp + " " + b, note + " " + a}
	if got := pairs(ans); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("linked = %v, want %v", got, want)
	}
	to, err := tryRun(v, versions, h.defs, "", "edge_to")
	if err != nil {
		t.Fatalf("edge_to: %v", err)
	}
	if got := pairs(to); strings.Join(got, ",") != interp+" "+a+","+note+" "+a {
		t.Errorf("edge_to = %v", got)
	}
	from, err := tryRun(v, versions, h.defs, "", "edge_from")
	if err != nil {
		t.Fatalf("edge_from: %v", err)
	}
	if got := pairs(from); strings.Join(got, ",") != interp+" "+b {
		t.Errorf("edge_from = %v", got)
	}

	// A path of edges is a rule over linked: what the interpretation reaches through
	// the note, two hops, in a query of the reader's own.
	text := `reach(E, F) :- linked(E, F).
reach(E, G) :- reach(E, F), linked(X, F), linked(X, G), X != E, G != F.`
	r, err := tryRun(v, versions, h.defs, text, "reach")
	if err != nil {
		t.Fatalf("reach: %v", err)
	}
	if got := pairs(r); len(got) < 3 {
		t.Errorf("reach = %v", got)
	}
}

// The bundled set names one type by constant, and a rename would empty its rules
// without a word, so the constant is held to a type the binary bundles (card 104,
// finding 5).
func TestBundled_TypeConstantsAreBundledTypes(t *testing.T) {
	t.Parallel()
	rs, err := query.Bundled()
	if err != nil {
		t.Fatal(err)
	}
	bundled, err := def.Bundled()
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, r := range rs.Program.Rules {
		for _, l := range r.Body {
			if l.Atom == nil || l.Atom.Name != "entry" || l.Atom.Args[1].IsVar() {
				continue
			}
			n++
			typ := facet.Type(l.Atom.Args[1].Const.Text())
			if _, ok := bundled.Lookup(typ); !ok {
				t.Errorf("line %d names type %s, which no bundled Definition declares", l.Atom.Line, typ)
			}
		}
	}
	if n == 0 {
		t.Error("no type constant found; the edge rules name record.note")
	}
}
