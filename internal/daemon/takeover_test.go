package daemon

import (
	"context"
	"net/http"
	"testing"

	"github.com/mattjoyce/anam/internal/store/ops"
)

// Card 86: when a release drops a type from the binary, the operational store still
// holds the body it bundled. The daemon then publishes no current Definition for the
// type, so an importer's publish step finds 404 and registers its copy; the same body
// at that held version is adopted, and envelopes admit as before. The gap between the
// deploy and the importer's next batch closes itself.
func TestATypeDroppedFromTheBinaryIsTakenOverByItsImporter(t *testing.T) {
	cfg := importerConfig(t)
	buckets, err := cfg.Buckets()
	if err != nil {
		t.Fatal(err)
	}
	db, err := ops.Open(context.Background(), cfg.DBPath, buckets)
	if err != nil {
		t.Fatal(err)
	}
	// test.reading stands for a type an earlier build bundled at v1.
	if err := db.PutDefinitions(context.Background(), []ops.DefinitionRow{{Type: "test.reading", Version: 1, Body: readingManifest}}); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	_, url, stop := serveImporters(t, cfg)
	defer func() { _ = stop() }()

	call(t, "GET", url+"/v1/definitions/test.reading/1", importerToken, "", http.StatusNotFound, nil)
	var reg struct {
		Type    string `json:"type"`
		Version int    `json:"version"`
	}
	call(t, "POST", url+"/v1/definitions", importerToken, readingManifest, http.StatusCreated, &reg)
	if reg.Type != "test.reading" || reg.Version != 1 {
		t.Fatalf("registration = %+v", reg)
	}
	call(t, "GET", url+"/v1/definitions/test.reading/1", importerToken, "", http.StatusOK, nil)
	call(t, "POST", url+"/v1/envelopes", importerToken,
		batch(`{"type": "test.reading", "definition_version": 1, "payload": {"id": "r-1", "at": "2026-10-10T07:00:00Z", "value": 1.5}}`),
		http.StatusOK, nil)
}
