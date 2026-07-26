package ledger_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// CurrentVersion answers "the latest", which is what the gate needs. Version answers
// "exactly this one", which is what an as-of read needs — and the difference only
// shows once a facet has been superseded, which is what this test builds.
func TestVersion_ReturnsTheExactArtifact(t *testing.T) {
	t.Parallel()
	l := newLedger(t)
	ctx := context.Background()

	const ref = "bp-csv:2026-01-24T06:30:00"
	v1 := testFacet(t, ref, `{"systolic": 118, "diastolic": 76}`, 1)
	if err := l.Admit(ctx, 0, v1, admittedEvent(v1, 1)); err != nil {
		t.Fatalf("Admit v1: %v", err)
	}

	v2 := testFacet(t, ref, `{"systolic": 121, "diastolic": 79}`, 2)
	superseded := facet.AnamEvent{
		Kind:                 facet.EventSuperseded,
		FacetID:              v2.ID,
		AnamTime:             v2.FactTime.Add(3 * time.Hour),
		Seq:                  2,
		ArtifactVersion:      2,
		PriorArtifactVersion: 1,
	}
	if err := l.Admit(ctx, 1, v2, superseded); err != nil {
		t.Fatalf("Admit v2: %v", err)
	}

	// The latest is v2 — that is what the gate compares against.
	current, err := l.CurrentVersion(ctx, v1.ID)
	if err != nil {
		t.Fatalf("CurrentVersion: %v", err)
	}
	if current.ArtifactVersion != 2 {
		t.Fatalf("CurrentVersion returned v%d, want v2", current.ArtifactVersion)
	}

	// The displaced version is still there, unchanged, and reachable by name. This is
	// what makes a read at a historical watermark possible: the ledger is append-only,
	// so an old artifact is not merely remembered, it is present.
	got, err := l.Version(ctx, v1.ID, 1)
	if err != nil {
		t.Fatalf("Version(v1): %v", err)
	}
	if got.ArtifactVersion != 1 {
		t.Errorf("Version(1) returned v%d", got.ArtifactVersion)
	}
	if string(got.Payload.Bytes()) != string(v1.Payload.Bytes()) {
		t.Errorf("Version(1) payload:\n got %s\nwant %s", got.Payload.Bytes(), v1.Payload.Bytes())
	}
	if got.Descriptor.ContentHash != v1.Descriptor.ContentHash {
		t.Errorf("Version(1) content hash: got %s, want %s", got.Descriptor.ContentHash, v1.Descriptor.ContentHash)
	}

	got2, err := l.Version(ctx, v1.ID, 2)
	if err != nil {
		t.Fatalf("Version(v2): %v", err)
	}
	if string(got2.Payload.Bytes()) != string(v2.Payload.Bytes()) {
		t.Errorf("Version(2) payload:\n got %s\nwant %s", got2.Payload.Bytes(), v2.Payload.Bytes())
	}
}

func TestVersion_NotFound(t *testing.T) {
	t.Parallel()
	l := newLedger(t)
	ctx := context.Background()

	f := testFacet(t, "bp-csv:2026-01-24T06:30:00", `{"systolic": 118}`, 1)
	if err := l.Admit(ctx, 0, f, admittedEvent(f, 1)); err != nil {
		t.Fatalf("Admit: %v", err)
	}

	if _, err := l.Version(ctx, f.ID, 2); !errors.Is(err, ledger.ErrNotFound) {
		t.Errorf("Version of a version never written: err = %v, want ErrNotFound", err)
	}
	if _, err := l.Version(ctx, "facet:nonexistent", 1); !errors.Is(err, ledger.ErrNotFound) {
		t.Errorf("Version of an unknown facet: err = %v, want ErrNotFound", err)
	}
	if _, err := l.Version(ctx, f.ID, 0); err == nil {
		t.Error("Version(0) was accepted; there is no version zero")
	}
}
