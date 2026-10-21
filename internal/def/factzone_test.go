package def_test

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
)

// Card 198: a fact names the zone its wall clock was said in. These run on the bundled
// record.edge, the note the conversation writer saves, and the dose.

func bundledDef(t *testing.T, typ string) def.Definition {
	t.Helper()
	reg, err := def.Bundled()
	if err != nil {
		t.Fatal(err)
	}
	d, ok := reg.Lookup(facet.Type(typ))
	if !ok {
		t.Fatalf("no bundled %s", typ)
	}
	return d
}

// note is a record.edge payload said at the wall clock at, with source_time when zone
// is set.
func note(at, zone, offset string) string {
	st := ""
	if zone != "" {
		st = `, "source_time": {"value": "` + at + `", "zone": "` + zone + `", "basis": "source"`
		if offset != "" {
			st += `, "offset": "` + offset + `"`
		}
		st += `}`
	}
	return `{"id": "conversation:0123456789abcdef", "at": "` + at + `", "text": "words"` + st + `}`
}

func TestZonePointer_TheFactIsReadAtTheOffsetItNames(t *testing.T) {
	t.Parallel()
	d := bundledDef(t, "record.edge")
	if d.FactTime.ZonePointer != "/source_time/offset" {
		t.Fatalf("record.edge v%d fact time %+v, want it read at source_time's offset", d.Version, d.FactTime)
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
		{"a London note at 18:00", note("2026-10-19 18:00", "Europe/London", "+01:00"), utc("2026-10-19T17:00:00Z"), ""},
		{"a zone known only as its offset", note("2026-10-19 18:00", "+01:00", "+01:00"), utc("2026-10-19T17:00:00Z"), ""},
		{"a Sydney note", note("2026-10-19 18:00", "Australia/Sydney", "+11:00"), utc("2026-10-19T07:00:00Z"), ""},
		// London's clocks go back at 02:00 BST on 25 October 2026: 01:30 happens twice.
		{"a repeated hour, the first by its offset", note("2026-10-25 01:30", "Europe/London", "+01:00"), utc("2026-10-25T00:30:00Z"), ""},
		{"a repeated hour, the second by its offset", note("2026-10-25 01:30", "Europe/London", "+00:00"), utc("2026-10-25T01:30:00Z"), ""},
		{"a repeated hour, the name alone", note("2026-10-25 01:30", "Europe/London", ""), time.Time{}, "offset"},
		// London's clocks go forward at 01:00 GMT on 29 March 2026: 01:30 never happens.
		{"a skipped hour, an offset cannot conjure it", note("2026-03-29 01:30", "Europe/London", "+00:00"), time.Time{}, "is not a wall clock Europe/London shows at offset +00:00"},
		{"an offset the zone does not keep", note("2026-10-19 18:00", "Europe/London", "+05:00"), time.Time{}, "is not a wall clock Europe/London shows at offset +05:00"},
		// Sydney skips 02:30 on 4 October; said in London it is an ordinary time.
		{"Sydney's skipped hour, said in London", note("2026-10-04 02:30", "Europe/London", "+01:00"), utc("2026-10-04T01:30:00Z"), ""},
		{"a zone that is none", note("2026-10-19 18:00", "Mars/Olympus_Mons", "+01:00"), time.Time{}, "Mars/Olympus_Mons"},
		{"the host's zone", note("2026-10-19 18:00", "Local", "+01:00"), time.Time{}, `"Local" names no zone`},
		{"no source_time: the version requires it", note("2026-10-19 18:00", "", ""), time.Time{}, "source_time"},
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

	// A note admitted before v2 carries no source_time, and v2 reads it where v1 did:
	// in Sydney, its skipped hour refused as before.
	if got, err := d.FactTimeOf(mustDecode(t, note("2026-10-19 18:00", "", ""))); err != nil || !got.Equal(utc("2026-10-19T07:00:00Z")) {
		t.Errorf("an old note reads %v, %v; want 07:00Z, as v1 read it", got, err)
	}
	if _, err := d.FactTimeOf(mustDecode(t, note("2026-10-04 02:30", "", ""))); err == nil || !strings.Contains(err.Error(), "does not exist in Australia/Sydney") {
		t.Errorf("an old note in Sydney's skipped hour: %v, want refused", err)
	}
}

// The dose's id takes the offset when the dose names one (Matt, 2026-10-21): a dose
// recorded before keeps its id byte for byte, and one local minute in two zones is two
// doses.
func TestZonePointer_TheDoseIDTakesTheOffset(t *testing.T) {
	t.Parallel()
	now := bundledDef(t, "health.medication.dose")
	raw, err := os.ReadFile("manifests/history/health.medication.dose.v1.json")
	if err != nil {
		t.Fatal(err)
	}
	before := mustParse(t, string(raw))
	plain := `{"taken_at": "2026-10-17 22:00", "drug": "Melatonin", "amount": 5, "unit": "mg"}`
	was, err := def.ComposeID(before, "conversation", []byte(plain))
	if err != nil {
		t.Fatal(err)
	}
	is, err := def.ComposeID(now, "conversation", []byte(plain))
	if err != nil || is != was || was != "conversation:med/melatonin/dose/2026-10-17T22:00" {
		t.Fatalf("a dose without a zone: v%d composes %q (%v), v%d composed %q", now.Version, is, err, before.Version, was)
	}
	london := `{"taken_at": "2026-10-17 22:00", "drug": "Melatonin", "amount": 5, "unit": "mg",
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
	if l != "conversation:med/melatonin/dose/2026-10-17T22:00+01:00" || s != "conversation:med/melatonin/dose/2026-10-17T22:00+11:00" || l == s {
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
		return `{"meta": {"facet_type": "health.reading", "definition_version": 1, "classification": "Normal",
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
