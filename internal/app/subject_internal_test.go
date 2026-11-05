package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/store/ledger"
)

// TestInit_SubjectPinUnderEitherConfigName is card 270a in the engine: a ledger
// initialised under the old variable is the same ledger under the new one, under both,
// and refused under another subject. The pin is the value, not the key that named it.
//
// Not parallel: it sets ANAM_SUBJECT_ID and ANAM_PERSON_ID, which are process-wide.
func TestInit_SubjectPinUnderEitherConfigName(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	cfg := fmt.Sprintf("ledger_path: %s\ndb_path: %s\ntimeout: %s\n",
		filepath.Join(dir, "ledger.db"), filepath.Join(dir, "tracking.db"), testTimeout) + catalogKeys(t)
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("ANAM_SUBJECT_ID", "")
	t.Setenv("ANAM_PERSON_ID", "matt")
	out, _, err := cli(t, cfgPath, "init")
	if err != nil {
		t.Fatalf("init under ANAM_PERSON_ID: %v\n%s", err, out)
	}
	if !strings.Contains(out, "ready (subject matt)") {
		t.Errorf("init output does not name the subject:\n%s", out)
	}

	for _, env := range []struct{ name, subject, person string }{
		{"the new variable", "matt", ""},
		{"both, equal", "matt", "matt"},
	} {
		t.Setenv("ANAM_SUBJECT_ID", env.subject)
		t.Setenv("ANAM_PERSON_ID", env.person)
		out, _, err := cli(t, cfgPath, "status")
		if err != nil {
			t.Fatalf("status under %s: %v", env.name, err)
		}
		if !strings.Contains(out, "(subject matt)") {
			t.Errorf("status under %s does not print (subject matt):\n%s", env.name, out)
		}
	}

	t.Setenv("ANAM_SUBJECT_ID", "other")
	t.Setenv("ANAM_PERSON_ID", "")
	_, _, err = cli(t, cfgPath, "status")
	if !errors.Is(err, ledger.ErrSubjectMismatch) {
		t.Fatalf("status under another subject = %v, want ledger.ErrSubjectMismatch", err)
	}
	if want := `ledger is pinned to "matt", config says "other"`; !strings.Contains(err.Error(), want) {
		t.Errorf("error = %v, want it to read %s", err, want)
	}
}
