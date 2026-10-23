package daemon

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/fold"
)

// The link form (card 202) through a real daemon: the importers file lists record.edge
// for the workbench principal, the facets it joins are weigh-ins the test importer
// admits, and every page is read back from the ledger and the edge index the write
// caught up.

// weighIns admits n weigh-ins as the test importer and returns their facet ids.
func weighIns(t *testing.T, d *Daemon, base string, n int) []string {
	t.Helper()
	weight, _ := d.defs.current.Load().Lookup("health.weight")
	envs := make([]string, n)
	for i := range envs {
		envs[i] = weighIn(fmt.Sprintf("w-%02d", i), 80+float64(i)/10, weight.Version)
	}
	var adm api.Admission
	call(t, "POST", base+"/v1/envelopes", importerToken, batch(envs...), http.StatusOK, &adm)
	ids := make([]string, n)
	for i, r := range adm.Results {
		ids[i] = r.FacetID
	}
	return ids
}

// edgeVersion is record.edge's Definition version in force.
func edgeVersion(t *testing.T, d *Daemon) int {
	t.Helper()
	e, ok := d.defs.current.Load().Lookup("record.edge")
	if !ok {
		t.Fatal("record.edge is not in force")
	}
	return e.Version
}

// linkForm is what the link form posts: each facet drawn and kept on its side, the
// pinned ones ticked, at 14:30 in Sydney on 21 October.
func linkForm(version int, kind, text string, to, from, pins []string) url.Values {
	vals := url.Values{
		"v": {strconv.Itoa(version)}, "kind": {kind}, "text": {text},
		"at": {"2026-10-21T14:30"}, "zone": {"Australia/Sydney"}, "offset": {"+11:00"},
	}
	for _, id := range to {
		vals.Add("item", id)
		vals.Add("keep", id)
		vals.Set("side."+id, "to")
	}
	for _, id := range from {
		vals.Add("item", id)
		vals.Add("keep", id)
		vals.Set("side."+id, "from")
	}
	for _, id := range pins {
		vals.Add("pin", id)
	}
	return vals
}

// wantEdgeID is the link's facet id by card 202's rule, written out again here: the
// payload id is "workbench:edge:" and 16 hex of the SHA-256 of the kind, each side
// sorted and the moment with its offset, and the facet id is the workbench's for it.
func wantEdgeID(kind string, to, from []string) (payloadID, facetID string) {
	to, from = append([]string(nil), to...), append([]string(nil), from...)
	sort.Strings(to)
	sort.Strings(from)
	sum := sha256.Sum256([]byte("kind=" + kind + "\nfrom=" + strings.Join(from, ",") + "\nto=" + strings.Join(to, ",") +
		"\nat=2026-10-21 14:30+11:00"))
	payloadID = "workbench:edge:" + hex.EncodeToString(sum[:])[:16]
	return payloadID, facet.NewID(facet.SourceRef("workbench", payloadID))
}

// pageLists says a facet's page lists the edge in its links section.
func pageLists(t *testing.T, base, facetID, edge string) bool {
	t.Helper()
	status, body := page(t, "GET", base+"/facet?id="+facetID, nil, nil)
	if status != http.StatusOK {
		t.Fatalf("GET /facet %s = %d", facetID, status)
	}
	return strings.Contains(body, `<td class="mono"><span class="facetid"><a class="mono" href="/facet?id=`+edge+`">`+edge+`</a>`)
}

var handoffLink = regexp.MustCompile(`<a id="handoff" href="([^"]+)"`)

