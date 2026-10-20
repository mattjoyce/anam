package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/config"
)

// The description and the mux must agree (card 49): every route is a path in the
// document with the same roles, every path is a route, and every example decodes
// into the api type its operation names, with no field the type lacks.

// spec is the part of openapi.yaml the test reads.
type spec struct {
	Paths map[string]map[string]operation `yaml:"paths"`
}

type operation struct {
	Roles     []string `yaml:"x-anam-roles"`
	Type      string   `yaml:"x-anam-type"`
	Responses map[string]struct {
		Content map[string]struct {
			Example any `yaml:"example"`
		} `yaml:"content"`
	} `yaml:"responses"`
	RequestBody struct {
		Content map[string]struct {
			Example any `yaml:"example"`
		} `yaml:"content"`
	} `yaml:"requestBody"`
}

func loadSpec(t *testing.T) spec {
	t.Helper()
	var s spec
	if err := yaml.Unmarshal(openAPI, &s); err != nil {
		t.Fatalf("openapi.yaml: %v", err)
	}
	return s
}

// documented is every "METHOD /path" the spec describes, with its roles.
func documented(s spec) map[string][]string {
	out := map[string][]string{}
	for path, ops := range s.Paths {
		for method, op := range ops {
			out[strings.ToUpper(method)+" "+path] = op.Roles
		}
	}
	return out
}

// served is every pattern the mux registers that the spec must describe, with its
// roles: routes() less the inspector, which is a web page and not the API, plus the
// two routes Handler registers without a token.
func served(d *Daemon) map[string][]string {
	out := map[string][]string{"GET /healthz": {}, openAPIPath: {}}
	for _, rt := range d.routes() {
		if rt.pattern == "/" {
			continue
		}
		roles := make([]string, len(rt.roles))
		for i, r := range rt.roles {
			roles[i] = string(r)
		}
		out[rt.pattern] = roles
	}
	return out
}

func TestOpenAPI_DescribesEveryRouteAndNothingElse(t *testing.T) {
	d := open(t, newConfig(t))
	s := loadSpec(t)
	want, got := served(d), documented(s)
	for pattern, roles := range want {
		described, ok := got[pattern]
		if !ok {
			t.Errorf("%s is served and not described", pattern)
			continue
		}
		if !sameSet(roles, described) {
			t.Errorf("%s admits %v, the document says %v", pattern, roles, described)
		}
	}
	for pattern := range got {
		if _, ok := want[pattern]; !ok {
			t.Errorf("%s is described and not served", pattern)
		}
	}
}

// Every example decodes into the api type the operation names, strictly: a field in
// the example the type does not have is a document that drifted from the wire.
func TestOpenAPI_ExamplesDecodeIntoTheAPITypes(t *testing.T) {
	s := loadSpec(t)
	for path, ops := range s.Paths {
		for method, op := range ops {
			if op.Type == "" {
				continue
			}
			into, ok := apiTypes[op.Type]
			if !ok {
				t.Errorf("%s %s names x-anam-type %s, which this test does not know", method, path, op.Type)
				continue
			}
			for status, resp := range op.Responses {
				for media, c := range resp.Content {
					if c.Example == nil || media != "application/json" || !strings.HasPrefix(status, "2") {
						continue
					}
					if err := decodeStrict(c.Example, into()); err != nil {
						t.Errorf("%s %s %s example does not decode into api.%s: %v", method, path, status, op.Type, err)
					}
				}
			}
		}
	}
	if ex := s.Paths["/v1/entry"]["post"].RequestBody.Content["application/json"].Example; ex != nil {
		if err := decodeStrict(ex, new(api.EntryRequest)); err != nil {
			t.Errorf("POST /v1/entry request example does not decode into api.EntryRequest: %v", err)
		}
	}
	// The other request body with an example is a Submission.
	if ex := s.Paths["/v1/envelopes"]["post"].RequestBody.Content["application/json"].Example; ex != nil {
		if err := decodeStrict(ex, new(api.Submission)); err != nil {
			t.Errorf("POST /v1/envelopes request example does not decode into api.Submission: %v", err)
		}
	}
}

// The document is served to anyone, as YAML, and is the embedded bytes.
func TestOpenAPI_IsServedWithoutAToken(t *testing.T) {
	cfg := newConfig(t)
	cfg.TokenFile = filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(cfg.TokenFile, []byte("operator-token-0123456789abcdef0123456789abcdef\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	d := open(t, cfg)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	errc := make(chan error, 1)
	go func() { errc <- d.Serve(ctx, ln) }()
	t.Cleanup(func() { cancel(); <-errc })
	resp, err := http.Get("http://" + ln.Addr().String() + "/v1/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || !bytes.Equal(body, openAPI) {
		t.Fatalf("GET /v1/openapi.yaml = %d, %d bytes; want 200 and the embedded document", resp.StatusCode, len(body))
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/yaml" {
		t.Errorf("Content-Type = %q", ct)
	}
}

// apiTypes maps x-anam-type to a fresh value of the type, so an example can be
// decoded into it.
var apiTypes = map[string]func() any{
	"Status":               func() any { return new(api.Status) },
	"Timeline":             func() any { return new(api.Timeline) },
	"Changes":              func() any { return new(api.Changes) },
	"Coverage":             func() any { return new(api.Coverage) },
	"Receipts":             func() any { return new(api.Receipts) },
	"ReceiptShown":         func() any { return new(api.ReceiptShown) },
	"ReceiptVerified":      func() any { return new(api.ReceiptVerified) },
	"FacetShown":           func() any { return new(api.FacetShown) },
	"Published":            func() any { return new(api.Published) },
	"Projections":          func() any { return new(api.Projections) },
	"RegisteredProjection": func() any { return new(api.RegisteredProjection) },
	"Checkpoints":          func() any { return new(api.Checkpoints) },
	"Mint":                 func() any { return new(api.Mint) },
	"Definitions":          func() any { return new(api.Definitions) },
	"Registered":           func() any { return new(api.Registered) },
	"SnapshotReport":       func() any { return new(api.SnapshotReport) },
	"Admission":            func() any { return new(api.Admission) },
	"Refusal":              func() any { return new(api.Refusal) },
	"StoredDocument":       func() any { return new(api.StoredDocument) },
	"Backup":               func() any { return new(api.Backup) },
	"Retraction":           func() any { return new(api.Retraction) },
	"Entry":                func() any { return new(api.Entry) },
	"QueryCatalogue":       func() any { return new(api.QueryCatalogue) },
	"QueryAnswer":          func() any { return new(api.QueryAnswer) },
}

// decodeStrict round-trips a YAML example through JSON into v, refusing unknown
// fields. The YAML decoder yields map[string]any for mappings, which encodes to JSON
// as is.
func decodeStrict(example any, v any) error {
	b, err := json.Marshal(example)
	if err != nil {
		return fmt.Errorf("example to JSON: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	x, y := append([]string{}, a...), append([]string{}, b...)
	sort.Strings(x)
	sort.Strings(y)
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}

// Keep the roles' spelling tied to the config's, so a renamed role fails here.
var _ = []config.Role{config.RoleReader, config.RoleImporter, config.RoleOperator}
