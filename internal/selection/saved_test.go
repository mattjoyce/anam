package selection_test

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/selection"
	"github.com/mattjoyce/anam/internal/span"
	"github.com/mattjoyce/anam/internal/store/ops"
)

// Saved selections (card 206) over a real operational store.

func savedStore(t *testing.T) (*selection.Store, *ops.DB) {
	t.Helper()
	buckets, err := span.New(span.Default)
	if err != nil {
		t.Fatal(err)
	}
	a, err := ops.Open(t.Context(), filepath.Join(t.TempDir(), "tracking.db"), buckets)
	if err != nil {
		t.Fatalf("ops.Open: %v", err)
	}
	t.Cleanup(func() { _ = a.Close() })
	return selection.NewStore(a), a
}

func program(t *testing.T, f selection.Filter) string {
	t.Helper()
	p, err := f.Program()
	if err != nil {
		t.Fatalf("Program: %v", err)
	}
	return p
}

func TestSave_SameProgramOneCode(t *testing.T) {
	ctx := t.Context()
	s, a := savedStore(t)
	text := program(t, selection.Filter{Types: []string{"health.weight", "health.blood-pressure"}, Sources: []string{"withings"}})
	first, err := s.Save(ctx, text)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	// The same selection written another way: the rules reordered, a comment, spaces.
	again, err := s.Save(ctx, "  "+selectionReordered(text)+"\n\n")
	if err != nil {
		t.Fatalf("Save again: %v", err)
	}
	if first.Code != again.Code || first.Hash != again.Hash {
		t.Fatalf("one program, two codes: %+v and %+v", first, again)
	}
	if len(first.Code) != selection.CodeLen || !strings.HasPrefix(first.Hash, "sha256:"+first.Code) {
		t.Fatalf("code %q is not the first %d hex of %s", first.Code, selection.CodeLen, first.Hash)
	}
	if first.Hash != ops.Digest([]byte(text)) {
		t.Fatalf("hash %s is not the canonical text's %s", first.Hash, ops.Digest([]byte(text)))
	}
	stored, err := a.List(ctx, selection.Kind)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(stored) != 1 {
		t.Fatalf("saving one program twice filed %d rows", len(stored))
	}
	opened, err := s.Open(ctx, first.Ref())
	if err != nil {
		t.Fatalf("Open %s: %v", first.Ref(), err)
	}
	if opened != first {
		t.Fatalf("Open = %+v, saved %+v", opened, first)
	}
}

// selectionReordered is a chip program with each rule on one line, the rules in
// another order, and a comment of its own.
func selectionReordered(text string) string {
	lines := strings.Split(strings.TrimSpace(strings.ReplaceAll(text, ",\n    ", ", ")), "\n")
	for i, j := 0, len(lines)-1; i < j; i, j = i+1, j-1 {
		lines[i], lines[j] = lines[j], lines[i]
	}
	return "% written by hand\n" + strings.Join(lines, "\n")
}

func TestSave_DifferentProgramsDifferentCodes(t *testing.T) {
	ctx := t.Context()
	s, _ := savedStore(t)
	a, err := s.Save(ctx, program(t, selection.Filter{Types: []string{"health.weight"}}))
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	b, err := s.Save(ctx, program(t, selection.Filter{Types: []string{"health.blood-pressure"}}))
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	// A program outside the grammar is saved as its text, trimmed.
	joined := `pick(E) :- linked(E, "x").`
	c, err := s.Save(ctx, "\n"+joined+"\n")
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if a.Code == b.Code || a.Code == c.Code || b.Code == c.Code {
		t.Fatalf("different programs share a code: %s %s %s", a.Code, b.Code, c.Code)
	}
	if c.Program != joined {
		t.Fatalf("a program outside the grammar saved as %q, want it trimmed: %q", c.Program, joined)
	}
	for _, want := range []selection.Saved{a, b, c} {
		got, err := s.Open(ctx, want.Code)
		if err != nil || got != want {
			t.Fatalf("Open %s = %+v, %v; want %+v", want.Code, got, err, want)
		}
	}
}

// The ticks of two programs whose hashes share their first eight hex digits,
// f7c90470, found by search; the ninth digit is b for the first and d for the second.
const (
	collideFirst  = "n65980"
	collideSecond = "n81192"
)

func TestSave_ACollisionLengthensTheLaterCode(t *testing.T) {
	ctx := t.Context()
	s, a := savedStore(t)
	first, err := s.Save(ctx, program(t, selection.Filter{Ticks: []string{collideFirst}}))
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	second, err := s.Save(ctx, program(t, selection.Filter{Ticks: []string{collideSecond}}))
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if first.Hash[:7+selection.CodeLen] != second.Hash[:7+selection.CodeLen] {
		t.Fatalf("the fixture no longer collides: %s and %s", first.Hash, second.Hash)
	}
	if first.Code != "f7c90470" {
		t.Fatalf("the first saved keeps eight digits: got %q", first.Code)
	}
	if second.Code != "f7c90470d" {
		t.Fatalf("the later code lengthens to the shortest unique prefix: got %q, want f7c90470d", second.Code)
	}
	codes, err := a.List(ctx, selection.CodeKind)
	if err != nil || len(codes) != 1 {
		t.Fatalf("one lengthened code is filed: %v, %v", codes, err)
	}

	// Each code still names its own program, and the eight digits keep their meaning.
	for _, want := range []selection.Saved{first, second} {
		got, err := s.Open(ctx, want.Ref())
		if err != nil || got != want {
			t.Fatalf("Open %s = %+v, %v; want %+v", want.Ref(), got, err, want)
		}
	}
	// Saving either again answers the code it was given.
	for _, want := range []selection.Saved{first, second} {
		got, err := s.Save(ctx, want.Program)
		if err != nil || got.Code != want.Code {
			t.Fatalf("Save again = %+v, %v; want code %s", got, err, want.Code)
		}
	}
	// A longer prefix of the first's hash names it too.
	got, err := s.Open(ctx, first.Hash[7:7+12])
	if err != nil || got.Code != first.Code {
		t.Fatalf("Open by a longer prefix = %+v, %v; want code %s", got, err, first.Code)
	}
}

func TestOpen_Refusals(t *testing.T) {
	ctx := t.Context()
	s, _ := savedStore(t)
	if _, err := s.Open(ctx, "sel:00000000"); !errors.Is(err, selection.ErrNoCode) {
		t.Fatalf("an unknown code: %v, want ErrNoCode", err)
	}
	for _, bad := range []string{"", "sel:abc", "sel:nothexxx", strings.Repeat("a", 65)} {
		if _, err := s.Open(ctx, bad); !errors.Is(err, selection.ErrBadCode) {
			t.Fatalf("Open(%q): %v, want ErrBadCode", bad, err)
		}
	}
	for _, bad := range []string{"", "   ", "pick(E) :- "} {
		if _, err := s.Save(ctx, bad); !errors.Is(err, selection.ErrNotProgram) {
			t.Fatalf("Save(%q): %v, want ErrNotProgram", bad, err)
		}
	}
}
