package daemon

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"testing"

	"github.com/mattjoyce/anam/internal/api"
)

const scaleToken = "scale-importer-token-0123456789abcdef01234"

// serveCoverage serves importerConfig with a second importer, scale-test, which
// submits health.weight beside test-import, and rolesConfig's reader.
func serveCoverage(t *testing.T) (*Daemon, string) {
	t.Helper()
	cfg := importerConfig(t)
	scale, reader := sha256.Sum256([]byte(scaleToken)), sha256.Sum256([]byte(readerToken))
	extra := fmt.Sprintf("  - source: scale-test\n    token_sha256: %s\n    types: [health.weight]\n"+
		"principals:\n  - name: assistant\n    role: reader\n    token_sha256: %s\n",
		hex.EncodeToString(scale[:]), hex.EncodeToString(reader[:]))
	f, err := os.OpenFile(cfg.ImportersFile, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.WriteString(extra); err != nil {
		t.Fatal(err)
	}
	d, url, _ := serveImporters(t, cfg)
	return d, url
}

func weighAt(version int, id, date string) string {
	return fmt.Sprintf(`{"type": "health.weight", "definition_version": %d, "payload": {"id": %q, "date": %q, "kg": 80}}`,
		version, id, date)
}

func readingAt(id, at string) string {
	return fmt.Sprintf(`{"type": "test.reading", "definition_version": 1, "payload": {"id": %q, "at": %q, "value": 1}}`, id, at)
}

// Card 90: coverage counts each type's live facets, their fact-time range and the
// sources they came from, and a W in the past answers the record as it stood then.
func TestCoverage(t *testing.T) {
	d, url := serveCoverage(t)
	call(t, "POST", url+"/v1/definitions", importerToken, readingManifest, http.StatusCreated, nil)
	weight, _ := d.defs.current.Load().Lookup("health.weight")
	v := weight.Version

	submit := func(token, body string) int64 {
		t.Helper()
		var a api.Admission
		call(t, "POST", url+"/v1/envelopes", token, body, http.StatusOK, &a)
		return a.W
	}
	first := submit(importerToken, batch(weighAt(v, "w-1", "2026-03-01 07:00"), readingAt("r-1", "2026-05-01T09:00:00Z")))
	submit(scaleToken, batch(weighAt(v, "s-1", "2025-12-31 23:00"), weighAt(v, "s-2", "2026-04-02 07:00")))

	coverage := func(query string) api.Coverage {
		t.Helper()
		var c api.Coverage
		call(t, "GET", url+"/v1/coverage"+query, readerToken, "", http.StatusOK, &c)
		return c
	}
	now := coverage("")
	if now.LedgerID == "" || now.Live != 4 || now.Projection != nil {
		t.Fatalf("head coverage: %+v", now)
	}
	// Weight is logged in Sydney time; fact-times are held in UTC.
	want := []api.TypeCoverage{
		{Type: "health.weight", Count: 3, Projections: []string{"body"},
			Earliest: "2025-12-31T12:00:00.000000000Z", Latest: "2026-04-01T20:00:00.000000000Z",
			Sources: []api.SourceCoverage{
				{Source: "scale-test", Count: 2, Earliest: "2025-12-31T12:00:00.000000000Z", Latest: "2026-04-01T20:00:00.000000000Z"},
				{Source: "test-import", Count: 1, Earliest: "2026-02-28T20:00:00.000000000Z", Latest: "2026-02-28T20:00:00.000000000Z"},
			}},
		{Type: "test.reading", Count: 1, Projections: []string{},
			Earliest: "2026-05-01T09:00:00.000000000Z", Latest: "2026-05-01T09:00:00.000000000Z",
			Sources: []api.SourceCoverage{
				{Source: "test-import", Count: 1, Earliest: "2026-05-01T09:00:00.000000000Z", Latest: "2026-05-01T09:00:00.000000000Z"},
			}},
	}
	if !reflect.DeepEqual(now.Types, want) {
		t.Fatalf("head coverage types:\n got %+v\nwant %+v", now.Types, want)
	}
	if again := coverage(fmt.Sprintf("?w=%d", now.W)); !reflect.DeepEqual(again, now) {
		t.Fatalf("the head's W named: %+v, want %+v", again, now)
	}

	then := coverage(fmt.Sprintf("?w=%d", first))
	if then.W != first || then.Live != 2 || len(then.Types) != 2 || then.Canonical == now.Canonical {
		t.Fatalf("coverage at w=%d: %+v", first, then)
	}
	if w := then.Types[0]; w.Count != 1 || len(w.Sources) != 1 || w.Sources[0].Source != "test-import" {
		t.Fatalf("weight at w=%d: %+v", first, w)
	}

	// Card 231: each type names the projections that serve it, the newest version of each
	// name held. The bundled body serves weight; the reading is the gap, an empty list that
	// reads [] and not null. A projection the operator registers joins the list, sorted.
	call(t, "POST", url+"/v1/projections", testToken, weightsV1, http.StatusCreated, nil)
	registered := coverage("")
	if got := registered.Types[0].Projections; !reflect.DeepEqual(got, []string{"body", "weights"}) {
		t.Fatalf("weight is served by %v, want body and weights", got)
	}
	if got := registered.Types[1].Projections; got == nil || len(got) != 0 {
		t.Fatalf("a type no projection serves: %#v, want an empty list", got)
	}

	// A projection counts what it serves: meds serves neither type.
	meds := coverage("?projection=meds")
	if meds.Projection == nil || meds.Projection.Name != "meds" || meds.Live != 0 || len(meds.Types) != 0 ||
		meds.Canonical != now.Canonical {
		t.Fatalf("coverage through meds: %+v", meds)
	}

	// Before the first anam-event Anam knew nothing.
	if before := coverage("?as_of=2000-01-01&zone=UTC"); before.W != 0 || before.Live != 0 || len(before.Types) != 0 {
		t.Fatalf("coverage as of 2000: %+v", before)
	}

	for _, bad := range []string{"?as_of=2026-01-01", "?w=1&as_of=2026-01-01&zone=UTC", "?w=-1", fmt.Sprintf("?w=%d", now.W+1), "?w=x", "?projection=nonesuch"} {
		call(t, "GET", url+"/v1/coverage"+bad, readerToken, "", http.StatusBadRequest, nil)
	}
}
