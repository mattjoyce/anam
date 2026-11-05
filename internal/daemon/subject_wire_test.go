package daemon

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/facet"
)

// assertedReading is a test.reading carrying the asserted fields given as JSON
// members (`"asserted_subject": "matt"`), or none when fields is empty.
func assertedReading(id string, version int, fields string) string {
	extra := ""
	if fields != "" {
		extra = ", " + fields
	}
	return fmt.Sprintf(`{"type": "test.reading", "definition_version": %d,
		"payload": {"id": %q, "at": "2026-10-09 07:30", "source_time": {"value": "2026-10-09 07:30", "zone": "Australia/Sydney", "offset": "+11:00", "basis": "source"}, "value": 80.5, "unit": "kg"}%s}`, version, id, extra)
}

// Card 270a, slice 2: an envelope names who it is about as asserted_subject, and
// asserted_person is read as its old name until a later release removes it. Either
// name binds the same way, the two set to different values refuse the batch, and the
// gate's three assurance arms are unchanged. Read from the facet row, as the gate
// wrote it, on a daemon whose subject is matt.
func TestEnvelopes_AssertedSubjectAndItsOldName(t *testing.T) {
	t.Parallel()
	d, url, _ := serveImporters(t, importerConfig(t))
	reading, _ := d.defs.current.Load().Lookup("test.reading")
	v := reading.Version

	admitted := []struct {
		name, fields string
		want         facet.BindingAssurance
	}{
		{"old name", `"asserted_person": "matt"`, facet.AssuranceHigh},
		{"new name", `"asserted_subject": "matt"`, facet.AssuranceHigh},
		{"both, equal", `"asserted_subject": "matt", "asserted_person": "matt"`, facet.AssuranceHigh},
		{"neither", ``, facet.AssuranceMedium},
		// An explicit empty string or null is an absent assertion, under either name
		// and beside the other: a client that always sends the key is not refused.
		{"old name empty", `"asserted_person": ""`, facet.AssuranceMedium},
		{"old name null", `"asserted_person": null`, facet.AssuranceMedium},
		{"new name empty", `"asserted_subject": ""`, facet.AssuranceMedium},
		{"new name null", `"asserted_subject": null`, facet.AssuranceMedium},
		{"both empty", `"asserted_subject": "", "asserted_person": ""`, facet.AssuranceMedium},
		{"both null", `"asserted_subject": null, "asserted_person": null`, facet.AssuranceMedium},
		{"new empty, old set", `"asserted_subject": "", "asserted_person": "matt"`, facet.AssuranceHigh},
		{"new null, old set", `"asserted_subject": null, "asserted_person": "matt"`, facet.AssuranceHigh},
		{"new set, old empty", `"asserted_subject": "matt", "asserted_person": ""`, facet.AssuranceHigh},
		{"new set, old null", `"asserted_subject": "matt", "asserted_person": null`, facet.AssuranceHigh},
	}
	for i, tc := range admitted {
		var a api.Admission
		call(t, "POST", url+"/v1/envelopes", importerToken,
			batch(assertedReading(fmt.Sprintf("w-%d", i), v, tc.fields)), http.StatusOK, &a)
		if len(a.Results) != 1 || a.Results[0].Outcome != "admitted" {
			t.Fatalf("%s: admission = %+v", tc.name, a)
		}
		f, err := d.read.CurrentVersion(context.Background(), a.Results[0].FacetID)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if f.Binding.Assurance != tc.want || f.Binding.SubjectID != "matt" {
			t.Errorf("%s: binding = %+v, want %s for matt", tc.name, f.Binding, tc.want)
		}
	}

	// Another subject asserted is the review arm under either name, and review is
	// below test.reading's floor, so the gate refuses it (422) as it always has.
	for _, fields := range []string{`"asserted_person": "other"`, `"asserted_subject": "other"`} {
		before := head(t, d)
		var r api.Refusal
		call(t, "POST", url+"/v1/envelopes", importerToken,
			batch(assertedReading("w-other", v, fields)), http.StatusUnprocessableEntity, &r)
		if len(r.Refused) != 1 || !strings.Contains(r.Refused[0].Reason, "review") {
			t.Errorf("%s: refusal = %+v, want the review arm below the floor", fields, r)
		}
		if got := head(t, d); got != before {
			t.Errorf("%s: W = %d, want %d", fields, got, before)
		}
	}

	// The two names set differently refuse the whole batch before the gate, naming
	// both fields and both values, whichever way round.
	for _, tc := range []struct{ subject, person string }{{"matt", "other"}, {"other", "matt"}} {
		before := head(t, d)
		fields := fmt.Sprintf(`"asserted_subject": %q, "asserted_person": %q`, tc.subject, tc.person)
		var e api.Error
		call(t, "POST", url+"/v1/envelopes", importerToken,
			batch(assertedReading("w-ok", v, `"asserted_subject": "matt"`), assertedReading("w-differ", v, fields)),
			http.StatusBadRequest, &e)
		want := fmt.Sprintf(`envelope 1 (test.reading): asserted_subject (%q) and its old name asserted_person (%q) differ; send asserted_subject only`,
			tc.subject, tc.person)
		if !strings.Contains(e.Error, want) {
			t.Errorf("refusal = %q, want it to hold %q", e.Error, want)
		}
		if got := head(t, d); got != before {
			t.Errorf("W = %d after a refused batch, want %d", got, before)
		}
	}
}

