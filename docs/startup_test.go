package docs_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

type startupSite struct {
	file, scope, call string
	message, count    int
}

// Each ID names an emitter in a particular declaration, not a bag of literals.
// G4 deliberately requires both branches of logCatalogGaps.
var startupSites = map[string]startupSite{
	"C1": {"app/app.go", "loadRoutedConfig", "fmt.Fprintf", 1, 1},
	"C2": {"app/app.go", "loadRoutedConfig", "fmt.Fprintln", 1, 1},
	"C3": {"config/config.go", "adoptSubjectID", "slog.Warn", 0, 1},
	"C4": {"config/config.go", "adoptSubjectID", "slog.Info", 0, 1},
	"C5": {"config/config.go", "adoptLongWriteTimeout", "slog.Warn", 0, 1},
	"G1": {"daemon/importers.go", "loadDefinitions", "slog.Warn", 0, 1},
	"G2": {"daemon/daemon.go", "Open", "slog.Info", 0, 1},
	"G3": {"daemon/catalog_gaps.go", "logCatalogGaps", "slog.Warn", 0, 1},
	"G4": {"daemon/catalog_gaps.go", "logCatalogGaps", "slog.Warn", 0, 2},
	"G5": {"daemon/catalog_gaps.go", "logCatalogGaps", "slog.Info", 0, 1},
	"S1": {"app/daemon.go", "daemonCmd", "fmt.Fprintf", 1, 1},
	"S2": {"daemon/daemon.go", "Serve", "slog.Info", 0, 1},
	"I1": {"daemon/minting.go", "catchUp", "slog.Info", 0, 1},
	"I2": {"daemon/minting.go", "catchUp", "slog.Warn", 0, 1},
	"I3": {"daemon/minting.go", "catchUp", "slog.Error", 0, 1},
	"M1": {"daemon/minting.go", "mintAtHead", "slog.Info", 0, 1},
	"M2": {"daemon/minting.go", "mintAtHead", "slog.Error", 0, 1},
	"H1": {"daemon/daemon.go", "healthz", "fmt.Fprintln", 1, 1},
	"H2": {"daemon/daemon.go", "healthz", "http.Error", 1, 1},
	"H3": {"daemon/daemon.go", "healthz", "slog.Warn", 0, 1},
	"R0": {"config/config.go", "ErrInvalid", "errors.New", 0, 1},
	"R1": {"config/config.go", "Validate", "invalidSetting", 1, 1},
	"R2": {"config/config.go", "adoptSubjectID", "invalidSetting", 1, 1},
	"R3": {"store/ledger/ledger.go", "ErrSubjectMismatch", "errors.New", 0, 1},
	"R4": {"store/ledger/ledger.go", "checkPin", "fmt.Errorf", 0, 1},
}

type startupQuote struct {
	literal, level string
	keys           []string
}

