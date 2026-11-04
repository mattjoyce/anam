package catalogtest_test

import (
	"go/ast"
	"go/token"
	"strconv"
)

// The walker's readings of values: whether an expression is the empty string.

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

func stringValue(x ast.Expr) (string, bool) {
	lit, ok := x.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(lit.Value)
	return s, err == nil
}
