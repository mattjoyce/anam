package daemon

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/mattjoyce/anam/internal/api"
)

// Card 107: a part is its envelopes, order aside. A resend of a part number with
// other envelopes is 409 and admits nothing, while the run is incomplete and after it
// completes; an exact retry, in any order, is 200 and changes nothing. The audit
// (the API pressure review of 2026-10-11, finding 2, in the private review notes) resent a completed 1/1 part with
// another facet and watched carried and absent move under a fixed completed_w.
func TestSnapshotPartIsBoundToItsEnvelopes(t *testing.T) {
	cfg := importerConfig(t)
	d, url, stop := serveImporters(t, cfg)
	defer func() { _ = stop() }()
	weight, _ := d.defs.current.Load().Lookup("health.weight")
	v := weight.Version

	// a, b and c are live before the run; the run will carry a and b.
	call(t, "POST", url+"/v1/envelopes", importerToken, batch(weighIn("a", 80, v), weighIn("b", 81, v), weighIn("c", 82, v)), http.StatusOK, nil)
	w0 := head(t, d)

	var a api.Admission
	call(t, "POST", url+"/v1/envelopes", importerToken, snapBatch("run", 1, 2, weighIn("a", 80, v)), http.StatusOK, &a)
	// Changed while incomplete, carrying a facet the ledger has never seen: refused
	// before admission, so W does not move and the facet is not in the record.
	call(t, "POST", url+"/v1/envelopes", importerToken, snapBatch("run", 1, 2, weighIn("a", 80, v), weighIn("new", 90, v)), http.StatusConflict, nil)
	if w := head(t, d); w != w0 {
		t.Fatalf("a changed part admitted something: W %d -> %d", w0, w)
	}
	var rep api.SnapshotReport
	call(t, "GET", url+"/v1/snapshots/run", importerToken, "", http.StatusOK, &rep)
	if rep.PartsAdmitted != 1 || rep.Carried != 1 {
		t.Fatalf("after a changed part: %+v", rep)
	}

	// Part 2 carries b; its exact retry in another order with a second envelope.
	call(t, "POST", url+"/v1/envelopes", importerToken, snapBatch("run", 2, 2, weighIn("b", 81, v), weighIn("a", 80, v)), http.StatusOK, &a)
	if !a.Snapshot.Complete {
		t.Fatalf("after both parts: %+v", a.Snapshot)
	}
	call(t, "GET", url+"/v1/snapshots/run", importerToken, "", http.StatusOK, &rep)
	if rep.Carried != 2 || *rep.Live != 3 || *rep.Absent != 1 || len(rep.AbsentIDs) != 1 {
		t.Fatalf("complete: %+v absent %v", rep, rep.AbsentIDs)
	}
	for _, r := range a.Results {
		if r.FacetID == rep.AbsentIDs[0] {
			t.Fatalf("a carried facet %s is reported absent", r.FacetID)
		}
	}
	before := rep

	// The audit's move: the completed part again, carrying c instead.
	call(t, "POST", url+"/v1/envelopes", importerToken, snapBatch("run", 2, 2, weighIn("c", 82, v)), http.StatusConflict, nil)
	// An exact retry of the completed part, envelopes in the other order.
	call(t, "POST", url+"/v1/envelopes", importerToken, snapBatch("run", 2, 2, weighIn("a", 80, v), weighIn("b", 81, v)), http.StatusOK, &a)
	if !a.Snapshot.Complete || a.Snapshot.PartsAdmitted != 2 || a.Changed != 0 {
		t.Errorf("exact retry after completion: %+v changed %d", a.Snapshot, a.Changed)
	}
	call(t, "GET", url+"/v1/snapshots/run", importerToken, "", http.StatusOK, &rep)
	if rep.Carried != before.Carried || *rep.Absent != *before.Absent || *rep.Live != *before.Live ||
		rep.CompletedW != before.CompletedW || rep.CompletedAt != before.CompletedAt {
		t.Errorf("a completed snapshot moved: %+v -> %+v", before, rep)
	}
	if w := head(t, d); w != w0 {
		t.Errorf("retries moved W: %d -> %d", w0, w)
	}
}

// Concurrent exact retries of one part are serialised by the write queue: every one is
// 200, and the part is recorded once.
func TestSnapshotConcurrentRetriesRecordOnePart(t *testing.T) {
	cfg := importerConfig(t)
	d, url, stop := serveImporters(t, cfg)
	defer func() { _ = stop() }()
	weight, _ := d.defs.current.Load().Lookup("health.weight")
	v := weight.Version
	body := snapBatch("run", 1, 1, weighIn("a", 80, v), weighIn("b", 81, v))

	const n = 8
	codes := make([]int, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, err := http.NewRequest("POST", url+"/v1/envelopes", strings.NewReader(body))
			if err != nil {
				codes[i] = -1
				return
			}
			req.Header.Set("Authorization", "Bearer "+importerToken)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				codes[i] = -1
				return
			}
			_ = resp.Body.Close()
			codes[i] = resp.StatusCode
		}()
	}
	wg.Wait()
	for i, c := range codes {
		if c != http.StatusOK {
			t.Errorf("retry %d: %d", i, c)
		}
	}
	var rep api.SnapshotReport
	call(t, "GET", url+"/v1/snapshots/run", importerToken, "", http.StatusOK, &rep)
	if !rep.Complete || rep.PartsAdmitted != 1 || rep.Carried != 2 || *rep.Absent != 0 {
		t.Errorf("after %d concurrent retries: %+v", n, rep)
	}
	if w := head(t, d); w != 2 {
		t.Errorf("W = %d, want 2: the two facets once", w)
	}
}

// The promoted audit probe, as written: a completed 1/1 part resent with another
// facet already in the ledger.
func TestSnapshotCompletedPartCannotGainFacets(t *testing.T) {
	d, url, stop := serveImporters(t, importerConfig(t))
	defer func() { _ = stop() }()
	weight, _ := d.defs.current.Load().Lookup("health.weight")
	call(t, "POST", url+"/v1/envelopes", importerToken, batch(weighIn("a", 80, weight.Version), weighIn("b", 81, weight.Version)), http.StatusOK, nil)
	call(t, "POST", url+"/v1/envelopes", importerToken, snapBatch("sealed", 1, 1, weighIn("a", 80, weight.Version)), http.StatusOK, nil)
	var before, after api.SnapshotReport
	call(t, "GET", url+"/v1/snapshots/sealed", importerToken, "", http.StatusOK, &before)
	req := httptest.NewRequest("POST", "/v1/envelopes", strings.NewReader(snapBatch("sealed", 1, 1, weighIn("b", 81, weight.Version))))
	req.Header.Set("Authorization", "Bearer "+importerToken)
	rec := httptest.NewRecorder()
	d.Handler().ServeHTTP(rec, req)
	call(t, "GET", url+"/v1/snapshots/sealed", importerToken, "", http.StatusOK, &after)
	if rec.Code != http.StatusConflict || before.Carried != after.Carried || *before.Absent != *after.Absent {
		t.Errorf("changed retry status=%d; carried %d->%d absent %d->%d completed_w %d->%d ledger W=%d",
			rec.Code, before.Carried, after.Carried, *before.Absent, *after.Absent, before.CompletedW, after.CompletedW, head(t, d))
	}
}