// TestStartupQuotedOutput checks the reference table against actual message
// arguments, levels and attribute keys. Comments, dead strings and other callers
// cannot preserve a deleted emitter. It also rejects unchecked diagnostic examples.
func TestStartupQuotedOutput(t *testing.T) {
	t.Parallel()
	doc, err := os.ReadFile("STARTUP.md")
	if err != nil {
		t.Fatal(err)
	}
	quotes := startupTable(t, string(doc))
	startupExamples(t, string(doc), quotes)
	files := make(map[string]*ast.File)
	for id, site := range startupSites {
		quote, ok := quotes[id]
		if !ok {
			t.Errorf("STARTUP.md: missing reference row %s", id)
			continue
		}
		file := files[site.file]
		if file == nil {
			file, err = parser.ParseFile(token.NewFileSet(), filepath.Join("..", "internal", site.file), nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			files[site.file] = file
		}
		checkStartupSite(t, id, site, quote, file)
	}
	t.Logf("checked %d reference literals against their production emitters", len(quotes))
}

func startupTable(t *testing.T, doc string) map[string]startupQuote {
	t.Helper()
	quotes := make(map[string]startupQuote)
	inTable := false
	for _, line := range strings.Split(doc, "\n") {
		if line == "| ID | Level / stream | Source literal or format | Attributes, in call order |" {
			if inTable {
				t.Fatal("STARTUP.md: repeated reference table")
			}
			inTable = true
			continue
		}
		if !inTable {
			continue
		}
		if strings.HasPrefix(line, "## ") {
			break
		}
		if strings.TrimSpace(line) == "" || line == "|---|---|---|---|" {
			continue
		}
		cells := strings.Split(line, "|")
		if len(cells) != 6 {
			t.Fatalf("STARTUP.md: malformed reference row: %s", line)
		}
		id, cell := strings.TrimSpace(cells[1]), strings.TrimSpace(cells[3])
		if _, ok := startupSites[id]; !ok {
			t.Fatalf("STARTUP.md: reference ID %s has no emitter binding", id)
		}
		if _, exists := quotes[id]; exists {
			t.Fatalf("STARTUP.md: duplicate reference ID %s", id)
		}
		literal, ok := startupLiteral(cell)
		if !ok {
			t.Fatalf("STARTUP.md %s: quoted-line cell must be exactly one backticked Go string literal: %s", id, cell)
		}
		quote := startupQuote{literal: literal, level: strings.TrimSpace(cells[2])}
		for _, key := range regexp.MustCompile("`([^`]+)`").FindAllStringSubmatch(cells[4], -1) {
			quote.keys = append(quote.keys, key[1])
		}
		quotes[id] = quote
	}
	return quotes
}

func startupLiteral(span string) (string, bool) {
	if !strings.HasPrefix(span, "`\"") || !strings.HasSuffix(span, "\"`") {
		return "", false
	}
	value, err := strconv.Unquote(span[1 : len(span)-1])
	return value, err == nil
}

func startupExamples(t *testing.T, doc string, quotes map[string]startupQuote) {
	t.Helper()
	checked := make(map[string]bool)
	for _, quote := range quotes {
		checked[quote.literal] = true
	}
	isDiagnostic := func(s string) bool {
		return strings.Contains(s, "daemon: ") || strings.Contains(s, "config: ") || strings.Contains(s, "healthz: ")
	}
	var inline strings.Builder
	fence := ""
	for _, line := range strings.Split(doc, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			end := 0
			for end < len(trimmed) && trimmed[end] == trimmed[0] {
				end++
			}
			if fence == "" {
				fence = trimmed[:end]
				continue
			}
			if strings.HasPrefix(trimmed[:end], fence) && strings.TrimSpace(trimmed[end:]) == "" {
				fence = ""
				continue
			}
		}
		if fence != "" {
			if isDiagnostic(line) {
				value, err := strconv.Unquote(trimmed)
				if err != nil || !checked[value] {
					t.Errorf("STARTUP.md: unchecked fenced diagnostic: %s", line)
				}
			}
			continue
		}
		inline.WriteString(line + "\n")
	}
	if fence != "" {
		t.Error("STARTUP.md: unclosed code fence")
	}
	// Backtick runs include ordinary and multi-backtick spans, even across lines.
	for _, span := range regexp.MustCompile("(?s)(`+)([^`]*)(`+)").FindAllStringSubmatch(inline.String(), -1) {
		if len(span[1]) != len(span[3]) {
			t.Errorf("STARTUP.md: mismatched code span: %s", span[0])
			continue
		}
		value, err := strconv.Unquote(span[2])
		if isDiagnostic(span[2]) || err == nil {
			if err != nil || !checked[value] {
				t.Errorf("STARTUP.md: unchecked inline diagnostic/literal: %s", span[0])
			}
		}
	}
}

func checkStartupSite(t *testing.T, id string, site startupSite, quote startupQuote, file *ast.File) {
	t.Helper()
	if strings.HasPrefix(site.call, "slog.") && quote.level != strings.ToUpper(strings.TrimPrefix(site.call, "slog.")) {
		t.Errorf("STARTUP.md %s: level %s disagrees with bound %s", id, quote.level, site.call)
	}
	matches := 0
	for _, decl := range file.Decls {
		var scope ast.Node
		switch decl := decl.(type) {
		case *ast.FuncDecl:
			if decl.Name.Name == site.scope {
				scope = decl.Body
			}
		case *ast.GenDecl:
			for _, spec := range decl.Specs {
				if value, ok := spec.(*ast.ValueSpec); ok {
					for _, name := range value.Names {
						if name.Name == site.scope {
							scope = value
						}
					}
				}
			}
		}
		if scope == nil {
			continue
		}
		ast.Inspect(scope, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || startupCallName(call.Fun) != site.call || len(call.Args) <= site.message {
				return true
			}
			message, ok := startupString(call.Args[site.message])
			if !ok || message != quote.literal {
				return true
			}
			matches++
			if strings.HasPrefix(site.call, "slog.") {
				checkStartupKeys(t, id, quote.keys, call.Args[site.message+1:])
			}
			return true
		})
	}
	if matches != site.count {
		t.Errorf("STARTUP.md %s: %s %s: want %d %s message argument(s) %q, found %d", id, site.file, site.scope, site.count, site.call, quote.literal, matches)
	}
}

func checkStartupKeys(t *testing.T, id string, want []string, args []ast.Expr) {
	t.Helper()
	var keys []string
	if len(args)%2 != 0 {
		t.Errorf("STARTUP.md %s: log attributes are not key/value pairs", id)
	}
	for i := 0; i < len(args); i += 2 {
		key, ok := startupString(args[i])
		if !ok {
			t.Errorf("STARTUP.md %s: attribute key is not a literal", id)
		}
		keys = append(keys, key)
	}
	if !reflect.DeepEqual(keys, want) {
		t.Errorf("STARTUP.md %s: attribute keys %v, source has %v", id, want, keys)
	}
}

func startupString(expr ast.Expr) (string, bool) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(lit.Value)
	return value, err == nil
}

func startupCallName(expr ast.Expr) string {
	switch expr := expr.(type) {
	case *ast.Ident:
		return expr.Name
	case *ast.SelectorExpr:
		return startupCallName(expr.X) + "." + expr.Sel.Name
	default:
		return ""
	}
}
