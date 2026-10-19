package def_test

import (
	"errors"
	"testing"

	"github.com/mattjoyce/anam/internal/def"
)

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Pointers is the schema's declared fields as HasPointer accepts them: every
// property, nested ones by their path, an array by its first item. A verbatim type
// has none, and a history body answers the same from its schema alone.
func TestPointers_ListWhatHasPointerAccepts(t *testing.T) {
	t.Parallel()
	d := mustParse(t, nestedManifest)
	want := []string{"/a~1b", "/distance", "/distance/unit", "/distance/value", "/measured_at", "/splits", "/splits/0", "/splits/0/m"}
	got := d.Pointers()
	if !sameStrings(got, want) {
		t.Errorf("Pointers = %v, want %v", got, want)
	}
	for _, p := range got {
		if err := d.HasPointer(p); err != nil {
			t.Errorf("HasPointer(%s) = %v on a pointer Pointers listed", p, err)
		}
	}
	fromBody, err := def.SchemaPointers([]byte(nestedManifest))
	if err != nil {
		t.Fatalf("SchemaPointers: %v", err)
	}
	if !sameStrings(fromBody, want) {
		t.Errorf("SchemaPointers = %v, want %v", fromBody, want)
	}
	if none, err := def.SchemaPointers([]byte(`{"meta": {}}`)); err != nil || none != nil {
		t.Errorf("SchemaPointers over no schema = %v, %v; want none", none, err)
	}
	if _, err := def.SchemaPointers([]byte(`{`)); !errors.Is(err, def.ErrBadManifest) {
		t.Errorf("SchemaPointers over bad JSON = %v, want ErrBadManifest", err)
	}
}
