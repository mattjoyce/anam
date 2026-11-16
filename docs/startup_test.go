package docs_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// TestStartupQuotedOutput keeps the guide's quoted diagnostics tied to production
// literals. Reading Go syntax excludes comments and tests that repeat old messages.
func TestStartupQuotedOutput(t *testing.T) {
	t.Parallel()
	doc, err := os.ReadFile("STARTUP.md")
	if err != nil {
		t.Fatal(err)
	}
	quoted := regexp.MustCompile("`(\"(?:[^\"\\\\]|\\\\.)*\")`").FindAllSubmatch(doc, -1)
	if len(quoted) == 0 {
		t.Fatal("STARTUP.md has no quoted output to check")
	}
	literals := make(map[string]bool)
	for _, dir := range []string{"daemon", "config", "app"} {
		files, err := filepath.Glob(filepath.Join("..", "internal", dir, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, path := range files {
			if strings.HasSuffix(path, "_test.go") {
				continue
			}
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			ast.Inspect(file, func(node ast.Node) bool {
				if lit, ok := node.(*ast.BasicLit); ok && lit.Kind == token.STRING {
					value, err := strconv.Unquote(lit.Value)
					if err != nil {
						t.Fatal(err)
					}
					literals[value] = true
				}
				return true
			})
		}
	}
	for _, match := range quoted {
		value, err := strconv.Unquote(string(match[1]))
		if err != nil {
			t.Fatal(err)
		}
		if !literals[value] {
			t.Errorf("STARTUP.md quotes %q, absent from production source in internal/{daemon,config,app}", value)
		}
	}
	t.Logf("checked %d quoted output strings against production source", len(quoted))
}
