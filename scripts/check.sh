#!/usr/bin/env bash
# Pre-merge quality gate (AGENTS.md). Runs the toolchain that enforces Go's strictness —
# the compiler is deliberately minimal; the rigour lives here.
set -uo pipefail

# One gate at a time per machine (card 227). golangci-lint refuses a second run, and two
# test runs share the CPU so the daemon tests miss their deadlines; either way the second
# gate fails for a reason that is not in the code. flock exists on Linux; without it (the
# Mac) the gate runs as before.
if command -v flock >/dev/null 2>&1; then
  exec 9>"${TMPDIR:-/tmp}/anam-check.lock"
  if ! flock -n 9; then
    echo "another gate is running; waiting for it to finish"
    flock 9
  fi
fi

fail=0
run() {
  echo "→ $*"
  local start=$SECONDS
  if ! "$@"; then
    echo "  FAILED: $*" >&2
    fail=1
  fi
  echo "  ($((SECONDS - start))s)"
}

# Formatting: gofmt -l / goimports -l print offending files (non-empty = failure).
offenders="$(gofmt -l . 2>/dev/null)"
if [ -n "$offenders" ]; then echo "gofmt would change:"; echo "$offenders"; fail=1; fi
if command -v goimports >/dev/null 2>&1; then
  offenders="$(goimports -l . 2>/dev/null)"
  if [ -n "$offenders" ]; then echo "goimports would change:"; echo "$offenders"; fail=1; fi
else
  echo "goimports not installed — skipping (install: golang.org/x/tools/cmd/goimports)"
fi

# Substrate boundary. The stores are accidental complexity — the problem is a personal
# longitudinal record, not a database — so the choice must stay reversible. Only
# internal/store may name SQLite or database/sql; the domain must not know what it is
# stored in.
#
# Nothing outside the store reads a source: every source reaches the record through
# an importer, over the envelope API (card 119).
echo "→ substrate boundary (stores confined to internal/store)"
imports="$(go list -f '{{.ImportPath}}{{range .Imports}} {{.}}{{end}}{{range .TestImports}} {{.}}{{end}}{{range .XTestImports}} {{.}}{{end}}' ./... 2>/dev/null)"
boundary="$(echo "$imports" \
  | grep -E 'database/sql|sqlite' \
  | grep -vE '^github.com/mattjoyce/anam/internal/store' \
  || true)"
if [ -n "$boundary" ]; then
  echo "  FAILED: the store substrate is named outside internal/store:" >&2
  echo "$boundary" | awk '{print "    " $1}' >&2
  fail=1
fi

# Fixture boundary (card 255). The test catalogs in internal/catalog/fixture, and
# catalogtest built on them, are for tests only: a binary that carried one would carry
# a catalog nobody chose. The rule reads the real dependency graph, so an import
# through another package (one under testdata, which ./... leaves out) is caught: no
# package but fixture and catalogtest may have either among its non-test deps, and no
# package any of them builds may //go:embed a file of fixture's testdata. These stop a
# test fixture being relied on by accident; a deliberate way round would show in review.
# internal/catalog/fixture/boundary_test.go holds the same rule without a go command.
echo "→ fixture boundary (test catalogs reach no production package)"
fixture_pkgs='github.com/mattjoyce/anam/internal/catalog/(fixture|catalogtest)'
prod_pkgs="$(go list ./... 2>/dev/null | grep -vE "^${fixture_pkgs}\$" || true)"
fixture_data="$(go list -f '{{.Dir}}' github.com/mattjoyce/anam/internal/catalog/fixture 2>/dev/null)/testdata"
if [ -z "$prod_pkgs" ] || [ ! -d "$fixture_data" ]; then
  echo "  FAILED: cannot list the packages or find the fixture's testdata" >&2
  fail=1
