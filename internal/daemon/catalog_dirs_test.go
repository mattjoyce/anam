package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/catalog/fixture"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/store/ops"
)

func openStore(t *testing.T, ctx context.Context, cfg config.Config) *ops.DB {
	t.Helper()
	buckets, err := cfg.Buckets()
	if err != nil {
		t.Fatal(err)
	}
	db, err := ops.Open(ctx, cfg.DBPath, buckets)
	if err != nil {
		t.Fatalf("ops.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// A template whose file has left the projections directory is gone from the next
// start: the files say what is current, so it is neither listed nor served. A receipt
// served through it still verifies, from the body the receipt filed when it was served.
func TestReceiptVerifiesAfterItsTemplateFileLeaves(t *testing.T) {
	t.Parallel()
	// readerConfig's projections directory is the test's own copy of the fixture.
	cfg := readerConfig(t)

	d := open(t, cfg)
	srv := httptest.NewServer(d.Handler())
	var minted api.Published
	call(t, "GET", srv.URL+timelineMeasures, recorderToken, "", http.StatusOK, &minted)
	srv.Close()
	if err := d.closeStores(); err != nil {
		t.Fatal(err)
	}

	if err := os.Remove(filepath.Join(cfg.ProjectionsDir, "measures.json")); err != nil {
		t.Fatal(err)
	}
	d2 := open(t, cfg)
	t.Cleanup(func() { _ = d2.closeStores() })
	if _, err := d2.projections.current.Load().Lookup("measures"); err == nil {
		t.Error("measures is still served after its file left the catalog")
	}
	srv2 := httptest.NewServer(d2.Handler())
	defer srv2.Close()
	var verified api.ReceiptVerified
	call(t, "GET", srv2.URL+"/v1/receipts/"+minted.ReceiptHash+"/verify", recorderToken, "", http.StatusOK, &verified)
	if !verified.Result.OK() {
		t.Fatalf("the receipt does not verify after the measures file left: %s", verified.Result.Summary())
	}
	if !slices.ContainsFunc(verified.Result.Notes, func(n string) bool {
		return strings.Contains(n, "from the body filed with the receipt")
	}) {
		t.Errorf("the verdict does not say it replayed the filed body: %q", verified.Result.Notes)
	}
}

// A start refused by a check writes nothing of the catalog: the check runs before the
// store takes the catalog's Definitions.
func TestOpenRefusedStartWritesNoCatalog(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	cfg := newConfig(t)
	cfg.DefaultProjection = "no-such-projection"

	if _, err := Open(ctx, cfg, Options{}); err == nil {
		t.Fatal("Open with an unknown default_projection succeeded")
	}
	db := openStore(t, ctx, cfg)
	if rows, err := db.Definitions(ctx); err != nil || len(rows) != 0 {
		t.Errorf("a refused start stored %d definitions, %v", len(rows), err)
	}
}

// A registered Definition that contradicts the catalog's at the same type and version
// stops the start, and the catalog is not written.
func TestOpenRefusesARegisteredDefinitionTheCatalogContradicts(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	cfg := newConfig(t)
	db := openStore(t, ctx, cfg)

	defs, _ := fixture.Engine()
	manifests, err := def.ManifestsFS(defs)
	if err != nil || len(manifests) == 0 {
		t.Fatalf("no catalog manifests: %v", err)
	}
	m := manifests[0]
	var body map[string]any
	if err := json.Unmarshal(m.Body, &body); err != nil {
		t.Fatal(err)
	}
	body["x_contradiction"] = true
	changed, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.RegisterDefinition(ctx, ops.DefinitionRow{Type: string(m.Type), Version: m.Version, Body: string(changed)}, "2026-10-30T00:00:00Z"); err != nil {
		t.Fatalf("RegisterDefinition: %v", err)
	}
	_ = db.Close()

	_, err = Open(ctx, cfg, Options{})
	if !errors.Is(err, def.ErrBadManifest) {
		t.Fatalf("Open = %v, want def.ErrBadManifest", err)
	}
	if !strings.Contains(err.Error(), string(m.Type)) {
		t.Errorf("error %q does not name %s", err, m.Type)
	}
	db2 := openStore(t, ctx, cfg)
	if rows, err := db2.Definitions(ctx); err != nil || len(rows) != 0 {
		t.Errorf("a refused start stored %d definitions, %v", len(rows), err)
	}
}

// One start event names where the catalog came from and its Definition set version.
// Not parallel: it swaps the default logger, and Go runs a package's serial tests
// before any parallel one resumes, so no other test logs into its buffer.
func TestOpenLogsTheCatalogItAdmitted(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	cfg := newConfig(t)
	d, err := Open(context.Background(), cfg, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.closeStores() }()

	out := buf.String()
	for _, want := range []string{
		"catalog admitted",
		"definitions_source=" + cfg.DefinitionsDir,
		"projections_source=" + cfg.ProjectionsDir,
		"definition_set_version=" + d.catalog.SetVersion(),
	} {
		if !strings.Contains(out, want) {
			t.Errorf("log %q lacks %q", out, want)
		}
	}
}

// A receipt minted on one catalog directory verifies the same on another directory
// copied from the same files: same result, same notes, no "definition set has moved".
// Where the files came from never enters a content hash.
func TestReceiptVerifiesTheSameOnACopiedCatalog(t *testing.T) {
	t.Parallel()
	cfg := readerConfig(t)

	d := open(t, cfg)
	srv := httptest.NewServer(d.Handler())
	var minted api.Published
	call(t, "GET", srv.URL+timelineMeasures, recorderToken, "", http.StatusOK, &minted)
	var first api.ReceiptVerified
	call(t, "GET", srv.URL+"/v1/receipts/"+minted.ReceiptHash+"/verify", recorderToken, "", http.StatusOK, &first)
	srv.Close()
	if err := d.closeStores(); err != nil {
		t.Fatal(err)
	}
	if !first.Result.OK() {
		t.Fatalf("the receipt does not verify on the first directory: %s", first.Result.Summary())
	}

	defs, projs := fixture.Engine()
	cfg.DefinitionsDir, cfg.ProjectionsDir = fixture.Dirs(t, defs, projs)
	d2 := open(t, cfg)
	t.Cleanup(func() { _ = d2.closeStores() })
	srv2 := httptest.NewServer(d2.Handler())
	defer srv2.Close()
	var copied api.ReceiptVerified
	call(t, "GET", srv2.URL+"/v1/receipts/"+minted.ReceiptHash+"/verify", recorderToken, "", http.StatusOK, &copied)

	if copied.Result.OK() != first.Result.OK() || copied.Result.Summary() != first.Result.Summary() {
		t.Errorf("verification differs on a copied catalog:\n first:  %s\n copied: %s",
			first.Result.Summary(), copied.Result.Summary())
	}
	if !reflect.DeepEqual(copied.Result, first.Result) {
		t.Errorf("verification result differs on a copied catalog:\n first:  %+v\n copied: %+v", first.Result, copied.Result)
	}
}