// The card's example: from a facet's page, link this, add three more in the selection
// page, use the selection, write a comment and save. One record.edge with four facets
// in to, its id by the rule, and each facet's page lists it.
func TestLinkFormLinksFourFacetsAndEachPageListsIt(t *testing.T) {
	d, base := workbenchServing(t, "record.edge")
	ids := weighIns(t, d, base, 4)

	_, first := page(t, "GET", base+"/facet?id="+ids[0], nil, nil)
	if !strings.Contains(first, `href="/select?tick=`+ids[0]+`">link this…</a>`) {
		t.Fatalf("the facet page offers no link this:\n%s", first)
	}
	q := url.Values{"tick": ids}
	_, sel := page(t, "GET", base+"/select?"+q.Encode(), nil, nil)
	m := handoffLink.FindStringSubmatch(sel)
	if m == nil {
		t.Fatalf("the selection page has no handoff:\n%s", sel)
	}
	status, form := page(t, "GET", base+html.UnescapeString(m[1]), nil, nil)
	if status != http.StatusOK {
		t.Fatalf("GET the handoff = %d:\n%s", status, form)
	}
	for _, id := range ids {
		if !strings.Contains(form, `name="keep" value="`+id+`" checked`) || !strings.Contains(form, `<a class="mono" href="/facet?id=`+id+`">`) {
			t.Errorf("the form lacks %s, kept and linked to its page", id)
		}
	}
	for _, want := range []string{"<td>health.weight</td>", "4 in the link", `<option value="about" selected>about</option>`,
		`data-fact-time data-now`, `<input type="hidden" id="zone" name="zone" value="">`, "Intl.DateTimeFormat().resolvedOptions().timeZone"} {
		if !strings.Contains(form, want) {
			t.Errorf("the form lacks %q", want)
		}
	}
	if strings.Contains(form, `name="confirm"`) {
		t.Error("four facets ask for confirmation")
	}

	status, body := page(t, "POST", base+"/link", linkForm(edgeVersion(t, d), "about", "During a large work meeting, presenting.", ids, nil, nil), nil)
	payloadID, edge := wantEdgeID("about", ids, nil)
	if status != http.StatusOK || !strings.Contains(body, "admitted") || !strings.Contains(body, edge) {
		t.Fatalf("save = %d, want %s admitted:\n%s", status, edge, body)
	}
	f, err := d.read.CurrentVersion(context.Background(), edge)
	if err != nil {
		t.Fatal(err)
	}
	if f.Type != "record.edge" || f.Provenance.Source != "workbench" {
		t.Fatalf("the link is a %s from %s", f.Type, f.Provenance.Source)
	}
	sortedIDs := append([]string(nil), ids...)
	sort.Strings(sortedIDs)
	for _, want := range []string{`"id":"` + payloadID + `"`, `"kind":"about"`, `"at":"2026-10-21 14:30"`,
		`"to":["` + strings.Join(sortedIDs, `","`) + `"]`, `"text":"During a large work meeting, presenting."`,
		`"source_time":{"basis":"source","offset":"+11:00","value":"2026-10-21 14:30","zone":"Australia/Sydney"}`} {
		if !strings.Contains(string(f.Payload.Bytes()), want) {
			t.Errorf("payload %s lacks %s", f.Payload.Bytes(), want)
		}
	}
	if strings.Contains(string(f.Payload.Bytes()), `"from"`) || strings.Contains(string(f.Payload.Bytes()), `"pins"`) {
		t.Errorf("payload %s has a from side or pins nobody asked for", f.Payload.Bytes())
	}
	for _, id := range ids {
		if !pageLists(t, base, id, edge) {
			t.Errorf("the page of %s does not list the link", id)
		}
	}
}

// The same link saved twice is one fact: the second save is the gate's no-op and the
// watermark does not move.
func TestTheSameLinkSavedTwiceIsOneFact(t *testing.T) {
	d, base := workbenchServing(t, "record.edge")
	ids := weighIns(t, d, base, 2)
	form := linkForm(edgeVersion(t, d), "about", "Both before breakfast.", ids, nil, nil)
	if status, body := page(t, "POST", base+"/link", form, nil); status != http.StatusOK || !strings.Contains(body, "admitted") {
		t.Fatalf("first save = %d:\n%s", status, body)
	}
	at := head(t, d)
	status, body := page(t, "POST", base+"/link", form, nil)
	if status != http.StatusOK || !strings.Contains(body, "no-op") {
		t.Fatalf("second save = %d, want no-op:\n%s", status, body)
	}
	if w := head(t, d); w != at {
		t.Fatalf("W moved from %d to %d on the same link saved again", at, w)
	}
}