else
  # shellcheck disable=SC2086 # one argument per package
  fixture_boundary="$(go list -e -f '{{.ImportPath}}{{range .Deps}} {{.}}{{end}}' $prod_pkgs 2>/dev/null \
    | grep -E " ${fixture_pkgs}( |\$)" || true)"
  # shellcheck disable=SC2086
  fixture_embeds="$(go list -e -deps -f '{{$d := .Dir}}{{.ImportPath}}{{range .EmbedFiles}} {{$d}}/{{.}}{{end}}' $prod_pkgs 2>/dev/null \
    | grep -F " $fixture_data/" | grep -vE "^${fixture_pkgs} " || true)"
  # Native sources can carry bytes no import or //go:embed shows (an .incbin in a cgo
  # preamble or a .s file, a .syso), so no package of this module that production
  # builds may have any. The module has none; one that needs it revisits this rule.
  # shellcheck disable=SC2086
  fixture_native="$(go list -e -deps -f '{{.ImportPath}} {{join .CgoFiles " "}} {{join .CFiles " "}} {{join .CXXFiles " "}} {{join .MFiles " "}} {{join .HFiles " "}} {{join .FFiles " "}} {{join .SFiles " "}} {{join .SwigFiles " "}} {{join .SwigCXXFiles " "}} {{join .SysoFiles " "}}' $prod_pkgs 2>/dev/null \
    | grep -E '^github.com/mattjoyce/anam(/| )' | awk 'NF > 1' || true)"
  # A symlink to a directory is an import path the dependency graph cannot see
  # through: an alias of the fixture builds under another name. None is needed, so
  # none is allowed, and none may point into the fixture either.
  fixture_real="$(cd "$fixture_data/.." && pwd -P)"
  fixture_links="$(find . -path ./.git -prune -o -type l -print | while IFS= read -r link; do
      target="$(realpath -q "$link" 2>/dev/null || true)"
      if [ -d "$link" ] || [ "$target" = "$fixture_real" ] || [[ "$target" == "$fixture_real"/* ]]; then
        echo "$link"
      fi
    done)"
  if [ -n "$fixture_native" ]; then
    echo "  FAILED: a production package has native sources:" >&2
    echo "$fixture_native" | awk '{print "    " $0}' >&2
    fail=1
  fi
  if [ -n "$fixture_links" ]; then
    echo "  FAILED: a symlink aliases a directory or the fixture:" >&2
    echo "$fixture_links" | awk '{print "    " $0}' >&2
    fail=1
  fi
  if [ -n "$fixture_boundary" ]; then
    echo "  FAILED: a test catalog is in a production package's dependencies:" >&2
    echo "$fixture_boundary" | awk '{print "    " $1}' >&2
    fail=1
  fi
  if [ -n "$fixture_embeds" ]; then
    echo "  FAILED: a production package embeds the fixture's testdata:" >&2
    echo "$fixture_embeds" | awk '{print "    " $1}' >&2
    fail=1
  fi
fi

# The prose is checked too: a dead link in the README's docs index or in docs/ fails
# the gate, so the index a stranger follows is true (card 73).
run scripts/check-links.sh

# The explainer bundle cites tracked files in this repo and in anam-importers, beside this
# checkout or at EXPLAINER_IMPORTERS_DIR. Without that checkout the citations cannot be read.
if [ -d "${EXPLAINER_IMPORTERS_DIR:-../anam-importers}" ]; then
  run python3 -B -I docs/explainer/check.py
else
  echo "anam-importers checkout not found — skipping the explainer check (set EXPLAINER_IMPORTERS_DIR)"
fi

run go vet ./...
if command -v golangci-lint >/dev/null 2>&1; then run golangci-lint run ./...; else echo "golangci-lint not installed — skipping"; fi
# gosec: the gate passes clean under v2.24.0, v2.29.0 and v2.30.0; use v2.30.0. Under v2.24.0 rule
# G120 spent over four minutes on internal/inspect alone (card 262); v2.30.0 runs the whole
# repo in seconds. v2.29.0 adds the rules G703, G710 and
# G124. G703 and G710 are fixed. G124, the inspector's default-view cookie without the Secure
# flag, carries a #nosec Matt approved on 2026-10-22 (internal/inspect/timeline_view.go).
if command -v gosec >/dev/null 2>&1; then run gosec -severity medium -quiet ./...; else echo "gosec not installed — skipping"; fi
# The suite is integration-heavy by choice (AGENTS.md: real components over mocks).
# Every ledger and every operational store under test is a file in that test's own temp
# directory, so packages run in parallel without queueing on anything shared.
# Tests are hermetic: the developer's ANAM_ settings (a profile exporting ANAM_SERVER,
# say) must not steer a test toward a real daemon or ledger.
for v in $(compgen -e | grep '^ANAM_' || true); do unset "$v"; done
run go test -race -count=1 -shuffle=on -vet=all ./...

if [ "$fail" -ne 0 ]; then echo "CHECK FAILED"; exit 1; fi
echo "CHECK OK"
