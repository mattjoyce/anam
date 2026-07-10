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
# internal/store may name DynamoDB, SQLite or database/sql; the domain must not know
# what it is stored in. Because Anam owns no source fact, a swap is a re-ingest.
#
# One carve-out: internal/capture may use database/sql/sqlite to READ a source in its
# native format (healthdata.db is a SQLite file that happens to exist in the world).
# That is not Anam's substrate — a capture worker is outside the gate, disposable, and
# speaks whatever its source speaks. The ledger's SDK stays confined to the store.
echo "→ substrate boundary (stores confined to internal/store; sources to internal/capture)"
boundary="$(go list -f '{{.ImportPath}}{{range .Imports}} {{.}}{{end}}{{range .TestImports}} {{.}}{{end}}' ./... 2>/dev/null \
  | grep -E 'database/sql|sqlite|aws-sdk-go' \
  | grep -vE '^github.com/mattjoyce/anam/internal/(store|capture)' \
  || true)"
# capture packages may name sql/sqlite but never the ledger's SDK:
capture_aws="$(go list -f '{{.ImportPath}}{{range .Imports}} {{.}}{{end}}{{range .TestImports}} {{.}}{{end}}' ./internal/capture/... 2>/dev/null \
  | grep 'aws-sdk-go' || true)"
if [ -n "$capture_aws" ]; then
  echo "  FAILED: a capture worker names the ledger's SDK:" >&2
  echo "$capture_aws" | awk '{print "    " $1}' >&2
  fail=1
fi
if [ -n "$boundary" ]; then
  echo "  FAILED: the store substrate is named outside internal/store:" >&2
  echo "$boundary" | awk '{print "    " $1}' >&2
  fail=1
fi

run go vet ./...
if command -v golangci-lint >/dev/null 2>&1; then run golangci-lint run ./...; else echo "golangci-lint not installed — skipping"; fi
if command -v gosec >/dev/null 2>&1; then run gosec -severity medium -quiet ./...; else echo "gosec not installed — skipping"; fi
run go test -race -count=1 -shuffle=on -vet=all ./...

if [ "$fail" -ne 0 ]; then echo "CHECK FAILED"; exit 1; fi
echo "CHECK OK"
