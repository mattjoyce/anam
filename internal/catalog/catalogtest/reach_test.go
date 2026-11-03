package catalogtest_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// The packages that hold the embedded catalog, module-relative.
const (
	defPkg     = "internal/def"
	projPkg    = "internal/projection"
	catalogPkg = "internal/catalog"
)

// exported names, by package, that return or read the embedded catalog: the functions
// R4b deletes. A test that names one, called or passed as a value, reads the catalog
// the binary carries rather than one it chose.
var exported = map[string][]string{
	defPkg:  {"Bundled", "EmbeddedFS", "History", "Manifests", "SetVersion"},
	projPkg: {"Bundled", "EmbeddedFS"},
}

// unexported names that hold the embedded catalog, which only a test inside the
// package can name.
var unexported = map[string][]string{
	defPkg:     {"embedded", "manifestFS"},
	projPkg:    {"embedded", "manifestFS"},
	catalogPkg: {"embedded"},
}

// loaders are the functions that fall back on the embedded catalog for a component
// whose directory is empty: catalog.Load, and load inside the package. A variable
// bound to one is a loader too, and any call through it counts, whatever its
// arguments.
var loaders = map[string]bool{"Load": true, "load": true}

// configDirs are the config.Config fields that select a catalog directory. A test
// that sets one to "" opens a daemon on the embedded catalog for that component.
var configDirs = map[string]bool{"DefinitionsDir": true, "ProjectionsDir": true}

// catalogDirs are the embedded catalog's source directories, which a test can read
// from disk without naming any symbol. Built with path.Join so this file names no
// such path as a literal itself.
var catalogDirs = []string{path.Join(defPkg, "manifests"), path.Join(projPkg, "manifests")}

// kind is the way a reach gets to the embedded catalog.
type kind int

const (
	kindSymbol kind = iota // names an embedded def, projection or catalog symbol
	kindLoad               // catalog.Load with an empty directory
	kindConfig             // a config catalog directory set to ""
	kindPath               // a string naming an embedded catalog directory on disk
	kindHelper             // a call to a test helper that reaches
)

// reach is one place a test reads the embedded catalog.
type reach struct {
	Pkg  string // the directory and package name, which scope a helper's name
	File string // module-relative, slash-separated
	Func string // the top-level function ("Recv.Method" for a method, the first name for a package-level var), "" elsewhere at file scope
	Line int
	Kind kind
	What string
}

func (r reach) String() string {
	fn := r.Func
	if fn == "" {
		fn = "(file scope)"
	}
	return fmt.Sprintf("%s:%d %s: %s", r.File, r.Line, fn, r.What)
}

// ref is a use, inside a top-level function, of a package-level name of the same
// package: To is the name, or "*.m" for a method m called on any value, since the
// parser does not know a receiver's type.
type ref struct {
	Pkg, File, From, To string
	Line                int
}

// fileReaches parses one test file and walks it.
func fileReaches(root, mod, rel string) ([]reach, []ref, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filepath.Join(root, filepath.FromSlash(rel)), nil, 0) // object resolution on: it tells a local from a package name
	if err != nil {
		return nil, nil, fmt.Errorf("parse %s: %w", rel, err)
	}
	w := &walker{fset: fset, file: f, rel: rel, dir: path.Dir(rel), imports: map[string]string{},
		aliases: map[*ast.Object]bool{}, reassigned: map[*ast.Object]bool{}}
	w.pkg = w.dir + " " + f.Name.Name
	// A file in package def (not def_test) names def's symbols unqualified; so does
	// one that dot-imports it.
	if !strings.HasSuffix(f.Name.Name, "_test") {
		w.self = w.dir
	}
	for _, spec := range f.Imports {
		p, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: import %s: %w", rel, spec.Path.Value, err)
		}
		local, ok := strings.CutPrefix(p, mod+"/")
		if !ok {
			// Outside the module: only the path helpers matter, for literal joins.
			local = p
		}
		name := path.Base(p)
		if spec.Name != nil {
			name = spec.Name.Name
		}
		switch name {
		case "_":
		case ".":
			w.dot = append(w.dot, local)
		default:
			w.imports[name] = local
		}
	}
	w.findReassigned()
	for _, d := range f.Decls {
		w.decl(d)
	}
	// A literal inside a literal join names one path, so a line's paths count once.
	paths := map[int]bool{}
	var out []reach
	for _, r := range w.found {
		if r.Kind == kindPath {
			if paths[r.Line] {
				continue
			}
			paths[r.Line] = true
		}
		out = append(out, r)
	}
	return out, w.refs, nil
}

