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

run go vet ./...
if command -v golangci-lint >/dev/null 2>&1; then run golangci-lint run ./...; else echo "golangci-lint not installed — skipping"; fi
if command -v gosec >/dev/null 2>&1; then run gosec -severity medium -quiet ./...; else echo "gosec not installed — skipping"; fi
run go test -race -count=1 -shuffle=on -vet=all ./...

if [ "$fail" -ne 0 ]; then echo "CHECK FAILED"; exit 1; fi
echo "CHECK OK"
