package bench

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/mattjoyce/anam/internal/catalog"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
)

// ErrType is a type the generator cannot write: absent from the catalog, or with a
// schema a time and two numbers do not satisfy.
var ErrType = errors.New("bench: cannot generate this type")

// Pair is a type the generator writes, read from its Definition: a fact time at a
// top-level field, which is also the whole identity key, and exactly two required
// numeric fields beside it. A payload is those three fields and nothing more, so
// the type's schema must require nothing else and constrain none of the three
// beyond its type. The layout and zone are the
// Definition's own (meta.time_semantics.fact_time).
type Pair struct {
	def  def.Definition
	loc  *time.Location
	at   string // the fact time's field
	a, b string // the two numeric fields, in the order the schema requires them
}

// PairOf reads typ's Definition in force in cat and says how a synthetic payload of
// it is written, or refuses it with ErrType naming why.
func PairOf(cat *catalog.Catalog, typ facet.Type) (Pair, error) {
	d, ok := cat.Definitions().Lookup(typ)
	if !ok {
		return Pair{}, fmt.Errorf("%w: the catalog (definitions %s) holds no Definition for %q", ErrType, cat.DefinitionsSource(), typ)
	}
	refuse := func(format string, args ...any) (Pair, error) {
		return Pair{}, fmt.Errorf("%w: %s v%d: %s", ErrType, typ, d.Version, fmt.Sprintf(format, args...))
	}
	at, ok := strings.CutPrefix(d.FactTime.Pointer, "/")
	if !ok || at == "" || strings.Contains(at, "/") {
		return refuse("its fact time %s is not a top-level field", d.FactTime.Pointer)
	}
	if !slices.Equal(d.IdentityKey, []string{d.FactTime.Pointer}) {
		return refuse("its identity key %v is not its fact time alone, so a correction would not supersede", d.IdentityKey)
	}
	if d.IdentityTemplate != "" {
		return refuse("it declares an identity template")
	}
	if d.Series != nil {
		return refuse("it belongs to the series %s", d.Series.Name)
	}
	loc, err := zoneOf(d.FactTime.Zone)
	if err != nil {
		return refuse("its fact-time zone: %v", err)
	}
	schema, err := schemaOf(cat, d)
	if err != nil {
		return refuse("%v", err)
	}
	if t := def.TypesOf(schema.Properties[at]); len(t) > 0 && !slices.Contains(t, "string") {
		return refuse("its fact time %s is not a string", d.FactTime.Pointer)
	}
	if k := constraint(schema.Properties[at], true); k != "" {
		return refuse("its fact time %s declares %s, which the generator does not honour", d.FactTime.Pointer, k)
	}
	var numbers []string
	for _, name := range schema.Required {
		if name == at {
			continue
		}
		t := def.TypesOf(schema.Properties[name])
		if !slices.Contains(t, "number") && !slices.Contains(t, "integer") {
			return refuse("it requires /%s, which is not a number", name)
		}
		if k := constraint(schema.Properties[name], false); k != "" {
			return refuse("its /%s declares %s, which the generator does not honour", name, k)
		}
		numbers = append(numbers, name)
	}
	if len(numbers) != 2 {
		return refuse("it requires %d numeric fields beside the time, not two", len(numbers))
	}
	return Pair{def: d, loc: loc, at: at, a: numbers[0], b: numbers[1]}, nil
}

// annotations are the keywords of a field's schema that constrain no value.
var annotations = map[string]bool{
	"type": true, "title": true, "description": true, "examples": true, "$comment": true,
	"default": true, "deprecated": true, "readOnly": true, "writeOnly": true,
}

// constraint is the first keyword, in name order, of a field's schema that could
// refuse a value the generator writes, or "". The generator writes numbers from 100
// up and wall clocks in the Definition's layout, and honours no bound, step, list or
// pattern, so any keyword but an annotation refuses the type: a correction that
// broke a maximum would fail halfway through a build and leave a ledger a rerun
// refuses. The time field may say minLength of 1, which every clock meets.
func constraint(field map[string]any, isTime bool) string {
	keys := make([]string, 0, len(field))
	for k := range field {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		if annotations[k] {
			continue
		}
		if isTime && k == "minLength" {
			if n, ok := field[k].(float64); ok && n <= 1 {
				continue
			}
		}
		return k
	}
	return ""
}

