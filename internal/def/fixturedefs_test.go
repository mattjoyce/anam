package def_test

import (
	"io/fs"

	"github.com/mattjoyce/anam/internal/catalog/fixture"
)

// fixtureDefs is the definitions root of fixture.Mattjoyce, the catalog the package's
// external tests read in place of the one embedded in the binary (card 255, R4a).
func fixtureDefs() fs.FS {
	defs, _ := fixture.Mattjoyce()
	return defs
}
