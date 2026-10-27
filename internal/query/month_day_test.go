package query_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/query"
)

// Acceptance: month_of/2 and day_of/2 derive the UTC month and day of each
// fact-time an entry carries, through query.Run.
func TestMonthOfDay_GroupsFactTimes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		ft    string
		month string
		day   string
	}{
		{"2026-01-31T23:59:59.999999999Z", "2026-01", "2026-01-31"},
		{"2026-02-01T00:00:00.000000000Z", "2026-02", "2026-02-01"},
		{"2025-12-31T21:00:00.000000000Z", "2025-12", "2025-12-31"},
		{"2024-02-29T12:00:00.000000000Z", "2024-02", "2024-02-29"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.ft, func(t *testing.T) {
			t.Parallel()
			v, versions := value(t, entry("e1", "health.weight", c.ft, "", `{"measured_at": "x", "kg": 1}`))
			defs := registry(t)
			mAns, _, err := query.Run(context.Background(), v, versions, defs, `m(M) :- month_of(_, M).`, "m", 0)
			if err != nil {
				t.Fatalf("month run: %v", err)
			}
			if !same(rows(mAns), []string{c.month}) {
				t.Errorf("month = %v, want %s", rows(mAns), c.month)
			}
			dAns, _, err := query.Run(context.Background(), v, versions, defs, `d(D) :- day_of(_, D).`, "d", 0)
			if err != nil {
				t.Fatalf("day run: %v", err)
			}
			if !same(rows(dAns), []string{c.day}) {
				t.Errorf("day = %v, want %s", rows(dAns), c.day)
			}
		})
	}
}

// Acceptance: two entries at one fact-time give one month_of and one day_of tuple.
func TestMonthOfDay_OneTuplePerDistinctFactTime(t *testing.T) {
	t.Parallel()
	ft := "2026-01-15T00:00:00.000000000Z"
	v, versions := value(t,
		entry("e1", "health.weight", ft, "", `{"measured_at": "x", "kg": 1}`),
		entry("e2", "health.weight", ft, "", `{"measured_at": "x", "kg": 2}`),
	)
	defs := registry(t)
	mAns, _, err := query.Run(context.Background(), v, versions, defs, `m(At, M) :- month_of(At, M).`, "m", 0)
	if err != nil {
		t.Fatal(err)
	}
	if mAns.Len() != 1 {
		t.Errorf("month_of tuples = %d, want 1: %v", mAns.Len(), rows(mAns))
	}
	dAns, _, err := query.Run(context.Background(), v, versions, defs, `d(At, D) :- day_of(At, D).`, "d", 0)
	if err != nil {
		t.Fatal(err)
	}
	if dAns.Len() != 1 {
		t.Errorf("day_of tuples = %d, want 1: %v", dAns.Len(), rows(dAns))
	}
}

// Acceptance: a count grouped by month.
func TestMonthOfDay_CountByMonth(t *testing.T) {
	t.Parallel()
	v, versions := value(t,
		entry("a", "health.weight", "2026-01-15T00:00:00.000000000Z", "", `{"measured_at": "x", "kg": 1}`),
		entry("b", "health.weight", "2026-01-20T00:00:00.000000000Z", "", `{"measured_at": "x", "kg": 2}`),
		entry("c", "health.weight", "2026-02-01T00:00:00.000000000Z", "", `{"measured_at": "x", "kg": 3}`),
	)
	defs := registry(t)
	text := `n(M, N) :- month_of(_, M), N = count : { entry(_, _, At, _, _, _, _), month_of(At, M) }.`
	ans, _, err := query.Run(context.Background(), v, versions, defs, text, "n", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !same(rows(ans), []string{"2026-01 2", "2026-02 1"}) {
		t.Errorf("count = %v, want [2026-01 2, 2026-02 1]", rows(ans))
	}
}

// Acceptance: a rule whose head is month_of is refused at the plan check.
func TestMonthOfDay_RefusedHead(t *testing.T) {
	t.Parallel()
	defs := registry(t)
	rules, err := query.Bundled()
	if err != nil {
		t.Fatal(err)
	}
	p, err := query.Parse(`month_of(X, Y) :- entry(X, _, Y, _, _, _, _).`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = query.Compile(query.BaseSchema, defs, rules.Program, p)
	if !errors.Is(err, query.ErrInvalid) || !strings.Contains(err.Error(), "base relation") {
		t.Errorf("Compile = %v, want ErrInvalid containing %q", err, "base relation")
	}
}
