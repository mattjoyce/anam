#!/usr/bin/env bash
# check-subject.sh: the engine calls the thing its record is about the subject (card 270a).
# This step fails when an identifier shape of the old word, person, appears anywhere in
# the tree outside the allowlist below. It looks for identifier shapes only, so prose
# where a human acts ("the person types", "the person's words") is not policed.
#
# Every allowlist entry must still match at least one hit. An entry that matches nothing
# fails the step, so the table shrinks as 270b and the alias-removal release land.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

# The identifier shapes, as one ERE. (^|[^[:alnum:]_])person= stands for \bperson=,
# which not every regex library reads.
PATTERN='person_id|PERSON_ID|asserted_person|AssertedPerson|PersonID|personID|"person"|--person|(^|[^[:alnum:]_])person=|\(person |ErrPersonMismatch|whole-person|whole person'

# Out of scope. Tests and their fixtures follow the Go rename mechanically, and the alias
# tests must say person_id; old receipts and fixtures say whole-person because that is
# what they were filed with (receipt_test.go, principal_test.go, the schema-9 fixtures
# that schema10_carry_test.go verifies). History is not rewritten, and the explainer
# page is generated from the bundle, which is checked.
EXCLUDE=(
  ':!*_test.go'            # tests, including the alias tests and old receipts
  ':!*/testdata/*'         # fixtures, including opstest/testdata/schema9
  ':!CHANGELOG.md'         # history
  ':!docs/HISTORY.md'      # history
  ':!docs/explainer/explainer.html' # generated from docs/explainer/bundle
)

# The allowlist: allow <file> <ERE the hit line must also match> <reason>.
paths=() lines=() reasons=()
allow() { paths+=("$1"); lines+=("$2"); reasons+=("$3"); }

# The config alias, until the alias-removal release.
allow internal/config/config.go   '.' 'the person_id alias and the retired PersonID field'
allow internal/config/resolved.go '.' 'the person_id alias in composition and provenance'
# The deprecated wire fields, until the alias-removal release.
allow internal/api/api.go          '"person"'        'the deprecated person field on /v1/status'
allow internal/api/importers.go    'AssertedPerson|asserted_person' 'the deprecated asserted_person envelope field'
allow internal/daemon/envelopes.go 'AssertedPerson|asserted_person' 'assertedSubjectOf settles asserted_person, the old name'
allow internal/daemon/openapi.yaml 'asserted_person|whole-person'   'documents the deprecated field and the old span label'
allow docs/API.md                  'whole-person'    'says receipts filed before 270a keep span whole-person'
allow docs/IMPORTERS.md            'asserted_person' 'documents asserted_person as the deprecated old name'
allow docs/explainer/bundle/data-structures/batch-submission.md 'asserted_person' 'documents asserted_person as the deprecated old name'
allow docs/explainer/bundle/data-structures/envelope.md         'asserted_person' 'documents asserted_person as the deprecated old name'
# The deprecated --person flags, until the alias-removal release.
allow internal/app/query.go         '.'              'the deprecated --person alias of query check --subject'
allow scripts/souffle-diff.py       '.'              'the --person argparse alias and ANAM_PERSON_ID'
allow scripts/check-souffle-diff.sh '.'              'the gate step that proves the wrapper leaves the alias to anam'
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
allow docs/explainer/bundle/log.md '.' "the bundle's own history"
allow scripts/check-subject.sh     '.' 'this check names the patterns'

used=()
for i in "${!paths[@]}"; do used[i]=0; done

fail=0
hits=0
while IFS= read -r hit; do
  hits=$((hits + 1))
  file=${hit%%:*}
  rest=${hit#*:}
  text=${rest#*:}
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
done < <(git grep -n -I --untracked -E "$PATTERN" -- . "${EXCLUDE[@]}" || true)

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
