# Naur × Procida: the README, theory against need

Reviewed /home/matt/Projects/anam-card-73/README.md in the working tree, against the same tree at b253db7 (README and check.sh modified, CHANGELOG.md and scripts/check-links.sh untracked).
Every quick start command, from init to receipt verify and checkpoint list, ran against a scratch directory and printed what the README says.

## Findings

1. fix. Line 51. `med.started` is listed at version 4; its manifest says 5. Change it, and have `scripts/check.sh` compare the table to `internal/def/manifests/` as it now checks links; a hand-kept 35-row table has already drifted.

2. fix. Line 84. `location.stay` is attributed to owntracks; on anam-importers master it ships in `stays/definitions/`. Name `stays`.

3. consider. Lines 31 to 88. Two reference tables sit between the ideas and the first command, so a stranger scrolls 35 rows before proving the thing runs. Move them to `docs/DEFINITIONS.md`, or point at `GET /v1/definitions`, which is already the truth; keep the count and three example rows.

4. consider. Lines 106 to 131. "Letting an AI read it" precedes "Quick start", and its example exports `ANAM_TOKEN_FILE=~/.config/anam/token`, which the quick start never creates. The reader meets the author's deployment before their own first run. Move it below the quick start and say the token exists only when the daemon has `token_file`.

5. consider. Lines 153, 163, 171, 186. The quick start leans on W, the canonical, the fold and buckets before any is defined; "How it fits together" (line 194) comes later and names none of them. Those four words are the theory a reader needs to predict what the commands print. Put one short paragraph before the quick start (ledger, fold, canonical, W), or move "How it fits together" above it and add W.

6. note. Line 135. "Nothing here needs a server", then line 151 starts one. Say "another machine".

7. note. Line 224. "Every link above is checked by scripts/check.sh" holds in the working tree only; `check-links.sh` and `CHANGELOG.md` are untracked at b253db7. Commit them with the README.

8. consider. Lines 206 to 222. The index is a flat list with two "start here"s (this file and the PRFAQ) and two docs it omits (`docs/compelling-reason-for-Anam.md`, `docs/pipeline.html`). Group it by what the reader is doing: understand (PRFAQ, ARCHITECTURE, HISTORY, explainer), look up (API, GLOSSARY, CHANGELOG), do (IMPORTERS, INBOX, the anam-service skill). Mark MVP and SELF-LOG as records of a slice. Index or delete the orphans.

9. note. Line 130. "Reads are narrowed by role" overstates. Roles open routes; no role narrows which facets a read returns, and `docs/API.md` calls a narrowed reader later work. Say "routes are opened by role".

10. note. Lines 90 to 104. "Getting facts in" names six sources and links a repo but gives a reader with a running daemon nothing to run. Add the "run it by hand" line from the anam-importers README, or say feeding Anam is that repo's job and link its how-to.

Holding: the count of 27, every other table cell, the eight registered versions, the ANAM_SERVER refusal, the `serve` menu, the image caption, anam-mcp's four reading and two writing tools, every link.

## Where the two voices meet and part

They meet on the quick start. A stranger proves in two minutes that Anam runs on loopback against two files, and each printed line (W=0, a canonical hash, a receipt that verifies) is the theory showing itself. They agree on the tables: Procida sees reference in a tutorial's way, Naur sees a residue copied by hand that has drifted from the manifests (findings 1 and 2), and both want the machine, not the prose, to be the source.

They part on how much theory the README should carry. Naur wants it stated before the commands: a reader who does not know W or the fold can mimic the quick start but not predict it, and the "Ideas" list stops short of the four words the commands print. Procida would keep the README a tutorial plus an index and send the theory to the PRFAQ and ARCHITECTURE, so the page serves one need. The settlement is finding 5: one paragraph of theory where the tutorial first needs it, and an index shaped so the reader can tell which door is which.
