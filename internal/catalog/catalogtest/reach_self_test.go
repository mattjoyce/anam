package catalogtest_test

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const mod = "github.com/mattjoyce/anam"

// src is a test file in package pkg with the given imports (each "path" or
// "name path") and one test function T holding body.
func src(pkg string, imports []string, body string) string {
	var b strings.Builder
	b.WriteString("package " + pkg + "\n\nimport (\n")
	for _, imp := range imports {
		name, p, ok := strings.Cut(imp, " ")
		if !ok {
			name, p = "", imp
		}
		if strings.HasPrefix(p, "internal/") {
			p = mod + "/" + p
		}
		b.WriteString("\t" + strings.TrimSpace(name+` "`+p+`"`) + "\n")
	}
	b.WriteString(")\n\nfunc T(t *testing.T) {\n" + body + "\n}\n")
	return b.String()
}

// writeModule writes files, and a go.mod, to a new directory and returns it.
func writeModule(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	all := map[string]string{"go.mod": "module " + mod + "\n\ngo 1.25\n"}
	maps.Copy(all, files)
	for name, body := range all {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// TestReach_SeesEveryWayIn runs the walk over small modules, one per way a test can
// reach the embedded catalog and one per decoy that must not count, so a clean pass
// over the real module is not a walk that saw nothing. Each want is "file func: what".
func TestReach_SeesEveryWayIn(t *testing.T) {
	t.Parallel()
	ext := func(body string, imports ...string) map[string]string {
		return map[string]string{"internal/x/x_test.go": src("x_test", append([]string{"testing"}, imports...), body)}
	}
	in := func(dir, pkg, body string, imports ...string) map[string]string {
		return map[string]string{dir + "/in_test.go": src(pkg, append([]string{"testing"}, imports...), body)}
	}
	const x = "internal/x/x_test.go T: "
	for _, c := range []struct {
		name  string
		files map[string]string
		want  []string
	}{
		// Each embedded symbol, called or passed as a value.
		{"def.Bundled", ext(`def.Bundled()`, "internal/def"), []string{x + "def.Bundled"}},
		{"def.Manifests", ext(`def.Manifests()`, "internal/def"), []string{x + "def.Manifests"}},
		{"def.History", ext(`def.History()`, "internal/def"), []string{x + "def.History"}},
		{"def.SetVersion", ext(`def.SetVersion()`, "internal/def"), []string{x + "def.SetVersion"}},
		{"def.EmbeddedFS as a value", ext(`use(def.EmbeddedFS)`, "internal/def"), []string{x + "def.EmbeddedFS"}},
		{"projection.Bundled", ext(`projection.Bundled()`, "internal/projection"), []string{x + "projection.Bundled"}},
		{"projection.EmbeddedFS", ext(`projection.EmbeddedFS()`, "internal/projection"), []string{x + "projection.EmbeddedFS"}},
		{"renamed import", ext(`d.Bundled()`, "d internal/def"), []string{x + "def.Bundled"}},
		{"dot import", ext(`Bundled()`, ". internal/projection"), []string{x + "projection.Bundled"}},
		{"in a helper", map[string]string{"internal/x/x_test.go": "package x\n\nimport \"" + mod + "/internal/def\"\n\nvar reg, _ = def.Bundled()\n\nfunc helper() { _ = def.EmbeddedFS() }\n"},
			[]string{"internal/x/x_test.go reg: def.Bundled", "internal/x/x_test.go helper: def.EmbeddedFS"}},
		{"in a method", map[string]string{"internal/x/x_test.go": "package x\n\nimport \"" + mod + "/internal/def\"\n\nfunc (f *fx) cat() { def.Bundled() }\n"},
			[]string{"internal/x/x_test.go fx.cat: def.Bundled"}},

		// Inside the packages, the bare names, unexported ones included.
		{"in package def", in("internal/def", "def", "Bundled(); embedded(); _ = manifestFS; History()"),
			[]string{"internal/def/in_test.go T: def.Bundled", "internal/def/in_test.go T: def.embedded",
				"internal/def/in_test.go T: def.manifestFS", "internal/def/in_test.go T: def.History"}},
		{"in package projection", in("internal/projection", "projection", "embedded()"),
			[]string{"internal/projection/in_test.go T: projection.embedded"}},
		{"in package catalog", in("internal/catalog", "catalog", `embedded(); load(ctx, "", dir)`),
			[]string{"internal/catalog/in_test.go T: catalog.embedded", "internal/catalog/in_test.go T: catalog.load with an empty definitions directory"}},

		// The loader with an empty directory: the brief's grep missed the first two.
		{"Load(context.Background(), \"\", \"\")", ext(`catalog.Load(context.Background(), "", "")`, "context", "internal/catalog"),
			[]string{x + "catalog.Load with an empty definitions directory"}},
		{"Load(ctx, dir, \"\")", ext(`catalog.Load(ctx, dir, "")`, "internal/catalog"),
			[]string{x + "catalog.Load with an empty projections directory"}},
		{"Load(ctx, \"\", dir) multi-line", ext("catalog.Load(\n\t\tctx,\n\t\t``,\n\t\tdir,\n\t)", "internal/catalog"),
			[]string{x + "catalog.Load with an empty definitions directory"}},
		{"dot-imported Load", ext(`Load(ctx, "", "")`, ". internal/catalog"),
			[]string{x + "catalog.Load with an empty definitions directory"}},
		{"Load in parentheses", ext(`(catalog.Load)(ctx, defs, "")`, "internal/catalog"),
			[]string{x + "catalog.Load with an empty projections directory"}},
		{"Load through a variable", ext("f := catalog.Load\n\tf(ctx, defs, projs)\n\tvar g = (catalog.Load)\n\tg(ctx, a, b)", "internal/catalog"),
			[]string{x + "a call through f, a variable bound to catalog.Load", x + "a call through g, a variable bound to catalog.Load"}},
		{"an empty variable", ext("dir := \"\"\n\tcatalog.Load(ctx, dir, dir)", "internal/catalog"),
			[]string{x + "catalog.Load with an empty definitions directory"}},
		{"a zero string var", ext("var dir string\n\tcatalog.Load(ctx, defs, dir)", "internal/catalog"),
			[]string{x + "catalog.Load with an empty projections directory"}},
		{"an empty const at file scope", map[string]string{"internal/x/x_test.go": "package x_test\n\nimport \"" + mod + "/internal/catalog\"\n\nconst none = \"\"\n\nconst alsoNone = none\n\nfunc T() { catalog.Load(ctx, alsoNone, d) }\n"},
			[]string{x + "catalog.Load with an empty definitions directory"}},
		{"an empty config var", ext("dir := \"\"\n\tcfg.ProjectionsDir = dir"),
			[]string{x + `ProjectionsDir = "" selects the embedded catalog`}},

		// A config whose catalog directories are "", so a daemon opens on the embedded one.
		{"config dir assigned \"\"", ext(`cfg.DefinitionsDir, cfg.ProjectionsDir = "", ""`),
			[]string{x + `DefinitionsDir = "" selects the embedded catalog`, x + `ProjectionsDir = "" selects the embedded catalog`}},
		{"config literal with \"\"", ext(`_ = config.Config{ProjectionsDir: ""}`, "internal/config"),
			[]string{x + `ProjectionsDir: "" selects the embedded catalog`}},

		// The catalog's directories read from disk.
		{"relative path", map[string]string{"internal/inspect/x_test.go": src("inspect", []string{"os"}, `os.ReadDir("../def/manifests")`)},
			[]string{`internal/inspect/x_test.go T: the path "../def/manifests" reads internal/def/manifests from disk`}},
		{"bare dir in its package", in("internal/def", "def_test", `filepath.Glob(filepath.Join("manifests", "*.json"))`, "path/filepath"),
			[]string{`internal/def/in_test.go T: the path "manifests/*.json" reads internal/def/manifests from disk`}},
		{"bare dir in a slice", in("internal/projection", "projection", `for _, d := range []string{"manifests"} { _ = d }`),
			[]string{`internal/projection/in_test.go T: the path "manifests" reads internal/projection/manifests from disk`}},
		{"literal join", ext(`filepath.Join("..", "projection", "manifests", "bp.json")`, "path/filepath"),
			[]string{x + `the path "../projection/manifests/bp.json" reads internal/projection/manifests from disk`}},
		{"join under a computed root", ext(`os.DirFS(filepath.Join(root, "internal", "def", "manifests"))`, "os", "path/filepath"),
			[]string{x + "a path ending in internal/def/manifests"}},
		{"module-relative literal", ext(`os.ReadFile(root + "/internal/projection/manifests/history/bp.v1.json")`, "os"),
			[]string{x + `the path "/internal/projection/manifests/history/bp.v1.json" reads internal/projection/manifests from disk`}},

		// Decoys: none of these reads the embedded catalog.
		{"decoy: query's own Bundled", in("internal/query", "query", "rules, err := Bundled(); _, _ = rules, err"), nil},
		{"decoy: the FS functions over a fixture", ext(`def.BundledFS(f); def.ManifestsFS(f); def.HistoryFS(f); def.SetVersionFS(f); projection.BundledFS(f)`,
			"internal/def", "internal/projection"), nil},
		{"decoy: catalog methods", ext(`cat.Manifests(); cat.History(); cat.SetVersion(); reg.Bundled`), nil},
		{"decoy: Load from directories", ext(`catalog.Load(ctx, defs, projs)`, "internal/catalog"), nil},
		{"decoy: a local shadows the package", ext(`def := fake{}; def.Bundled()`, "internal/def"), nil},
		{"decoy: a local shadows in package", in("internal/catalog", "catalog", `embedded, err := Load(ctx, a, b); _, _ = embedded, err`), nil},
		{"decoy: a field key", in("internal/def", "def", `_ = Registry{History: nil}`), nil},
		{"decoy: a method named History", map[string]string{"internal/def/m_test.go": "package def\n\ntype h struct{}\n\nfunc (h) History() {}\n"}, nil},
		{"decoy: source text and a comment", ext("_ = `def.Bundled() and \"../def/manifests\"`\n// def.Bundled() catalog.Load(ctx, \"\", \"\")", "internal/def"), nil},
		{"decoy: a manifests dir elsewhere", in("internal/serve", "serve", `os.MkdirAll(filepath.Join(t.TempDir(), "manifests"), 0o750)`, "os", "path/filepath"), nil},
		{"decoy: registered projections", ext(`os.ReadFile(filepath.Join("..", "..", "projections", "heart.v3.json"))`, "os", "path/filepath"), nil},
		{"decoy: a config dir set to a path", ext(`cfg.DefinitionsDir = dir; _ = config.Config{ProjectionsDir: dir}`, "internal/config"), nil},
		{"decoy: a variable set again", ext("dir := \"\"\n\tdir = t.TempDir()\n\tcatalog.Load(ctx, dir, dir)", "internal/catalog"), nil},
		{"decoy: a variable whose address is taken", ext("var dir string\n\tflagVar(&dir)\n\tcatalog.Load(ctx, dir, dir)", "internal/catalog"), nil},
		{"decoy: catalog.Load passed, not bound", ext(`use(catalog.Load)`, "internal/catalog"), nil},
		{"decoy: a package's own Load", in("internal/x", "x", `Load(ctx, "", "")`), nil},
		{"decoy: under testdata", map[string]string{"internal/x/testdata/x_test.go": src("x", []string{"internal/def"}, "def.Bundled()")}, nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			s, err := findReaches(writeModule(t, c.files))
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, r := range s.reaches {
				fn := r.Func
				if fn == "" {
					fn = "(file scope)"
				}
				got = append(got, r.File+" "+fn+": "+r.What)
			}
			slices.Sort(got)
			want := slices.Clone(c.want)
			slices.Sort(want)
			if !slices.Equal(got, want) {
				t.Errorf("reaches:\n got  %q\n want %q", got, want)
			}
		})
	}
}

// TestCheck_HoldsTheListToWhatReaches runs the list check over given reaches, so an
// entry that allows nothing, a gone file, and a reach in an unlisted function of a
// listed file all fail.
func TestCheck_HoldsTheListToWhatReaches(t *testing.T) {
	t.Parallel()
	files := []string{"a_test.go", "b_test.go"}
	sym := func(file, fn string) reach { return reach{File: file, Func: fn, Kind: kindSymbol, What: "def.Bundled"} }
	lit := func(file, fn string) reach { return reach{File: file, Func: fn, Kind: kindPath, What: "a path"} }
	for _, c := range []struct {
		name    string
		reaches []reach
		allow   []allowed
		labels  []allowed
		want    []string // a substring of each problem, in order
	}{
		{name: "clean", reaches: []reach{sym("a_test.go", "TestA")},
			allow: []allowed{{file: "a_test.go", reason: "r"}}},
		{name: "unlisted file", reaches: []reach{sym("b_test.go", "TestB")},
			want: []string{"b_test.go:0 TestB: def.Bundled reaches the embedded catalog outside the allow-list"}},
		{name: "unlisted function in a listed file", reaches: []reach{sym("a_test.go", "TestA"), sym("a_test.go", "TestNew")},
			allow: []allowed{{file: "a_test.go", funcs: []string{"TestA"}, reason: "r"}},
			want:  []string{"a_test.go:0 TestNew: def.Bundled reaches"}},
		{name: "stale file entry", allow: []allowed{{file: "a_test.go", reason: "r"}},
			want: []string{"listed a_test.go no longer reaches the embedded catalog"}},
		{name: "stale function", reaches: []reach{sym("a_test.go", "TestA")},
			allow: []allowed{{file: "a_test.go", funcs: []string{"TestA", "TestGone"}, reason: "r"}},
			want:  []string{"listed a_test.go TestGone no longer reaches the embedded catalog"}},
		{name: "gone file", allow: []allowed{{file: "gone_test.go", reason: "r"}},
			want: []string{"listed gone_test.go is not a test file in the module"}},
		{name: "a label excuses a path", reaches: []reach{lit("a_test.go", "golden")},
			labels: []allowed{{file: "a_test.go", funcs: []string{"golden"}, reason: "r"}}},
		{name: "a label does not excuse a symbol", reaches: []reach{sym("a_test.go", "golden")},
			labels: []allowed{{file: "a_test.go", funcs: []string{"golden"}, reason: "r"}},
			want:   []string{"a_test.go:0 golden: def.Bundled reaches", "listed a_test.go golden no longer names a catalog path"}},
		{name: "a method is not the function of its name", reaches: []reach{sym("a_test.go", "fx.TestA")},
			allow: []allowed{{file: "a_test.go", funcs: []string{"TestA"}, reason: "r"}},
			want:  []string{"a_test.go:0 fx.TestA: def.Bundled reaches", "listed a_test.go TestA no longer reaches"}},
		{name: "a file listed twice", reaches: []reach{sym("a_test.go", "TestA")},
			allow: []allowed{{file: "a_test.go", reason: "r"}, {file: "a_test.go", funcs: []string{"TestA"}, reason: "r"}},
			want:  []string{"a_test.go is listed twice in one list", "listed a_test.go TestA no longer reaches"}},
		{name: "lists overlap", reaches: []reach{lit("a_test.go", "golden")},
			allow:  []allowed{{file: "a_test.go", reason: "r"}},
			labels: []allowed{{file: "a_test.go", funcs: []string{"golden"}, reason: "r"}},
			want:   []string{"a_test.go is in both the allow-list and pathLabels", "listed a_test.go no longer reaches"}},
		{name: "usage is per list", reaches: []reach{lit("a_test.go", "golden")},
			allow:  []allowed{{file: "a_test.go", funcs: []string{"golden"}, reason: "r"}},
			labels: []allowed{{file: "a_test.go", funcs: []string{"golden"}, reason: "r"}},
			want:   []string{"a_test.go is in both the allow-list and pathLabels", "listed a_test.go golden no longer reaches"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := checkReaches(scan{reaches: c.reaches, files: files}, c.allow, c.labels)
			if len(got) != len(c.want) {
				t.Fatalf("problems = %q, want %d like %q", got, len(c.want), c.want)
			}
			for i := range got {
				if !strings.Contains(got[i], c.want[i]) {
					t.Errorf("problem %d = %q, want it to contain %q", i, got[i], c.want[i])
				}
			}
		})
	}
}

// TestCheck_FollowsHelpers runs the walk and the list check over small modules, so a
// new test that calls an allowed helper fails, and a label excuses only the function
// it names.
func TestCheck_FollowsHelpers(t *testing.T) {
	t.Parallel()
	head := "package x\n\nimport (\n\t\"os\"\n\t\"" + mod + "/internal/def\"\n)\n\n"
	allowA := []allowed{{file: "internal/x/a_test.go", reason: "r"}}
	for _, c := range []struct {
		name   string
		files  map[string]string
		allow  []allowed
		labels []allowed
		want   []string // a substring of each problem, in order
	}{
		{name: "an allowed helper called only from its file",
			files: map[string]string{"internal/x/a_test.go": head + "func helper() { def.Bundled() }\n\nfunc TestA() { helper() }\n"},
			allow: allowA},
		{name: "a new test calls an allowed helper",
			files: map[string]string{
				"internal/x/a_test.go": head + "func helper() { def.Bundled() }\n\nfunc TestA() { helper() }\n",
				"internal/x/b_test.go": head + "func TestNew() { helper() }\n",
			},
			allow: allowA, want: []string{"internal/x/b_test.go:8 TestNew: calls helper, which reaches"}},
		{name: "through another helper",
			files: map[string]string{
				"internal/x/a_test.go": head + "func helper() { def.Bundled() }\n",
				"internal/x/b_test.go": head + "func mid() { helper() }\n\nfunc TestNew() { mid() }\n",
			},
			allow: allowA, want: []string{"b_test.go:8 mid: calls helper", "b_test.go:10 TestNew: calls mid"}},
		{name: "a method helper",
			files: map[string]string{
				"internal/x/a_test.go": head + "type fx struct{}\n\nfunc (fx) cat() { def.Bundled() }\n",
				"internal/x/b_test.go": head + "func TestNew(f fx) { f.cat() }\n",
			},
			allow: allowA, want: []string{"b_test.go:8 TestNew: calls fx.cat"}},
		{name: "a package-level var",
			files: map[string]string{
				"internal/x/a_test.go": head + "var reg, _ = def.Bundled()\n",
				"internal/x/b_test.go": head + "func TestNew() { _ = reg }\n",
			},
			allow: allowA, want: []string{"b_test.go:8 TestNew: calls reg"}},
		{name: "decoy: the same name in another package",
			files: map[string]string{
				"internal/x/a_test.go": head + "func helper() { def.Bundled() }\n",
				"internal/y/b_test.go": "package y\n\nfunc TestNew() { helper() }\n",
			},
			allow: allowA},
		{name: "a label excuses only its formatter",
			files: map[string]string{"internal/def/b_test.go": "package def\n\nimport \"os\"\n\n" +
				"func label(n string) string { return \"manifests/\" + n }\n\n" +
				"func lines() { _ = label(\"x\"); os.ReadFile(\"manifests/x.json\") }\n"},
			labels: []allowed{{file: "internal/def/b_test.go", funcs: []string{"label"}, reason: "r"}},
			want:   []string{`internal/def/b_test.go:7 lines: the path "manifests/x.json"`}},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			s, err := findReaches(writeModule(t, c.files))
			if err != nil {
				t.Fatal(err)
			}
			got := checkReaches(s, c.allow, c.labels)
			if len(got) != len(c.want) {
				t.Fatalf("problems = %q, want %d like %q", got, len(c.want), c.want)
			}
			for i := range got {
				if !strings.Contains(got[i], c.want[i]) {
					t.Errorf("problem %d = %q, want it to contain %q", i, got[i], c.want[i])
				}
			}
		})
	}
}
