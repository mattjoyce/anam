package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/safepath"
)

// renameTable is the transform of a rename run (card 140), kept as data: which type
// each facet leaves and which it arrives as. The payload is carried as it is; a
// reshape has no place in this table yet, and a facet the new Definition refuses is
// what the dry run reports. JSON, as a projection template is: an operator's data
// file, not configuration.
type renameTable struct {
	Renames []renamePair `json:"renames"`
}

// renamePair is one type left and the type arrived at. A pair with a selector carries
// only the facets it matches, so several pairs can split one type (card 194); a pair
// without one carries every live facet of its type, as before.
type renamePair struct {
	From   facet.Type    `json:"from"`
	To     facet.Type    `json:"to"`
	Select *renameSelect `json:"select,omitempty"`
}

// renameSelect picks facets by what they say: the source they came from, and a
// string in the payload that starts with a prefix. Each half is optional, and a
// selector names at least one.
type renameSelect struct {
	Source  string `json:"source,omitempty"`
	Pointer string `json:"pointer,omitempty"`
	Prefix  string `json:"prefix,omitempty"`
}

// matches reports whether the facet is one this pair carries, given its payload
// decoded once (nil when it would not decode). A pair without a selector carries
// every facet of its type; a payload that lacks the pointer, or holds something
// other than a string there, does not match.
func (p renamePair) matches(f facet.Facet, doc any) bool {
	if f.Type != p.From {
		return false
	}
	s := p.Select
	if s == nil {
		return true
	}
	if s.Source != "" && f.Provenance.Source != s.Source {
		return false
	}
	if s.Pointer == "" {
		return true
	}
	v, err := def.Resolve(doc, s.Pointer)
	if err != nil {
		return false // the payload has no such field, so this pair does not select it
	}
	str, ok := v.(string)
	return ok && strings.HasPrefix(str, s.Prefix)
}

// String is the selector as the report prints it, with no spaces, or "-" for none.
func (s *renameSelect) String() string {
	if s == nil {
		return "-"
	}
	var parts []string
	if s.Source != "" {
		parts = append(parts, "source="+s.Source)
	}
	if s.Pointer != "" {
		parts = append(parts, s.Pointer+"="+s.Prefix+"*")
	}
	return strings.Join(parts, ",")
}

// check refuses a selector that names nothing, half a pointer-and-prefix pair, or a
// malformed pointer.
func (s *renameSelect) check() error {
	switch {
	case s.Source == "" && s.Pointer == "" && s.Prefix == "":
		return fmt.Errorf("select names nothing; give a source, a pointer and prefix, or both")
	case (s.Pointer == "") != (s.Prefix == ""):
		return fmt.Errorf("select needs a pointer and a prefix together")
	case s.Pointer != "":
		if err := def.CheckPointer(s.Pointer); err != nil {
			return fmt.Errorf("select: %w", err)
		}
	}
	return nil
}

// place is the indexes of every pair that carries the facet. A run carries a facet
// only when exactly one does.
func (t renameTable) place(f facet.Facet) []int {
	doc, err := def.Decode(f.Payload.Bytes())
	if err != nil {
		doc = nil // a payload that does not decode is selected by no pointer, so it is reported unmatched
	}
	var rows []int
	for i, p := range t.Renames {
		if p.matches(f, doc) {
			rows = append(rows, i)
		}
	}
	return rows
}

// loadRenameTable reads and checks the table, then answers it.
func loadRenameTable(path string) (renameTable, error) {
	fh, err := safepath.Open(path)
	if err != nil {
		return renameTable{}, fmt.Errorf("rename table: %w", err)
	}
	defer func() { _ = fh.Close() }()
	body, err := io.ReadAll(fh)
	if err != nil {
		return renameTable{}, fmt.Errorf("rename table %s: %w", path, err)
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	var t renameTable
	if err := dec.Decode(&t); err != nil {
		return renameTable{}, fmt.Errorf("rename table %s: %w", path, err)
	}
	if err := t.check(); err != nil {
		return renameTable{}, fmt.Errorf("rename table %s: %w", path, err)
	}
	return t, nil
}

// check holds the table to one step: two valid types in every pair, no pair that
// goes nowhere, and no chain, so a rerun is all no-op. A type left without a
// selector is left by that one pair; a type split by selectors has a selector on
// every pair that leaves it, and no pair is repeated.
func (t renameTable) check() error {
	if len(t.Renames) == 0 {
		return fmt.Errorf("names no renames")
	}
	whole := map[facet.Type]bool{}
	split := map[facet.Type]bool{}
	seen := map[string]bool{}
	for i, p := range t.Renames {
		switch {
		case !p.From.Valid():
			return fmt.Errorf("rename %d: from %q is not a valid type", i+1, p.From)
		case !p.To.Valid():
			return fmt.Errorf("rename %d: to %q is not a valid type", i+1, p.To)
		case p.From == p.To:
			return fmt.Errorf("rename %d: %s goes nowhere", i+1, p.From)
		case p.Select == nil && (whole[p.From] || split[p.From]):
			return fmt.Errorf("%s is left twice", p.From)
		case p.Select != nil && whole[p.From]:
			return fmt.Errorf("%s is left twice: once whole and once by select", p.From)
		}
		if p.Select == nil {
			whole[p.From] = true
			continue
		}
		if err := p.Select.check(); err != nil {
			return fmt.Errorf("rename %d: %w", i+1, err)
		}
		key := string(p.From) + " " + string(p.To) + " " + p.Select.Source + " " + p.Select.Pointer + " " + p.Select.Prefix
		if seen[key] {
			return fmt.Errorf("rename %d repeats an earlier one", i+1)
		}
		seen[key] = true
		split[p.From] = true
	}
	for _, p := range t.Renames {
		if whole[p.To] || split[p.To] {
			return fmt.Errorf("%s is both left and arrived at; one run is one step", p.To)
		}
	}
	return nil
}
