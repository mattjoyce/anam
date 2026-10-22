package gate_test

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/gate"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// Card 222: the attention types, over the bundled Definitions, a real gate and a real
// ledger. A person says what is absorbing them; the gate holds the reading's topic to
// one spelling and its intensity to 1 to 5, and the started and ended pair is a series.

func newAttentionGate(t *testing.T) (*gate.Gate, *ledger.Ledger, *def.Registry) {
	t.Helper()
	l, err := ledger.Create(context.Background(), filepath.Join(t.TempDir(), "ledger.db"), person)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	reg, err := def.Bundled()
	if err != nil {
		t.Fatal(err)
	}
	scopes := gate.StaticScopes{"conversation": {"mind.attention", "mind.attention.started", "mind.attention.ended"}}
	g := gate.New(l, reg, scopes, clock.Real{}, person, gate.Limits{MaxPayloadBytes: config.DefaultMaxPayloadBytes, MaxRetries: config.DefaultCommitRetries})
	return g, l, reg
}

func sayAttention(t *testing.T, g *gate.Gate, typ, payload string) (gate.Result, error) {
	t.Helper()
	env := facet.Envelope{Type: facet.Type(typ), Source: "conversation", Injector: "conversation", Payload: []byte(payload)}
	b, err := g.Resolve(env)
	if err != nil {
		return gate.Result{}, err
	}
	res, _, err := g.Commit(context.Background(), env, b)
	return res, err
}

// attentionClock is source_time for a wall clock said in Sydney in October, at +11:00.
func attentionClock(wall string) string {
	return fmt.Sprintf(`"source_time": {"value": %q, "zone": "Australia/Sydney", "offset": "+11:00", "basis": "source"}`, wall)
}

func reading(at, topic, extra string) string {
	return fmt.Sprintf(`{"at": %q, "topic": %q, "intensity": 4, %s%s}`, at, topic, extra, attentionClock(at))
}

func TestAttention_EachTypeAdmitsAGoodPayloadAndRefusesABadOne(t *testing.T) {
	t.Parallel()
	const at = "2026-10-22 21:40"
	for _, tc := range []struct {
		name, typ, payload string
		ok                 bool
	}{
		{"a reading", "mind.attention", reading(at, "deskcam", ""), true},
		{"a reading with a note", "mind.attention", reading(at, "deskcam", `"note": "the stand prints straight", `), true},
		{"intensity at the top", "mind.attention", strings.Replace(reading(at, "deskcam", ""), `"intensity": 4`, `"intensity": 5`, 1), true},
		{"intensity at the bottom", "mind.attention", strings.Replace(reading(at, "deskcam", ""), `"intensity": 4`, `"intensity": 1`, 1), true},
		{"intensity zero", "mind.attention", strings.Replace(reading(at, "deskcam", ""), `"intensity": 4`, `"intensity": 0`, 1), false},
		{"intensity six", "mind.attention", strings.Replace(reading(at, "deskcam", ""), `"intensity": 4`, `"intensity": 6`, 1), false},
		{"intensity fractional", "mind.attention", strings.Replace(reading(at, "deskcam", ""), `"intensity": 4`, `"intensity": 2.5`, 1), false},
		{"intensity as text", "mind.attention", strings.Replace(reading(at, "deskcam", ""), `"intensity": 4`, `"intensity": "4"`, 1), false},
		{"no intensity", "mind.attention", strings.Replace(reading(at, "deskcam", ""), `"intensity": 4, `, ``, 1), false},
		{"a topic with spaces", "mind.attention", reading(at, "desk cam", ""), false},
		{"a topic in capitals", "mind.attention", reading(at, "DeskCam", ""), false},
		{"a topic with a trailing hyphen", "mind.attention", reading(at, "deskcam-", ""), false},
		{"an empty topic", "mind.attention", reading(at, "", ""), false},
		{"a hyphenated topic", "mind.attention", reading(at, "bench-camera-2", ""), true},
		{"no source_time", "mind.attention", fmt.Sprintf(`{"at": %q, "topic": "deskcam", "intensity": 3}`, at), false},
		{"an empty note", "mind.attention", reading(at, "deskcam", `"note": "", `), false},
		{"a field of its own", "mind.attention", reading(at, "deskcam", `"mood": "good", `), false},
		{"started", "mind.attention.started", fmt.Sprintf(`{"at": %q, "topic": "deskcam", %s}`, at, attentionClock(at)), true},
		{"started with a note", "mind.attention.started", fmt.Sprintf(`{"at": %q, "topic": "deskcam", "note": "design day", %s}`, at, attentionClock(at)), true},
		{"started with spaces", "mind.attention.started", fmt.Sprintf(`{"at": %q, "topic": "desk cam", %s}`, at, attentionClock(at)), false},
		{"started without a clock", "mind.attention.started", fmt.Sprintf(`{"at": %q, "topic": "deskcam"}`, at), false},
		{"started with an intensity", "mind.attention.started", fmt.Sprintf(`{"at": %q, "topic": "deskcam", "intensity": 3, %s}`, at, attentionClock(at)), false},
		{"ended", "mind.attention.ended", fmt.Sprintf(`{"at": %q, "topic": "deskcam", %s}`, at, attentionClock(at)), true},
		{"ended without a topic", "mind.attention.ended", fmt.Sprintf(`{"at": %q, %s}`, at, attentionClock(at)), false},
		{"ended without a clock", "mind.attention.ended", fmt.Sprintf(`{"at": %q, "topic": "deskcam"}`, at), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g, l, _ := newAttentionGate(t)
			res, err := sayAttention(t, g, tc.typ, tc.payload)
			switch {
			case tc.ok && (err != nil || res.Outcome != gate.OutcomeAdmitted):
				t.Fatalf("got %+v, %v; want admitted", res, err)
			case !tc.ok && err == nil:
				t.Fatalf("admitted %+v; want a refusal", res)
			}
			if !tc.ok {
				t.Logf("refused: %v", err)
				got, err := fold.Read(context.Background(), l)
				if err != nil || len(got.Live) != 0 {
					t.Errorf("a refused payload left %d live facets (%v)", len(got.Live), err)
				}
			}
		})
	}
}

