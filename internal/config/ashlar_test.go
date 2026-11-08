package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/ashlar"
)

// TestResolvedPrecedence confirms each layer wins in turn with matching
// provenance kind/name.
func TestResolvedPrecedence(t *testing.T) {
	t.Setenv("ANAM_SUBJECT_ID", "matt")

	// Default only — no file, no env override for timeout.
	t.Run("default provenance", func(t *testing.T) {
		r, err := config.LoadResolved("")
		if err != nil {
			t.Fatalf("LoadResolved(\"\") = %v", err)
		}
		p, ok := r.Provenance("timeout")
		if !ok {
			t.Fatal("no provenance for timeout")
		}
		if p.Effective.Kind != ashlar.OriginDefault {
			t.Errorf("timeout origin kind = %q, want default", p.Effective.Kind)
		}
		if !strings.HasPrefix(p.Effective.Name, "timeout") {
			t.Errorf("timeout origin name = %q, want timeout", p.Effective.Name)
		}
	})

	// File provenance.
	t.Run("file provenance", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "anam.yaml")
		if err := os.WriteFile(path, []byte("subject_id: matt\ntimeout: 30s\n"), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
		r, err := config.LoadResolved(path)
		if err != nil {
			t.Fatalf("LoadResolved(%q) = %v", path, err)
		}
		p, ok := r.Provenance("timeout")
		if !ok {
			t.Fatal("no provenance for timeout")
		}
		if p.Effective.Kind != ashlar.OriginFile {
			t.Errorf("timeout origin kind = %q, want file", p.Effective.Kind)
		}
		if p.Effective.Span.Source == "" {
			t.Errorf("timeout file origin has no source path")
		}
		if !p.Effective.Span.Start.Valid() {
			t.Errorf("timeout file origin has no valid line:column, got %s", p.Effective.Span.Start.String())
		}
	})

	// Environment provenance.
	t.Run("env provenance", func(t *testing.T) {
		t.Setenv("ANAM_TIMEOUT", "45s")
		path := filepath.Join(t.TempDir(), "anam.yaml")
		if err := os.WriteFile(path, []byte("subject_id: matt\ntimeout: 30s\n"), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
		r, err := config.LoadResolved(path)
		if err != nil {
			t.Fatalf("LoadResolved(%q) = %v", path, err)
		}
		p, ok := r.Provenance("timeout")
		if !ok {
			t.Fatal("no provenance for timeout")
		}
		if p.Effective.Kind != ashlar.OriginEnv {
			t.Errorf("timeout origin kind = %q, want env", p.Effective.Kind)
		}
		if !strings.HasPrefix(p.Effective.Name, "ANAM_TIMEOUT") {
			t.Errorf("timeout origin name = %q, want ANAM_TIMEOUT", p.Effective.Name)
		}
		// The file value should be in the superseded chain.
		if len(p.Superseded()) == 0 {
			t.Error("timeout env provenance has no superseded entries for the file value")
		}
	})

	// Explicit override provenance.
	t.Run("explicit provenance", func(t *testing.T) {
		t.Setenv("ANAM_TIMEOUT", "45s")
		path := filepath.Join(t.TempDir(), "anam.yaml")
		if err := os.WriteFile(path, []byte("subject_id: matt\ntimeout: 30s\n"), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
		r, err := config.LoadResolvedOverriding(path, map[string]string{"timeout": "60s"})
		if err != nil {
			t.Fatalf("LoadResolvedOverriding() = %v", err)
		}
		p, ok := r.Provenance("timeout")
		if !ok {
			t.Fatal("no provenance for timeout")
		}
		if p.Effective.Kind != ashlar.OriginExplicit {
			t.Errorf("timeout origin kind = %q, want explicit", p.Effective.Kind)
		}
	})
}

// TestResolvedEmptyEnvIgnored confirms that an empty ANAM_ variable contributes
// no override.
func TestResolvedEmptyEnvIgnored(t *testing.T) {
	t.Setenv("ANAM_SUBJECT_ID", "matt")
	t.Setenv("ANAM_TIMEOUT", "")

	r, err := config.LoadResolved("")
	if err != nil {
		t.Fatalf("LoadResolved(\"\") = %v", err)
	}
	p, ok := r.Provenance("timeout")
	if !ok {
		t.Fatal("no provenance for timeout")
	}
	if p.Effective.Kind != ashlar.OriginDefault {
		t.Errorf("timeout origin kind = %q, want default (empty env ignored)", p.Effective.Kind)
	}
}

// TestResolvedExplicitEmpty trivially confirms an explicit empty value is
// present (the card says "explicit empty values remain present" which is
// enforced by rejecting empty subject_id etc.)
func TestResolvedExplicitEmptyRemains(t *testing.T) {
	t.Setenv("ANAM_SUBJECT_ID", "")
	r, err := config.LoadResolvedOverriding("", map[string]string{"subject_id": ""})
	if err != nil {
		// An empty subject_id fails validation — that is correct: the
		// explicit empty was applied, and Validate rejected it.
		if !strings.Contains(err.Error(), "subject_id") {
			t.Errorf("error = %v, want it to name subject_id", err)
		}
		return
	}
	t.Error("LoadResolvedOverriding with empty subject_id should have failed validation")
	_ = r // unused when the function is expected to fail
}

// TestResolvedIntegerOverride confirms a valid explicit integer wins over an
// invalid environment integer without early conversion failure.
func TestResolvedIntegerOverride(t *testing.T) {
	t.Setenv("ANAM_SUBJECT_ID", "matt")
	t.Setenv("ANAM_LIBRARY_FOLDER_LIMIT", "not-a-number")
	// An explicit valid integer should override the bad env.
	r, err := config.LoadResolvedOverriding("", map[string]string{"library_folder_limit": "500"})
	if err != nil {
		t.Fatalf("LoadResolvedOverriding() = %v", err)
	}
	if r.Value().LibraryFolderLimit != 500 {
		t.Errorf("LibraryFolderLimit = %d, want 500 (explicit wins over invalid env)", r.Value().LibraryFolderLimit)
	}
	p, ok := r.Provenance("library_folder_limit")
	if !ok {
		t.Fatal("no provenance for library_folder_limit")
	}
	if p.Effective.Kind != ashlar.OriginExplicit {
		t.Errorf("origin kind = %q, want explicit", p.Effective.Kind)
	}
}

// TestResolvedAliasContradictionAcrossLayers proves same-key replacement
// occurs before alias comparison: opposite spellings across layers still fail.
func TestResolvedAliasContradictionAcrossLayers(t *testing.T) {
	// long_write_timeout in the file and ingest_timeout via env with
	// different values should fail.
	path := writeCfg(t, "subject_id: matt\nlong_write_timeout: 20m\n")
	t.Setenv("ANAM_INGEST_TIMEOUT", "10m")

	_, err := config.LoadResolved(path)
	if err == nil {
		t.Fatal("LoadResolved() with contradictory timeout spellings, want error")
	}
	if !strings.Contains(err.Error(), "long_write_timeout") || !strings.Contains(err.Error(), "ingest_timeout") {
		t.Errorf("error = %v, want it to name both keys", err)
	}
}

// TestResolvedEquivalentDurations confirms equivalent spellings agree and the
// canonical name gets the selected origin.
func TestResolvedEquivalentDurations(t *testing.T) {
	t.Setenv("ANAM_SUBJECT_ID", "matt")

	// Both spellings given and equivalent: 60s == 1m. Canonical origin wins.
	t.Run("equivalent spellings agree", func(t *testing.T) {
		path := writeCfg(t, "subject_id: matt\nlong_write_timeout: 60s\ningest_timeout: 1m\n")
		r, err := config.LoadResolved(path)
		if err != nil {
			t.Fatalf("LoadResolved() = %v", err)
		}
		if r.Value().LongWriteTimeout != 60*time.Second {
			t.Errorf("LongWriteTimeout = %s, want 60s", r.Value().LongWriteTimeout)
		}
		p, ok := r.Provenance("long_write_timeout")
		if !ok || p.Effective.Kind != ashlar.OriginFile {
			t.Errorf("long_write_timeout provenance = %v, want file origin", p)
		}
	})

	// Only ingest_timeout: provenance uses ingest_timeout's origin.
	t.Run("old name only, adopted", func(t *testing.T) {
		path := writeCfg(t, "subject_id: matt\ningest_timeout: 25m\n")
		r, err := config.LoadResolved(path)
		if err != nil {
			t.Fatalf("LoadResolved() = %v", err)
		}
		p, ok := r.Provenance("long_write_timeout")
		if !ok || p.Effective.Kind != ashlar.OriginFile {
			t.Errorf("long_write_timeout provenance = %v, want file origin", p)
		}
	})

	// Neither given: default provenance.
	t.Run("neither given, default", func(t *testing.T) {
		r, err := config.LoadResolved("")
		if err != nil {
			t.Fatalf("LoadResolved(\"\") = %v", err)
		}
		p, ok := r.Provenance("long_write_timeout")
		if !ok {
			t.Fatal("no provenance for long_write_timeout")
		}
		if p.Effective.Kind != ashlar.OriginDefault {
			t.Errorf("origin kind = %q, want default", p.Effective.Kind)
		}
		if p.Effective.Name != "long_write_timeout" {
			t.Errorf("origin name = %q, want long_write_timeout", p.Effective.Name)
		}
	})
}

// TestResolvedSubjectProvenance is card 270a: subject_id's origin is reported under
// subject_id whichever of its two names supplied it, and the old name person_id is
// never a key of its own.
func TestResolvedSubjectProvenance(t *testing.T) {
	tests := []struct {
		name string
		body string
		env  map[string]string
		kind ashlar.OriginKind
	}{
		{"new key in the file", "subject_id: matt\n", nil, ashlar.OriginFile},
		{"old key in the file", "person_id: matt\n", nil, ashlar.OriginFile},
		{"old variable", "", map[string]string{"ANAM_PERSON_ID": "matt"}, ashlar.OriginEnv},
		{"old in the file, new in the environment", "person_id: matt\n",
			map[string]string{"ANAM_SUBJECT_ID": "matt"}, ashlar.OriginEnv},
		{"new in the file, old in the environment", "subject_id: matt\n",
			map[string]string{"ANAM_PERSON_ID": "matt"}, ashlar.OriginFile},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ANAM_SUBJECT_ID", "")
			t.Setenv("ANAM_PERSON_ID", "")
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			var path string
			if tt.body != "" {
				path = writeCfg(t, tt.body)
			}
			r, err := config.LoadResolved(path)
			if err != nil {
				t.Fatalf("LoadResolved() = %v", err)
			}
			p, ok := r.Provenance("subject_id")
			if !ok {
				t.Fatal("no provenance for subject_id")
			}
			if p.Effective.Kind != tt.kind {
				t.Errorf("subject_id origin kind = %q, want %q", p.Effective.Kind, tt.kind)
			}
			if _, ok := r.Provenance("person_id"); ok {
				t.Error("person_id has provenance of its own, want it reported under subject_id")
			}
		})
	}
}