// walker visits one file's syntax. It resolves a name to its package from the file's
// imports, and a bare name inside the package to the package itself, using the
// parser's scopes to tell a package-level name from a local one.
type walker struct {
	fset       *token.FileSet
	file       *ast.File
	rel        string
	dir        string            // module-relative directory of the file
	pkg        string            // dir and package name
	self       string            // the package a bare name belongs to, or "" in an _test package
	dot        []string          // dot-imported packages
	imports    map[string]string // local name to module-relative (or full) import path
	aliases    map[*ast.Object]bool
	reassigned map[*ast.Object]bool
	fn         string
	found      []reach
	refs       []ref
}

func (w *walker) add(pos token.Pos, k kind, what string) {
	w.found = append(w.found, reach{Pkg: w.pkg, File: w.rel, Func: w.fn, Line: w.fset.Position(pos).Line, Kind: k, What: what})
}

// walk visits a child; the caller passes no typed nil.
func (w *walker) walk(n ast.Node) {
	ast.Inspect(n, w.visit)
}

// decl walks one top-level declaration under the name its reaches are keyed by.
func (w *walker) decl(d ast.Decl) {
	switch d := d.(type) {
	case *ast.FuncDecl:
		w.fn = funcKey(d)
		if d.Recv != nil {
			w.walk(d.Recv)
		}
		w.walk(d.Type)
		if d.Body != nil {
			w.walk(d.Body)
		}
	case *ast.GenDecl:
		for _, spec := range d.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				w.walk(spec)
				continue
			}
			// A package-level var is a helper too: a test that names it reads what
			// its initializer read.
			w.fn = vs.Names[0].Name
			for _, n := range vs.Names[1:] {
				if n.Name == "_" {
					continue
				}
				w.refs = append(w.refs, ref{Pkg: w.pkg, File: w.rel, From: n.Name, To: w.fn, Line: w.fset.Position(n.Pos()).Line})
			}
			w.valueSpec(vs)
			if vs.Type != nil {
				w.walk(vs.Type)
			}
			for _, v := range vs.Values {
				w.walk(v)
			}
			w.fn = ""
		}
	}
	w.fn = ""
}

// funcKey names a function, or a method as "Recv.Method".
func funcKey(d *ast.FuncDecl) string {
	if d.Recv == nil || len(d.Recv.List) == 0 {
		return d.Name.Name
	}
	t := d.Recv.List[0].Type
	for {
		switch x := t.(type) {
		case *ast.StarExpr:
			t = x.X
			continue
		case *ast.IndexExpr:
			t = x.X
			continue
		case *ast.IndexListExpr:
			t = x.X
			continue
		case *ast.Ident:
			return x.Name + "." + d.Name.Name
		}
		return "?." + d.Name.Name
	}
}

func (w *walker) visit(n ast.Node) bool {
	switch n := n.(type) {
	case *ast.ImportSpec:
		return false
	case *ast.Field:
		// Field and parameter names declare; only the type is a use.
		w.walk(n.Type)
		return false
	case *ast.KeyValueExpr:
		if id, ok := n.Key.(*ast.Ident); ok {
			if configDirs[id.Name] && w.empty(n.Value, 0) {
				w.add(n.Pos(), kindConfig, id.Name+": \"\" selects the embedded catalog")
			}
			w.walk(n.Value)
			return false
		}
	case *ast.ValueSpec:
		w.valueSpec(n)
	case *ast.AssignStmt:
		w.assign(n)
	case *ast.SelectorExpr:
		if pkg, ok := w.pkgOf(n.X); ok {
			w.symbol(n.Sel.Pos(), pkg, n.Sel.Name, false)
			return false
		}
		if w.fn != "" {
			w.refs = append(w.refs, ref{Pkg: w.pkg, File: w.rel, From: w.fn, To: "*." + n.Sel.Name, Line: w.fset.Position(n.Sel.Pos()).Line})
		}
		w.walk(n.X)
		return false
	case *ast.Ident:
		if w.packageLevel(n) {
			if w.self != "" {
				w.symbol(n.Pos(), w.self, n.Name, true)
			}
			for _, pkg := range w.dot {
				w.symbol(n.Pos(), pkg, n.Name, false)
			}
			if w.fn != "" && n.Name != w.fn && n.Name != "_" {
				w.refs = append(w.refs, ref{Pkg: w.pkg, File: w.rel, From: w.fn, To: n.Name, Line: w.fset.Position(n.Pos()).Line})
			}
		}
	case *ast.CallExpr:
		w.call(n)
	case *ast.BasicLit:
		if s, ok := stringValue(n); ok {
			w.path(n.Pos(), s)
		}
	}
	return true
}

