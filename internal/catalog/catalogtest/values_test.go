package catalogtest_test

import (
	"fmt"
	"go/ast"
	"go/token"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

// The walker's readings of values: the paths a string or a join names, and whether
// an expression is the empty string.

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

// findReassigned marks each variable the file assigns after declaring it, or takes
// the address of, so empty does not trust its first value.
func (w *walker) findReassigned() {
	mark := func(x ast.Expr, decl ast.Node) {
		if id, ok := ast.Unparen(x).(*ast.Ident); ok && id.Obj != nil && id.Obj.Decl != decl {
			w.reassigned[id.Obj.Pos()] = true
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
	if !ok || id.Obj == nil || w.reassigned[id.Obj.Pos()] || depth > 8 {
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
