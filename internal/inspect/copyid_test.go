package inspect_test

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// copyButton finds the copy button whose data-copy is id and returns its tag.
func copyButton(body, id string) (string, bool) {
	re := regexp.MustCompile(`<button[^>]*data-copy="` + regexp.QuoteMeta(id) + `"[^>]*>`)
	tag := re.FindString(body)
	return tag, tag != ""
}

// Every page that prints a facet id puts a real, labelled button beside it that carries
// the whole id, so a click has the full hash to copy and a keyboard can reach it.
func TestFacetID_PagesShowACopyButtonBesideTheID(t *testing.T) {
	t.Parallel()
	srv, bpID, hrID := seededServer(t)

	for _, tc := range []struct{ name, path, id string }{
		{"facet heading", "/facet?id=" + bpID, bpID},
		{"facet list", "/facets", hrID},
		{"events", "/events", bpID},
	} {
		code, body := get(t, srv.URL+tc.path)
		if code != http.StatusOK {
			t.Fatalf("%s: GET %s = %d", tc.name, tc.path, code)
		}
		tag, ok := copyButton(body, tc.id)
		if !ok {
			t.Errorf("%s: no copy button for %s", tc.name, tc.id)
			continue
		}
		for _, want := range []string{`type="button"`, `aria-label="copy facet id"`, `class="copyid"`} {
			if !strings.Contains(tag, want) {
				t.Errorf("%s: button %s lacks %s", tc.name, tag, want)
			}
		}
		// The button follows the id it copies, inside the same span.
		if !strings.Contains(body, tc.id+"</") || !strings.Contains(body, `<span class="facetid">`) {
			t.Errorf("%s: id and button are not drawn together", tc.name)
		}
	}

	// The shared script that does the copy rides every page's frame.
	_, body := get(t, srv.URL+"/facets")
	for _, want := range []string{"navigator.clipboard.writeText", `execCommand('copy')`} {
		if !strings.Contains(body, want) {
			t.Errorf("page lacks the copy script: missing %q", want)
		}
	}
}
