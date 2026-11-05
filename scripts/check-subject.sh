#!/usr/bin/env bash
# check-subject.sh: the engine calls the thing its record is about the subject (card 270a).
# This step fails when an identifier shape of the old word, person, appears anywhere in
# the tree outside the allowlist below. It looks for identifier shapes only, so prose
# where a human acts ("the person types", "the person's words") is not policed.
#
# Every allowlist entry must still match at least one hit. An entry that matches nothing
# fails the step, so the table shrinks as 270b and the alias-removal release land.
#
# The limit: an entry allows a file and a line pattern, not a line. A new use of an
# allowlisted name in an allowlisted file passes (another person_id in config.go, say).
# Review covers those; the entries are as narrow as the names in each file allow.
set -euo pipefail

if ! command -v git >/dev/null 2>&1; then
  echo "  FAILED: check-subject.sh needs git, and git is not on PATH" >&2
  exit 1
fi
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
if [ "$(git rev-parse --is-inside-work-tree 2>/dev/null)" != "true" ]; then
  echo "  FAILED: $ROOT is not a git work tree, so the scan cannot run" >&2
  exit 1
fi

# The identifier shapes, as EREs matched without regard to case, each over the files
# where it names an identifier rather than prose. (^|[^[:alpha:]]) stands for \b, which
# not every regex library reads.
#
# Everywhere: the id in any spelling (person_id, personId, PERSON_ID, Person_ID), the
# asserted field, a quoted key or json tag ("person", "person,omitempty"), the flag, the
# old CLI output "(person <id>)", the old error name, and the old span label.
PAT_ALL='person_?id|asserted_?person|"person[",]|--person|\(person[[:space:]]|errpersonmismatch|whole[-_ ]?person'
# A YAML or JSON key at any indent, quoted or not and as a list item. Only in YAML and
# JSON files, since prose may start a line "Person: someone who uses Anam."
PAT_KEY='^[[:space:]]*(-[[:space:]]+)?['\''"]?person['\''"]?[[:space:]]*:'
KEY_FILES=('*.yaml' '*.yml' '*.json')
# An assignment, person=x or person =x. Not in markdown, where "the person = x" is prose.
PAT_SET='(^|[^[:alpha:]])person ?=[^[:space:]]'
SET_FILES=('.' ':!*.md')

