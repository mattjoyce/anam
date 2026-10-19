package query

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
)

// Answer is one relation of a result as a value: its tuples in one order, with one
// serialisation and a hash over it, so two evaluations of one program over one value
// answer identically and a receipt can pin what was answered.
type Answer struct {
	Relation string  `json:"relation"`
	Tuples   []Tuple `json:"tuples"`

	bytes []byte
	hash  string
}

// Relation returns the named relation of the result as an Answer. A relation the
// program never named is an error, not an empty answer.
func (r Result) Relation(name string) (Answer, error) {
	rel, ok := r.relations[name]
	if !ok {
		return Answer{}, fmt.Errorf("%w: no relation %s in the result", ErrInvalid, name)
	}
	tuples := make([]Tuple, len(rel.tuples))
	copy(tuples, rel.tuples)
	sort.Slice(tuples, func(i, j int) bool { return tupleLess(tuples[i], tuples[j]) })
	a := Answer{Relation: name, Tuples: tuples}
	if err := a.seal(); err != nil {
		return Answer{}, err
	}
	return a, nil
}

// seal pins the answer's one serialisation and its hash.
func (a *Answer) seal() error {
	b, err := json.Marshal(struct {
		Relation string  `json:"relation"`
		Tuples   []Tuple `json:"tuples"`
	}{a.Relation, a.Tuples})
	if err != nil {
		return fmt.Errorf("query: serialise answer: %w", err)
	}
	sum := sha256.Sum256(b)
	a.bytes, a.hash = b, "sha256:"+hex.EncodeToString(sum[:])
	return nil
}

// UnmarshalJSON reads an answer as it travels and seals it again, so an answer
// decoded on the client, or read back from a served artifact, has the bytes and
// the hash the server's had.
func (a *Answer) UnmarshalJSON(b []byte) error {
	type plain Answer // no methods: a plain decode, not this one again
	var p plain
	if err := json.Unmarshal(b, &p); err != nil {
		return err
	}
	if p.Relation == "" || p.Tuples == nil {
		return fmt.Errorf("%w: not an answer (no relation or tuples)", ErrSyntax)
	}
	*a = Answer{Relation: p.Relation, Tuples: p.Tuples}
	return a.seal()
}

// Len is how many tuples the answer holds.
func (a Answer) Len() int { return len(a.Tuples) }

// Bytes is the answer's one serialisation.
func (a Answer) Bytes() []byte { return a.bytes }

// Hash is "sha256:<hex>" over Bytes.
func (a Answer) Hash() string { return a.hash }
