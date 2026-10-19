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
	b, err := json.Marshal(struct {
		Relation string  `json:"relation"`
		Tuples   []Tuple `json:"tuples"`
	}{a.Relation, a.Tuples})
	if err != nil {
		return Answer{}, fmt.Errorf("query: serialise answer: %w", err)
	}
	sum := sha256.Sum256(b)
	a.bytes, a.hash = b, "sha256:"+hex.EncodeToString(sum[:])
	return a, nil
}

// Len is how many tuples the answer holds.
func (a Answer) Len() int { return len(a.Tuples) }

// Bytes is the answer's one serialisation.
func (a Answer) Bytes() []byte { return a.bytes }

// Hash is "sha256:<hex>" over Bytes.
func (a Answer) Hash() string { return a.hash }
