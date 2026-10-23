package inspect

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/mattjoyce/anam/internal/catchup"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
)

// The Definitions pages are reference: what each type's contract says, at every
// version Anam holds, rendered from the Definitions alone. They read no facet and
// count nothing (card 71, Matt's decision); a type links into the timeline filter for
// its facts instead.

// HeldDefinitions answers every Definition body Anam holds, by type and version, and
// where each type's came from (bundled or registered). The daemon serves the same
// answer at GET /v1/definitions; nil means the bundled manifests alone.
type HeldDefinitions func(ctx context.Context) (map[facet.Type]map[int][]byte, map[facet.Type]string, error)

// held is s's held Definitions, or the bundled manifests when none were given.
func (s *Server) held(ctx context.Context) (map[facet.Type]map[int][]byte, map[facet.Type]string, error) {
	if s.heldDefs != nil {
		return s.heldDefs(ctx)
	}
	manifests, err := def.Manifests()
	if err != nil {
		return nil, nil, err
	}
	bodies := map[facet.Type]map[int][]byte{}
	origin := map[facet.Type]string{}
	for _, m := range manifests {
		if bodies[m.Type] == nil {
			bodies[m.Type] = map[int][]byte{}
		}
		bodies[m.Type][m.Version] = m.Body
		origin[m.Type] = "bundled"
	}
	return bodies, origin, nil
}

// defSummary is one row of the Definitions index.
type defSummary struct {
	Type           facet.Type
	Current        int
	Versions       []int
	Origin         string
	Classification facet.Classification
	// Kind and Family are what the Definition declares (card 176), or "" for none.
	Kind      def.Kind
	Family    string
	Fields    int
	Described int
	// Enterable is set for a type the Workbench enters by hand (card 188), and
	// EnterURL is its form.
	Enterable bool
	EnterURL  string
}

func (s *Server) definitionsIndex(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), s.timeout)
	defer cancel()
	bodies, origin, err := s.held(ctx)
	if err != nil {
		httpError(w, err)
		return
	}
	reg, err := s.definitions()
	if err != nil {
		httpError(w, err)
		return
	}
	var rows []defSummary
	for _, d := range reg.All() {
		row := defSummary{Type: d.Type, Current: d.Version, Origin: origin[d.Type], Classification: d.Classification,
			Kind: d.Kind, Family: d.Family, Enterable: s.enterable(d.Type), EnterURL: enterURL(d.Type)}
		for v := range bodies[d.Type] {
			row.Versions = append(row.Versions, v)
		}
		sort.Ints(row.Versions)
		if body, ok := bodies[d.Type][d.Version]; ok {
			if fields, err := fieldsOf(body, d); err == nil {
				row.Fields, row.Described = len(fields), described(fields)
			}
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Type < rows[j].Type })
	s.render(w, "definitions.html", struct {
		Definitions []defSummary
		// Bench is set when this inspector has a Workbench at all; without one the
		// Enter column says so rather than offering links that do not exist.
		Bench bool
	}{rows, s.bench != nil})
}

// definitionData is one Definition at one version.
type definitionData struct {
	Def         def.Definition
	Origin      string
	Current     int
	Versions    []int
	Description string
	Fields      []fieldRow
	Described   int
	ParseError  string
	Raw         []jsonToken
	TimelineURL string

	// Guidance is the handling block's `guidance` prose, and Handling the rest of the
	// block, highlighted; both empty when the Definition declares none (card 166).
	Guidance string
	Handling []jsonToken

	// EnterURL is the form for this type when the Workbench enters it and this is its
	// current version; empty otherwise.
	EnterURL string
}

// handlingOf splits a Definition's handling block for the page: the `guidance` key's
// prose, shown as text, and whatever else the block holds, shown as JSON. The block
// is the type's own and Anam reads nothing in it, so the page shows all of it.
func handlingOf(d def.Definition) (string, []jsonToken) {
	if len(d.Handling) == 0 {
		return "", nil
	}
	var block map[string]json.RawMessage
	if err := json.Unmarshal(d.Handling, &block); err != nil {
		return "", highlightJSON(d.Handling)
	}
	var guidance string
	if raw, ok := block["guidance"]; ok {
		if err := json.Unmarshal(raw, &guidance); err == nil {
			delete(block, "guidance")
		}
	}
	if len(block) == 0 {
		return guidance, nil
	}
	rest, err := json.Marshal(block)
	if err != nil {
		return guidance, highlightJSON(d.Handling)
	}
	return guidance, highlightJSON(rest)
}

