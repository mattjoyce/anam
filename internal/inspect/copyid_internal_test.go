package inspect

import (
	"bytes"
	"strings"
	"testing"
)

// The fragment is handed a hash, but it does not trust that: an id with markup in it is
// escaped in the text, the link and the attribute alike.
func TestFacetIDFragment_EscapesTheID(t *testing.T) {
	t.Parallel()
	const hostile = `"><script>alert(1)</script>&x=1`
	for _, name := range []string{"facetid", "facetidplain", "copyid"} {
		var buf bytes.Buffer
		if err := pagesFor(Build{}, false).ExecuteTemplate(&buf, name, hostile); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		out := buf.String()
		if strings.Contains(out, "<script>") || strings.Contains(out, `data-copy=""><`) {
			t.Errorf("%s: id was not escaped: %s", name, out)
		}
		if !strings.Contains(out, `data-copy="&#34;&gt;&lt;script&gt;alert(1)&lt;/script&gt;&amp;x=1"`) {
			t.Errorf("%s: data-copy does not carry the escaped id: %s", name, out)
		}
	}
}

// A save or a retraction names its facet through the fragment, so the receipt's id has
// its button too.
func TestResultPage_FacetIDHasACopyButton(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	data := resultData{Heading: "retracted", Kind: "tombstoned", Facet: "abc-123", Detail: " is tombstoned."}
	if err := pagesFor(Build{}, false).ExecuteTemplate(&buf, "result.html", data); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `data-copy="abc-123"`) {
		t.Errorf("result page has no copy button for its facet:\n%s", buf.String())
	}
}
