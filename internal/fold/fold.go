// Package fold derives current state from the anam-event stream — the inversion
// made executable (ARCHITECTURE.md §9). A Facet stores no current status; this
// package is the only place one comes from, so the ledger can never drift from its
// own events.
//
// Fold is pure: anam-events in, per-facet states out. Read composes it with the
// ledger's two access paths to produce the canonical — the live-only value at a
// watermark (§7) — whose content hash is the equality oracle for the rebuild test.
package fold

import (
	"errors"
	"fmt"

	"github.com/mattjoyce/anam/internal/facet"
)

// ErrCorruptStream is returned when the anam-event stream is not one the gate
// could have written. The stream is the error kernel's output; a hole in it is not
// repaired here, it is reported and everything stops (AGENTS.md, Armstrong).
var ErrCorruptStream = errors.New("fold: anam-event stream is not one the gate could have written")

// State is one logical facet's folded position: the artifact version currently in
// force, and the status that version holds.
//
// StatusSuperseded never appears here. Only an artifact *version* is ever
// superseded; the logical facet it belongs to is simultaneously re-admitted at the
// next version, so the displaced version simply stops being the one State points at.
type State struct {
	Version int
	Status  facet.Status
}

// Live reports whether the facet counts into the canonical (§9): admitted and
// endorsed are live; superseded, tombstoned and rejected are excluded from the value.
func (s State) Live() bool {
	return s.Status == facet.StatusAdmitted || s.Status == facet.StatusEndorsed
}

// Fold replays anam-events, in seq order, into each logical facet's State.
// Events up to the head watermark yield current state; events up to a historical
// watermark yield the as-of state — same fold, different upper bound.
func Fold(events []facet.AnamEvent) (map[string]State, error) {
	states := make(map[string]State)
	for _, ev := range events {
		cur, known := states[ev.FacetID]
		next, err := Apply(cur, known, ev)
		if err != nil {
			return nil, err
		}
		states[ev.FacetID] = next
	}
	return states, nil
}

// Apply advances one facet's folded state by one anam-event. It is exported so
// that an *incremental* consumer — the operational store's catch-up, which persists
// each facet's State as its index row — replays new events through the same map
// rather than growing a second, drifting one. known is false only when the facet
// has no state yet (its first event).
//
// This switch is the kind→status map, spelled out case by case. It must never be
// abbreviated to facet.Status(ev.Kind): that conversion compiles — the two types
// share spellings — and it encodes a lie. A "superseded" event means a new artifact
// version displaced the prior one, so the facet it lands on is *admitted* at the
// higher version; the cast would mark the whole facet superseded and silently drop
// every corrected fact from the canonical. Spelling the arms out also means a sixth
// event kind lands in the default arm and fails loudly instead of flowing through a
// cast unexamined.
func Apply(cur State, known bool, ev facet.AnamEvent) (State, error) {
	switch ev.Kind {
	case facet.EventAdmitted:
		// Only a first sight admits; every later capture of the row supersedes.
		if known {
			return State{}, fmt.Errorf("%w: facet %s admitted at v%d but already folded to %+v",
				ErrCorruptStream, ev.FacetID, ev.ArtifactVersion, cur)
		}
		if ev.ArtifactVersion != 1 {
			return State{}, fmt.Errorf("%w: facet %s admitted at v%d, first sight must be v1",
				ErrCorruptStream, ev.FacetID, ev.ArtifactVersion)
		}
		return State{Version: 1, Status: facet.StatusAdmitted}, nil

	case facet.EventSuperseded:
		if !known {
			return State{}, fmt.Errorf("%w: facet %s superseded before it was admitted",
				ErrCorruptStream, ev.FacetID)
		}
		if ev.PriorArtifactVersion != cur.Version || ev.ArtifactVersion != cur.Version+1 {
			return State{}, fmt.Errorf("%w: facet %s superseded v%d→v%d over folded state %+v",
				ErrCorruptStream, ev.FacetID, ev.PriorArtifactVersion, ev.ArtifactVersion, cur)
		}
		// The displaced version is superseded; the facet is re-admitted at the new one.
		return State{Version: ev.ArtifactVersion, Status: facet.StatusAdmitted}, nil

	case facet.EventTombstoned:
		if err := mustConcernCurrent(cur, known, ev); err != nil {
			return State{}, err
		}
		// A repeated tombstone is tolerated, not rejected: the R3 gate appended one
		// per Retract before this fold existed to refuse the second, and the ledger
		// keeps what it was given. Idempotent here, refused at the gate from R4 on.
		return State{Version: cur.Version, Status: facet.StatusTombstoned}, nil

	case facet.EventEndorsed:
		if err := mustConcernCurrent(cur, known, ev); err != nil {
			return State{}, err
		}
		return State{Version: cur.Version, Status: facet.StatusEndorsed}, nil

	case facet.EventRejected:
		if err := mustConcernCurrent(cur, known, ev); err != nil {
			return State{}, err
		}
		return State{Version: cur.Version, Status: facet.StatusRejected}, nil

	default:
		return State{}, fmt.Errorf("%w: unknown anam-event kind %q on facet %s",
			ErrCorruptStream, ev.Kind, ev.FacetID)
	}
}

// StateAfter is the state ev leaves its facet in, whatever came before: every event
// names the artifact version it concerns, and its kind decides the status. It is what
// lets a reader apply a tail of events to a checkpoint without the facets' history.
// It does not check the stream; Apply does, on the full fold, and the two must agree
// on every event Apply accepts.
func StateAfter(ev facet.AnamEvent) (State, error) {
	var st facet.Status
	switch ev.Kind {
	case facet.EventAdmitted, facet.EventSuperseded:
		st = facet.StatusAdmitted
	case facet.EventTombstoned:
		st = facet.StatusTombstoned
	case facet.EventEndorsed:
		st = facet.StatusEndorsed
	case facet.EventRejected:
		st = facet.StatusRejected
	default:
		return State{}, fmt.Errorf("%w: unknown anam-event kind %q on facet %s",
			ErrCorruptStream, ev.Kind, ev.FacetID)
	}
	return State{Version: ev.ArtifactVersion, Status: st}, nil
}

// mustConcernCurrent checks that a pure status transition names the artifact version
// actually in force — the gate stamps the current version into every Mark, so any
// other version is a stream nothing wrote.
func mustConcernCurrent(cur State, known bool, ev facet.AnamEvent) error {
	if !known {
		return fmt.Errorf("%w: facet %s marked %s before it was admitted",
			ErrCorruptStream, ev.FacetID, ev.Kind)
	}
	if ev.ArtifactVersion != cur.Version {
		return fmt.Errorf("%w: facet %s marked %s at v%d but folded version is v%d",
			ErrCorruptStream, ev.FacetID, ev.Kind, ev.ArtifactVersion, cur.Version)
	}
	return nil
}
