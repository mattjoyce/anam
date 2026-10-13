package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"

	"github.com/mattjoyce/anam/internal/safepath"
)

// Imports are declared, not improvised: `anam ingest` reads a YAML file naming
// each source and the file it captures from, and ingests only what is declared.
// A path typed ad hoc on a command line is provenance that evaporates with the
// shell history; a declared import is reviewable, repeatable, and the only kind
// the CLI accepts.

// EnvImports names the environment variable that overrides imports discovery.
const EnvImports = "ANAM_IMPORTS"

// OriginImportsEnv marks an imports file named by $ANAM_IMPORTS. It is not OriginEnv,
// which names $ANAM_CONFIG: a label that names the wrong variable sends its reader to
// the wrong place.
const OriginImportsEnv Origin = "env:" + EnvImports

// ErrInvalidImports is returned for an imports file that cannot be used.
var ErrInvalidImports = errors.New("invalid imports file")

// Import declares one capture: which source, from which file.
type Import struct {
	// Source is the import's identity: records, documents.
	Source string `mapstructure:"source"`

	// Path is the source file, expanded on load (~ and $VARs) and resolved to a
	// real regular file at ingest time.
	Path string `mapstructure:"path"`
}

type importsFile struct {
	Imports []Import `mapstructure:"imports"`
}

// ResolveImports finds the imports file: the explicit path, then $ANAM_IMPORTS,
// then <user-config-dir>/anam/imports.yaml. Same asymmetry as Resolve: an
// explicitly named file must exist; a discovered one is optional — but unlike
// config, ingest cannot run on defaults, so "" means there is nothing to ingest.
func ResolveImports(explicit string) (string, Origin, error) {
	if explicit != "" {
		expanded, err := safepath.Expand(explicit)
		if err != nil {
			return "", "", err
		}
		path, err := mustExist(expanded)
		if err != nil {
			return "", "", err
		}
		return path, OriginFlag, nil
	}

	if fromEnv := os.Getenv(EnvImports); fromEnv != "" {
		expanded, err := safepath.Expand(fromEnv)
		if err != nil {
			return "", "", err
		}
		path, err := mustExist(expanded)
		if err != nil {
			return "", "", fmt.Errorf("%s: %w", EnvImports, err)
		}
		return path, OriginImportsEnv, nil
	}

	dir, err := os.UserConfigDir()
	if err != nil {
		return "", OriginDefaults, nil
	}
	candidate := filepath.Join(dir, "anam", "imports.yaml")
	switch _, err := os.Stat(candidate); {
	case err == nil:
		return candidate, OriginUserDir, nil
	case errors.Is(err, os.ErrNotExist):
		return "", OriginDefaults, nil
	default:
		return "", "", fmt.Errorf("stat %q: %w", candidate, err)
	}
}

// LoadImports reads and validates the declarations. Unknown keys are errors, as
// in Load: a silently-ignored `sorce:` would leave an import that never runs.
// Paths come back expanded but not resolved — existence is checked at ingest
// time, against the file the capture will actually open.
func LoadImports(path string) ([]Import, error) {
	v := viper.New()
	v.SetConfigFile(path)
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read imports %q: %w", path, err)
	}

	var f importsFile
	errorOnUnknownKeys := func(dc *mapstructure.DecoderConfig) { dc.ErrorUnused = true }
	if err := v.Unmarshal(&f, errorOnUnknownKeys); err != nil {
		return nil, fmt.Errorf("decode imports %q: %w: %v", path, ErrInvalidImports, err)
	}
	if len(f.Imports) == 0 {
		return nil, fmt.Errorf("%w: %q declares no imports", ErrInvalidImports, path)
	}

	for i := range f.Imports {
		imp := &f.Imports[i]
		if imp.Source == "" {
			return nil, fmt.Errorf("%w: import %d has no source", ErrInvalidImports, i+1)
		}
		if imp.Path == "" {
			return nil, fmt.Errorf("%w: import %d (%s) has no path", ErrInvalidImports, i+1, imp.Source)
		}
		expanded, err := safepath.Expand(imp.Path)
		if err != nil {
			return nil, fmt.Errorf("%w: import %d (%s): %v", ErrInvalidImports, i+1, imp.Source, err)
		}
		imp.Path = expanded
	}
	return f.Imports, nil
}
