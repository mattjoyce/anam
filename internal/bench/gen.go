// Package bench is Anam's read performance bench: deterministic synthetic ledgers
// built through the real gate, and the same read scenarios timed against any ledger,
// synthetic or a copy of the record. Results are values written as JSON, so a run
// before a change and a run after it can be compared line for line.
package bench

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"time"

	"github.com/mattjoyce/anam/internal/catchup"
	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/gate"
	"github.com/mattjoyce/anam/internal/mint"
	"github.com/mattjoyce/anam/internal/span"
	"github.com/mattjoyce/anam/internal/store/ledger"
	"github.com/mattjoyce/anam/internal/store/ops"
)

// Person is whose record a synthetic ledger is.
const Person = "bench"

const (
	source   = "bp-csv"
	factType = "health.blood-pressure"
	// layout and zone are the blood pressure Definition's fact-time rule.
	layout = "2006-1-2 15:04:05"
	zone   = "Australia/Sydney"
)

// Shape is a synthetic ledger: Months monthly buckets of PerMonth readings each,
// starting at Start, then corrections and retractions over the whole history, a mint,
// and a tail of new readings and late corrections after the mint, the way an hourly
// ingest lands after the last checkpoint.
type Shape struct {
	Name     string `json:"name"`
	Months   int    `json:"months"`
	PerMonth int    `json:"per_month"`
	// CorrectEvery supersedes every Nth reading once, after the base load; 0 for none.
	CorrectEvery int `json:"correct_every"`
	// Retract is how many readings are tombstoned after the corrections.
	Retract int `json:"retract"`
	// TailNew and TailCorrect are the readings admitted and superseded after the mint.
	TailNew     int       `json:"tail_new"`
	TailCorrect int       `json:"tail_correct"`
	Start       time.Time `json:"start"`
	// Budget is the mint's checkpoint_budget in bytes; 0 mints at the fixed monthly
	// width, as the first bench datasets were.
	Budget int `json:"budget"`
}

// Shapes are the bench's named scales. S is about today's record; M is several years
// of daily and intraday summaries from more sources; L is the density a minute-level
// source such as Garmin's monitoring tables would bring.
func Shapes() map[string]Shape {
	start := time.Date(2016, 1, 1, 0, 0, 0, 0, time.UTC)
	return map[string]Shape{
		"S": {Name: "S", Months: 36, PerMonth: 500, CorrectEvery: 20, Retract: 20, TailNew: 300, TailCorrect: 30, Start: start},
		"M": {Name: "M", Months: 60, PerMonth: 2000, CorrectEvery: 20, Retract: 20, TailNew: 300, TailCorrect: 30, Start: start},
		"L": {Name: "L", Months: 120, PerMonth: 5000, CorrectEvery: 20, Retract: 20, TailNew: 300, TailCorrect: 30, Start: start},
	}
}

// Paths are the files of one bench record, laid out as Anam lays out its own.
type Paths struct {
	Ledger, DB string
}

// PathsIn is the layout of a record in dir.
func PathsIn(dir string) Paths {
	return Paths{
		Ledger: filepath.Join(dir, "ledger.db"),
		DB:     filepath.Join(dir, "tracking.db"),
	}
}

// stepClock is the anam-time a synthetic ingest is stamped with. Each batch is
// stamped a day after the last reading in it, so anam-time runs forward with
// fact-time and an as-of read has a history to wind back through.
type stepClock struct{ now time.Time }

func (c *stepClock) Now() time.Time { return c.now }

// Generate builds the record sh describes in dir, which must not hold one. The same
// shape always yields the same facts, anam-times included; only the ledger's id
// differs between two builds.
func Generate(ctx context.Context, dir string, sh Shape, progress func(string)) error {
	p := PathsIn(dir)
	l, err := ledger.Create(ctx, p.Ledger, Person)
	if err != nil {
		return err
	}
	defer func() { _ = l.Close() }()
	defs, err := def.Bundled()
	if err != nil {
		return err
	}
	clk := &stepClock{}
	g := gate.New(l, defs, gate.StaticScopes{source: {factType}}, clk, Person, gate.Limits{MaxPayloadBytes: config.DefaultMaxPayloadBytes, MaxRetries: config.DefaultCommitRetries})
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return err
	}

	base := readings(sh, loc)
	ids, err := admit(ctx, g, clk, base, dayAfterLast, progress, "base")
	if err != nil {
		return err
	}
	if err := correctAndRetract(ctx, g, clk, sh, base, ids, progress); err != nil {
		return err
	}
	if err := checkpoint(ctx, l, p, sh.Budget, progress); err != nil {
		return err
	}
	return tail(ctx, g, clk, sh, base, l, p, progress)
}

