package def_test

import (
	"errors"
	"io/fs"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/catalog/catalogtest"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
)

// Card 198: a fact names the zone its wall clock was said in. These run on the engine
// fixture's test.reading, read at the offset source_time names or else in
// Europe/Berlin, and test.templated, whose id takes the offset from v2.

func engineDef(t *testing.T, typ string) def.Definition {
	t.Helper()
	d, ok := catalogtest.Engine.Definitions(t).Lookup(facet.Type(typ))
	if !ok {
		t.Fatalf("no %s in the engine fixture", typ)
	}
	return d
}

// historyDef parses a version of a type from the engine fixture's history.
func historyDef(t *testing.T, name string) def.Definition {
	t.Helper()
	raw, err := fs.ReadFile(fixtureDefs(), "history/"+name)
	if err != nil {
		t.Fatal(err)
	}
	return mustParse(t, string(raw))
}

// zonedReading is a test.reading payload said at the wall clock at, with source_time when
// zone is set.
func zonedReading(at, zone, offset string) string {
	st := ""
	if zone != "" {
		st = `, "source_time": {"value": "` + at + `", "zone": "` + zone + `", "basis": "source"`
		if offset != "" {
			st += `, "offset": "` + offset + `"`
		}
		st += `}`
	}
	return `{"id": "reading:0123456789abcdef", "at": "` + at + `", "value": 5, "unit": "mg"` + st + `}`
}

func TestZonePointer_TheFactIsReadAtTheOffsetItNames(t *testing.T) {
	t.Parallel()
	d := engineDef(t, "test.reading")
	if d.FactTime.ZonePointer != "/source_time/offset" || d.FactTime.Zone != "Europe/Berlin" {
		t.Fatalf("test.reading v%d fact time %+v, want it read at source_time's offset, else in Europe/Berlin", d.Version, d.FactTime)
	}
	utc := func(s string) time.Time {
		t.Helper()
		v, err := time.Parse(time.RFC3339, s)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	cases := []struct {
		name, payload string
		want          time.Time
		refused       string
	}{
		{"a London reading at 18:00", zonedReading("2026-10-19 18:00", "Europe/London", "+01:00"), utc("2026-10-19T17:00:00Z"), ""},
		{"a zone known only as its offset", zonedReading("2026-10-19 18:00", "+01:00", "+01:00"), utc("2026-10-19T17:00:00Z"), ""},
		{"a Sydney reading", zonedReading("2026-10-19 18:00", "Australia/Sydney", "+11:00"), utc("2026-10-19T07:00:00Z"), ""},
		// London's clocks go back at 02:00 BST on 25 October 2026: 01:30 happens twice.
		{"a repeated hour, the first by its offset", zonedReading("2026-10-25 01:30", "Europe/London", "+01:00"), utc("2026-10-25T00:30:00Z"), ""},
		{"a repeated hour, the second by its offset", zonedReading("2026-10-25 01:30", "Europe/London", "+00:00"), utc("2026-10-25T01:30:00Z"), ""},
		{"a repeated hour, the name alone", zonedReading("2026-10-25 01:30", "Europe/London", ""), time.Time{}, "offset"},
		// London's clocks go forward at 01:00 GMT on 29 March 2026: 01:30 never happens.
		{"a skipped hour, an offset cannot conjure it", zonedReading("2026-03-29 01:30", "Europe/London", "+00:00"), time.Time{}, "is not a wall clock Europe/London shows at offset +00:00"},
		{"an offset the zone does not keep", zonedReading("2026-10-19 18:00", "Europe/London", "+05:00"), time.Time{}, "is not a wall clock Europe/London shows at offset +05:00"},
		// Berlin skips 02:30 on 29 March; said in London, an hour behind, it is an
		// ordinary time.
		{"Berlin's skipped hour, said in London", zonedReading("2026-03-29 02:30", "Europe/London", "+01:00"), utc("2026-03-29T01:30:00Z"), ""},
		{"a zone that is none", zonedReading("2026-10-19 18:00", "Mars/Olympus_Mons", "+01:00"), time.Time{}, "Mars/Olympus_Mons"},
		{"the host's zone", zonedReading("2026-10-19 18:00", "Local", "+01:00"), time.Time{}, `"Local" names no zone`},
		{"no source_time: the version requires it", zonedReading("2026-10-19 18:00", "", ""), time.Time{}, "source_time"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			doc := mustDecode(t, c.payload)
			err := d.Check(doc)
			got, ferr := d.FactTimeOf(doc)
			if err == nil {
				err = ferr
			}
			if c.refused != "" {
				if !errors.Is(err, def.ErrNotCompliant) || !strings.Contains(err.Error(), c.refused) {
					t.Fatalf("Check and FactTimeOf = %v; want refused naming %q", err, c.refused)
				}
				return
			}
			if err != nil || !got.Equal(c.want) {
				t.Fatalf("FactTimeOf = %v, %v; want %v", got, err, c.want)
			}
		})
	}

	// A reading admitted before v2 carries no source_time, and v2 reads it where v1
	// did: in Berlin, its skipped hour refused as before.
	v1 := historyDef(t, "test.reading.v1.json")
	old := mustDecode(t, zonedReading("2026-10-19 18:00", "", ""))
	if got, err := d.FactTimeOf(old); err != nil || !got.Equal(utc("2026-10-19T16:00:00Z")) {
		t.Errorf("an old reading reads %v, %v; want 16:00Z", got, err)
	}
	if got, err := v1.FactTimeOf(old); err != nil || !got.Equal(utc("2026-10-19T16:00:00Z")) {
		t.Errorf("v1 reads the old reading at %v, %v; want 16:00Z, as v2 does", got, err)
	}
	if _, err := d.FactTimeOf(mustDecode(t, zonedReading("2026-03-29 02:30", "", ""))); err == nil || !strings.Contains(err.Error(), "does not exist in Europe/Berlin") {
		t.Errorf("an old reading in Berlin's skipped hour: %v, want refused", err)
	}
}