// pkgOf reports the package an expression names, when it is an import's name that
// no local declaration shadows.
func (w *walker) pkgOf(x ast.Expr) (string, bool) {
	id, ok := x.(*ast.Ident)
	if !ok || id.Obj != nil {
		return "", false
	}
	pkg, ok := w.imports[id.Name]
	return pkg, ok
}

// packageLevel reports whether a bare name refers to a package-level declaration:
// one the parser left unresolved (declared in another file of the package) or one
// declared at this file's top level. A local variable has its own object.
func (w *walker) packageLevel(id *ast.Ident) bool {
	return id.Obj == nil || id.Obj == w.file.Scope.Lookup(id.Name)
}

// symbol records a use of name in pkg when it is one of the embedded catalog's.
func (w *walker) symbol(pos token.Pos, pkg, name string, inside bool) {
	names := exported[pkg]
	if inside {
		names = append(append([]string(nil), names...), unexported[pkg]...)
	}
	for _, s := range names {
		if s == name {
			w.add(pos, kindSymbol, path.Base(pkg)+"."+name)
			return
		}
	}
}

// loader reports the loader an expression names: catalog.Load (through an import,
// a dot import, or bare inside catalog), load inside catalog, or a variable bound to
// one, which reports alias.
func (w *walker) loader(x ast.Expr) (name string, alias, ok bool) {
	x = ast.Unparen(x)
	switch x := x.(type) {
	case *ast.SelectorExpr:
		if p, ok := w.pkgOf(x.X); ok && p == catalogPkg && x.Sel.Name == "Load" {
			return "Load", false, true
		}
	case *ast.Ident:
		if x.Obj != nil && w.aliases[x.Obj] {
			return x.Name, true, true
		}
		if !w.packageLevel(x) || !loaders[x.Name] {
			return "", false, false
		}
		if w.self == catalogPkg {
			return x.Name, false, true
		}
		if x.Name == "Load" && slices.Contains(w.dot, catalogPkg) {
			return "Load", false, true
		}
	}
	return "", false, false
}

// call records a catalog loader given an empty directory or called through a
// variable, and checks a join of literal path parts as the one path it builds.
func (w *walker) call(c *ast.CallExpr) {
	if name, alias, ok := w.loader(c.Fun); ok {
		if alias {
			w.add(c.Pos(), kindLoad, fmt.Sprintf("a call through %s, a variable bound to catalog.Load", name))
			return
		}
		for i, label := range []string{"definitions", "projections"} {
			if len(c.Args) > i+1 && w.empty(c.Args[i+1], 0) {
				w.add(c.Pos(), kindLoad, fmt.Sprintf("catalog.%s with an empty %s directory", name, label))
				return
			}
		}
		return
	}
	if sel, ok := ast.Unparen(c.Fun).(*ast.SelectorExpr); ok && sel.Sel.Name == "Join" {
		if p, ok := w.pkgOf(sel.X); ok && (p == "path/filepath" || p == "path") {
			w.join(c)
		}
	}
}

// join checks the literal parts of a filepath.Join: all of them as a path relative to
// the file, and the literal tail after the last computed part as a path that ends
// in the catalog wherever it starts.
func (w *walker) join(c *ast.CallExpr) {
	var tail []string
	all := true
	for _, a := range c.Args {
		s, ok := stringValue(a)
		if !ok {
			all, tail = false, nil
			continue
		}
		tail = append(tail, s)
	}
	if len(tail) < 2 && all {
		return // one literal is checked on its own
	}
	joined := path.Join(tail...)
	if all {
		w.path(c.Pos(), joined)
		return
	}
	for _, d := range catalogDirs {
		if hasPathSuffixIn(joined, d) {
			w.add(c.Pos(), kindPath, fmt.Sprintf("a path ending in %s", d))
			return
		}
	}
}

