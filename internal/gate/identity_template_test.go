package gate_test

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/catalog/catalogtest"
	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/gate"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// Card 188 S1: a Definition that declares an identity_template has its identity field
// filled when an envelope leaves it out and held to it when it carries it. Card 198:
// the template takes the offset source_time names, so one minute said in two zones is
// two facts. Over the engine fixture's test.templated, whose v2 template is
// {source}:templated/{/name|slug}/{/at|2006-01-02T15:04}{/source_time/offset?} and
// whose fact time is at read at that offset, and a real ledger.

func newTemplatedGate(t *testing.T) (*gate.Gate, *ledger.Ledger) {
	t.Helper()
	l, err := ledger.Create(context.Background(), filepath.Join(t.TempDir(), "ledger.db"), person)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	reg := catalogtest.Engine.Definitions(t)
	scopes := gate.StaticScopes{"conversation": {"test.templated", "test.measure"}}
	return gate.New(l, reg, scopes, clock.Real{}, person, gate.Limits{MaxPayloadBytes: config.DefaultMaxPayloadBytes, MaxRetries: config.DefaultCommitRetries}), l
}

func sayTemplated(t *testing.T, g *gate.Gate, payload string) (gate.Result, error) {
	t.Helper()
	env := facet.Envelope{Type: "test.templated", Source: "conversation", Injector: "conversation", Payload: []byte(payload)}
	b, err := g.Resolve(env)
	if err != nil {
		return gate.Result{}, err
	}
	res, _, err := g.Commit(context.Background(), env, b)
	return res, err
}

// templatedClock is source_time for a wall clock said in zone at offset.
func templatedClock(wall, zone, offset string) string {
	return fmt.Sprintf(`"source_time": {"value": %q, "zone": %q, "offset": %q, "basis": "source"}`, wall, zone, offset)
}

// templated is a test.templated payload for name at the wall clock at, said in Berlin
// in October (+02:00), with extra (a JSON fragment ending in a comma, or empty) first.
func templated(at, name, extra string) string {
	return fmt.Sprintf(`{%s"at": %q, "name": %q, "amount": 5, "unit": "mg", %s}`, extra, at, name, templatedClock(at, "Europe/Berlin", "+02:00"))
}

const (
	templatedAt = "2026-10-17 22:00"
	templatedID = "conversation:templated/melatonin-dose/2026-10-17T22:00+02:00"
)

func TestTemplate_OmittedIdentityIsFilled(t *testing.T) {
	t.Parallel()
	g, _ := newTemplatedGate(t)
	// The name is slugged: lower case, each run of other characters one hyphen.
	res, err := sayTemplated(t, g, templated(templatedAt, "Melatonin Dose", ""))
	if err != nil || res.Outcome != gate.OutcomeAdmitted {
		t.Fatalf("got %+v, %v; want admitted", res, err)
	}
	// The filled envelope is the same facet, and the same bytes, as one that carried
	// the id: a re-send with it is a no-op on the same facet id.
	again, err := sayTemplated(t, g, templated(templatedAt, "Melatonin Dose", `"id": "`+templatedID+`", `))
	if err != nil || again.Outcome != gate.OutcomeNoOp || again.FacetID != res.FacetID {
		t.Errorf("explicit id after filled = %+v, %v; want no-op on %s", again, err, res.FacetID)
	}
	if want := facet.NewID(facet.SourceRef("conversation", templatedID)); res.FacetID != want {
		t.Errorf("facet id %s, want %s (as the hand-built id gave)", res.FacetID, want)
	}
}

func TestTemplate_CarriedIdentityMustMatch(t *testing.T) {
	t.Parallel()
	g, _ := newTemplatedGate(t)
	if res, err := sayTemplated(t, g, templated(templatedAt, "Melatonin Dose", `"id": "`+templatedID+`", `)); err != nil || res.Outcome != gate.OutcomeAdmitted {
		t.Fatalf("matching id: %+v, %v", res, err)
	}
	wrong := templated(templatedAt, "Melatonin Dose", `"id": "`+strings.Replace(templatedID, "T22:00", "T22:30", 1)+`", `)
	_, err := sayTemplated(t, g, wrong)
	if !errors.Is(err, def.ErrIdentityField) {
		t.Fatalf("got %v, want ErrIdentityField", err)
	}
	if !strings.Contains(err.Error(), "T22:30") || !strings.Contains(err.Error(), templatedID) {
		t.Errorf("error should show both ids: %v", err)
	}
}

