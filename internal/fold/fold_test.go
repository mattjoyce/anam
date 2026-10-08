package fold_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/fold"
)

// ev builds an anam-event; the fold reads only Kind, FacetID and the two versions.
func ev(kind facet.AnamEventKind, id string, version, prior int) facet.AnamEvent {
	return facet.AnamEvent{Kind: kind, FacetID: id, ArtifactVersion: version, PriorArtifactVersion: prior}
}

func TestFold_DerivesEachStatus(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		events []facet.AnamEvent
		want   fold.State
	}{
		{"admitted", []facet.AnamEvent{
			ev(facet.EventAdmitted, "a", 1, 0),
		}, fold.State{Version: 1, Status: facet.StatusAdmitted}},

		// The load-bearing arm of the kind→status map: a superseding event lands the
		// facet *admitted* at the higher version, never "superseded". The cast
		// facet.Status(ev.Kind) would get exactly this case wrong.
		{"superseded re-admits at the new version", []facet.AnamEvent{
			ev(facet.EventAdmitted, "a", 1, 0),
			ev(facet.EventSuperseded, "a", 2, 1),
		}, fold.State{Version: 2, Status: facet.StatusAdmitted}},

		{"tombstoned", []facet.AnamEvent{
			ev(facet.EventAdmitted, "a", 1, 0),
			ev(facet.EventTombstoned, "a", 1, 0),
		}, fold.State{Version: 1, Status: facet.StatusTombstoned}},

		// The R3 gate could append a second tombstone; the fold absorbs it.
		{"double tombstone is idempotent", []facet.AnamEvent{
			ev(facet.EventAdmitted, "a", 1, 0),
			ev(facet.EventTombstoned, "a", 1, 0),
			ev(facet.EventTombstoned, "a", 1, 0),
		}, fold.State{Version: 1, Status: facet.StatusTombstoned}},

		// A tombstoned facet is not gone from the source's point of view: a changed
		// row re-read supersedes it, and the facet is live again at the new version.
		{"supersession resurrects a tombstoned facet", []facet.AnamEvent{
			ev(facet.EventAdmitted, "a", 1, 0),
			ev(facet.EventTombstoned, "a", 1, 0),
			ev(facet.EventSuperseded, "a", 2, 1),
		}, fold.State{Version: 2, Status: facet.StatusAdmitted}},

		{"endorsed", []facet.AnamEvent{
			ev(facet.EventAdmitted, "a", 1, 0),
			ev(facet.EventEndorsed, "a", 1, 0),
		}, fold.State{Version: 1, Status: facet.StatusEndorsed}},

		{"rejected", []facet.AnamEvent{
			ev(facet.EventAdmitted, "a", 1, 0),
			ev(facet.EventRejected, "a", 1, 0),
		}, fold.State{Version: 1, Status: facet.StatusRejected}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			states, err := fold.Fold(tc.events)
			if err != nil {
				t.Fatalf("Fold: %v", err)
			}
			if got := states["a"]; got != tc.want {
				t.Errorf("folded to %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestFold_Liveness(t *testing.T) {
	t.Parallel()

	live := []facet.Status{facet.StatusAdmitted, facet.StatusEndorsed}
	dead := []facet.Status{facet.StatusSuperseded, facet.StatusTombstoned, facet.StatusRejected}
	for _, s := range live {
		if !(fold.State{Version: 1, Status: s}).Live() {
			t.Errorf("%s must be live", s)
		}
	}
	for _, s := range dead {
		if (fold.State{Version: 1, Status: s}).Live() {
			t.Errorf("%s must not be live", s)
		}
	}
}

// The fold trusts nothing: a stream the gate could not have written stops the read.
func TestFold_RefusesCorruptStreams(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		events  []facet.AnamEvent
		wantMsg string
	}{
		{"re-admission", []facet.AnamEvent{
			ev(facet.EventAdmitted, "a", 1, 0),
			ev(facet.EventAdmitted, "a", 1, 0),
		}, "already folded"},
		{"first sight above v1", []facet.AnamEvent{
			ev(facet.EventAdmitted, "a", 2, 0),
		}, "must be v1"},
		{"supersede before admit", []facet.AnamEvent{
			ev(facet.EventSuperseded, "a", 2, 1),
		}, "before it was admitted"},
		{"supersede skipping a version", []facet.AnamEvent{
			ev(facet.EventAdmitted, "a", 1, 0),
			ev(facet.EventSuperseded, "a", 3, 2),
		}, "over folded state"},
		{"tombstone before admit", []facet.AnamEvent{
			ev(facet.EventTombstoned, "a", 1, 0),
		}, "before it was admitted"},
		{"tombstone of a displaced version", []facet.AnamEvent{
			ev(facet.EventAdmitted, "a", 1, 0),
			ev(facet.EventSuperseded, "a", 2, 1),
			ev(facet.EventTombstoned, "a", 1, 0),
		}, "folded version"},
		{"unknown kind", []facet.AnamEvent{
			ev("published", "a", 1, 0),
		}, "unknown anam-event kind"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := fold.Fold(tc.events)
			if !errors.Is(err, fold.ErrCorruptStream) {
				t.Fatalf("got %v, want ErrCorruptStream", err)
			}
			if !strings.Contains(err.Error(), tc.wantMsg) {
				t.Errorf("error %q does not name the corruption (%q)", err, tc.wantMsg)
			}
		})
	}
}

func TestFold_FacetsFoldIndependently(t *testing.T) {
	t.Parallel()

	states, err := fold.Fold([]facet.AnamEvent{
		ev(facet.EventAdmitted, "a", 1, 0),
		ev(facet.EventAdmitted, "b", 1, 0),
		ev(facet.EventTombstoned, "a", 1, 0),
		ev(facet.EventSuperseded, "b", 2, 1),
	})
	if err != nil {
		t.Fatalf("Fold: %v", err)
	}
	if got := states["a"]; got != (fold.State{Version: 1, Status: facet.StatusTombstoned}) {
		t.Errorf("a = %+v", got)
	}
	if got := states["b"]; got != (fold.State{Version: 2, Status: facet.StatusAdmitted}) {
		t.Errorf("b = %+v", got)
	}
	if _, ok := states["never-admitted"]; ok {
		t.Error("a facet with no events has no state")
	}
}

// StateAfter must agree with Apply on every event Apply accepts: it is how a composed
// read applies a tail without the history, and a disagreement would be a second
// account of the same fold.
func TestStateAfterAgreesWithApply(t *testing.T) {
	t.Parallel()
	events := []facet.AnamEvent{
		{Kind: facet.EventAdmitted, FacetID: "a", ArtifactVersion: 1},
		{Kind: facet.EventSuperseded, FacetID: "a", ArtifactVersion: 2, PriorArtifactVersion: 1},
		{Kind: facet.EventEndorsed, FacetID: "a", ArtifactVersion: 2},
		{Kind: facet.EventSuperseded, FacetID: "a", ArtifactVersion: 3, PriorArtifactVersion: 2},
		{Kind: facet.EventRejected, FacetID: "a", ArtifactVersion: 3},
		{Kind: facet.EventTombstoned, FacetID: "a", ArtifactVersion: 3},
	}
	var cur fold.State
	known := false
	for _, ev := range events {
		want, err := fold.Apply(cur, known, ev)
		if err != nil {
			t.Fatalf("Apply %s: %v", ev.Kind, err)
		}
		got, err := fold.StateAfter(ev)
		if err != nil || got != want {
			t.Errorf("after %s: StateAfter = %+v, %v; Apply = %+v", ev.Kind, got, err, want)
		}
		cur, known = want, true
	}
	if _, err := fold.StateAfter(facet.AnamEvent{Kind: "bogus"}); err == nil {
		t.Error("an unknown kind was accepted")
	}
}