// Editing a link opens the form with its sides, kind, words and moment; removing a
// facet and saving supersedes it under the same id, and the removed facet's page no
// longer lists it. The selection page carries the edit through. Retracting it works as
// for any Workbench facet.
func TestEditingALinkSupersedesItKeepingItsID(t *testing.T) {
	d, base := workbenchServing(t, "record.edge")
	ids := weighIns(t, d, base, 4)
	v := edgeVersion(t, d)
	page(t, "POST", base+"/link", linkForm(v, "about", "Four weigh-ins.", ids, nil, nil), nil)
	payloadID, edge := wantEdgeID("about", ids, nil)

	_, edgePage := page(t, "GET", base+"/facet?id="+edge, nil, nil)
	if !strings.Contains(edgePage, `href="/link?edit=`+edge+`">edit this link</a>`) || !strings.Contains(edgePage, `action="/retract"`) {
		t.Fatalf("the link's page lacks edit this link or Retract:\n%s", edgePage)
	}
	status, form := page(t, "GET", base+"/link?edit="+edge, nil, nil)
	if status != http.StatusOK {
		t.Fatalf("GET edit = %d:\n%s", status, form)
	}
	for _, want := range []string{`name="edit" value="` + edge + `"`, ">Four weigh-ins.</textarea>", `value="2026-10-21T14:30"`,
		`name="zone" value="Australia/Sydney"`, `value="&#43;11:00"`, "Save a new version"} {
		if !strings.Contains(form, want) {
			t.Errorf("the edit form lacks %q", want)
		}
	}
	for _, id := range ids {
		if !strings.Contains(form, `name="keep" value="`+id+`" checked`) {
			t.Errorf("the edit form lacks %s", id)
		}
	}
	m := regexp.MustCompile(`<a href="(/select\?[^"]+)">add or remove facets`).FindStringSubmatch(form)
	if m == nil {
		t.Fatalf("the edit form links no selection page")
	}
	_, sel := page(t, "GET", base+html.UnescapeString(m[1]), nil, nil)
	if h := handoffLink.FindStringSubmatch(sel); h == nil || !strings.Contains(html.UnescapeString(h[1]), "edit="+edge) {
		t.Errorf("the selection page does not hand the edit back: %v", h)
	}

	removed := ids[3]
	edit := linkForm(v, "about", "Three weigh-ins.", ids[:3], nil, nil)
	edit.Add("item", removed) // drawn, but no longer kept
	edit.Set("edit", edge)
	status, body := page(t, "POST", base+"/link", edit, nil)
	if status != http.StatusOK || !strings.Contains(body, "superseded") || !strings.Contains(body, edge) {
		t.Fatalf("edit save = %d, want %s superseded:\n%s", status, edge, body)
	}
	f, err := d.read.CurrentVersion(context.Background(), edge)
	if err != nil {
		t.Fatal(err)
	}
	if f.ArtifactVersion != 2 || !strings.Contains(string(f.Payload.Bytes()), `"id":"`+payloadID+`"`) ||
		strings.Contains(string(f.Payload.Bytes()), removed) {
		t.Fatalf("the edited link is v%d %s, want v2 keeping %s without %s", f.ArtifactVersion, f.Payload.Bytes(), payloadID, removed)
	}
	if pageLists(t, base, removed, edge) {
		t.Error("the removed facet's page still lists the link")
	}
	for _, id := range ids[:3] {
		if !pageLists(t, base, id, edge) {
			t.Errorf("the page of %s no longer lists the link", id)
		}
	}

	if status, body := page(t, "POST", base+"/retract", url.Values{"id": {edge}}, nil); status != http.StatusOK || !strings.Contains(body, "retracted") {
		t.Fatalf("retract = %d:\n%s", status, body)
	}
	res, err := fold.Read(context.Background(), d.read)
	if err != nil {
		t.Fatal(err)
	}
	if res.States[edge].Live() {
		t.Fatal("the retracted link is still live")
	}
	if pageLists(t, base, ids[0], edge) {
		t.Error("a facet's page lists a retracted link")
	}
	if status, _ := page(t, "GET", base+"/link?edit="+edge, nil, nil); status != http.StatusConflict {
		t.Errorf("GET edit of a retracted link = %d, want 409", status)
	}
}

