#!/usr/bin/env bash
# Is the health pipeline moving? (docs/pipeline.html, "How to tell a stage has stopped")
#
# Every stage can stop without an error: the process stays up and the gateway stays
# healthy. The only reliable sign is that a stage's output stops moving. This reads
# the newest date each stage holds and compares neighbours.
#
# Exit 0  every stage is within MAX_LAG_DAYS of the one before it (FRESH)
# Exit 1  a stage has fallen behind (STALE); the reasons are on stdout
# Exit 2  a stage could not be read; the check itself failed
#
# The ThinkPad ductile gateway runs this daily as job `health_staleness` and posts
# stdout to Discord on exit 1. Paths can be overridden for a test.
#
# With ANAM_SERVER set, Anam's stages are asked of the daemon (its timeline, with
# the token in ANAM_TOKEN_FILE), since the daemon owns the ledger and nothing else
# may open it. Without it they are read from LEDGER_DB.
set -uo pipefail

MAX_LAG_DAYS="${MAX_LAG_DAYS:-2}"
GARMIN_DB="${GARMIN_DB:-/mnt/Projects/healthdata/garmin/DBs/garmin.db}"
WITHINGS_DB="${WITHINGS_DB:-/mnt/Projects/healthdata/withings/withings.db}"
LEDGER_DB="${LEDGER_DB:-$HOME/.local/share/anam/ledger.db}"
ANAM_SERVER="${ANAM_SERVER:-}"
ANAM_TOKEN_FILE="${ANAM_TOKEN_FILE:-$HOME/.config/anam/token}"
TODAY="${TODAY:-$(date +%F)}"
# DAY_ZONE is the zone whose calendar the sources' days are in, so an Anam fact time
# (an instant) is compared as the date it falls on there.
DAY_ZONE="${DAY_ZONE:-Australia/Sydney}"

# newest <label> <db> <sql>: prints the first 10 characters of the scalar (a date),
# or exits 2 naming the stage that could not be read on stderr, which is what the
# gateway reports as the job's error. Read-only, and bounded so a share whose server
# has gone cannot hold the check forever.
newest() {
  local label=$1 db=$2 sql=$3 out
  if [ ! -r "$db" ]; then
    echo "CHECK FAILED: $label: cannot read $db" >&2
    exit 2
  fi
  if ! out=$(timeout 30 sqlite3 -batch "file:${db}?mode=ro" "$sql" 2>&1); then
    echo "CHECK FAILED: $label: $out" >&2
    exit 2
  fi
  if [ -z "$out" ]; then
    echo "CHECK FAILED: $label: no rows" >&2
    exit 2
  fi
  printf '%s' "${out:0:10}"
}

# newest_anam <label> <facet type>: the newest live fact-time of a type, from the
# daemon when ANAM_SERVER is set, else from the ledger file. The window is the last
# 90 days: a stage further behind than that is stale whatever its exact date.
newest_anam() {
  local label=$1 typ=$2 out from
  if [ -z "$ANAM_SERVER" ]; then
    newest "$label" "$LEDGER_DB" "SELECT MAX(fact_time) FROM facet WHERE type='$typ'"
    return
  fi
  from=$(date -d "$TODAY -90 days" +%F)
  if ! out=$(curl -sS --fail-with-body --max-time 30 \
      -H "Authorization: Bearer $(cat "$ANAM_TOKEN_FILE")" \
      "${ANAM_SERVER%/}/v1/timeline?zone=UTC&type=${typ}&from=${from}" 2>&1); then
    echo "CHECK FAILED: $label: $out" >&2
    exit 2
  fi
  if ! out=$(jq -r '.entries[-1].fact_time // empty' <<<"$out" 2>&1); then
    echo "CHECK FAILED: $label: $out" >&2
    exit 2
  fi
  if [ -z "$out" ]; then
    echo "CHECK FAILED: $label: no rows since $from" >&2
    exit 2
  fi
  TZ="$DAY_ZONE" date -d "$out" +%F
}

# days_between <earlier> <later>: whole days from one calendar date to another.
days_between() {
  echo $(( ( $(date -d "$2" +%s) - $(date -d "$1" +%s) ) / 86400 ))
}

garmin=$(newest "Garmin collected" "$GARMIN_DB" "SELECT MAX(day) FROM daily_summary") || exit $?
withings=$(newest "Withings collected" "$WITHINGS_DB" "SELECT MAX(date) FROM measurements") || exit $?
anam_day=$(newest_anam "Anam admitted (Garmin days)" garmin.day) || exit $?
anam_meas=$(newest_anam "Anam ingested (Withings)" health.measurement) || exit $?

printf 'Garmin collected      %s\nWithings collected    %s\nAnam, Garmin days     %s\nAnam, Withings rows   %s\n' \
  "$garmin" "$withings" "$anam_day" "$anam_meas"

stale=0
# lag <what> <earlier> <later>: reports when <later> is more than MAX_LAG_DAYS behind
# <earlier>. Withings is not compared with today: a scale is stepped on when it is.
lag() {
  local what=$1 earlier=$2 later=$3 d
  d=$(days_between "$later" "$earlier")
  if [ "$d" -gt "$MAX_LAG_DAYS" ]; then
    echo "STALE: $what is $d days behind"
    stale=1
  fi
}
lag "Garmin collection (against today)" "$TODAY" "$garmin"
lag "Anam's Garmin days (against GarminDB)" "$garmin" "$anam_day"
lag "Anam ingest of Withings rows (against Withings)" "$withings" "$anam_meas"

if [ "$stale" -eq 0 ]; then
  echo "FRESH: every stage is within $MAX_LAG_DAYS days of the one before it"
fi
exit "$stale"
