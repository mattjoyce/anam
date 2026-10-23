package projection_test

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"testing"

	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/projection"
)

// The two reasons a bundled type may go unserved. Anything else is a gap.
const (
	reasonRetired   = "retired"
	reasonOnPurpose = "unserved on purpose"
)

// unserved is every bundled type that no current projection serves, and why. It is
// short on purpose: a type that fits neither reason is a type an agent cannot query,
// and the fix is a projection, not a line here.
var unserved = map[facet.Type]string{
	// The health ETL wrote these and is retired (card 55); Garmin's own importer writes
	// health.heart-rate.day, health.sleep-episode and the rest. Their facts stay in the
	// record under their Definitions.
	"health.heart-rate":     reasonRetired,
	"health.metric-group":   reasonRetired,
	"health.sleep":          reasonRetired,
	"health.heart-rate-day": reasonRetired, // three facts a day, replaced by health.heart-rate.day

	// Documents are kept out of recent-life for size and sensitivity; a projection for
	// them is its own decision.
	"record.document": reasonOnPurpose,
}

// currentProjections is the projections as the live daemon holds them: each bundled
// template, replaced by the newest version of its name under projections/ when that
// is newer, which is what the operator registers.
func currentProjections(t *testing.T) []projection.Template {
	t.Helper()
	bundled, err := projection.Bundled()
	if err != nil {
		t.Fatal(err)
	}
	current := map[string]projection.Template{}
	for _, tpl := range bundled.All() {
		current[tpl.Name] = tpl
	}
	files, err := filepath.Glob(filepath.Join("..", "..", "projections", "*.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("projections/*.json: %v, %d files", err, len(files))
	}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		tpl, err := projection.Parse(raw)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		if want := tpl.Name + ".v" + strconv.Itoa(tpl.Version) + ".json"; filepath.Base(f) != want {
			t.Errorf("%s declares %s v%d, so should be named %s", f, tpl.Name, tpl.Version, want)
		}
		if cur, ok := current[tpl.Name]; !ok || tpl.Version > cur.Version {
			current[tpl.Name] = tpl
		}
	}
	out := make([]projection.Template, 0, len(current))
	for _, tpl := range current {
		out = append(out, tpl)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// A type with no projection cannot be reached by the query route, which runs through
// one, so an agent has only a whole-history read. Every bundled Definition's type is
// served by the current version of some projection, or is named in unserved with a
// reason.
func TestEveryBundledTypeIsServedOrNamed(t *testing.T) {
	t.Parallel()
	defs, err := def.Bundled()
	if err != nil {
		t.Fatal(err)
	}
	all := defs.All()
	served := map[facet.Type][]string{}
	for _, tpl := range currentProjections(t) {
		for _, typ := range tpl.Covers(all) {
			served[typ] = append(served[typ], tpl.Name)
		}
	}
	bundledTypes := map[facet.Type]bool{}
	for _, d := range all {
		bundledTypes[d.Type] = true
		reason, excepted := unserved[d.Type]
		switch {
		case len(served[d.Type]) == 0 && !excepted:
			t.Errorf("%s is served by no current projection and is not in the exception list", d.Type)
		case len(served[d.Type]) > 0 && excepted:
			t.Errorf("%s is in the exception list (%s) but %v serve it: remove the line", d.Type, reason, served[d.Type])
		}
	}
	for typ, reason := range unserved {
		if !bundledTypes[typ] {
			t.Errorf("exception %s is not a bundled type", typ)
		}
		if reason != reasonRetired && reason != reasonOnPurpose {
			t.Errorf("exception %s: reason %q is neither %q nor %q", typ, reason, reasonRetired, reasonOnPurpose)
		}
	}
}

// Card 231: the projections by concept serve the types an agent asks about, and the
// registered versions add the types an importer registers.
func TestProjectionsByConcept(t *testing.T) {
	t.Parallel()
	defs, err := def.Bundled()
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]projection.Template{}
	for _, tpl := range currentProjections(t) {
		byName[tpl.Name] = tpl
	}
	for _, tc := range []struct {
		name    string
		version int
		covers  []facet.Type
	}{
		{"body", 2, []facet.Type{"health.blood-pressure", "health.body-composition", "health.measurement.withings", "health.weight"}},
		{"activity", 2, []facet.Type{"activity.day.garmin", "activity.exercise-session.concept2", "activity.exercise-session.garmin",
			"activity.exercise-session.strong", "activity.exercise-session.technogym", "activity.gym-visit", "activity.steps"}},
		{"clinical", 1, []facet.Type{"health.condition", "health.donation", "health.encounter", "health.lab-panel",
			"health.lab-result", "health.vision-prescription"}},
		{"doses", 4, []facet.Type{"health.medication.compliance", "health.medication.dose", "health.medication.dose-missed"}},
		{"sleep", 5, []facet.Type{"health.cpap-session.dreamstation", "health.sleep-episode"}},
	} {
		tpl, ok := byName[tc.name]
		if !ok || tpl.Version != tc.version {
			t.Errorf("%s is v%d (held %v), want v%d", tc.name, tpl.Version, ok, tc.version)
			continue
		}
		if got := tpl.Covers(defs.All()); !reflect.DeepEqual(got, tc.covers) {
			t.Errorf("%s v%d covers %v, want %v", tc.name, tc.version, got, tc.covers)
		}
	}
	// The bundled floor names bundled types only: a template cannot name a type an
	// importer registers, and the registered version adds it.
	bundled, err := projection.Bundled()
	if err != nil {
		t.Fatal(err)
	}
	for _, tpl := range bundled.All() {
		for _, typ := range tpl.Types {
			if !defs.Has(typ) {
				t.Errorf("bundled %s v%d names %s, which has no bundled Definition", tpl.Name, tpl.Version, typ)
			}
		}
	}
	for _, name := range []string{"body", "activity"} {
		v1, err := bundled.Version(name, 1)
		if err != nil {
			t.Fatal(err)
		}
		v2 := byName[name]
		if !slices.Equal(v1.Prefixes, v2.Prefixes) || len(v2.Types) != len(v1.Types)+1 {
			t.Errorf("%s v2 should be v1 plus one type: v1 %v, v2 %v", name, v1.Patterns(), v2.Patterns())
		}
	}
}