// Above 20 facets the form asks for confirmation, and a save without it writes nothing.
func TestALinkOfMoreThanTwentyFacetsIsConfirmed(t *testing.T) {
	d, base := workbenchServing(t, "record.edge")
	ids := weighIns(t, d, base, 21)
	v := edgeVersion(t, d)
	_, twenty := page(t, "GET", base+"/link?"+url.Values{"tick": ids[:20]}.Encode(), nil, nil)
	if strings.Contains(twenty, `name="confirm"`) {
		t.Error("twenty facets ask for confirmation")
	}
	_, form := page(t, "GET", base+"/link?"+url.Values{"tick": ids}.Encode(), nil, nil)
	if !strings.Contains(form, `name="confirm" value="yes" required`) || !strings.Contains(form, "link all 21 facets") {
		t.Fatalf("21 facets ask for no confirmation:\n%s", form)
	}

	post := linkForm(v, "about", "Three weeks of weigh-ins.", ids, nil, nil)
	before := head(t, d)
	status, body := page(t, "POST", base+"/link", post, nil)
	if status != http.StatusUnprocessableEntity || !strings.Contains(body, "names 21 facets, more than 20") {
		t.Fatalf("an unconfirmed save = %d, want 422 asking to confirm:\n%s", status, body)
	}
	if w := head(t, d); w != before {
		t.Fatalf("W moved on an unconfirmed save")
	}
	post.Set("confirm", "yes")
	if status, body := page(t, "POST", base+"/link", post, nil); status != http.StatusOK || !strings.Contains(body, "admitted") {
		t.Fatalf("a confirmed save = %d:\n%s", status, body)
	}
}

// Kind about has no from side: the form says so when the basket puts a facet there,
// and a save is refused with the same words. Another kind takes it.
func TestAboutWithAFromSideIsRefused(t *testing.T) {
	d, base := workbenchServing(t, "record.edge")
	ids := weighIns(t, d, base, 2)
	v := edgeVersion(t, d)
	_, form := page(t, "GET", base+"/link?"+url.Values{"tick": ids, "fromside": {ids[0]}}.Encode(), nil, nil)
	if !strings.Contains(form, `id="side-problem"`) || !strings.Contains(form, "A link of kind about has no from side") {
		t.Fatalf("the form does not say about takes no from side:\n%s", form)
	}
	at := head(t, d)
	status, body := page(t, "POST", base+"/link", linkForm(v, "about", "A reading.", ids[1:], ids[:1], nil), nil)
	if status != http.StatusUnprocessableEntity || !strings.Contains(body, "Not saved.") ||
		!strings.Contains(body, "A link of kind about has no from side") || !strings.Contains(body, "Move "+ids[0]+" to the to side") {
		t.Fatalf("about with a from side = %d, want 422 with the rule:\n%s", status, body)
	}
	if w := head(t, d); w != at {
		t.Fatalf("W moved from %d to %d on a refused link", at, w)
	}
	status, body = page(t, "POST", base+"/link", linkForm(v, "interpreted_from", "A reading.", ids[1:], ids[:1], ids), nil)
	if status != http.StatusOK || !strings.Contains(body, "admitted") {
		t.Fatalf("interpreted_from with a from side = %d:\n%s", status, body)
	}
}

