package token_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/token"
)

func TestRead(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("a", 32)
	for _, tt := range []struct {
		name, contents string
		want           string
		wantErr        error
	}{
		{"trimmed", "  " + long + "\n", long, nil},
		{"too short", "short\n", "", token.ErrWeak},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := filepath.Join(t.TempDir(), "token")
			if err := os.WriteFile(p, []byte(tt.contents), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := token.Read(p)
			if !errors.Is(err, tt.wantErr) || string(got) != tt.want {
				t.Fatalf("Read = %q, %v; want %q, %v", got, err, tt.want, tt.wantErr)
			}
			if err != nil && strings.Contains(err.Error(), tt.contents) {
				t.Errorf("the error quotes the token: %v", err)
			}
		})
	}
	if _, err := token.Read(filepath.Join(t.TempDir(), "absent")); err == nil {
		t.Error("a missing token file was accepted")
	}
}
