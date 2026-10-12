package frame_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/frame"
)

func entry(id, factTime, payload string) fold.Entry {
	return fold.Entry{
		FacetID: id, Type: "t.x", FactTime: factTime, Source: "src", SourceRef: "src:" + id,
		Status: "admitted", Classification: "Normal", ContentHash: "sha256:" + id,
		Payload: json.RawMessage(payload),
	}
}

// A frame is the envelope's columns then the pointers, one row per entry in the order
// given; a cell is the value's own JSON with a number's lexeme kept, and a pointer a
// row lacks is null.
func TestOf_CutsEntriesToTheirFields(t *testing.T) {
	t.Parallel()
	entries := []fold.Entry{
		entry("a", "2026-01-01T00:00:00.000000000Z", `{"n": {"v": 1.50, "u": "m"}, "s": "x", "arr": [{"m": 3}], "big": {"deep": true}}`),
		entry("b", "2026-01-02T00:00:00.000000000Z", `{"n": {"v": 2}, "s": "y"}`),
	}
	f, err := frame.Of(entries, []string{"/n/v", "/s", "/arr/0/m", "/n"})
	if err != nil {
		t.Fatalf("Of: %v", err)
	}
	wantCols := append(append([]string{}, frame.Envelope...), "/n/v", "/s", "/arr/0/m", "/n")
	if strings.Join(f.Columns, " ") != strings.Join(wantCols, " ") {
		t.Errorf("columns = %v, want %v", f.Columns, wantCols)
	}
	if f.Len() != 2 {
		t.Fatalf("rows = %d, want 2", f.Len())
	}
	cells := func(row []json.RawMessage) string {
		parts := make([]string, 0, len(row))
		for _, c := range row {
			parts = append(parts, string(c))
		}
		return strings.Join(parts, " ")
	}
	if got, want := cells(f.Rows[0]), `"a" "t.x" "2026-01-01T00:00:00.000000000Z" "src" "admitted" "Normal" "sha256:a" 1.50 "x" 3 {"u":"m","v":1.50}`; got != want {
		t.Errorf("row a = %s\nwant    %s", got, want)
	}
	if got, want := cells(f.Rows[1]), `"b" "t.x" "2026-01-02T00:00:00.000000000Z" "src" "admitted" "Normal" "sha256:b" 2 "y" null {"v":2}`; got != want {
		t.Errorf("row b = %s\nwant    %s", got, want)
	}
	if strings.Contains(string(f.Bytes()), "deep") {
		t.Errorf("a field not named reached the frame: %s", f.Bytes())
	}
}

// The frame's bytes are one serialisation, and reading them back gives the same
// frame and the same hash: what a receipt pins is what show renders.
func TestBytes_RoundTripAndHash(t *testing.T) {
	t.Parallel()
	entries := []fold.Entry{entry("a", "2026-01-01T00:00:00.000000000Z", `{"s": "x"}`)}
	f, err := frame.Of(entries, []string{"/s"})
	if err != nil {
		t.Fatalf("Of: %v", err)
	}
	if !strings.HasPrefix(f.Hash(), "sha256:") {
		t.Errorf("hash = %q", f.Hash())
	}
	again, err := frame.Parse(f.Bytes())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if again.Hash() != f.Hash() || string(again.Bytes()) != string(f.Bytes()) {
		t.Errorf("round trip changed the frame:\n%s\n%s", f.Bytes(), again.Bytes())
	}
	if _, err := frame.Parse([]byte(`[{"facet_id": "a"}]`)); err == nil {
		t.Error("a list of entries parsed as a frame")
	}
}

func TestOf_RefusesNoFieldsAndABadPointer(t *testing.T) {
	t.Parallel()
	entries := []fold.Entry{entry("a", "2026-01-01T00:00:00.000000000Z", `{"s": "x"}`)}
	if _, err := frame.Of(entries, nil); err == nil {
		t.Error("no fields should be refused")
	}
	if _, err := frame.Of(entries, []string{"s"}); err == nil || !strings.Contains(err.Error(), "must start with '/'") {
		t.Errorf("a pointer without a slash: %v", err)
	}
	// No entries is an empty frame, not an error: the columns still say what was asked.
	f, err := frame.Of(nil, []string{"/s"})
	if err != nil || f.Len() != 0 || len(f.Columns) != len(frame.Envelope)+1 {
		t.Errorf("empty frame: %v, %d rows, %d columns", err, f.Len(), len(f.Columns))
	}
}
