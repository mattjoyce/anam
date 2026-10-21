package gate_test

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/gate"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// Card 198: the self-report types gained a version whose fact time reads the zone a
// fact names. The guard lets that version stand because no facet admitted before
// carries one. This holds it over a real ledger: facets admitted under each type's
// previous version, then read and sent again under the new one, keep their instant
// and their id.

// zonedTypes are the bundled types whose fact time reads a zone from the fact, each
// with the version before that one, from the binary's history.
func zonedTypes(t *testing.T) (now, before []def.Definition, bodies [][]byte) {
	t.Helper()
	reg, err := def.Bundled()
	if err != nil {
		t.Fatal(err)
	}
	history, err := def.History()
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
		body, ok := old[fmt.Sprintf("%s@%d", d.Type, d.Version-1)]
		if !ok {
			t.Fatalf("%s v%d reads a zone but its v%d is not in the history", d.Type, d.Version, d.Version-1)
		}
		prev, err := def.Parse(body)
		if err != nil {
			t.Fatalf("%s v%d: %v", d.Type, d.Version-1, err)
		}
		if prev.FactTime.ZonePointer != "" {
			t.Fatalf("%s v%d already read a zone", d.Type, prev.Version)
		}
		now, before, bodies = append(now, d), append(before, prev), append(bodies, body)
	}
	if len(now) < 3 {
		t.Fatalf("%d zoned types, want the self-report types", len(now))
	}
	return now, before, bodies
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

	sydney, _ := time.LoadLocation("Australia/Sydney")
	moments := []time.Time{ // summer, winter, either side of both of Sydney's changes, a midnight
		time.Date(2026, 1, 15, 9, 30, 0, 0, sydney), time.Date(2025, 7, 1, 0, 0, 0, 0, sydney),
		time.Date(2026, 4, 5, 3, 30, 0, 0, sydney), time.Date(2026, 10, 4, 3, 0, 0, 0, sydney),
		time.Date(2026, 10, 19, 18, 0, 0, 0, sydney),
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
		// read. One that only declares it (health.weight, which records sends without)
		// takes it back as the same facet at the same instant.
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
			continue
		}
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
