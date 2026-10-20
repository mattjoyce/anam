# IEEE 1752.1 schemas, pinned

Copied unchanged from IEEE's `omh/1752` repository (https://opensource.ieee.org/omh/1752),
`schemas/physical_activity/` and `schemas/utility/`, at commit 7000803a101d (2026-08-03).
Each file keeps its own `$id` under `https://w3id.org/ieee/ieee-1752-schema/`; their `$ref`s are
relative to that base, and every file they reach is here, so nothing is fetched.

`physical-activity-1.0.json` is what the four leaves of `activity.exercise-session` (`.garmin`,
`.concept2`, `.technogym` and `.strong`, each with `source_schema` `ieee-1752/physical-activity-1.0`)
conform to. `internal/def/conformance_test.go` holds each leaf's examples to it.
