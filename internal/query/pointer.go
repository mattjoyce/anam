package query

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mattjoyce/anam/internal/def"
)

// checkPointer is the fields' own check (card 138): the pointer is well formed and
// some Definition declares a field there. The types a value holds decide which
// Definition answers a given facet; a pointer no type declares cannot answer for any.
//
// A pointer that names a container is refused too, when every Definition that
// declares it declares it as one: attr holds the leaves of a payload, and a
// container holds none, so the program would answer empty forever and say nothing.
// The refusal names the leaves to ask for instead.
func checkPointer(p string, defs Definitions) error {
	if err := def.CheckPointer(p); err != nil {
		return fmt.Errorf("attr: %w", err)
	}
	var (
		declared  int
		leaves    []string
		container = true
	)
	for _, d := range defs.All() {
		if d.HasPointer(p) != nil {
			continue
		}
		declared++
		isContainer, l, err := d.ContainerAt(p)
		if err != nil {
			return fmt.Errorf("attr: %w", err)
		}
		if !isContainer {
			container = false
			continue
		}
		leaves = append(leaves, l...)
	}
	if declared == 0 {
		var types []string
		for _, d := range defs.All() {
			types = append(types, string(d.Type))
		}
		return fmt.Errorf("attr: no Definition declares a field at %s (the types are %s)", p, strings.Join(types, ", "))
	}
	if !container {
		return nil
	}
	leaves = uniq(leaves)
	if len(leaves) == 0 {
		return fmt.Errorf("attr: %s names a container, not a field, and the schema does not say what is in it", p)
	}
	return fmt.Errorf("attr: %s names a container, not a field; ask for its leaves: %s", p, strings.Join(leaves, ", "))
}

// uniq sorts and removes the duplicates, so a leaf several Definitions declare is
// named once.
func uniq(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	sort.Strings(in)
	out := in[:1]
	for _, s := range in[1:] {
		if s != out[len(out)-1] {
			out = append(out, s)
		}
	}
	return out
}
