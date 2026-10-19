package mint_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/gate"
	"github.com/mattjoyce/anam/internal/mint"
)

// useRegistry rebuilds the harness gate over a wider registry and scope.
func (h *harness) useRegistry(t *testing.T, reg *def.Registry, extra map[string][]facet.Type) {
	t.Helper()
	scopes := gate.StaticScopes{
		"bp-csv": {"health.blood-pressure"},
		"scale":  {"health.weight"},
	}
	for source, types := range extra {
		scopes[source] = types
	}
	h.gate = gate.New(h.l, reg, scopes, clock.Real{}, person, gate.Limits{MaxPayloadBytes: config.DefaultMaxPayloadBytes, MaxRetries: config.DefaultCommitRetries})
}

func medManifest(typ string, closes bool) string {
	return fmt.Sprintf(`{
  "meta": {
    "facet_type": %q,
    "definition_version": 1,
    "classification": "Sensitive",
    "validation_mode": "strict",
    "source_schema": "records/%s",
    "time_semantics": {"fact_time": {"pointer": "/date", "layout": "2006-01-02", "zone": "UTC"}},
    "identity_key": ["/id"],
    "series": {"name": "med.regimen", "key": ["/drug", "/course"], "closes": %t}
  },
  "schema": {"type": "object", "required": ["id", "date", "drug", "course"],
    "properties": {"id": {"type": "string", "minLength": 1}, "date": {"type": "string", "minLength": 1}, "drug": {"type": "string", "minLength": 1}, "course": {"type": "number"}, "dose": {"type": "string", "minLength": 1}},
    "additionalProperties": false}
}`, typ, typ, closes)
}

// medRegistry is the three regimen types beside the harness's two.
func medRegistry(t *testing.T) *def.Registry {
	t.Helper()
	var defs []def.Definition
	for _, m := range []string{bpManifest, weightManifest,
		medManifest("health.medication-started", false), medManifest("health.medication-dose-changed", false), medManifest("health.medication-stopped", true)} {
		d, err := def.Parse([]byte(m))
		if err != nil {
			t.Fatalf("def.Parse: %v", err)
		}
		defs = append(defs, d)
	}
	reg, err := def.NewRegistry(defs...)
	if err != nil {
		t.Fatalf("def.NewRegistry: %v", err)
	}
	return reg
}

func (h *harness) med(t *testing.T, typ facet.Type, id, date, drug string, course int) string {
	t.Helper()
	return h.admit(t, "self-log", typ, facet.SourceRef("self-log", id),
		fmt.Sprintf(`{"id": %q, "date": %q, "drug": %q, "course": %d}`, id, date, drug, course))
}

