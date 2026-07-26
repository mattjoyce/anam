package bpcsv_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/capture/bpcsv"
	"github.com/mattjoyce/anam/internal/def"
)

// The fixture reproduces the real export's every mess: "\t "-padded fields, CRLF
// endings, quotes inside padded fields, a trailing empty column, unpadded month
// and day digits, two distinct readings sharing a timestamp, sparse cells — and a
// typo in a comment, which is the source's to keep.
const fixture = "Date (Excel),\tDate,\tSystolic,\tDiastolic,\tPulse,\tIrregular heartbeat,\tMissed medication,\tDizzy / Light-headed,\tWeight [kg],\tTemperature [Celsius],\tBlood Glucose [mg/dL],\tOxygen Saturation [%],\tLocation,\tPosture,\tComment,\tTags,\t\r\n" +
	"2026-1-24 06:36:00,\t 24 January 2026 6:36,\t 114.0,\t 80.0,\t 70.0,\t ,\t ,\t ,\t 96.3,\t ,\t ,\t ,\t \"Left Arm\",\t \"Seated\",\t \"No filofopine\",\t ,\t \r\n" +
	"2026-2-10 05:04:00,\t 10 February 2026 5:04,\t 102.0,\t 72.0,\t 68.0,\t ,\t ,\t ,\t ,\t ,\t ,\t ,\t ,\t ,\t ,\t ,\t \r\n" +
	"2026-2-10 05:04:00,\t 10 February 2026 5:04,\t 101.0,\t 72.0,\t 66.0,\t ,\t ,\t ,\t ,\t ,\t ,\t ,\t ,\t ,\t ,\t ,\t \r\n"

func scan(t *testing.T) []struct {
	SourceRef string
	Payload   string
} {
	t.Helper()
	path := filepath.Join(t.TempDir(), "bp.csv")
	if err := os.WriteFile(path, []byte(fixture), 0o600); err != nil {
		t.Fatal(err)
	}
	envs, err := bpcsv.Scan(path, "matt")
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	out := make([]struct {
		SourceRef string
		Payload   string
	}, len(envs))
	for i, e := range envs {
		out[i] = struct {
			SourceRef string
			Payload   string
		}{e.SourceRef, string(e.Payload)}
	}
	return out
}

// Two distinct readings in one minute must be two facets, not a supersession:
// the ordinal is part of WHICH reading a row is.
func TestScan_CollidingTimestampsStayDistinct(t *testing.T) {
	t.Parallel()
	rows := scan(t)
	if len(rows) != 3 {
		t.Fatalf("Scan composed %d envelopes, want 3", len(rows))
	}
	if rows[1].SourceRef != "bp-csv:2026-2-10 05:04:00:1" ||
		rows[2].SourceRef != "bp-csv:2026-2-10 05:04:00:2" {
		t.Errorf("colliding readings keyed %q and %q — must differ by ordinal",
			rows[1].SourceRef, rows[2].SourceRef)
	}
	if !strings.Contains(rows[1].Payload, `"102.0"`) || !strings.Contains(rows[2].Payload, `"101.0"`) {
		t.Error("the two readings' values did not both survive")
	}
}

// Values are the source's own strings: unparsed numbers, packaging stripped,
// typos intact, empty cells absent rather than empty.
func TestScan_ValuesAreVerbatimStrings(t *testing.T) {
	t.Parallel()
	rows := scan(t)
	p := rows[0].Payload

	for _, want := range []string{
		`"systolic":"114.0"`, `"weight_kg":"96.3"`,
		`"location":"Left Arm"`, `"comment":"No filofopine"`,
		`"date_excel":"2026-1-24 06:36:00"`, `"reading":"1"`,
	} {
		if !strings.Contains(p, want) {
			t.Errorf("payload missing %s\n%s", want, p)
		}
	}
	for _, absent := range []string{"temperature", "glucose", "tags", `""`} {
		if strings.Contains(p, absent) {
			t.Errorf("payload carries an empty cell (%s): %s", absent, p)
		}
	}
}

// Every envelope must clear the bundled contract the gate will hold it to —
// including the identity agreement: the worker's source_ref byte-identical to the
// one the definition composes from the payload.
func TestScan_ComposesGateCompliantEnvelopes(t *testing.T) {
	t.Parallel()
	reg, err := def.Bundled()
	if err != nil {
		t.Fatalf("Bundled: %v", err)
	}
	d, ok := reg.Lookup(bpcsv.Type)
	if !ok {
		t.Fatalf("no bundled definition for %s", bpcsv.Type)
	}

	path := filepath.Join(t.TempDir(), "bp.csv")
	if err := os.WriteFile(path, []byte(fixture), 0o600); err != nil {
		t.Fatal(err)
	}
	envs, err := bpcsv.Scan(path, "matt")
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	for _, env := range envs {
		if err := env.Validate(); err != nil {
			t.Errorf("%s: %v", env.SourceRef, err)
		}
		doc, err := def.Decode(env.Payload)
		if err != nil {
			t.Fatalf("%s: decode: %v", env.SourceRef, err)
		}
		if err := d.Check(doc); err != nil {
			t.Errorf("%s: not compliant: %v", env.SourceRef, err)
		}
		event, err := d.FactTimeOf(doc)
		if err != nil {
			t.Errorf("%s: times: %v", env.SourceRef, err)
		} else if event.Year() != 2026 {
			t.Errorf("%s: fact time %s", env.SourceRef, event)
		}
		ref, err := d.SourceRef(env.Source, doc)
		if err != nil {
			t.Errorf("%s: compose ref: %v", env.SourceRef, err)
		} else if ref != env.SourceRef {
			t.Errorf("identity disagreement: worker %q, definition %q", env.SourceRef, ref)
		}
	}
}
