package query_test

import (
	"bufio"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/query"
	"github.com/mattjoyce/anam/internal/selection"
)

// docs/QUERY.md is held to the code. Each worked question in it is a fenced block
// whose info string names it, and a second block holds the rows it answers:
//
//	```datalog question=NAME relation=REL
//	...program...
//	```
//	```answer question=NAME
//	...one row per tuple, the terms separated by a space...
//	```
//
// A refused program is the same pair with refused=NAME in place of question=NAME,
// and an error block holding a fragment of the message. The test runs every program
// over the record the page describes, through a real ledger, so the page cannot
// drift from the evaluator.
const queryDoc = "../../docs/QUERY.md"

type docBlock struct {
	lang  string
	attrs map[string]string
	body  string
	line  int
}

func docBlocks(t *testing.T) []docBlock {
	t.Helper()
	f, err := os.Open(queryDoc)
	if err != nil {
		t.Fatalf("open %s: %v", queryDoc, err)
	}
	defer func() { _ = f.Close() }()
	var (
		out  []docBlock
		cur  *docBlock
		body []string
		n    int
	)
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		n++
		line := sc.Text()
		switch {
		case cur == nil && strings.HasPrefix(line, "```") && len(line) > 3:
			fields := strings.Fields(line[3:])
			cur = &docBlock{lang: fields[0], attrs: map[string]string{}, line: n}
			for _, kv := range fields[1:] {
				k, v, _ := strings.Cut(kv, "=")
				cur.attrs[k] = v
			}
			body = nil
		case cur != nil && strings.HasPrefix(line, "```"):
			cur.body = strings.Join(body, "\n")
			out = append(out, *cur)
			cur = nil
		case cur != nil:
			body = append(body, line)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// pairedWith is the block of lang that follows block i and names the same question.
func pairedWith(blocks []docBlock, i int, lang, key string) (docBlock, bool) {
	for _, b := range blocks[i+1:] {
		if b.lang == "datalog" {
			return docBlock{}, false
		}
		if b.lang == lang && b.attrs[key] == blocks[i].attrs[key] {
			return b, true
		}
	}
	return docBlock{}, false
}

// docsLedger is the record the page describes, served: the regimen, a note on the
// April reading, a week of intake and two nights' sleep.
func docsLedger(t *testing.T) (*harness, fold.Value, map[string]int) {
	t.Helper()
	h := edgeHarness(t)
	h.seedRegimen(t)
	v, _, _ := h.served(t)
	reading := ""
	for _, e := range v.Entries() {
		if e.Type == "health.blood-pressure" && strings.HasPrefix(e.FactTime, "2026-04-10") {
			reading = e.FacetID
		}
	}
	if reading == "" {
		t.Fatal("the April reading is not served")
	}
	h.admit(t, "records", "record.edge", "note:1", `{"id": "note:1", "at": "2026-04-10 18:00", "source_time": {"value": "2026-04-10 18:00", "zone": "Australia/Sydney", "offset": "+10:00", "basis": "source"}, "about": ["`+reading+`"], "text": "Before the dentist."}`)
	// An evening of wine, a beer the next evening, a coffee, and a beer after the last sleep, with
	// two nights' sleep: what the alcohol question on the page is asked of. The intake's id is
	// left out for the gate to compose.
	syd := func(wall string) string {
		return `"at": "` + wall + `", "source_time": {"value": "` + wall + `", "zone": "Australia/Sydney", "offset": "+11:00", "basis": "source"}`
	}
	for _, in := range []struct{ kind, at, body string }{
		{"alcohol", "2026-10-21 20:00", `"kind": "alcohol", "drink": "red-wine", "count": 2`},
		{"caffeine", "2026-10-22 08:15", `"kind": "caffeine", "drink": "flat-white", "count": 1`},
		{"alcohol", "2026-10-22 19:30", `"kind": "alcohol", "drink": "beer", "count": 1`},
		{"alcohol", "2026-10-23 21:00", `"kind": "alcohol", "drink": "beer", "count": 1`},
	} {
		wall := strings.Replace(in.at, " ", "T", 1)
		h.admit(t, "conversation", "health.intake", "conversation:intake/"+in.kind+"/"+wall+"+11:00", `{`+syd(in.at)+`, `+in.body+`}`)
	}
	for _, s := range []struct{ id, start, end string }{
		{"sleep:2026-10-22", "2026-10-21T12:30:00Z", "2026-10-21T20:00:00Z"},
		{"sleep:2026-10-23", "2026-10-22T12:00:00Z", "2026-10-22T19:30:00Z"},
	} {
		h.admit(t, "garmin", "health.sleep-episode", s.id, `{"id": "`+s.id+`", "effective_time_frame": {"time_interval": {"start_date_time": "`+s.start+`", "end_date_time": "`+s.end+`"}}, "source_time": {"value": "`+s.start+`", "zone": "UTC", "basis": "source"}}`)
	}
	v, versions, _ := h.served(t)
	return h, v, versions

}

func TestDocs_WorkedQuestionsRunAgainstARealLedger(t *testing.T) {
	t.Parallel()
	h, v, versions := docsLedger(t)

	blocks := docBlocks(t)
	ran := 0
	for i, b := range blocks {
		if b.lang != "datalog" {
			continue
		}
		name, refused := b.attrs["question"], false
		if name == "" {
			name, refused = b.attrs["refused"], true
		}
		if name == "" {
			continue // a program shown for its shape, not run
		}
		ran++
		t.Run(name, func(t *testing.T) {
			ans, err := tryRun(v, versions, h.defs, b.body, b.attrs["relation"])
			if refused {
				want, ok := pairedWith(blocks, i, "error", "refused")
				if !ok {
					t.Fatalf("line %d: no error block follows", b.line)
				}
				wantErr := query.ErrInvalid
				if strings.Contains(b.body, "avg") {
					wantErr = query.ErrSyntax
				}
				if !errors.Is(err, wantErr) || !strings.Contains(err.Error(), strings.TrimSpace(want.body)) {
					t.Errorf("line %d: error = %v, want ErrInvalid containing %q", b.line, err, strings.TrimSpace(want.body))
				}
				return
			}
			if err != nil {
				t.Fatalf("line %d: %v", b.line, err)
			}
			want, ok := pairedWith(blocks, i, "answer", "question")
			if !ok {
				t.Fatalf("line %d: no answer block follows", b.line)
			}
			var wantRows []string
			if strings.TrimSpace(want.body) != "" {
				wantRows = strings.Split(strings.TrimSpace(want.body), "\n")
			}
			if got := rows(ans); !same(got, wantRows) {
				t.Errorf("line %d: answer =\n%s\nthe page says\n%s", b.line, strings.Join(got, "\n"), strings.Join(wantRows, "\n"))
			}
		})
	}
	// The in_force form answers what the instant/1 form does: one question, two ways.
	var forms []string
	for _, name := range []string{"regimen-at-each-reading", "regimen-in-force"} {
		for _, b := range blocks {
			if b.lang == "answer" && b.attrs["question"] == name {
				forms = append(forms, strings.TrimSpace(b.body))
			}
		}
	}
	if len(forms) != 2 || forms[0] == "" || forms[0] != forms[1] {
		t.Errorf("the in_force form of the regimen question does not answer what the instant/1 form does:\n%q", forms)
	}
	if ran < 8 {
		t.Errorf("ran %d questions from %s, want at least 8", ran, queryDoc)
	}
}

// The selection builder's program, as the page shows it, is the text selection
// writes for that filter, so the page's account of its shape is the shape.
func TestDocs_SelectionProgramIsWhatTheBuilderWrites(t *testing.T) {
	t.Parallel()
	filters := map[string]selection.Filter{
		"summer-medication": {
			Types: []string{"health.medication."}, Sources: []string{"self-log"},
			// 1 June to 31 August in Sydney, where June is UTC+10.
			From: "2026-05-31T14:00:00.000000000Z", To: "2026-08-31T14:00:00.000000000Z", Exclusions: []string{"m4"}, Ticks: []string{"w1"},
		},
	}
	seen := 0
	for _, b := range docBlocks(t) {
		name := b.attrs["selection"]
		if b.lang != "datalog" || name == "" {
			continue
		}
		seen++
		f, ok := filters[name]
		if !ok {
			t.Fatalf("line %d: no filter named %q in the test", b.line, name)
		}
		got, err := f.ProgramIn("Australia/Sydney")
		if err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(got) != strings.TrimSpace(b.body) {
			t.Errorf("line %d: the builder writes\n%s\nthe page shows\n%s", b.line, got, b.body)
		}
	}
	if seen != len(filters) {
		t.Errorf("the page shows %d selection programs, the test builds %d", seen, len(filters))
	}
}
