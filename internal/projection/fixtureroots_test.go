package projection_test

import (
	"io/fs"
	"testing"

	"github.com/mattjoyce/anam/internal/catalog/fixture"
	"github.com/mattjoyce/anam/internal/projection"
)

// fixtureDefs and fixtureProjs are the two roots of fixture.Mattjoyce, the catalog
// the package's external tests read in place of the one embedded in the binary (card
// 255, R4a).
func fixtureDefs() fs.FS {
	defs, _ := fixture.Mattjoyce()
	return defs
}

func fixtureProjs() fs.FS {
	_, projs := fixture.Mattjoyce()
	return projs
}

// fixtureProjections is the projection registry over fixture.Mattjoyce.
func fixtureProjections(t *testing.T) *projection.Registry {
	t.Helper()
	reg, err := projection.BundledFS(fixtureProjs())
	if err != nil {
		t.Fatalf("BundledFS(fixture): %v", err)
	}
	return reg
}
