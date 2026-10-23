---
type: term
title: Intake (caffeine, alcohol and notable meals)
description: What the person says they took in, kept as a fact at its minute so a later question can ask whether it came before a poor sleep or a reading; drinks with a count, meals in the person's words, and no nutrition.
timestamp: 2026-10-24T00:00:00Z
---

`health.intake` holds an account like "a coffee", "two glasses of wine" or "ate out, rich meal" as a fact at its time. The reason is a question asked later: did it come before a poor sleep, a heart-rate alert or a blood-pressure reading. Before this type such an account was a `record.edge` note, which no query can count or filter. Nothing here is nutrition. There are no calories, no portion weights and no food database, and Anam does not judge the intake. It is a [Definition](/terms/facet-definition.md) bundled with the binary, classed Sensitive like the other health types.

`kind` is `caffeine`, `alcohol` or `meal`. A drink says which one and how many. `drink` has one agreed spelling, lower case with words joined by hyphens (`flat-white`, `red-wine`), the way `topic` has on [attention](/terms/attention.md), because the store never reads inside a payload and a second spelling would be a second drink. `count` is a whole number from 1. `size`, `volume_ml` and, for alcohol, `abv` are added when the person knows them. Standard drinks or units are worked out on read from those, never stored. A meal says `text` in the person's words and two yes or no answers, `rich` and `out`. The schema is one object with an `if` and `then` for each kind: a caffeine or alcohol fact that lacks its drink or count is refused, a meal that lacks any of its three is refused, and a drink carrying a meal's field, or a meal carrying a drink's, is refused too. `place` is the place's agreed spelling when worth naming; joining an intake to a place fact is an edge's work.

The moment is a wall clock the person said it at, read at the offset `source_time` names, as for `health.medication.dose`. When the person says only a part of the day, `part_of_day` is morning, afternoon, evening or night and `at` is that part's nominal minute (08:00, 14:00, 19:00, 22:00), so the fact's clock is nominal and not a minute anyone knew. The id is the kind and the minute, then the offset. Two intakes of one kind in one minute are one fact, which is why `count` carries the number: a coffee tap that sends one coffee sends count 1 at the tap's minute. A different kind in the same minute is its own fact.

The type closes no series. A bundled `intake` projection serves it as a log, up to Sensitive. To enter it by hand, an operator lists `health.intake` under the `workbench` principal's types; an agent saves one through anam-mcp's `save_fact`.
