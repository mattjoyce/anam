package catalog

import (
	"bytes"
	"sort"
)

// Drift describes file differences from here to another loaded catalog. Names are
// relative to each component's root, including history/. Lists are sorted.
type Drift struct {
	DefinitionsOnlyHere, DefinitionsOnlyThere, DefinitionsDiffer []string
	ProjectionsOnlyHere, ProjectionsOnlyThere, ProjectionsDiffer []string
	SetVersionDiffers                                            bool
}

// None reports whether both catalogs have the same files, bytes and set version.
func (d Drift) None() bool {
	return len(d.DefinitionsOnlyHere)+len(d.DefinitionsOnlyThere)+len(d.DefinitionsDiffer)+
		len(d.ProjectionsOnlyHere)+len(d.ProjectionsOnlyThere)+len(d.ProjectionsDiffer) == 0 &&
		!d.SetVersionDiffers
}

// DriftFrom compares the original file names and raw bytes retained at load time.
// It performs no I/O and changes neither catalog.
func (c *Catalog) DriftFrom(other *Catalog) Drift {
	var d Drift
	d.DefinitionsOnlyHere, d.DefinitionsOnlyThere, d.DefinitionsDiffer =
		compareSnapshots(c.definitionFiles, other.definitionFiles)
	d.ProjectionsOnlyHere, d.ProjectionsOnlyThere, d.ProjectionsDiffer =
		compareSnapshots(c.projectionFiles, other.projectionFiles)
	d.SetVersionDiffers = c.setVersion != other.setVersion
	return d
}

func compareSnapshots(here, there *snapshot) (onlyHere, onlyThere, differ []string) {
	for _, dir := range []string{".", "history"} {
		h, _ := here.dir(dir)
		t, _ := there.dir(dir)
		prefix := ""
		if dir != "." {
			prefix = dir + "/"
		}
		for name, body := range h {
			other, ok := t[name]
			if !ok {
				onlyHere = append(onlyHere, prefix+name)
			} else if !bytes.Equal(body, other) {
				differ = append(differ, prefix+name)
			}
		}
		for name := range t {
			if _, ok := h[name]; !ok {
				onlyThere = append(onlyThere, prefix+name)
			}
		}
	}
	sort.Strings(onlyHere)
	sort.Strings(onlyThere)
	sort.Strings(differ)
	return onlyHere, onlyThere, differ
}
