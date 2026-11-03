package def_test

import (
	"io/fs"

	"github.com/mattjoyce/anam/internal/catalog/fixture"
)

// fixtureDefs is the definitions root of fixture.Engine, the catalog the package's
// external tests read: one type per engine feature, nobody's record (card 255, R4b).
func fixtureDefs() fs.FS {
	defs, _ := fixture.Engine()
	return defs
}
