---
type: term
title: Envelope
description: The one message a capture worker or importer hands the gate; the only way anything enters the ledger.
timestamp: 2026-10-11T00:00:00Z
---

The Envelope is the message a source hands the [admission gate](/components/gate.md): a type, a definition version, a payload, and optionally a `source_ref`, an asserted person and a span hint. Over the daemon the source is not a field; the importer's token names it.

It is the only ingress. Capture workers and importers share nothing with the ledger except through it, which is what lets them be disposable and written in any language. Its wire shape is the [batch submission](/data-structures/batch-submission.md); the Go struct is the [envelope](/data-structures/envelope.md).
