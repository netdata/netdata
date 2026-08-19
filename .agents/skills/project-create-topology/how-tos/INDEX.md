# project-create-topology how-tos

This directory contains developer-facing recipes for validating and maintaining
`netdata.topology.v1` producers, fixtures, schemas, and handoff artifacts.

These are not operator workflows. Public/operator recipes for fetching Agent or
Cloud data live under `docs/netdata-ai/skills/`.

## Index

- [add-graph-presentation.md](./add-graph-presentation.md) -- add backend-controlled graph presentation, safe labels, port-bullet sources, legends, highlight paths, and validation to a `netdata.topology.v1` producer.
- [define-per-actor-highlight-paths.md](./define-per-actor-highlight-paths.md) -- encode per-actor ordered highlight paths without collapsing the clicked actor and path member into one column.
- [preserve-semantic-link-types.md](./preserve-semantic-link-types.md) -- keep graph link types distinct when protocols, confidence, or inferred state need different visual treatment.
- [verify-network-connections-layout-tokens.md](./verify-network-connections-layout-tokens.md) -- verify local `topology:network-connections` v1 link layout tokens and correlation rule wiring through a token-safe direct-agent call.
- `migrate-network-connections.md` (stub -- not yet authored)
- `migrate-streaming-topology.md` (stub -- not yet authored)
- `migrate-snmp-l2-topology.md` (stub -- not yet authored)
- `create-vsphere-topology.md` (stub -- coordinate before authoring in the separate worktree)
