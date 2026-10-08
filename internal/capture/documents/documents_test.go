package documents_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/capture/documents"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
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
	envs = identified(t, envs)
	if len(envs) != 2 {
		t.Fatalf("Scan returned %d envelopes, want 2", len(envs))
	}

	pdf := payloadOf(t, envs[0])
	sum := sha256.Sum256([]byte("%PDF-1.4 not really"))
	if envs[0].SourceRef != facet.SourceRef("documents", "2026-01-30_lipids.pdf") || envs[0].Type != documents.Type {
		t.Errorf("first envelope = %s %s", envs[0].Type, envs[0].SourceRef)
	}
	if pdf["path"] != "2026-01-30_lipids.pdf" || pdf["sha256"] != hex.EncodeToString(sum[:]) ||
		pdf["media_type"] != "application/pdf" || pdf["bytes"] != float64(19) ||
		pdf["date"] != "2026-01-30" || pdf["title"] != "Lipid studies" {
		t.Errorf("pdf payload = %v", pdf)
	}
	jpg := payloadOf(t, envs[1])
	if envs[1].SourceRef != facet.SourceRef("documents", "scans/bp.jpg") || jpg["media_type"] != "image/jpeg" || jpg["date"] != "2024-09-16" {
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

// identified gives each envelope the source_ref the gate admits it under. The
// worker leaves it empty; the Definition's identity_key composes it.
func identified(t *testing.T, envs []facet.Envelope) []facet.Envelope {
	t.Helper()
	reg, err := def.Bundled()
	if err != nil {
		t.Fatalf("Bundled: %v", err)
	}
	out := make([]facet.Envelope, len(envs))
	for i, env := range envs {
		ref, err := reg.Identify(env)
		if err != nil {
			t.Fatalf("envelope %d (%s): %v", i, env.Type, err)
		}
		env.SourceRef = ref
		out[i] = env
	}
	return out
}
