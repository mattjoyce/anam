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

func includeFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, body := range files {
		file := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(file), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return filepath.Join(root, "config.yaml")
}

func TestIncludePrecedence(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, root, child, nested, sibling string
		want                               time.Duration
		source                             string
	}{
		{"root", "timeout: 11s", "", "", "", 11 * time.Second, "config.yaml"},
		{"child", "timeout: 11s\ninclude: sub/child.yaml", "timeout: 22s", "", "", 22 * time.Second, "sub/child.yaml"},
		{"nested", "timeout: 11s\ninclude: sub/child.yaml", "timeout: 22s\ninclude: nested.yaml", "timeout: 33s", "", 33 * time.Second, "sub/nested.yaml"},
		{"later sibling", "timeout: 11s\ninclude: [sub/child.yaml, local.yaml]", "timeout: 22s\ninclude: nested.yaml", "timeout: 33s", "timeout: 44s", 44 * time.Second, "local.yaml"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			file := includeFiles(t, map[string]string{"config.yaml": "subject_id: test\n" + tt.root, "sub/child.yaml": tt.child, "sub/nested.yaml": tt.nested, "local.yaml": tt.sibling})
			r, err := config.LoadResolved(file)
			if err != nil {
				t.Fatal(err)
			}
			if r.Value().Timeout != tt.want {
				t.Fatalf("timeout = %s, want %s", r.Value().Timeout, tt.want)
			}
			p, ok := r.ProvenanceAt("/timeout")
			if !ok || p.Effective.Span.Source != tt.source {
				t.Fatalf("origin = %+v, want %s", p, tt.source)
			}
		})
	}
}

func TestIncludeScalarOverrides(t *testing.T) {
	// Changes environment variables to exercise the supported scalar layers.
	file := includeFiles(t, map[string]string{"config.yaml": "subject_id: test\ntimeout: 11s\ninclude: child.yaml", "child.yaml": "timeout: 22s\nbackup_keep: 23"})
	for _, tt := range []struct {
		name, env string
		explicit  map[string]string
		want      time.Duration
		kind      ashlar.OriginKind
	}{
		{"empty env", "", nil, 22 * time.Second, ashlar.OriginFile},
		{"env", "33s", nil, 33 * time.Second, ashlar.OriginEnv},
		{"explicit", "33s", map[string]string{"timeout": "44s", "backup_keep": "45"}, 44 * time.Second, ashlar.OriginExplicit},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ANAM_TIMEOUT", tt.env)
			r, err := config.LoadResolvedOverriding(file, tt.explicit)
			if err != nil {
				t.Fatal(err)
			}
			p, _ := r.Provenance("timeout")
			if r.Value().Timeout != tt.want || p.Effective.Kind != tt.kind {
				t.Fatalf("timeout = %s, origin = %+v", r.Value().Timeout, p)
			}
			if tt.explicit != nil && r.Value().BackupKeep != 45 {
				t.Fatal("explicit integer lost")
			}
		})
	}
}

func TestIncludeValidationOrigin(t *testing.T) {
	t.Parallel()
	file := includeFiles(t, map[string]string{
		"config.yaml":     "subject_id: test\ninclude: sub/child.yaml\n",
		"sub/child.yaml":  "# child\ninclude: nested.yaml\n",
		"sub/nested.yaml": "# original line one\ninclude: null\n\ntimeout: 0s\n",
	})
	_, err := config.LoadResolved(file)
	var diagnostic *ashlar.Error
	if !errors.Is(err, config.ErrInvalid) || !errors.As(err, &diagnostic) {
		t.Fatalf("error = %v", err)
	}
	if diagnostic.Pointer != "/timeout" || diagnostic.Origin.Span.Source != "sub/nested.yaml" || diagnostic.Origin.Span.Start.Line != 4 {
		t.Fatalf("wrong diagnostic = %v", err)
	}
	via := diagnostic.Origin.Via()
	if len(via) != 2 || via[0].Source != "config.yaml" || via[0].Start.Line != 2 || via[1].Source != "sub/child.yaml" || via[1].Start.Line != 2 {
		t.Fatalf("include chain = %+v", via)
	}
	if !strings.Contains(err.Error(), "sub/nested.yaml:4:") {
		t.Fatalf("formatted origin missing = %v", err)
	}
}

