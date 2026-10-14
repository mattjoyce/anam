# 12 October: the phone's hours, a page the size of the screen, and a read that names its fields

Three threads ran through the day, and each was about size in a different way. The phone's
positions arrived as a new source, and most of the work was finding out how much of what OwnTracks
sends is actually new. The inspector's landing page had grown to 10.9 MB with the record, and it
came down to about 100 KB by drawing for the screen rather than the ledger. And a model asking about
rowing was paying for every stroke of every session, so a read can now name the fields it wants and
get back a frame. By midnight all three were merged, card 58 had moved the one-off extractors out
of Anam, and the gate had caught a bug that review had not.

## The phone's positions

Card 130 took eleven amendments in one day. The route it first assumed did not exist, ductile's webhooks accept only HMAC, and a header probe
of OwnTracks on Android settled the rest. The app sends Basic auth on its first request and never
answers a challenge, it has no setting for a custom header, and it reports one fix several times.
Anything that needs a bearer token or a signature cannot take the phone directly.

So the phone posts to a small Worker at the edge with a database behind it, the unraid gateway's
ductile pulls from it every 15 minutes into a spool, and `anam_owntracks` in anam-importers reads
the spool. The importer sends one `location.track` facet per phone per UTC hour of fix time. Two
decisions in it come straight from the data. Messages that share a `tst` are one point, carrying
every trigger that reported it, because the first live day sent 45 messages over 11 hours and they
held two distinct fixes. And an hour that gains a message is sent whole, since a facet replaces its
hour, and a part sent alone would replace the whole with less. No coordinate is ever logged.

Card 133 drew it. A track hour sits in the travel row as a measure, and the facet page draws it as a
plot in metres about the track's centre, each fix ringed by its accuracy, with a scale bar and no
map. That last part was deliberate. A basemap is fetched as tiles, and the tiles a page asks for are
the area it shows, sent to whoever serves them. A pipeline that never logs a coordinate would leak
the same thing through the browser the first time a map appeared. Whether to have one, and whose
tiles, is Matt's call, not a rendering detail.

## A page the size of the screen

Card 135 came from ThinkPad.unraid's handoff, in Matt's order. The landing page drew one SVG mark
per facet over all time, and on every request it fetched every dead facet's old version before
filtering. At 14,000 facets that was 10.9 MB live, and two years of a daily measure landed in the
same few pixels anyway.

The fix ties detail to pixels, not zoom levels. Per lane, three or more marks within three viewBox
units of each other draw as a run, its two ends joined by a line with a count, linking to the run's
own span.
The server does the grouping, so the page is bounded by lanes times pixels whatever the record
holds.

Measuring after the first cut found the next cost hiding behind it. With the band bounded, the page
was still 1.8 MB, and the canonical strip below it, one square per live facet, was the other 1.7 MB.
Past 1,500 facets the strip now tallies by type. On the ThinkPad's copy of the ledger "/" went from
1.84 MB to 100 KB.

Live became the default. No status means live, and only `status=all` or `dead` fetches the dead, in
one batch. The legend still says "N dead hidden", from the fold's counts alone. That reversed a
sentence in the design, "the band is the ledger surface", and the request cost is what decided it,
not the sentence ([[ledger-and-canonical-two-surfaces]]). A "none" preset joined "all types", and
since a view is a URL, "make this my default" keeps the query in a cookie that a bare `/` opens.

## A read names its fields

Card 138 started from the MCP adapter. A question like "what was my best rowing week" needs three
numbers per session, and they travel inside a 72 KB payload with every stroke. A month of
`exercise.session` was 184 KB whole and 4.6 KB cut to distance, duration and erg type.

Matt decided the doctrine change in the evening, and its name changed twice before any code. It
began as a mask, then a masked payload that keeps its own shape, then neither. A mask names what
should not show, and the API document already used "element masking" for redaction, so one word
would have carried both signs. The parameter is `fields`, JSON pointers into the type's schema, a
temporary subset of it. The answer is a frame, columns and rows in place of entries. The columns
are the envelope's attributes and then the pointers, nothing nested, null where a row lacks a
field, so it reads straight into a DataFrame
([[a-read-names-its-fields-and-gets-back-a-frame]]).

Fields only take away. They are cut after the projection and the narrowing, from facets those
admitted, so they can neither bring back a facet nor exclude one, and the rule that a projection
prunes and a consumer selects still holds. A pointer is checked against the schema of every type
the read could serve, never against the data, so the same query is refused the same way on an
empty record and a full one. The stored payload stays opaque, with nothing indexed by field. A
receipt pins the fields beside the narrowing, verify re-cuts, and a receipt without fields keeps
its bytes ([[published-is-a-receipt-not-a-record-event]]). `anam timeline --field` prints the frame
as a table, and anam-mcp's `read_record` takes fields that the model copies from `describe_type`.

## What the gate caught

The publish path's bound check sat in an if-else chain whose header declared `err` with `:=`. The
later branches assigned to that shadow, and the check after the chain read the outer `err`, always
nil. A publish could never be refused for size. The diff read cleanly. The existing test that a
refused publish files nothing failed on the first gate run, and the fix moved the declaration onto
its own line. It is the argument for a test per promise rather than per function.

## Card 58, the extractors leave

The one-off extractors, the ACL pathology parser, Fitbit weight and sleep, Strong, Technogym and
the BP diary, moved to `records/extract` in anam-importers. Matt narrowed the card first. Technogym
biometrics were dropped, because the rows Matt wanted were the sessions, live since card 56. Weight
in Open mHealth shape split off as card 137. The merge kept both sides of conflicts with cards 122
and 130, and 137 tests passed against Anam. Fitbit sleep stopped on a question for core.
`sleep.episode` is registered by the Garmin importer, not bundled, and a second importer may not
send a type another one owns. Either core bundles it, as it did `exercise.session`, or the rule
bends.

## Hickey × Armstrong

**Hickey.** Two of the day's fixes unbraided a surface from its cost. The timeline had tied what it
draws to what the record holds, and the read had tied what a reader wants to what the facet
carries. Both now take their size from the reader, the screen in one case and the named fields in
the other, and neither touches what is stored.

**Armstrong.** The location pipeline has three stores of the same points, the edge database, the
spool and the ledger, and the spool is the one a rebuild would start from. Until it has a backup it
is the weak link, which is why card 134 exists.

## Left open

- Card 134: back up the OwnTracks spool off the box, prove a rebuild from it, then prune the edge
  database.
- Fitbit sleep waits on whether `sleep.episode` is bundled.
- Card 137: weight in Open mHealth shape, Matt to decide.
- Card 136, cut in passing: a note's fact time is when it was written.
- Cards 133, 135 and 138 were merged and awaited deploy at the end of the day.
