package inspect_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/inspect"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// The index lists every bundled type with its versions and how many fields are
// described, and each page carries the phone viewport.
func TestDefinitions_IndexListsEveryType(t *testing.T) {
	t.Parallel()
	srv, _, _ := seededServer(t)
	code, body := get(t, srv.URL+"/definitions")
	if code != http.StatusOK {
		t.Fatalf("GET /definitions = %d", code)
	}
	for _, want := range []string{
		`href="/definition?type=health.blood-pressure"`,
		`href="/definition?type=exercise.session"`,
		` of `, // the described count, "0 of 15"
		`name="viewport"`,
		`href="/definitions"`, // the shared nav
	} {
		if !strings.Contains(body, want) {
			t.Errorf("/definitions lacks %q", want)
		}
	}
}

// A Definition page puts the meta's pointers on the rows they point at, states the
// field count, and shows the manifest colour-coded.
func TestDefinition_PageMarksRolesOnTheirRows(t *testing.T) {
	t.Parallel()
	srv, _, _ := seededServer(t)
	code, body := get(t, srv.URL+"/definition?type=exercise.session")
	if code != http.StatusOK {
		t.Fatalf("GET /definition = %d: %s", code, body)
	}
	for _, want := range []string{
		`id="f/effective_time_frame/time_interval/start_date_time"`, // a nested row, keyed by pointer
		`<span class="role">fact time</span>`,
		`<span class="role">identity</span>`,
		`described</span>`,
		`class="j-key"`,
		`href="/?type=exercise.session"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("/definition?type=exercise.session lacks %q", want)
		}
	}
	// blood pressure writes its numbers as text: the type says so, once, not as a regex.
	_, bp := get(t, srv.URL+"/definition?type=health.blood-pressure")
	if !strings.Contains(bp, "number as text") {
		t.Error("blood pressure's numeric strings are not named as number as text")
	}
}

func TestDefinition_Refusals(t *testing.T) {
	t.Parallel()
	srv, _, _ := seededServer(t)
	for _, tc := range []struct {
		q    string
		want int
	}{
		{"", http.StatusBadRequest},
		{"?type=no.such-type", http.StatusNotFound},
		{"?type=health.blood-pressure&v=x", http.StatusBadRequest},
		{"?type=health.blood-pressure&v=99", http.StatusNotFound},
	} {
		if code, _ := get(t, srv.URL+"/definition"+tc.q); code != tc.want {
			t.Errorf("GET /definition%s = %d, want %d", tc.q, code, tc.want)
		}
	}
}

// With an older version held, the page shows it as it was and says which is current;
// a body this build cannot parse is still shown, raw.
func TestDefinition_AnOlderVersionIsShownAsHeld(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	l, err := ledger.Create(ctx, filepath.Join(t.TempDir(), "ledger.db"), person)
	if err != nil {
		t.Fatalf("ledger.Create: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })

	current := bundledBody(t, "health.blood-pressure")
	cur, err := def.Parse(current)
	if err != nil {
		t.Fatalf("def.Parse: %v", err)
	}
	held := func(context.Context) (map[facet.Type]map[int][]byte, map[facet.Type]string, error) {
		return map[facet.Type]map[int][]byte{
				"health.blood-pressure": {
					1:           []byte(`{"meta": {"facet_type": "health.blood-pressure", "definition_version": 1, "<b>": 1}}`),
					cur.Version: current,
				},
			}, map[facet.Type]string{"health.blood-pressure": "bundled"},
			nil
	}
	srv := httptest.NewServer(inspect.Handler(inspect.Deps{Ledger: l, Timeout: 30 * time.Second, Buckets: defaultBuckets, Held: held}))
	t.Cleanup(srv.Close)

	code, body := get(t, srv.URL+"/definition?type=health.blood-pressure&v=1")
	if code != http.StatusOK {
		t.Fatalf("GET v1 = %d", code)
	}
	for _, want := range []string{fmt.Sprintf("v%d is current", cur.Version), "cannot parse this version", "&lt;b&gt;"} {
		if !strings.Contains(body, want) {
			t.Errorf("v1 page lacks %q", want)
		}
	}
	if strings.Contains(body, `"<b>"`) {
		t.Error("the held body reached the page unescaped")
	}
}

// A facet links to the Definition it was admitted under, says when a newer one is
// current, and shows its payload colour-coded.
func TestFacet_LinksToItsAdmittedDefinition(t *testing.T) {
	t.Parallel()
	srv, bp, _ := seededServer(t)
	_, body := get(t, srv.URL+"/facet?id="+bp)
	for _, want := range []string{
		`href="/definition?type=health.blood-pressure&amp;v=1"`,
		` is current</a>`,
		`<span class="j-key">&#34;systolic&#34;</span>`,
		`<span class="j-str">&#34;Before breakfast&#34;</span>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("facet page lacks %q", want)
		}
	}
}

// bundledBody is the bundled manifest of t, as the binary carries it.
func bundledBody(t *testing.T, typ facet.Type) []byte {
	t.Helper()
	ms, err := def.Manifests()
	if err != nil {
		t.Fatalf("def.Manifests: %v", err)
	}
	for _, m := range ms {
		if m.Type == typ {
			return m.Body
		}
	}
	t.Fatalf("no bundled manifest for %s", typ)
	return nil
}
