package daemon

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/config"
)

const importerToken = "importer-token-0123456789abcdef0123456789"

// readingManifest is a Definition no binary bundles, as an importer's repo carries it.
const readingManifest = `{
  "meta": {
    "facet_type": "test.reading",
    "definition_version": 1,
    "classification": "Normal",
    "validation_mode": "strict",
    "source_schema": "test/reading",
    "time_semantics": {"fact_time": {"pointer": "/at", "layout": "2006-01-02T15:04:05Z07:00", "zone": "UTC"}},
    "identity_key": ["/id"]
  },
  "schema": {
    "$schema": "https://json-schema.org/draft/2020-12/schema",
    "type": "object",
    "required": ["id", "at", "value"],
    "properties": {
      "id": {"type": "string", "minLength": 1},
      "at": {"type": "string", "minLength": 1},
      "value": {"type": "number"}
    },
    "additionalProperties": false
  }
}`

// importerConfig is a config with the operator's token and one importer, test-import,
// which may submit health.weight and test.reading.
func importerConfig(t *testing.T) config.Config {
	t.Helper()
	cfg := newConfig(t)
	dir := t.TempDir()
	cfg.TokenFile = writeToken(t, dir, testToken)
	sum := sha256.Sum256([]byte(importerToken))
	cfg.ImportersFile = filepath.Join(dir, "importers.yaml")
	body := fmt.Sprintf("importers:\n  - source: test-import\n    token_sha256: %s\n    types: [health.weight, test.reading]\n",
		hex.EncodeToString(sum[:]))
	if err := os.WriteFile(cfg.ImportersFile, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return cfg
}

// serveImporters opens and runs a daemon on cfg and returns its URL and how to stop it.
func serveImporters(t *testing.T, cfg config.Config) (*Daemon, string, func() error) {
	t.Helper()
	d := open(t, cfg)
	stop := start(t, d)
	srv := httptest.NewServer(d.Handler())
	t.Cleanup(srv.Close)
	return d, srv.URL, stop
}

// call sends body to url under token and decodes the answer into into, failing unless
// the status is want.
func call(t *testing.T, method, url, token, body string, want int, into any) {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != want {
		t.Fatalf("%s %s: %d, want %d: %s", method, url, resp.StatusCode, want, raw)
	}
	if into != nil {
		if err := json.Unmarshal(raw, into); err != nil {
			t.Fatalf("%s %s: decode: %v: %s", method, url, err, raw)
		}
	}
}

func weighIn(id string, kg float64, version int) string {
	return fmt.Sprintf(`{"type": "health.weight", "definition_version": %d,
		"payload": {"id": %q, "date": "2026-10-09 07:30", "source_time": {"value": "2026-10-09 07:30", "zone": "Australia/Sydney", "offset": "+11:00", "basis": "source"}, "kg": %v}}`, version, id, kg)
}

func batch(envs ...string) string { return `{"envelopes": [` + strings.Join(envs, ",") + `]}` }

func head(t *testing.T, d *Daemon) int64 {
	t.Helper()
	w, err := d.read.Head(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return int64(w)
}

// An importer may not hold the operator's token: it would submit as the importer and
// run the daemon too.
func TestOpenRefusesAnImporterWithTheOperatorsToken(t *testing.T) {
	cfg := importerConfig(t)
	sum := sha256.Sum256([]byte(testToken))
	body := fmt.Sprintf("importers:\n  - source: x\n    token_sha256: %s\n    types: [health.weight]\n", hex.EncodeToString(sum[:]))
	if err := os.WriteFile(cfg.ImportersFile, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(context.Background(), cfg, Options{}); !errors.Is(err, config.ErrInvalidImporters) {
		t.Fatalf("Open: got %v, want ErrInvalidImporters", err)
	}
}

// Card 122: a bundled type may be named by two importers, since no importer can
// register a version of it, and each submits under its own source, so the same id
// from both is two facets. A type no binary bundles still has one owner.
func TestImportersShareABundledType(t *testing.T) {
	cfg := importerConfig(t)
	const secondToken = "second-importer-token-0123456789abcdef01"
	first, second := sha256.Sum256([]byte(importerToken)), sha256.Sum256([]byte(secondToken))
	write := func(types string) {
		t.Helper()
		body := fmt.Sprintf("importers:\n  - source: test-import\n    token_sha256: %s\n    types: [health.weight, test.reading]\n"+
			"  - source: second-import\n    token_sha256: %s\n    types: [%s]\n",
			hex.EncodeToString(first[:]), hex.EncodeToString(second[:]), types)
		if err := os.WriteFile(cfg.ImportersFile, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	write("health.weight, test.reading")
	if _, err := Open(context.Background(), cfg, Options{}); !errors.Is(err, config.ErrInvalidImporters) {
		t.Fatalf("Open with an unbundled type named twice: got %v, want ErrInvalidImporters", err)
	}

	write("health.weight")
	d, url, _ := serveImporters(t, cfg)
	weight, _ := d.defs.current.Load().Lookup("health.weight")
	var a api.Admission
	for _, token := range []string{importerToken, secondToken} {
		call(t, "POST", url+"/v1/envelopes", token, batch(weighIn("w-1", 80.5, weight.Version)), http.StatusOK, &a)
		if a.Changed != 1 || a.Results[0].Outcome != "admitted" {
			t.Fatalf("submission under %s: %+v", token[:6], a)
		}
	}
	if got := head(t, d); got != 2 {
		t.Fatalf("W = %d, want 2: one facet per source", got)
	}
}

// Card 177: an importer with any_type submits every type the daemon knows, as its
// own source, and owns none: it registers no Definition, and a type with no
// Definition is refused to it as to anyone.
func TestAnyTypeImporterSubmitsEverythingAndDefinesNothing(t *testing.T) {
	cfg := importerConfig(t)
	const spokenToken = "spoken-importer-token-0123456789abcdef0123"
	first, spoken := sha256.Sum256([]byte(importerToken)), sha256.Sum256([]byte(spokenToken))
	body := fmt.Sprintf("importers:\n  - source: test-import\n    token_sha256: %s\n    types: [health.weight, test.reading]\n"+
		"  - source: conversation\n    token_sha256: %s\n    any_type: true\n",
		hex.EncodeToString(first[:]), hex.EncodeToString(spoken[:]))
	if err := os.WriteFile(cfg.ImportersFile, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	d, url, _ := serveImporters(t, cfg)

	// test-import owns test.reading and registers it; the conversation may not. Nor
	// may it store a document: any_type reaches facets, not the library.
	call(t, "POST", url+"/v1/definitions", spokenToken, readingManifest, http.StatusForbidden, nil)
	call(t, "POST", url+"/v1/definitions", importerToken, readingManifest, http.StatusCreated, nil)
	call(t, "POST", url+"/v1/documents?ext=pdf", spokenToken, "%PDF-1.4 spoken", http.StatusForbidden, nil)

	// The conversation submits a bundled type and a registered one, each a facet of
	// its own beside the owner's.
	weight, _ := d.defs.current.Load().Lookup("health.weight")
	reading, _ := d.defs.current.Load().Lookup("test.reading")
	readingEnv := fmt.Sprintf(`{"type": "test.reading", "definition_version": %d,
		"payload": {"id": "r-1", "at": "2026-10-09T07:30:00Z", "value": 7}}`, reading.Version)
	var a api.Admission
	for _, token := range []string{spokenToken, importerToken} {
		call(t, "POST", url+"/v1/envelopes", token, batch(weighIn("w-1", 80.5, weight.Version), readingEnv), http.StatusOK, &a)
		if a.Changed != 2 || a.Results[0].Outcome != "admitted" || a.Results[1].Outcome != "admitted" {
			t.Fatalf("submission under %s: %+v", token[:6], a)
		}
	}
	if got := head(t, d); got != 4 {
		t.Fatalf("W = %d, want 4: two facets per source", got)
	}
	unknown := `{"type": "test.unknown", "definition_version": 1, "payload": {"id": "u-1"}}`
	call(t, "POST", url+"/v1/envelopes", spokenToken, batch(unknown), http.StatusUnprocessableEntity, nil)
}

// Card 53: an importer's token opens the importer routes, and the operator's token
// submits nothing. Since card 89 an importer also reads (roles_test.go).
func TestImporterRoutesAuthorize(t *testing.T) {
	_, url, _ := serveImporters(t, importerConfig(t))

	tests := []struct {
		name, method, path, token string
		want                      int
	}{
		{"envelopes with no token", "POST", "/v1/envelopes", "", http.StatusUnauthorized},
		{"envelopes with the operator's token", "POST", "/v1/envelopes", testToken, http.StatusForbidden},
		{"envelopes with an unknown token", "POST", "/v1/envelopes", testToken + "x", http.StatusUnauthorized},
		{"definitions with the operator's token", "GET", "/v1/definitions", testToken, http.StatusOK},
		{"definitions with an importer's token", "GET", "/v1/definitions", importerToken, http.StatusOK},
		{"definitions with no token", "GET", "/v1/definitions", "", http.StatusUnauthorized},
		{"the record with an importer's token", "GET", "/v1/status", importerToken, http.StatusOK},
		{"the inspector with an importer's token", "GET", "/", importerToken, http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			call(t, tt.method, url+tt.path, tt.token, `{"envelopes": []}`, tt.want, nil)
		})
	}
}

// Card 53: a batch is admitted whole, again is a no-op, and a refused batch writes
// nothing and names every envelope that failed.
func TestPostEnvelopes(t *testing.T) {
	d, url, _ := serveImporters(t, importerConfig(t))
	weight, _ := d.defs.current.Load().Lookup("health.weight")
	v := weight.Version

	var a api.Admission
	call(t, "POST", url+"/v1/envelopes", importerToken, batch(weighIn("w-1", 80.5, v), weighIn("w-2", 80.1, v)), http.StatusOK, &a)
	if a.Changed != 2 || a.W != 2 || a.Results[1].Outcome != "admitted" || a.Results[1].FacetID == "" {
		t.Fatalf("first batch: %+v", a)
	}
	call(t, "POST", url+"/v1/envelopes", importerToken, batch(weighIn("w-1", 80.5, v), weighIn("w-2", 79.9, v)), http.StatusOK, &a)
	if a.Changed != 1 || a.Results[0].Outcome != "no-op" || a.Results[1].Outcome != "superseded" || a.W != 3 {
		t.Fatalf("second batch: %+v", a)
	}

	var r api.Refusal
	call(t, "POST", url+"/v1/envelopes", importerToken, batch(
		weighIn("w-3", 81, v),
		weighIn("w-4", 81, v+1),
		`{"type": "health.weight", "definition_version": `+fmt.Sprint(v)+`, "payload": {"id": "w-5", "date": "2026-10-09 07:30"}}`,
		`{"type": "health.sleep", "definition_version": 1, "payload": {}}`,
	), http.StatusUnprocessableEntity, &r)
	if len(r.Refused) != 3 || r.Refused[0].Index != 1 || r.Refused[1].Index != 2 || r.Refused[2].Index != 3 {
		t.Fatalf("refusal: %+v", r)
	}
	for i, want := range []string{"definition version", "kg", "not authorized"} {
		if !strings.Contains(r.Refused[i].Reason, want) {
			t.Errorf("refused[%d] = %q, want it to mention %q", i, r.Refused[i].Reason, want)
		}
	}
	if got := head(t, d); got != 3 {
		t.Fatalf("W = %d after a refused batch, want 3", got)
	}

	for name, body := range map[string]string{
		"a source named":        `{"envelopes": [{"type": "health.weight", "definition_version": 1, "source": "x", "payload": {}}]}`,
		"no definition version": `{"envelopes": [{"type": "health.weight", "payload": {}}]}`,
		"no envelopes":          `{"envelopes": []}`,
		"more than a batch":     batch(strings.Repeat(weighIn("w", 1, v)+",", 49) + weighIn("w", 1, v)),
		"not JSON":              `envelopes`,
	} {
		t.Run(name, func(t *testing.T) {
			call(t, "POST", url+"/v1/envelopes", importerToken, body, http.StatusBadRequest, nil)
		})
	}

	st := d.sources.statuses()
	if len(st) != 1 || st[0].Source != "test-import" || st[0].LastAdmissionAt == "" {
		t.Fatalf("source status = %+v", st)
	}
}

// Card 109: the body is one submission and nothing else. A valid batch followed by a
// second value, by garbage, or by enough whitespace to carry the body over its bound
// is refused whole, before the queue: the ledger does not move, no snapshot part is
// recorded, and the source is not marked as seen. A batch padded to the bound exactly
// is still a submission.
func TestPostEnvelopesReadTheWholeBody(t *testing.T) {
	d, url, _ := serveImporters(t, importerConfig(t))
	weight, _ := d.defs.current.Load().Lookup("health.weight")
	one := batch(weighIn("w-1", 80.5, weight.Version))

	for name, tc := range map[string]struct {
		body string
		want int
	}{
		"a second submission": {one + ` {"envelopes": []}`, http.StatusBadRequest},
		"a second value":      {one + ` 1`, http.StatusBadRequest},
		"garbage":             {one + ` garbage`, http.StatusBadRequest},
		"whitespace past the bound": {
			one + strings.Repeat(" ", int(maxSubmission(config.DefaultMaxPayloadBytes))-len(one)+1), http.StatusRequestEntityTooLarge},
		"a snapshot part with garbage": {
			snapBatch("torn", 1, 1, weighIn("w-1", 80.5, weight.Version)) + ` garbage`, http.StatusBadRequest},
	} {
		t.Run(name, func(t *testing.T) {
			call(t, "POST", url+"/v1/envelopes", importerToken, tc.body, tc.want, nil)
		})
	}
	if got := head(t, d); got != 0 {
		t.Fatalf("W = %d after refused bodies, want 0", got)
	}
	call(t, "GET", url+"/v1/snapshots/torn", importerToken, "", http.StatusNotFound, nil)
	if st := d.sources.statuses(); len(st) != 0 {
		t.Fatalf("a refused body marked the source as seen: %+v", st)
	}

	var a api.Admission
	call(t, "POST", url+"/v1/envelopes", importerToken, one+"\n  \t\n", http.StatusOK, &a)
	if a.Changed != 1 || a.W != 1 {
		t.Fatalf("batch with trailing whitespace: %+v", a)
	}
	call(t, "POST", url+"/v1/envelopes", importerToken, one+strings.Repeat(" ", int(maxSubmission(config.DefaultMaxPayloadBytes))-len(one)), http.StatusOK, &a)
	if a.Changed != 0 || a.W != 1 || a.Results[0].Outcome != "no-op" {
		t.Fatalf("batch padded to the bound: %+v", a)
	}
}

// Card 53: an importer registers the Definitions it emits; a registered version is
// immutable; envelopes of the type are admitted against it, and still are after a
// restart.
// A handling block registers with its manifest, comes back verbatim on the GET, and a
// changed block at the same version is another body (card 166).
func TestDefinitions_HandlingRoundTrips(t *testing.T) {
	_, url, _ := serveImporters(t, importerConfig(t))
	handled := strings.Replace(readingManifest, `"schema"`,
		`"handling": {"guidance": "value is the meter's reading, not a mean", "date_basis": ["observed"]}, "schema"`, 1)
	var reg api.Registered
	call(t, "POST", url+"/v1/definitions", importerToken, handled, http.StatusCreated, &reg)
	call(t, "POST", url+"/v1/definitions", importerToken, strings.Replace(handled, "a mean", "a median", 1), http.StatusConflict, nil)
	call(t, "POST", url+"/v1/definitions", importerToken,
		strings.Replace(readingManifest, `"schema"`, `"handling": "be kind", "schema"`, 1), http.StatusUnprocessableEntity, nil)

	var body map[string]any
	call(t, "GET", url+"/v1/definitions/test.reading", testToken, "", http.StatusOK, &body)
	want := map[string]any{"guidance": "value is the meter's reading, not a mean", "date_basis": []any{"observed"}}
	if got, _ := body["handling"].(map[string]any); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("GET handling = %v, want %v", body["handling"], want)
	}
}

func TestPostDefinitions(t *testing.T) {
	cfg := importerConfig(t)
	d, url, stop := serveImporters(t, cfg)
	post := func(body string, want int) api.Registered {
		t.Helper()
		var reg api.Registered
		into := any(&reg)
		if want >= 300 {
			into = nil
		}
		call(t, "POST", url+"/v1/definitions", importerToken, body, want, into)
		return reg
	}

	if reg := post(readingManifest, http.StatusCreated); !reg.New || reg.Type != "test.reading" || reg.Version != 1 {
		t.Fatalf("first registration: %+v", reg)
	}
	if reg := post(strings.ReplaceAll(readingManifest, "\n", " "), http.StatusOK); reg.New {
		t.Fatalf("the same body again was new: %+v", reg)
	}
	post(strings.Replace(readingManifest, `"minLength": 1}`, `"minLength": 2}`, 1), http.StatusConflict)
	post(strings.Replace(readingManifest, `"test.reading"`, `"test.other"`, 1), http.StatusForbidden)
	post(`{"meta": {}}`, http.StatusUnprocessableEntity)
	weight, _ := d.defs.bundled.Lookup("health.weight")
	bundledBody, err := os.ReadFile("../def/manifests/health.weight.json")
	if err != nil {
		t.Fatal(err)
	}
	post(string(bytes.Replace(bundledBody, []byte(fmt.Sprintf(`"definition_version": %d`, weight.Version)),
		[]byte(fmt.Sprintf(`"definition_version": %d`, weight.Version+1)), 1)), http.StatusConflict)
	call(t, "POST", url+"/v1/definitions", testToken, readingManifest, http.StatusForbidden, nil)

	reading := `{"type": "test.reading", "definition_version": 1, "payload": {"id": "r-1", "at": "2026-10-09T07:30:00Z", "value": 4}}`
	var a api.Admission
	call(t, "POST", url+"/v1/envelopes", importerToken, batch(reading), http.StatusOK, &a)
	if a.Changed != 1 {
		t.Fatalf("admission: %+v", a)
	}

	var list api.Definitions
	call(t, "GET", url+"/v1/definitions", importerToken, "", http.StatusOK, &list)
	found := false
	for _, s := range list.Definitions {
		if s.Type == "test.reading" {
			found = s.Origin == api.OriginRegistered && s.Current == 1 && len(s.Versions) == 1
		}
	}
	if !found {
		t.Fatalf("test.reading is not listed as registered v1: %+v", list)
	}
	var body map[string]any
	call(t, "GET", url+"/v1/definitions/test.reading/1", importerToken, "", http.StatusOK, &body)
	call(t, "GET", url+"/v1/definitions/test.reading", testToken, "", http.StatusOK, &body)
	call(t, "GET", url+"/v1/definitions/test.reading/2", testToken, "", http.StatusNotFound, nil)
	call(t, "GET", url+"/v1/definitions/test.none", testToken, "", http.StatusNotFound, nil)

	// A restart admits against what was registered before it.
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	_, url, _ = serveImporters(t, cfg)
	reading2 := strings.Replace(reading, "r-1", "r-2", 1)
	call(t, "POST", url+"/v1/envelopes", importerToken, batch(reading2), http.StatusOK, &a)
	if a.Changed != 1 || a.W != 2 {
		t.Fatalf("after restart: %+v", a)
	}
}

// Card 53: an importer's admission is minted without being asked for.
func TestPostEnvelopesMints(t *testing.T) {
	d, url, _ := serveImporters(t, importerConfig(t))
	weight, _ := d.defs.current.Load().Lookup("health.weight")
	call(t, "POST", url+"/v1/envelopes", importerToken, batch(weighIn("w-1", 80.5, weight.Version)), http.StatusOK, nil)
	waitFor(t, "a mint at W=1", func() bool {
		w, ok, err := d.readOps.LastMintRun(context.Background(), 1)
		return err == nil && ok && w == 1
	})
}
