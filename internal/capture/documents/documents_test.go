package documents_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/capture/documents"
	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/gate"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func payloadOf(t *testing.T, env facet.Envelope) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(env.Payload, &m); err != nil {
		t.Fatalf("payload: %v", err)
	}
	return m
}

func TestScan_DescribesEachFileFromItsSidecar(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write(t, filepath.Join(dir, "2026-01-30_lipids.pdf"), "%PDF-1.4 not really")
	write(t, filepath.Join(dir, "2026-01-30_lipids.pdf.meta.yaml"), "date: 2026-01-30\ntitle: Lipid studies\n")
	write(t, filepath.Join(dir, "scans", "bp.jpg"), "JFIF bytes")
	write(t, filepath.Join(dir, "scans", "bp.jpg.meta.yaml"), "date: 2024-09-16\n")
	write(t, filepath.Join(dir, ".hidden", "x.pdf"), "skipped")
	write(t, filepath.Join(dir, ".DS_Store"), "skipped")

	envs, err := documents.Scan(dir, "matt")
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(envs) != 2 {
		t.Fatalf("Scan returned %d envelopes, want 2", len(envs))
	}

	pdf := payloadOf(t, envs[0])
	sum := sha256.Sum256([]byte("%PDF-1.4 not really"))
	if envs[0].SourceRef != "documents:2026-01-30_lipids.pdf" || envs[0].Type != documents.Type {
		t.Errorf("first envelope = %s %s", envs[0].Type, envs[0].SourceRef)
	}
	if pdf["path"] != "2026-01-30_lipids.pdf" || pdf["sha256"] != hex.EncodeToString(sum[:]) ||
		pdf["media_type"] != "application/pdf" || pdf["bytes"] != float64(19) ||
		pdf["date"] != "2026-01-30" || pdf["title"] != "Lipid studies" {
		t.Errorf("pdf payload = %v", pdf)
	}
	jpg := payloadOf(t, envs[1])
	if envs[1].SourceRef != "documents:scans/bp.jpg" || jpg["media_type"] != "image/jpeg" || jpg["date"] != "2024-09-16" {
		t.Errorf("jpg = %s %v", envs[1].SourceRef, jpg)
	}
}

func TestScan_RefusesAFileWithoutAFactTime(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write(t, filepath.Join(dir, "letter.pdf"), "bytes")

	_, err := documents.Scan(dir, "matt")
	if !errors.Is(err, documents.ErrNoSidecar) {
		t.Fatalf("Scan = %v, want ErrNoSidecar", err)
	}
	if !strings.Contains(err.Error(), "letter.pdf.meta.yaml") || !strings.Contains(err.Error(), "date: YYYY-MM-DD") {
		t.Errorf("the refusal does not say where the date goes: %v", err)
	}

	write(t, filepath.Join(dir, "letter.pdf.meta.yaml"), "title: no date\n")
	if _, err := documents.Scan(dir, "matt"); !errors.Is(err, documents.ErrBadSidecar) {
		t.Errorf("sidecar without date = %v, want ErrBadSidecar", err)
	}
	write(t, filepath.Join(dir, "letter.pdf.meta.yaml"), "date: 2026-01-01\nsha256: deadbeef\n")
	if _, err := documents.Scan(dir, "matt"); !errors.Is(err, documents.ErrBadSidecar) {
		t.Errorf("sidecar declaring the hash = %v, want ErrBadSidecar", err)
	}
}

// Through the real gate with the bundled Definition: the facet is a reference, a
// renamed file is a new facet, and changed bytes supersede the old one.
func TestScan_ThroughTheGate(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a.pdf"), "version one")
	write(t, filepath.Join(dir, "a.pdf.meta.yaml"), "date: 2026-01-30\n")

	l, err := ledger.Create(ctx, filepath.Join(t.TempDir(), "ledger.db"), "matt")
	if err != nil {
		t.Fatalf("ledger.Create: %v", err)
	}
	defer func() { _ = l.Close() }()
	defs, err := def.Bundled()
	if err != nil {
		t.Fatalf("def.Bundled: %v", err)
	}
	g := gate.New(l, defs, gate.StaticScopes{documents.Source: {documents.Type}}, clock.Real{}, "matt")

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

	// The bytes change: same path, new hash, superseded.
	write(t, filepath.Join(dir, "a.pdf"), "version two")
	if changed := admit(); changed[0].Outcome != gate.OutcomeSuperseded || changed[0].ArtifactVersion != 2 {
		t.Errorf("changed bytes = %s v%d, want superseded v2", changed[0].Outcome, changed[0].ArtifactVersion)
	}

	// The file is renamed: a new identity, admitted at v1; the old facet stands.
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
}
