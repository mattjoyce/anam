# Ousterhout × Liskov: the README as Anam's interface

Reviewed `README.md` in `/home/matt/Projects/anam-card-73` as the module's surface to a stranger, against the code in the same tree.
Commit `b253db7`, with the uncommitted README, `CHANGELOG.md` and `scripts/check-links.sh` changes in the working tree.

## Findings

1. **Fix.** Facet table, `med.started` row. The README says version 4; `internal/def/manifests/med.started.json` says `definition_version: 5`. The table promises a contract the daemon no longer admits against. Correct the row, and add a check (the manifests are JSON, the table is text) so the gate catches the next drift the way `check-links.sh` catches a dead link.

2. **Fix.** Quick start, `./anam serve bp`. Alone among the quick start's commands, `serve` never routes to the daemon: `serveCmd` calls `loadConfig`, which refuses when `ANAM_SERVER` is set, and otherwise opens `tracking.db` and the artifact directory locally while the daemon the reader was told to leave running owns them (`daemon` help: "it is the only process writing them"). A reader who exported `ANAM_SERVER` as the earlier block said gets a refusal; one who did not runs a second writer beside the daemon. Either show the routed form (`anam timeline --projection bp --publish`, which the `timeline` flags offer) or tell the reader to stop the daemon first.

3. **Consider.** Quick start comment, "you select a projection, you cannot define one". `anam projection register <manifest.json>` exists and the `serve` help says templates are "registered by the operator". The person running the quick start is the operator. Say "a reader cannot define one; the operator registers one with `anam projection register`", and name `projection list`, which the README never mentions.

4. **Consider.** "Letting an AI read it", last paragraph: "Reads are narrowed by role." Every `GET /v1/*` route admits `anyRole`; the one narrowing is `getSnapshot`, which hides another source's snapshot from an importer. An importer token reads the whole timeline. Roles gate writes, not reads. Say that, or the stranger builds an access model the daemon does not keep.

5. **Consider.** "Registered by importers" table. The eight versions are a promise about another repository, frozen in this one; nothing here checks them. Drop the version column for that table and point at `GET /v1/definitions`, which the README already names as the daemon's own answer.

6. **Note.** Quick start opener: "Nothing here needs a server" then "Run the daemon." The daemon is the server. "Needs no remote machine or token" is what is meant, and what the code enforces (`loopbackOnly`, tokenless operator on loopback, both verified).

7. **Note.** Docs section: "Every link above is checked." True for existence (`check-links.sh` tests `-e` on each relative target in README, AGENTS and `docs/*.md`); the description beside each link is not. Say "every path above exists at the last merge".

8. **Note.** The surface is deep where it counts: one paragraph and six bullets carry the model, and the quick start exercises every pillar in twenty lines. Verified: 27 bundled manifests and every other row's type, key and fact-time pointer; `/healthz`; `init` and `daemon` refusing under a configured server; default paths `~/.local/share/anam/{ledger,tracking}.db`; `person_id` without default; `make build` producing `./anam`; `go 1.25.0`; every flag used; `receipt list|show|verify`; `checkpoint list`; `bp` bundled in `internal/projection/manifests`.

## Where the two voices meet and part

Both read the README as a contract, not a brochure. A version in a table, a comment in a shell block, "narrowed by role": each is a postcondition the reader will build on, and findings 1 to 4 are where the text promises what the code does not keep. Ousterhout likes that the document hides a great deal behind a little; Liskov insists that what it exposes must hold on substitution, and the quick start is that test: a reader whose shell differs from the author's only in whether `ANAM_SERVER` is exported.

They part on the remedy. Ousterhout would shrink the surface: cut the importer versions and the "cannot define one" aside, and let `GET /v1/definitions` and `--help` carry what changes. Liskov would keep the sentences and make them true and checked, as the link gate now does for paths and finding 1 asks for the table. Both are right for different rows. Keep and check what this repository owns; drop what it does not.