// The equality oracle for the snapshot (SELF-LOG.md §8): the snapshot at D from the
// hard checkpoints composed up to D's bucket equals the snapshot from a full fold,
// and both equal the answer worked by hand over a seeded regimen — a start, a dose
// change, a stop, and a restart as a new course.
func TestSnapshot_FromComposedCheckpointsEqualsTheFold(t *testing.T) {
	h := newHarness(t)
	reg := medRegistry(t)
	h.useRegistry(t, reg, map[string][]facet.Type{
		"self-log": {"health.medication-started", "health.medication-dose-changed", "health.medication-stopped"},
	})

	h.weight(t, "w-1", "2026-04-02T07:00:00Z", 81.0)
	h.med(t, "health.medication-started", "m1", "2026-03-01", "ramipril", 1)
	h.med(t, "health.medication-dose-changed", "m2", "2026-05-10", "ramipril", 1)
	h.med(t, "health.medication-stopped", "m3", "2026-08-01", "ramipril", 1)
	h.med(t, "health.medication-started", "m4", "2026-10-01", "ramipril", 2)
	h.run(t, false)

	full, err := fold.Read(context.Background(), h.l)
	if err != nil {
		t.Fatalf("fold.Read: %v", err)
	}

	cases := []struct {
		at     string
		bucket string   // the prefix of hard checkpoints up to D's bucket
		want   []string // source refs, in value order
	}{
		{"2026-04-15T00:00:00.000000000Z", "bucket:2026-04", []string{facet.SourceRef("self-log", "m1"), facet.SourceRef("scale", "w-1")}},
		{"2026-06-15T00:00:00.000000000Z", "bucket:2026-06", []string{facet.SourceRef("scale", "w-1"), facet.SourceRef("self-log", "m2")}},
		{"2026-09-01T00:00:00.000000000Z", "bucket:2026-09", []string{facet.SourceRef("scale", "w-1")}},
		{"2026-11-01T00:00:00.000000000Z", "bucket:2026-11", []string{facet.SourceRef("scale", "w-1"), facet.SourceRef("self-log", "m4")}},
	}
	for _, c := range cases {
		t.Run(c.at[:10], func(t *testing.T) {
			fromFold, err := fold.Snapshot(full.Value, c.at, reg.Closes)
			if err != nil {
				t.Fatalf("Snapshot(fold): %v", err)
			}
			fromCheckpoints, err := fold.Snapshot(h.composePrefix(t, c.bucket), c.at, reg.Closes)
			if err != nil {
				t.Fatalf("Snapshot(checkpoints): %v", err)
			}
			if fromFold.Hash() != fromCheckpoints.Hash() {
				t.Errorf("snapshot at %s: fold %s, composed checkpoints %s", c.at[:10], fromFold.Hash(), fromCheckpoints.Hash())
			}
			var got []string
			for _, e := range fromFold.Entries() {
				got = append(got, e.SourceRef)
			}
			if fmt.Sprint(got) != fmt.Sprint(c.want) {
				t.Errorf("snapshot at %s = %v, want %v", c.at[:10], got, c.want)
			}
		})
	}
}

// composePrefix loads every head at or before the named bucket and composes them.
func (h *harness) composePrefix(t *testing.T, upto string) fold.Value {
	t.Helper()
	var values []fold.Value
	for id, row := range h.heads(t) {
		if id > upto {
			continue
		}
		v, err := mint.Load(h.deps, row)
		if err != nil {
			t.Fatalf("Load %s: %v", id, err)
		}
		values = append(values, v)
	}
	composed, err := mint.Compose(values...)
	if err != nil {
		t.Fatalf("Compose: %v", err)
	}
	return composed
}

// The bitemporal case (SELF-LOG.md §8): the snapshot at D as Anam knew it before the
// stop row arrived. --as-of picks the canonical, --at picks the point within it.
func TestSnapshot_AsOfBeforeTheStopArrived(t *testing.T) {
	h := newHarness(t)
	reg := medRegistry(t)
	h.useRegistry(t, reg, map[string][]facet.Type{
		"self-log": {"health.medication-started", "health.medication-dose-changed", "health.medication-stopped"},
	})
	ctx := context.Background()

	h.med(t, "health.medication-started", "m1", "2026-03-01", "ramipril", 1)
	h.med(t, "health.medication-dose-changed", "m2", "2026-05-10", "ramipril", 1)
	beforeStop, err := h.l.Head(ctx)
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	h.med(t, "health.medication-stopped", "m3", "2026-08-01", "ramipril", 1)

	const at = "2026-09-01T00:00:00.000000000Z"
	now, err := fold.Read(ctx, h.l)
	if err != nil {
		t.Fatalf("fold.Read: %v", err)
	}
	then, err := fold.ReadAt(ctx, h.l, beforeStop)
	if err != nil {
		t.Fatalf("fold.ReadAt: %v", err)
	}
	snapNow, _ := fold.Snapshot(now.Value, at, reg.Closes)
	snapThen, _ := fold.Snapshot(then.Value, at, reg.Closes)

	if snapNow.Len() != 0 {
		t.Errorf("at September, knowing of the stop: %d entries, want none", snapNow.Len())
	}
	if snapThen.Len() != 1 || snapThen.Entries()[0].SourceRef != facet.SourceRef("self-log", "m2") {
		t.Errorf("at September, before the stop was known: %v, want the dose change", snapThen.Entries())
	}
}
