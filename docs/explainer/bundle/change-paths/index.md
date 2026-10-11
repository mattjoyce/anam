# Change paths

* [Add a read command that works locally and over the daemon](add-a-cli-command-with-a-daemon-route.md) - A new read lives in internal/read against a read.Source, gets a daemon route on the read-only handles, a client method, and a cobra command that routes on cfg.Server.
* [Add a source as an importer](add-an-importer.md) - A new vendor source is a folder in anam-importers with a manifest, a run.py that shapes payloads, and the Definitions it emits, plus a token line in the daemon's importers file.
* [Add or change a projection template](add-a-projection-template.md) - A template is one embedded JSON file; a changed prune is a new version, because receipts pin the version and verify refuses a moved one.
* [Change a Definition's shape with a new version](add-a-definition-version.md) - A new version may change the schema, classification or series but never identity_key or fact_time; bundled types change in the binary, registered ones in the importer's folder.
* [Change the bucket width or the checkpoint budget](change-bucket-width-or-budget.md) - The budget can change at any time and the next mint settles it; the width is pinned into the index, so changing it is a rebuild of tracking.db and a full mint.
