package config

import (
	"os"
	"strings"
	"testing"
)

// TestMain clears every ANAM_ variable before the tests run. Config reads the
// environment, so without this a developer's shell (ANAM_SERVER, since the record
// moved to the daemon) would point the tests at a live service.
func TestMain(m *testing.M) {
	for _, kv := range os.Environ() {
		if name, _, _ := strings.Cut(kv, "="); strings.HasPrefix(name, "ANAM_") {
			_ = os.Unsetenv(name) // only fails for an invalid name, which this is not
		}
	}
	os.Exit(m.Run())
}
