package app

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/frame"
)

// A read that names its fields answers with a frame, the same locally and over the
// daemon: the envelope's columns then the pointers, one row per reading, and nothing
// of the payload but those fields (card 138).
func TestTimeline_FieldsAnswerWithAFrame(t *testing.T) {
	t.Parallel()
	f := newRemoteFixtureIn(t, "Europe/Berlin")
	// A nested pointer and a top-level one: the frame cuts source_time to its basis.
	read := []string{"timeline", "--type", "test.measure", "--field", "/source_time/basis", "--field", "/note"}

	out := f.sameOutput(t, read...)
	for _, want := range []string{"shown=3", "/SOURCE_TIME/BASIS", "/NOTE", "FACET_ID", "configuration", "70.0"} {
		if !strings.Contains(out, want) {
			t.Errorf("the frame does not show %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "/SEQ") || strings.Contains(out, "/COUNT") || strings.Contains(out, "Europe/Berlin") {
		t.Errorf("a field not named reached the answer:\n%s", out)
	}

	asJSON := f.sameOutput(t, append(read, "--json")...)
	fr, err := frame.Parse([]byte(asJSON))
	if err != nil {
		t.Fatalf("--json is not a frame: %v\n%s", err, asJSON)
	}
	if fr.Len() != 3 || len(fr.Columns) != len(frame.Envelope)+2 {
		t.Errorf("frame has %d rows and %d columns", fr.Len(), len(fr.Columns))
	}
	if fr.Columns[len(fr.Columns)-1] != "/note" {
		t.Errorf("columns = %v", fr.Columns)
	}
	// Narrowing composes under the cut: a window keeps one row.
	if out := f.sameOutput(t, append(read, "--to", "2026-01-31")...); !strings.Contains(out, "shown=1") {
		t.Errorf("a windowed fields read:\n%s", out)
	}
}

// A field the schema does not declare is refused before anything is read, locally
// and over the daemon alike; so is a read with no type that names a field some
// served type lacks, and an explained read with fields.
func TestTimeline_FieldsAreCheckedAgainstTheSchema(t *testing.T) {
	t.Parallel()
	f := newRemoteFixture(t)
	for _, tt := range []struct {
		name string
		args []string
		want string
	}{
		{"undeclared", []string{"timeline", "--type", "test.measure", "--field", "/nope"}, `declares no field /nope`},
		{"no type", []string{"timeline", "--field", "/note"}, `declares no field /note`},
		{"not a pointer", []string{"timeline", "--type", "test.measure", "--field", "pulse"}, `must start with '/'`},
		{"explained", []string{"timeline", "--explain", "--field", "/note"}, `explain shows verdicts`},
	} {
		for _, cfg := range []string{f.cfgPath, f.remoteCfg} {
			_, _, err := cli(t, cfg, tt.args...)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("%s with %s: %v, want %q", tt.name, cfg, err, tt.want)
			}
		}
	}
}

// A published read with fields files the frame under a receipt that names the
// fields; the receipt verifies, and show renders the frame from the artifact alone.
func TestPublish_FieldsFileAFrameThatVerifiesAndShows(t *testing.T) {
	t.Parallel()
	f := newRemoteFixture(t)
	read := []string{"timeline", "--projection", "measures", "--field", "/source_time/basis"}

	shown := f.sameOutput(t, read...)
	published := f.sameOutput(t, append(read, "--publish")...)
	if published != shown {
		t.Errorf("a publish shows another frame than the read\n--- read\n%s--- published\n%s", shown, published)
	}

	_, stderr, err := cli(t, f.remoteCfg, append(read, "--publish")...)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	hash := receiptOf(t, stderr)
	if out := f.sameOutput(t, "receipt", "verify", hash); !strings.Contains(out, "OK   reproduced") {
		t.Errorf("the fields receipt did not reproduce:\n%s", out)
	}
	show := f.sameOutput(t, "receipt", "show", hash)
	for _, want := range []string{"fields /source_time/basis", "/SOURCE_TIME/BASIS", "configuration"} {
		if !strings.Contains(show, want) {
			t.Errorf("receipt show lacks %q:\n%s", want, show)
		}
	}
	asJSON := f.sameOutput(t, "receipt", "show", "--json", hash)
	if _, err := frame.Parse([]byte(asJSON)); err != nil {
		t.Errorf("receipt show --json of a fields receipt is not a frame: %v\n%s", err, asJSON)
	}
	var cols struct {
		Columns []string `json:"columns"`
	}
	if json.Unmarshal([]byte(asJSON), &cols) != nil || cols.Columns[len(cols.Columns)-1] != "/source_time/basis" {
		t.Errorf("the shown frame's columns: %v", cols.Columns)
	}
}

// Fields are a narrowing the bound counts: a whole read over max_read_bytes is
// refused naming fields among what is left, and the same read cut to one field fits.
func TestTimeline_FieldsNarrowAReadUnderTheBound(t *testing.T) {
	t.Parallel()
	// A bound the three whole readings just cross, and their frame of one field,
	// which is smaller by the rest of the payload, does not.
	f := newRemoteFixtureWith(t, "UTC", func(t *testing.T, f remoteFixture, cfg *config.Config) {
		whole, _, err := cli(t, f.cfgPath, "timeline", "--json")
		if err != nil {
			t.Fatal(err)
		}
		cfg.MaxReadBytes = len(whole) - 2 // the array without its newline, less one
	})

	_, _, err := cli(t, f.remoteCfg, "timeline", "--type", "test.measure")
	refused := tooLarge(t, err, "the whole type")
	if got := strings.Join(refused.TooLarge.Narrow, " "); got != "from to at projection fields" {
		t.Errorf("narrow = %q, want fields among what is left", got)
	}

	out, _, err := cli(t, f.remoteCfg, "timeline", "--type", "test.measure", "--field", "/note")
	if err != nil || !strings.Contains(out, "shown=3") {
		t.Errorf("the same read cut to one field: %v\n%s", err, out)
	}
}
