package inspect_test

import (
	"context"
	"sync"

	"github.com/mattjoyce/anam/internal/catalog"
)

// testCatalog is the embedded catalog, loaded once for every inspector test: the
// catalog a daemon or `anam inspect` would hand the pages.
var testCatalog = sync.OnceValue(func() *catalog.Catalog {
	cat, err := catalog.Load(context.Background(), "", "")
	if err != nil {
		panic("inspect test: load the embedded catalog: " + err.Error())
	}
	return cat
})
