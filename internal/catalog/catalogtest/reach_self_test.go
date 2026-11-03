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
// select the empty catalog unchecked and one per decoy that must not count, so a
// clean pass over the real module is not a walk that saw nothing. Each want is
// "file func: what".
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
		// The loader with an empty directory.
		{"Load(context.Background(), \"\", \"\")", ext(`catalog.Load(context.Background(), "", "")`, "context", "internal/catalog"),
			[]string{x + "catalog.Load with an empty definitions directory"}},
		{"Load(ctx, dir, \"\")", ext(`catalog.Load(ctx, dir, "")`, "internal/catalog"),
			[]string{x + "catalog.Load with an empty projections directory"}},
		{"Load(ctx, \"\", dir) multi-line", ext("catalog.Load(\n\t\tctx,\n\t\t``,\n\t\tdir,\n\t)", "internal/catalog"),
			[]string{x + "catalog.Load with an empty definitions directory"}},
		{"renamed import", ext(`c.Load(ctx, "", dir)`, "c internal/catalog"),
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
		{"in package catalog", in("internal/catalog", "catalog", `load(ctx, "", dir); Load(ctx, dir, "")`),
			[]string{"internal/catalog/in_test.go T: catalog.load with an empty definitions directory",
				"internal/catalog/in_test.go T: catalog.Load with an empty projections directory"}},
		{"in a helper", map[string]string{"internal/x/x_test.go": "package x\n\nimport \"" + mod + "/internal/catalog\"\n\nvar cat, _ = catalog.Load(ctx, \"\", d)\n\nfunc (f *fx) cat() { catalog.Load(ctx, d, \"\") }\n"},
			[]string{"internal/x/x_test.go cat: catalog.Load with an empty definitions directory",
				"internal/x/x_test.go fx.cat: catalog.Load with an empty projections directory"}},

		// A config whose catalog directories are "", so a daemon opens on the empty one.
		{"config dir assigned \"\"", ext(`cfg.DefinitionsDir, cfg.ProjectionsDir = "", ""`),
			[]string{x + `DefinitionsDir = "" selects the empty catalog`, x + `ProjectionsDir = "" selects the empty catalog`}},
		{"config literal with \"\"", ext(`_ = config.Config{ProjectionsDir: ""}`, "internal/config"),
			[]string{x + `ProjectionsDir: "" selects the empty catalog`}},
		{"an empty config var", ext("dir := \"\"\n\tcfg.ProjectionsDir = dir"),
			[]string{x + `ProjectionsDir = "" selects the empty catalog`}},

		// The none label excuses only the function that names it, as a string.
		{"none in another function", map[string]string{"internal/x/x_test.go": "package x_test\n\nimport \"" + mod + "/internal/catalog\"\n\nfunc T() { catalog.Load(ctx, \"\", \"\") }\n\nfunc U() { _ = \"none\" }\n"},
			[]string{x + "catalog.Load with an empty definitions directory"}},
		{"none in a comment", ext("// none\n\tcatalog.Load(ctx, \"\", d)", "internal/catalog"),
			[]string{x + "catalog.Load with an empty definitions directory"}},
		{"none inside a longer string", ext("_ = \"none, none\"\n\tcatalog.Load(ctx, \"\", d)", "internal/catalog"),
			[]string{x + "catalog.Load with an empty definitions directory"}},
		{"none at file scope", map[string]string{"internal/x/x_test.go": "package x_test\n\nimport \"" + mod + "/internal/catalog\"\n\nconst label = \"none\"\n\nfunc T() { catalog.Load(ctx, \"\", d); _ = label }\n"},
			[]string{x + "catalog.Load with an empty definitions directory"}},

		// Decoys: none of these selects the empty catalog unchecked.
		{"decoy: a test that checks the none label", ext("cat, _ := catalog.Load(ctx, \"\", \"\")\n\tif cat.DefinitionsSource() != \"none\" { t.Fail() }", "internal/catalog"), nil},
		{"decoy: a config test that checks the none label", ext("cfg.DefinitionsDir = \"\"\n\twant(map[string]any{\"definitions_source\": \"none\"})"), nil},
		{"decoy: a closure inside a test that checks none", ext("f := func() { catalog.Load(ctx, \"\", d) }\n\tf()\n\t_ = `none`", "internal/catalog"), nil},
		{"decoy: the FS functions over a fixture", ext(`def.BundledFS(f); def.ManifestsFS(f); def.HistoryFS(f); def.SetVersionFS(f); projection.BundledFS(f)`,
			"internal/def", "internal/projection"), nil},
		{"decoy: catalog methods", ext(`cat.Manifests(); cat.History(); cat.SetVersion(); reg.Bundled`), nil},
		{"decoy: Load from directories", ext(`catalog.Load(ctx, defs, projs)`, "internal/catalog"), nil},
		{"decoy: a local shadows the package", ext(`catalog := fake{}; catalog.Load(ctx, "", "")`, "internal/catalog"), nil},
		{"decoy: a local shadows in package", in("internal/catalog", "catalog", `load := fake; load(ctx, "", "")`), nil},
		{"decoy: source text and a comment", ext("_ = `catalog.Load(ctx, \"\", \"\")`\n// catalog.Load(ctx, \"\", \"\")", "internal/catalog"), nil},
		{"decoy: a config dir set to a path", ext(`cfg.DefinitionsDir = dir; _ = config.Config{ProjectionsDir: dir}`, "internal/config"), nil},
		{"decoy: a variable set again", ext("dir := \"\"\n\tdir = t.TempDir()\n\tcatalog.Load(ctx, dir, dir)", "internal/catalog"), nil},
		{"decoy: a variable whose address is taken", ext("var dir string\n\tflagVar(&dir)\n\tcatalog.Load(ctx, dir, dir)", "internal/catalog"), nil},
		{"decoy: catalog.Load passed, not bound", ext(`use(catalog.Load)`, "internal/catalog"), nil},
		{"decoy: a package's own Load", in("internal/x", "x", `Load(ctx, "", "")`), nil},
		{"decoy: under testdata", map[string]string{"internal/x/testdata/x_test.go": src("x", []string{"internal/catalog"}, `catalog.Load(ctx, "", "")`)}, nil},
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
// listed file all fail. The real list is nil; the check stays for an entry a later
// review admits.
func TestCheck_HoldsTheListToWhatReaches(t *testing.T) {
	t.Parallel()
	files := []string{"a_test.go", "b_test.go"}
	load := func(file, fn string) reach {
		return reach{File: file, Func: fn, Kind: kindLoad, What: "catalog.Load with an empty definitions directory"}
	}
	for _, c := range []struct {
		name    string
		reaches []reach
		allow   []allowed
		want    []string // a substring of each problem, in order
	}{
		{name: "clean", reaches: []reach{load("a_test.go", "TestA")},
			allow: []allowed{{file: "a_test.go", reason: "r"}}},
		{name: "nothing reaches and nothing is listed"},
		{name: "unlisted file", reaches: []reach{load("b_test.go", "TestB")},
			want: []string{"b_test.go:0 TestB: catalog.Load with an empty definitions directory selects the empty catalog without checking its none label"}},
		{name: "unlisted function in a listed file", reaches: []reach{load("a_test.go", "TestA"), load("a_test.go", "TestNew")},
			allow: []allowed{{file: "a_test.go", funcs: []string{"TestA"}, reason: "r"}},
			want:  []string{"a_test.go:0 TestNew: catalog.Load"}},
		{name: "stale file entry", allow: []allowed{{file: "a_test.go", reason: "r"}},
			want: []string{"listed a_test.go no longer reaches"}},
		{name: "stale function", reaches: []reach{load("a_test.go", "TestA")},
			allow: []allowed{{file: "a_test.go", funcs: []string{"TestA", "TestGone"}, reason: "r"}},
			want:  []string{"listed a_test.go TestGone no longer reaches"}},
		{name: "gone file", allow: []allowed{{file: "gone_test.go", reason: "r"}},
			want: []string{"listed gone_test.go is not a test file in the module"}},
		{name: "a method is not the function of its name", reaches: []reach{load("a_test.go", "fx.TestA")},
			allow: []allowed{{file: "a_test.go", funcs: []string{"TestA"}, reason: "r"}},
			want:  []string{"a_test.go:0 fx.TestA: catalog.Load", "listed a_test.go TestA no longer reaches"}},
		{name: "a file listed twice", reaches: []reach{load("a_test.go", "TestA")},
			allow: []allowed{{file: "a_test.go", reason: "r"}, {file: "a_test.go", funcs: []string{"TestA"}, reason: "r"}},
			want:  []string{"a_test.go is listed twice", "listed a_test.go TestA no longer reaches"}},
		{name: "an empty set of functions", reaches: []reach{load("a_test.go", "TestA")},
			allow: []allowed{{file: "a_test.go", funcs: []string{}, reason: "r"}},
			want:  []string{"a_test.go lists an empty set of functions", "a_test.go:0 TestA: catalog.Load"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := checkReaches(scan{reaches: c.reaches, files: files}, c.allow)
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
// new test that calls a helper that selects the empty catalog unchecked fails, and a
// helper that checks the label passes on to no one.
func TestCheck_FollowsHelpers(t *testing.T) {
	t.Parallel()
	head := "package x\n\nimport \"" + mod + "/internal/catalog\"\n\n"
	const sel = `catalog.Load(ctx, "", d)`
	allowA := []allowed{{file: "internal/x/a_test.go", reason: "r"}}
	for _, c := range []struct {
		name  string
		files map[string]string
		allow []allowed
		want  []string // a substring of each problem, in order
	}{
		{name: "an allowed helper called only from its file",
			files: map[string]string{"internal/x/a_test.go": head + "func helper() { " + sel + " }\n\nfunc TestA() { helper() }\n"},
			allow: allowA},
		{name: "a new test calls an allowed helper",
			files: map[string]string{
				"internal/x/a_test.go": head + "func helper() { " + sel + " }\n\nfunc TestA() { helper() }\n",
				"internal/x/b_test.go": head + "func TestNew() { helper() }\n",
			},
			allow: allowA, want: []string{"internal/x/b_test.go:5 TestNew: via helper helper"}},
		{name: "through another helper",
			files: map[string]string{
				"internal/x/a_test.go": head + "func helper() { " + sel + " }\n",
				"internal/x/b_test.go": head + "func mid() { helper() }\n\nfunc TestNew() { mid() }\n",
			},
			allow: allowA, want: []string{"b_test.go:5 mid: via helper helper", "b_test.go:7 TestNew: via helper mid"}},
		{name: "a method helper",
			files: map[string]string{
				"internal/x/a_test.go": head + "type fx struct{}\n\nfunc (fx) cat() { " + sel + " }\n",
				"internal/x/b_test.go": head + "func TestNew(f fx) { f.cat() }\n",
			},
			allow: allowA, want: []string{"b_test.go:5 TestNew: via helper fx.cat"}},
		{name: "a package-level var",
			files: map[string]string{
				"internal/x/a_test.go": head + "var cat, _ = " + sel + "\n",
				"internal/x/b_test.go": head + "func TestNew() { _ = cat }\n",
			},
			allow: allowA, want: []string{"b_test.go:5 TestNew: via helper cat"}},
		{name: "an unlisted helper and its caller",
			files: map[string]string{"internal/x/a_test.go": head + "func helper() { " + sel + " }\n\nfunc TestA() { helper() }\n"},
			want:  []string{"a_test.go:5 helper: catalog.Load", "a_test.go:7 TestA: via helper helper"}},
		{name: "a helper that checks the label",
			files: map[string]string{"internal/x/a_test.go": head +
				"func helper() bool { c, _ := " + sel + "; return c.DefinitionsSource() == \"none\" }\n\nfunc TestA() { helper() }\n"}},
		{name: "decoy: the same name in another package",
			files: map[string]string{
				"internal/x/a_test.go": head + "func helper() { " + sel + " }\n",
				"internal/y/b_test.go": "package y\n\nfunc TestNew() { helper() }\n",
			},
			allow: allowA},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			s, err := findReaches(writeModule(t, c.files))
			if err != nil {
				t.Fatal(err)
			}
			got := checkReaches(s, c.allow)
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
