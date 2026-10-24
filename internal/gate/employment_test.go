package gate_test

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/consent"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/gate"
	"github.com/mattjoyce/anam/internal/projection"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// Card 239: the employment types over the bundled Definitions, a real gate and a real
// ledger. The employers here are made up. A start opens a stint, a change of role
// displaces the row before it, an end closes the stint, and a return to the same
// employer is a second stint, so the series is keyed by org and stint.

const (
	startedType = "org.employment.started"
	roleType    = "org.employment.role-changed"
	endedType   = "org.employment.ended"
)

func newEmploymentGate(t *testing.T) (*gate.Gate, *ledger.Ledger, *def.Registry) {
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
	scopes := gate.StaticScopes{"records": {startedType, roleType, endedType}}
	g := gate.New(l, reg, scopes, clock.Real{}, person, gate.Limits{MaxPayloadBytes: config.DefaultMaxPayloadBytes, MaxRetries: config.DefaultCommitRetries})
	return g, l, reg
}

func sayEmployment(t *testing.T, g *gate.Gate, typ, payload string) (gate.Result, error) {
	t.Helper()
	env := facet.Envelope{Type: facet.Type(typ), Source: "records", Injector: "records", Payload: []byte(payload)}
	b, err := g.Resolve(env)
	if err != nil {
		return gate.Result{}, err
	}
	res, _, err := g.Commit(context.Background(), env, b)
	return res, err
}

// employmentClock is source_time for a date read in Sydney with nothing behind the zone
// but the Definition's own.
func employmentClock(value string) string {
	return fmt.Sprintf(`"source_time": {"value": %q, "zone": "Australia/Sydney", "basis": "assumption"}`, value)
}

// employmentRow is a payload with the keys every employment type shares, then body (a JSON
// fragment without braces, possibly empty) and a source_time that restates the date.
func employmentRow(org string, stint int, date, precision, body string) string {
	if body != "" {
		body += ", "
	}
	return fmt.Sprintf(`{"org": %q, "stint": %d, "date": %q, "date_precision": %q, %s%s}`,
		org, stint, date, precision, body, employmentClock(date))
}

