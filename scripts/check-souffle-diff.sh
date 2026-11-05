#!/usr/bin/env bash
# check-souffle-diff.sh: souffle-diff.py hands --subject (or its old name --person) to
# anam as ANAM_SUBJECT_ID and leaves ANAM_PERSON_ID alone, so anam's own config rule
# settles the two (card 270a): different values are refused, equal ones read with an
# INFO. Runs a freshly built anam in an empty environment under a temp directory; no
# souffle is needed, since every case ends in anam before souffle would run.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
T="$(mktemp -d)"
trap 'rm -rf "$T"' EXIT
go build -o "$T/anam" "$ROOT/cmd/anam"

fail=0
# expect <want> <env assignment> <flag> <value>: run the wrapper and require want in what it says.
expect() {
  local want=$1 var=$2 flag=$3 value=$4 out
  out="$(env -i PATH="$PATH" HOME="$T" XDG_CONFIG_HOME="$T/config" XDG_DATA_HOME="$T/data" "$var" \
    python3 -B -I "$ROOT/scripts/souffle-diff.py" --anam "$T/anam" --ledger "$T/ledger.db" \
    --projection none "$flag" "$value" 2>&1 || true)"
  if [[ "$out" != *"$want"* ]]; then
    echo "  FAILED: $var with $flag $value: want \"$want\", got:" >&2
    echo "$out" | sed 's/^/    /' >&2
    fail=1
  else
    echo "  ok: $var with $flag $value"
  fi
}
expect 'subject_id ("other") and its old name person_id ("matt") differ' ANAM_PERSON_ID=matt --subject other
expect 'subject_id ("matt") and its old name person_id ("other") differ' ANAM_PERSON_ID=other --person matt
expect 'subject_id and its old name person_id are both set and equal' ANAM_PERSON_ID=matt --subject matt
exit "$fail"
