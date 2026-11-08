The daemon serves this directory at `/feed/` (card 276). What follows is the prototype's note from card 275.

# Card 275 timeline feed prototype

A browser prototype of the vertical timeline feed. The spec is `DESIGN.md`. No build, no network.

## Open it

Open `index.html` in Firefox or Chromium (a `file://` URL is fine). It loads `fixture.js` with a
script tag. The view state lives in the URL hash, so reload and the back button restore it.

## Use a real answer

Save a real `/v1/timeline` answer, then wrap it as the fixture:

```sh
curl -H "Authorization: Bearer $ANAM_TOKEN" \
  "$ANAM_SERVER/v1/timeline?from=2026-10-01&to=2026-11-07" > answer.json
{ printf 'window.ANAM_FIXTURE = '; cat answer.json; printf ';\n'; } > fixture.js
```

Keep the original `fixture.js` somewhere first. The page needs `{w, live, entries}`; entries come
oldest first. `window.ANAM_FIXTURE_TAIL` is optional, so a real answer simply has no "new since you
opened this" bar.

## What is fixture and what is real

- Real: the entry shape (`facet_id`, `type`, `fact_time`, `source`, `source_ref`, `status`,
  `classification`, `content_hash`, `payload`), the answer envelope (`w`, `live`), edges as
  `record.edge` entries, and the domain hues from the inspector's timeline template.
- Fixture: all of the data in `fixture.js` (seeded, about 2,300 entries over 14 months ending
  2026-11-07, zone +11:00), the superseded and retracted rows, the "3 new" tail, and the fake
  300 ms load with its deterministic one-in-twenty failure.
- Stand-in: the presentation table at the top of the script in `index.html`. It stands for card
  256's presentation value (domain, family, kind, label template) plus a span hint
  (`span: {start, end}` or `{start, minutes}`) that does not exist yet. A type missing from the
  table gets the generic rendering, as `vehicle.service` does.
- Not in the answer: the recorded time. The inspector says so.

## Notes on the address

`#from=&to=&type=&domain=&source=&topic=&status=&q=&layout=&density=&sel=&open=`. Values may be
comma lists. `from` and `to` are a date or a local `YYYY-MM-DDTHH:MM`. A superseded row is
selected as `sel=<facet_id>@<hash8>`. History scroll position and the number of loaded 14-day blocks
are kept in `history.state`, not in the address. Typing in search, the density toggle and the
domain tree use `replaceState`; everything else pushes.