// zoneOf is the location a Definition's fact-time zone names: an IANA zone, or a
// fixed offset +HH:MM, which time.Parse reads as a zone of that offset.
func zoneOf(name string) (*time.Location, error) {
	if loc, err := time.LoadLocation(name); err == nil {
		return loc, nil
	}
	t, err := time.Parse("-07:00", name)
	if err != nil {
		return nil, fmt.Errorf("%q is neither an IANA zone nor an offset", name)
	}
	return t.Location(), nil
}

// objectSchema is the part of a manifest's schema the generator reads.
type objectSchema struct {
	Required   []string                  `json:"required"`
	Properties map[string]map[string]any `json:"properties"`
}

// schemaOf decodes the schema of the manifest d was parsed from.
func schemaOf(cat *catalog.Catalog, d def.Definition) (objectSchema, error) {
	for _, s := range cat.Manifests() {
		if s.Type != d.Type || s.Version != d.Version {
			continue
		}
		var m struct {
			Schema objectSchema `json:"schema"`
		}
		if err := json.Unmarshal(s.Body, &m); err != nil {
			return objectSchema{}, fmt.Errorf("its manifest does not decode: %w", err)
		}
		return m.Schema, nil
	}
	return objectSchema{}, errors.New("it has no manifest in the catalog (a registered type?)")
}

// reading is one synthetic reading, before it is an envelope.
type reading struct {
	at    time.Time
	value int
}

// readings spreads PerMonth readings evenly across each month in the type's zone,
// each at a wall clock the Definition reads as one instant, no two the same.
func (pr Pair) readings(sh Shape) ([]reading, error) {
	out := make([]reading, 0, sh.Months*sh.PerMonth)
	var prev time.Time
	for m := 0; m < sh.Months; m++ {
		first := time.Date(sh.Start.Year(), sh.Start.Month()+time.Month(m), 1, 0, 0, 0, 0, pr.loc)
		step := first.AddDate(0, 1, 0).Sub(first) / time.Duration(sh.PerMonth)
		for i := 0; i < sh.PerMonth; i++ {
			at, err := pr.place(first.Add(time.Duration(i)*step).Truncate(time.Second), prev)
			if err != nil {
				return nil, err
			}
			out = append(out, reading{at: at, value: 100 + (m*7+i)%40})
			prev = at
		}
	}
	return out, pr.distinct(out)
}

// place is the first instant from t, and after prev, whose wall clock the Definition
// reads as one instant. A clock the zone repeats when it leaves daylight saving names
// two, and the gate refuses it (card 197), so a reading that falls in the repeated
// hour moves a minute at a time past it, a second after the reading before, which
// keeps the readings in order and their clocks distinct.
func (pr Pair) place(t, prev time.Time) (time.Time, error) {
	if !t.After(prev) {
		t = prev.Add(time.Second)
	}
	var err error
	for range 24 * 60 {
		if _, err = pr.def.FactTimeOf(map[string]any{pr.at: pr.clock(t)}); err == nil {
			return t, nil
		}
		t = t.Add(time.Minute)
	}
	return time.Time{}, fmt.Errorf("%w: %s: no wall clock in the day before %s reads as one instant: %v", ErrType, pr.def.Type, t, err)
}

// clock is t as the source writes it: the wall clock in the type's zone, in its layout.
func (pr Pair) clock(t time.Time) string { return t.In(pr.loc).Format(pr.def.FactTime.Layout) }

// distinct refuses readings two of which share a wall clock, since they would be one
// facet and the shape's counts would not hold.
func (pr Pair) distinct(rs []reading) error {
	seen := make(map[string]bool, len(rs))
	for _, r := range rs {
		ts := pr.clock(r.at)
		if seen[ts] {
			return fmt.Errorf("%w: %s: two readings fall on %s in the layout %q", ErrType, pr.def.Type, ts, pr.def.FactTime.Layout)
		}
		seen[ts] = true
	}
	return nil
}

// envelope is r as the source sends it: the time and the two numbers, keyed as the
// Definition keys them.
func (pr Pair) envelope(r reading) (facet.Envelope, error) {
	doc := map[string]any{pr.at: pr.clock(r.at), pr.a: r.value, pr.b: 60 + r.value%25}
	ref, err := pr.def.SourceRef(source, doc)
	if err != nil {
		return facet.Envelope{}, err
	}
	payload, err := json.Marshal(doc)
	if err != nil {
		return facet.Envelope{}, err
	}
	return facet.Envelope{
		Type: pr.def.Type, Source: source, Injector: source,
		SourceRef: ref, Payload: payload, AssertedPerson: Person,
	}, nil
}
