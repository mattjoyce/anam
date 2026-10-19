# Registered projection templates

The templates the operator registers with the live daemon (card 125). They are not
compiled in: the bundled ones under `internal/projection/manifests` are the floor, and
these are registered over the API, kept in the daemon's operational store, and come
back from a backup or by registering them again from here.

```sh
export ANAM_SERVER=http://192.168.20.4:8787 ANAM_TOKEN_FILE=~/.config/anam/token
anam projection register projections/heart.v1.json
anam projection register projections/sleep.v2.json
anam projection register projections/recent-life.v1.json
anam projection list
```

A registered version is immutable. The same file again is reported unchanged; a change
is a new version in a new file (`<name>.v<n>.json`), and every version ever registered
stays readable, because a receipt names the version that served it.

Every type a template names must have a Definition in force on the daemon when it is
registered. `heart`, `sleep` v2 and `recent-life` name types the Garmin and OwnTracks importers
register, so they register against the live daemon and not against a bare one.
