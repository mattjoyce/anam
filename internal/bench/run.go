package bench

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/catchup"
	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/mint"
	"github.com/mattjoyce/anam/internal/read"
	"github.com/mattjoyce/anam/internal/span"
	"github.com/mattjoyce/anam/internal/store/artifact"
	"github.com/mattjoyce/anam/internal/store/ledger"
	"github.com/mattjoyce/anam/internal/store/ops"
)

// Report is one bench run: where it ran, what it ran against, and each scenario's
// timings.
type Report struct {
	Label     string     `json:"label"`
	Build     string     `json:"build"`
	Host      string     `json:"host"`
	Platform  string     `json:"platform"`
	CPUs      int        `json:"cpus"`
	Started   string     `json:"started"`
	Dataset   Dataset    `json:"dataset"`
	Scenarios []Scenario `json:"scenarios"`
}

// Dataset describes the record a run read.
type Dataset struct {
	Name        string `json:"name"`
	W           int64  `json:"w"`
	Facets      int    `json:"facets"`
	Live        int    `json:"live"`
	Buckets     int    `json:"buckets"`
	Checkpoints int    `json:"checkpoints"`
	// LastRun is the latest mint run at or below W, 0 for none; W minus it is the
	// tail a composed read replays.
	LastRun     int64 `json:"last_run"`
	LedgerBytes int64 `json:"ledger_bytes"`
}

// Scenario is one read, timed Reps times. Rows is what the read returned, so a run
// that got faster by returning less is visible as such.
type Scenario struct {
	Name     string  `json:"name"`
	Reps     int     `json:"reps"`
	MinMS    float64 `json:"min_ms"`
	MedianMS float64 `json:"median_ms"`
	MaxMS    float64 `json:"max_ms"`
	Rows     int     `json:"rows"`
}

// Options say what to run against.
type Options struct {
	Name, Label, Build, Person string
	Paths                      Paths
	Reps                       int
	Timeout                    time.Duration
	// Only, when set, names the scenarios to run by a substring of their names.
	Only []string
}

// Run times every scenario against the record at o.Paths. It catches the index up
// first, untimed, so a read of it measures the read.
func Run(ctx context.Context, o Options, progress func(string)) (Report, error) {
	l, err := ledger.OpenReadOnly(ctx, o.Paths.Ledger, o.Person)
	if err != nil {
		return Report{}, err
	}
	defer func() { _ = l.Close() }()
	buckets, err := span.New(span.Default)
	if err != nil {
		return Report{}, err
	}
	db, err := ops.Open(ctx, o.Paths.DB, buckets)
	if err != nil {
		return Report{}, err
	}
	defer func() { _ = db.Close() }()
	if _, err := catchup.Sync(ctx, l, db, buckets); err != nil {
		return Report{}, fmt.Errorf("bench: catch the index up: %w", err)
	}
	store, err := artifact.New(o.Paths.Artifacts)
	if err != nil {
		return Report{}, err
	}
	defer func() { _ = store.Close() }()

	rep, plan, err := describe(ctx, o, l, db, buckets)
	if err != nil {
		return Report{}, err
	}
	deps := mint.Deps{Ledger: l, Index: db, Artifacts: store, Buckets: buckets, Clock: clock.Real{}}
	for _, sc := range scenarios(l, db, plan, deps) {
		if !o.wants(sc.name) {
			continue
		}
		if progress != nil {
			progress(sc.name)
		}
		s, err := measure(ctx, o, sc)
		if err != nil {
			return Report{}, fmt.Errorf("bench: %s: %w", sc.name, err)
		}
		rep.Scenarios = append(rep.Scenarios, s)
	}
	return rep, nil
}

// wants reports whether a scenario is among those Only names.
func (o Options) wants(name string) bool {
	if len(o.Only) == 0 {
		return true
	}
	for _, s := range o.Only {
		if strings.Contains(name, s) {
			return true
		}
	}
	return false
}

// plan is the parameters the scenarios read with, taken from the record itself so
// any record, synthetic or real, gets the same questions.
type plan struct {
	head               ledger.Watermark
	monthFrom, monthTo string
	at, asOf, since    string
	commonType         string
}

