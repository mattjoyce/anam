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
// select the empty catalog and one per decoy that must not count, so a
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

		// Decoys: none of these selects the empty catalog.
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

// TestCheck_HoldsTheListToWhatReaches runs the walk and the list check over small
// modules with T listed, so a listed test passes only while it checks every catalog
// it selects with a Require helper in its own body, and nothing else is excused.
func TestCheck_HoldsTheListToWhatReaches(t *testing.T) {
	t.Parallel()
	const file = "internal/x/x_test.go"
	listT := []exempt{{file: file, fn: "T", reason: "r"}}
	mod1 := func(body string) map[string]string {
		return map[string]string{file: src("x_test", []string{"testing", "internal/catalog", "internal/catalog/catalogtest"}, body)}
	}
	for _, c := range []struct {
		name  string
		files map[string]string
		list  []exempt
		want  []string // a substring of each problem, in order
	}{
		{name: "a listed load, checked", list: listT,
			files: mod1("cat, err := catalog.Load(ctx, \"\", \"\")\n\tcatalogtest.RequireEmpty(t, cat)\n\t_ = err")},
		{name: "a listed load assigned with =, checked", list: listT,
			files: mod1("var cat *catalog.Catalog\n\tcat, _ = catalog.Load(ctx, d, \"\")\n\tcatalogtest.RequireNoProjections(t, cat)")},
		{name: "a listed refusal, checked", list: listT,
			files: mod1("cat, err := catalog.Load(canceled, \"\", \"\")\n\tcatalogtest.RequireRefused(t, cat, err)")},
		{name: "a listed config, checked", list: listT,
			files: mod1("cfg.DefinitionsDir, cfg.ProjectionsDir = \"\", \"\"\n\td := start(cfg)\n\tcatalogtest.RequireEmpty(t, d.catalog)")},

		{name: "unlisted, with an unrelated none", files: mod1("_ = \"none\"\n\tcat, _ := catalog.Load(ctx, \"\", \"\")\n\tcatalogtest.RequireEmpty(t, cat)"),
			want: []string{file + ":11 T: catalog.Load with an empty definitions directory selects the empty catalog outside emptyOnPurpose"}},
		{name: "listed, an ignored comparison", list: listT,
			files: mod1("cat, _ := catalog.Load(ctx, \"\", \"\")\n\t_ = cat.DefinitionsSource() == \"none\""),
			want:  []string{"selects the empty catalog on 1 lines and calls a catalogtest Require helper 0 times", "the catalog this load returns is not passed"}},
		{name: "listed, the check in an uncalled closure", list: listT,
			files: mod1("cat, _ := catalog.Load(ctx, \"\", \"\")\n\tf := func() { catalogtest.RequireEmpty(t, cat) }\n\t_ = f"),
			want:  []string{"on 1 lines and calls a catalogtest Require helper 0 times", "is not passed"}},
		{name: "listed, the load and check in a subtest", list: listT,
			files: mod1("t.Run(\"none\", func(t *testing.T) {\n\t\tcat, _ := catalog.Load(ctx, \"\", \"\")\n\t\tcatalogtest.RequireEmpty(t, cat)\n\t})"),
			want:  []string{"on 1 lines and calls a catalogtest Require helper 0 times", "is not passed"}},
		{name: "a sibling of a listed test", list: listT,
			files: map[string]string{file: "package x_test\n\nimport (\n\t\"" + mod + "/internal/catalog\"\n\t\"" + mod + "/internal/catalog/catalogtest\"\n)\n\n" +
				"func T(t *testing.T) { cat, _ := catalog.Load(ctx, \"\", \"\"); catalogtest.RequireEmpty(t, cat) }\n\n" +
				"func U(t *testing.T) { cat, _ := catalog.Load(ctx, \"\", \"\"); _ = cat }\n"},
			want: []string{file + ":10 U: catalog.Load with an empty definitions directory selects the empty catalog outside emptyOnPurpose"}},
		{name: "listed, a second load unchecked", list: listT,
			files: mod1("a, _ := catalog.Load(ctx, \"\", \"\")\n\tcatalogtest.RequireEmpty(t, a)\n\tb, _ := catalog.Load(ctx, \"\", d)\n\t_ = b"),
			want:  []string{"on 2 lines and calls a catalogtest Require helper 1 times", file + ":12 T: catalog.Load with an empty definitions directory: listed, but the catalog this load returns is not passed"}},
		{name: "listed, the first load checked twice", list: listT,
			files: mod1("a, _ := catalog.Load(ctx, \"\", \"\")\n\tb, _ := catalog.Load(ctx, \"\", d)\n\tcatalogtest.RequireEmpty(t, a)\n\tcatalogtest.RequireEmpty(t, a)\n\t_ = b"),
			want:  []string{file + ":11 T: catalog.Load with an empty definitions directory: listed, but the catalog this load returns is not passed"}},
		{name: "listed, a load whose result is dropped", list: listT,
			files: mod1("catalog.Load(ctx, \"\", \"\")\n\tcatalogtest.RequireEmpty(t, other)"),
			want:  []string{"the catalog this load returns is not passed"}},
		{name: "listed, a Require of another package", list: listT,
			files: map[string]string{file: src("x_test", []string{"testing", "internal/catalog", "catalogtest internal/y"},
				"cat, _ := catalog.Load(ctx, \"\", \"\")\n\tcatalogtest.RequireEmpty(t, cat)")},
			want: []string{"calls a catalogtest Require helper 0 times", "is not passed"}},
		{name: "a test that calls a listed one", list: listT,
			files: map[string]string{
				file:                   src("x", []string{"testing", "internal/catalog", "internal/catalog/catalogtest"}, "cat, _ := catalog.Load(ctx, \"\", \"\")\n\tcatalogtest.RequireEmpty(t, cat)"),
				"internal/x/b_test.go": "package x\n\nfunc TestNew(t *testing.T) { T(t) }\n",
			},
			want: []string{"internal/x/b_test.go:3 TestNew: via helper T"}},

		{name: "a listed function that selects nothing", list: listT, files: mod1("_ = 1"),
			want: []string{"listed " + file + " T no longer selects the empty catalog"}},
		{name: "a listed function that has gone", list: []exempt{{file: file, fn: "Gone", reason: "r"}}, files: mod1("_ = 1"),
			want: []string{"listed " + file + " Gone is not a function in that file"}},
		{name: "a listed file that has gone", list: []exempt{{file: "internal/x/gone_test.go", fn: "T", reason: "r"}}, files: mod1("_ = 1"),
			want: []string{"listed internal/x/gone_test.go is not a test file"}},
		{name: "a function listed twice", list: append(slices.Clone(listT), listT...),
			files: mod1("cat, _ := catalog.Load(ctx, \"\", \"\")\n\tcatalogtest.RequireEmpty(t, cat)"),
			want:  []string{file + " T is listed twice"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			s, err := findReaches(writeModule(t, c.files))
			if err != nil {
				t.Fatal(err)
			}
			got := checkReaches(s, c.list)
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
// test that calls a helper that selects the empty catalog fails, however far away.
func TestCheck_FollowsHelpers(t *testing.T) {
	t.Parallel()
	head := "package x\n\nimport \"" + mod + "/internal/catalog\"\n\n"
	const sel = `catalog.Load(ctx, "", d)`
	for _, c := range []struct {
		name  string
		files map[string]string
		want  []string // a substring of each problem, in order
	}{
		{name: "a helper and its caller",
			files: map[string]string{"internal/x/a_test.go": head + "func helper() { " + sel + " }\n\nfunc TestA() { helper() }\n"},
			want:  []string{"a_test.go:5 helper: catalog.Load", "a_test.go:7 TestA: via helper helper"}},
		{name: "through another helper in another file",
			files: map[string]string{
				"internal/x/a_test.go": head + "func helper() { " + sel + " }\n",
				"internal/x/b_test.go": head + "func mid() { helper() }\n\nfunc TestNew() { mid() }\n",
			},
			want: []string{"a_test.go:5 helper: catalog.Load", "b_test.go:5 mid: via helper helper", "b_test.go:7 TestNew: via helper mid"}},
		{name: "a method helper",
			files: map[string]string{
				"internal/x/a_test.go": head + "type fx struct{}\n\nfunc (fx) cat() { " + sel + " }\n",
				"internal/x/b_test.go": head + "func TestNew(f fx) { f.cat() }\n",
			},
			want: []string{"a_test.go:7 fx.cat: catalog.Load", "b_test.go:5 TestNew: via helper fx.cat"}},
		{name: "a package-level var",
			files: map[string]string{
				"internal/x/a_test.go": head + "var cat, _ = " + sel + "\n",
				"internal/x/b_test.go": head + "func TestNew() { _ = cat }\n",
			},
			want: []string{"a_test.go:5 cat: catalog.Load", "b_test.go:5 TestNew: via helper cat"}},
		{name: "decoy: the same name in another package",
			files: map[string]string{
				"internal/x/a_test.go": head + "func helper() { " + sel + " }\n",
				"internal/y/b_test.go": "package y\n\nfunc TestNew() { helper() }\n",
			},
			want: []string{"a_test.go:5 helper: catalog.Load"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			s, err := findReaches(writeModule(t, c.files))
			if err != nil {
				t.Fatal(err)
			}
			got := checkReaches(s, nil)
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
