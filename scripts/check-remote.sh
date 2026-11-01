#!/usr/bin/env bash
# check-remote: run the full gate (scripts/check.sh) on one pushed commit, on a build
# host when it is up, on this machine when it is not (card 262).
#
#   scripts/check-remote.sh [ref]          ref defaults to HEAD of the checkout you are in
#   scripts/check-remote.sh --local [ref]  skip the build host
#
# The gate runs in a clean, detached worktree at the commit's exact SHA, so the result
# names a commit rather than whatever the working tree held. The commit must be pushed:
# the build host fetches it from origin. The log lands in /tmp/anam-check-<sha>.log here,
# headed by the host, the anam and anam-importers SHAs, and the tool versions. The exit
# code is the gate's.
#
# check.sh skips goimports, golangci-lint, gosec and the explainer check when they are
# missing and can still print CHECK OK. This script refuses to run without them, so a
# pass from any machine means the same thing.
#
# Settings (environment):
#   ANAM_CHECK_HOST  the build host's Tailscale name, e.g. quantize-tower, the B550 tower
#                    (16 cores; see card 262 for the gate's time there). Unset: run locally.
#   ANAM_CHECK_ROOT  where the build host keeps its clones (default ~/anam-check)
#
# Build host setup, once:
#   - On the tailnet, reachable by `ssh <host>` with key auth (BatchMode).
#   - Go at or above go.mod's version on PATH for a non-interactive ssh shell, with
#     $(go env GOPATH)/bin on PATH too.
#   - go install golang.org/x/tools/cmd/goimports@latest
#   - go install github.com/securego/gosec/v2/cmd/gosec@v2.30.0
#   - golangci-lint v2 (the ThinkPad runs 2.10.1)
#   - Go newer than the lint tools can read (1.27 vs golangci-lint 2.10.1):
#     export GOTOOLCHAIN=go1.26.0 near the top of ~/.bashrc, then rebuild the three tools
#   - python3, git, flock
#   - HTTPS access to github.com (both repos are public, so no key is needed)
#     The script clones both into ANAM_CHECK_ROOT on first use.
set -euo pipefail

local_only=0
if [ "${1:-}" = "--local" ]; then local_only=1; shift; fi
ref="${1:-HEAD}"

repo="$(git rev-parse --show-toplevel)"
sha="$(git -C "$repo" rev-parse --verify "$ref^{commit}")"
short="${sha:0:7}"
log="/tmp/anam-check-$short.log"

# The gate as it runs on either machine: tools present, clean worktree at the SHA,
# explainer against anam-importers' origin/master. Arguments: sha, clones root.
# shellcheck disable=SC2016
gate='
set -uo pipefail
sha="$1"; root="$2"
missing=""
for t in go gofmt goimports golangci-lint gosec python3 git; do
  command -v "$t" >/dev/null 2>&1 || missing="$missing $t"
done
if [ -n "$missing" ]; then echo "check-remote: missing on $(hostname):$missing; not running a gate that would skip them"; exit 2; fi
mkdir -p "$root"
for r in anam anam-importers; do
  [ -d "$root/$r/.git" ] || git clone -q "https://github.com/mattjoyce/$r.git" "$root/$r"
  git -C "$root/$r" fetch -q origin
done
git -C "$root/anam-importers" checkout -q --detach origin/master
git -C "$root/anam" cat-file -e "$sha^{commit}" 2>/dev/null || { echo "check-remote: $sha is not on origin; push it first"; exit 2; }
run="$root/runs/$sha"
git -C "$root/anam" worktree remove --force "$run" 2>/dev/null || true
git -C "$root/anam" worktree add -q --detach "$run" "$sha"
trap "git -C \"$root/anam\" worktree remove --force \"$run\" >/dev/null 2>&1 || true" EXIT
echo "host:           $(hostname)"
echo "anam:           $sha"
echo "anam-importers: $(git -C "$root/anam-importers" rev-parse HEAD)"
echo "go:             $(go version)"
echo "golangci-lint:  $(golangci-lint --version 2>&1 | head -1)"
echo "started:        $(date -Is)"
echo
cd "$run" && EXPLAINER_IMPORTERS_DIR="$root/anam-importers" scripts/check.sh
'

host="${ANAM_CHECK_HOST:-}"
where=local
if [ "$local_only" -eq 0 ] && [ -n "$host" ]; then
  peer="$(tailscale status 2>/dev/null | awk -v h="$host" '$2 == h' || true)"
  if [ -n "$peer" ] && ! grep -q offline <<<"$peer" \
     && ssh -o BatchMode=yes -o ConnectTimeout=5 "$host" true 2>/dev/null; then
    where="$host"
  else
    echo "check-remote: $host is not reachable; running on $(hostname) instead" >&2
  fi
fi

if [ "$where" != local ]; then
  if ! git -C "$repo" branch -r --contains "$sha" | grep -q .; then
    echo "check-remote: $short is not on any remote branch; push it, or use --local" >&2
    exit 2
  fi
  root="${ANAM_CHECK_ROOT:-\$HOME/anam-check}"
  echo "check-remote: $short on $where, log $log" >&2
  set +e
  # shellcheck disable=SC2029
  ssh -o BatchMode=yes -o ServerAliveInterval=30 "$where" "bash -s -- $sha $root" <<<"$gate" >"$log" 2>&1
  status=$?
else
  # Locally the clones are the ones beside this checkout's repo, fetched as above.
  root="${ANAM_CHECK_ROOT:-$HOME/anam-check}"
  echo "check-remote: $short on $(hostname), log $log" >&2
  set +e
  bash -s -- "$sha" "$root" <<<"$gate" >"$log" 2>&1
  status=$?
fi
set -e
echo "finished:       $(date -Is), exit $status" >>"$log"
grep -E "FAILED|--- FAIL|missing on|not on origin" "$log" >&2 || true
tail -2 "$log" >&2
exit "$status"