// A reading's fact time is `at` read at the offset source_time names, and its id
// carries the offset, so one minute said in two zones is two readings, while the same
// minute said again is the one reading.
func TestAttention_OneMinuteInTwoZonesIsTwoReadings(t *testing.T) {
	t.Parallel()
	g, l, _ := newAttentionGate(t)
	const at = "2026-10-22 21:40"
	sydney := reading(at, "deskcam", "")
	london := fmt.Sprintf(`{"at": %q, "topic": "deskcam", "intensity": 4, "source_time": {"value": %q, "zone": "Europe/London", "offset": "+01:00", "basis": "source"}}`, at, at)

	a, err := sayAttention(t, g, "mind.attention", sydney)
	if err != nil || a.Outcome != gate.OutcomeAdmitted {
		t.Fatalf("sydney: %+v, %v", a, err)
	}
	b, err := sayAttention(t, g, "mind.attention", london)
	if err != nil || b.Outcome != gate.OutcomeAdmitted {
		t.Fatalf("london: %+v, %v", b, err)
	}
	if a.FacetID == b.FacetID {
		t.Fatalf("one minute in two zones is one facet, %s", a.FacetID)
	}
	wantA := facet.NewID(facet.SourceRef("conversation", "conversation:attention/deskcam/2026-10-22T21:40+11:00"))
	wantB := facet.NewID(facet.SourceRef("conversation", "conversation:attention/deskcam/2026-10-22T21:40+01:00"))
	if a.FacetID != wantA || b.FacetID != wantB {
		t.Errorf("ids = %s, %s; want %s, %s", a.FacetID, b.FacetID, wantA, wantB)
	}
	again, err := sayAttention(t, g, "mind.attention", sydney)
	if err != nil || again.Outcome != gate.OutcomeNoOp || again.FacetID != a.FacetID {
		t.Errorf("saying the same minute again = %+v, %v; want a no-op on %s", again, err, a.FacetID)
	}
	// Another topic in the same minute is its own reading.
	other, err := sayAttention(t, g, "mind.attention", reading(at, "garden", ""))
	if err != nil || other.Outcome != gate.OutcomeAdmitted || other.FacetID == a.FacetID {
		t.Errorf("another topic, same minute = %+v, %v; want its own facet", other, err)
	}

	res, err := fold.Read(context.Background(), l)
	if err != nil {
		t.Fatal(err)
	}
	times := map[string]string{}
	for _, e := range res.Value.Entries() {
		times[e.FacetID] = e.FactTime
	}
	if times[a.FacetID] != "2026-10-22T10:40:00.000000000Z" || times[b.FacetID] != "2026-10-22T20:40:00.000000000Z" {
		t.Errorf("fact times = %v; want 21:40 at +11:00 and at +01:00 placed as two instants", times)
	}
}

// An ended closes the series of its topic's latest started, a later started opens it
// again, and another topic's series is its own.
func TestAttention_AnEndedClosesItsStartedAndALaterStartedReopensIt(t *testing.T) {
	t.Parallel()
	g, l, reg := newAttentionGate(t)
	say := func(typ, topic, at string) string {
		t.Helper()
		p := fmt.Sprintf(`{"at": %q, "topic": %q, %s}`, at, topic, attentionClock(at))
		res, err := sayAttention(t, g, typ, p)
		if err != nil || res.Outcome != gate.OutcomeAdmitted {
			t.Fatalf("%s %s %s: %+v, %v", typ, topic, at, res, err)
		}
		return res.FacetID
	}
	started1 := say("mind.attention.started", "deskcam", "2026-10-20 09:00")
	garden := say("mind.attention.started", "garden", "2026-10-20 10:00")
	say("mind.attention.ended", "deskcam", "2026-10-21 18:00")
	started2 := say("mind.attention.started", "deskcam", "2026-10-23 08:00")

	res, err := fold.Read(context.Background(), l)
	if err != nil {
		t.Fatal(err)
	}
	// Instants are Sydney 09:00 and so on, at +11:00; a bound is the UTC instant.
	for _, tc := range []struct {
		name, at string
		want     []string
	}{
		{"before anything", "2026-10-19T00:00:00.000000000Z", nil},
		{"deskcam open", "2026-10-19T22:30:00.000000000Z", []string{started1}},
		{"deskcam and garden open", "2026-10-20T00:00:00.000000000Z", []string{started1, garden}},
		{"deskcam closed, garden still open", "2026-10-21T08:00:00.000000000Z", []string{garden}},
		{"deskcam reopened", "2026-10-23T00:00:00.000000000Z", []string{garden, started2}},
	} {
		s, err := fold.Snapshot(res.Value, tc.at, reg.Closes)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		var got []string
		for _, e := range s.Entries() {
			got = append(got, e.FacetID)
		}
		if strings.Join(got, ",") != strings.Join(tc.want, ",") {
			t.Errorf("%s: open = %v, want %v", tc.name, got, tc.want)
		}
	}
}
