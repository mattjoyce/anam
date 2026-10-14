// Package token reads the bearer token the daemon checks and its clients present. Both
// sides read it the same way, from a file named in config, so a token that one side
// would refuse is refused by the other too.
package token

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/mattjoyce/anam/internal/safepath"
)

// ErrWeak is returned for a token too short to stand in for a password.
var ErrWeak = errors.New("token is shorter than 32 characters")

// minLen is 32 characters: `openssl rand -hex 16` and up. It is a floor, not a
// default (card 151): below it a token can be guessed, and a longer token already
// passes.
const minLen = 32

// Read returns the token in the file at path: its contents, trimmed. The token is
// never logged, and no error quotes it.
func Read(path string) ([]byte, error) {
	f, err := safepath.Open(path)
	if err != nil {
		return nil, fmt.Errorf("token file: %w", err)
	}
	defer func() { _ = f.Close() }()
	// A token file is one line; 4 KiB bounds reading a wrong file, not a choice.
	b, err := io.ReadAll(io.LimitReader(f, 4096))
	if err != nil {
		return nil, fmt.Errorf("read token file: %w", err)
	}
	tok := []byte(strings.TrimSpace(string(b)))
	if len(tok) < minLen {
		return nil, fmt.Errorf("%w (token_file %s)", ErrWeak, path)
	}
	return tok, nil
}
