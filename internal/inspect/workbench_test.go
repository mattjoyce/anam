package inspect_test

import (
	"net/http"
	"strings"
	"testing"
)

// A standalone inspector, given no Workbench, only reads: the entry routes do not
// exist, the link form among them, and the Definitions page says why no type can be
// entered.
func TestWorkbench_AbsentMeansReadOnly(t *testing.T) {
	t.Parallel()
	srv, bp, _ := seededServer(t)
	if code, _ := get(t, srv.URL+"/entry?type=health.medication.dose"); code != http.StatusNotFound {
		t.Fatalf("GET /entry = %d, want 404", code)
	}
	_, body := get(t, srv.URL+"/definitions")
	if strings.Contains(body, `href="/entry?`) || !strings.Contains(body, "has no Workbench") {
		t.Fatalf("the Definitions page offers entry without a Workbench")
	}
	_, facetPage := get(t, srv.URL+"/facet?id="+bp)
	if strings.Contains(facetPage, `action="/retract"`) {
		t.Fatalf("a facet page offers Retract without a Workbench")
	}
	if strings.Contains(facetPage, "link this…") {
		t.Fatalf("a facet page offers link this without a Workbench")
	}
	if code, _ := get(t, srv.URL+"/link?tick="+bp); code != http.StatusNotFound {
		t.Fatalf("GET /link = %d, want 404", code)
	}
}
