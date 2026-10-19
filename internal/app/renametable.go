package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

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

// renamePair is one type left and the type arrived at.
type renamePair struct {
	From facet.Type `json:"from"`
	To   facet.Type `json:"to"`
}

// loadRenameTable reads and checks the table: two valid types in every pair, no pair
// that goes nowhere, no type left twice, and no chain, so one run is one step and a
// rerun is all no-op.
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
	if len(t.Renames) == 0 {
		return renameTable{}, fmt.Errorf("rename table %s: names no renames", path)
	}
	left := map[facet.Type]bool{}
	for i, p := range t.Renames {
		switch {
		case !p.From.Valid():
			return renameTable{}, fmt.Errorf("rename table %s: rename %d: from %q is not a valid type", path, i+1, p.From)
		case !p.To.Valid():
			return renameTable{}, fmt.Errorf("rename table %s: rename %d: to %q is not a valid type", path, i+1, p.To)
		case p.From == p.To:
			return renameTable{}, fmt.Errorf("rename table %s: rename %d: %s goes nowhere", path, i+1, p.From)
		case left[p.From]:
			return renameTable{}, fmt.Errorf("rename table %s: %s is left twice", path, p.From)
		}
		left[p.From] = true
	}
	for _, p := range t.Renames {
		if left[p.To] {
			return renameTable{}, fmt.Errorf("rename table %s: %s is both left and arrived at; one run is one step", path, p.To)
		}
	}
	return t, nil
}
