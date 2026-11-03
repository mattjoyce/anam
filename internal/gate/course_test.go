package gate_test

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/mattjoyce/anam/internal/catalog/catalogtest"
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

// Cards 222 and 239: a series over the engine fixture's course types, a real gate and a
// real ledger. A start opens a course, a change displaces the row before it, a stop
// closes the course, a later start of the same course opens it again, and another
// course is a series of its own, since the series is keyed by name and course. Each
// row's fact time is its date at midnight in Berlin.

const (
	courseStarted = "test.course.started"
	courseChanged = "test.course.changed"
	courseStopped = "test.course.stopped"
)

func newCourseGate(t *testing.T) (*gate.Gate, *ledger.Ledger, *def.Registry) {
	t.Helper()
	l, err := ledger.Create(context.Background(), filepath.Join(t.TempDir(), "ledger.db"), person)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	reg := catalogtest.Engine.Definitions(t)
	scopes := gate.StaticScopes{"records": {courseStarted, courseChanged, courseStopped}}
	g := gate.New(l, reg, scopes, clock.Real{}, person, gate.Limits{MaxPayloadBytes: config.DefaultMaxPayloadBytes, MaxRetries: config.DefaultCommitRetries})
	return g, l, reg
}

// sayCourse admits one course row, its id named by the type, name, course and date, and
// returns its facet id. A stop has no dose.
func sayCourse(t *testing.T, g *gate.Gate, typ, name string, course int, date string) string {
	t.Helper()
	dose := `, "dose": "5 mg"`
	if typ == courseStopped {
		dose = ""
	}
	payload := fmt.Sprintf(`{"id": "%s/%s/%d/%s", "date": %q, "name": %q, "course": %d%s}`, typ, name, course, date, date, name, course, dose)
	env := facet.Envelope{Type: facet.Type(typ), Source: "records", Injector: "records", Payload: []byte(payload)}
	b, err := g.Resolve(env)
	if err != nil {
		t.Fatalf("%s: %v", payload, err)
	}
	res, _, err := g.Commit(context.Background(), env, b)
	if err != nil || res.Outcome != gate.OutcomeAdmitted {
		t.Fatalf("%s: %+v, %v", payload, res, err)
	}
	return res.FacetID
}

// A stop closes the series of its course's latest start, a later start opens it again,
// and another name's series is its own.
func TestCourse_AStopClosesItsStartAndALaterStartReopensIt(t *testing.T) {
	t.Parallel()
	g, l, reg := newCourseGate(t)
	started1 := sayCourse(t, g, courseStarted, "deskcam", 1, "2026-10-20")
	garden := sayCourse(t, g, courseStarted, "garden", 1, "2026-10-20")
	sayCourse(t, g, courseStopped, "deskcam", 1, "2026-10-21")
	started2 := sayCourse(t, g, courseStarted, "deskcam", 1, "2026-10-23")

	res, err := fold.Read(context.Background(), l)
	if err != nil {
		t.Fatal(err)
	}
	// A date is Berlin's midnight, at +02:00; a bound is the UTC instant.
	for _, tc := range []struct {
		name, at string
		want     []string
	}{
		{"before anything", "2026-10-19T00:00:00.000000000Z", nil},
		{"both open", "2026-10-19T22:00:00.000000000Z", []string{started1, garden}},
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
		if !sameIDs(got, tc.want) {
			t.Errorf("%s: open = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// Two courses of one name and one of another. The course snapshot, read at a day, names
// the rows then in force, and a course whose stop has come is gone from it. The
// everything log keeps all of them.
func TestCourse_AStopClosesItsCourseAndALogKeepsEverything(t *testing.T) {
	t.Parallel()
	g, l, reg := newCourseGate(t)
	start1 := sayCourse(t, g, courseStarted, "ramipril", 1, "2026-06-01")
	side := sayCourse(t, g, courseStarted, "vitamin", 1, "2026-06-10")
	changed1 := sayCourse(t, g, courseChanged, "ramipril", 1, "2026-07-01")
	stop1 := sayCourse(t, g, courseStopped, "ramipril", 1, "2026-08-01")
	start2 := sayCourse(t, g, courseStarted, "ramipril", 2, "2026-09-01")

	res, err := fold.Read(context.Background(), l)
	if err != nil {
		t.Fatal(err)
	}
	projs := catalogtest.Engine.Projections(t)
	course, err := projs.Lookup("course")
	if err != nil {
		t.Fatal(err)
	}
	if course.Shape != "snapshot" {
		t.Fatalf("course is a %s, want a snapshot", course.Shape)
	}
	// To is exclusive, so the midnight after a day, in Berlin, is the day's bound.
	for _, tc := range []struct {
		name, to string
		want     []string
	}{
		{"before the first start", "2026-05-31T22:00:00.000000000Z", nil},
		{"on the first day", "2026-06-01T22:00:00.000000000Z", []string{start1}},
		{"with a second name", "2026-06-15T00:00:00.000000000Z", []string{start1, side}},
		{"after the change, which displaces the start", "2026-07-15T00:00:00.000000000Z", []string{side, changed1}},
		{"on the last day before the stop, still open", "2026-07-31T22:00:00.000000000Z", []string{side, changed1}},
		{"on the day of the stop, the course is gone", "2026-08-01T22:00:00.000000000Z", []string{side}},
		{"a second course is its own series", "2026-09-15T00:00:00.000000000Z", []string{side, start2}},
	} {
		tmpl := course
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
			t.Errorf("%s: course = %v, want %v", tc.name, got, tc.want)
		}
	}

	everything, err := projs.Lookup("everything")
	if err != nil {
		t.Fatal(err)
	}
	served, err := projection.Apply(res.Value, everything, consent.Open{}, reg)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range served.Entries() {
		got = append(got, e.FacetID)
	}
	if want := []string{start1, side, changed1, stop1, start2}; !sameIDs(got, want) {
		t.Errorf("everything = %v, want every start, change and stop %v", got, want)
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
