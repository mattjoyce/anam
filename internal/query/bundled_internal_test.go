package query

import (
	"slices"
	"testing"
)

// The hashes of every text a receipt may name, pinned. A receipt names a rule set by
// the hash of its text, so a byte changed in a held file would orphan its receipts;
// the test fails before that ships. The v1 texts are the three that were served under
// that one version number (see history).
const (
	hashV1First      = "sha256:bd62040288c0e5b4e6de576c13618b065714736ed6c628106b73b0769c32727d"
	hashV1RecordNote = "sha256:e29634877441262c765923739d4d2cb49ae7e15d93b537dd972ebc68bb7c026d"
	hashV1Last       = "sha256:f8a1a46bce8701e37c18e92340e5ce7ab082d3f791ec98c0dbacbc61802a2220"
)

func TestHistory_EveryFileIsHeldAndEveryHashIsPinned(t *testing.T) {
	t.Parallel()
	files, err := historyFiles()
	if err != nil {
		t.Fatalf("history dir: %v", err)
	}
	var listed []string
	for _, h := range history {
		listed = append(listed, h.File)
		if h.Note == "" {
			t.Errorf("%s says nothing about which text it is", h.File)
		}
	}
	slices.Sort(listed)
	if !slices.Equal(files, listed) {
		t.Errorf("rules/history holds %v, the table lists %v: a text on disk that the table omits cannot be found by its hash", files, listed)
	}

	want := []string{hashV1First, hashV1RecordNote, hashV1Last}
	var got []string
	for _, h := range history {
		text, err := historyFS.ReadFile("rules/history/" + h.File)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, hashOf(string(text)))
	}
	if !slices.Equal(got, want) {
		t.Errorf("history hashes = %v, want the pinned %v", got, want)
	}
}

// The bundled text is not a history text, and no two held texts share a hash: one
// hash names one text.
func TestHistory_HashesAreDistinct(t *testing.T) {
	t.Parallel()
	seen := map[string]string{hashOf(bundledText): "rules/anam.dl"}
	for _, h := range history {
		text, err := historyFS.ReadFile("rules/history/" + h.File)
		if err != nil {
			t.Fatal(err)
		}
		hash := hashOf(string(text))
		if prev, dup := seen[hash]; dup {
			t.Errorf("%s has the hash of %s", h.File, prev)
		}
		seen[hash] = h.File
	}
}

func historyFiles() ([]string, error) {
	entries, err := historyFS.ReadDir("rules/history")
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	slices.Sort(out)
	return out, nil
}
