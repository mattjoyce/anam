package def_test

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/mattjoyce/anam/internal/catalog/catalogtest"
	"github.com/mattjoyce/anam/internal/facet"
)

// Identify composes the ref the gate admits under, from the payload alone: a ref the
// envelope carries does not change the answer, and a type with no Definition has none.
func TestRegistry_Identify(t *testing.T) {
	t.Parallel()
	reg := catalogtest.Engine.Definitions(t)
	payload := []byte(`{"path":"scans/bp.jpg","sha256":"00","media_type":"image/jpeg","bytes":10,"date":"2024-09-16"}`)
	want := facet.SourceRef("documents", "scans/bp.jpg")
	for _, carried := range []string{"", want, "documents:something-else"} {
		env := facet.Envelope{Type: "record.document", Source: "documents", SourceRef: carried, Payload: payload}
		if got, err := reg.Identify(env); err != nil || got != want {
			t.Errorf("carrying %q: Identify = %q, %v; want %q", carried, got, err, want)
		}
	}
	if _, err := reg.Identify(facet.Envelope{Type: "no.such", Source: "documents", Payload: payload}); err == nil {
		t.Error("Identify composed a ref for a type with no Definition")
	}
}

// The source_time shapes are copied into every Definition that reads a clock, and
// read by people and agents as what the field is. They say the thing itself and never
// which kanban card made it. A catalog's own words are its deployment's to check.
func TestSourceTime_WordsNameNoCard(t *testing.T) {
	t.Parallel()
	cardRef := regexp.MustCompile(`(?i)\bcards?[ -]\d`)
	files, err := filepath.Glob(filepath.Join("sourcetime", "*.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("glob sourcetime: %v (%d files)", err, len(files))
	}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		if loc := cardRef.FindIndex(raw); loc != nil {
			t.Errorf("%s names a card: %q", f, raw[max(0, loc[0]-40):loc[1]+20])
		}
	}
}
