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

// Card 200: a carry admits a facet another ledger holds as the facet it is, held to the
// Definition version it was admitted under. The id, hash and fact time come out as they
// were, the pillars still run, and the concession reaches nothing a commit can say.

// bpManifestV2 is bpManifest at v2 with an optional pulse: the Definition in force when
// the old ledger's facet is carried, so the facet's own version is not the type's.
var bpManifestV2 = strings.NewReplacer(`"definition_version": 1`, `"definition_version": 2`,
	`"diastolic": {"type": "number"},`, `"diastolic": {"type": "number"}, "pulse": {"type": "number"},`).Replace(bpManifest)

func newLedgerAt(t *testing.T) *ledger.Ledger {
	t.Helper()
	l, err := ledger.Create(context.Background(), filepath.Join(t.TempDir(), "ledger.db"), person)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	return l
}

func registryOf(t *testing.T, manifests ...string) *def.Registry {
	t.Helper()
	var ds []def.Definition
	for _, m := range manifests {
		d, err := def.Parse([]byte(m))
		if err != nil {
			t.Fatal(err)
		}
		ds = append(ds, d)
	}
	reg, err := def.NewRegistry(ds...)
	if err != nil {
		t.Fatal(err)
	}
	return reg
}

var carryLimits = gate.Limits{MaxPayloadBytes: config.DefaultMaxPayloadBytes, MaxRetries: config.DefaultCommitRetries}

// admittedAtV1 is a facet admitted under bpManifest v1 into its own ledger, as stored.
func admittedAtV1(t *testing.T) facet.Facet {
	t.Helper()
	l := newLedgerAt(t)
	g := gate.New(l, registryOf(t, bpManifest), gate.StaticScopes{"bp-csv": {"health.blood-pressure"}}, clock.Real{}, person, carryLimits)
	env := bpEnvelope(bpPayload)
	b, err := g.Resolve(env)
	if err != nil {
		t.Fatal(err)
	}
	res, _, err := g.Commit(context.Background(), env, b)
	if err != nil {
		t.Fatal(err)
	}
	f, err := l.CurrentVersion(context.Background(), res.FacetID)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func carryOfStored(f facet.Facet) gate.Carry {
	return gate.Carry{
		Env: facet.Envelope{Type: f.Type, Source: f.Provenance.Source, Injector: f.Provenance.Injector,
			SourceRef: f.Provenance.SourceRef, Payload: f.Payload.Bytes()},
		Binding: f.Binding, DefinitionVersion: f.DefinitionVersion,
	}
}

func TestCarry_AdmitsTheFacetUnderTheVersionItWasAdmittedUnder(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	old := admittedAtV1(t)

	current := registryOf(t, bpManifestV2)
	held := def.NewVersions(current, []def.Stored{
		{Type: "health.blood-pressure", Version: 1, Body: []byte(bpManifest)},
		{Type: "health.blood-pressure", Version: 2, Body: []byte(bpManifestV2)},
	})
	l := newLedgerAt(t)
	g := gate.New(l, current, gate.StaticScopes{"bp-csv": {"health.blood-pressure"}}, clock.Real{}, person, carryLimits).Holding(held)

	// The dry run's question writes nothing and answers the facet that would be built.
	vet, err := g.VetCarry(carryOfStored(old))
	if err != nil {
		t.Fatalf("VetCarry: %v", err)
	}
	if vet.ID != old.ID || vet.Descriptor.ContentHash != old.Descriptor.ContentHash || !vet.FactTime.Equal(old.FactTime) || vet.DefinitionVersion != 1 {
		t.Errorf("VetCarry built %s hash %s v%d, want %s %s v1", vet.ID, vet.Descriptor.ContentHash, vet.DefinitionVersion, old.ID, old.Descriptor.ContentHash)
	}
	if w, _ := l.Head(ctx); w != 0 {
		t.Fatalf("VetCarry wrote: W %d", w)
	}

	results, w, err := g.CarryBatch(ctx, []gate.Carry{carryOfStored(old)})
	if err != nil || w != 1 || results[0].Outcome != gate.OutcomeAdmitted {
		t.Fatalf("CarryBatch: %+v, W %d, %v", results, w, err)
	}
	got, err := l.CurrentVersion(ctx, old.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.DefinitionVersion != 1 || got.Descriptor != old.Descriptor || got.Binding != old.Binding || got.Provenance != old.Provenance || !got.FactTime.Equal(old.FactTime) {
		t.Errorf("carried facet %+v, want the old one's fields under definition v1", got)
	}

	// The same carry again is a no-op, and a plain commit of the payload is still
	// held to the Definition in force.
	if results, w, err = g.CarryBatch(ctx, []gate.Carry{carryOfStored(old)}); err != nil || w != 1 || results[0].Outcome != gate.OutcomeNoOp {
		t.Errorf("second CarryBatch: %+v, W %d, %v; want a no-op", results, w, err)
	}
	env := bpEnvelope(bpPayload)
	env.DefinitionVersion = 1
	if _, _, err := g.Commit(ctx, env, old.Binding); !errors.Is(err, gate.ErrDefinitionVersion) {
		t.Errorf("a commit shaped to v1 under a gate that holds versions: %v, want ErrDefinitionVersion", err)
	}
}

func TestCarry_StillRunsEveryPillar(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	old := admittedAtV1(t)
	current := registryOf(t, bpManifestV2)
	held := def.NewVersions(current, []def.Stored{{Type: "health.blood-pressure", Version: 1, Body: []byte(bpManifest)}})
	scopes := gate.StaticScopes{"bp-csv": {"health.blood-pressure"}}

	cases := []struct {
		name  string
		gate  *gate.Gate
		carry func(gate.Carry) gate.Carry
		want  error
	}{
		{"a gate that holds no versions", gate.New(newLedgerAt(t), current, scopes, clock.Real{}, person, carryLimits), nil, gate.ErrNoDefinition},
		{"a version nothing holds", gate.New(newLedgerAt(t), current, scopes, clock.Real{}, person, carryLimits).Holding(held),
			func(c gate.Carry) gate.Carry { c.DefinitionVersion = 9; return c }, gate.ErrNoDefinition},
		{"a source that is not scoped to the type", gate.New(newLedgerAt(t), current, gate.StaticScopes{}, clock.Real{}, person, carryLimits).Holding(held), nil, gate.ErrNotAuthorized},
		{"a payload the version refuses", gate.New(newLedgerAt(t), current, scopes, clock.Real{}, person, carryLimits).Holding(held),
			func(c gate.Carry) gate.Carry {
				c.Env.Payload = []byte(strings.Replace(string(c.Env.Payload), `"comment"`, `"pulse": 60, "comment"`, 1))
				return c
			}, nil},
		{"a source ref the identity key does not compose", gate.New(newLedgerAt(t), current, scopes, clock.Real{}, person, carryLimits).Holding(held),
			func(c gate.Carry) gate.Carry { c.Env.SourceRef = "bp-csv:somewhere-else"; return c }, gate.ErrIdentityMismatch},
	}
	for _, c := range cases {
		carry := carryOfStored(old)
		if c.carry != nil {
			carry = c.carry(carry)
		}
		_, _, err := c.gate.CarryBatch(ctx, []gate.Carry{carry})
		var refused *gate.Refused
		switch {
		case err == nil:
			t.Errorf("%s: admitted", c.name)
		case !errors.As(err, &refused):
			t.Errorf("%s: %v, want the batch refused", c.name, err)
		case c.want != nil && !errors.Is(err, c.want):
			t.Errorf("%s: %v, want %v", c.name, err, c.want)
		}
	}
}
