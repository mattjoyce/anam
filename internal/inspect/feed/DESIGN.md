# Card 275: a vertical timeline feed of the record, prototype

A browser view of the Anam record, newest first, scrolling down into history. One HTML file,
no dependencies, no build, no network. It is the prototype the card asks for: the thing to
review, not the implementation. The decisions and the findings it rests on are on card 275.

## What it is for

Seven journeys from the card. The review walks each one:

1. Open at recent activity and scroll into older history with reliable date orientation.
2. Jump to a month or date and return to recent activity.
3. Narrow by domain, type, source, or topic using the sidebar and chips; understand and clear
   the active scope.
4. Inspect an item and follow linked evidence, then return to the same place.
5. Expand a dense group and understand an overlapping period without losing events or context.
6. Save or share a view and restore it through reload and browser history.
7. Use the core browsing flow by keyboard and on a narrow screen.

## Files

- `index.html`: the whole prototype. Inline CSS and JS, ES2020, no modules, no frameworks,
  no web fonts, no fetch to anywhere. Opens from `file://`.
- `fixture.js`: a deterministic generator (seeded PRNG) that returns `{w, live, entries:[...]}`
  in exactly the shape of a `GET /v1/timeline` answer. `index.html` loads it with a script tag.
  Keep it separate so a saved real answer can replace it.
- `README.md`: how to open it, how to import a real answer, what is fixture and what is real.

## Data shape (the real one, do not invent fields)

An entry, from `internal/fold/value.go`:

```json
{"facet_id":"f_…","type":"health.weight","fact_time":"2026-11-06T07:12:00+11:00",
 "source":"withings","source_ref":"…","status":"live","classification":"Sensitive",
 "content_hash":"sha256:…","payload":{...}}
```

`status` is `live`, `superseded` or `retracted` (an import may carry only live rows; the
fixture carries all three). The answer carries `w` (an integer watermark) and `live` (bool).
Entries come oldest first; the view sorts `fact_time` desc, tie `facet_id` asc.

Presentation on a type comes from a small table in `index.html` that stands in for what the
daemon will one day serve (card 256's presentation value plus a span hint that does not exist
yet). Per type: `domain` (the head before the first dot, always derivable), `family`,
`kind` (`measure` | `reading` | `event`), `label` (a template `{/field}` over the payload, the
same grammar as `handling.label`), and optionally `span: {start:"/a", end:"/b"}` or
`span: {start:"/a", minutes:"/m"}` for periods. A type absent from the table gets the generic
rendering: domain from the head, no family, kind unknown, label = type plus the first two
scalar payload fields. The unfamiliar-type scenario must look acceptable that way.

Domain hues, from `internal/inspect/templates/timeline.html` (light / dark):
record `#2a78d6`/`#3987e5`, place `#eb6834`/`#d95926`, money `#1baf7a`/`#199e70`,
activity `#eda100`/`#c98500`, org `#008300`, mind `#4a3aa7`/`#9085e9`, health `#e34948`/`#e66767`,
other `#898781`. Colour never carries identity alone: every chip and lane has a text label.

## The fixture

About 2,000 entries over 14 months, ending "today" (the generator takes a fixed end date so the
page is stable). Types and payloads, with realistic shapes:

- `health.weight` `{kg, source_time}` most mornings. `health.blood-pressure` `{systolic,
  diastolic, pulse}` some mornings. `health.heart-rate` `{bpm}` every 5–15 min on three busy days
  (the repetition scenario; hundreds in a day).
- `health.sleep` `{period_start, period_end, minutes}` nightly, crossing midnight.
- `health.medication.dose` `{drug, amount, unit}` twice daily for one drug; one
  `health.medication.started` and one `health.medication.stopped` for a second drug months apart.
- `activity.gym-visit` `{venue, minutes}` three times a week; `activity.steps` `{count}` daily.
- `place.stay` `{place, arrived, departed}` a few a week, some crossing midnight; one
  `place.trip.departed` / `place.trip.returned` pair spanning five days.
- `mind.attention.started` / `mind.attention.ended` `{topic}` pairs: one topic ("the boat")
  spanning four months with a dozen starts and ends, the project thread.
- `money.purchase` `{merchant, total, currency}` a few a week; several tagged to "the boat".
- `record.note` `{text}` a few a week. `record.edge` `{kind:"about", from, to, text}` linking
  notes to purchases, gym visits and attention spans (the linked-evidence scenario). Edges
  render as chips on both ends, never as their own cards.
- `record.document` `{title, mime}` occasionally.
- One correction: a `health.weight` whose first version is `superseded` and second `live` with
  the same `facet_id` and a different `content_hash` (the import shape shows versions as
  separate rows only in `status=all` reads; the fixture does the same, two rows, one id).
- One retracted purchase.
- `vehicle.service` `{odometer_km, workshop, items}`: the unfamiliar type, absent from the
  presentation table.
- A quiet day (nothing but sleep and weight) and a fully empty week in the middle of the range.

## Layout

Three columns on a wide screen; under 800 px one stream with a filter drawer (button, top
left) and a detail sheet (bottom, full width).

