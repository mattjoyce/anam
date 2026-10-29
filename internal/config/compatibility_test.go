package config_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/mattjoyce/ashlar/yamlconfig"

	"github.com/mattjoyce/anam/internal/config"
)

// sideBySide is the test-only shape both loaders decode into. It mirrors only the
// Config fields the rows touch.
type sideBySide struct {
	PersonID           string        `yaml:"person_id"`
	LongWriteTimeout   time.Duration `yaml:"long_write_timeout"`
	LibraryFolderLimit int           `yaml:"library_folder_limit"`
}

// sideBySideWant names the fields a row is about. A nil pointer is a field the row
// does not assert, so a loader's defaults elsewhere cannot manufacture a gap.
type sideBySideWant struct {
	personID           *string
	longWriteTimeout   *time.Duration
	libraryFolderLimit *int
}

func strp(s string) *string               { return &s }
func durp(d time.Duration) *time.Duration { return &d }
func intp(n int) *int                     { return &n }

func (w sideBySideWant) check(t *testing.T, loader string, got sideBySide) {
	t.Helper()
	if w.personID != nil && got.PersonID != *w.personID {
		t.Errorf("%s person_id = %q, want %q", loader, got.PersonID, *w.personID)
	}
	if w.longWriteTimeout != nil && got.LongWriteTimeout != *w.longWriteTimeout {
		t.Errorf("%s long_write_timeout = %s, want %s", loader, got.LongWriteTimeout, *w.longWriteTimeout)
	}
	if w.libraryFolderLimit != nil && got.LibraryFolderLimit != *w.libraryFolderLimit {
		t.Errorf("%s library_folder_limit = %d, want %d", loader, got.LibraryFolderLimit, *w.libraryFolderLimit)
	}
}

// equal compares the fields w names, so a gap is judged only on what the row is about.
func (w sideBySideWant) equal(a, b sideBySide) bool {
	if w.personID != nil && a.PersonID != b.PersonID {
		return false
	}
	if w.longWriteTimeout != nil && a.LongWriteTimeout != b.LongWriteTimeout {
		return false
	}
	if w.libraryFolderLimit != nil && a.LibraryFolderLimit != b.LibraryFolderLimit {
		return false
	}
	return true
}