func TestIncludeNullCompatibility(t *testing.T) {
	t.Parallel()
	for _, body := range []string{"", "# comment\n", "null\n", "~\n", "---\n", "include: null\n", "include: ~\n", "{include: null, timeout: null}", "include: !!null |\n  null\n"} {
		t.Run(body, func(t *testing.T) {
			t.Parallel()
			file := includeFiles(t, map[string]string{"config.yaml": "subject_id: test\ntimeout: 17s\ninclude: child.yaml\n", "child.yaml": body})
			r, err := config.LoadResolved(file)
			if err != nil {
				t.Fatal(err)
			}
			if r.Value().Timeout != 17*time.Second {
				t.Fatalf("child changed timeout to %s", r.Value().Timeout)
			}
		})
	}
	for _, body := range []string{"subject_id: test\ninclude: null\n\ntimeout: 0s\n", "{subject_id: test, include: null, timeout: 0s}"} {
		t.Run("root "+body, func(t *testing.T) {
			t.Parallel()
			file := includeFiles(t, map[string]string{"config.yaml": body})
			_, err := config.LoadResolved(file)
			var diagnostic *ashlar.Error
			if !errors.As(err, &diagnostic) || diagnostic.Pointer != "/timeout" || diagnostic.Origin.Span.Source != "config.yaml" {
				t.Fatalf("error = %v", err)
			}
			wantLine := 4
			if strings.HasPrefix(body, "{") {
				wantLine = 1
			}
			if diagnostic.Origin.Span.Start.Line != wantLine {
				t.Fatalf("error = %v, want line %d", err, wantLine)
			}
		})
	}
}

func TestIncludeFailures(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, root, child, nested string
		category                  error
	}{
		{"missing", "include: missing.yaml", "", "", ashlar.ErrNotFound},
		{"cycle", "include: sub/child.yaml", "include: /config.yaml", "", ashlar.ErrIncludeCycle},
		{"repeated", "include: [sub/child.yaml, sub/child.yaml]", "", "", ashlar.ErrIncludeCycle},
		{"diamond", "include: [sub/child.yaml, sub/nested.yaml]", "include: nested.yaml", "", ashlar.ErrIncludeCycle},
		{"lexical escape", "include: ../outside.yaml", "", "", ashlar.ErrComposition},
		{"absolute lexical escape", "include: /../outside.yaml", "", "", ashlar.ErrComposition},
		{"nested lexical escape", "include: sub/child.yaml", "include: ../../outside.yaml", "", ashlar.ErrComposition},
		{"null array entry", "include: [null]", "", "", ashlar.ErrComposition},
		{"child null array entry", "include: sub/child.yaml", "include: [nested.yaml, null]", "", ashlar.ErrComposition},
		{"kind conflict", "timeout: {}\ninclude: sub/child.yaml", "timeout: 1s", "", ashlar.ErrComposition},
		{"inspector unknown", "include: sub/child.yaml", "inspector: {}", "", ashlar.ErrUnknownField},
		{"duplicate before null strip", "include: sub/child.yaml", "include: null\ninclude: null", "", ashlar.ErrDuplicateKey},
		{"alias before null strip", "include: sub/child.yaml", "include: null\ntimeout: &t 3s\nzone: *t", "", ashlar.ErrSyntax},
		{"documents before null strip", "include: sub/child.yaml", "include: null\n---\n{}", "", ashlar.ErrTrailingContent},
		{"blank first document", "include: sub/child.yaml", "---\n---\n{}", "", ashlar.ErrTrailingContent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			file := includeFiles(t, map[string]string{"config.yaml": "subject_id: test\n" + tt.root, "sub/child.yaml": tt.child, "sub/nested.yaml": tt.nested})
			got, err := config.LoadResolved(file)
			if !errors.Is(err, config.ErrInvalid) || !errors.Is(err, tt.category) {
				t.Fatalf("error = %v, want %v", err, tt.category)
			}
			if got.Value().SubjectID != "" {
				t.Fatal("failed load returned a partial config")
			}
		})
	}
}

func TestIncludeSymlinkConfinement(t *testing.T) {
	t.Parallel()
	for _, directory := range []bool{false, true} {
		t.Run(map[bool]string{false: "file", true: "directory"}[directory], func(t *testing.T) {
			t.Parallel()
			outside := t.TempDir()
			if err := os.WriteFile(filepath.Join(outside, "outside.yaml"), []byte("timeout: 99s"), 0o600); err != nil {
				t.Fatal(err)
			}
			target, include := filepath.Join(outside, "outside.yaml"), "escape.yaml"
			if directory {
				target, include = outside, "escape.yaml/outside.yaml"
			}
			file := includeFiles(t, map[string]string{"config.yaml": "subject_id: test\ninclude: " + include})
			if err := os.Symlink(target, filepath.Join(filepath.Dir(file), "escape.yaml")); err != nil {
				t.Fatal(err)
			}
			if _, err := config.LoadResolved(file); err == nil {
				t.Fatal("symlink escape accepted")
			}
		})
	}
}

