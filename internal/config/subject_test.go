package config_test

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/config"
)

// TestSubjectIDReadsItsOldName is card 270a: person_id and ANAM_PERSON_ID still set the
// subject, so a deployment that has not moved keeps its pin, with a WARN that names the
// new key and variable; both names given and equal is an INFO, so a deployment can
// carry both while it moves without its startup check refusing a WARN; the two given
// differently, in one layer or across layers, are refused.
//
// Not parallel: it swaps the default slog logger, which is process-wide, to read the
// level of the line Load writes. The level matters because the deploy post-flight
// refuses a WARN.
func TestSubjectIDReadsItsOldName(t *testing.T) {
	type want struct {
		subject string
		warn    int
		info    int
		errHas  []string // a refusal, naming each of these
	}
	tests := []struct {
		name string
		body string
		env  map[string]string
		set  map[string]string
		want want
	}{
		{name: "new key in the file", body: "subject_id: matt\n",
			want: want{subject: "matt"}},
		{name: "old key in the file", body: "person_id: matt\n",
			want: want{subject: "matt", warn: 1}},
		{name: "old variable, as unraid sets it", env: map[string]string{"ANAM_PERSON_ID": "matt"},
			want: want{subject: "matt", warn: 1}},
		{name: "new variable", env: map[string]string{"ANAM_SUBJECT_ID": "matt"},
			want: want{subject: "matt"}},
		{name: "both in the file, equal", body: "subject_id: matt\nperson_id: matt\n",
			want: want{subject: "matt", info: 1}},
		{name: "both in the file, different", body: "subject_id: matt\nperson_id: other\n",
			want: want{errHas: []string{"subject_id", "person_id", `"matt"`, `"other"`}}},
		{name: "old in the file, new in the environment, equal", body: "person_id: matt\n",
			env:  map[string]string{"ANAM_SUBJECT_ID": "matt"},
			want: want{subject: "matt", info: 1}},
		{name: "old in the file, new in the environment, different", body: "person_id: matt\n",
			env:  map[string]string{"ANAM_SUBJECT_ID": "thinkpad-t14s"},
			want: want{errHas: []string{"subject_id", "person_id", `"thinkpad-t14s"`, `"matt"`}}},
		{name: "new in the file, old in the environment, different", body: "subject_id: matt\n",
			env:  map[string]string{"ANAM_PERSON_ID": "other"},
			want: want{errHas: []string{"subject_id", "person_id", `"matt"`, `"other"`}}},
		{name: "old in the file, new explicit, different", body: "person_id: matt\n",
			set:  map[string]string{"subject_id": "other"},
			want: want{errHas: []string{"subject_id", "person_id", `"other"`, `"matt"`}}},
		{name: "neither, no server",
			want: want{errHas: []string{"subject_id is required (ANAM_SUBJECT_ID)"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ANAM_SUBJECT_ID", "")
			t.Setenv("ANAM_PERSON_ID", "")
			t.Setenv("ANAM_SERVER", "")
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			logs := captureLog(t)
			var path string
			if tt.body != "" {
				path = writeConfig(t, tt.body)
			}

			got, err := config.LoadOverriding(path, tt.set)
			if len(tt.want.errHas) > 0 {
				if err == nil {
					t.Fatalf("Load() = subject %q, want an error", got.SubjectID)
				}
				if !errors.Is(err, config.ErrInvalid) {
					t.Errorf("Load() error = %v, want it to wrap ErrInvalid", err)
				}
				for _, s := range tt.want.errHas {
					if !strings.Contains(err.Error(), s) {
						t.Errorf("error = %v, want it to name %s", err, s)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if got.SubjectID != tt.want.subject {
				t.Errorf("SubjectID = %q, want %q", got.SubjectID, tt.want.subject)
			}
			if got.PersonID != "" {
				t.Errorf("PersonID = %q after Load, want it moved across and zero", got.PersonID)
			}
			out := logs.String()
			if n := strings.Count(out, "level=WARN"); n != tt.want.warn {
				t.Errorf("%d WARN lines, want %d:\n%s", n, tt.want.warn, out)
			}
			if n := strings.Count(out, "level=INFO"); n != tt.want.info {
				t.Errorf("%d INFO lines, want %d:\n%s", n, tt.want.info, out)
			}
			if tt.want.warn > 0 && (!strings.Contains(out, "subject_id") || !strings.Contains(out, "ANAM_SUBJECT_ID")) {
				t.Errorf("the WARN does not name subject_id and ANAM_SUBJECT_ID:\n%s", out)
			}
			if tt.want.warn+tt.want.info > 0 && !strings.Contains(out, "subject="+tt.want.subject) {
				t.Errorf("the line does not carry subject=%s:\n%s", tt.want.subject, out)
			}
		})
	}
}

// captureLog points the default slog logger at a buffer for the rest of the test, at
// every level, and puts the old one back after.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(old) })
	return &buf
}
