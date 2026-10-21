package inspect

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/def"
)

const (
	noteA = "2c6f38a3-3db7-5e65-b933-9c59980e1c6c"
	noteB = "7d1b2c3e-4f5a-5b6c-8d7e-9f0a1b2c3d4e"
)

// The link's id is card 202's rule exactly: the value below was computed apart from
// this code, from the canonical string the rule's comment writes. The order of a side
// does not change it; the kind, a side, the moment and the offset each do; the words
// do not.
func TestEdgeIDIsTheRule(t *testing.T) {
	t.Parallel()
	base := edgeID("about", nil, []string{noteA, noteB}, "2026-10-21 14:30", "+11:00")
	if base != "workbench:edge:2d2f1575f837e477" {
		t.Fatalf("edgeID = %s, want workbench:edge:2d2f1575f837e477", base)
	}
	if got := edgeID("about", nil, []string{noteB, noteA}, "2026-10-21 14:30", "+11:00"); got != base {
		t.Errorf("the order of a side changed the id: %s", got)
	}
	for name, other := range map[string]string{
		"kind":   edgeID("interpreted_from", nil, []string{noteA, noteB}, "2026-10-21 14:30", "+11:00"),
		"side":   edgeID("about", []string{noteA}, []string{noteB}, "2026-10-21 14:30", "+11:00"),
		"moment": edgeID("about", nil, []string{noteA, noteB}, "2026-10-21 14:31", "+11:00"),
		"offset": edgeID("about", nil, []string{noteA, noteB}, "2026-10-21 14:30", "+10:00"),
	} {
		if other == base {
			t.Errorf("a different %s gave the same id", name)
		}
	}
}

// linkFormAt is record.edge's form as the Definition in force draws it, without a
// server: its kinds and its at field.
func linkFormAt(t *testing.T) linkForm {
	t.Helper()
	reg, err := def.Bundled()
	if err != nil {
		t.Fatal(err)
	}
	d, ok := reg.Lookup("record.edge")
	if !ok {
		t.Fatal("record.edge is not bundled")
	}
	loc, err := time.LoadLocation(d.FactTime.Zone)
	if err != nil {
		t.Fatal(err)
	}
	return linkForm{def: d, kinds: []string{"about", "interpreted_from"},
		at: entryForm{timeAt: d.FactTime.Pointer, layout: d.FactTime.Layout, loc: loc, Zoned: d.FactTime.ZonePointer != ""}}
}

func posting(kind string, to, from, pins []string) url.Values {
	vals := url.Values{"kind": {kind}, "at": {"2026-10-21T14:30"}, "zone": {"Australia/Sydney"}, "offset": {"+11:00"}}
	for _, id := range to {
		vals.Add("item", id)
		vals.Add("keep", id)
	}
	for _, id := range from {
		vals.Add("item", id)
		vals.Add("keep", id)
		vals.Set("side."+id, "from")
	}
	vals["pin"] = pins
	return vals
}

// held marks every item as a facet the ledger holds at hash h, as describe would.
func held(d *linkData, h string) {
	for i := range d.Items {
		d.Items[i].Held, d.Items[i].Current = true, h
		if d.Items[i].Hash == "" {
			d.Items[i].Hash = h
		}
	}
}

// The form's own refusals, before the gate: about with a from side, more than 20
// facets unconfirmed, a kind off the list, and a pin on a version no longer current.
// Pins are written only for the ticked facets.
func TestLinkPayloadRules(t *testing.T) {
	t.Parallel()
	lf := linkFormAt(t)
	h := "sha256:" + strings.Repeat("ab", 32)
	build := func(vals url.Values) (string, string) {
		d := lf.posted(vals)
		held(&d, h)
		raw, why := lf.payload(&d, vals, edgeValue{})
		return string(raw), why
	}

	if _, why := build(posting("about", []string{noteB}, []string{noteA}, nil)); !strings.Contains(why, "has no from side") {
		t.Errorf("about with a from side: %q", why)
	}
	if _, why := build(posting("sideways", []string{noteA}, nil, nil)); !strings.Contains(why, "is not one record.edge") {
		t.Errorf("a kind off the list: %q", why)
	}

	many := make([]string, confirmAbove+1)
	for i := range many {
		many[i] = strings.Replace(noteA, "2c6f", string(rune('a'+i%26))+string(rune('a'+i/26))+"6f", 1)
	}
	vals := posting("about", many, nil, nil)
	if _, why := build(vals); !strings.Contains(why, "more than 20") {
		t.Errorf("21 facets unconfirmed: %q", why)
	}
	vals.Set("confirm", "yes")
	if _, why := build(vals); why != "" {
		t.Errorf("21 facets confirmed: %q", why)
	}

	raw, why := build(posting("about", []string{noteA, noteB}, nil, []string{noteB}))
	if why != "" || !strings.Contains(raw, `"pins":{"`+noteB+`":"`+h+`"}`) {
		t.Errorf("one pin ticked: %s %q", raw, why)
	}
	raw, _ = build(posting("about", []string{noteA, noteB}, nil, nil))
	if strings.Contains(raw, "pins") || !strings.Contains(raw, `"id":"workbench:edge:2d2f1575f837e477"`) {
		t.Errorf("no pin ticked: %s", raw)
	}

	stale := posting("about", []string{noteA}, nil, []string{noteA})
	stale.Set("hash."+noteA, "sha256:"+strings.Repeat("cd", 32))
	d := lf.posted(stale)
	held(&d, h)
	if _, why := lf.payload(&d, stale, edgeValue{}); !strings.Contains(why, "has a newer version") || d.Items[0].Hash != h {
		t.Errorf("a stale pin: %q, hash now %s", why, d.Items[0].Hash)
	}
}