// A pin is written only for a facet whose "link to this version" is ticked, and names
// its current content hash. Kind about starts with none ticked, another kind with all.
func TestPinsAreWrittenOnlyWhenTicked(t *testing.T) {
	d, base := workbenchServing(t, "record.edge")
	ids := weighIns(t, d, base, 2)
	v := edgeVersion(t, d)
	pin := regexp.MustCompile(`name="pin" value="[^"]+" data-pin( checked)?>`)
	for kind, want := range map[string]string{"about": "", "interpreted_from": " checked"} {
		_, form := page(t, "GET", base+"/link?"+url.Values{"tick": ids, "kind": {kind}}.Encode(), nil, nil)
		boxes := pin.FindAllStringSubmatch(form, -1)
		if len(boxes) != 2 {
			t.Fatalf("kind %s draws %d pin boxes, want 2", kind, len(boxes))
		}
		for _, b := range boxes {
			if b[1] != want {
				t.Errorf("kind %s draws a pin box %q, want checked=%q", kind, b[0], want)
			}
		}
	}

	page(t, "POST", base+"/link", linkForm(v, "about", "Unpinned.", ids, nil, nil), nil)
	_, unpinned := wantEdgeID("about", ids, nil)
	f, err := d.read.CurrentVersion(context.Background(), unpinned)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(f.Payload.Bytes()), `"pins"`) {
		t.Errorf("a link with no pin ticked wrote pins: %s", f.Payload.Bytes())
	}

	target, err := d.read.CurrentVersion(context.Background(), ids[0])
	if err != nil {
		t.Fatal(err)
	}
	pinned := linkForm(v, "about", "Pinned.", ids, nil, ids[:1])
	pinned.Set("at", "2026-10-21T14:31")
	if status, body := page(t, "POST", base+"/link", pinned, nil); status != http.StatusOK || !strings.Contains(body, "admitted") {
		t.Fatalf("pinned save = %d:\n%s", status, body)
	}
	res, err := fold.Read(context.Background(), d.read)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, e := range res.Live {
		p := string(e.Payload.Bytes())
		if e.Type != "record.edge" || !strings.Contains(p, "Pinned.") {
			continue
		}
		found = true
		if want := `"pins":{"` + ids[0] + `":"` + target.Descriptor.ContentHash + `"}`; !strings.Contains(p, want) {
			t.Errorf("payload %s lacks %s, and only it", p, want)
		}
	}
	if !found {
		t.Fatal("the pinned link was not written")
	}
}

// Without record.edge on the workbench's list the link form is refused and no page
// offers it; with it, the Definitions page's Enter for record.edge opens the link form.
func TestTheLinkFormNeedsRecordEdgeOnTheList(t *testing.T) {
	d, base := workbenchServing(t, "health.medication.dose")
	ids := weighIns(t, d, base, 1)
	if status, body := page(t, "GET", base+"/link?tick="+ids[0], nil, nil); status != http.StatusForbidden ||
		!strings.Contains(body, "importers file lists for the workbench principal") {
		t.Fatalf("GET /link off the list = %d, want 403 naming the importers file:\n%s", status, body)
	}
	if _, body := page(t, "GET", base+"/facet?id="+ids[0], nil, nil); strings.Contains(body, "link this…") {
		t.Error("a facet page offers link this when links cannot be entered")
	}

	_, base = workbenchServing(t, "record.edge")
	_, defs := page(t, "GET", base+"/definitions", nil, nil)
	if !strings.Contains(defs, `<a href="/link">Enter</a>`) {
		t.Errorf("record.edge's Enter does not open the link form")
	}
	if status, _ := page(t, "GET", base+"/link", nil, nil); status != http.StatusOK {
		t.Errorf("GET /link with an empty basket = %d, want 200", status)
	}
}
