# The importers' day, audited (2026-10-10)

Hickey × Armstrong, After pass, over anam-importers `08d84ac..a01a193`: 14 commits between 10:42 and
22:06 on 2026-10-10. Cards 55 (Garmin), 70 (DreamStation), 78 (snapshots), 80 and 83 (blood
pressure), plus anam_submit's token_secret. 50 files, about 3,200 lines added. Three slices audited
independently, collated here. Companion to `luminary-day-audit-2026-10-10.md` for the Anam side.

No luminary pass ran on any of these commits before merge.

## Gate

At `a01a193`, exported clean of the working tree: `python3 -m unittest discover -s tests -t .`
passes, 56 tests, 12 skipped without a daemon. With `ANAM_BIN` pointing at an anam built from
`313f9c9`, three consecutive runs pass with 3 skips, all wanting a physical DreamStation card.
`compileall` is clean. Ruff in the E and F classes reports only E501 line length (225); no
undefined names, no unused imports. The inherited ruff config adds 233 pyupgrade and import-order
items, none of them defects. Hygiene grep for hosts, home paths and the operator's name finds one
hit, below. Every `run.py` is executable (c668104).

The working tree in `~/Projects/anam-importers` was being edited by the importers stream while this
audit ran (`garmin/run.py`, `tests/test_garmin.py`, two new Definitions for `garmin.day` and
`health.step-count`, 22:29). Runs against that tree failed in ways the committed head does not; the
uncommitted state fails `test_every_type_is_admitted_and_a_resend_is_no_op` with the fifth batch
superseded on first send. That is in-flight work, outside this window, and is theirs to finish.

## Findings, ranked

1. **bp_csv, medium, inherited from the Go worker. Six export columns are outside the closed
   schema, so one ticked checkbox stops the export.** `slug()` (`bp_csv/run.py:106`) yields
   `irregular_heartbeat`, `dizzy_light_headed`, `temperature_celsius`, `blood_glucose_mg_dl`,
   `oxygen_saturation_pct` and `tags`; v4 is `additionalProperties: false` and names none of them.
   The fixture leaves those cells blank, so no test sees it. Scenario: the app records one
   irregular-heartbeat flag; `check()` raises, `handle` returns `retry=False`, and every later export
   containing that row never lands, new readings included. The fix is a v5 Definition that admits
   them, or an importer that drops unnamed columns loudly.

2. **bp_csv, medium. Exports resend in path-string order, so an older export can supersede a newer
   correction.** `exports()` (`bp_csv/run.py:207`) sorts paths; `BloodPressure_9-October-2026`
   sorts after `BloodPressure_10-October-2026`. The gate takes the last changed payload, so a
   comment fixed in the app and re-exported is overwritten by the earlier export's typo on the next
   resend-everything. Sort by the name's date or mtime, and test it.

3. **dreamstation, medium. The run labels itself a snapshot even when the decoder skipped files.**
   `dreamstation_card/run.py:97-103` builds the snapshot parts unconditionally and only logs
   `card.skipped_files`. Garmin got this right the same day (`garmin/run.py:241,257` withholds the
   label when any file fails to decode). Scenario: a session file this run cannot read is not
   carried, the snapshot completes, Anam lists that night absent, and an operator who trusts the
   alert tombstones a night the card still holds.

4. **garmin, low. Manifest and module docstring describe two types; the plugin emits three.**
   `garmin/manifest.yaml:8` and `garmin/run.py:9` predate a01a193. An operator who grants the token
   from the manifest omits `health.heart-rate-daily` and every poll 403s on its third batch.

5. **bp_csv, low. "omh:blood-pressure:4.0, pinned" is claimed and nothing is pinned.** No
   `blood-pressure-4.0.json` under `tests/conformance/omh/`, and `test_conformance.py` covers only
   Garmin's two IEEE and OMH bodies. A drift in `date_time` form or posture vocabulary passes
   every test.

