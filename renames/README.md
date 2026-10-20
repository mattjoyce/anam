# Rename tables

The tables `anam rename --table <file>` runs (card 140): which type every live facet of a type
leaves and which it arrives as. Each facet gets one new version under the new type with the same
id, source, identity values, fact-time and payload; the old version stays, so a read as-of before
the run is unchanged.

- `taxonomy.json`: the taxonomy rename of 2026-10-20, card 147. Every type moves under one of the eight
  domains (`health`, `activity`, `place`, `record`, with `social`, `money`, `media` and `work`
  reserved). The retired health-db types `health.heart-rate`, `health.sleep` and
  `health.metric-group`, tombstoned at card 55, are not in it and stay as they are.
- `record-edge.json`: card 191, `record.note` to `record.edge`, the note that card 104 made an edge.
  Its own table, because `taxonomy.json` already arrives at `record.note` and a table is one step.
- `kinds-segment.json`: card 196, twelve 1:1 renames that give the medication, illness and trip
  types their third segment (`health.medication-started` to `health.medication.started`,
  `health.medication-missed-dose` to `health.medication.dose-missed`, `health.illness-ended` to
  `health.illness.ended`, `place.flight` to `place.trip.flight`, and the rest alike). Its own
  table, because `taxonomy.json` arrives at the hyphenated names and a table is one step. The
  `med.regimen`, `illness.episode` and `travel.trip` series are stamped by each Definition and move
  with it, so a read through the `meds` projection is the same before and after.

## The table

```json
{"renames": [
  {"from": "health.heart-rate-daily", "to": "health.heart-rate-day"},
  {"from": "activity.exercise-session", "to": "activity.exercise-session.technogym",
   "select": {"source": "records", "pointer": "/id", "prefix": "technogym:"}}
]}
```

A row without `select` carries every live facet of its `from`, and no other row may name that
`from`. A row with `select` carries only the facets it matches, so several rows split one type
into leaves (card 194). `source` matches the facet's source; `pointer` and `prefix` come
together and match when the payload holds a string at the pointer that starts with the prefix.
Each is optional, and a `select` names at least one. Every live facet of a split type must match
exactly one row: the dry run lists the facets that match none and those that match more than
one, and the run refuses on either before it writes anything. Rows are not tried in order, so
the table means the same whichever way its rows are sorted. A table is still one step, with no
type both left and arrived at.

A split's leaves repeat the fields they share, since a manifest cannot reference another file.
`internal/def/shared-core.json` names, for each split type, its leaves and those shared pointers,
and a def test holds the schema at each one identical across the leaves, description and
examples aside.

A table names types the binary bundles or the daemon holds as registered, so the importers' new
Definitions (`health.cpap-session`, `activity.day`, `place.stay`, `place.track`) are registered
before the run. The run is dry by default and reports what the gate would refuse; `--yes` writes,
with the daemon stopped.

```sh
anam rename --table renames/taxonomy.json          # the dry run: counts and refusals
anam rename --table renames/taxonomy.json --yes    # the run
```
