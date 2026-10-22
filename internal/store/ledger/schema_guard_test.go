package ledger

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"regexp"
	"slices"
	"testing"
)

// The ledger's schema lives in ledger.sql and indexes.sql, embedded. These pins hold
// the text and the version together, as change detection and nothing more.
//
// A changed hash does not prove the stored layout changed, and an unchanged one does
// not prove its meaning did not (version 3 changed no DDL at all). Compatibility is a
// decision, recorded here on purpose by whoever makes it:
//
//   - the file changed what an existing ledger holds, or means: bump schemaVersion, add
//     the step to migrations (migrate.go), and update both pins below;
//   - the change is compatible, a comment, whitespace, or an addition an older ledger
//     gains on open: leave schemaVersion alone and update only the hash below, in the
//     same commit, with a line saying why it is compatible.
//
// Whether the triggers still hold is not decided here. TestAppendOnly_* and
// TestSeq_*, TestVersion_Database* in appendonly_test.go do that, against the file.
const (
	pinnedSchemaVersion = 3
	pinnedLedgerSQL     = "906d1cda1bdc769932ef7cc8a9c09ed2845ee5c6437b9c943b007a553e738ea0"
	pinnedIndexesSQL    = "24a1caf45c986d7ce20a4aa4e8d96efddde74365b4dbc481ea6764bed4a34eb0"
)

func sha(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func TestSchemaFiles_AreHeldToTheirVersion(t *testing.T) {
	t.Parallel()
	if schemaVersion != pinnedSchemaVersion {
		t.Errorf("schemaVersion is %d, the pin says %d. A bump is a decision about stored meaning: "+
			"check that ledger.sql and the migrations agree with it, then update pinnedSchemaVersion "+
			"and pinnedLedgerSQL together", schemaVersion, pinnedSchemaVersion)
	}
	if got := sha(schema); got != pinnedLedgerSQL {
		t.Errorf("ledger.sql changed (sha256 %s). If it changes what an existing ledger holds or "+
			"means, bump schemaVersion and add a migration. If it is a comment or another change an "+
			"older ledger accepts as it is, update pinnedLedgerSQL deliberately and say why in the commit", got)
	}
	if got := sha(indexes); got != pinnedIndexesSQL {
		t.Errorf("indexes.sql changed (sha256 %s). Indexes are installed on every writable open and "+
			"do not bump schemaVersion; update pinnedIndexesSQL deliberately", got)
	}
}

// wantTriggers is the error kernel's inventory: the append-only triggers the schema
// installs, by name. A trigger of the same name can still have a weaker body, which is
// what the behavioural tests in appendonly_test.go refuse; this catches one going
// missing, from the file or from a ledger created out of it.
var wantTriggers = []string{
	"anam_event_no_delete", "anam_event_no_update", "anam_event_seq_follows",
	"facet_no_delete", "facet_no_replace", "facet_no_update", "facet_version_follows",
	"ledger_meta_no_delete", "ledger_meta_no_replace", "ledger_meta_no_update",
}

func TestSchema_EveryTriggerIsInTheFileAndInANewLedger(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	var inText []string
	for _, m := range regexp.MustCompile(`(?m)^CREATE TRIGGER IF NOT EXISTS (\w+) `).FindAllStringSubmatch(schema, -1) {
		inText = append(inText, m[1])
	}
	slices.Sort(inText)
	if !slices.Equal(inText, wantTriggers) {
		t.Errorf("ledger.sql creates triggers %v, want %v", inText, wantTriggers)
	}

	path := filepath.Join(t.TempDir(), "ledger.db")
	l, err := Create(ctx, path, internalPerson)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })

	rows, err := outsider(t, path).QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'trigger' ORDER BY name`)
	if err != nil {
		t.Fatalf("list triggers: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var inFile []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan: %v", err)
		}
		inFile = append(inFile, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("list triggers: %v", err)
	}
	if !slices.Equal(inFile, wantTriggers) {
		t.Errorf("a new ledger holds triggers %v, want %v", inFile, wantTriggers)
	}
}
