package policy

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

const module = "github.com/mattjoyce/anam/"

// reachable are the packages a policy decision may read, by their path in the module:
// the importers file's shape (config) and what config itself reads, and facet for the
// name of a type, never a payload.
var reachable = []string{
	"internal/policy", "internal/config", "internal/facet", "internal/safepath", "internal/span",
}

// readsPayload are the packages that read a payload, a fold or a projection, named so a
// failure says which line was crossed. The allow-list above already keeps them out.
var readsPayload = []string{
	"internal/fold", "internal/query", "internal/projection", "internal/gate", "internal/serve",
	"internal/read", "internal/receipt", "internal/def", "internal/consent", "internal/frame",
	"internal/selection", "internal/store",
}

// The policy reads the importers file's principals and the request, never a payload:
// every package it reaches, directly or through another, is on the short list, and none
// is one that reads a payload.
func TestPolicyReadsNoPayload(t *testing.T) {
	root := filepath.Join("..", "..")
	seen := map[string]bool{}
	queue := []string{"internal/policy"}
	for len(queue) > 0 {
		pkg := queue[0]
		queue = queue[1:]
		if seen[pkg] {
			continue
		}
		seen[pkg] = true
		queue = append(queue, moduleImports(t, filepath.Join(root, pkg))...)
	}
	for pkg := range seen {
		for _, bad := range readsPayload {
			if pkg == bad || strings.HasPrefix(pkg, bad+"/") {
				t.Errorf("the policy reaches %s, which reads payloads", pkg)
			}
		}
		if !slices.Contains(reachable, pkg) {
			t.Errorf("the policy reaches %s, which is not the importers file or the request", pkg)
		}
	}
}

// moduleImports are the module's packages the non-test Go files in dir import.
func moduleImports(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	fset := token.NewFileSet()
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, spec := range f.Imports {
			path, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			if rest, ok := strings.CutPrefix(path, module); ok {
				out = append(out, rest)
			}
		}
	}
	return out
}