// reading is one synthetic blood pressure reading, before it is an envelope.
type reading struct {
	at       time.Time
	ordinal  int
	systolic int
}

// readings spreads PerMonth readings evenly across each month.
func readings(sh Shape, loc *time.Location) []reading {
	out := make([]reading, 0, sh.Months*sh.PerMonth)
	for m := 0; m < sh.Months; m++ {
		first := time.Date(sh.Start.Year(), sh.Start.Month()+time.Month(m), 1, 0, 0, 0, 0, loc)
		step := first.AddDate(0, 1, 0).Sub(first) / time.Duration(sh.PerMonth)
		for i := 0; i < sh.PerMonth; i++ {
			at := first.Add(time.Duration(i) * step).Truncate(time.Second)
			out = append(out, reading{at: at, ordinal: 1, systolic: 100 + (m*7+i)%40})
		}
	}
	return out
}

func envelope(r reading) facet.Envelope {
	ts := r.at.Format(layout)
	ord := strconv.Itoa(r.ordinal)
	mmHg := func(v int) map[string]any { return map[string]any{"value": v, "unit": "mmHg"} }
	payload, _ := json.Marshal(map[string]any{ // plain values always marshal
		"date_excel": ts, "reading": ord,
		"source_time":              map[string]string{"value": ts, "zone": "Australia/Sydney", "basis": "source"},
		"systolic_blood_pressure":  mmHg(r.systolic),
		"diastolic_blood_pressure": mmHg(60 + r.systolic%25),
		"effective_time_frame":     map[string]string{"date_time": r.at.Format(time.RFC3339)},
	})
	return facet.Envelope{
		Type: factType, Source: source, Injector: source,
		SourceRef: facet.SourceRef(source, ts, ord), Payload: payload, AssertedPerson: Person,
	}
}

// admit commits rs in gate-sized batches, each stamped with stamp(batch). It returns
// the facet id of each reading, in order.
func admit(ctx context.Context, g *gate.Gate, clk *stepClock, rs []reading, stamp func([]reading) time.Time,
	progress func(string), stage string,
) ([]string, error) {
	ids := make([]string, 0, len(rs))
	for start := 0; start < len(rs); start += gate.MaxBatch {
		chunk := rs[start:min(start+gate.MaxBatch, len(rs))]
		envs := make([]facet.Envelope, len(chunk))
		bindings := make([]facet.Binding, len(chunk))
		for i, r := range chunk {
			envs[i] = envelope(r)
			b, err := g.Resolve(envs[i])
			if err != nil {
				return nil, err
			}
			bindings[i] = b
		}
		clk.now = stamp(chunk)
		results, _, err := g.CommitBatch(ctx, envs, bindings)
		if err != nil {
			return nil, fmt.Errorf("bench: %s batch at %d: %w", stage, start, err)
		}
		for _, res := range results {
			ids = append(ids, res.FacetID)
		}
		if progress != nil && (start/gate.MaxBatch)%500 == 0 {
			progress(fmt.Sprintf("%s %d/%d", stage, start+len(chunk), len(rs)))
		}
	}
	return ids, nil
}

// dayAfterLast stamps a batch a day after its last reading, as a daily export would.
func dayAfterLast(chunk []reading) time.Time { return chunk[len(chunk)-1].at.Add(24 * time.Hour) }

// at stamps every batch at t.
func at(t time.Time) func([]reading) time.Time { return func([]reading) time.Time { return t } }

// correctAndRetract supersedes every CorrectEvery-th reading and tombstones Retract
// readings spread over the history, all stamped after the base load.
func correctAndRetract(ctx context.Context, g *gate.Gate, clk *stepClock, sh Shape, base []reading,
	ids []string, progress func(string),
) error {
	end := base[len(base)-1].at
	if sh.CorrectEvery > 0 {
		var fixes []reading
		for i := 0; i < len(base); i += sh.CorrectEvery {
			r := base[i]
			r.systolic++
			fixes = append(fixes, r)
		}
		// Stamped after the base load, as a re-export of the whole source would be.
		if _, err := admit(ctx, g, clk, fixes, at(end.Add(48*time.Hour)), progress, "corrections"); err != nil {
			return err
		}
	}
	clk.now = end.Add(72 * time.Hour)
	for k := 0; k < sh.Retract; k++ {
		if _, _, err := g.Retract(ctx, ids[(k*len(ids))/sh.Retract+1]); err != nil {
			return fmt.Errorf("bench: retract: %w", err)
		}
	}
	return nil
}

