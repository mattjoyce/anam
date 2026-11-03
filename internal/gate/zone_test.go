package gate_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/catalog/catalogtest"
	"github.com/mattjoyce/anam/internal/catalog/fixture"
	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/gate"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// Card 198: a type may gain a version whose fact time reads the zone a fact names. The
// guard lets that version stand because no facet admitted before carries one. This
// holds it over a real ledger: facets admitted under each type's previous version, then
// read and sent again under the new one, keep their instant and their id. The types
// are the engine fixture's test.reading and test.templated, whose v2 reads the offset
// source_time names and requires source_time, and a copy of test.reading whose v2 only
// declares it, so both answers to a resent payload are held.

// zonedTypes are the engine fixture's types whose fact time reads a zone from the fact,
// each with the version before that one, from its history, and test.optional, a copy of
// test.reading at both versions whose v2 does not require source_time.
func zonedTypes(t *testing.T) (now, before []def.Definition, bodies [][]byte) {
	t.Helper()
	reg := catalogtest.Engine.Definitions(t)
	defs, _ := fixture.Engine()
	history, err := def.HistoryFS(defs)
	if err != nil {
		t.Fatal(err)
	}
	old := map[string][]byte{}
	for _, h := range history {
		old[fmt.Sprintf("%s@%d", h.Type, h.Version)] = h.Body
	}
	for _, d := range reg.All() {
		if d.FactTime.ZonePointer == "" {
			continue
		}
		// The version before the type first read a zone is what its facets were
		// admitted under: walk back past versions that already read one (a later
		// version may change only its label or its words).
		var prev def.Definition
		var body []byte
		born := true
		for v := d.Version - 1; v >= 1; v-- {
			b, ok := old[fmt.Sprintf("%s@%d", d.Type, v)]
			if !ok {
				t.Fatalf("%s v%d reads a zone but its v%d is not in the history", d.Type, d.Version, v)
			}
			p, err := def.Parse(b)
			if err != nil {
				t.Fatalf("%s v%d: %v", d.Type, v, err)
			}
			if p.FactTime.ZonePointer == "" {
				prev, body, born = p, b, false
				break
			}
		}
		if born {
			continue // born reading a zone (record.edge): no facet was admitted before
		}
		now, before, bodies = append(now, d), append(before, prev), append(bodies, body)
	}
	var names []string
	for _, d := range now {
		names = append(names, string(d.Type))
	}
	if got := strings.Join(names, ","); got != "test.reading,test.templated" {
		t.Fatalf("zoned types = %s, want test.reading,test.templated", got)
	}
	current, err := fs.ReadFile(defs, "test.reading.json")
	if err != nil {
		t.Fatal(err)
	}
	optNow, optBefore, optBody := optional(t, current, old["test.reading@1"])
	return append(now, optNow), append(before, optBefore), append(bodies, optBody)
}

// samplePayload is a payload of d's schema with its fact time at, built from the
// schema's own examples, so the test names no type.
func samplePayload(t *testing.T, d def.Definition, body []byte, at time.Time, n int) []byte {
	t.Helper()
	var m struct {
		Schema struct {
			Required   []string                  `json:"required"`
			Properties map[string]map[string]any `json:"properties"`
		} `json:"schema"`
	}
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatal(err)
	}
	fields := append([]string(nil), m.Schema.Required...)
	if _, ok := m.Schema.Properties["text"]; ok {
		fields = append(fields, "text") // record.edge needs words or ends
	}
	p := map[string]any{}
	for _, name := range fields {
		prop := m.Schema.Properties[name]
		switch {
		case "/"+name == d.FactTime.Pointer:
			p[name] = at.Format(d.FactTime.Layout)
		case name == "id":
			p[name] = fmt.Sprintf("test:%s:%d", d.Type, n)
		case prop["enum"] != nil:
			p[name] = prop["enum"].([]any)[0]
		case prop["examples"] != nil:
			p[name] = prop["examples"].([]any)[0]
		case prop["type"] == "number" || prop["type"] == "integer":
			p[name] = 1
		default:
			p[name] = "x"
		}
	}
	if d.IdentityTemplate != "" {
		delete(p, "id") // the gate composes it
	}
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func zoneGate(t *testing.T, l *ledger.Ledger, defs ...def.Definition) *gate.Gate {
	t.Helper()
	reg, err := def.NewRegistry(defs...)
	if err != nil {
		t.Fatal(err)
	}
	var types []facet.Type
	for _, d := range defs {
		types = append(types, d.Type)
	}
	scopes := gate.StaticScopes{"conversation": types}
	return gate.New(l, reg, scopes, clock.Real{}, person, gate.Limits{MaxPayloadBytes: config.DefaultMaxPayloadBytes, MaxRetries: config.DefaultCommitRetries})
}

func commitAs(t *testing.T, g *gate.Gate, typ facet.Type, payload []byte) gate.Result {
	t.Helper()
	env := facet.Envelope{Type: typ, Source: "conversation", Injector: "conversation", Payload: payload}
	b, err := g.Resolve(env)
	if err != nil {
		t.Fatal(err)
	}
	res, _, err := g.Commit(context.Background(), env, b)
	if err != nil {
		t.Fatalf("%s %s: %v", typ, payload, err)
	}
	return res
}