func TestIncludeSymlinkInsideRoot(t *testing.T) {
	t.Parallel()
	file := includeFiles(t, map[string]string{"config.yaml": "subject_id: test\ninclude: alias.yaml", "child.yaml": "timeout: 19s"})
	if err := os.Symlink("child.yaml", filepath.Join(filepath.Dir(file), "alias.yaml")); err != nil {
		t.Fatal(err)
	}
	got, err := config.Load(file)
	if err != nil || got.Timeout != 19*time.Second {
		t.Fatalf("confined symlink timeout = %s, error = %v", got.Timeout, err)
	}
	if err := os.WriteFile(file, []byte("subject_id: test\ninclude: [child.yaml, alias.yaml]"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := config.Load(file); !errors.Is(err, ashlar.ErrIncludeCycle) {
		t.Fatalf("repeated symlink target error = %v", err)
	}
}

func TestIncludeWorkingDirectory(t *testing.T) {
	// Changes cwd to prove includes and catalog paths use different bases.
	cwd := t.TempDir()
	t.Chdir(cwd)
	file := includeFiles(t, map[string]string{
		"config.yaml":    "subject_id: test\ninclude: sub/child.yaml",
		"sub/child.yaml": "include: /local.yaml\ndefinitions_dir: definitions\nprojections_dir: projections",
		"local.yaml":     "timeout: 19s",
	})
	got, err := config.Load(file)
	if err != nil {
		t.Fatal(err)
	}
	if got.Timeout != 19*time.Second || got.DefinitionsDir != filepath.Join(cwd, "definitions") || got.ProjectionsDir != filepath.Join(cwd, "projections") {
		t.Fatalf("wrong include or catalog paths = %+v", got)
	}
	definitions, projections, err := config.CatalogDirs(file)
	if err != nil || definitions != got.DefinitionsDir || projections != got.ProjectionsDir {
		t.Fatalf("CatalogDirs = %q, %q, %v", definitions, projections, err)
	}
}

func TestIncludeIsYAMLOnly(t *testing.T) {
	// Changes environment to prove graph controls and future schema stay YAML-only.
	t.Setenv("ANAM_INCLUDE", "missing.yaml")
	t.Setenv("ANAM_INSPECTOR", "{}")
	file := includeFiles(t, map[string]string{"config.yaml": "subject_id: test"})
	if _, err := config.Load(file); err != nil {
		t.Fatal(err)
	}
	if _, err := config.LoadOverriding(file, map[string]string{"include": "missing.yaml"}); err == nil {
		t.Fatal("explicit include string accepted")
	}
	if _, err := config.LoadOverriding(file, map[string]string{"inspector": "{}"}); err == nil {
		t.Fatal("explicit inspector string accepted")
	}
}

func TestIncludeStrictDecodeOriginAndRedaction(t *testing.T) {
	t.Parallel()
	file := includeFiles(t, map[string]string{
		"config.yaml": "subject_id: test\ninclude: child.yaml\n",
		"child.yaml":  "# original line one\ninclude: null\ntoken_file: private-value-must-not-be-dumped\nbackup_keep: wrong-type-must-not-be-dumped\n",
	})
	_, err := config.LoadResolved(file)
	var diagnostic *ashlar.Error
	if !errors.Is(err, config.ErrInvalid) || !errors.As(err, &diagnostic) {
		t.Fatalf("error = %v", err)
	}
	if diagnostic.Pointer != "/backup_keep" || diagnostic.Origin.Span.Source != "child.yaml" || diagnostic.Origin.Span.Start.Line != 4 || len(diagnostic.Origin.Via()) != 1 {
		t.Fatalf("wrong diagnostic = %v", err)
	}
	if strings.Contains(err.Error(), "must-not-be-dumped") {
		t.Fatal("decode error exposed configuration content")
	}
}

func TestIncludeValidationPointers(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct{ body, pointer string }{
		{"max_response_bytes: -1", "/max_response_bytes"},
		{"zone: not-a-zone", "/zone"},
		{"server: ftp://host", "/server"},
		{"ingest_timeout: 0s", "/long_write_timeout"},
		{"definitions_dir: ~another-user", "/definitions_dir"},
	} {
		t.Run(tt.pointer, func(t *testing.T) {
			t.Parallel()
			file := includeFiles(t, map[string]string{"config.yaml": "subject_id: test\ninclude: child.yaml", "child.yaml": "include: null\n" + tt.body})
			_, err := config.LoadResolved(file)
			var diagnostic *ashlar.Error
			if !errors.Is(err, config.ErrInvalid) || !errors.As(err, &diagnostic) {
				t.Fatalf("error = %v", err)
			}
			if diagnostic.Pointer.String() != tt.pointer || diagnostic.Origin.Span.Source != "child.yaml" || diagnostic.Origin.Span.Start.Line != 2 {
				t.Fatalf("wrong diagnostic = %v", err)
			}
		})
	}
}
