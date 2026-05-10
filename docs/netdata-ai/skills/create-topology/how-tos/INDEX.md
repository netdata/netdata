# create-topology -- How-tos index

This directory holds operational how-tos for creating and migrating Netdata
topology producers.

## The "if you analyze, you author a how-to" rule

Every time an AI assistant answers a concrete topology-authoring question that
requires multiple code reads, schema checks, payload-size experiments, or
cross-referencing more than one topology guide, and the workflow is not already
documented here, it must add a focused how-to before completing the task.

## How-to template

Filename: `<slug>.md`.

Sections:

1. **Question** -- the user-visible task.
2. **Inputs** -- producer, topology type, fixtures, expected scale.
3. **Schema choices** -- actors, links, evidence, tables, overlays.
4. **Implementation steps** -- concrete files and checks.
5. **Validation** -- schema validation, semantic tests, size checks.
6. **Gotchas** -- direction, aggregation, rollout compatibility, sensitive data.

## Index

- `add-graph-presentation.md` -- add backend-controlled graph presentation,
  safe labels, port-bullet sources, legends, highlight paths, and validation to
  a `netdata.topology.v1` producer.
- `define-per-actor-highlight-paths.md` -- encode per-actor ordered highlight
  paths without collapsing the clicked actor and path member into one column.
- `preserve-semantic-link-types.md` -- keep graph link types distinct when
  protocols, confidence, or inferred state need different visual treatment.
- `migrate-network-connections.md` (stub -- not yet authored)
- `migrate-streaming-topology.md` (stub -- not yet authored)
- `migrate-snmp-l2-topology.md` (stub -- not yet authored)
- `create-vsphere-topology.md` (stub -- coordinate before authoring in the separate worktree)