6. **garmin, low. A file removed between `rglob` and `stat` fails the run as not retryable**
   (`garmin/run.py:101-103`). A GarminDB sync replacing files mid-listing tells the supervisor not
   to retry when a retry would succeed.

7. **garmin, low. `is_main_sleep: true` is asserted by the importer, not read from Garmin**
   (`garmin/run.py:148`). Documented in the Definition, so a chosen constant; a nap table in a
   later GarminDB would make it silently false.

8. **dreamstation, low. The decoder is fetched code pinned to a tag, not a hash**
   (`dreamstation_card/run.py:6`). uv does the fetch and extraction, so traversal and size are its
   problem, but a moved tag changes what runs inside the gateway under the importer's identity.
   The README's "pins to a tag" should say what that does not guarantee.

9. **Hygiene, low.** `garmin/run.py:65` says "read from Matt's watch" in a public repo whose
   AGENTS.md bars personal identifiers. `Australia/Sydney` is restated as a code constant in
   `garmin/run.py:51` and `bp_csv/run.py:143` beside the Definitions that already carry it.

10. **bp_csv, low.** `number("abc")` fails without naming its row where a missing `date_excel`
    does; `number("nan")` yields NaN, which `json.dump` writes as invalid JSON. `records()` is
    sixty lines of bespoke CSV parsing to match Go's LazyQuotes and has no unit test for
    multi-line or doubled quotes.

## Clean

- Snapshots on the importer side: `{id, part, parts, types}` per batch, id random per run
  (correct: two runs are two claims), no importer state, a ductile retry resends the same id and
  part, a whole rerun starts a fresh snapshot and the old one expires server-side. The absent
  alert says "Nothing is retracted: the operator decides" and dedupes on the snapshot id.
- The token is read from the secret `token_secret` names, never logged, never on a command line,
  never in a response; the tests prove failures name the secret's name, not its value.
- Every HTTP call has a timeout and the `Anam-Timeout` header; 4xx is not retried, 5xx and 429
  are, and the body is kept for the operator.
- Garmin's three daily figures are Garmin's own columns reshaped one row to one facet, not an
  aggregate; the alert dedupe across files is keyed by the Definition's identity, so it is the
  batch rule, not a join. A truncated FIT file comes back as errors, not an exception, is logged,
  and the snapshot label is withdrawn.
- bp_csv's Definition is byte-identical to Anam's v4 manifest; identity is composed by the gate,
  not the importer; `effective_time_frame` is a pure function of the row and the zone.
- No function over 60 lines except `records()` at exactly 60.

## Hickey

Each importer is close to a pure function from source to payloads, and none holds record state.
The braids are the same across all three: a zone constant in code beside the Definition that
already states it; a derived time written as a fact next to the fact time it derives from; a body
half-converted, numbers in the OMH fields and strings in the Garmin or app fields beside them, so
one payload carries two vocabularies. Finding 1 is a schema that drifted from the source it
describes; finding 3 is a claim wider than its evidence.

## Armstrong

Failure and retry are named at every seam and the token lives in one process. Two findings are the
runtime kind. In finding 1 the blast radius is wrong: one cell fails an export of hundreds, and the
recovery is a Definition bump plus a resend, not a restart. In finding 3 a failure does not announce
itself as one: the skipped file is an info log, and the consequence lands in the irreversible path
on the Anam side. The fetched decoder is the one unsupervised dependency.

## Where the lenses converge and pull apart

Both land on finding 3: a claim wider than its evidence becomes a wrong tombstone under a plausible
fault. Both land on finding 1: a closed schema that does not name the source's columns is a braid
and an outage waiting on one checkbox. Combined with Anam-side finding 1 (absence judged against
the completion watermark alone), the retraction path now has two independent ways to list a facet
absent that the source still holds, and one irreversible step after them.

They pull apart on the conformance claim (finding 5): Armstrong is content that `check()` runs the
real validator on every payload; Hickey says the Definition claims a standard nobody loaded. And
on `is_main_sleep` (finding 7): Hickey wants a field the source did not write dropped; Armstrong
accepts it because the Definition says so and resend corrects it.
