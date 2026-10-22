---
type: term
title: Attention (the mind domain)
description: What the person says is absorbing them, kept as a reading with a topic and a strength from 1 to 5, and as a declared start and end; the first concept in the mind domain.
timestamp: 2026-10-23T00:00:00Z
---

`mind` is the ninth domain head. It holds what a person says about their own inner state, so mood or energy can join later as concepts without another head. A head is added only by a logged decision, and this one was decided on 2026-10-22 (card 222); see [where a distinction is encoded](/decisions/where-a-distinction-is-encoded.md). Anam records what the person said and does not judge it. A model's guess at where attention went is a different source and sits beside these facts without editing them.

Attention is the first concept. Three types are built, each a [Definition](/terms/facet-definition.md) bundled with the binary.

- `mind.attention` is one reading. The person says, at a moment, what is holding them (`topic`), how strongly (`intensity`, a whole number from 1 to 5) and, if they like, a `note`. It is a reading in the way a blood pressure is, but the instrument is the person.
- `mind.attention.started` and `mind.attention.ended` are a pair the person declares, for how long something held them. Their [series](/terms/series.md) key is the topic, so an ended closes the latest started of that topic and a later started opens a new one. Readings never open or close an episode. Nothing here works episodes out from readings, so a gap between readings means nothing until the person says it.

The topic has one agreed spelling, lower case with words joined by hyphens (`deskcam`), the way a drug has one spelling. The store never reads inside a payload, so a topic can be found only by matching it exactly, and a second spelling would be a second topic. The Definition's pattern refuses spaces and capitals.

The moment is a wall clock the person said it at, so each type reads `at` with the offset that `source_time` carries, as `health.medication.dose` does. The id is the topic and the minute, then the offset: one minute said in two zones is two readings, and the same minute said twice is one. The classification is Confidential, as for the other things a person does with their time. The bundled `attention` projection is a log of the three types.

A fourth, `mind.attention.reflection`, is not built. It would hold something said later about a period, with the person's own verdict. It waits until a question needs it, as do a bounded session and a deliberate shift from one topic to another.

Hand entry: the Workbench draws a form for any type the importers file lists under its `workbench` principal, so the three types are entered by hand once an operator lists them. An agent saves one through anam-mcp's `save_fact`.
