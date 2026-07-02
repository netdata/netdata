# SOW Specs Index

Specs are durable project memory: current contracts, cross-cutting behavioral
rules, and area-specific guarantees extracted from completed work.

Keep this directory flat until scale proves hierarchy is needed. Add one row to
this index in the same change that adds, renames, or removes a spec.

Exception: `.agents/sow/specs/snmp-traps/` is a domain hierarchy with its own
README. Netdata-owned SNMP trap specs live at the top of that directory, while
research evidence lives under `.agents/sow/specs/snmp-traps/research/` so
research cannot be mistaken for product specification.

## Specs

| Spec | Scope | Main consumers |
|---|---|---|
| [go-v2-host-scope.md](go-v2-host-scope.md) | Go framework V2 host-scope and vnode metric routing contract. | Go V2 collector work, `metrix`, `jobruntime`, `chartengine`, go.d collector docs and skills. |
| [journal-log-writer-directory-contract.md](journal-log-writer-directory-contract.md) | `Log::new` requires an absolute journal directory (rejected early otherwise); consumers must resolve relative config against a stable base. | journal-log-writer consumers: otel-plugin logs, netflow-plugin tiers. |
| [mcp-build-run.md](mcp-build-run.md) | MCP build/run tool contracts: two-profile model, single build dir per worktree, dedicated-tree ownership, install-path/lock conventions, job/run lifecycle. | `packaging/tools/automation/mcp/` server work. |
| [netflow-tier-commit-workers.md](netflow-tier-commit-workers.md) | NetFlow rollup-tier commit worker contract: ownership, doorbell protocol, shutdown order, stretch semantics, lock discipline, telemetry. | netflow-plugin ingest/tiering work, tier benchmark and soak/crash tests, network-flows operator docs. |
| [otel-legacy-logs-viewer.md](otel-legacy-logs-viewer.md) | Read-only `legacy-otel-logs` viewer for former-otel-plugin journal logs, plus the obsolete-plugin deny for upgrade safety. | otel-plugin supervisor/worker work, `otel-legacy-logs`, `journal-function`, `plugins.d` obsolete-plugin handling. |
| [otel-logs-ng-flatten-format.md](otel-logs-ng-flatten-format.md) | OTel logs ng-flatten WAL frame + SFST v9 typed descriptor: bincode `FlattenedRequest` payload, the `attributes.*`/`resource.attributes.*`/`scope.*` field namespace (collapsed arrays, per-row id columns), ingest normalization, content_meta identity authority, and the structural seal/tail render-parity guarantee. | ng-flatten, ng-index, sfst index build, sfsq, otel-ingestor logs ingest, otel-ledger seal/query, operator field-namespace docs. |
| [otel-offline-wal-sfst-query.md](otel-offline-wal-sfst-query.md) | Offline WAL/SFST OTel-log query contract (`sfsq-cli` + `otel-plugin logs`): one `sfsq_cli` lib shared by two front doors, directory resolution, SFST/WAL discovery, SFST-wins dedup, stream filter, time window, NDJSON output, and the clap-dispatch/tracing discipline for the shipped subcommand. | `sfsq-cli` work, `otel-plugin logs` subcommand, offline forensic log inspection, `sfsq` engine consumers. |
| [otel-remote-storage-config.md](otel-remote-storage-config.md) | OTel plugin remote object-storage config + credential contract: single OpenDAL `uri` (non-secret options only), credentials delegated to OpenDAL ambient loading (never in `otel.yaml`), backends added via cargo features, and the non-blocking startup reachability probe. | otel-ledger storage/uploader, otel-plugin + bridge storage config, `otel.yaml`, operator remote-storage setup. |
| [otel-storage-substrate.md](otel-storage-substrate.md) | Content-agnostic `file-lifecycle` crate contract: substrate vs logs-binding split, the hard dependency boundary (no sfsq/otel-logs-identity, enforced by `tests/dep_guard.rs`), and the per-signal `Pipeline` seam (private fields + accessors + `new`). | `file-lifecycle` substrate work, `otel-ledger` logs binding, future signal (traces) bindings, crate-split refactors. |
| [otel-stream-identity.md](otel-stream-identity.md) | OTel log-stream identity contract: the content-plane (`otel-logs-identity` `ServiceStream`, absent==empty `service.namespace`, canonical `ns_hash`→`part_key`, `content_meta` codec) vs content-agnostic substrate split, the `part_key` single-source-of-truth in `FileId`, and the `part_key`-membership candidate filters. | `otel-logs-identity` hashing/codec, otel-ingestor stream extraction, WAL/SFST/catalog registry candidate filters, `sfsq` query library, query-planner partition filter, offline stream-query CLIs. |
| [prometheus-profiles.md](prometheus-profiles.md) | go.d prometheus chart-profile format, selection modes, app-segment resolution, and chart-context assembly. | prometheus collector profile work, chart-context/UI app-section contract, profile authoring. |
| [query-planner-tier-selection.md](query-planner-tier-selection.md) | Automatic storage-tier selection for metric query planning. | Query planner code, API data paths, MCP metric query behavior. |
| [rust-cross-crate-doc-references.md](rust-cross-crate-doc-references.md) | Cross-crate doc/comment reference rule for workspace Rust crates: verified links for public items, natural language for private ones. | All Rust crate work, refactoring reference searches, doc reviews. |
| [secret-reference-grammar.md](secret-reference-grammar.md) | `${scheme[+modifier]:rest}` secret-reference grammar, resolution, the `urienc` value-encoding modifier, and the two grammar-parse sites that must stay in sync. | go.d secret resolution (`secrets/resolver`, `secretstore`), jobmgr dependency tracking, `SECRETS.md` generator. |
| [sensitive-data-discipline.md](sensitive-data-discipline.md) | Sensitive-data rules for committed artifacts, scripts, skills, specs, SOWs, commit messages, and PR text. | All repository work, SOW audits, private/public skills, token-safe scripts. |
| [snmp-profile-projection.md](snmp-profile-projection.md) | SNMP profile projection contract across metrics, topology, licensing, and BGP consumers. | SNMP profile authoring, ddsnmp loader/projection code, SNMP project skill. |
| [snmp-traps/netdata.md](snmp-traps/netdata.md) | Netdata SNMP trap ingestion, enrichment, storage, journal, OTLP, profile, and metric design contract. | SNMP trap implementation, docs, generated profile work, receiver metrics, operator workflows. |
| [snmp-traps/netdata-snmp-hub-architecture.md](snmp-traps/netdata-snmp-hub-architecture.md) | Netdata distributed SNMP hub architecture principle for trap, polling, topology, flow, and syslog co-location. | SNMP trap docs, topology correlation, network observability architecture decisions. |
| [snmp-traps/trap-metrics-profiles.md](snmp-traps/trap-metrics-profiles.md) | SNMP trap profile-local metric extraction use cases, identity, cardinality, merge, loader, generator, and compatibility design. | SNMP trap metric profile implementation, profile authoring, generator work, chart-template integration, trap docs and skills. |
| [taxonomy.md](taxonomy.md) | Collector chart taxonomy source, validation, and generated artifact contract. | Collector taxonomy files, integrations generators/checkers, Cloud dashboard taxonomy consumption. |
| [topology-containers-ipc-contract.md](topology-containers-ipc-contract.md) | Topology container IPC contracts between cgroups.plugin, apps.plugin, and network-viewer.plugin. | Topology containers IPC producers/consumers, APPS_LOOKUP and CGROUPS_LOOKUP changes, reviewer handoffs. |
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

When a domain hierarchy contains research evidence, the hierarchy MUST document
which files are product contracts and which files are research inputs. Research
MUST NOT be treated as a product spec unless a top-level spec or decision file
explicitly adopts the rule.
