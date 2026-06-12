# SOW Specs Index

Specs are durable project memory: current contracts, cross-cutting behavioral
rules, and area-specific guarantees extracted from completed work.

Keep this directory flat until scale proves hierarchy is needed. Add one row to
this index in the same change that adds, renames, or removes a spec.

## Specs

| Spec | Scope | Main consumers |
|---|---|---|
| [go-v2-host-scope.md](go-v2-host-scope.md) | Go framework V2 host-scope and vnode metric routing contract. | Go V2 collector work, `metrix`, `jobruntime`, `chartengine`, go.d collector docs and skills. |
| [prometheus-profiles.md](prometheus-profiles.md) | go.d prometheus chart-profile format, selection modes, app-segment resolution, and chart-context assembly. | prometheus collector profile work, chart-context/UI app-section contract, profile authoring. |
| [query-planner-tier-selection.md](query-planner-tier-selection.md) | Automatic storage-tier selection for metric query planning. | Query planner code, API data paths, MCP metric query behavior. |
| [sensitive-data-discipline.md](sensitive-data-discipline.md) | Sensitive-data rules for committed artifacts, scripts, skills, specs, SOWs, commit messages, and PR text. | All repository work, SOW audits, private/public skills, token-safe scripts. |
| [snmp-profile-projection.md](snmp-profile-projection.md) | SNMP profile projection contract across metrics, topology, licensing, and BGP consumers. | SNMP profile authoring, ddsnmp loader/projection code, SNMP project skill. |
| [snmp-traps/trap-metrics-profiles.md](snmp-traps/trap-metrics-profiles.md) | SNMP trap profile-local metric extraction use cases, identity, cardinality, merge, loader, generator, and compatibility design. | SNMP trap metric profile implementation, profile authoring, generator work, chart-template integration, trap docs and skills. |
| [taxonomy.md](taxonomy.md) | Collector chart taxonomy source, validation, and generated artifact contract. | Collector taxonomy files, integrations generators/checkers, Cloud dashboard taxonomy consumption. |
| [topology-function-schema.md](topology-function-schema.md) | Production `netdata.topology.v1` Function payload, presentation, modal, and producer contract. | Topology producers, Function schemas, Cloud aggregation, UI rendering, topology project skill. |
| [topology-modes-correlation-aggregation.md](topology-modes-correlation-aggregation.md) | Topology detailed/aggregated modes, correlation, aggregation, and actor identification contract. | Topology producers, Cloud aggregator, UI modal/table behavior. |
| [vsphere-parity-matrix.md](vsphere-parity-matrix.md) | vSphere collector parity classification and implementation policy. | vSphere collector work, Go V2 migration decisions, reviewer parity checks. |
| [vsphere-v1-compatibility-manifest.md](vsphere-v1-compatibility-manifest.md) | Historical vSphere V1 compatibility baseline retained for migration traceability. | vSphere collector compatibility review and historical reference. |

## Adding A Spec

- Use a flat path: `.agents/sow/specs/<domain>-<topic>.md`.
- Capture current reality, not aspiration.
- Prefer one durable contract or cross-cutting rule per file.
- Do not restate implementation details that are already clear from code.
- Do not split specs by repository path; choose the contract audience.
- Update this index in the same change.
- Run `.agents/sow/audit.sh` before commit.

## Reconsidering Hierarchy

Do not add subdirectories only for aesthetics. Revisit hierarchy when the specs
set grows beyond roughly 25 files, or when one domain accumulates roughly 6 or
more specs. If hierarchy becomes necessary, use domain/contract ownership rather
than repository layout.
