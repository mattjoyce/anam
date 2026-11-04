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

// catalogPkg is the package whose loader selects a catalog, module-relative.
const catalogPkg = "internal/catalog"

// R4b (card 255) deleted the embedded catalog in slice 5, and with it the symbols
// and source directories this rule once matched: def.Bundled, EmbeddedFS, History,
// Manifests and SetVersion, projection.Bundled and EmbeddedFS, the unexported roots,
// and internal/def/manifests and internal/projection/manifests. None of them can
// appear in a test that compiles. What is left is the call that once fell back on
// that catalog and now selects the empty one.

// loaders are the functions that select the empty catalog for a component whose
// directory is empty: catalog.Load, and load inside the package. A variable bound to
// one is a loader too, and any call through it counts, whatever its arguments.
var loaders = map[string]bool{"Load": true, "load": true}

// configDirs are the config.Config fields that select a catalog directory. A test
// that sets one to "" opens a daemon on the empty catalog for that component.
var configDirs = map[string]bool{"DefinitionsDir": true, "ProjectionsDir": true}

// catalogtestPkg holds the Require helpers a listed test checks its selection with.
const catalogtestPkg = "internal/catalog/catalogtest"

// requireHelpers are catalogtest's helpers that fail a test unless the catalog it was
// given is what an empty directory selects (RequireRefused: unless the load was
// refused). A test on emptyOnPurpose passes each catalog it selects to one of them.
var requireHelpers = map[string]bool{
	"RequireEmpty": true, "RequireNoDefinitions": true, "RequireNoProjections": true, "RequireRefused": true,
}

// kind is the way a reach selects the empty catalog.
type kind int

const (
	kindLoad   kind = iota // catalog.Load with an empty directory
	kindConfig             // a config catalog directory set to ""
	kindHelper             // a call to a test helper that reaches
)

// reach is one place a test selects the empty catalog.
type reach struct {
	Pkg  string // the directory and package name, which scope a helper's name
	File string // module-relative, slash-separated
	Func string // the top-level function ("Recv.Method" for a method, the first name for a package-level var), "" elsewhere at file scope
	Line int
	Kind kind
	What string
	Bind token.Pos // for a load assigned to a variable, where that variable is declared; 0 otherwise
}

// check is a call to a Require helper written directly in a top-level function's
// body, outside any function literal, so it runs whenever the function does.
type check struct {
	File, Func string
	Line       int
	Args       []token.Pos // where each identifier argument is declared; 0 for any other argument
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
func fileReaches(root, mod, rel string) ([]reach, []ref, []check, []string, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filepath.Join(root, filepath.FromSlash(rel)), nil, 0) // object resolution on: it tells a local from a package name
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("parse %s: %w", rel, err)
	}
	w := &walker{fset: fset, file: f, rel: rel, dir: path.Dir(rel), imports: map[string]string{},
		aliases: map[token.Pos]bool{}, reassigned: map[token.Pos]bool{}, binds: map[token.Pos]token.Pos{}}
	w.pkg = w.dir + " " + f.Name.Name
	// A file in package catalog (not catalog_test) names load unqualified; one that
	// dot-imports catalog names Load so.
	if !strings.HasSuffix(f.Name.Name, "_test") {
		w.self = w.dir
	}
	for _, spec := range f.Imports {
		p, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf("%s: import %s: %w", rel, spec.Path.Value, err)
		}
		local, ok := strings.CutPrefix(p, mod+"/")
		if !ok {
			// Outside the module: kept, so a local name is not taken for a package.
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
	return w.found, w.refs, w.checks, w.funcs, nil
}

// walker visits one file's syntax. It resolves a name to its package from the file's
// imports, and a bare name inside the package to the package itself, using the
// parser's scopes to tell a package-level name from a local one.
type walker struct {
	fset       *token.FileSet
	file       *ast.File
	rel        string
	dir        string                  // module-relative directory of the file
	pkg        string                  // dir and package name
	self       string                  // the package a bare name belongs to, or "" in an _test package
	dot        []string                // dot-imported packages
	imports    map[string]string       // local name to module-relative (or full) import path
	aliases    map[token.Pos]bool      // variables bound to a loader, by where they are declared
	reassigned map[token.Pos]bool      // variables set again after their declaration
	binds      map[token.Pos]token.Pos // a call's position to the variable its result is assigned to
	lit        int                     // depth of function literals around the node being walked
	checks     []check
	funcs      []string // the top-level functions declared
	fn         string
	found      []reach
	refs       []ref
}

