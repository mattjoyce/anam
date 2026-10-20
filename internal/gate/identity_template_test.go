package gate_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/gate"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// Card 188 S1: a Definition that declares an identity_template has its identity field
// filled when an envelope leaves it out and held to it when it carries it. Over the
// bundled health.medication-dose and a real ledger.

func newDoseGate(t *testing.T) *gate.Gate {
	t.Helper()
	l, err := ledger.Create(context.Background(), filepath.Join(t.TempDir(), "ledger.db"), person)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	reg, err := def.Bundled()
	if err != nil {
		t.Fatal(err)
	}
	scopes := gate.StaticScopes{"conversation": {"health.medication-dose", "health.weight"}}
	return gate.New(l, reg, scopes, clock.Real{}, person, gate.Limits{MaxPayloadBytes: config.DefaultMaxPayloadBytes, MaxRetries: config.DefaultCommitRetries})
}

func doseEnvelope(payload string) facet.Envelope {
	return facet.Envelope{Type: "health.medication-dose", Source: "conversation", Injector: "conversation", Payload: []byte(payload)}
}

const (
	doseID      = "conversation:med/melatonin/dose/2026-10-17T22:00"
	doseWithout = `{"taken_at": "2026-10-17 22:00", "drug": "melatonin", "amount": 5, "unit": "mg"}`
	doseWith    = `{"id": "` + doseID + `", "taken_at": "2026-10-17 22:00", "drug": "melatonin", "amount": 5, "unit": "mg"}`
)

func commitDose(t *testing.T, g *gate.Gate, payload string) (gate.Result, error) {
	t.Helper()
	env := doseEnvelope(payload)
	b, err := g.Resolve(env)
	if err != nil {
		t.Fatal(err)
	}
	res, _, err := g.Commit(context.Background(), env, b)
	return res, err
}

func TestTemplate_OmittedIdentityIsFilled(t *testing.T) {
	t.Parallel()
	g := newDoseGate(t)
	res, err := commitDose(t, g, doseWithout)
	if err != nil || res.Outcome != gate.OutcomeAdmitted {
		t.Fatalf("got %+v, %v; want admitted", res, err)
	}
	// The filled envelope is the same facet, and the same bytes, as one that carried
	// the id: a re-send with it is a no-op on the same facet id.
	again, err := commitDose(t, g, doseWith)
	if err != nil || again.Outcome != gate.OutcomeNoOp || again.FacetID != res.FacetID {
		t.Errorf("explicit id after filled = %+v, %v; want no-op on %s", again, err, res.FacetID)
	}
	if want := facet.NewID(facet.SourceRef("conversation", doseID)); res.FacetID != want {
		t.Errorf("facet id %s, want %s (as the hand-built id gave)", res.FacetID, want)
	}
}

func TestTemplate_CarriedIdentityMustMatch(t *testing.T) {
	t.Parallel()
	g := newDoseGate(t)
	if res, err := commitDose(t, g, doseWith); err != nil || res.Outcome != gate.OutcomeAdmitted {
		t.Fatalf("matching id: %+v, %v", res, err)
	}
	wrong := strings.Replace(doseWith, "T22:00", "T22:30", 1)
	_, err := commitDose(t, g, wrong)
	if !errors.Is(err, def.ErrIdentityField) {
		t.Fatalf("got %v, want ErrIdentityField", err)
	}
	if !strings.Contains(err.Error(), "T22:30") || !strings.Contains(err.Error(), doseID) {
		t.Errorf("error should show both ids: %v", err)
	}
}

func TestTemplate_ATypeWithoutOneIsUntouched(t *testing.T) {
	t.Parallel()
	g := newDoseGate(t)
	w := facet.Envelope{Type: "health.weight", Source: "conversation", Injector: "conversation",
		Payload: []byte(`{"id": "w-1", "date": "2026-10-17 07:00", "kg": 80}`)}
	b, err := g.Resolve(w)
	if err != nil {
		t.Fatal(err)
	}
	res, _, err := g.Commit(context.Background(), w, b)
	if err != nil || res.Outcome != gate.OutcomeAdmitted {
		t.Errorf("a type with no template: %+v, %v; want admitted as before", res, err)
	}
}
