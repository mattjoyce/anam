package facet

import (
	"strconv"
	"strings"

	"github.com/google/uuid"
)

// namespaceUUID is the fixed UUIDv5 namespace for Anam facet ids.
//
// It must never change. Every facet id Anam has ever minted derives from it, and the
// ids are the ledger's identity: a new namespace would make every stored facet
// unreachable from its source row and every re-ingest a duplicate.
var namespaceUUID = uuid.MustParse("22123d0e-64ee-4412-9c1f-b00c380fcd43")

// NewID derives a Facet's deterministic id from its source ref, so the same source row
// always maps to the same logical facet however often it is captured. Together with
// the atomic conditional write, this is what makes commit idempotent and ingest
// crash-clean (§6a).
func NewID(sourceRef string) string {
	return uuid.NewSHA1(namespaceUUID, []byte(sourceRef)).String()
}

// SourceRef composes a source row's identity from the source's name and the identity
// values in declared order: "<source>:" then each value as "<byte length>:<bytes>",
// joined with ':'. It is the one composition rule, used by the gate (from the
// Definition's identity_key) and by anything that must know an id before the gate
// does, such as retraction's plan, so the two always agree.
//
// The values are length-prefixed rather than escaped because the values are
// external input and contain anything: a blood pressure timestamp carries colons, a
// records id carries a lab's own prefix. A plain join let two different tuples compose
// one string; with the length in front, no two tuples do, and the ref still reads as
// the values it was made from.
func SourceRef(source string, values ...string) string {
	return Stamp(source, values...)
}

// Stamp is the length-prefixed composition SourceRef uses, exposed for the other
// identity-shaped stamp Anam composes the same way: a Definition's series stamp,
// "<series name>:" over the series key's values.
func Stamp(prefix string, values ...string) string {
	var b strings.Builder
	b.WriteString(prefix)
	for _, v := range values {
		b.WriteByte(':')
		b.WriteString(strconv.Itoa(len(v)))
		b.WriteByte(':')
		b.WriteString(v)
	}
	return b.String()
}