**Top bar, sticky.** Search box. The window ("Nov 2026, last 14 days" or the explicit range).
The read basis: `live · w 82031` or `fixed at w 82031`. Active filter chips with × on each and a
"clear all". Density toggle (comfortable / compact). Layout toggle (stream / lanes). "Back to
recent" when the window is not the most recent. The result count ("312 of 2,014 in window").

**Left sidebar.** Saved views (three fixed ones: "Everything", "Health this week", "The boat"
= topic filter; the current view can be saved under a name into localStorage). The domain >
family > type tree with counts for the current window, collapsible; click a type to filter,
click a domain to filter by domain. Sources with counts. Status (live / superseded /
retracted, live only by default). A month navigator: the 14 months as a list with a count bar
each; click jumps the window to that month.

**Centre, the feed.** Day headings are sticky (`position: sticky`) and show the date, weekday,
and the day's count. Each card: time (HH:MM, zone from the fact_time offset), the label, then
chips: domain (coloured), type, source, status if not live, "n links" if edged. A period card
(span) shows start → end and a thin duration bar whose length is proportional to the span,
capped; a span crossing midnight appears under the day it started with "→ next day". Kind is
marked by form: a measure has a filled dot, a reading a hollow dot, an event a square, a period
a bar, in the card's left gutter, in the domain hue.

A quiet gap: when consecutive cards are more than 6 hours apart (within a day) or a day has no
entries, a thin row says "9 h quiet" or "no entries for 6 days". Scrolling order never implies
even spacing.

Grouping: a run of the same type with consecutive neighbours within 30 minutes and at least 5
members becomes one group row: "42 × heart-rate · 06:00–09:15 · 58–131 bpm" with an expand
control that shows the members in place and a collapse to return. Groups keep their place
in the day. The rule is written in the row's title attribute.

Lanes layout: the domains with entries in the window become side-by-side columns under the
same sticky day headings, each with a lane header; a domain's column collapses to a count strip
with one click. Keep one scroll container; never one per lane.

Load more: the feed first shows the most recent 14 days of the window; "Load 14 more days" at
the foot appends the previous window from the same data; a loading state (a 300 ms fake wait)
and a retry button (one in twenty fake loads fails, deterministically by window index, to show
the state). New entries: a "3 new since you opened this" bar at the top in live mode, appears
once after 20 s from a fixture tail, and only on click are they inserted; the scroll position
does not move.

**Right inspector.** Opens on card click or enter. Shows: label large; type and domain; fact
time in full with offset and the zone it implies; recorded time is not in the answer, so say
"recorded: not in this read"; source and source_ref; status and, for a superseded row, "a
newer version is live" with a link to it; the payload as a two-column table of its fields, then
raw JSON in a details element; facet_id and content_hash with a copy button each; "Related"
listing every edge touching this facet with its kind, text, and the other end as a link that
selects it. "Show around this time" sets the window to ±3 hours of the item and keeps the
current filters, with a "back to previous view" link. The inspector's back and close restore the
scroll position and focus to the card.

## State and the address

Everything that defines the view lives in the URL hash as query parameters:
`#from=…&to=…&type=…&domain=…&source=…&topic=…&status=…&q=…&layout=stream|lanes&density=…&sel=<facet_id>&open=<domain,…>`.
Every change pushes history (replaceState for typing in search, pushState for the rest). Back
and forward restore the view including the selected item and the scroll position (keep a
scroll position per history entry in `history.state`). Reload restores. Deep link with `sel`
scrolls the item into view, selects it, and opens the inspector.

## Keyboard and accessibility

j / k move the selection down and up, enter opens the inspector, esc closes it, / focuses
search, g then r goes to recent, `[` and `]` move a window. Visible focus ring on everything.
Chips are buttons. Contrast at least 4.5:1 on text. `prefers-reduced-motion` turns off
transitions. `prefers-color-scheme: dark` is supported with the dark hues above. Nothing is
hover-only.

## Look

Warm paper, restrained surfaces, subtle separators: background `#faf9f5`, surface `#fffefa`,
text `#242c28`, muted `#616b64`, accent `#27644e`; dark: background `#1b211e`, surface
`#222925`, text `#e7eadf`, muted `#a8b5aa`, accent `#9ac9ae`. Headings in a serif stack
(`Iowan Old Style, Palatino Linotype, Georgia, serif`), body in a sans stack
(`system-ui, -apple-system, Segoe UI, Roboto, sans-serif`), identifiers in monospace. Chips are
small, outlined, with the domain hue as a left bar or dot plus text. Generous spacing in
comfortable density; compact halves the vertical padding and hides the second chip row.

## Not in the prototype

Writes of any kind. Summaries and callouts beyond the quiet-gap row. Real fetches. A time rail
(the month navigator stands in for it; the review decides).

## Done when

- Each of the seven journeys can be walked from the fixture.
- `index.html` has no external references; opens from `file://` in Firefox and Chromium.
- The fixture is deterministic: two loads give the same page.
- README says how to export a real answer with curl and the token and drop it in as `fixture.js`.