// The same envelope sent under either name is the same facet: the same id and the same
// content hash, on two daemons that each saw one name. The asserted id is a binding
// input, not part of the hashed bytes, and the field's name is in neither.
func TestEnvelopes_SameFacetUnderEitherAssertedName(t *testing.T) {
	t.Parallel()
	var got []facet.Facet
	for _, fields := range []string{`"asserted_person": "matt"`, `"asserted_subject": "matt"`} {
		d, url, _ := serveImporters(t, importerConfig(t))
		reading, _ := d.defs.current.Load().Lookup("test.reading")
		var a api.Admission
		call(t, "POST", url+"/v1/envelopes", importerToken,
			batch(assertedReading("w-1", reading.Version, fields)), http.StatusOK, &a)
		f, err := d.read.CurrentVersion(context.Background(), a.Results[0].FacetID)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, f)
	}
	if got[0].ID != got[1].ID || got[0].Descriptor.ContentHash != got[1].Descriptor.ContentHash || got[0].Binding != got[1].Binding {
		t.Fatalf("the two names made different facets:\n%+v\n%+v", got[0], got[1])
	}
}

// decodeOne decodes body as test-import's submission.
func decodeOne(t *testing.T, body string) []facet.Envelope {
	t.Helper()
	r := httptest.NewRequest("POST", "/v1/envelopes", strings.NewReader(body))
	envs, _, err := decodeSubmission(httptest.NewRecorder(), r, config.Principal{Source: "test-import"}, config.DefaultLimits())
	if err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	return envs
}

// A snapshot part's digest is the same whichever name carried the asserted id, so a
// complete resend that straddles an importer's switch to asserted_subject sees no
// changed part. The digest is also the one this body had before the new name existed:
// the literal is partDigest at a8b089a, where the field was AssertedPerson.
func TestPartDigest_SameUnderEitherAssertedName(t *testing.T) {
	t.Parallel()
	const before = "sha256:6af1ab6133e30db8c1c8ded128fd2b1e0625be54ab06217217bb032795fb5ef8"
	old := partDigest(decodeOne(t, batch(assertedReading("w-1", 1, `"asserted_person": "matt"`))))
	renamed := partDigest(decodeOne(t, batch(assertedReading("w-1", 1, `"asserted_subject": "matt"`))))
	both := partDigest(decodeOne(t, batch(assertedReading("w-1", 1, `"asserted_subject": "matt", "asserted_person": "matt"`))))
	if old != before || renamed != before || both != before {
		t.Fatalf("partDigest: old %s, new %s, both %s; want each %s", old, renamed, both, before)
	}
	// The digest still covers the asserted value: another id is another part.
	if other := partDigest(decodeOne(t, batch(assertedReading("w-1", 1, `"asserted_subject": "other"`)))); other == before {
		t.Fatalf("partDigest ignores the asserted id: %s", other)
	}
}

// /v1/status answers subject, and person beside it with the same value until the
// release that removes the old name.
func TestStatus_AnswersSubjectAndItsOldName(t *testing.T) {
	t.Parallel()
	_, url, _ := serveImporters(t, importerConfig(t))
	var raw map[string]any
	call(t, "GET", url+"/v1/status", testToken, "", http.StatusOK, &raw)
	if raw["subject"] != "matt" || raw["person"] != "matt" {
		t.Fatalf("status subject = %v, person = %v; want both matt", raw["subject"], raw["person"])
	}
}
