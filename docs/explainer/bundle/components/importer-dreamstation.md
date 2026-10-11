---
type: component
title: anam_dreamstation
description: Decodes a CPAP SD card copy with the dreamstation package and sends every session as sleep.cpap-session, a type the binary does not bundle, so it registers its own Definition.
timestamp: 2026-10-11T00:00:00Z
resource: ../anam-importers/dreamstation_card/run.py#batch_events
anchor: "def batch_events(request: Dict[str, Any]) -> Dict[str, Any]:"
basis: code
tier: detail
group: importers
depends_on:
  - /components/importer-base.md
---

Every run sends the whole card, old sessions come back no-op. The run is labelled a snapshot only when the decoder skipped no file; an events file whose summary is not on the card belongs to a session the run cannot send, so that run carries no label.
