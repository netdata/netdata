<!-- markdownlint-disable MD013 MD043 -->

# Python GC Prometheus operator model

## Responsibility

This document owns the human semantic decisions for the shared Python garbage-collector surface. The external inventory
owns exact family and selector dispositions; `proof.yaml` and replay tests own executable behavior.

## Entity and signal model

- The collector job is the process identity; these metrics do not expose a safer subordinate process key.
- Collected and uncollectable object counters are rendered as object rates.
- Collection-cycle counters are rendered as collection rates.
- `generation` is a bounded dimension within each service-level chart, not a separate entity identity.

## Composition boundary

The profile deliberately has no `app`. An automatically selected application profile supplies the resolved application
namespace, so the same Python GC views can compose without receiving a generic plugin-wide context. A standalone job may
still set `app` when no application profile is present. Its charts retain the shared `Process Runtime` family and
`process_runtime` context namespace used before the selection split.
