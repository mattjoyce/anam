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

// Card 140: a rename carries a facet into another type as the next version of
// itself, same id, same source and identity values, through the gate. The payload is
// unchanged, so the hash is equal and a plain commit would read as a no-op; the type
// differs, so a plain commit would be refused (card 105). Only Rehearse and
// RenameBatch may do it, and only when the facet's current type is the one the
// rename says it is leaving.

// pressureManifest is bpManifest under another name with the same keying and schema:
// the shape of a rename, where the type moves and nothing else does.
var pressureManifest = strings.Replace(bpManifest, `"health.blood-pressure"`, `"health.pressure"`, 1)

func newRenameGate(t *testing.T) (*gate.Gate, *ledger.Ledger) {
	t.Helper()
	ctx := context.Background()
	l, err := ledger.Create(ctx, filepath.Join(t.TempDir(), "ledger.db"), person)
	if err != nil {
		t.Fatalf("ledger.Create: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })
	var defs []def.Definition
	for _, m := range []string{bpManifest, pressureManifest, pulseManifest} {
		d, err := def.Parse([]byte(m))
		if err != nil {
			t.Fatal(err)
		}
		defs = append(defs, d)
	}
	reg, err := def.NewRegistry(defs...)
	if err != nil {
		t.Fatal(err)
	}
	scopes := gate.StaticScopes{"bp-csv": {"health.blood-pressure", "health.pressure", "health.pulse"}}
	return gate.New(l, reg, scopes, clock.Real{}, person, gate.Limits{MaxPayloadBytes: config.DefaultMaxPayloadBytes, MaxRetries: config.DefaultCommitRetries}), l
}

// admitReading admits bpPayload as a blood pressure and returns the facet as stored.
func admitReading(t *testing.T, g *gate.Gate, l *ledger.Ledger) facet.Facet {
	t.Helper()
	ctx := context.Background()
	env := bpEnvelope(bpPayload)
	b, err := g.Resolve(env)
	if err != nil {
		t.Fatal(err)
	}
	res, _, err := g.Commit(ctx, env, b)
	if err != nil {
		t.Fatalf("admit the reading: %v", err)
	}
	f, err := l.CurrentVersion(ctx, res.FacetID)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// renameOf is the rename of a stored facet into another type: its source, binding and
// payload as they are, under the new type, leaving the type it holds.
func renameOf(f facet.Facet, to facet.Type) gate.Rename {
	return gate.Rename{
		Env:     facet.Envelope{Type: to, Source: f.Provenance.Source, Injector: "rename", Payload: f.Payload.Bytes()},
		Binding: f.Binding,
		From:    f.Type,
	}
}

func TestRename_IsTheNextVersionUnderTheNewType(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	g, l := newRenameGate(t)
	f := admitReading(t, g, l)
	before, _ := l.Head(ctx)

	// Rehearse answers what the run would do, and writes nothing.
	r, err := g.Rehearse(ctx, renameOf(f, "health.pressure"))
	if err != nil || r.Outcome != gate.OutcomeSuperseded || r.ArtifactVersion != 2 || r.FacetID != f.ID {
		t.Fatalf("Rehearse = %+v, %v; want superseded v2 of %s", r, err, f.ID)
	}
	if w, _ := l.Head(ctx); w != before {
		t.Errorf("Rehearse wrote: W %d → %d", before, w)
	}

	results, w, err := g.RenameBatch(ctx, []gate.Rename{renameOf(f, "health.pressure")})
	if err != nil {
		t.Fatalf("RenameBatch: %v", err)
	}
	if results[0].Outcome != gate.OutcomeSuperseded || results[0].FacetID != f.ID || w != before+1 {
		t.Fatalf("rename = %+v at W %d; want superseded %s at W %d", results[0], w, f.ID, before+1)
	}
	cur, err := l.CurrentVersion(ctx, f.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cur.Type != "health.pressure" || cur.ArtifactVersion != 2 {
		t.Errorf("current is %s v%d, want health.pressure v2", cur.Type, cur.ArtifactVersion)
	}
	if cur.Provenance.SourceRef != f.Provenance.SourceRef || cur.Descriptor.ContentHash != f.Descriptor.ContentHash {
		t.Errorf("the rename changed the source ref or the payload hash")
	}
	// Anam-time is kept: the first version is still there, as it was admitted.
	old, err := l.Version(ctx, f.ID, 1)
	if err != nil || old.Type != "health.blood-pressure" {
		t.Errorf("v1 = %s, %v; want the blood pressure as admitted", old.Type, err)
	}

	// The run again, as the command would send it (leaving the table's old type), is
	// a no-op and moves nothing.
	again := renameOf(cur, "health.pressure")
	again.From = "health.blood-pressure"
	results, w2, err := g.RenameBatch(ctx, []gate.Rename{again})
	if err != nil || results[0].Outcome != gate.OutcomeNoOp || w2 != w {
		t.Errorf("rerun = %+v at W %d, %v; want no-op at W %d", results, w2, err, w)
	}
}

// The mark is checked: a rename that names the wrong type as the one left is the
// collision card 105 refuses, a plain commit under the new type has no mark and is
// refused as before, and a rename into a type the payload does not fit is refused as
// any non-compliant envelope is. None of them writes.
func TestRename_RefusesWhatIsNotTheTypeLeft(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	g, l := newRenameGate(t)
	f := admitReading(t, g, l)
	before, _ := l.Head(ctx)

	wrong := renameOf(f, "health.pressure")
	wrong.From = "health.pulse"
	var refused *gate.Refused
	if _, _, err := g.RenameBatch(ctx, []gate.Rename{wrong}); !errors.As(err, &refused) || !errors.Is(err, gate.ErrTypeChanged) {
		t.Errorf("the wrong type left: %v, want a refusal with ErrTypeChanged", err)
	}
	if _, err := g.Rehearse(ctx, wrong); !errors.Is(err, gate.ErrTypeChanged) {
		t.Errorf("Rehearse with the wrong type left: %v, want ErrTypeChanged", err)
	}
	plain := wrong.Env
	b, _ := g.Resolve(plain)
	if _, _, err := g.Commit(ctx, plain, b); !errors.Is(err, gate.ErrTypeChanged) {
		t.Errorf("Commit under the new type: %v, want ErrTypeChanged (card 105)", err)
	}
	if _, err := g.Rehearse(ctx, renameOf(f, "health.pulse")); err == nil || errors.Is(err, gate.ErrTypeChanged) {
		t.Errorf("rename into a type the payload does not fit: %v, want a compliance refusal", err)
	}
	if w, _ := l.Head(ctx); w != before {
		t.Errorf("the refusals wrote: W %d → %d", before, w)
	}
}
