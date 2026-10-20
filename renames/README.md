# Rename tables

The tables `anam rename --table <file>` runs (card 140): which type every live facet of a type
leaves and which it arrives as. Each facet gets one new version under the new type with the same
id, source, identity values, fact-time and payload; the old version stays, so a read as-of before
the run is unchanged.

- `taxonomy.json`: the one rename of 2026-10, card 147. Every type moves under one of the eight
  domains (`health`, `activity`, `place`, `record`, with `social`, `money`, `media` and `work`
  reserved). The retired health-db types `health.heart-rate`, `health.sleep` and
  `health.metric-group`, tombstoned at card 55, are not in it and stay as they are.

A table names types the binary bundles or the daemon holds as registered, so the importers' new
Definitions (`health.cpap-session`, `activity.day`, `place.stay`, `place.track`) are registered
before the run. The run is dry by default and reports what the gate would refuse; `--yes` writes,
with the daemon stopped.

```sh
anam rename --table renames/taxonomy.json          # the dry run: counts and refusals
anam rename --table renames/taxonomy.json --yes    # the run
```
