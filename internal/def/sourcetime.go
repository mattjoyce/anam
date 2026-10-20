package def

import (
	"embed"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"
)

// SourceTimeField is the payload key that says how a source's clock was read (card
// 197): the fact time as the source supplied it, the zone it is read in, and who
// decided that zone. fact_time stays the one UTC instant the gate derives; this sits
// beside it so a reader can tell a supplied offset from an importer's guess.
//
// A Definition carries it by declaring a top-level property of this name whose
// schema is a copy of one of the shapes under sourcetime/. A schema cannot $ref out
// of its manifest, and a stored body must keep its meaning whatever the binary
// carries, so the shape is copied rather than referenced, and Parse checks the copy.
// The shapes are append-only: a later one is a new file beside v1, never an edit of
// it, so a body stored with v1 still parses. A type that needs the field lists it in
// required, as any other field.
const SourceTimeField = "source_time"

//go:embed sourcetime/*.json
var sourceTimeFS embed.FS

// sourceTimeShapes is every shape, prose removed, read once from the embedded files.
var sourceTimeShapes = sync.OnceValues(func() ([]any, error) {
	entries, err := sourceTimeFS.ReadDir("sourcetime")
	if err != nil {
		return nil, fmt.Errorf("def: read source_time shapes: %w", err)
	}
	shapes := make([]any, 0, len(entries))
	for _, e := range entries {
		raw, err := sourceTimeFS.ReadFile("sourcetime/" + e.Name())
		if err != nil {
			return nil, fmt.Errorf("def: read source_time shape %s: %w", e.Name(), err)
		}
		var shape any
		if err := json.Unmarshal(raw, &shape); err != nil {
			return nil, fmt.Errorf("def: source_time shape %s: %w", e.Name(), err)
		}
		shapes = append(shapes, withoutProse(shape, false))
	}
	return shapes, nil
})

// checkSourceTimeShape refuses a schema whose source_time property is none of the
// shapes. Descriptions and examples may differ, so a type can say what its usual
// basis is; every rule must be the shape's.
func checkSourceTimeShape(tree map[string]any) error {
	props, _ := tree["properties"].(map[string]any)
	got, ok := props[SourceTimeField]
	if !ok {
		return nil
	}
	shapes, err := sourceTimeShapes()
	if err != nil {
		return err
	}
	bare := withoutProse(got, false)
	for _, s := range shapes {
		if reflect.DeepEqual(bare, s) {
			return nil
		}
	}
	return fmt.Errorf("%w: schema: %q is not a copy of a source_time shape (internal/def/sourcetime)",
		ErrBadManifest, SourceTimeField)
}

// withoutProse is a schema node with every description and examples keyword
// removed. Under "properties" the keys are field names, not keywords, so they stay.
func withoutProse(node any, fields bool) any {
	switch n := node.(type) {
	case map[string]any:
		out := make(map[string]any, len(n))
		for k, v := range n {
			if !fields && (k == "description" || k == "examples") {
				continue
			}
			out[k] = withoutProse(v, !fields && k == "properties")
		}
		return out
	case []any:
		out := make([]any, len(n))
		for i, v := range n {
			out[i] = withoutProse(v, false)
		}
		return out
	}
	return node
}

// carriesSourceTime reports whether the Definition's schema declares source_time.
func (d Definition) carriesSourceTime() bool {
	props, _ := d.tree["properties"].(map[string]any)
	_, ok := props[SourceTimeField]
	return ok
}

// checkSourceTime refuses a payload whose source_time names a zone that is none: the
// schema's pattern admits the shape of a name, and only loading it says whether the
// zone exists. An absent source_time is the schema's to require or not.
func (d Definition) checkSourceTime(doc any) error {
	if !d.carriesSourceTime() {
		return nil
	}
	top, _ := doc.(map[string]any)
	st, _ := top[SourceTimeField].(map[string]any)
	zone, ok := st["zone"].(string)
	if !ok {
		return nil
	}
	if err := validZone(zone); err != nil {
		return fmt.Errorf("%w: %s zone: %v", ErrNotCompliant, SourceTimeField, err)
	}
	return nil
}

// maxOffset is the widest offset from UTC any zone keeps (+14:00). Not configurable:
// it is the world's.
const maxOffset = 14 * time.Hour

// validZone accepts an IANA zone the binary's tzdata knows, or a fixed offset
// +HH:MM or -HH:MM within maxOffset. "Local" is refused: it is whatever host reads
// it, which is the guess source_time exists to rule out.
func validZone(s string) error {
	switch {
	case s == "" || s == "Local":
		return fmt.Errorf("%q names no zone", s)
	case s[0] == '+' || s[0] == '-':
		t, err := time.Parse("-07:00", s)
		if err != nil {
			return fmt.Errorf("%q is not an offset +HH:MM or -HH:MM", s)
		}
		if _, off := t.Zone(); time.Duration(off)*time.Second > maxOffset || time.Duration(-off)*time.Second > maxOffset {
			return fmt.Errorf("%q is wider than any zone's offset", s)
		}
		return nil
	}
	if _, err := time.LoadLocation(s); err != nil {
		return fmt.Errorf("%q is not an IANA zone: %v", s, err)
	}
	return nil
}

// zoneless reports whether a Go time layout writes no offset and no zone name, so
// its values are wall clocks read in the Definition's zone.
func zoneless(layout string) bool {
	return !strings.Contains(layout, "-07") && !strings.Contains(layout, "Z07") && !strings.Contains(layout, "MST")
}

// wallLayout writes every field of a wall clock, for comparing two of them.
const wallLayout = "2006-01-02T15:04:05.999999999"

// oneInstant refuses a wall clock s that names no instant in loc, because the clocks
// skip it, or two, because they repeat it (card 197). t is s as ParseInLocation read
// it, which moves a skipped wall clock on by the gap and picks one side of a repeated
// one without a word. A skipped one does not survive the trip back to the wall; a
// repeated one has a second instant at the zone's other offset nearby.
func oneInstant(layout, s string, t time.Time, loc *time.Location) error {
	wall, err := time.ParseInLocation(layout, s, time.UTC)
	if err != nil {
		return err
	}
	want := wall.Format(wallLayout)
	if t.In(loc).Format(wallLayout) != want {
		return fmt.Errorf("does not exist in %s: the clocks skip it (daylight saving)", loc)
	}
	// A transition that makes t repeat lies within its own size of t, and no zone
	// moves its clocks by more than a few hours, so probing three hours either side
	// finds the other offset if there is one.
	_, off := t.Zone()
	for _, probe := range []time.Time{t.Add(-3 * time.Hour), t.Add(3 * time.Hour)} {
		_, other := probe.Zone()
		if other == off {
			continue
		}
		twin := t.Add(time.Duration(off-other) * time.Second)
		if twin.In(loc).Format(wallLayout) == want {
			return fmt.Errorf("happens twice in %s: the clocks repeat it (daylight saving)", loc)
		}
	}
	return nil
}