func (w *walker) add(pos token.Pos, k kind, what string) {
	w.found = append(w.found, reach{Pkg: w.pkg, File: w.rel, Func: w.fn, Line: w.fset.Position(pos).Line, Kind: k, What: what,
		Bind: w.binds[pos]})
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
		w.funcs = append(w.funcs, w.fn)
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
	case *ast.FuncLit:
		// A check inside a literal runs only if the literal is called, so it is not one.
		w.lit++
		w.walk(n.Type)
		w.walk(n.Body)
		w.lit--
		return false
	case *ast.Field:
		// Field and parameter names declare; only the type is a use.
		w.walk(n.Type)
		return false
	case *ast.KeyValueExpr:
		if id, ok := n.Key.(*ast.Ident); ok {
			if configDirs[id.Name] && w.empty(n.Value, 0) {
				w.add(n.Pos(), kindConfig, id.Name+": \"\" selects the empty catalog")
			}
			w.walk(n.Value)
			return false
		}
	case *ast.ValueSpec:
		w.valueSpec(n)
	case *ast.AssignStmt:
		w.assign(n)
	case *ast.SelectorExpr:
		if _, ok := w.pkgOf(n.X); ok {
			return false
		}
		if w.fn != "" {
			w.refs = append(w.refs, ref{Pkg: w.pkg, File: w.rel, From: w.fn, To: "*." + n.Sel.Name, Line: w.fset.Position(n.Sel.Pos()).Line})
		}
		w.walk(n.X)
		return false
	case *ast.Ident:
		if w.packageLevel(n) {
			if w.fn != "" && n.Name != w.fn && n.Name != "_" {
				w.refs = append(w.refs, ref{Pkg: w.pkg, File: w.rel, From: w.fn, To: n.Name, Line: w.fset.Position(n.Pos()).Line})
			}
		}
	case *ast.CallExpr:
		w.call(n)
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
		if x.Obj != nil && w.aliases[x.Obj.Pos()] {
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
// variable.
func (w *walker) call(c *ast.CallExpr) {
	if sel, ok := ast.Unparen(c.Fun).(*ast.SelectorExpr); ok && requireHelpers[sel.Sel.Name] && w.lit == 0 && w.fn != "" {
		if p, ok := w.pkgOf(sel.X); ok && p == catalogtestPkg {
			k := check{File: w.rel, Func: w.fn, Line: w.fset.Position(c.Pos()).Line}
			for _, a := range c.Args {
				var at token.Pos
				if id, ok := ast.Unparen(a).(*ast.Ident); ok && id.Obj != nil {
					at = id.Obj.Pos()
				}
				k.Args = append(k.Args, at)
			}
			w.checks = append(w.checks, k)
		}
	}
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
	}
}

// assign records a config catalog directory set to "", and a variable bound to a
// loader.
func (w *walker) assign(a *ast.AssignStmt) {
	if len(a.Rhs) == 1 && len(a.Lhs) > 0 {
		if c, ok := ast.Unparen(a.Rhs[0]).(*ast.CallExpr); ok {
			if id, ok := a.Lhs[0].(*ast.Ident); ok && id.Obj != nil {
				w.binds[c.Pos()] = id.Obj.Pos()
			}
		}
	}
	if len(a.Lhs) != len(a.Rhs) {
		return
	}
	for i, l := range a.Lhs {
		if sel, ok := l.(*ast.SelectorExpr); ok && configDirs[sel.Sel.Name] && w.empty(a.Rhs[i], 0) {
			w.add(a.Pos(), kindConfig, sel.Sel.Name+" = \"\" selects the empty catalog")
		}
		if id, ok := l.(*ast.Ident); ok && id.Obj != nil {
			if _, _, ok := w.loader(a.Rhs[i]); ok {
				w.aliases[id.Obj.Pos()] = true
			}
		}
	}
}

// valueSpec records a var bound to a loader.
func (w *walker) valueSpec(vs *ast.ValueSpec) {
	if len(vs.Values) == 1 && len(vs.Names) > 0 && vs.Names[0].Obj != nil {
		if c, ok := ast.Unparen(vs.Values[0]).(*ast.CallExpr); ok {
			w.binds[c.Pos()] = vs.Names[0].Obj.Pos()
		}
	}
	for i, n := range vs.Names {
		if i < len(vs.Values) && n.Obj != nil {
			if _, _, ok := w.loader(vs.Values[i]); ok {
				w.aliases[n.Obj.Pos()] = true
			}
		}
	}
}