func TestZonePointer_EveryAdmittedFacetReadsTheSameInstant(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	l, err := ledger.Create(ctx, filepath.Join(t.TempDir(), "ledger.db"), person)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	now, before, bodies := zonedTypes(t)
	old := zoneGate(t, l, before...)

	berlin, _ := time.LoadLocation("Europe/Berlin")
	moments := []time.Time{ // winter, summer, either side of both of Berlin's changes, a midnight
		time.Date(2026, 1, 15, 9, 30, 0, 0, berlin), time.Date(2025, 7, 1, 0, 0, 0, 0, berlin),
		time.Date(2026, 3, 29, 3, 30, 0, 0, berlin), time.Date(2026, 10, 25, 3, 0, 0, 0, berlin),
		time.Date(2026, 10, 19, 18, 0, 0, 0, berlin),
	}
	type admitted struct {
		typ     facet.Type
		payload []byte
		id      string
		at      time.Time
	}
	var held []admitted
	for i, d := range before {
		for j, at := range moments {
			payload := samplePayload(t, d, bodies[i], at, i*len(moments)+j)
			res := commitAs(t, old, d.Type, payload)
			f, err := l.CurrentVersion(ctx, res.FacetID)
			if err != nil {
				t.Fatal(err)
			}
			if f.DefinitionVersion != d.Version {
				t.Fatalf("%s admitted at v%d, want v%d", d.Type, f.DefinitionVersion, d.Version)
			}
			held = append(held, admitted{d.Type, f.Payload.Bytes(), res.FacetID, f.FactTime})
		}
	}

	// Read under the new versions, every admitted facet is where it was and composes
	// the id it has; sent again, it is the same facet at the same instant.
	newer := zoneGate(t, l, now...)
	byType := map[facet.Type]def.Definition{}
	for _, d := range now {
		byType[d.Type] = d
	}
	var refused, taken int
	for _, h := range held {
		d := byType[h.typ]
		doc, err := def.Decode(h.payload)
		if err != nil {
			t.Fatal(err)
		}
		at, err := d.FactTimeOf(doc)
		if err != nil || !at.Equal(h.at) {
			t.Errorf("%s %s reads %v (%v) under v%d, was admitted at %v", h.typ, h.payload, at, err, d.Version, h.at)
		}
		if _, err := d.WithID("conversation", h.payload); err != nil {
			t.Errorf("%s %s: its id is not what v%d composes: %v", h.typ, h.payload, d.Version, err)
		}
		// A type whose new version requires source_time refuses the old payload sent
		// again, naming the field: a writer that resends must say how its clock was
		// read. One that only declares it (test.optional) takes it back as the same
		// facet at the same instant.
		env := facet.Envelope{Type: h.typ, Source: "conversation", Injector: "conversation", Payload: h.payload}
		b, err := newer.Resolve(env)
		if err != nil {
			t.Fatal(err)
		}
		res, _, err := newer.Commit(ctx, env, b)
		if requires(t, d, h.payload) {
			if err == nil || !strings.Contains(err.Error(), "source_time") {
				t.Errorf("%s v%d took an old payload without source_time: %+v, %v", h.typ, d.Version, res, err)
			}
			refused++
			continue
		}
		taken++
		if err != nil {
			t.Fatalf("%s sent again under v%d: %v", h.typ, d.Version, err)
		}
		f, err := l.CurrentVersion(ctx, res.FacetID)
		if err != nil {
			t.Fatal(err)
		}
		if res.FacetID != h.id || !f.FactTime.Equal(h.at) {
			t.Errorf("%s sent again under v%d: facet %s at %v, want %s at %v", h.typ, d.Version, res.FacetID, f.FactTime, h.id, h.at)
		}
	}
	types := map[string]bool{}
	for _, h := range held {
		types[string(h.typ)] = true
	}
	names := make([]string, 0, len(types))
	for n := range types {
		names = append(names, n)
	}
	sort.Strings(names)
	t.Logf("%d facets of %s", len(held), strings.Join(names, ", "))
	// Both answers to a resent payload were given: two types refuse it, one takes it back.
	if refused != 2*len(moments) || taken != len(moments) {
		t.Errorf("resent: %d refused and %d taken back, want %d and %d", refused, taken, 2*len(moments), len(moments))
	}
}

// requires reports whether d refuses payload for want of source_time.
func requires(t *testing.T, d def.Definition, payload []byte) bool {
	t.Helper()
	doc, err := def.Decode(payload)
	if err != nil {
		t.Fatal(err)
	}
	err = d.Check(doc)
	return err != nil && strings.Contains(err.Error(), "source_time")
}

// optional is test.reading's v2 (now) and v1 (before) renamed test.optional, with v2
// declaring source_time and no longer requiring it, and v1's body. A facet admitted under
// v1 and sent again under v2 is then taken back, not refused.
func optional(t *testing.T, now, before []byte) (def.Definition, def.Definition, []byte) {
	t.Helper()
	rename := func(body []byte, unrequire string) []byte {
		var m map[string]any
		if err := json.Unmarshal(body, &m); err != nil {
			t.Fatal(err)
		}
		m["meta"].(map[string]any)["facet_type"] = "test.optional"
		schema := m["schema"].(map[string]any)
		var required []any
		for _, f := range schema["required"].([]any) {
			if f != unrequire {
				required = append(required, f)
			}
		}
		schema["required"] = required
		out, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	nowBody, beforeBody := rename(now, "source_time"), rename(before, "")
	n, err := def.Parse(nowBody)
	if err != nil {
		t.Fatal(err)
	}
	b, err := def.Parse(beforeBody)
	if err != nil {
		t.Fatal(err)
	}
	if n.FactTime.ZonePointer == "" || b.FactTime.ZonePointer != "" || n.Version != b.Version+1 {
		t.Fatalf("test.optional: v%d zone pointer %q, v%d zone pointer %q; want v2 reading one and v1 none", n.Version, n.FactTime.ZonePointer, b.Version, b.FactTime.ZonePointer)
	}
	return n, b, beforeBody
}
