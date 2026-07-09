package facet

import "github.com/google/uuid"

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
