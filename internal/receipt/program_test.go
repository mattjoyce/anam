package receipt_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/receipt"
)

// A query receipt carries its program whole, or not at all; and it is not narrowed,
// since the program names its own instants.
func TestReceipt_ProgramIsWholeOrAbsent(t *testing.T) {
	t.Parallel()
	base := receipt.Receipt{
		ServedHash: "sha256:a", FacetIDs: []string{}, Watermark: 1, Template: "bp", TemplateVersion: 1,
		DefinitionSetVersion: "sha256:d", ServedAt: "2026-10-19T00:00:00.000000000Z", Span: "all",
	}
	whole := &receipt.Program{RuleSet: "anam", RuleSetVersion: 1, RuleSetHash: "sha256:r", Text: "a(E) :- entry(E, _, _, _, _, _, _).", Relation: "a"}
	cases := []struct {
		name    string
		program *receipt.Program
		query   *receipt.Query
		want    string
	}{
		{"whole", whole, nil, ""},
		{"none", nil, nil, ""},
		{"no rule set", &receipt.Program{Text: "a.", Relation: "a"}, nil, "names no rule set"},
		{"no text", &receipt.Program{RuleSet: "anam", RuleSetVersion: 1, RuleSetHash: "h", Text: " ", Relation: "a"}, nil, "has no text"},
		{"no relation", &receipt.Program{RuleSet: "anam", RuleSetVersion: 1, RuleSetHash: "h", Text: "a."}, nil, "names no relation"},
		{"narrowed", whole, &receipt.Query{Type: "x"}, "not narrowed"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			r := base
			r.Program, r.Query = c.program, c.query
			err := r.Validate()
			if c.want == "" {
				if err != nil {
					t.Fatalf("Validate: %v", err)
				}
				b, err := r.Bytes()
				if err != nil {
					t.Fatal(err)
				}
				back, err := receipt.Parse(b)
				if err != nil {
					t.Fatalf("Parse: %v", err)
				}
				if (back.Program == nil) != (c.program == nil) {
					t.Errorf("program did not round-trip")
				}
				if c.program == nil && strings.Contains(string(b), "program") {
					t.Errorf("a receipt without a program carries the key: %s", b)
				}
				return
			}
			if !errors.Is(err, receipt.ErrMalformed) || !strings.Contains(err.Error(), c.want) {
				t.Errorf("Validate = %v, want ErrMalformed containing %q", err, c.want)
			}
		})
	}
}