// tail lands after the mint: TailNew readings in the month after the history, and
// TailCorrect late corrections spread over old buckets, the way an hourly ingest
// lands after the last checkpoint. The index is caught up after it.
func tail(ctx context.Context, g *gate.Gate, clk *stepClock, sh Shape, base []reading, l *ledger.Ledger,
	p Paths, progress func(string),
) error {
	end := base[len(base)-1].at
	stamp := at(end.Add(96 * time.Hour))
	var fresh []reading
	for i := 0; i < sh.TailNew; i++ {
		fresh = append(fresh, reading{at: end.Add(time.Duration(i+1) * time.Hour), ordinal: 1, systolic: 120})
	}
	if _, err := admit(ctx, g, clk, fresh, stamp, progress, "tail"); err != nil {
		return err
	}
	var late []reading
	for k := 0; k < sh.TailCorrect; k++ {
		r := base[(k*len(base))/max(sh.TailCorrect, 1)+3]
		r.systolic += 2
		late = append(late, r)
	}
	if _, err := admit(ctx, g, clk, late, stamp, progress, "late corrections"); err != nil {
		return err
	}
	return syncIndex(ctx, l, p)
}

// syncIndex catches the operational index up to the ledger's head.
func syncIndex(ctx context.Context, l *ledger.Ledger, p Paths) error {
	buckets, err := span.New(span.Default)
	if err != nil {
		return err
	}
	db, err := ops.Open(ctx, p.DB, buckets)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	_, err = catchup.Sync(ctx, l, db, buckets)
	return err
}

// checkpoint catches the index up and mints every bucket, as `anam checkpoint` does.
func checkpoint(ctx context.Context, l *ledger.Ledger, p Paths, budget int, progress func(string)) error {
	buckets, err := span.New(span.Default)
	if err != nil {
		return err
	}
	db, err := ops.Open(ctx, p.DB, buckets)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	if _, err := catchup.Sync(ctx, l, db, buckets); err != nil {
		return err
	}
	res, err := mint.Run(ctx, mint.Deps{
		Ledger: l, Index: db, Values: db, Buckets: buckets, Clock: clock.Real{}, Budget: budget,
	}, false)
	if err != nil {
		return err
	}
	if progress != nil {
		progress(fmt.Sprintf("minted %d buckets at W=%d", res.Minted(), res.W))
	}
	return nil
}

// MintRecord mints the record at p, as `anam checkpoint` does, and returns how many
// buckets it minted and the run's watermark.
func MintRecord(ctx context.Context, p Paths, person string, budget int) (int, int64, error) {
	l, err := ledger.OpenReadOnly(ctx, p.Ledger, person)
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = l.Close() }()
	buckets, err := span.New(span.Default)
	if err != nil {
		return 0, 0, err
	}
	db, err := ops.Open(ctx, p.DB, buckets)
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = db.Close() }()
	if _, err := catchup.Sync(ctx, l, db, buckets); err != nil {
		return 0, 0, err
	}
	res, err := mint.Run(ctx, mint.Deps{
		Ledger: l, Index: db, Values: db, Buckets: buckets, Clock: clock.Real{}, Budget: budget,
	}, false)
	if err != nil {
		return 0, 0, err
	}
	return res.Minted(), int64(res.W), nil
}

// BackfillSizes records the facet count of every checkpoint in the record at p that has
// none, by decoding it. A record minted before counts were recorded can then be read
// narrowly without re-minting, which would also discard its tail.
func BackfillSizes(ctx context.Context, p Paths) (int, error) {
	buckets, err := span.New(span.Default)
	if err != nil {
		return 0, err
	}
	db, err := ops.Open(ctx, p.DB, buckets)
	if err != nil {
		return 0, err
	}
	defer func() { _ = db.Close() }()
	rows, err := db.Checkpoints(ctx)
	if err != nil {
		return 0, err
	}
	have, err := db.CheckpointSizes(ctx)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, r := range rows {
		if _, ok := have[ops.SpanVersion{SpanID: r.SpanID, V: r.V}]; ok {
			continue
		}
		b, err := db.Get(ctx, mint.Kind, r.ValueHash)
		if err != nil {
			return n, err
		}
		var entries []json.RawMessage
		if err := json.Unmarshal(b, &entries); err != nil {
			return n, err
		}
		if err := db.AppendCheckpointSize(ctx, r.SpanID, r.V, len(entries)); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}
