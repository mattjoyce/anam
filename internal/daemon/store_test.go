package daemon

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/api"
)

// Receipts, served values, saved selections and checkpoint values are rows of the
// operational store (card 215). A daemon that published them, minted over them and was
// stopped is opened again on the same two files and nothing else: no folder of
// artifacts stands beside them, every receipt is listed and verifies, a saved selection
// resolves, and a read at an older watermark is the one it was before the mints
// collected the checkpoints it started from.
func TestStore_ANewDaemonOnTheTwoFilesServesWhatTheLastPublished(t *testing.T) {
	cfg := importerConfig(t)
	d, base, stop := serveImporters(t, cfg)
	call(t, "POST", base+"/v1/projections", testToken, weightsV1, http.StatusCreated, nil)
	weight, _ := d.defs.current.Load().Lookup("health.weight")
	admit := func(from, n int) {
		t.Helper()
		var envs []string
		for i := from; i < from+n; i++ {
			envs = append(envs, weighIn(fmt.Sprintf("w-%02d", i), 80+float64(i)/10, weight.Version))
		}
		call(t, "POST", base+"/v1/envelopes", importerToken, batch(envs...), http.StatusOK, nil)
	}
	publish := func() api.Published {
		t.Helper()
		var pub api.Published
		call(t, "POST", base+"/v1/receipts?zone=UTC&projection=weights", testToken, "", http.StatusCreated, &pub)
		return pub
	}
	mint := func() api.Mint {
		t.Helper()
		var m api.Mint
		call(t, "POST", base+"/v1/checkpoints", testToken, "", http.StatusOK, &m)
		return m
	}

	// Three rounds of publishing and minting. A receipt keeps the checkpoints its
	// watermark reads from; the facet each round admits after its receipt is a mint no
	// receipt reads from, and the next mint collects it. The second round's receipt is
	// the one a read is repeated against.
	var published []api.Published
	for round := 0; round < 3; round++ {
		admit(round*3, 3)
		mint()
		published = append(published, publish())
		admit(100+round, 1)
		mint()
	}
	// One watermark a receipt reads from, and one just after the first receipt's, which
	// no receipt reads from and whose checkpoints the mints have collected.
	atSecond := published[1].Receipt.Watermark
	atUnpinned := published[0].Receipt.Watermark + 1
	timelineAt := func(base string, w int64) api.Timeline {
		t.Helper()
		var tl api.Timeline
		call(t, "GET", fmt.Sprintf("%s/v1/timeline?zone=UTC&w=%d", base, w), testToken, "", http.StatusOK, &tl)
		return tl
	}
	before, beforeUnpinned := timelineAt(base, atSecond), timelineAt(base, atUnpinned)
	if before.Live != 7 || beforeUnpinned.Live != 4 {
		t.Fatalf("the watermarks read %d and %d live facets, want 7 and 4", before.Live, beforeUnpinned.Live)
	}

	var saved api.Selection
	call(t, "POST", base+"/v1/selections", testToken,
		fmt.Sprintf(`{"program": %q}`, weightProgram), http.StatusCreated, &saved)

	if err := stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	entries, err := os.ReadDir(filepath.Dir(cfg.DBPath))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), "artifact") {
			t.Fatalf("the data directory holds %s: nothing is published as a file now", e.Name())
		}
	}

	_, base2, _ := serveImporters(t, cfg)
	var list api.Receipts
	call(t, "GET", base2+"/v1/receipts", testToken, "", http.StatusOK, &list)
	var want, got []string
	for _, p := range published {
		want = append(want, p.ReceiptHash)
	}
	for _, l := range list.Receipts {
		got = append(got, l.Hash)
	}
	slices.Sort(want)
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Fatalf("the new daemon lists receipts %v, the old one published %v", got, want)
	}
	for _, p := range published {
		var v api.ReceiptVerified
		call(t, "GET", base2+"/v1/receipts/"+p.ReceiptHash+"/verify", testToken, "", http.StatusOK, &v)
		if !v.Result.OK() {
			t.Errorf("receipt %s no longer verifies:\n%s", p.ReceiptHash, v.Result.Summary())
		}
		var shown api.ReceiptShown
		call(t, "GET", base2+"/v1/receipts/"+p.ReceiptHash, testToken, "", http.StatusOK, &shown)
		if shown.Receipt == nil || len(shown.Entries) != len(p.Entries) {
			t.Errorf("receipt %s shows %d entries, was published with %d", p.ReceiptHash, len(shown.Entries), len(p.Entries))
		}
	}

	var opened api.Selection
	call(t, "GET", base2+"/v1/selections/sel:"+saved.Code, testToken, "", http.StatusOK, &opened)
	if opened != saved {
		t.Errorf("the saved selection opens as %+v, was saved as %+v", opened, saved)
	}

	for _, tc := range []struct {
		w            int64
		before, then api.Timeline
	}{{atSecond, before, timelineAt(base2, atSecond)}, {atUnpinned, beforeUnpinned, timelineAt(base2, atUnpinned)}} {
		if tc.then.Canonical != tc.before.Canonical || tc.then.Live != tc.before.Live || tc.then.W != tc.before.W {
			t.Errorf("a read at W=%d is %s (%d live), was %s (%d live) before the daemon restarted",
				tc.w, tc.then.Canonical, tc.then.Live, tc.before.Canonical, tc.before.Live)
		}
	}

	// Seven mints cut seven versions of the one bucket the weigh-ins fall in. What is
	// left is the head and the versions the three receipts read from.
	var cps api.Checkpoints
	call(t, "GET", base2+"/v1/checkpoints", testToken, "", http.StatusOK, &cps)
	if n := len(cps.Checkpoints); n > 4 {
		t.Errorf("the log holds %d checkpoints, want the head and at most the three a receipt reads from", n)
	}
}
