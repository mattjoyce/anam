package app

import (
	"strings"
	"testing"
)

const highProgram = `high(T, S) :- entry(E, "health.blood-pressure", T, _, _, _, _), attr(E, "/systolic_blood_pressure/value", S), S > 110.`

// answerOf is the answer section of a query run's output: everything after the
// header, which carries the receipt hash and so differs between two runs.
func answerOf(out string) string {
	_, after, _ := strings.Cut(out, "\n\n")
	return after
}

// receiptHashOf reads the receipt line of a query run's output.
func receiptHashOf(t *testing.T, out string) string {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		if rest, ok := strings.CutPrefix(line, "receipt    "); ok {
			return strings.Fields(rest)[0]
		}
	}
	t.Fatalf("no receipt line in:\n%s", out)
	return ""
}

// A query runs the same over the local files and over the daemon: the same answer,
// a receipt each side verifies, and the same refusals.
func TestQueryRun_SameOverTheDaemonAndVerifies(t *testing.T) {
	t.Parallel()
	f := newRemoteFixture(t)

	local, _, err := cli(t, f.cfgPath, "query", "run", "--projection", "bp", "-e", highProgram)
	if err != nil {
		t.Fatalf("local: %v", err)
	}
	remote, _, err := cli(t, f.remoteCfg, "query", "run", "--projection", "bp", "-e", highProgram)
	if err != nil {
		t.Fatalf("remote: %v", err)
	}
	if answerOf(local) != answerOf(remote) {
		t.Errorf("answers differ\n--- local\n%s--- remote\n%s", local, remote)
	}
	want := []string{"high  1 tuples  sha256:", "2026-01-23T19:36:00.000000000Z  114", "projection bp v1", "rules      anam v1", "(filed)"}
	for _, w := range want {
		if !strings.Contains(local, w) {
			t.Errorf("output omits %q:\n%s", w, local)
		}
	}
	if !strings.Contains(local, "warning    health.blood-pressure declares /systolic_blood_pressure/value at") {
		t.Errorf("no per-version warning for a pointer older versions lack:\n%s", local)
	}

	for _, side := range []struct{ name, cfg, out string }{{"local", f.cfgPath, local}, {"remote", f.remoteCfg, remote}} {
		hash := receiptHashOf(t, side.out)
		verified := f.sameOutput(t, "receipt", "verify", hash)
		if !strings.Contains(verified, "OK   reproduced") || !strings.Contains(verified, "answered high with anam v1") {
			t.Errorf("%s receipt %s did not verify:\n%s", side.name, hash, verified)
		}
		shown := f.sameOutput(t, "receipt", "show", hash)
		if !strings.Contains(shown, "program    high answered with anam v1") || !strings.Contains(shown, "2026-01-23T19:36:00.000000000Z  114") {
			t.Errorf("%s receipt %s does not show its program and answer:\n%s", side.name, hash, shown)
		}
	}

	// A dry run derives the same receipt hash as the filed one and files nothing.
	dry, _, err := cli(t, f.remoteCfg, "query", "run", "--projection", "bp", "-e", highProgram, "--dry-run")
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if !strings.Contains(dry, "(dry run, not filed)") {
		t.Errorf("dry run output:\n%s", dry)
	}
}

// What cannot run is refused the same on both sides, as the reader's error, with a
// message that says what to change.
func TestQueryRun_RefusalsAreTheSame(t *testing.T) {
	t.Parallel()
	f := newRemoteFixture(t)
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"no projection", []string{"-e", highProgram}, "a query runs through a projection; name one"},
		{"unknown projection", []string{"--projection", "nope", "-e", highProgram}, "unknown projection"},
		{"syntax", []string{"--projection", "bp", "-e", `high(E) :- entry(E, _, _, _, _, _, _)`}, `line 1: expected "."`},
		{"unsafe", []string{"--projection", "bp", "-e", `high(E, X) :- entry(E, _, _, _, _, _, _).`}, "X in the head of high is not bound"},
		{"undeclared pointer", []string{"--projection", "bp", "-e", `a(E) :- attr(E, "/nope", _).`}, "no Definition declares a field at /nope"},
		{"instant lexeme", []string{"--projection", "bp", "-e", `instant("2026-01-24"). a(E) :- current(E, _).`}, "write the whole instant, as 2026-01-24T00:00:00.000000000Z"},
		{"no rules", []string{"--projection", "bp", "-e", `% nothing`}, "name --relation"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			for _, cfg := range []string{f.cfgPath, f.remoteCfg} {
				_, _, err := cli(t, cfg, append([]string{"query", "run"}, c.args...)...)
				if err == nil || !strings.Contains(err.Error(), c.want) {
					t.Errorf("%s: %v, want %q", cfg, err, c.want)
				}
			}
		})
	}
}

// The catalogue is the same from the files and from the daemon, and says what the
// record's types declare.
func TestQueryDescribe_SameOverTheDaemon(t *testing.T) {
	t.Parallel()
	f := newRemoteFixture(t)
	out := f.sameOutput(t, "query", "describe")
	for _, want := range []string{"fact-time lexeme  2006-01-02T15:04:05.000000000Z07:00", "base       entry(E, Type, FactTime, Series, Status, Source, DefinitionVersion)",
		"parameter  instant(At)", "rule       current(E, At)", "health.blood-pressure  v6", "/systolic_blood_pressure/value", "health.medication-started  v1  series med.regimen", "health.medication-stopped  v1  series med.regimen  closes"} {
		if !strings.Contains(out, want) {
			t.Errorf("describe omits %q:\n%s", want, out)
		}
	}
	asJSON := f.sameOutput(t, "query", "describe", "--json")
	if !strings.Contains(asJSON, `"relations"`) {
		t.Errorf("describe --json:\n%s", asJSON)
	}
}
