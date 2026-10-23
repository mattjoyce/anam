package daemon

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/projection"
	"github.com/mattjoyce/anam/internal/store/ops"
)

// projections is the registry reads select a template from and receipts verify
// against: the bundled templates and every version the operator registered (card
// 125). It is replaced whole when a registration lands, so a reader always holds one
// consistent set.
type projections struct {
	bundled *projection.Registry
	current atomic.Pointer[projection.Registry]
}

// loadProjections builds the registry from the bundled templates and those the
// operational store holds as registered. A stored body this binary cannot hold
// beside the bundled ones stops the daemon: the store is authored state and comes
// back from a backup, and a daemon that silently dropped a template would verify
// receipts against the wrong one. A default_projection that names neither a bundled
// nor a registered template also stops it: only here are the registered ones visible,
// and an operator who mistyped the name should learn at start, not from an agent.
func loadProjections(ctx context.Context, db *ops.DB, defaultName string) (*projections, error) {
	bundled, err := projection.Bundled()
	if err != nil {
		return nil, err
	}
	rows, err := db.RegisteredProjections(ctx)
	if err != nil {
		return nil, err
	}
	reg, err := projection.WithRegistered(bundled, projectionStored(rows))
	if err != nil {
		return nil, fmt.Errorf("daemon: registered projections: %w", err)
	}
	if defaultName != "" {
		if _, err := reg.Lookup(defaultName); err != nil {
			return nil, fmt.Errorf("daemon: default_projection: %w", err)
		}
	}
	p := &projections{bundled: bundled}
	p.current.Store(reg)
	return p, nil
}

func projectionStored(rows []ops.ProjectionRow) []projection.Stored {
	out := make([]projection.Stored, len(rows))
	for i, r := range rows {
		out[i] = projection.Stored{Name: r.Name, Version: r.Version, Body: []byte(r.Body)}
	}
	return out
}

// postProjections registers a template the operator wrote. Every type it names must
// have a Definition in force; a held version is immutable, so the same body again is
// unchanged and anything else at that version is refused.
func (d *Daemon) postProjections(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, int64(d.cfg.MaxManifestBytes)))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, err)
		return
	}
	var reg projection.Registration
	err = d.tryWriteWithin(r.Context(), d.cfg.ImporterWait, d.timeout(r), func(ctx context.Context, s Stores) error {
		var err error
		reg, err = d.registerProjection(ctx, s.Ops, body)
		return err
	})
	switch {
	case errors.Is(err, projection.ErrBadTemplate), errors.Is(err, projection.ErrUndefinedType):
		writeError(w, http.StatusUnprocessableEntity, err)
	case errors.Is(err, projection.ErrRegistered), errors.Is(err, projection.ErrVersionBehind):
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
		writeJSON(w, status, api.RegisteredProjection{
			Name: reg.Template.Name, Version: reg.Template.Version, Hash: reg.Template.Hash, New: reg.New,
		})
	}
}

// registerProjection decides a registration against everything held and, when it is
// new, stores it and replaces the registry. The registry is built before the row is
// stored, so a row the daemon could not start on is never written.
func (d *Daemon) registerProjection(ctx context.Context, db *ops.DB, body []byte) (projection.Registration, error) {
	defs := d.defs.current.Load()
	defined := func(t facet.Type) bool { _, ok := defs.Lookup(t); return ok }
	reg, err := projection.Register(body, d.projections.current.Load(), defined)
	if err != nil || !reg.New {
		return reg, err
	}
	rows, err := db.RegisteredProjections(ctx)
	if err != nil {
		return projection.Registration{}, err
	}
	row := ops.ProjectionRow{Name: reg.Template.Name, Version: reg.Template.Version, Body: string(body)}
	next, err := projection.WithRegistered(d.projections.bundled, projectionStored(append(rows, row)))
	if err != nil {
		return projection.Registration{}, err
	}
	if err := db.RegisterProjection(ctx, row, clock.Format(time.Now())); err != nil {
		return projection.Registration{}, err
	}
	d.projections.current.Store(next)
	return reg, nil
}

// getProjections lists every template a consumer may select, with its newest version
// and every version held.
func (d *Daemon) getProjections(w http.ResponseWriter, _ *http.Request) {
	reg := d.projections.current.Load()
	out := api.Projections{Default: d.cfg.DefaultProjection, Projections: []api.ProjectionSummary{}}
	for _, cur := range reg.All() {
		s := api.ProjectionSummary{
			Name: cur.Name, Origin: d.projectionOrigin(cur), Current: cur.Version,
			Description: cur.Description, Shape: string(cur.Shape), Hash: cur.Hash, Types: []string{},
		}
		s.Types = append(s.Types, cur.Patterns()...)
		for _, v := range reg.Versions(cur.Name) {
			s.Versions = append(s.Versions, v.Version)
		}
		sort.Ints(s.Versions)
		out.Projections = append(out.Projections, s)
	}
	writeJSON(w, http.StatusOK, out)
}

// projectionOrigin says whether a template's newest version ships with this build or
// was registered over it.
func (d *Daemon) projectionOrigin(t projection.Template) string {
	if b, err := d.projections.bundled.Version(t.Name, t.Version); err == nil && b.Hash == t.Hash {
		return api.OriginBundled
	}
	return api.OriginRegistered
}

// getProjection answers the manifest of {name} at {version}, or at its newest version
// when none is named, as canonical JSON.
func (d *Daemon) getProjection(w http.ResponseWriter, r *http.Request) {
	reg := d.projections.current.Load()
	name := r.PathValue("name")
	t, err := reg.Lookup(name)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if v := r.PathValue("version"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			writeError(w, http.StatusBadRequest, fmt.Errorf("version %q: want a positive integer", v))
			return
		}
		if t, err = reg.Version(name, n); err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(t.Body)
}
