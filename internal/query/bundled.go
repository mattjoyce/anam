package query

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
)

// The bundled rule set ships with the binary (AGENTS.md, embedded assets): small,
// static, versioned. Its hash goes into an answer's receipt so an agent's query
// names the reading of time it stood on.
//
//go:embed rules/anam.dl
var bundledText string

// BundledName and BundledVersion identify the bundled rule set. The version is an
// invariant, not a default: it is part of what an answer means, and moves when the
// rules' meaning does, with the hash catching any change the version did not.
const (
	BundledName    = "anam"
	BundledVersion = 1
)

// RuleSet is a named, versioned program with a hash over its text.
type RuleSet struct {
	Name    string
	Version int
	Hash    string
	Program Program
}

// Bundled parses the bundled rule set. It fails rather than returning a partial set:
// the rules are one reading, and a binary that lost half of it would answer wrongly
// with confidence.
func Bundled() (RuleSet, error) {
	p, err := Parse(bundledText)
	if err != nil {
		return RuleSet{}, fmt.Errorf("query: bundled rule set: %w", err)
	}
	sum := sha256.Sum256([]byte(bundledText))
	return RuleSet{Name: BundledName, Version: BundledVersion, Hash: "sha256:" + hex.EncodeToString(sum[:]), Program: p}, nil
}
