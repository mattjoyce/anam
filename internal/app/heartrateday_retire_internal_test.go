package app

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/gate"
	"github.com/mattjoyce/anam/internal/projection"
	"github.com/mattjoyce/anam/internal/span"
	"github.com/mattjoyce/anam/internal/store/ledger"
	"github.com/mattjoyce/anam/internal/store/ops"
)

// Card 205 retires health.heart-rate-day by the normal reconciliation. The Garmin
// importer's poll sends health.heart-rate.day, one fact a day, and its snapshot also
// names the old type with nothing of it carried. Every old fact is then absent from a
// complete snapshot, so anam retract garmin plans exactly them, and with --yes
// tombstones them and nothing else.
func TestRetract_ASnapshotThatNamesAnOldTypeRetiresItsFacts(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	cfg := config.Config{
		LedgerPath: filepath.Join(dir, "ledger.db"), DBPath: filepath.Join(dir, "tracking.db"),
		PersonID:    "matt",
		BucketWidth: span.Default, Timeout: testTimeout, Limits: config.DefaultLimits(),
	}
	l, err := ledger.Create(ctx, cfg.LedgerPath, cfg.PersonID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	defs, err := def.Bundled()
	if err != nil {
		t.Fatal(err)
	}
	const oldType, newType = "health.heart-rate-day", "health.heart-rate.day"
	g := gate.New(l, defs, gate.StaticScopes{"garmin": {oldType, newType}}, clock.Real{}, cfg.PersonID,
		gate.Limits{MaxPayloadBytes: config.DefaultMaxPayloadBytes, MaxRetries: config.DefaultCommitRetries})
	admit := func(typ, payload string) string {
		t.Helper()
		env := facet.Envelope{Type: facet.Type(typ), Source: "garmin", Injector: "garmin", Payload: []byte(payload)}
		b, err := g.Resolve(env)
		if err != nil {
			t.Fatal(err)
		}
		res, _, err := g.Commit(ctx, env, b)
		if err != nil {
			t.Fatal(err)
		}
		return res.FacetID
	}
	frame := func(day string) string {
		return fmt.Sprintf(`"effective_time_frame": {"time_interval": {"start_date_time": "%sT00:00:00+11:00",
			"end_date_time": "%sT23:00:00+11:00"}}, "source_time": {"value": %q, "zone": "+11:00", "basis": "source"}`,
			day, day, day)
	}

	// What the old importer sent: three figures for each of two days.
	old := map[string]bool{}
	for _, day := range []string{"2026-02-17", "2026-02-19"} {
		for _, f := range []string{
			`"statistic": "resting", "heart_rate": {"value": 57, "unit": "beats/min"}, "temporal_relationship_to_physical_activity": "at rest"`,
			`"statistic": "minimum", "heart_rate": {"value": 56, "unit": "beats/min"}, "descriptive_statistic": "minimum"`,
			`"statistic": "maximum", "heart_rate": {"value": 113, "unit": "beats/min"}, "descriptive_statistic": "maximum"`,
		} {
			old[admit(oldType, fmt.Sprintf(`{"id": %q, %s, %s}`, day, f, frame(day)))] = true
		}
	}

	// The new importer's poll: one fact a day, its snapshot naming both types.
	w, _ := l.Head(ctx)
	fresh := map[string]bool{}
	for _, day := range []string{"2026-02-17", "2026-02-19"} {
		fresh[admit(newType, fmt.Sprintf(`{"id": %q, "resting": {"value": 57, "unit": "beats/min"},
			"minimum": {"value": 56, "unit": "beats/min"}, "maximum": {"value": 113, "unit": "beats/min"}, %s}`,
			day, frame(day)))] = true
	}
	carried := make([]string, 0, len(fresh))
	for id := range fresh {
		carried = append(carried, id)
	}
	buckets, _ := cfg.Buckets()
	db, err := ops.Open(ctx, cfg.DBPath, buckets)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.RecordSnapshotPart(ctx, ops.SnapshotPart{ID: "poll", Source: "garmin",
		Types: []string{newType, oldType}, Part: 1, Parts: 1, FacetIDs: carried}, time.Now(), int64(w),
		ops.SnapshotRetention{Incomplete: config.DefaultSnapshotTTL, Complete: config.DefaultSnapshotKeep}); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	cmd := &cobra.Command{}
	cmd.SetContext(ctx)
	var out bytes.Buffer
	cmd.SetOut(&out)
	absent, total, held, err := planFromSnapshot(ctx, cmd, cfg, "garmin")
	if err != nil {
		t.Fatalf("planFromSnapshot: %v", err)
	}
	if len(absent) != len(old) || total != len(old)+len(fresh) || held != len(fresh) {
		t.Fatalf("plan = %d absent, %d live, %d held; want %d of %d, %d held",
			len(absent), total, held, len(old), len(old)+len(fresh), len(fresh))
	}
	for _, f := range absent {
		if !old[f.ID] || f.Type != oldType {
			t.Errorf("planned %s (%s), which is not an old fact", f.ID, f.Type)
		}
	}
	if err := guardRetraction(len(absent), total, held, 0.75); err != nil {
		t.Fatalf("guard: %v", err)
	}
	if err := retractAll(cmd, cfg, absent); err != nil {
		t.Fatalf("retractAll: %v", err)
	}
	if !strings.Contains(out.String(), fmt.Sprintf("tombstoned %d facets", len(old))) {
		t.Errorf("report: %q", out.String())
	}

	res, err := fold.Read(ctx, l)
	if err != nil {
		t.Fatal(err)
	}
	for id := range old {
		if s := res.States[id].Status; s != facet.StatusTombstoned {
			t.Errorf("old fact %s is %s, want tombstoned", id, s)
		}
	}
	for id := range fresh {
		if s := res.States[id].Status; s != facet.StatusAdmitted {
			t.Errorf("new fact %s is %s, want admitted", id, s)
		}
	}
}

// heart v3 serves the day's one fact and the alerts, exactly the set v2 served under
// the old name: not the old three-a-day type, and not health.heart-rate, the retired
// health-db type a prefix would also have to keep out. It registers against the
// bundled Definitions alone, since both types it names are bundled.
func TestHeartV3ServesOneFactADay(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(filepath.Join("..", "..", "projections", "heart.v3.json"))
	if err != nil {
		t.Fatal(err)
	}
	defs, err := def.Bundled()
	if err != nil {
		t.Fatal(err)
	}
	held, err := projection.Bundled()
	if err != nil {
		t.Fatal(err)
	}
	reg, err := projection.Register(raw, held, defs.Has)
	if err != nil {
		t.Fatalf("Register heart v3: %v", err)
	}
	tpl := reg.Template
	if !reg.New || tpl.Name != "heart" || tpl.Version != 3 {
		t.Fatalf("heart v3 registers as %s v%d, new %v", tpl.Name, tpl.Version, reg.New)
	}
	got := tpl.Covers(defs.All())
	slices.Sort(got)
	if want := []facet.Type{"health.heart-rate-alert", "health.heart-rate.day"}; !slices.Equal(got, want) {
		t.Errorf("heart v3 covers %v, want %v", got, want)
	}
}
