package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestLoadOverridingPrecedence pins LoadOverriding's four layers in one place: the
// explicit override (a CLI flag) beats the ANAM_ variable, which beats the file,
// which beats the default. It is checked over two keys, one a duration and one not,
// so the precedence holds for both decodings. Load only ever passes nil for the top
// layer, so nothing until now asserted that the explicit override actually won.
func TestLoadOverridingPrecedence(t *testing.T) {
	tests := []struct {
		name    string
		body    string // YAML written to the config file; "" means no file
		env     map[string]string
		set     map[string]string
		want    time.Duration // LongWriteTimeout
		wantN   int           // LibraryFolderLimit
		wantErr bool
	}{
		{
			name:  "default only",
			want:  DefaultLongWriteTimeout,
			wantN: DefaultLibraryFolderLimit,
		},
		{
			name:  "file only",
			body:  "long_write_timeout: 20m\nlibrary_folder_limit: 250\n",
			want:  20 * time.Minute,
			wantN: 250,
		},
		{
			name:  "file + env, env wins",
			body:  "long_write_timeout: 20m\nlibrary_folder_limit: 250\n",
			env:   map[string]string{"ANAM_LONG_WRITE_TIMEOUT": "25m", "ANAM_LIBRARY_FOLDER_LIMIT": "125"},
			want:  25 * time.Minute,
			wantN: 125,
		},
		{
			name:  "file + env + explicit, explicit wins",
			body:  "long_write_timeout: 20m\nlibrary_folder_limit: 250\n",
			env:   map[string]string{"ANAM_LONG_WRITE_TIMEOUT": "25m", "ANAM_LIBRARY_FOLDER_LIMIT": "125"},
			set:   map[string]string{"long_write_timeout": "30m", "library_folder_limit": "60"},
			want:  30 * time.Minute,
			wantN: 60,
		},
		{
			// ingest_timeout in the file and a different long_write_timeout given as an
			// explicit override: adoptLongWriteTimeout sees the same key under both
			// names with different values and refuses, rather than letting one win.
			name:    "ingest_timeout in the file, a different long_write_timeout explicit: refused as a contradiction",
			body:    "ingest_timeout: 10m\n",
			set:     map[string]string{"long_write_timeout": "30m"},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ANAM_PERSON_ID", "matt")
			t.Setenv("ANAM_LONG_WRITE_TIMEOUT", "")
			t.Setenv("ANAM_INGEST_TIMEOUT", "")
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			var path string
			if tt.body != "" {
				path = filepath.Join(t.TempDir(), "anam.yaml")
				if err := os.WriteFile(path, []byte("person_id: matt\n"+tt.body), 0o600); err != nil {
					t.Fatalf("write config: %v", err)
				}
			}

			got, err := LoadOverriding(path, tt.set)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("LoadOverriding() = %+v, want an error", got)
				}
				if !errors.Is(err, ErrInvalid) {
					t.Errorf("LoadOverriding() error = %v, want it to wrap ErrInvalid", err)
				}
				for _, key := range []string{"long_write_timeout", "ingest_timeout"} {
					if !strings.Contains(err.Error(), key) {
						t.Errorf("error does not name %s: %v", key, err)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadOverriding() error = %v", err)
			}
			if got.LongWriteTimeout != tt.want {
				t.Errorf("LongWriteTimeout = %s, want %s", got.LongWriteTimeout, tt.want)
			}
			if got.LibraryFolderLimit != tt.wantN {
				t.Errorf("LibraryFolderLimit = %d, want %d", got.LibraryFolderLimit, tt.wantN)
			}
		})
	}
}
