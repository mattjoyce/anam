package ops

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

// The operational store's schema lives in ops.sql, embedded. These pins hold the text
// and the version together, as change detection and nothing more.
//
// A changed hash does not prove the layout changed. Several additions here are
// documented as compatible and never bumped the version (registered_definition, the
// snapshot tables), and a comment edit changes the bytes too. Compatibility is a
// decision, recorded here on purpose by whoever makes it:
//
//   - the file changes what an existing store holds, or means: bump schemaVersion (see
//     its history in ops.go for the carry), and update both pins below;
//   - the change is compatible, a comment, or a table an older store gains on open
//     without misleading either build: leave schemaVersion alone and update only the
//     hash below, in the same commit, with a line saying why it is compatible.
const (
	pinnedSchemaVersion  = 9
	pinnedOpsSQL         = "2e757b443b5709a3a42e097b35a2137b2bfd3d91699e57fea4f7f07db1cfc7aa"
	pinnedSnapshotDigest = "825ac2dab2735474a64c9030a5944f77845e2da208f33ba336687072e60f7edf"
)

func TestSchemaFile_IsHeldToItsVersion(t *testing.T) {
	t.Parallel()
	if schemaVersion != pinnedSchemaVersion {
		t.Errorf("schemaVersion is %d, the pin says %d. A bump is a decision about stored meaning: "+
			"check that ops.sql and the carry agree with it, then update pinnedSchemaVersion "+
			"and pinnedOpsSQL together", schemaVersion, pinnedSchemaVersion)
	}
	sum := sha256.Sum256([]byte(schema))
	if got := hex.EncodeToString(sum[:]); got != pinnedOpsSQL {
		t.Errorf("ops.sql changed (sha256 %s). If it changes what an existing store holds or means, "+
			"bump schemaVersion and make sure the carry covers it. If it is a comment or another "+
			"change an older store accepts as it is, update pinnedOpsSQL deliberately and say why in the commit", got)
	}
}

func TestMigrationFiles_AreHeldToTheirPins(t *testing.T) {
	t.Parallel()
	sum := sha256.Sum256([]byte(snapshotPartDigest))
	if got := hex.EncodeToString(sum[:]); got != pinnedSnapshotDigest {
		t.Errorf("migrations/snapshot-part-digest.sql changed (sha256 %s). An older store runs it as "+
			"written when it opens. A change to what it does needs a new step and, if stored meaning "+
			"moves, a schemaVersion bump; a comment-only change may update pinnedSnapshotDigest "+
			"deliberately, saying why in the commit", got)
	}
}
