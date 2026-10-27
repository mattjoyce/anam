package query_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/query"
)

func TestAggregate_CountAndSum(t *testing.T) {
	t.Parallel()
	entries := []fold.Entry{
		entry("r1", "health.blood-pressure", "2026-06-01T07:00:00.000000000Z", "", `{"measured_at":"2026-06-01T07:00:00Z","systolic":118}`),
		entry("r2", "health.blood-pressure", "2026-07-20T07:00:00.000000000Z", "", `{"measured_at":"2026-07-20T07:00:00Z","systolic":146.5}`),
		entry("r3", "health.blood-pressure", "2026-08-20T07:00:00.000000000Z", "", `{"measured_at":"2026-08-20T07:00:00Z","systolic":9.25}`),
	}
	v, versions := value(t, entries...)

	ans, _, err := query.Run(context.Background(), v, versions, registry(t),
		`n(N) :- N = count : { entry(_, "health.blood-pressure", _, _, _, _, _) }.`, "n", 0)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if got := rows(ans); len(got) != 1 || got[0] != "3" {
		t.Fatalf("count answer = %v, want [3]", got)
	}

	ans2, _, err := query.Run(context.Background(), v, versions, registry(t),
		`s(S) :- S = sum V : { entry(E, "health.blood-pressure", _, _, _, _, _), attr(E, "/systolic", V) }.`, "s", 0)
	if err != nil {
		t.Fatalf("sum: %v", err)
	}
	if got := rows(ans2); len(got) != 1 || got[0] != "273.75" {
		t.Fatalf("sum answer = %v, want [273.75]", got)
	}
}

func TestAggregate_ReadsItsOwnResult(t *testing.T) {
	t.Parallel()
	entries := []fold.Entry{
		entry("r1", "health.blood-pressure", "2026-06-01T07:00:00.000000000Z", "", `{"measured_at":"2026-06-01T07:00:00Z","systolic":118}`),
	}
	v, versions := value(t, entries...)
	_, _, err := query.Run(context.Background(), v, versions, registry(t),
		`n(N) :- N = count : { n(_) }.`, "n", 0)
	if !errors.Is(err, query.ErrInvalid) {
		t.Fatalf("got %v, want ErrInvalid", err)
	}
}

func TestAggregate_RefusesPrefixInsideBraces(t *testing.T) {
	t.Parallel()
	entries := []fold.Entry{
		entry("r1", "health.blood-pressure", "2026-06-01T07:00:00.000000000Z", "", `{"measured_at":"2026-06-01T07:00:00Z","systolic":118}`),
	}
	v, versions := value(t, entries...)
	_, _, err := query.Run(context.Background(), v, versions, registry(t),
		`n(N) :- N = count : { entry(X, _, _, _, _, _, _), prefix("a", X) }.`, "n", 0)
	if !errors.Is(err, query.ErrInvalid) || !strings.Contains(err.Error(), "prefix is not part of Anam's language") {
		t.Fatalf("got %v", err)
	}
}

func TestAggregate_RefusesBareDateInstantInsideBraces(t *testing.T) {
	t.Parallel()
	entries := []fold.Entry{
		entry("r1", "health.blood-pressure", "2026-06-01T07:00:00.000000000Z", "", `{"measured_at":"2026-06-01T07:00:00Z","systolic":118}`),
	}
	v, versions := value(t, entries...)
	_, _, err := query.Run(context.Background(), v, versions, registry(t),
		`n(N) :- N = count : { instant("2026-07-20") }.`, "n", 0)
	if !errors.Is(err, query.ErrInvalid) || !strings.Contains(err.Error(), "is not a fact-time") {
		t.Fatalf("got %v", err)
	}
}
