package ledger

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// Append-only fell from a substrate-enforced invariant to a code-enforced one when
// the ledger moved to DynamoDB, which cannot refuse item mutation (§8). This is the
// test that must therefore exist: no code in this package may call the item-mutating
// or item-deleting APIs. The one permitted update is the watermark counter, expressed
// as a transaction item (types.Update), never as the top-level API call.
//
// The check walks the AST rather than grepping source, so a comment may name the
// forbidden calls (this one just did) without tripping it. The threat is not an
// adversary; it is us, mid-refactor, reaching for the convenient thing.
func TestAppendOnly_NoMutatingCallsInCode(t *testing.T) {
	t.Parallel()

	forbidden := map[string]bool{
		"UpdateItem":       true,
		"DeleteItem":       true,
		"BatchWriteItem":   true,
		"ExecuteStatement": true,
		"PartiQL":          true,
	}

	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", nil, 0)
	if err != nil {
		t.Fatal(err)
	}

	for _, pkg := range pkgs {
		for filename, file := range pkg.Files {
			ast.Inspect(file, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				if forbidden[sel.Sel.Name] {
					t.Errorf("%s: %s is called at %s — the ledger is append-only, and the substrate will not hold that invariant for us",
						filename, sel.Sel.Name, fset.Position(sel.Pos()))
				}
				return true
			})
		}
	}
}
