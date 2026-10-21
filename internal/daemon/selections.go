package daemon

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/selection"
)

// The saved-selection routes (card 206). Saving writes an artifact and never the
// ledger, so, like publishing a receipt, it is every role's and takes no turn on the
// write queue; the artifact store's writes are atomic.

// postSelection saves a program and answers its code. The body is bounded as a
// query's is: a program is text a person or an agent wrote.
func (d *Daemon) postSelection(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, int64(d.cfg.MaxManifestBytes)))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, err)
		return
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	var req api.SelectionRequest
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("selection request: %w", err))
		return
	}
	saved, err := d.selections.Save(req.Program)
	if err != nil {
		writeSelectionError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, selectionOf(saved))
}

// getSelection answers the program a code names.
func (d *Daemon) getSelection(w http.ResponseWriter, r *http.Request) {
	saved, err := d.selections.Open(r.PathValue("code"))
	if err != nil {
		writeSelectionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, selectionOf(saved))
}

func selectionOf(s selection.Saved) api.Selection {
	return api.Selection{Code: s.Code, Hash: s.Hash, URL: "/?sel=" + url.QueryEscape(s.Code), Program: s.Program}
}

// writeSelectionError answers 400 for text that is not a program or a string that is
// not a code, 404 for a code nothing was saved under, 409 for one naming two, 500
// otherwise.
func writeSelectionError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, selection.ErrNotProgram), errors.Is(err, selection.ErrBadCode):
		writeError(w, http.StatusBadRequest, err)
	case errors.Is(err, selection.ErrNoCode):
		writeError(w, http.StatusNotFound, err)
	case errors.Is(err, selection.ErrAmbiguous):
		writeError(w, http.StatusConflict, err)
	default:
		writeError(w, http.StatusInternalServerError, err)
	}
}
