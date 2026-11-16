package ledger_test

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/store/ledger"
)

func pinMatrixPath(t *testing.T, kind string) string {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "ledger.db")
	if strings.HasPrefix(kind, "schema3") || kind == "carried" {
		path = buildV3(t)
	}
	if !strings.HasPrefix(kind, "schema3") {
		l, err := ledger.Create(ctx, path, person)
		if err != nil {
			t.Fatal(err)
		}
		closeIfOpen(l)
	}
	if strings.HasSuffix(kind, "damaged") {
		key := "person_id"
		if strings.HasPrefix(kind, "schema3") {
			key = "subject_id"
		}
		if _, err := rawDB(t, path).ExecContext(ctx, `INSERT INTO ledger_meta VALUES (?, 'other')`, key); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func TestSchema4_SubjectPinMatrix(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"schema3", "carried", "fresh", "schema3 damaged", "fresh damaged"} {
		for _, subject := range []string{person, "other"} {
			for name, open := range map[string]func(context.Context, string, string) (*ledger.Ledger, error){
				"Open": ledger.Open, "OpenReadOnly": ledger.OpenReadOnly, "Init": ledger.Create,
			} {
				t.Run(kind+"/"+subject+"/"+name, func(t *testing.T) {
					t.Parallel()
					path := pinMatrixPath(t, kind)
					db := rawDB(t, path)
					before := ledgerState(t, db)
					l, err := open(context.Background(), path, subject)
					closeIfOpen(l)
					checkPinOutcome(t, kind, subject, name, err)
					if err != nil {
						if after := ledgerState(t, db); !reflect.DeepEqual(after, before) {
							t.Errorf("refused %s changed the file", name)
						}
					}
				})
			}
		}
	}
}

func checkPinOutcome(t *testing.T, kind, subject, opener string, err error) {
	t.Helper()
	switch {
	case strings.HasSuffix(kind, "damaged"):
		if !errors.Is(err, ledger.ErrSchemaMismatch) {
			t.Fatalf("different subject pins = %v, want ErrSchemaMismatch", err)
		}
		for _, text := range []string{"subject_id pin", "person_id pin", `"test-person"`, `"other"`, "differ"} {
			if !strings.Contains(err.Error(), text) {
				t.Errorf("damaged-pin error %q does not name %s", err, text)
			}
		}
	case subject != person:
		if !errors.Is(err, ledger.ErrSubjectMismatch) {
			t.Fatalf("wrong subject = %v, want ErrSubjectMismatch before version check", err)
		}
		if !strings.Contains(err.Error(), `ledger is pinned to "test-person", config says "other"`) {
			t.Errorf("subject mismatch lost its pinned/configured values: %v", err)
		}
	case kind == "schema3" && opener != "Init":
		if !errors.Is(err, ledger.ErrSchemaMismatch) {
			t.Fatalf("schema 3 open = %v, want version refusal", err)
		}
		want := "holds schema 3, this build writes 4. Migrate it (`anam init`) or start a new ledger at another path. Do not delete it, it is the record"
		if !strings.Contains(err.Error(), want) {
			t.Errorf("schema 3 refusal lost its remedy: %v", err)
		}
	default:
		if err != nil {
			t.Fatalf("right subject = %v", err)
		}
	}
}
