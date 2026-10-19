package daemon

import (
	_ "embed"
	"log/slog"
	"net/http"
)

// openAPI is the API's description, hand-written beside the routes (card 49) and
// held to them by openapi_test.go. It is embedded so a running build serves the
// document that describes it, not whichever file a deployment happens to have.
//
//go:embed openapi.yaml
var openAPI []byte

// openAPIPath is where the description is served. It takes no token: it holds no
// data, only shapes, and a client needs it before it has a token to try.
const openAPIPath = "GET /v1/openapi.yaml"

func (d *Daemon) getOpenAPI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/yaml")
	if _, err := w.Write(openAPI); err != nil {
		slog.Warn("daemon: write openapi", "err", err)
	}
}
