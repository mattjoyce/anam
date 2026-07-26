package consent_test

import (
	"reflect"
	"testing"

	"github.com/mattjoyce/anam/internal/consent"
	"github.com/mattjoyce/anam/internal/facet"
)

// The gate is default-allow today (ARCHITECTURE.md §10). This test exists to pin that
// as a deliberate state rather than an unfinished one: when a policy arrives, this is
// the test that has to change, and changing it is the announcement.
func TestOpen_AllowsEverything(t *testing.T) {
	t.Parallel()

	classifications := []facet.Classification{
		facet.ClassificationNormal,
		facet.ClassificationConfidential,
		facet.ClassificationSensitive,
	}
	statuses := []facet.Status{facet.StatusAdmitted, facet.StatusEndorsed}

	for _, c := range classifications {
		for _, s := range statuses {
			req := consent.Request{
				Type:           "health.blood-pressure",
				Classification: c,
				Status:         s,
				FactTime:       "2026-01-24T06:36:00.000000000Z",
			}
			got := consent.Open{}.Evaluate(req)
			if !got.Allow {
				t.Errorf("Evaluate(%s/%s) denied: %q", c, s, got.Reason)
			}
			if got.Reason != "" {
				t.Errorf("Evaluate(%s/%s) allowed but gave a reason %q", c, s, got.Reason)
			}
		}
	}
}

// Request has nowhere to put a payload, so "filter on metadata only, never the
// payload" (§9) is a property of the type rather than a rule a future policy author
// has to remember. This asserts that structurally: a field added later fails here.
func TestRequest_CarriesNoPayload(t *testing.T) {
	t.Parallel()

	want := []string{"Type", "Classification", "Status", "FactTime"}
	rt := reflect.TypeOf(consent.Request{})

	var got []string
	for i := range rt.NumField() {
		got = append(got, rt.Field(i).Name)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("consent.Request fields = %v, want %v — a consent gate must see governing metadata and nothing else (§9)", got, want)
	}
}
