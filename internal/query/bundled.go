package query

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
)

// The bundled rule set ships with the binary (AGENTS.md, embedded assets): small,
// static, versioned. Its hash goes into an answer's receipt so an agent's query
// names the reading of time it stood on.
//
//go:embed rules/anam.dl
var bundledText string

// historyFS holds every earlier text a receipt may name, kept as it was written.
// A receipt pins a rule set by the hash of its text, so a text a receipt can name
// is never edited or removed: a change to rules/anam.dl moves BundledVersion, and
// the text it replaces goes here first, byte for byte.
//
//go:embed rules/history/*.dl
var historyFS embed.FS

// RelInstant is the bundled set's parameter relation, instant(At): the fact-times a
// query asks about. It is declared by the bundled set and defined by the query, and
// a constant in it must be a whole fact-time lexeme, since a shorter one sorts before
// every entry of its day and asks a question nobody meant.
const RelInstant = "instant"

// BundledName and BundledVersion identify the bundled rule set. The version is an
// invariant, not a default: it is part of what an answer means, and moves when the
// rules' meaning does. The hash catches a change the version did not, and a receipt
// is verified by its hash, never by the version alone.
const (
	BundledName    = "anam"
	BundledVersion = 2
)

// historic is one earlier text. Version is the one the rule set was called while it
// was served. Note says which text this is: the hash covers the file's every byte, so
// the note cannot be a comment inside it.
type historic struct {
	File    string
	Version int
	Note    string
}

// history lists the earlier texts. Version 1 was served as three different texts: the
// edge rules (cards 104 and 91, then the record.edge rename of card 191) landed on
// 2026-10-21 without a bump, so the first of the three, and then the other two, all
// called themselves v1. The hashes tell them apart; the version could not.
var history = []historic{
	{"anam.v1.dl", 1, "as first served, 2026-10-19 (card 173): before, later, displaced, latest, closed and current"},
	{"anam.v1-2026-10-21-record-note.dl", 1, "2026-10-21, with the edge rules over record.note (card 104), served live and named by the one receipt cut before the rename; still called v1"},
	{"anam.v1-2026-10-21.dl", 1, "2026-10-21, with the edge rules over record.edge (card 191), the text on master until rule set v2; still called v1"},
}

// RuleSet is a named, versioned program with a hash over its text.
type RuleSet struct {
	Name    string
	Version int
	Hash    string
	Program Program
}

// hashOf is the rule set hash of a text: sha256 over its bytes, as a receipt names it.
func hashOf(text string) string {
	sum := sha256.Sum256([]byte(text))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func parseRuleSet(version int, text string) (RuleSet, error) {
	p, err := Parse(text)
	if err != nil {
		return RuleSet{}, fmt.Errorf("query: rule set %s v%d: %w", BundledName, version, err)
	}
	return RuleSet{Name: BundledName, Version: version, Hash: hashOf(text), Program: p}, nil
}

// Bundled parses the bundled rule set. It fails rather than returning a partial set:
// the rules are one reading, and a binary that lost half of it would answer wrongly
// with confidence.
func Bundled() (RuleSet, error) { return parseRuleSet(BundledVersion, bundledText) }

// HeldRuleSet is the rule set whose text has hash, among the bundled one and every
// text under rules/history. ok is false when no held text has that hash: the receipt
// was answered under rules this binary does not have, which is a moved rule set, and
// nothing can re-derive its answer.
func HeldRuleSet(hash string) (rs RuleSet, ok bool, err error) {
	if hashOf(bundledText) == hash {
		rs, err = Bundled()
		return rs, err == nil, err
	}
	for _, h := range history {
		text, err := historyFS.ReadFile("rules/history/" + h.File)
		if err != nil {
			return RuleSet{}, false, fmt.Errorf("query: rule set history %s: %w", h.File, err)
		}
		if hashOf(string(text)) == hash {
			rs, err = parseRuleSet(h.Version, string(text))
			return rs, err == nil, err
		}
	}
	return RuleSet{}, false, nil
}
