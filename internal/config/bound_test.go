package config_test

import (
	"errors"
	"testing"

	"github.com/mattjoyce/anam/internal/config"
)

// max_read_bytes defaults to 512 KiB, the environment sets it, and a negative one
// dies at load rather than on the read path.
func TestMaxReadBytes(t *testing.T) {
	t.Setenv("ANAM_PERSON_ID", "matt")

	got, err := config.Load("")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.MaxReadBytes != 512<<10 {
		t.Errorf("MaxReadBytes = %d, want the default 512 KiB", got.MaxReadBytes)
	}

	t.Setenv("ANAM_MAX_READ_BYTES", "0")
	if got, err = config.Load(""); err != nil || got.MaxReadBytes != 0 {
		t.Errorf("MaxReadBytes = %d, %v; want 0 (no bound) from the environment", got.MaxReadBytes, err)
	}

	t.Setenv("ANAM_MAX_READ_BYTES", "-1")
	if _, err = config.Load(""); !errors.Is(err, config.ErrInvalid) {
		t.Errorf("Load() with a negative max_read_bytes = %v, want ErrInvalid", err)
	}
}

// default_projection is empty by default, the environment names one, and Load takes
// any name: only the daemon can see the registered projections, so it is the daemon
// that refuses a name that is nothing (card 180). Load refuses only stray space.
func TestDefaultProjection(t *testing.T) {
	t.Setenv("ANAM_PERSON_ID", "matt")

	got, err := config.Load("")
	if err != nil || got.DefaultProjection != "" {
		t.Fatalf("Load() = %q, %v; want no default projection", got.DefaultProjection, err)
	}

	t.Setenv("ANAM_DEFAULT_PROJECTION", "recent-life")
	if got, err = config.Load(""); err != nil || got.DefaultProjection != "recent-life" {
		t.Errorf("DefaultProjection = %q, %v; want recent-life from the environment", got.DefaultProjection, err)
	}

	t.Setenv("ANAM_DEFAULT_PROJECTION", "no-such-thing")
	if got, err = config.Load(""); err != nil || got.DefaultProjection != "no-such-thing" {
		t.Errorf("DefaultProjection = %q, %v; want Load to take any name", got.DefaultProjection, err)
	}

	t.Setenv("ANAM_DEFAULT_PROJECTION", " recent-life")
	if _, err = config.Load(""); !errors.Is(err, config.ErrInvalid) {
		t.Errorf("Load() with a padded default_projection = %v, want ErrInvalid", err)
	}
}