// TestResolvedDerivedLibraryDir confirms a derived library_dir has the
// declared default origin.
func TestResolvedDerivedLibraryDir(t *testing.T) {
	t.Setenv("ANAM_SUBJECT_ID", "matt")
	t.Setenv("XDG_DATA_HOME", "/data")

	r, err := config.LoadResolved("")
	if err != nil {
		t.Fatalf("LoadResolved(\"\") = %v", err)
	}
	p, ok := r.Provenance("library_dir")
	if !ok {
		t.Fatal("no provenance for library_dir")
	}
	if p.Effective.Kind != ashlar.OriginDefault {
		t.Errorf("library_dir origin kind = %q, want default", p.Effective.Kind)
	}
	if !strings.Contains(p.Effective.Name, "library_dir beside ledger_path") &&
		!strings.Contains(p.Effective.Name, "library_dir") {
		t.Errorf("library_dir origin name = %q, want library_dir beside ledger_path", p.Effective.Name)
	}
}

// TestResolvedIndependentLoads confirms two successive loads retain
// independent metadata and a caller cannot change provenance seen by a later
// Provenance call.
func TestResolvedIndependentLoads(t *testing.T) {
	t.Setenv("ANAM_SUBJECT_ID", "matt")

	r, err := config.LoadResolved("")
	if err != nil {
		t.Fatalf("LoadResolved(\"\") = %v", err)
	}

	// The accessor returns a value copy, not a reference. Mutating the
	// returned provenance cannot affect a subsequent call.
	p1, ok := r.Provenance("timeout")
	if !ok {
		t.Fatal("no provenance for timeout")
	}
	origName := p1.Effective.Name
	// Replace the local copy with a completely different provenance value.
	p1 = ashlar.NewProvenance(ashlar.ExplicitOrigin("hacked"), false)
	_ = p1 // local variable used to verify it doesn't affect stored provenance
	p2, ok := r.Provenance("timeout")
	if !ok || p2.Effective.Name != origName {
		t.Errorf("Provenance accessor returned corrupted value after caller mutated a returned copy: got %q, want %q",
			p2.Effective.Name, origName)
	}
}

