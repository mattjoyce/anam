package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/mattjoyce/anam/internal/capture/documents"
	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/gate"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// The documents source through the real gate, here for the same reason the records
// source is: a capture package may not import the store, and what is under test is
// the wiring — the bundled doc.reference Definition, the scope in mvpScopes, and the
// worker, together.

func writeDoc(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// The facet is a reference, a re-scan is a no-op, changed bytes supersede, and a
// renamed file is a new facet with the old one standing.
func TestDocuments_ThroughTheGate(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	writeDoc(t, filepath.Join(dir, "a.pdf"), "version one")
	writeDoc(t, filepath.Join(dir, "a.pdf.meta.yaml"), "date: 2026-01-30\n")

	l, err := ledger.Create(ctx, filepath.Join(t.TempDir(), "ledger.db"), "matt")
	if err != nil {
		t.Fatalf("ledger.Create: %v", err)
	}
	defer func() { _ = l.Close() }()
	defs, err := def.Bundled()
	if err != nil {
		t.Fatalf("def.Bundled: %v", err)
	}
	g := gate.New(l, defs, mvpScopes, clock.Real{}, "matt")

	admit := func() []gate.Result {
		t.Helper()
		envs, err := documents.Scan(dir, "matt")
		if err != nil {
			t.Fatalf("Scan: %v", err)
		}
		var out []gate.Result
		for _, env := range envs {
			b, err := g.Resolve(env)
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			r, _, err := g.Commit(ctx, env, b)
			if err != nil {
				t.Fatalf("Commit %s: %v", env.SourceRef, err)
			}
			out = append(out, r)
		}
		return out
	}

	first := admit()
	if first[0].Outcome != gate.OutcomeAdmitted {
		t.Fatalf("first scan = %s", first[0].Outcome)
	}
	f, err := l.CurrentVersion(ctx, first[0].FacetID)
	if err != nil {
		t.Fatalf("CurrentVersion: %v", err)
	}
	if f.Descriptor.PayloadKind != facet.PayloadReference || f.Descriptor.MediaType != "application/json" {
		t.Errorf("descriptor = %s %s, want reference application/json", f.Descriptor.PayloadKind, f.Descriptor.MediaType)
	}
	if again := admit(); again[0].Outcome != gate.OutcomeNoOp {
		t.Errorf("re-scan = %s, want no-op", again[0].Outcome)
	}

	writeDoc(t, filepath.Join(dir, "a.pdf"), "version two")
	if changed := admit(); changed[0].Outcome != gate.OutcomeSuperseded || changed[0].ArtifactVersion != 2 {
		t.Errorf("changed bytes = %s v%d, want superseded v2", changed[0].Outcome, changed[0].ArtifactVersion)
	}

	if err := os.Rename(filepath.Join(dir, "a.pdf"), filepath.Join(dir, "b.pdf")); err != nil {
		t.Fatalf("rename: %v", err)
	}
	if err := os.Rename(filepath.Join(dir, "a.pdf.meta.yaml"), filepath.Join(dir, "b.pdf.meta.yaml")); err != nil {
		t.Fatalf("rename sidecar: %v", err)
	}
	renamed := admit()
	if renamed[0].Outcome != gate.OutcomeAdmitted || renamed[0].FacetID == first[0].FacetID {
		t.Errorf("renamed = %s %s, want a new facet admitted", renamed[0].Outcome, renamed[0].FacetID)
	}
	if head, _ := l.Head(ctx); head != 3 {
		t.Errorf("head = %d, want 3 (admit, supersede, admit)", head)
	}
}
