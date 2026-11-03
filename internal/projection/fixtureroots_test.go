package projection_test

import (
	"io/fs"

	"github.com/mattjoyce/anam/internal/catalog/fixture"
)

// fixtureProjs is the projections root of fixture.Engine, the catalog the package's
// external tests read: one template per engine feature, nobody's record (card 255,
// R4b). A test that needs the Definitions too reads catalogtest.Engine.
func fixtureProjs() fs.FS {
	_, projs := fixture.Engine()
	return projs
}