// TestViperAshlarSideBySide records what the Viper loader accepts today and what Ashlar does with the same file. Rows marked gap are where a straight swap would change accepted input; the swap must close each one or have it approved.
func TestViperAshlarSideBySide(t *testing.T) {
	// Config has no list-valued field (config.go and limits.go), so there is no
	// scalar-where-a-list-is-expected row.
	tests := []struct {
		name       string
		yaml       string
		envPerson  bool
		viperOK    bool
		viperWant  sideBySideWant
		viperErr   string
		ashlarOK   bool
		ashlarWant sideBySideWant
		gap        bool
	}{
		{
			name:      "empty file",
			yaml:      "",
			envPerson: true,
			viperOK:   true,
			viperWant: sideBySideWant{
				personID:           strp("matt"),
				longWriteTimeout:   durp(config.DefaultLongWriteTimeout),
				libraryFolderLimit: intp(config.DefaultLibraryFolderLimit),
			},
			ashlarOK: false,
			gap:      true,
		},
		{
			name:      "comment-only file",
			yaml:      "# only a comment\n",
			envPerson: true,
			viperOK:   true,
			viperWant: sideBySideWant{
				personID:           strp("matt"),
				longWriteTimeout:   durp(config.DefaultLongWriteTimeout),
				libraryFolderLimit: intp(config.DefaultLibraryFolderLimit),
			},
			ashlarOK: false,
			gap:      true,
		},
		{
			name:      "quoted integer",
			yaml:      "library_folder_limit: \"250\"\n",
			viperOK:   true,
			viperWant: sideBySideWant{libraryFolderLimit: intp(250)},
			ashlarOK:  false,
			gap:       true,
		},
		{
			name:      "bare number for a duration",
			yaml:      "long_write_timeout: 60\n",
			viperOK:   true,
			viperWant: sideBySideWant{longWriteTimeout: durp(60 * time.Nanosecond)},
			ashlarOK:  false,
			gap:       true,
		},
		{
			name:       "quoted duration string",
			yaml:       "long_write_timeout: \"20m\"\n",
			viperOK:    true,
			viperWant:  sideBySideWant{longWriteTimeout: durp(20 * time.Minute)},
			ashlarOK:   true,
			ashlarWant: sideBySideWant{longWriteTimeout: durp(20 * time.Minute)},
			gap:        false,
		},
		{
			name:       "null integer",
			yaml:       "library_folder_limit: ~\n",
			viperOK:    true,
			viperWant:  sideBySideWant{libraryFolderLimit: intp(config.DefaultLibraryFolderLimit)},
			ashlarOK:   true,
			ashlarWant: sideBySideWant{libraryFolderLimit: intp(0)},
			gap:        true,
		},
		{
			name:     "anchor and alias",
			yaml:     "x: &n 250\nlibrary_folder_limit: *n\n",
			viperOK:  false,
			viperErr: "invalid keys: x",
			ashlarOK: false,
			gap:      false,
		},
		{
			name:      "merge key into the root",
			yaml:      "<<: {library_folder_limit: 250}\n",
			viperOK:   true,
			viperWant: sideBySideWant{libraryFolderLimit: intp(250)},
			ashlarOK:  false,
			gap:       true,
		},
		{
			name:      "two documents separated by ---",
			yaml:      "library_folder_limit: 250\n---\nlibrary_folder_limit: 300\n",
			viperOK:   true,
			viperWant: sideBySideWant{libraryFolderLimit: intp(250)},
			ashlarOK:  false,
			gap:       true,
		},
		{
			name:      "upper-case key",
			yaml:      "LIBRARY_FOLDER_LIMIT: 250\n",
			viperOK:   true,
			viperWant: sideBySideWant{libraryFolderLimit: intp(250)},
			ashlarOK:  false,
			gap:       true,
		},
		{
			name:      "float for an integer",
			yaml:      "library_folder_limit: 250.0\n",
			viperOK:   true,
			viperWant: sideBySideWant{libraryFolderLimit: intp(250)},
			ashlarOK:  false,
			gap:       true,
		},
		{
			name:      "boolean for an integer",
			yaml:      "library_folder_limit: true\n",
			viperOK:   true,
			viperWant: sideBySideWant{libraryFolderLimit: intp(1)},
			ashlarOK:  false,
			gap:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ANAM_PERSON_ID", "")
			body := tt.yaml
			if tt.envPerson {
				t.Setenv("ANAM_PERSON_ID", "matt")
			} else {
				body = "person_id: matt\n" + body
			}
			path := filepath.Join(t.TempDir(), "anam.yaml")
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatalf("write config: %v", err)
			}

			c, viperErr := config.LoadOverriding(path, nil)
			viperOK := viperErr == nil
			viperGot := sideBySide{}
			if viperOK {
				viperGot = sideBySide{
					PersonID:           c.PersonID,
					LongWriteTimeout:   c.LongWriteTimeout,
					LibraryFolderLimit: c.LibraryFolderLimit,
				}
				tt.viperWant.check(t, "Viper", viperGot)
			} else if tt.viperErr == "" {
				t.Errorf("Viper error = %v, want an error", viperErr)
			} else if !strings.Contains(viperErr.Error(), tt.viperErr) {
				t.Errorf("Viper error = %v, want it to contain %q", viperErr, tt.viperErr)
			}

			ashlarGot := sideBySide{}
			v, ashlarErr := yamlconfig.Read(path, []byte(body))
			if ashlarErr == nil {
				ashlarErr = yamlconfig.DecodeValueStrict(v, &ashlarGot)
			}
			ashlarOK := ashlarErr == nil
			if ashlarOK {
				tt.ashlarWant.check(t, "Ashlar", ashlarGot)
			}

			if viperOK != tt.viperOK {
				t.Errorf("Viper OK = %v (%v), want %v", viperOK, viperErr, tt.viperOK)
			}
			if ashlarOK != tt.ashlarOK {
				t.Errorf("Ashlar OK = %v (%v), want %v", ashlarOK, ashlarErr, tt.ashlarOK)
			}

			gap := viperOK != ashlarOK
			if viperOK && ashlarOK {
				gap = !tt.viperWant.equal(viperGot, ashlarGot) || !tt.ashlarWant.equal(viperGot, ashlarGot)
			}
			if gap != tt.gap {
				t.Errorf("gap = %v, want %v", gap, tt.gap)
			}
		})
	}
}

// principalSideBySide mirrors the importers file's principal shape for Ashlar.
type principalSideBySide struct {
	Name        string   `yaml:"name"`
	Role        string   `yaml:"role"`
	Source      string   `yaml:"source"`
	TokenSHA256 string   `yaml:"token_sha256"`
	Types       []string `yaml:"types"`
	AnyType     bool     `yaml:"any_type"`
	Receipts    string   `yaml:"receipts"`
}

type principalsSideBySide struct {
	Principals []principalSideBySide `yaml:"principals"`
	Importers  []principalSideBySide `yaml:"importers"`
}

func flattenPrincipals(f principalsSideBySide) []principalSideBySide {
	out := make([]principalSideBySide, 0, len(f.Principals)+len(f.Importers))
	out = append(out, f.Principals...)
	return append(out, f.Importers...)
}

