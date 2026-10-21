package daemon

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/api"
)

// Saved selections (card 206) through a real daemon: the API saves and opens a
// program for any role, and the timeline and the link form open its code.

// weightProgram selects every weigh-in, written by hand in a layout of its own.
const weightProgram = "pick/1.\npick(E) :- entry(E, T, _, _, _, _, _), T = \"health.weight\".\n"

func saveSelection(t *testing.T, base, token, program string, want int) api.Selection {
	t.Helper()
	body, err := json.Marshal(api.SelectionRequest{Program: program})
	if err != nil {
		t.Fatal(err)
	}
	var s api.Selection
	call(t, "POST", base+"/v1/selections", token, string(body), want, &s)
	return s
}

func TestSelections_APIRoundTrip(t *testing.T) {
	d, base := workbenchServing(t, "record.edge")
	ids := weighIns(t, d, base, 4)

	saved := saveSelection(t, base, importerToken, weightProgram, http.StatusCreated)
	if len(saved.Code) != 8 || !strings.HasPrefix(saved.Hash, "sha256:"+saved.Code) || saved.URL != "/?sel="+saved.Code {
		t.Fatalf("saved = %+v, want eight hex of the hash and the timeline's address", saved)
	}
	if !strings.HasPrefix(saved.Program, "% An Anam selection") {
		t.Errorf("a chip program is not saved in the chips' layout:\n%s", saved.Program)
	}
	// Another role, another layout of the same selection: one code.
	again := saveSelection(t, base, testToken, strings.ReplaceAll(weightProgram, ", T =", ",\n  T ="), http.StatusCreated)
	if again != saved {
		t.Fatalf("the same program saved twice: %+v and %+v", saved, again)
	}
	other := saveSelection(t, base, testToken, `pick(E) :- entry(E, "health.blood-pressure", _, _, _, _, _).`, http.StatusCreated)
	if other.Code == saved.Code {
		t.Fatalf("two programs share the code %s", saved.Code)
	}

	var opened api.Selection
	call(t, "GET", base+"/v1/selections/sel:"+saved.Code, importerToken, "", http.StatusOK, &opened)
	if opened != saved {
		t.Fatalf("GET = %+v, saved %+v", opened, saved)
	}
	call(t, "GET", base+"/v1/selections/00000000", testToken, "", http.StatusNotFound, nil)
	call(t, "GET", base+"/v1/selections/xyz", testToken, "", http.StatusBadRequest, nil)
	call(t, "POST", base+"/v1/selections", testToken, `{"program": ""}`, http.StatusBadRequest, nil)
	call(t, "POST", base+"/v1/selections", testToken, `{"program": "pick(E) :- "}`, http.StatusBadRequest, nil)
	call(t, "POST", base+"/v1/selections", testToken, `{"program": "pick/1.", "relation": "pick"}`, http.StatusBadRequest, nil)

	// The timeline draws the four weigh-ins the code selects.
	status, body := page(t, "GET", base+saved.URL, nil, nil)
	if status != http.StatusOK || !strings.Contains(body, "selection sel:"+saved.Code+", 4 facets") {
		t.Fatalf("GET %s = %d, want the selection's header:\n%s", saved.URL, status, body)
	}
	if status, body := page(t, "GET", base+"/?sel=00000000", nil, nil); status != http.StatusNotFound ||
		!strings.Contains(body, "No saved selection opens as") {
		t.Fatalf("an unknown code on the timeline = %d:\n%s", status, body)
	}

	// The link form fills its basket from the code, as from the program.
	status, form := page(t, "GET", base+"/link?sel=sel:"+saved.Code, nil, nil)
	if status != http.StatusOK {
		t.Fatalf("GET /link?sel= = %d:\n%s", status, form)
	}
	for _, id := range ids {
		if !strings.Contains(form, `name="keep" value="`+id+`" checked`) {
			t.Errorf("the form from the code lacks %s", id)
		}
	}
	if status, _ := page(t, "GET", base+"/link?sel=00000000", nil, nil); status != http.StatusNotFound {
		t.Errorf("GET /link with an unknown code = %d, want 404", status)
	}
}
