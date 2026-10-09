package daemon

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net/http"
	"sync/atomic"

	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/store/ops"
)

// importers are the importers the daemon accepts envelopes and Definitions from, by
// the hex SHA-256 of their token. The daemon never holds an importer's token.
type importers map[string]config.Importer

func loadImporters(path string) (importers, error) {
	out := importers{}
	if path == "" {
		return out, nil
	}
	list, err := config.LoadImporters(path)
	if err != nil {
		return nil, fmt.Errorf("daemon: %w", err)
	}
	for _, imp := range list {
		out[imp.TokenSHA256] = imp
	}
	return out, nil
}

// of is the importer whose token the request presents, if any. A map lookup by the
// token's hash leaks nothing an attacker could use: matching a hash means knowing a
// preimage.
func (in importers) of(r *http.Request) (config.Importer, bool) {
	p := presented(r)
	if len(p) == 0 {
		return config.Importer{}, false
	}
	sum := sha256.Sum256(p)
	imp, ok := in[hex.EncodeToString(sum[:])]
	return imp, ok
}

// mayWrite reports whether imp may submit facets of type t, and so register its
// Definitions.
func mayWrite(imp config.Importer, t facet.Type) bool {
	for _, allowed := range imp.Types {
		if allowed == t {
			return true
		}
	}
	return false
}

type importerKey struct{}

func importerOf(r *http.Request) config.Importer {
	imp, _ := r.Context().Value(importerKey{}).(config.Importer)
	return imp
}

// isOperator reports whether the request presents the operator's token, or needs
// none because none is configured.
func (d *Daemon) isOperator(r *http.Request) bool {
	return d.token == nil || subtle.ConstantTimeCompare(presented(r), d.token) == 1
}

// asImporter lets through only a request that presents an importer's token, with the
// importer in its context. The operator's token is refused with 403: it reads the
// record and runs the daemon, and an envelope admitted under it would name no source.
func (d *Daemon) asImporter(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		imp, ok := d.importers.of(r)
		switch {
		case ok:
			next(w, r.WithContext(context.WithValue(r.Context(), importerKey{}, imp)))
		case d.token != nil && d.isOperator(r):
			writeError(w, http.StatusForbidden, fmt.Errorf("the operator token submits nothing: send an importer's token"))
		default:
			w.Header().Set("WWW-Authenticate", `Bearer realm="anam"`)
			writeError(w, http.StatusUnauthorized, fmt.Errorf("an importer's token is required"))
		}
	})
}

// asReader lets through the operator or any importer: the published Definitions are
// the contract every importer shapes to, and hold nothing of the record.
func (d *Daemon) asReader(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := d.importers.of(r); ok || d.isOperator(r) {
			next(w, r)
			return
		}
		w.Header().Set("WWW-Authenticate", `Basic realm="anam"`)
		writeError(w, http.StatusUnauthorized, fmt.Errorf("unauthorized"))
	})
}

// definitions is the registry importers' envelopes are admitted against: the bundled
// Definitions and the newest registered version of every other type. It is replaced
// whole when a registration lands, so a reader always holds one consistent set.
type definitions struct {
	bundled *def.Registry
	current atomic.Pointer[def.Registry]
}

// loadDefinitions builds the registry from the bundled Definitions and those the
// operational store holds as registered. A type both bundled and registered refuses
// the start: one of the two would have to win silently.
func loadDefinitions(ctx context.Context, db *ops.DB) (*definitions, error) {
	bundled, err := def.Bundled()
	if err != nil {
		return nil, err
	}
	rows, err := db.RegisteredDefinitions(ctx)
	if err != nil {
		return nil, err
	}
	reg, err := def.WithRegistered(bundled, storedOf(rows))
	if err != nil {
		return nil, fmt.Errorf("daemon: registered definitions: %w", err)
	}
	d := &definitions{bundled: bundled}
	d.current.Store(reg)
	return d, nil
}

func storedOf(rows []ops.DefinitionRow) []def.Stored {
	out := make([]def.Stored, len(rows))
	for i, r := range rows {
		out[i] = def.Stored{Type: facet.Type(r.Type), Version: r.Version, Body: []byte(r.Body)}
	}
	return out
}