func flattenConfigPrincipals(ps []config.Principal) []principalSideBySide {
	out := make([]principalSideBySide, 0, len(ps))
	for _, p := range ps {
		var types []string
		if p.Types != nil {
			types = make([]string, len(p.Types))
			for i, ty := range p.Types {
				types[i] = string(ty)
			}
		}
		out = append(out, principalSideBySide{
			Name:        p.Name,
			Role:        string(p.Role),
			Source:      p.Source,
			TokenSHA256: p.TokenSHA256,
			Types:       types,
			AnyType:     p.AnyType,
			Receipts:    string(p.Receipts),
		})
	}
	return out
}

// TestPrincipalsSideBySide records the same input shapes against the importers
// loader, whose file has its own shape.
func TestPrincipalsSideBySide(t *testing.T) {
	const (
		compatHashA = "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"
		compatHashB = "60303ae22b998861bce3b28f33eec1be758a213c86c93c076dbe9f558c11c752"
	)
	minimal := `importers:
  - source: withings
    token_sha256: ` + compatHashA + `
    types: [health.weight]
`
	withings := principalSideBySide{
		Name:        "withings",
		Role:        "importer",
		Source:      "withings",
		TokenSHA256: compatHashA,
		Types:       []string{"health.weight"},
	}
	garmin := principalSideBySide{
		Name:        "garmin",
		Role:        "importer",
		Source:      "garmin",
		TokenSHA256: compatHashB,
		Types:       []string{"health.sleep"},
	}

	tests := []struct {
		name       string
		yaml       string
		viperOK    bool
		viperWant  []principalSideBySide
		viperErr   string
		ashlarOK   bool
		ashlarWant []principalSideBySide
		gap        bool
	}{
		{
			name:      "empty file",
			yaml:      "",
			viperOK:   true,
			viperWant: []principalSideBySide{},
			ashlarOK:  false,
			gap:       true,
		},
		{
			name:      "comment-only file",
			yaml:      "# only a comment\n",
			viperOK:   true,
			viperWant: []principalSideBySide{},
			ashlarOK:  false,
			gap:       true,
		},
		{
			name: "anchor and alias",
			yaml: `importers:
  - source: withings
    token_sha256: ` + compatHashA + `
    types: [health.weight]
    role: &r importer
  - source: garmin
    token_sha256: ` + compatHashB + `
    types: [health.sleep]
    role: *r
`,
			viperOK:   true,
			viperWant: []principalSideBySide{withings, garmin},
			ashlarOK:  false,
			gap:       true,
		},
		{
			name:      "two documents separated by ---",
			yaml:      minimal + "---\n" + minimal,
			viperOK:   true,
			viperWant: []principalSideBySide{withings},
			ashlarOK:  false,
			gap:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "importers.yaml")
			if err := os.WriteFile(path, []byte(tt.yaml), 0o600); err != nil {
				t.Fatalf("write importers: %v", err)
			}

			loaded, viperErr := config.LoadPrincipals(path, nil)
			viperOK := viperErr == nil
			viperGot := []principalSideBySide{}
			if viperOK {
				viperGot = flattenConfigPrincipals(loaded)
				if !reflect.DeepEqual(viperGot, tt.viperWant) {
					t.Errorf("Viper principals = %+v, want %+v", viperGot, tt.viperWant)
				}
			} else if tt.viperErr == "" {
				t.Errorf("Viper error = %v, want an error", viperErr)
			} else if !strings.Contains(viperErr.Error(), tt.viperErr) {
				t.Errorf("Viper error = %v, want it to contain %q", viperErr, tt.viperErr)
			}

			var decoded principalsSideBySide
			v, ashlarErr := yamlconfig.Read(path, []byte(tt.yaml))
			if ashlarErr == nil {
				ashlarErr = yamlconfig.DecodeValueStrict(v, &decoded)
			}
			ashlarOK := ashlarErr == nil
			ashlarGot := flattenPrincipals(decoded)
			if ashlarOK && !reflect.DeepEqual(ashlarGot, tt.ashlarWant) {
				t.Errorf("Ashlar principals = %+v, want %+v", ashlarGot, tt.ashlarWant)
			}

			if viperOK != tt.viperOK {
				t.Errorf("Viper OK = %v (%v), want %v", viperOK, viperErr, tt.viperOK)
			}
			if ashlarOK != tt.ashlarOK {
				t.Errorf("Ashlar OK = %v (%v), want %v", ashlarOK, ashlarErr, tt.ashlarOK)
			}

			gap := viperOK != ashlarOK
			if viperOK && ashlarOK {
				gap = !reflect.DeepEqual(viperGot, ashlarGot)
			}
			if gap != tt.gap {
				t.Errorf("gap = %v, want %v", gap, tt.gap)
			}
		})
	}
}
