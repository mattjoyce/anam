package query

import (
	"context"
	"errors"
	"io/fs"

	"github.com/mattjoyce/tarski"

	"github.com/mattjoyce/anam/internal/fold"
)

// The second path's entry points, for the differential test in this package's
// external tests. They go when the swap lands (card 230, slice 2).

// RunTarskiWith is runTarskiWith.
func RunTarskiWith(ctx context.Context, rulesText string, served fold.Value, versions map[string]int, defs Definitions,
	text, relation string, maxBytes int,
) (tarski.Answer, error) {
	return runTarskiWith(ctx, rulesText, served, versions, defs, text, relation, maxBytes)
}

// CurrentAtTarski is the tarski path of Check's per-instant step: the ids current at
// at under the rules' text, over a base built once.
func CurrentAtTarski(ctx context.Context, rulesText string, v fold.Value, versions map[string]int, defs Definitions, ats []string) ([][]string, error) {
	rules, err := parseTarski(rulesText)
	if err != nil {
		return nil, err
	}
	base, err := newBaseTarski(v, versions, defs, false)
	if err != nil {
		return nil, err
	}
	var out [][]string
	for _, at := range ats {
		ids, err := currentAtTarski(ctx, rules, base, defs, at)
		if err != nil {
			return nil, err
		}
		out = append(out, ids)
	}
	return out, nil
}

// TarskiTooLarge reports err as tarski's *TooLarge, if it is one.
func TarskiTooLarge(err error) (*tarski.TooLarge, bool) {
	var tl *tarski.TooLarge
	return tl, errors.As(err, &tl)
}

// ParseTarski is parseTarski.
func ParseTarski(text string) (tarski.Program, error) { return parseTarski(text) }

// HeldTexts is every rule set text this binary holds, the bundled one first, by hash.
func HeldTexts() (map[string]string, error) {
	out := map[string]string{hashOf(bundledText): bundledText}
	for _, h := range history {
		b, err := fs.ReadFile(historyFS, "rules/history/"+h.File)
		if err != nil {
			return nil, err
		}
		out[hashOf(string(b))] = string(b)
	}
	return out, nil
}
