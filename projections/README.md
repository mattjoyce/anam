# Registered projection templates

The templates the operator registers with the live daemon. They are not
compiled in: the bundled ones under `internal/projection/manifests` are the floor, and
these are registered over the API, kept in the daemon's operational store, and come
back from a backup or by registering them again from here.

```sh
export ANAM_SERVER=http://192.168.20.4:8787 ANAM_TOKEN_FILE=~/.config/anam/token
anam projection register projections/heart.v5.json
anam projection register projections/sleep.v5.json
anam projection register projections/body.v2.json
anam projection register projections/activity.v2.json
anam projection register projections/recent-life.v6.json
anam projection list
```

The files here run from the first version of each to the newest. The earlier ones stay for the
receipts that name them. `heart.v1`, `sleep.v2` and `recent-life.v1` name the types as they were
before the domain taxonomy. `recent-life.v2` names notes `record.note`, `recent-life.v3` names the trip and
illness types as `place.departed` and `health.illness-started` and the like, and `recent-life.v4`
names `activity.exercise-session` as one type, before it was split into a leaf per vendor, so v5 names the
prefix `activity.exercise-session.`. `heart.v2` names `health.heart-rate-day`, three facts a day,
which became one fact a day, `health.heart-rate.day`, in v3, and `heart.v5` adds
`health.heart-rate.hour`, the watch's own readings, a minute a slot. `sleep.v4` and
`recent-life.v6` serve the same types as the version before and only reword the description.
`body.v2`, `activity.v2` and `sleep.v5` are the registered versions of three bundled names, and
each adds one type an importer registers, which a bundled template cannot name. `body` v1 is the
bundled floor (weight, body composition, blood pressure) and `body.v2` adds
`health.measurement.withings`, the Withings scale's own readings. `activity` v1 is the bundled floor
(every exercise session by the `activity.exercise-session.` prefix, steps, gym visits) and `activity.v2`
adds `activity.day.garmin`, Garmin's daily totals. `sleep.v5` adds
`health.cpap-session.dreamstation` to `health.sleep-episode`, since a reader asking about CPAP nights is
asking about sleep.
A registered version is immutable. The same file again is reported unchanged; a change
is a new version in a new file (`<name>.v<n>.json`), and every version ever registered
stays readable, because a receipt names the version that served it.

A `types` entry is a type or a type prefix. An entry ending in `.`, such as
`activity.exercise-session.`, admits every type under it, so a leaf added later is served without a new
version of the template. A prefix matches at a segment boundary only: `health.heart-rate.` admits
`health.heart-rate.day` and not `health.heart-rate-variability`, nor `health.heart-rate` itself. A prefix
needs at least a domain and a concept (`health.` alone is refused). A receipt over a prefix still
verifies after a new leaf is registered, since verify re-derives at the receipt's own W and a type
created later has no facets at that W.

Every exact type a template names must have a Definition in force on the daemon when it is
registered. `heart`, `sleep`, `body`, `activity` and `recent-life` name types the Garmin, OwnTracks and stays importers
register, so they register against the live daemon and not against a bare one.