func (s *Server) definitionPage(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), s.timeout)
	defer cancel()
	t := facet.Type(r.URL.Query().Get("type"))
	if t == "" {
		http.Error(w, "missing ?type=<facet type>", http.StatusBadRequest)
		return
	}
	reg, err := s.definitions()
	if err != nil {
		httpError(w, err)
		return
	}
	bodies, origin, err := s.held(ctx)
	if err != nil {
		httpError(w, err)
		return
	}
	versions := bodies[t]
	if len(versions) == 0 {
		http.Error(w, fmt.Sprintf("no definition held for %s", t), http.StatusNotFound)
		return
	}
	data := definitionData{Origin: origin[t], TimelineURL: "/?type=" + url.QueryEscape(string(t))}
	for v := range versions {
		data.Versions = append(data.Versions, v)
		data.Current = max(data.Current, v)
	}
	sort.Ints(data.Versions)
	if cur, ok := reg.Lookup(t); ok {
		data.Current = cur.Version
	}

	v := data.Current
	if q := r.URL.Query().Get("v"); q != "" {
		n, err := strconv.Atoi(q)
		if err != nil || n < 1 {
			http.Error(w, fmt.Sprintf("?v=%q: want a positive integer", q), http.StatusBadRequest)
			return
		}
		v = n
	}
	body, ok := versions[v]
	if !ok {
		http.Error(w, fmt.Sprintf("%s v%d is not held", t, v), http.StatusNotFound)
		return
	}
	data.Raw = highlightJSON(body)
	d, err := def.Parse(body)
	if err != nil {
		// An old body this build would refuse is still what was in force: show it raw.
		data.ParseError = err.Error()
		data.Def = def.Definition{Type: t, Version: v}
		s.render(w, "definition.html", data)
		return
	}
	data.Def = d
	data.Guidance, data.Handling = handlingOf(d)
	if data.Fields, err = fieldsOf(body, d); err != nil {
		data.ParseError = err.Error()
	}
	data.Described = described(data.Fields)
	data.Description = schemaDescription(body)
	if s.enterable(t) && v == data.Current {
		data.EnterURL = enterURL(t)
	}
	s.render(w, "definition.html", data)
}

// fieldRow is one field of a Definition's payload schema, keyed by its JSON Pointer.
// A field inside an array's items is written with "[]" after the array's name, and a
// field inside each value of a map with "/*" after the map's name: display forms, as
// no meta pointer reaches into either.
type fieldRow struct {
	Pointer     string
	Depth       int
	Name        string
	Type        string
	Required    bool
	Roles       []string
	Description string
	Examples    []string
	Notes       []string
}

// numericText is the pattern the manifests use for a number the source writes as text.
const numericText = `^-?[0-9]+(\.[0-9]+)?$`

// fieldsOf walks a manifest's payload schema into one row per field, in declaration
// order where the schema is read in order, and marks the rows the meta points at.
func fieldsOf(body []byte, d def.Definition) ([]fieldRow, error) {
	var m struct {
		Schema json.RawMessage `json:"schema"`
	}
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, err
	}
	if len(m.Schema) == 0 {
		return nil, nil // verbatim: no schema
	}
	var root map[string]any
	if err := json.Unmarshal(m.Schema, &root); err != nil {
		return nil, err
	}
	roles := map[string][]string{}
	for _, p := range d.IdentityKey {
		roles[p] = append(roles[p], "identity")
	}
	if d.FactTime.Pointer != "" {
		roles[d.FactTime.Pointer] = append(roles[d.FactTime.Pointer], "fact time")
	}
	if d.Series != nil {
		for _, p := range d.Series.Key {
			roles[p] = append(roles[p], "series")
		}
	}
	var rows []fieldRow
	walkFields(root, "", 0, roles, &rows)
	return rows, nil
}

