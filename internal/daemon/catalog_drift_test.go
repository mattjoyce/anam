package daemon

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/projection"
)

// Serial because each case replaces the process default logger.
func TestOpenReportsCatalogDrift(t *testing.T) {
	for _, mode := range []string{"embedded", "copy", "definitions only", "projections only", "changed", "capped"} {
		t.Run(mode, func(t *testing.T) {
			cfg, changed := driftConfig(t, mode)
			count := len(changed)
			var buf bytes.Buffer
			prev := slog.Default()
			slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
			t.Cleanup(func() { slog.SetDefault(prev) })
			d, err := Open(context.Background(), cfg, Options{})
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = d.closeStores() }()
			reports := driftReports(t, &buf)
			if mode == "embedded" {
				if len(reports) != 0 {
					t.Fatalf("embedded start logged %+v", reports)
				}
				return
			}
			if len(reports) != 1 {
				t.Fatalf("got %d drift reports, want one", len(reports))
			}
			report := reports[0]
			if count == 0 {
				if report["msg"] != "daemon: catalog matches embedded" || report["level"] != "INFO" {
					t.Fatalf("match report = %+v", report)
				}
				return
			}
			if report["msg"] != "daemon: catalog drifts from embedded" || report["level"] != "WARN" || report["set_version_differs"] != true {
				t.Fatalf("drift report = %+v", report)
			}
			checkDriftFields(t, report, changed)
			if mode == "capped" && count <= 20 {
				t.Fatal("fixture does not exercise truncation")
			}
		})
	}
}

func driftConfig(t *testing.T, mode string) (config.Config, []string) {
	t.Helper()
	cfg := newConfig(t)
	if mode != "embedded" && mode != "projections only" {
		cfg.DefinitionsDir = copyCatalogRoot(t, def.EmbeddedFS())
	}
	if mode != "embedded" && mode != "definitions only" {
		cfg.ProjectionsDir = copyCatalogRoot(t, projection.EmbeddedFS())
	}
	var names []string
	if mode == "changed" || mode == "capped" {
		names = []string{"health.weight.json"}
		if mode == "capped" {
			entries, err := os.ReadDir(cfg.DefinitionsDir)
			if err != nil {
				t.Fatal(err)
			}
			names = nil
			for _, e := range entries {
				if strings.HasSuffix(e.Name(), ".json") {
					names = append(names, e.Name())
				}
			}
		}
		for _, name := range names {
			path := filepath.Join(cfg.DefinitionsDir, name)
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, append(raw, '\n'), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	sort.Strings(names)
	return cfg, names
}

func driftReports(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var reports []map[string]any
	admitted := false
	scanner := bufio.NewScanner(buf)
	for scanner.Scan() {
		var record map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			t.Fatal(err)
		}
		msg, _ := record["msg"].(string)
		if msg == "daemon: catalog admitted" {
			admitted = true
		}
		if msg == "daemon: catalog matches embedded" || msg == "daemon: catalog drifts from embedded" {
			if !admitted {
				t.Fatal("drift logged before admission")
			}
			reports = append(reports, record)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return reports
}

// checkDriftFields wants changed, sorted, as definitions_differ: the count, the first
// 20 names in order, and how many more. Every other field is empty.
func checkDriftFields(t *testing.T, report map[string]any, changed []string) {
	t.Helper()
	for _, field := range []string{"definitions_only_here", "definitions_only_there", "definitions_differ", "projections_only_here", "projections_only_there", "projections_differ"} {
		var wantNames []string
		if field == "definitions_differ" {
			wantNames = changed
		}
		want := len(wantNames)
		if report[field+"_count"] != float64(want) || report[field+"_more"] != float64(max(0, want-20)) {
			t.Errorf("%s counts = %+v", field, report)
		}
		files, ok := report[field].([]any)
		// An empty slice can encode as null.
		if report[field] != nil && !ok {
			t.Fatalf("%s files = %v", field, report[field])
		}
		if len(files) != min(want, 20) {
			t.Errorf("%s names = %v", field, files)
		}
		for i, f := range files {
			if i < len(wantNames) && f != wantNames[i] {
				t.Errorf("%s names[%d] = %v, want %s", field, i, f, wantNames[i])
			}
		}
	}
}
