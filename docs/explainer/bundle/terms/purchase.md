---
type: term
title: Purchase (the money domain)
description: One order or receipt from a merchant, kept as a single fact with its lines and totals as the document states them; the first concept in the money domain.
timestamp: 2026-10-25T00:00:00Z
---

`money` was one of the four reserved domain heads, and Matt opened it on 2026-10-25 (card 241): "We need a new top level domain, money." That leaves ten heads, seven in use and `social`, `media` and `work` still reserved; see [where a distinction is encoded](/decisions/where-a-distinction-is-encoded.md). Purchases are its first concept, and a payment or a transaction would come later as a concept of its own.

`money.purchase` is a [Definition](/terms/facet-definition.md) bundled with the binary, written by hand into a records file, entered in the Workbench or saved by an agent. It is an event, Confidential and strict, in the family `purchases`.

One order or receipt is one [facet](/terms/facet.md), never one per line: the lines are `items` inside it, each with a `name` and, where the document gives them, a `sku`, `quantity`, `unit_price` and `line_total`. `subtotal`, `shipping`, `tax` with its `tax_label`, and `total` are numbers as the document prints them, in `currency`, the document's own ISO 4217 code. Nothing is converted: what the bank charged in another currency is a later fact, and so is a refund, each joined to the purchase by an edge.

`merchant` is one agreed spelling, lower case with words joined by hyphens, the name the person uses for the shop and not its legal entity, which `merchant_legal_entity` carries. It follows the rule a drug and an organisation follow, so the same merchant is the same string across every source.

The id is declared: `<source>:purchase/<merchant>/<order_ref>`, so `order_ref` is required. It is the merchant's reference, verbatim. When the merchant gives none, the writer uses the receipt number, else the date and one word, chosen once, because it is part of the identity. Leave `id` out and Anam fills it; send it and it must match. The same order written twice is one facet.

`date` is a local date, read in Australia/Sydney as the other date types are. It carries no `source_time`, since a date has no clock to read (card 198).

What the purchase was for, whether it is deductible and which financial year it falls in are the person's judgement about it, and they live in the note that joins the purchase, an edge in `record.edge`, never in this type. The invoice behind it is a `record.document` facet joined by the same kind of edge, not named in the payload. A schema that refuses fields it does not list keeps `deductible` out. Card 146 carries the claim as a concept of its own when it is built.

One bundled projection serves the type, `purchases`, a log up to Confidential. It is not part of `recent-life`.
