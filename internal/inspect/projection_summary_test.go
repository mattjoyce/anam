package inspect_test

import (
	"context"
	"fmt"
	"html"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/inspect"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// notLiveType is the summary's line for facets that are no longer live.
const notLiveType = "(not live)"

// summaryLine is one type's line of the summary: each count, and the address that
// count opens when it is not zero.
type summaryLine struct {
	served, excluded, superseded int
	link                         string // the superseded link, for the (not live) line
	links                        map[string]string
}

var (
	summaryRow = regexp.MustCompile(`(?s)<tr>\s*<td>([^<]+)</td>\s*<td class="num">(.*?)</td>\s*<td class="num">(.*?)</td>\s*<td class="num">(.*?)</td>\s*</tr>`)
	cellCount  = regexp.MustCompile(`>(\d+)<`)
	cellLink   = regexp.MustCompile(`href="([^"]+)"`)
)

// summaryCounts reads the summary table off the page.
func summaryCounts(t *testing.T, body string) map[string]summaryLine {
	t.Helper()
	out := map[string]summaryLine{}
	for _, m := range summaryRow.FindAllStringSubmatch(body, -1) {
		line := summaryLine{links: map[string]string{}}
		for i, standing := range []string{"served", "excluded", "superseded"} {
			cell := m[2+i]
			n := 0
			if c := cellCount.FindStringSubmatch(cell); c != nil {
				n, _ = strconv.Atoi(c[1])
			}
			if l := cellLink.FindStringSubmatch(cell); l != nil {
				line.links[standing] = html.UnescapeString(l[1])
			}
			switch standing {
			case "served":
				line.served = n
			case "excluded":
				line.excluded = n
			default:
				line.superseded = n
				line.link = line.links[standing]
			}
		}
		out[html.UnescapeString(m[1])] = line
	}
	return out
}

var rowsClaim = regexp.MustCompile(`(\d+) rows?`)

// rowsDrawn is how many facet rows the page draws.
func rowsDrawn(body string) int { return strings.Count(body, `href="/facet?id=`) }

// rowsClaimed is the total the rows view says match.
func rowsClaimed(t *testing.T, body string) int {
	t.Helper()
	i := strings.Index(body, `<p class="meta">`)
	if i < 0 {
		t.Fatalf("no rows header on the page:\n%s", body)
	}
	m := rowsClaim.FindStringSubmatch(body[i:])
	if m == nil {
		t.Fatalf("rows header does not give a count:\n%s", body[i:])
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

// The count in each cell is a link, and the link opens exactly that many rows, for
// every projection and every standing. The summary and the rows are one answer.
func TestProjectionSummary_CountsEqualTheRowsEachLinkOpens(t *testing.T) {
	t.Parallel()
	srv := projectionServer(t)

	for _, p := range []string{"canonical", "bp", "sleep"} {
		_, body := get(t, srv.URL+"/projection?p="+p)
		sum := summaryCounts(t, body)
		if len(sum) == 0 {
			t.Fatalf("%s: no summary table:\n%s", p, body)
		}
		if rowsDrawn(body) != 0 {
			t.Errorf("%s: the summary draws %d facet rows, want none", p, rowsDrawn(body))
		}

		opened := 0
		for typ, line := range sum {
			for standing, want := range map[string]int{"served": line.served, "excluded": line.excluded, "superseded": line.superseded} {
				link := line.links[standing]
				if want == 0 {
					if link != "" {
						t.Errorf("%s %s %s: a zero count links to rows", p, typ, standing)
					}
					continue
				}
				if link == "" {
					t.Errorf("%s %s %s: count %d has no link", p, typ, standing, want)
					continue
				}
				code, rows := get(t, srv.URL+link)
				if code != 200 {
					t.Fatalf("GET %s = %d", link, code)
				}
				if got := rowsDrawn(rows); got != want {
					t.Errorf("%s %s %s: count %d, link draws %d rows", p, typ, standing, want, got)
				}
				if got := rowsClaimed(t, rows); got != want {
					t.Errorf("%s %s %s: count %d, link says %d rows", p, typ, standing, want, got)
				}
				opened += want
			}
		}
		if opened == 0 {
			t.Errorf("%s: no count opened any rows", p)
		}
	}
}

// The fixture's numbers, spelled out: the live bp reading was corrected once, the live
// sleep period is out of the bp template, and the retracted heart rate is not live.
func TestProjectionSummary_CountsByType(t *testing.T) {
	t.Parallel()
	srv := projectionServer(t)

	_, body := get(t, srv.URL+"/projection?p=bp")
	got := summaryCounts(t, body)
	for typ, want := range map[string][3]int{
		"health.blood-pressure": {1, 0, 1},
		"health.sleep":          {0, 1, 0},
		notLiveType:             {0, 0, 1},
	} {
		g := got[typ]
		if [3]int{g.served, g.excluded, g.superseded} != want {
			t.Errorf("%s = served %d excluded %d superseded %d, want %v", typ, g.served, g.excluded, g.superseded, want)
		}
	}
}

// Superseded rows are read from the ledger in the rows view and nowhere else. The
// summary counts them from the fold's state, so a ledger that refuses to hand out
// older versions still gives the same summary.
type readCounter struct {
	*ledger.Ledger
	version, versions atomic.Int64
}

func (c *readCounter) Version(ctx context.Context, id string, v int) (facet.Facet, error) {
	c.version.Add(1)
	return c.Ledger.Version(ctx, id, v)
}

func (c *readCounter) Versions(ctx context.Context, refs []ledger.VersionRef) ([]facet.Facet, error) {
	c.versions.Add(1)
	return c.Ledger.Versions(ctx, refs)
}

func TestProjectionSummary_ReadsNoVersionsFromTheLedger(t *testing.T) {
	t.Parallel()
	l, err := seedProjectionLedger(filepath.Join(t.TempDir(), "ledger.db"))
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })
	c := &readCounter{Ledger: l}
	srv := httptest.NewServer(inspect.Handler(inspect.Deps{Ledger: c, Timeout: 30 * time.Second, Buckets: defaultBuckets, CachedReads: 4}))
	t.Cleanup(srv.Close)

	// The first read folds the head, which fetches the live versions in one batch.
	// That is the cached fold, and every later page shares it.
	get(t, srv.URL+"/projection")
	c.version.Store(0)
	c.versions.Store(0)

	for _, u := range []string{
		"/projection", "/projection?p=bp", "/projection?p=sleep",
		"/projection?p=bp&type=health.blood-pressure&show=served",
		"/projection?p=bp&type=health.sleep&show=excluded",
	} {
		if code, _ := get(t, srv.URL+u); code != 200 {
			t.Fatalf("GET %s = %d", u, code)
		}
	}
	if c.version.Load() != 0 || c.versions.Load() != 0 {
		t.Errorf("the summary and the served and excluded rows read %d single and %d batched versions, want none",
			c.version.Load(), c.versions.Load())
	}

	// An as-of summary folds the past once, in a batch, and still reads no older version.
	c.version.Store(0)
	get(t, srv.URL+"/projection?p=bp&asof=2026-07-13T00:00:00Z")
	if c.version.Load() != 0 {
		t.Errorf("an as-of summary read %d older versions, want none", c.version.Load())
	}

	// Only the superseded rows view reads them.
	get(t, srv.URL+"/projection?p=bp&type=health.blood-pressure&show=superseded")
	if c.version.Load() == 0 {
		t.Error("the superseded rows view read no versions, so it cannot have shown them")
	}
}

// seedBig admits n steps facets across days, one fact time each, plus a few of another
// type, and corrects every tenth step so the type has superseded versions too.
func seedBig(t *testing.T, n int) *httptest.Server {
	t.Helper()
	ctx := context.Background()
	l, err := ledger.Create(ctx, filepath.Join(t.TempDir(), "ledger.db"), person)
	if err != nil {
		t.Fatalf("ledger.Create: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })

	var seq int64
	admit := func(f facet.Facet, kind facet.AnamEventKind, prior int) {
		t.Helper()
		ev := facet.AnamEvent{
			Kind: kind, FacetID: f.ID, ArtifactVersion: f.ArtifactVersion, PriorArtifactVersion: prior,
			AnamTime: time.Date(2026, 7, 10, 9, 0, 0, int(seq+1), time.UTC), Seq: seq + 1,
		}
		if err := l.Admit(ctx, ledger.Watermark(seq), f, ev); err != nil {
			t.Fatalf("Admit %s: %v", f.ID, err)
		}
		seq++
	}
	base := time.Date(2026, 3, 1, 8, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ {
		f := seedFacetNoT(fmt.Sprintf("steps:%04d", i), "activity.steps", fmt.Sprintf(`{"count": %d}`, i))
		f.FactTime = base.AddDate(0, 0, i/50).Add(time.Duration(i%50) * time.Minute)
		admit(f, facet.EventAdmitted, 0)
		if i%10 == 0 {
			f.ArtifactVersion = 2
			f.Payload, _ = facet.Canonicalize([]byte(fmt.Sprintf(`{"count": %d}`, i+1)))
			f.Descriptor.ContentHash = facet.ContentHash(f.Payload)
			admit(f, facet.EventSuperseded, 1)
		}
	}
	w := seedFacetNoT("weight:1", "health.weight", `{"kg": 70}`)
	admit(w, facet.EventAdmitted, 0)

	srv := httptest.NewServer(inspect.Handler(inspect.Deps{Ledger: l, Timeout: 60 * time.Second, Buckets: defaultBuckets}))
	t.Cleanup(srv.Close)
	return srv
}

// A type with more rows than a page holds shows the first page and says so, and a
// narrower window brings the rest into reach. The summary, however big the type,
// draws one line.
func TestProjectionRows_OverTheLimitSaysSoAndAsksForANarrowerWindow(t *testing.T) {
	t.Parallel()
	srv := seedBig(t, 520)

	_, sum := get(t, srv.URL+"/projection")
	if rowsDrawn(sum) != 0 || len(sum) > 20_000 {
		t.Errorf("the summary of 520 facets draws %d facet rows in %d bytes", rowsDrawn(sum), len(sum))
	}
	steps := summaryCounts(t, sum)["activity.steps"]
	if steps.served != 520 || steps.superseded != 52 {
		t.Fatalf("steps = served %d superseded %d, want 520 and 52", steps.served, steps.superseded)
	}

	_, rows := get(t, srv.URL+steps.links["served"])
	if got := rowsDrawn(rows); got != 500 {
		t.Errorf("rows drawn = %d, want 500", got)
	}
	if !strings.Contains(rows, "Showing the earliest 500 of 520") || !strings.Contains(rows, "Narrow the window") {
		t.Errorf("the limit is not stated:\n%s", rows)
	}

	// One day holds 50 facets: inside the limit, so nothing is held back and no
	// apology is made.
	_, day := get(t, srv.URL+steps.links["served"]+"&from=2026-03-02&to=2026-03-02")
	if got := rowsDrawn(day); got != 50 || rowsClaimed(t, day) != 50 {
		t.Errorf("one day draws %d rows and claims %d, want 50", got, rowsClaimed(t, day))
	}
	if strings.Contains(day, "Narrow the window") {
		t.Error("a window inside the limit still asks for a narrower one")
	}

	// The superseded rows have a window too, and are read only for this type.
	_, old := get(t, srv.URL+steps.links["superseded"])
	if got := rowsDrawn(old); got != 52 {
		t.Errorf("superseded rows drawn = %d, want 52", got)
	}
}
