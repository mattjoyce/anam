package query_test

import (
	"strings"
	"testing"
)

// A pointer that names a container passes the plan check and can never bind: attr
// holds every scalar leaf of a served payload, and a container's own pointer is
// never one of them. The engine refuses a pointer no Definition declares, and
// warns when a Definition declares one at some versions and not others; for a
// container it is silent, so a valid program answers empty forever.
//
// The engine's own premise is the opposite (checkDefined): "Reached silently it
// would be empty and the answer wrong."

func TestBug_AttrPointerToAContainerIsRefused(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.admit(t, "bp-csv", "health.blood-pressure", "2026-06-15T07:00:00Z",
		`{"measured_at": "2026-06-15T07:00:00Z", "systolic": 120, "diastolic": 80, "cuff": {"arm": "left"}}`)
	v, versions, _ := h.served(t)

	// The container's leaf binds.
	ans, err := tryRun(v, versions, h.defs,
		`arm(E, A) :- entry(E, "health.blood-pressure", _, _, _, _, _), attr(E, "/cuff/arm", A).`, "arm")
	if err != nil {
		t.Fatalf("the leaf of a container must bind: %v", err)
	}
	if ans.Len() != 1 {
		t.Fatalf("the leaf of a container bound %d times, want 1", ans.Len())
	}

	// The container's own pointer cannot: the engine must say so, not answer empty.
	_, err = tryRun(v, versions, h.defs,
		`cuff(E, A) :- entry(E, "health.blood-pressure", _, _, _, _, _), attr(E, "/cuff", A).`, "cuff")
	if err == nil {
		t.Fatal("the engine accepted attr(E, \"/cuff\", A): /cuff names a container and attr holds only " +
			"scalar leaves, so the program can never bind and answers empty — the shape checkDefined refuses: " +
			"\"reached silently it would be empty and the answer wrong\"")
	}
	if !strings.Contains(err.Error(), "/cuff") {
		t.Errorf("the refusal must name the pointer it refused: %v", err)
	}
	if !strings.Contains(err.Error(), "/cuff/arm") {
		t.Errorf("the refusal must name the container's leaves, the way the undeclared-pointer error names the types: %v", err)
	}
}