# Out of scope, by path. Tests follow the Go rename mechanically, and the alias tests
# must say person_id; old receipts and fixtures say whole-person because that is what
# they were filed with (receipt_test.go, principal_test.go, the schema-9 fixtures that
# schema10_carry_test.go verifies). A .go file under testdata that is not a test is
# still scanned, since a package can import it. History is not rewritten, and the
# explainer page is generated from the bundle, which is checked.
in_scope() {
  case $1 in
    *_test.go) return 1 ;;              # tests, including the alias tests and old receipts
    */testdata/*.go) return 0 ;;        # code under testdata is code
    */testdata/*) return 1 ;;           # fixtures, including opstest/testdata/schema9
    CHANGELOG.md | docs/HISTORY.md) return 1 ;; # history
    docs/explainer/explainer.html) return 1 ;;  # generated from docs/explainer/bundle
  esac
  return 0
}

# The allowlist: allow <file> <ERE the hit line must also match, case-sensitive> <reason>.
paths=() lines=() reasons=()
allow() { paths+=("$1"); lines+=("$2"); reasons+=("$3"); }

# The config alias, until the alias-removal release.
allow internal/config/config.go   'person_id|PersonID' 'the person_id alias and the retired PersonID field'
allow internal/config/resolved.go 'person_id'          'the person_id alias in composition and provenance'
# The deprecated wire fields, until the alias-removal release.
allow internal/api/api.go          'json:"person"'   'the deprecated person field on /v1/status'
allow internal/api/importers.go    'AssertedPerson|asserted_person' 'the deprecated asserted_person envelope field'
allow internal/daemon/envelopes.go 'AssertedPerson|asserted_person' 'assertedSubjectOf settles asserted_person, the old name'
allow internal/daemon/openapi.yaml '^        person:$|asserted_person|whole-person' 'documents the deprecated fields and the old span label'
allow docs/API.md                  'whole-person'    'says receipts filed before 270a keep span whole-person'
allow docs/IMPORTERS.md            'asserted_person' 'documents asserted_person as the deprecated old name'
allow docs/explainer/bundle/data-structures/batch-submission.md 'asserted_person' 'documents asserted_person as the deprecated old name'
allow docs/explainer/bundle/data-structures/envelope.md         'asserted_person' 'documents asserted_person as the deprecated old name'
# The deprecated --person flags, until the alias-removal release.
allow internal/app/query.go         '--person|"person"'                    'the deprecated --person alias of query check --subject'
allow scripts/souffle-diff.py       '--person|ANAM_PERSON_ID'              'the --person argparse alias and ANAM_PERSON_ID'
allow scripts/check-souffle-diff.sh '--person|person_id|ANAM_PERSON_ID'    'the gate step that proves the wrapper leaves the alias to anam'
allow scripts/check.sh              'ANAM_PERSON_ID' 'the comment on the check-souffle-diff.sh step'
# The stored names, until 270b.
allow internal/store/ledger/schema.go 'person_id|PersonID' 'the ledger_meta key person_id and the binding_person_id column'
allow internal/store/ledger/ledger.sql 'binding_person_id' 'the stored column, hash-pinned'
allow internal/store/ledger/row.go    'binding_person_id'  'the stored column in the row codec'
allow internal/store/ledger/ledger.go 'metaPersonID'       'pins and checks the stored ledger_meta key'
allow docs/ARCHITECTURE.md            'person_id'          'section 8 documents the stored ledger_meta key'
allow docs/explainer/bundle/components/store-ledger.md        'person_id'         'the stored ledger_meta key'
allow docs/explainer/bundle/data-structures/facet-artifact.md 'person_id'         'the stored binding_person_id column and ledger_meta key'
# Kept names and other repositories.
allow docs/explainer/bundle/invariants/index.md 'whole-person-read-is-the-default\.md' 'links the decision page, whose file name and anchor ids are kept'
allow docs/explainer/bundle/components/importer-base.md 'asserted_person' "anam-importers' own envelope() still takes asserted_person"
allow docs/explainer/bundle/workflows/admit-a-batch.md  'asserted_person' "anam-importers' own envelope() still yields asserted_person"
allow docs/explainer/bundle/log.md 'whole-person' "the bundle's own history"
allow scripts/check-subject.sh     '.' 'this check names the patterns'

used=()
for i in "${!paths[@]}"; do used[i]=0; done

# The scan, one git grep per shape. git grep answers 0 with hits, 1 with none, and more
# than 1 on an error; any other status, or anything on stderr (an unreadable file, say),
# means a pass did not cover its files, and an incomplete scan must not pass.
T="$(mktemp -d)"
trap 'rm -rf "$T"' EXIT
scan() { # scan <name> <pattern> <pathspec>...
  local name=$1 pattern=$2 status=0
  shift 2
  git -c color.grep=never grep --no-color -n -I -i --untracked -E "$pattern" -- "$@" \
    >"$T/$name" 2>"$T/$name.err" || status=$?
  if [ "$status" -gt 1 ] || [ -s "$T/$name.err" ]; then
    echo "  FAILED: git grep did not finish the $name scan (status $status):" >&2
    sed 's/^/    /' "$T/$name.err" >&2
    exit 1
  fi
}
scan all "$PAT_ALL" .
scan key "$PAT_KEY" "${KEY_FILES[@]}"
scan set "$PAT_SET" "${SET_FILES[@]}"
# A line two shapes match is one hit.
sort -u "$T/all" "$T/key" "$T/set" >"$T/hits"

fail=0
hits=0
while IFS= read -r hit; do
  file=${hit%%:*}
  rest=${hit#*:}
  text=${rest#*:}
  in_scope "$file" || continue
  hits=$((hits + 1))
  allowed=0
  for i in "${!paths[@]}"; do
    if [ "$file" = "${paths[i]}" ] && [[ $text =~ ${lines[i]} ]]; then
      used[i]=1
      allowed=1
      break
    fi
  done
  if [ "$allowed" -eq 0 ]; then
    echo "  FAILED: an old name outside the allowlist: $file:${rest%%:*}: $text" >&2
    fail=1
  fi
done <"$T/hits"

for i in "${!paths[@]}"; do
  if [ "${used[i]}" -eq 0 ]; then
    echo "  FAILED: the allowlist entry for ${paths[i]} (${lines[i]}) matches nothing; remove it (${reasons[i]})" >&2
    fail=1
  fi
done

if [ "$fail" -ne 0 ]; then
  echo "  the engine says subject; see card 270 and the allowlist in scripts/check-subject.sh" >&2
  exit 1
fi
echo "  ok: $hits allowlisted lines, ${#paths[@]} entries, no other hits"