// The id takes the offset when the payload names one (Matt, 2026-10-21): a fact
// recorded before keeps its id byte for byte, and one local minute in two zones is two
// facts. test.templated v1 composed no offset; v2 appends it as an optional segment.
func TestZonePointer_TheTemplatedIDTakesTheOffset(t *testing.T) {
	t.Parallel()
	now := engineDef(t, "test.templated")
	before := historyDef(t, "test.templated.v1.json")
	plain := `{"at": "2026-10-17 22:00", "name": "Melatonin", "amount": 5, "unit": "mg"}`
	was, err := def.ComposeID(before, "conversation", []byte(plain))
	if err != nil {
		t.Fatal(err)
	}
	is, err := def.ComposeID(now, "conversation", []byte(plain))
	if err != nil || is != was || was != "conversation:templated/melatonin/2026-10-17T22:00" {
		t.Fatalf("a payload without a zone: v%d composes %q (%v), v%d composed %q", now.Version, is, err, before.Version, was)
	}
	london := `{"at": "2026-10-17 22:00", "name": "Melatonin", "amount": 5, "unit": "mg",
		"source_time": {"value": "2026-10-17 22:00", "zone": "Europe/London", "offset": "+01:00", "basis": "source"}}`
	sydney := strings.ReplaceAll(strings.ReplaceAll(london, "Europe/London", "Australia/Sydney"), "+01:00", "+11:00")
	l, err := def.ComposeID(now, "conversation", []byte(london))
	if err != nil {
		t.Fatal(err)
	}
	s, err := def.ComposeID(now, "conversation", []byte(sydney))
	if err != nil {
		t.Fatal(err)
	}
	if l != "conversation:templated/melatonin/2026-10-17T22:00+01:00" || s != "conversation:templated/melatonin/2026-10-17T22:00+11:00" || l == s {
		t.Errorf("one local minute in two zones: %q and %q, want two ids ending in their offsets", l, s)
	}
	// The gate fills and holds the id the same way.
	filled, err := now.WithID("conversation", []byte(london))
	if err != nil || !strings.Contains(string(filled), `"id":"`+l+`"`) {
		t.Errorf("WithID = %s, %v; want the id %s", filled, err, l)
	}
}

// The zone pointer is Parse's to hold in place.
func TestZonePointer_ParseHoldsItsPlace(t *testing.T) {
	t.Parallel()
	shape, err := os.ReadFile("sourcetime/v2.json")
	if err != nil {
		t.Fatal(err)
	}
	manifest := func(fact, props, mode string) string {
		schema := `, "schema": {"type": "object", "required": ["at"], "additionalProperties": false,
		    "properties": {"at": {"type": "string", "minLength": 1}` + props + `}}`
		if mode == "verbatim" {
			schema = ""
		}
		return `{"meta": {"facet_type": "test.raw-reading", "definition_version": 1, "classification": "Normal",
		  "validation_mode": "` + mode + `", "source_schema": "test/reading",
		  "time_semantics": {"fact_time": ` + fact + `}, "identity_key": ["/at"]}` + schema + `}`
	}
	st := `, "source_time": ` + string(shape)
	zoned := `{"pointer": "/at", "layout": "2006-01-02 15:04", "zone": "Australia/Sydney", "zone_pointer": "/source_time/offset"}`
	if _, err := def.Parse([]byte(manifest(zoned, st, "strict"))); err != nil {
		t.Fatalf("a zoned fact time: %v", err)
	}
	for name, c := range map[string]struct{ fact, props, mode, want string }{
		"a layout with its own offset": {`{"pointer": "/at", "layout": "2006-01-02T15:04Z07:00", "zone": "UTC", "zone_pointer": "/source_time/offset"}`, st, "strict", "carries its own offset"},
		"a zone outside source_time":   {`{"pointer": "/at", "layout": "2006-01-02 15:04", "zone": "UTC", "zone_pointer": "/tz"}`, st + `, "tz": {"type": "string"}`, "strict", "live in source_time"},
		"a pointer the schema lacks":   {zoned, "", "strict", "declares no field /source_time/offset"},
		"a verbatim type":              {zoned, "", "verbatim", "verbatim type"},
	} {
		if _, err := def.Parse([]byte(manifest(c.fact, c.props, c.mode))); !errors.Is(err, def.ErrBadManifest) || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: Parse = %v, want ErrBadManifest naming %q", name, err, c.want)
		}
	}
}