// describe reads the record once to size it and to choose the scenarios' parameters:
// the month around the median fact-time, the snapshot at it, Anam's knowledge at
// half the head, and the last day of anam-time.
func describe(ctx context.Context, o Options, l *ledger.Ledger, db *ops.DB, buckets span.Buckets,
) (Report, plan, error) {
	res, err := fold.Read(ctx, l)
	if err != nil {
		return Report{}, plan{}, err
	}
	entries := res.Value.Entries()
	if len(entries) == 0 {
		return Report{}, plan{}, fmt.Errorf("bench: the record holds no live facet")
	}
	mid, err := clock.Parse(entries[len(entries)/2].FactTime)
	if err != nil {
		return Report{}, plan{}, err
	}
	p := plan{
		head:      res.W,
		monthFrom: time.Date(mid.Year(), mid.Month(), 1, 0, 0, 0, 0, time.UTC).Format("2006-01-02"),
		monthTo:   time.Date(mid.Year(), mid.Month()+1, 0, 0, 0, 0, 0, time.UTC).Format("2006-01-02"),
		at:        mid.Format(time.RFC3339),
	}
	if p.asOf, err = anamTimeAt(ctx, l, res.W/2); err != nil {
		return Report{}, plan{}, err
	}
	last, err := anamTimeAt(ctx, l, res.W)
	if err != nil {
		return Report{}, plan{}, err
	}
	lastT, _ := time.Parse(time.RFC3339, last)
	p.since = lastT.Add(-24 * time.Hour).Format(time.RFC3339)
	p.commonType = commonType(entries)

	parts, err := mint.Partition(res.Value, buckets)
	if err != nil {
		return Report{}, plan{}, err
	}
	cps, err := db.Checkpoints(ctx)
	if err != nil {
		return Report{}, plan{}, err
	}
	lastRun, _, err := db.LastMintRun(ctx, int64(res.W))
	if err != nil {
		return Report{}, plan{}, err
	}
	var size int64
	if fi, err := os.Stat(o.Paths.Ledger); err == nil {
		size = fi.Size()
	}
	host, _ := os.Hostname()
	return Report{
		Label: o.Label, Build: o.Build, Host: host, Platform: runtime.GOOS + "/" + runtime.GOARCH,
		CPUs: runtime.NumCPU(), Started: time.Now().UTC().Format(time.RFC3339),
		Dataset: Dataset{
			Name: o.Name, W: int64(res.W), Facets: len(res.States), Live: res.Value.Len(),
			Buckets: len(parts), Checkpoints: len(cps), LastRun: lastRun, LedgerBytes: size,
		},
	}, p, nil
}

// anamTimeAt is the anam-time of the event at w, as RFC 3339.
func anamTimeAt(ctx context.Context, l *ledger.Ledger, w ledger.Watermark) (string, error) {
	evs, err := l.Replay(ctx, w-1, w)
	if err != nil || len(evs) == 0 {
		return "", fmt.Errorf("bench: event %d: %v", w, err)
	}
	return evs[0].AnamTime.UTC().Format(time.RFC3339), nil
}

func commonType(entries []fold.Entry) string {
	n := map[string]int{}
	best := ""
	for _, e := range entries {
		n[string(e.Type)]++
		if n[string(e.Type)] > n[best] {
			best = string(e.Type)
		}
	}
	return best
}

// scenario is one timed read; it returns how many rows it read.
type scenario struct {
	name string
	fn   func(ctx context.Context) (int, error)
}

func scenarios(l *ledger.Ledger, db *ops.DB, p plan, deps mint.Deps) []scenario {
	src := read.Source{Ledger: l, Checkpoints: db, Artifacts: deps.Artifacts}
	timeline := func(q api.TimelineQuery) func(context.Context) (int, error) {
		return func(ctx context.Context) (int, error) {
			q.Zone = "UTC"
			t, err := read.Timeline(ctx, src, q)
			return t.Shown, err
		}
	}
	return []scenario{
		{"replay all events", func(ctx context.Context) (int, error) {
			evs, err := l.Replay(ctx, 0, p.head)
			return len(evs), err
		}},
		{"full fold (fold.ReadAt)", func(ctx context.Context) (int, error) {
			res, err := fold.ReadAt(ctx, l, p.head)
			return res.Value.Len(), err
		}},
		{"composed read (mint.ReadAt)", func(ctx context.Context) (int, error) {
			c, ok, err := mint.ReadAt(ctx, l, db, deps.Artifacts, p.head)
			if !ok {
				return 0, err // no mint run to start from: zero rows says so
			}
			return c.Value.Len(), err
		}},
		{"timeline, whole", timeline(api.TimelineQuery{})},
		{"timeline, one month", timeline(api.TimelineQuery{From: p.monthFrom, To: p.monthTo})},
		{"timeline, one type", timeline(api.TimelineQuery{Type: p.commonType})},
		{"timeline, snapshot --at", timeline(api.TimelineQuery{At: p.at})},
		{"timeline, --as-of half W", timeline(api.TimelineQuery{AsOf: p.asOf})},
		{"timeline, whole, as JSON", func(ctx context.Context) (int, error) {
			t, err := read.Timeline(ctx, src, api.TimelineQuery{Zone: "UTC"})
			if err != nil {
				return 0, err
			}
			b, err := json.Marshal(t)
			return len(b) >> 10, err
		}},
		{"changes, last day", func(ctx context.Context) (int, error) {
			head, err := l.Head(ctx)
			if err != nil {
				return 0, err
			}
			c, err := read.Changes(ctx, db, int64(head), api.ChangesQuery{Since: p.since, Zone: "UTC"})
			return len(c.Changes), err
		}},
		{"checkpoint --dry-run", func(ctx context.Context) (int, error) {
			m, err := mint.Report(ctx, deps, true)
			return m.MintedCount(), err
		}},
	}
}

// measure runs sc o.Reps times, each under o.Timeout.
func measure(ctx context.Context, o Options, sc scenario) (Scenario, error) {
	times := make([]float64, 0, o.Reps)
	rows := 0
	for i := 0; i < o.Reps; i++ {
		runCtx, cancel := context.WithTimeout(ctx, o.Timeout)
		start := time.Now()
		n, err := sc.fn(runCtx)
		elapsed := time.Since(start)
		cancel()
		if err != nil {
			return Scenario{}, err
		}
		rows = n
		times = append(times, float64(elapsed.Microseconds())/1000)
	}
	sort.Float64s(times)
	return Scenario{
		Name: sc.name, Reps: o.Reps, MinMS: times[0], MedianMS: times[len(times)/2],
		MaxMS: times[len(times)-1], Rows: rows,
	}, nil
}
