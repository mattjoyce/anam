package daemon

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/store/ops"
)

// postDefinitions registers a Definition an importer emits. The importer may register
// only types it may submit. A registered version is immutable: the same body again is
// unchanged, and anything else at that version is refused.
func (d *Daemon) postDefinitions(w http.ResponseWriter, r *http.Request) {
	imp := importerOf(r)
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, int64(d.cfg.MaxManifestBytes)))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, err)
		return
	}
	parsed, err := def.Parse(body)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}
	if !mayWrite(imp, parsed.Type) {
		writeError(w, http.StatusForbidden, fmt.Errorf("%s may not submit %s, so may not define it", imp.Source, parsed.Type))
		return
	}

	var reg def.Registration
	err = d.tryWriteWithin(d.cfg.ImporterWait, d.timeout(r), func(ctx context.Context, s Stores) error {
		var err error
		reg, err = d.register(ctx, s.Ops, body)
		return err
	})
	switch {
	case errors.Is(err, def.ErrBadManifest):
		writeError(w, http.StatusUnprocessableEntity, err)
	case errors.Is(err, def.ErrBundled), errors.Is(err, def.ErrRegistered),
		errors.Is(err, def.ErrVersionBehind), errors.Is(err, def.ErrKeyed):
		writeError(w, http.StatusConflict, err)
	case errors.Is(err, ErrBusy), errors.Is(err, ErrStopped):
		w.Header().Set("Retry-After", d.retryAfter())
		writeError(w, http.StatusServiceUnavailable, err)
	case err != nil:
		writeError(w, http.StatusInternalServerError, err)
	default:
		status := http.StatusOK
		if reg.New {
			status = http.StatusCreated
		}
		writeJSON(w, status, api.Registered{Type: string(reg.Definition.Type), Version: reg.Definition.Version, New: reg.New})
	}
}

// register decides a registration against everything held and, when it is new,
// stores it and replaces the registry. The registry is built before the row is
// stored, so a row the daemon could not start on is never written.
func (d *Daemon) register(ctx context.Context, db *ops.DB, body []byte) (def.Registration, error) {
	stored, err := db.Definitions(ctx)
	if err != nil {
		return def.Registration{}, err
	}
	registered, err := db.RegisteredDefinitions(ctx)
	if err != nil {
		return def.Registration{}, err
	}
	reg, err := def.Register(body, storedOf(stored), storedOf(registered), d.defs.bundled)
	if err != nil || !reg.New {
		return reg, err
	}
	row := ops.DefinitionRow{Type: string(reg.Definition.Type), Version: reg.Definition.Version, Body: string(body)}
	next, err := def.WithRegistered(d.defs.bundled, storedOf(append(registered, row)))
	if err != nil {
		return def.Registration{}, err
	}
	if err := db.RegisterDefinition(ctx, row, clock.Format(time.Now())); err != nil {
		return def.Registration{}, err
	}
	d.defs.current.Store(next)
	return reg, nil
}

// heldDefinitions is every Definition body the daemon can publish, by type and
// version: the bundled ones as this build carries them, the older bundled versions
// the store has kept, and every registered version.
func (d *Daemon) heldDefinitions(ctx context.Context) (map[facet.Type]map[int][]byte, map[facet.Type]string, error) {
	bodies := map[facet.Type]map[int][]byte{}
	origin := map[facet.Type]string{}
	put := func(t facet.Type, v int, body []byte, from string) {
		if bodies[t] == nil {
			bodies[t] = map[int][]byte{}
		}
		if _, ok := bodies[t][v]; !ok {
			bodies[t][v] = body
		}
		if origin[t] == "" {
			origin[t] = from
		}
	}
	// The stored body wins over the build's for the same type and version: it is
	// what was in force when the facets were admitted, and the page says "as it was
	// in force". The two differ only if a bundled manifest was edited without a
	// version bump, which is the one thing an edit must not do; the start-up upsert
	// then overwrites the stored row, so the rule is the guard, not this order.
	registered, err := d.readOps.RegisteredDefinitions(ctx)
	if err != nil {
		return nil, nil, err
	}
	for _, r := range registered {
		put(facet.Type(r.Type), r.Version, []byte(r.Body), api.OriginRegistered)
	}
	stored, err := d.readOps.Definitions(ctx)
	if err != nil {
		return nil, nil, err
	}
	for _, r := range stored {
		put(facet.Type(r.Type), r.Version, []byte(r.Body), api.OriginBundled)
	}
	current, err := def.Manifests()
	if err != nil {
		return nil, nil, err
	}
	for _, m := range current {
		put(m.Type, m.Version, m.Body, api.OriginBundled)
	}
	return bodies, origin, nil
}

// getDefinitions lists every type the daemon admits, with its current version and
// every version held. A type the binary no longer carries and no importer registered
// is not listed: nothing can be admitted against it.
func (d *Daemon) getDefinitions(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), d.timeout(r))
	defer cancel()
	bodies, origin, err := d.heldDefinitions(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	reg := d.defs.current.Load()
	out := api.Definitions{Definitions: []api.DefinitionSummary{}}
	for _, cur := range reg.All() {
		s := api.DefinitionSummary{Type: string(cur.Type), Origin: origin[cur.Type], Current: cur.Version}
		for v := range bodies[cur.Type] {
			s.Versions = append(s.Versions, v)
		}
		sort.Ints(s.Versions)
		out.Definitions = append(out.Definitions, s)
	}
	writeJSON(w, http.StatusOK, out)
}

// getDefinition answers the manifest of {type} at {version}, or at its current
// version when none is named, as it was published.
func (d *Daemon) getDefinition(w http.ResponseWriter, r *http.Request) {
	t := facet.Type(r.PathValue("type"))
	cur, ok := d.defs.current.Load().Lookup(t)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Errorf("no definition for %s", t))
		return
	}
	version := cur.Version
	if v := r.PathValue("version"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			writeError(w, http.StatusBadRequest, fmt.Errorf("version %q: want a positive integer", v))
			return
		}
		version = n
	}
	ctx, cancel := context.WithTimeout(r.Context(), d.timeout(r))
	defer cancel()
	bodies, _, err := d.heldDefinitions(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	body, ok := bodies[t][version]
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Errorf("no definition for %s v%d", t, version))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}
