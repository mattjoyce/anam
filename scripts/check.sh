#!/usr/bin/env bash
# Pre-merge quality gate (AGENTS.md). Runs the toolchain that enforces Go's strictness —
# the compiler is deliberately minimal; the rigour lives here.
set -uo pipefail

fail=0
run() {
  echo "→ $*"
  if ! "$@"; then
    echo "  FAILED: $*" >&2
    fail=1
  fi
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

run go vet ./...
if command -v golangci-lint >/dev/null 2>&1; then run golangci-lint run ./...; else echo "golangci-lint not installed — skipping"; fi
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
