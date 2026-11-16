package ledger_test

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/store/ledger"
)

func TestSchema4_CarriedFileIsAppendOnly(t *testing.T) {
	t.Parallel()
	statements := map[string]string{
		"update binding": `UPDATE facet SET binding_subject_id = 'other' WHERE facet_id = 'f1'`,
		"delete facet":   `DELETE FROM facet WHERE facet_id = 'f1'`,
		"replace binding": `INSERT OR REPLACE INTO facet
			SELECT facet_id, artifact_version, type, confidence, classification,
			       definition_version, fact_time, series, 'other', binding_assurance,
			       binding_span_hint, prov_source, prov_injector, prov_source_ref, prov_method,
			       prov_model, prov_watermark, source_schema, schema_version, content_hash,
			       payload_kind, media_type, payload FROM facet WHERE facet_id = 'f1'`,
		"update event": `UPDATE anam_event SET kind = 'tombstoned' WHERE seq = 1`,
		"delete event": `DELETE FROM anam_event WHERE seq = 4`,
		"replace event": `INSERT OR REPLACE INTO anam_event
			SELECT * FROM anam_event WHERE seq = 4`,
		"update subject pin":  `UPDATE ledger_meta SET value = 'other' WHERE key = 'subject_id'`,
		"delete subject pin":  `DELETE FROM ledger_meta WHERE key = 'subject_id'`,
		"replace subject pin": `INSERT OR REPLACE INTO ledger_meta VALUES ('subject_id', 'other')`,
		"update legacy pin":   `UPDATE ledger_meta SET value = 'other' WHERE key = 'person_id'`,
		"delete legacy pin":   `DELETE FROM ledger_meta WHERE key = 'person_id'`,
		"replace legacy pin":  `INSERT OR REPLACE INTO ledger_meta VALUES ('person_id', 'other')`,
	}
	for name, stmt := range statements {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			path := buildV3(t)
			l, err := ledger.Create(ctx, path, person)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = l.Close() })
			db := rawDB(t, path) // plain outsider connection, no package pragmas
			before := ledgerState(t, db)
			if _, err := db.ExecContext(ctx, stmt); err == nil || !strings.Contains(err.Error(), "ledger") {
				t.Errorf("%s: got %v, want ledger trigger refusal", name, err)
			}
			if after := ledgerState(t, db); !reflect.DeepEqual(after, before) {
				t.Errorf("%s changed the carried file", name)
			}
		})
	}
}
