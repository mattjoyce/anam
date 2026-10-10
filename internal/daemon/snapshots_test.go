package daemon

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/api"
)

// snapBatch is a batch labelled as one part of snapshot id.
func snapBatch(id string, part, parts int, envs ...string) string {
	return fmt.Sprintf(`{"snapshot": {"id": %q, "part": %d, "parts": %d, "types": ["health.weight"]}, "envelopes": [%s]}`,
		id, part, parts, strings.Join(envs, ","))
}

// Card 78: a complete resend in parts. Each part admits as a batch does and reports
// the run's progress; once complete, the report counts the source's live facets and
// what the run left absent. A later run that leaves one out reports it absent, and
// tombstones nothing.
func TestSnapshotReportsWhatACompleteResendLeftAbsent(t *testing.T) {
	cfg := importerConfig(t)
	d, url, stop := serveImporters(t, cfg)
	defer func() { _ = stop() }()
	weight, _ := d.defs.current.Load().Lookup("health.weight")
	v := weight.Version

	var a api.Admission
	call(t, "POST", url+"/v1/envelopes", importerToken, snapBatch("run-1", 2, 2, weighIn("w-3", 81, v)), http.StatusOK, &a)
	if a.Snapshot == nil || a.Snapshot.Complete || a.Snapshot.PartsAdmitted != 1 {
		t.Fatalf("after part 2 of 2: %+v", a.Snapshot)
	}
	var rep api.SnapshotReport
	call(t, "GET", url+"/v1/snapshots/run-1", importerToken, "", http.StatusOK, &rep)
	if rep.Complete || rep.Absent != nil {
		t.Fatalf("an incomplete snapshot reported %+v", rep)
	}
	call(t, "POST", url+"/v1/envelopes", importerToken, snapBatch("run-1", 1, 2, weighIn("w-1", 80, v), weighIn("w-2", 80.5, v)), http.StatusOK, &a)
	if !a.Snapshot.Complete {
		t.Fatalf("after both parts: %+v", a.Snapshot)
	}
	call(t, "GET", url+"/v1/snapshots/run-1", importerToken, "", http.StatusOK, &rep)
	if !rep.Complete || rep.Carried != 3 || *rep.Live != 3 || *rep.Absent != 0 {
		t.Fatalf("complete run-1: %+v live %v absent %v", rep, rep.Live, rep.Absent)
	}

	// A later run that no longer holds w-2.
	call(t, "POST", url+"/v1/envelopes", importerToken, snapBatch("run-2", 1, 1, weighIn("w-1", 80, v), weighIn("w-3", 81, v)), http.StatusOK, &a)
	call(t, "GET", url+"/v1/snapshots/run-2", importerToken, "", http.StatusOK, &rep)
	if *rep.Live != 3 || *rep.Absent != 1 || len(rep.AbsentIDs) != 1 {
		t.Fatalf("run-2: live %v absent %v ids %v", *rep.Live, *rep.Absent, rep.AbsentIDs)
	}
	before := head(t, d)

	// A weigh-in sent after run-2 completed, by an incremental run, is not run-2's
	// to call absent.
	call(t, "POST", url+"/v1/envelopes", importerToken, batch(weighIn("w-4", 82, v)), http.StatusOK, &a)
	call(t, "GET", url+"/v1/snapshots/run-2", importerToken, "", http.StatusOK, &rep)
	if *rep.Live != 4 || *rep.Absent != 1 {
		t.Fatalf("after an incremental weigh-in: live %v absent %v", *rep.Live, *rep.Absent)
	}
	// Reporting tombstoned nothing: only the incremental batch moved W.
	if after := head(t, d); after != before+1 {
		t.Errorf("W moved from %d to %d across a report", before, after)
	}
}

// A part that does not fit its label is refused before anything is admitted.
func TestSnapshotPartsAreChecked(t *testing.T) {
	cfg := importerConfig(t)
	d, url, stop := serveImporters(t, cfg)
	defer func() { _ = stop() }()
	weight, _ := d.defs.current.Load().Lookup("health.weight")
	v := weight.Version
	w0 := head(t, d)

	for name, body := range map[string]string{
		"part beyond parts": snapBatch("r", 3, 2, weighIn("w-1", 80, v)),
		"a bad id":          snapBatch("no spaces", 1, 1, weighIn("w-1", 80, v)),
		"a type the token lacks": `{"snapshot": {"id": "r", "part": 1, "parts": 1, "types": ["health.sleep"]}, "envelopes": [` +
			weighIn("w-1", 80, v) + `]}`,
		"an envelope outside the types": `{"snapshot": {"id": "r", "part": 1, "parts": 1, "types": ["test.reading"]}, "envelopes": [` +
			weighIn("w-1", 80, v) + `]}`,
	} {
		t.Run(name, func(t *testing.T) {
			call(t, "POST", url+"/v1/envelopes", importerToken, body, http.StatusBadRequest, nil)
		})
	}
	call(t, "POST", url+"/v1/envelopes", importerToken, snapBatch("r", 1, 2, weighIn("w-1", 80, v)), http.StatusOK, nil)
	w1 := head(t, d)
	// The same id with another part count is another run: refused, nothing admitted.
	call(t, "POST", url+"/v1/envelopes", importerToken, snapBatch("r", 2, 3, weighIn("w-2", 80, v)), http.StatusConflict, nil)
	if w := head(t, d); w != w1 || w1 != w0+1 {
		t.Errorf("W %d, %d, %d: a refused part admitted something", w0, w1, w)
	}
}

// An importer sees only its own source's snapshots; the operator sees any; an unknown
// id is 404.
func TestSnapshotReportAccess(t *testing.T) {
	cfg := importerConfig(t)
	const otherToken = "other-importer-token-0123456789abcdef0123"
	sum := sha256.Sum256([]byte(otherToken))
	f, err := os.OpenFile(cfg.ImportersFile, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fmt.Fprintf(f, "  - source: other-import\n    token_sha256: %s\n    types: [health.sleep]\n", hex.EncodeToString(sum[:])); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	d, url, stop := serveImporters(t, cfg)
	defer func() { _ = stop() }()
	weight, _ := d.defs.current.Load().Lookup("health.weight")
	call(t, "POST", url+"/v1/envelopes", importerToken, snapBatch("mine", 1, 1, weighIn("w-1", 80, weight.Version)), http.StatusOK, nil)

	var rep api.SnapshotReport
	call(t, "GET", url+"/v1/snapshots/mine", testToken, "", http.StatusOK, &rep)
	if rep.Source != "test-import" || !slices.Equal(rep.Types, []string{"health.weight"}) {
		t.Errorf("operator's report: %+v", rep)
	}
	call(t, "GET", url+"/v1/snapshots/mine", otherToken, "", http.StatusNotFound, nil)
	call(t, "GET", url+"/v1/snapshots/nope", importerToken, "", http.StatusNotFound, nil)
	call(t, "GET", url+"/v1/snapshots/mine", "", "", http.StatusUnauthorized, nil)
}