func walkFields(node map[string]any, at string, depth int, roles map[string][]string, rows *[]fieldRow) {
	props, _ := node["properties"].(map[string]any)
	required := map[string]bool{}
	if list, ok := node["required"].([]any); ok {
		for _, r := range list {
			if name, ok := r.(string); ok {
				required[name] = true
			}
		}
	}
	names := make([]string, 0, len(props))
	for name := range props {
		names = append(names, name)
	}
	// Required fields first, then the rest, each alphabetical: JSON objects carry no
	// order once decoded, and the fields that key and place a facet are required.
	sort.Slice(names, func(i, j int) bool {
		if required[names[i]] != required[names[j]] {
			return required[names[i]]
		}
		return names[i] < names[j]
	})
	for _, name := range names {
		sub, _ := props[name].(map[string]any)
		ptr := at + "/" + name
		row := fieldRow{
			Pointer: ptr, Depth: depth, Name: name, Required: required[name],
			Type: typeOf(sub), Roles: roles[ptr], Notes: notesOf(sub),
		}
		row.Description, _ = sub["description"].(string)
		if ex, ok := sub["examples"].([]any); ok {
			for _, e := range ex {
				b, _ := json.Marshal(e)
				row.Examples = append(row.Examples, string(b))
			}
		}
		*rows = append(*rows, row)
		if _, ok := sub["properties"].(map[string]any); ok {
			walkFields(sub, ptr, depth+1, roles, rows)
		}
		if items, ok := sub["items"].(map[string]any); ok {
			if _, ok := items["properties"].(map[string]any); ok {
				walkFields(items, ptr+"[]", depth+1, roles, rows)
			}
		}
		if values, ok := sub["additionalProperties"].(map[string]any); ok {
			if _, ok := values["properties"].(map[string]any); ok {
				walkFields(values, ptr+"/*", depth+1, roles, rows)
			}
		}
	}
}

// typeOf names a field's type the way a reader says it.
func typeOf(sub map[string]any) string {
	types := def.TypesOf(sub)
	if len(types) == 0 {
		if _, ok := sub["enum"]; ok {
			return "enum"
		}
		return "any"
	}
	out := make([]string, 0, len(types))
	for _, t := range types {
		switch t {
		case "array":
			item := "any"
			if items, ok := sub["items"].(map[string]any); ok {
				item = typeOf(items)
			}
			out = append(out, "array of "+item)
		case "object":
			if ap, ok := sub["additionalProperties"].(map[string]any); ok {
				out = append(out, "map of "+typeOf(ap))
			} else {
				out = append(out, "object")
			}
		case "string":
			if p, _ := sub["pattern"].(string); p == numericText {
				out = append(out, "number as text")
			} else {
				out = append(out, "string")
			}
		default:
			out = append(out, t)
		}
	}
	return strings.Join(out, " or ")
}

// notesOf states the constraints a field carries beyond its type, leaving out the two
// every manifest repeats (non-empty strings, closed objects), which the page states
// once.
func notesOf(sub map[string]any) []string {
	var notes []string
	if enum, ok := sub["enum"].([]any); ok {
		vals := make([]string, 0, len(enum))
		for _, v := range enum {
			b, _ := json.Marshal(v)
			vals = append(vals, string(b))
		}
		if len(vals) <= 8 {
			notes = append(notes, "one of "+strings.Join(vals, ", "))
		} else {
			notes = append(notes, fmt.Sprintf("one of %d values", len(vals)))
		}
	}
	if c, ok := sub["const"]; ok {
		b, _ := json.Marshal(c)
		notes = append(notes, "always "+string(b))
	}
	if p, _ := sub["pattern"].(string); p != "" && p != numericText {
		notes = append(notes, "pattern "+p)
	}
	for _, k := range []string{"format", "minimum", "maximum", "exclusiveMinimum", "exclusiveMaximum", "minItems", "maxItems"} {
		if v, ok := sub[k]; ok {
			notes = append(notes, fmt.Sprintf("%s %v", k, v))
		}
	}
	return notes
}

func described(rows []fieldRow) int {
	n := 0
	for _, r := range rows {
		if r.Description != "" {
			n++
		}
	}
	return n
}

// schemaDescription is the top-level schema's description: what one facet of the
// type is, in a line.
func schemaDescription(body []byte) string {
	var m struct {
		Schema struct {
			Description string `json:"description"`
		} `json:"schema"`
	}
	_ = json.Unmarshal(body, &m) // already parsed by def.Parse; a miss is an empty line
	return m.Schema.Description
}

// enterURL is the form that enters a fact of type t by hand: the link form for
// record.edge, whose sides are lists the entry form does not draw (card 202), the entry
// form drawn from the Definition for any other.
func enterURL(t facet.Type) string {
	if t == catchup.EdgeType {
		return "/link"
	}
	return "/entry?type=" + url.QueryEscape(string(t))
}
