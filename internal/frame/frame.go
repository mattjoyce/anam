// Package frame is the shape a read answers in when it names its fields (card 138):
// columns, the envelope's attributes then the pointers, and one row per facet in
// timeline order. A relation, as a query returns one, rather than a list of facets;
// it reads straight into a DataFrame.
//
// Fields only take away. A frame is cut from whole facets a projection and the
// read's narrowing already admitted, so it can neither resurrect nor exclude a facet
// and never shows a field the whole read would not have. The cut is made on the
// served copy of each payload; the stored payload stays opaque bytes.
package frame

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/fold"
)

// Envelope is the columns every frame starts with, in this order: what identifies
// each row's facet without its payload. content_hash is the whole payload's, so a
// row still names the fact it was cut from.
var Envelope = []string{"facet_id", "type", "fact_time", "source", "status", "classification", "content_hash"}

// Frame is one read's answer in its fields: Columns, then Rows, one per facet in the
// order the entries came in (timeline order). A cell is the value's own JSON, a
// number as its source lexeme; a pointer a row lacks is null.
type Frame struct {
	Columns []string            `json:"columns"`
	Rows    [][]json.RawMessage `json:"rows"`

	bytes []byte
	hash  string
}

var null = json.RawMessage("null")

// Of cuts entries to fields: every envelope column, then one column per pointer, in
// the order given. The pointers are taken as given and checked only for syntax; a
// pointer no row resolves is a column of nulls. Checking them against the schema
// is the read's job, before the cut (def.Definition.HasPointer).
func Of(entries []fold.Entry, fields []string) (Frame, error) {
	if len(fields) == 0 {
		return Frame{}, errors.New("frame: no fields named")
	}
	for _, p := range fields {
		if err := def.CheckPointer(p); err != nil {
			return Frame{}, fmt.Errorf("frame: %w", err)
		}
	}
	f := Frame{
		Columns: append(append(make([]string, 0, len(Envelope)+len(fields)), Envelope...), fields...),
		Rows:    make([][]json.RawMessage, 0, len(entries)),
	}
	for _, e := range entries {
		row, err := rowOf(e, fields)
		if err != nil {
			return Frame{}, err
		}
		f.Rows = append(f.Rows, row)
	}
	if err := f.seal(); err != nil {
		return Frame{}, err
	}
	return f, nil
}

// rowOf is one entry's row: the envelope cells, then the payload at each pointer.
func rowOf(e fold.Entry, fields []string) ([]json.RawMessage, error) {
	row := make([]json.RawMessage, 0, len(Envelope)+len(fields))
	for _, s := range []string{e.FacetID, string(e.Type), e.FactTime, e.Source, string(e.Status), string(e.Classification), e.ContentHash} {
		cell, err := json.Marshal(s)
		if err != nil {
			return nil, fmt.Errorf("frame: %w", err)
		}
		row = append(row, cell)
	}
	doc, err := def.Decode(e.Payload)
	if err != nil {
		return nil, fmt.Errorf("frame: facet %s: %w", e.FacetID, err)
	}
	for _, p := range fields {
		v, err := def.Resolve(doc, p)
		if err != nil {
			// The pointer is well formed (checked above), so this row lacks it.
			row = append(row, null)
			continue
		}
		cell, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("frame: facet %s at %s: %w", e.FacetID, p, err)
		}
		row = append(row, cell)
	}
	return row, nil
}

// seal pins the frame's one serialisation and its hash.
func (f *Frame) seal() error {
	b, err := json.Marshal(struct {
		Columns []string            `json:"columns"`
		Rows    [][]json.RawMessage `json:"rows"`
	}{f.Columns, f.Rows})
	if err != nil {
		return fmt.Errorf("frame: serialise: %w", err)
	}
	sum := sha256.Sum256(b)
	f.bytes, f.hash = b, "sha256:"+hex.EncodeToString(sum[:])
	return nil
}

// Parse reads a frame written by Bytes, as a receipt's served artifact is read back.
func Parse(b []byte) (Frame, error) {
	var f Frame
	if err := json.Unmarshal(b, &f); err != nil {
		return Frame{}, fmt.Errorf("frame: parse: %w", err)
	}
	return f, nil
}

// UnmarshalJSON reads a frame as it travels (the daemon's answer, a served artifact)
// and seals it again, so a frame decoded on the client has the bytes and the hash
// the server's had.
func (f *Frame) UnmarshalJSON(b []byte) error {
	type plain Frame // no methods: a plain decode, not this one again
	var p plain
	if err := json.Unmarshal(b, &p); err != nil {
		return err
	}
	if p.Columns == nil || p.Rows == nil {
		return errors.New("not a frame (no columns or rows)")
	}
	*f = Frame{Columns: p.Columns, Rows: p.Rows}
	return f.seal()
}

// Len is how many rows the frame holds.
func (f Frame) Len() int { return len(f.Rows) }

// Bytes is the frame's one serialisation: what a served artifact holds.
func (f Frame) Bytes() []byte { return f.bytes }

// Hash is "sha256:<hex>" over Bytes: the served hash a receipt records for a read
// that named fields.
func (f Frame) Hash() string { return f.hash }
