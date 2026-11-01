#!/usr/bin/env bash
# slow-tests: run tests the way the gate does and rank them by time (card 262).
#
#   scripts/slow-tests.sh [packages...]   default ./...
#
# Prints each package's wall time beside the sum of its tests' times, then the 25
# slowest tests. A package whose sum is close to its wall time runs its tests one after
# another; with one second each, that package is the gate's floor however many cores
# the machine has. Exit code is go test's.
set -uo pipefail
cd "$(dirname "$0")/.."
[ $# -gt 0 ] || set -- ./...
out="$(mktemp)"
trap 'rm -f "$out"' EXIT
go test -race -count=1 -json "$@" >"$out"
status=$?
python3 -I - "$out" <<'PY'
import json, sys
tests, wall = [], {}
for line in open(sys.argv[1]):
    try:
        e = json.loads(line)
    except ValueError:
        continue
    if e.get("Action") not in ("pass", "fail") or "Elapsed" not in e:
        continue
    pkg = e["Package"].rsplit("/", 1)[-1]
    if "Test" not in e:
        wall[pkg] = e["Elapsed"]
    elif "/" not in e["Test"]:
        tests.append((e["Elapsed"], pkg, e["Test"], e["Action"]))
print("%-14s %8s %8s %6s" % ("package", "wall", "sum", "tests"))
for pkg, w in sorted(wall.items(), key=lambda kv: -kv[1]):
    mine = [t[0] for t in tests if t[1] == pkg]
    if mine:
        print("%-14s %7.1fs %7.1fs %6d" % (pkg, w, sum(mine), len(mine)))
print()
for elapsed, pkg, name, action in sorted(tests, reverse=True)[:25]:
    print("%7.2fs  %-12s %s%s" % (elapsed, pkg, name, "" if action == "pass" else "  FAIL"))
PY
exit "$status"