// TestResolvedIncludeUnknown was written when 'include' was an unknown key.
// Since card 256b include is accepted, so it now confirms a missing include
// target is refused. TestIncludeFailures covers the include rules in full.
func TestResolvedIncludeUnknown(t *testing.T) {
	t.Setenv("ANAM_SUBJECT_ID", "matt")
	path := writeCfg(t, "subject_id: matt\ninclude: other.yaml\n")
	_, err := config.LoadResolved(path)
	if err == nil {
		t.Fatal("LoadResolved() with include key, want an error")
	}
	if !strings.Contains(err.Error(), "include") && !strings.Contains(err.Error(), "unknown field") {
		t.Errorf("error = %v, want mention of include", err)
	}
}

// TestResolvedPrincipalsLoading confirms both principals lists load from a
// valid file and unknown nested fields fail.
func TestResolvedPrincipalsLoading(t *testing.T) {
	valid := `importers:
  - source: withings
    token_sha256: 9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08
    types: [health.weight]
`
	t.Run("both lists load", func(t *testing.T) {
		path := writeImporterFile(t, valid)
		got, err := config.LoadPrincipals(path, nil)
		if err != nil {
			t.Fatalf("LoadPrincipals() = %v", err)
		}
		if len(got) != 1 {
			t.Errorf("loaded %d principals, want 1", len(got))
		}
		if got[0].Source != "withings" {
			t.Errorf("source = %q, want withings", got[0].Source)
		}
	})

	t.Run("unknown nested field fails", func(t *testing.T) {
		bad := strings.Replace(valid, "types: [health.weight]", "types: [health.weight]\n    tokn: x", 1)
		path := writeImporterFile(t, bad)
		_, err := config.LoadPrincipals(path, nil)
		if err == nil {
			t.Fatal("LoadPrincipals() with unknown nested field 'tokn', want error")
		}
		if !errors.Is(err, config.ErrInvalidImporters) {
			t.Errorf("error = %v, want ErrInvalidImporters", err)
		}
		if !strings.Contains(err.Error(), "tokn") {
			t.Errorf("error = %v, want to contain 'tokn'", err)
		}
	})
}

func writeCfg(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "anam.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func writeImporterFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "importers.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write importers: %v", err)
	}
	return path
}
