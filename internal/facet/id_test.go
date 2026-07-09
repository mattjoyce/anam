package facet_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/mattjoyce/anam/internal/facet"
)

// TestNewIDIsDeterministic is the property idempotent re-ingest rests on: the same
// source row maps to the same logical facet, forever and across processes.
func TestNewIDIsDeterministic(t *testing.T) {
	t.Parallel()

	const sourceRef = "bp-csv:2026-01-24T06:30:00:118/76/61"

	first := facet.NewID(sourceRef)
	if second := facet.NewID(sourceRef); first != second {
		t.Errorf("NewID(%q) is not deterministic: %q then %q", sourceRef, first, second)
	}

	// Pinned: the namespace must never change, so this id must never change.
	const want = "4e96b06f-f9e7-50f1-a631-764181c08cad"
	if first != want {
		t.Errorf("NewID(%q) = %q, want %q — has the namespace UUID changed?", sourceRef, first, want)
	}
}

func TestNewIDDistinguishesSourceRefs(t *testing.T) {
	t.Parallel()

	refs := []string{
		"bp-csv:2026-01-24T06:30:00:118/76/61",
		"bp-csv:2026-01-24T06:31:00:118/76/61", // different time
		"bp-csv:2026-01-24T06:30:00:121/76/61", // different reading
		"health-db:sleep:2026-06-06T00:00:00",
		"",
	}

	seen := make(map[string]string, len(refs))
	for _, ref := range refs {
		id := facet.NewID(ref)
		if prior, dup := seen[id]; dup {
			t.Errorf("NewID collided: %q and %q both yield %q", prior, ref, id)
		}
		seen[id] = ref
	}
}

func TestNewIDIsAUUIDv5(t *testing.T) {
	t.Parallel()

	id, err := uuid.Parse(facet.NewID("health-db:sleep:2026-06-06T00:00:00"))
	if err != nil {
		t.Fatalf("NewID() did not produce a parseable UUID: %v", err)
	}
	if got := id.Version(); got != 5 {
		t.Errorf("NewID() UUID version = %d, want 5 (name-based, SHA-1)", got)
	}
	if got := id.Variant(); got != uuid.RFC4122 {
		t.Errorf("NewID() UUID variant = %v, want RFC4122", got)
	}
}