// assign records a config catalog directory set to "", and a variable bound to a
// loader.
func (w *walker) assign(a *ast.AssignStmt) {
	if len(a.Lhs) != len(a.Rhs) {
		return
	}
	for i, l := range a.Lhs {
		if sel, ok := l.(*ast.SelectorExpr); ok && configDirs[sel.Sel.Name] && w.empty(a.Rhs[i], 0) {
			w.add(a.Pos(), kindConfig, sel.Sel.Name+" = \"\" selects the embedded catalog")
		}
		if id, ok := l.(*ast.Ident); ok && id.Obj != nil {
			if _, _, ok := w.loader(a.Rhs[i]); ok {
				w.aliases[id.Obj] = true
			}
		}
	}
}

// valueSpec records a var bound to a loader.
func (w *walker) valueSpec(vs *ast.ValueSpec) {
	for i, n := range vs.Names {
		if i < len(vs.Values) && n.Obj != nil {
			if _, _, ok := w.loader(vs.Values[i]); ok {
				w.aliases[n.Obj] = true
			}
		}
	}
}

// findReassigned marks each variable the file assigns after declaring it, or takes
// the address of, so empty does not trust its first value.
func (w *walker) findReassigned() {
	mark := func(x ast.Expr, decl ast.Node) {
		if id, ok := ast.Unparen(x).(*ast.Ident); ok && id.Obj != nil && id.Obj.Decl != decl {
			w.reassigned[id.Obj] = true
		}
	}
	ast.Inspect(w.file, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.AssignStmt:
			for _, l := range n.Lhs {
				mark(l, n)
			}
		case *ast.RangeStmt:
			if n.Tok == token.ASSIGN {
				mark(n.Key, n)
				if n.Value != nil {
					mark(n.Value, n)
				}
			}
		case *ast.UnaryExpr:
			if n.Op == token.AND {
				mark(n.X, nil)
			}
		}
		return true
	})
}

// empty reports whether x is the empty string: a literal, or a const or variable
// declared in this file with an empty value (or a string var with none) that the file
// never assigns again.
func (w *walker) empty(x ast.Expr, depth int) bool {
	x = ast.Unparen(x)
	if s, ok := stringValue(x); ok {
		return s == ""
	}
	id, ok := x.(*ast.Ident)
	if !ok || id.Obj == nil || w.reassigned[id.Obj] || depth > 8 {
		return false
	}
	switch d := id.Obj.Decl.(type) {
	case *ast.ValueSpec:
		for i, n := range d.Names {
			if n.Name != id.Name {
				continue
			}
			if len(d.Values) == 0 {
				t, ok := d.Type.(*ast.Ident)
				return id.Obj.Kind == ast.Var && ok && t.Name == "string"
			}
			return i < len(d.Values) && w.empty(d.Values[i], depth+1)
		}
	case *ast.AssignStmt:
		if len(d.Lhs) != len(d.Rhs) {
			return false
		}
		for i, l := range d.Lhs {
			if li, ok := l.(*ast.Ident); ok && li.Obj == id.Obj {
				return w.empty(d.Rhs[i], depth+1)
			}
		}
	}
	return false
}

// path records a string that names an embedded catalog directory on disk, either
// from the module root or relative to the test's own directory (go test runs a test
// in its package directory). A string with spaces or quotes is not a path.
func (w *walker) path(pos token.Pos, s string) {
	if s == "" || strings.ContainsAny(s, " \t\n\"`") {
		return
	}
	p := path.Clean(filepath.ToSlash(s))
	rel := path.Join(w.dir, p)
	for _, d := range catalogDirs {
		if under(p, d) || under(rel, d) || strings.Contains(p, "/"+d+"/") || strings.HasSuffix(p, "/"+d) {
			w.add(pos, kindPath, fmt.Sprintf("the path %q reads %s from disk", s, d))
			return
		}
	}
}

func under(p, dir string) bool { return p == dir || strings.HasPrefix(p, dir+"/") }

// hasPathSuffixIn reports whether p ends with dir, or ends inside it.
func hasPathSuffixIn(p, dir string) bool {
	for _, d := range []string{dir, path.Base(path.Dir(dir)) + "/" + path.Base(dir)} {
		if p == d || strings.HasPrefix(p, d+"/") || strings.HasSuffix(p, "/"+d) || strings.Contains(p, "/"+d+"/") {
			return true
		}
	}
	return false
}

func stringValue(x ast.Expr) (string, bool) {
	lit, ok := x.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(lit.Value)
	return s, err == nil
}