func TestTemplate_ATypeWithoutOneIsUntouched(t *testing.T) {
	t.Parallel()
	g, _ := newTemplatedGate(t)
	m := facet.Envelope{Type: "test.measure", Source: "conversation", Injector: "conversation",
		Payload: []byte(`{"at": "2026-10-17 07:00:00", "seq": 1, "count": 3, "source_time": {"value": "2026-10-17 07:00:00", "zone": "Europe/Berlin", "basis": "source"}}`)}
	b, err := g.Resolve(m)
	if err != nil {
		t.Fatal(err)
	}
	res, _, err := g.Commit(context.Background(), m, b)
	if err != nil || res.Outcome != gate.OutcomeAdmitted {
		t.Errorf("a type with no template: %+v, %v; want admitted as before", res, err)
	}
}

// A templated fact's time is at read at the offset source_time names, and its id
// carries the offset, so one minute said in two zones is two facts, the same minute
// said again is the one fact, and another name in that minute is its own.
func TestTemplate_OneMinuteInTwoZonesIsTwoFacts(t *testing.T) {
	t.Parallel()
	g, l := newTemplatedGate(t)
	const at = "2026-10-22 21:40"
	berlin := templated(at, "deskcam", "")
	sydney := fmt.Sprintf(`{"at": %q, "name": "deskcam", "amount": 5, "unit": "mg", %s}`, at, templatedClock(at, "Australia/Sydney", "+11:00"))

	a, err := sayTemplated(t, g, berlin)
	if err != nil || a.Outcome != gate.OutcomeAdmitted {
		t.Fatalf("berlin: %+v, %v", a, err)
	}
	b, err := sayTemplated(t, g, sydney)
	if err != nil || b.Outcome != gate.OutcomeAdmitted {
		t.Fatalf("sydney: %+v, %v", b, err)
	}
	if a.FacetID == b.FacetID {
		t.Fatalf("one minute in two zones is one facet, %s", a.FacetID)
	}
	wantA := facet.NewID(facet.SourceRef("conversation", "conversation:templated/deskcam/2026-10-22T21:40+02:00"))
	wantB := facet.NewID(facet.SourceRef("conversation", "conversation:templated/deskcam/2026-10-22T21:40+11:00"))
	if a.FacetID != wantA || b.FacetID != wantB {
		t.Errorf("ids = %s, %s; want %s, %s", a.FacetID, b.FacetID, wantA, wantB)
	}
	again, err := sayTemplated(t, g, berlin)
	if err != nil || again.Outcome != gate.OutcomeNoOp || again.FacetID != a.FacetID {
		t.Errorf("saying the same minute again = %+v, %v; want a no-op on %s", again, err, a.FacetID)
	}
	other, err := sayTemplated(t, g, templated(at, "garden", ""))
	if err != nil || other.Outcome != gate.OutcomeAdmitted || other.FacetID == a.FacetID {
		t.Errorf("another name, same minute = %+v, %v; want its own facet", other, err)
	}

	res, err := fold.Read(context.Background(), l)
	if err != nil {
		t.Fatal(err)
	}
	times := map[string]string{}
	for _, e := range res.Value.Entries() {
		times[e.FacetID] = e.FactTime
	}
	if times[a.FacetID] != "2026-10-22T19:40:00.000000000Z" || times[b.FacetID] != "2026-10-22T10:40:00.000000000Z" {
		t.Errorf("fact times = %v; want 21:40 at +02:00 and at +11:00 placed as two instants", times)
	}
}

// A version that requires source_time refuses a payload without it, and leaves the
// ledger as it was.
func TestTemplate_NoSourceTimeIsRefused(t *testing.T) {
	t.Parallel()
	g, l := newTemplatedGate(t)
	res, err := sayTemplated(t, g, `{"at": "2026-10-22 08:15", "name": "tea", "amount": 1, "unit": "cup"}`)
	if err == nil || !strings.Contains(err.Error(), "source_time") {
		t.Fatalf("got %+v, %v; want a refusal naming source_time", res, err)
	}
	got, err := fold.Read(context.Background(), l)
	if err != nil || len(got.Live) != 0 {
		t.Errorf("a refused payload left %d live facets (%v)", len(got.Live), err)
	}
}
