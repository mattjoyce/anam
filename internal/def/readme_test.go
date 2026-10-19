package def_test

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// The README's table of bundled Definitions is hand-written, and it had drifted
// once (card 73's review found med.started a version behind). This holds it to the
// manifests: every bundled type is a row at its current version, every row is a
// bundled type, and the count the prose states is the count on disk.
func TestREADMETableMatchesTheBundledManifests(t *testing.T) {
	t.Parallel()
	readme, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	rows, stated := readmeBundled(t, string(readme))

	bundled := map[string]int{}
	files, err := filepath.Glob(filepath.Join("manifests", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var m struct {
			Meta struct {
				Type    string `json:"facet_type"`
				Version int    `json:"definition_version"`
			} `json:"meta"`
		}
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		bundled[m.Meta.Type] = m.Meta.Version
	}

	for typ, v := range bundled {
		got, ok := rows[typ]
		switch {
		case !ok:
			t.Errorf("README: bundled type %s has no row", typ)
		case got != v:
			t.Errorf("README: %s is listed at v%d, the manifest is v%d", typ, got, v)
		}
	}
	for typ := range rows {
		if _, ok := bundled[typ]; !ok {
			t.Errorf("README: row %s names no bundled manifest", typ)
		}
	}
	if stated != len(bundled) {
		t.Errorf("README says Anam bundles %d, the manifests are %d", stated, len(bundled))
	}
}

var (
	bundledCount = regexp.MustCompile(`Anam bundles the (\d+) below`)
	tableRow     = regexp.MustCompile("^\\| `([a-z0-9.-]+)` \\| (\\d+) \\|")
)

// readmeBundled is the "Bundled with Anam" table as type to version, and the count
// the prose states.
func readmeBundled(t *testing.T, readme string) (map[string]int, int) {
	t.Helper()
	m := bundledCount.FindStringSubmatch(readme)
	if m == nil {
		t.Fatal("README: no sentence 'Anam bundles the N below'")
	}
	stated, _ := strconv.Atoi(m[1])

	rows := map[string]int{}
	in := false
	sc := bufio.NewScanner(strings.NewReader(readme))
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "### Bundled with Anam"):
			in = true
		case in && strings.HasPrefix(line, "### "):
			in = false
		case in:
			if r := tableRow.FindStringSubmatch(line); r != nil {
				v, _ := strconv.Atoi(r[2])
				rows[r[1]] = v
			}
		}
	}
	if len(rows) == 0 {
		t.Fatal("README: no rows under 'Bundled with Anam'")
	}
	return rows, stated
}

// A manifest's name says its type, so a misnamed file is noticed here too.
func TestBundledManifestFilesAreNamedForTheirType(t *testing.T) {
	t.Parallel()
	files, _ := filepath.Glob(filepath.Join("manifests", "*.json"))
	for _, f := range files {
		b, _ := os.ReadFile(f)
		var m struct {
			Meta struct {
				Type string `json:"facet_type"`
			} `json:"meta"`
		}
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		if want := fmt.Sprintf("%s.json", m.Meta.Type); filepath.Base(f) != want {
			t.Errorf("%s holds %s", f, m.Meta.Type)
		}
	}
}