func TestEmployment_EachTypeAdmitsAGoodPayloadAndRefusesABadOne(t *testing.T) {
	t.Parallel()
	const org = "example-health"
	title := `"title": "Manager, Digital Innovation"`
	for _, tc := range []struct {
		name, typ, payload string
		ok                 bool
	}{
		{"a start", startedType, employmentRow(org, 1, "2019-09-30", "day", title), true},
		{"a start with everything the writer knows", startedType, employmentRow(org, 1, "2019-09-30", "day",
			title+`, "employer": "Example Health", "legal_entity": "Example Health Care Ltd", "location": "National Office, Lakeport", "reports_to": "Chief Technology Officer", "grade": "Band 7", "terms": "permanent full-time", "basis": "letter of offer of 12 Aug 2019", "source_note": "read from the letter"`), true},
		{"a start known to the month, on its first day", startedType, employmentRow(org, 1, "2019-09-01", "month", title), true},
		{"a start known to the year, on 1 January", startedType, employmentRow(org, 1, "2019-01-01", "year", title), true},
		{"a month on the 15th", startedType, employmentRow(org, 1, "2019-09-15", "month", title), false},
		{"a year on 1 February", startedType, employmentRow(org, 1, "2019-02-01", "year", title), false},
		{"a year on 2 January", startedType, employmentRow(org, 1, "2019-01-02", "year", title), false},
		{"a precision nobody named", startedType, employmentRow(org, 1, "2019-09-30", "week", title), false},
		{"a date that is no day", startedType, employmentRow(org, 1, "2019-02-30", "day", title), false},
		{"a date with a time", startedType, employmentRow(org, 1, "2019-09-30 09:00", "day", title), false},
		{"an org with spaces", startedType, employmentRow("Example Health", 1, "2019-09-30", "day", title), false},
		{"an org in capitals", startedType, employmentRow("Example-Health", 1, "2019-09-30", "day", title), false},
		{"an org that is the legal entity", startedType, employmentRow("example health care ltd.", 1, "2019-09-30", "day", title), false},
		{"a stint of zero", startedType, employmentRow(org, 0, "2019-09-30", "day", title), false},
		{"a fractional stint", startedType, strings.Replace(employmentRow(org, 1, "2019-09-30", "day", title), `"stint": 1`, `"stint": 1.5`, 1), false},
		{"a stint as text", startedType, strings.Replace(employmentRow(org, 1, "2019-09-30", "day", title), `"stint": 1`, `"stint": "1"`, 1), false},
		{"a start without a title", startedType, employmentRow(org, 1, "2019-09-30", "day", ""), false},
		{"a start without a source_time", startedType, fmt.Sprintf(`{"org": %q, "stint": 1, "date": "2019-09-30", "date_precision": "day", %s}`, org, title), false},
		{"a start without a precision", startedType, fmt.Sprintf(`{"org": %q, "stint": 1, "date": "2019-09-30", %s, %s}`, org, title, employmentClock("2019-09-30")), false},
		{"a salary", startedType, employmentRow(org, 1, "2019-09-30", "day", title+`, "salary": 120000`), false},
		{"a pay rate", startedType, employmentRow(org, 1, "2019-09-30", "day", title+`, "pay": "120k"`), false},
		{"a bonus on a role change", roleType, employmentRow(org, 1, "2022-04-01", "day", title+`, "bonus": 5000`), false},
		{"an empty title", startedType, employmentRow(org, 1, "2019-09-30", "day", `"title": ""`), false},
		{"a role change", roleType, employmentRow(org, 1, "2022-04-01", "day", `"title": "Director", "grade": "Band 8", "reports_to": "Chief Executive"`), true},
		{"a role change without a title", roleType, employmentRow(org, 1, "2022-04-01", "day", `"grade": "Band 8"`), false},
		{"a role change with an employer", roleType, employmentRow(org, 1, "2022-04-01", "day", `"title": "Director", "employer": "Example Health"`), false},
		{"an end", endedType, employmentRow(org, 1, "2025-07-01", "day", `"basis": "separation certificate, last day 30 June"`), true},
		{"an end known to the month", endedType, employmentRow(org, 1, "2025-06-01", "month", ""), true},
		{"an end on the 30th of a month", endedType, employmentRow(org, 1, "2025-06-30", "month", ""), false},
		{"an end with a title", endedType, employmentRow(org, 1, "2025-06-30", "day", `"title": "Director"`), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g, l, _ := newEmploymentGate(t)
			res, err := sayEmployment(t, g, tc.typ, tc.payload)
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

// The id is declared, so a writer can leave it out, and one that sends a different id is
// refused. A start and an end are one each per stint; a role change is one per date.
func TestEmployment_TheIDIsTheOrgTheStintAndTheKind(t *testing.T) {
	t.Parallel()
	g, _, _ := newEmploymentGate(t)
	started, err := sayEmployment(t, g, startedType, employmentRow("example-health", 1, "2019-09-30", "day", `"title": "Manager"`))
	if err != nil {
		t.Fatal(err)
	}
	role, err := sayEmployment(t, g, roleType, employmentRow("example-health", 1, "2022-04-01", "day", `"title": "Director"`))
	if err != nil {
		t.Fatal(err)
	}
	ended, err := sayEmployment(t, g, endedType, employmentRow("example-health", 1, "2025-06-30", "day", ""))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		got  string
		want string
	}{
		{"started", started.FacetID, "records:employment/example-health/1/started"},
		{"role-changed", role.FacetID, "records:employment/example-health/1/role-changed/2022-04-01"},
		{"ended", ended.FacetID, "records:employment/example-health/1/ended"},
	} {
		if want := facet.NewID(facet.SourceRef("records", tc.want)); tc.got != want {
			t.Errorf("%s: facet id %s, want the one for %s", tc.name, tc.got, tc.want)
		}
	}
	wrong := strings.Replace(employmentRow("example-health", 1, "2019-09-30", "day", `"title": "Manager"`), `{`, `{"id": "records:employment/example-health/2/started", `, 1)
	if res, err := sayEmployment(t, g, startedType, wrong); err == nil {
		t.Errorf("a start carrying another id was admitted: %+v", res)
	}
	again, err := sayEmployment(t, g, startedType, employmentRow("example-health", 1, "2019-09-30", "day", `"title": "Manager"`))
	if err != nil || again.Outcome != gate.OutcomeNoOp || again.FacetID != started.FacetID {
		t.Errorf("saying the start again = %+v, %v; want a no-op on %s", again, err, started.FacetID)
	}
}

// A person starts, changes role, ends, rejoins and works for a second employer in the
// meantime. The employment snapshot, read at a day, names the rows then in force, and a
// stint whose end has come is gone from it. The career log keeps all of them.
func TestEmployment_AnEndClosesItsStintAndACareerLogKeepsEverything(t *testing.T) {
	t.Parallel()
	g, l, reg := newEmploymentGate(t)
	say := func(typ, org string, stint int, date, body string) string {
		t.Helper()
		res, err := sayEmployment(t, g, typ, employmentRow(org, stint, date, "day", body))
		if err != nil || res.Outcome != gate.OutcomeAdmitted {
			t.Fatalf("%s %s/%d %s: %+v, %v", typ, org, stint, date, res, err)
		}
		return res.FacetID
	}
	start1 := say(startedType, "example-health", 1, "2019-09-30", `"title": "Manager"`)
	role1 := say(roleType, "example-health", 1, "2022-04-01", `"title": "Director"`)
	side := say(startedType, "example-labs", 1, "2021-02-01", `"title": "Adviser"`)
	end1 := say(endedType, "example-health", 1, "2025-07-01", `"basis": "separation certificate, last day 30 June"`)
	start2 := say(startedType, "example-health", 2, "2026-02-02", `"title": "Consultant"`)

	res, err := fold.Read(context.Background(), l)
	if err != nil {
		t.Fatal(err)
	}
	bundled, err := projection.Bundled()
	if err != nil {
		t.Fatal(err)
	}
	employment, err := bundled.Lookup("employment")
	if err != nil {
		t.Fatal(err)
	}
	// To is exclusive, so the midnight after a day, in Sydney, is the day's bound.
	for _, tc := range []struct {
		name, to string
		want     []string
	}{
		{"before the first start", "2019-09-29T14:00:00.000000000Z", nil},
		{"on the first day", "2019-09-30T14:00:00.000000000Z", []string{start1}},
		{"after the first start", "2020-06-01T00:00:00.000000000Z", []string{start1}},
		{"with a second employer", "2021-06-01T00:00:00.000000000Z", []string{start1, side}},
		{"after the change of role, which displaces the start", "2023-05-10T14:00:00.000000000Z", []string{side, role1}},
		{"on the last day worked, still employed", "2025-06-30T14:00:00.000000000Z", []string{side, role1}},
		{"on the first day not employed, the stint is gone", "2025-07-01T14:00:00.000000000Z", []string{side}},
		{"a second stint is its own series", "2026-06-01T00:00:00.000000000Z", []string{side, start2}},
	} {
		tmpl := employment
		tmpl.To = tc.to
		served, err := projection.Apply(res.Value, tmpl, consent.Open{}, reg)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		var got []string
		for _, e := range served.Entries() {
			got = append(got, e.FacetID)
		}
		if !sameIDs(got, tc.want) {
			t.Errorf("%s: employment = %v, want %v", tc.name, got, tc.want)
		}
	}

	career, err := bundled.Lookup("career")
	if err != nil {
		t.Fatal(err)
	}
	served, err := projection.Apply(res.Value, career, consent.Open{}, reg)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range served.Entries() {
		got = append(got, e.FacetID)
	}
	if want := []string{start1, side, role1, end1, start2}; !sameIDs(got, want) {
		t.Errorf("career = %v, want every start, change and end %v", got, want)
	}
}

// sameIDs reports whether a and b hold the same ids, whatever the order: a snapshot is a
// set of rows, and which comes first is the canonical's business.
func sameIDs(a, b []string) bool {
	x, y := map[string]bool{}, map[string]bool{}
	for _, s := range a {
		x[s] = true
	}
	for _, s := range b {
		y[s] = true
	}
	return reflect.DeepEqual(x, y) && len(a) == len(b)
}
