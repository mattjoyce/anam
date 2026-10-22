package policy

import (
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/facet"
)

var (
	operator  = config.Principal{Name: config.OperatorName, Role: config.RoleOperator}
	reader    = config.Principal{Name: "plain", Role: config.RoleReader}
	recorder  = config.Principal{Name: "assistant", Role: config.RoleReader, Receipts: config.ReceiptsAlways}
	scale     = config.Principal{Name: "scale", Role: config.RoleImporter, Source: "scale", Types: []facet.Type{"health.weight"}}
	library   = config.Principal{Name: "docs", Role: config.RoleImporter, Source: "docs", Types: []facet.Type{"record.document"}}
	speaker   = config.Principal{Name: "conversation", Role: config.RoleImporter, Source: "conversation", AnyType: true}
	logger    = config.Principal{Name: "logger", Role: config.RoleImporter, Source: "logger", Types: []facet.Type{"health.weight"}, Receipts: config.ReceiptsAlways}
	workbench = config.Principal{Name: "workbench", Role: config.RoleImporter, Source: config.WorkbenchSource, Types: []facet.Type{"health.blood-pressure"}}
)

var (
	anyRole      = []config.Role{config.RoleReader, config.RoleImporter, config.RoleOperator}
	importerOnly = []config.Role{config.RoleImporter}
	operatorOnly = []config.Role{config.RoleOperator}
)

func TestAllow(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		req  Request
		want Answer
	}{
		{"no principal", Request{Roles: anyRole}, Unknown},
		{"no principal on an operator route", Request{Roles: operatorOnly}, Unknown},
		{"a reader reads", Request{Caller: reader, Known: true, Roles: anyRole}, Allowed},
		{"a reader may not submit", Request{Caller: reader, Known: true, Roles: importerOnly}, Forbidden},
		{"a reader may not mint", Request{Caller: reader, Known: true, Roles: operatorOnly}, Forbidden},
		{"an importer submits", Request{Caller: scale, Known: true, Roles: importerOnly}, Allowed},
		{"an importer may not open the inspector", Request{Caller: scale, Known: true, Roles: operatorOnly}, Forbidden},
		{"the operator mints", Request{Caller: operator, Known: true, Roles: operatorOnly}, Allowed},
		{"the operator submits nothing", Request{Caller: operator, Known: true, Roles: importerOnly}, Forbidden},
		{"a route no role reaches", Request{Caller: operator, Known: true}, Forbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := Allow(tt.req); got != tt.want {
				t.Errorf("Allow = %d, want %d", got, tt.want)
			}
		})
	}
}

// One rule answers the handlers and the gate: MaySubmitType and the scopes handed to
// the gate never disagree, and the scopes admit nothing for another source.
func TestSubmitAndRegister(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name               string
		p                  config.Principal
		t                  facet.Type
		submit, register   bool
		storeDocumentBytes bool
	}{
		{"an importer's own type", scale, "health.weight", true, true, false},
		{"another importer's type", scale, "health.blood-pressure", false, false, false},
		{"any_type submits every type", speaker, "health.weight", true, false, false},
		{"any_type owns none", speaker, "record.document", true, false, false},
		{"the library's importer", library, "record.document", true, true, true},
		{"the workbench's allow-list", workbench, "health.blood-pressure", true, true, false},
		{"off the workbench's list", workbench, "health.weight", false, false, false},
		{"a reader submits nothing", reader, "health.weight", false, false, false},
		{"the operator submits nothing", operator, "health.weight", false, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := MaySubmitType(tt.p, tt.t); got != tt.submit {
				t.Errorf("MaySubmitType = %v, want %v", got, tt.submit)
			}
			if got := ScopesOf(tt.p).Allowed(tt.p.Source, tt.t); got != tt.submit {
				t.Errorf("the gate's scopes allow %v, want %v", got, tt.submit)
			}
			if ScopesOf(tt.p).Allowed(tt.p.Source+"-other", tt.t) {
				t.Error("the scopes admit another source")
			}
			if got := MayRegisterType(tt.p, tt.t); got != tt.register {
				t.Errorf("MayRegisterType = %v, want %v", got, tt.register)
			}
			if got := MayStoreDocument(tt.p); got != tt.storeDocumentBytes {
				t.Errorf("MayStoreDocument = %v, want %v", got, tt.storeDocumentBytes)
			}
		})
	}
}

func TestReadsAndReceipts(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		p          config.Principal
		records    bool
		readerName string
		// opens is what MayOpenReceipt answers for a receipt naming assistant, one
		// naming nobody, and one naming logger.
		opens [3]bool
	}{
		{"the operator", operator, false, "", [3]bool{true, true, true}},
		{"an ordinary reader", reader, false, "", [3]bool{true, true, true}},
		{"an importer", scale, false, "", [3]bool{true, true, true}},
		{"a recording reader", recorder, true, "assistant", [3]bool{true, false, false}},
		{"a recording importer", logger, true, "logger", [3]bool{false, false, true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := Records(tt.p); got != tt.records {
				t.Errorf("Records = %v", got)
			}
			if got := ReaderOf(tt.p); got != tt.readerName {
				t.Errorf("ReaderOf = %q, want %q", got, tt.readerName)
			}
			if got := OpensAnyReceipt(tt.p); got == tt.records {
				t.Errorf("OpensAnyReceipt = %v for a token that records %v", got, tt.records)
			}
			for i, named := range []string{"assistant", "", "logger"} {
				if got := MayOpenReceipt(tt.p, named); got != tt.opens[i] {
					t.Errorf("MayOpenReceipt(%q) = %v, want %v", named, got, tt.opens[i])
				}
			}
		})
	}
	if err := OpenRefusal(recorder); err == nil || !strings.Contains(err.Error(), "assistant") {
		t.Errorf("OpenRefusal = %v, want a refusal naming the token", err)
	}
}

