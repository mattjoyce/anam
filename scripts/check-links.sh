#!/usr/bin/env bash
# Dead links in the prose (card 73). Every relative link in README.md, AGENTS.md and
# docs/*.md must name a file or directory that exists, so the docs index the README
# promises is checked, not trusted. Links to the web are not fetched: the gate runs
# offline. The explainer bundle checks its own links (docs/explainer/check.py).
set -uo pipefail
cd "$(dirname "$0")/.." || exit 1

# links prints one "file<TAB>target" per Markdown link, the fragment and any title
# stripped, so a link to a heading in a file is checked as a link to the file.
links() {
  for f in README.md AGENTS.md docs/*.md; do
    grep -o '\]([^)]*)' "$f" \
      | sed -e 's/^](//' -e 's/)$//' -e 's/ ".*$//' -e 's/#.*$//' \
      | awk -v f="$f" 'length($0) {print f "\t" $0}'
  done
}

dead=0
while IFS=$'\t' read -r f target; do
  case "$target" in http://*|https://*|mailto:*) continue ;; esac
  if [ ! -e "$(dirname "$f")/$target" ]; then
    echo "  $f: dead link ($target)" >&2
    dead=1
  fi
done < <(links)

if [ "$dead" -ne 0 ]; then
  echo "  FAILED: dead links in the prose" >&2
  exit 1
fi