// A recording token's timeline read is a publish and must be one a receipt describes.
// dry_run has no rule: a query's dry run is open to every token.
func TestTimelineRead(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		p          config.Principal
		projection string
		explain    bool
		publish    bool
		refused    bool
	}{
		{"a plain read", reader, "", false, false, false},
		{"a plain read may explain", reader, "bp", true, false, false},
		{"a recorded read through a projection", recorder, "bp", false, true, false},
		{"a recorded read names a projection", recorder, "", false, true, true},
		{"a recorded read leaves out explain", recorder, "bp", true, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			publish, err := TimelineRead(tt.p, tt.projection, tt.explain)
			if publish != tt.publish || (err != nil) != tt.refused {
				t.Errorf("TimelineRead = %v, %v; want %v, refused %v", publish, err, tt.publish, tt.refused)
			}
		})
	}
}

// A recording token lists only the receipts that name it; the operator and an ordinary
// reader list every receipt, by any filter they ask; an importer sees only its own
// source's snapshots.
func TestListScope(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		p       config.Principal
		asked   string
		filter  string
		refused bool
		sees    map[string]bool // snapshot source -> visible
	}{
		{"the operator lists all", operator, "", "", false, map[string]bool{"scale": true, "docs": true}},
		{"the operator filters by anyone", operator, "assistant", "assistant", false, nil},
		{"an ordinary reader lists all", reader, "", "", false, map[string]bool{"scale": true}},
		{"an ordinary reader filters by anyone", reader, "assistant", "assistant", false, nil},
		{"a recording reader lists its own", recorder, "", "assistant", false, nil},
		{"a recording reader may name itself", recorder, "assistant", "assistant", false, nil},
		{"a recording reader may not list another's", recorder, "plain", "", true, nil},
		{"an importer lists all receipts", scale, "", "", false, map[string]bool{"scale": true, "docs": false}},
		{"a recording importer lists its own", logger, "", "logger", false, map[string]bool{"logger": true, "scale": false}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := ListScope(tt.p)
			got, err := s.Receipts(tt.asked)
			if got != tt.filter || (err != nil) != tt.refused {
				t.Errorf("Receipts(%q) = %q, %v; want %q, refused %v", tt.asked, got, err, tt.filter, tt.refused)
			}
			for source, want := range tt.sees {
				if got := s.Snapshot(source); got != want {
					t.Errorf("Snapshot(%s) = %v, want %v", source, got, want)
				}
			}
		})
	}
}

func TestDeclaredSources(t *testing.T) {
	t.Parallel()
	file := []config.Principal{reader, scale, workbench}
	tests := []struct {
		name     string
		file     []config.Principal
		source   string
		declared bool
	}{
		{"an importer's source", file, "scale", true},
		{"the workbench is declared", file, config.WorkbenchSource, true},
		{"a source nobody declares", file, "conversation", false},
		{"no importers file", nil, "scale", false},
		{"a reader's name is no source", file, "plain", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := Declared(tt.file, tt.source); got != tt.declared {
				t.Errorf("Declared = %v", got)
			}
			if got := MayRetract(tt.file, tt.source); got == tt.declared {
				t.Errorf("MayRetract = %v for declared %v", got, tt.declared)
			}
			if got := MayRetire(tt.file, tt.source); got == tt.declared {
				t.Errorf("MayRetire = %v for declared %v", got, tt.declared)
			}
		})
	}
}

// A held source submits nothing; the workbench is never held; the hold is what the
// request carries, read by the daemon inside the write.
func TestHeld(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		p    config.Principal
		hold Hold
		held bool
	}{
		{"no hold", scale, Hold{}, false},
		{"its source is held", scale, Hold{Sources: []string{"scale"}}, true},
		{"another source is held", scale, Hold{Sources: []string{"docs"}}, false},
		{"every source is held", scale, Hold{All: true}, true},
		{"any_type is held like any importer", speaker, Hold{All: true}, true},
		{"the workbench is never held", workbench, Hold{All: true, Sources: []string{config.WorkbenchSource}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := Held(Request{Caller: tt.p, Hold: tt.hold}); got != tt.held {
				t.Errorf("Held = %v, want %v", got, tt.held)
			}
			if got := Holdable(tt.p); got == (tt.p.Source == config.WorkbenchSource) {
				t.Errorf("Holdable = %v", got)
			}
			if got := MayHold(tt.p.Source); got != Holdable(tt.p) {
				t.Errorf("MayHold(%s) = %v, Holdable = %v", tt.p.Source, got, Holdable(tt.p))
			}
		})
	}
}
