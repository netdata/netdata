# SOW-0002 - Unified multi-layered topology schema and merge engine

## Status

Status: open

Sub-state: scope captured, awaiting user decisions on merge semantics, identity-matching policy, conflict resolution, storage model, and scale targets before implementation planning advances.

## Requirements

### Purpose

Make Netdata produce a single coherent topology view that unifies every source contributing topology evidence — at the same layer (e.g. multiple L2-speaking switches) and across layers (L2 ↔ L3 ↔ L7) — so users see one map of their network/infrastructure rather than four disconnected maps. This is foundational work: topology in Netdata is new, and the choice of merge semantics will shape every downstream consumer (UI, alerts, automation, AI agents).

The work is fit-for-purpose for production observability on large enterprise networks where a single physical device routinely shows up under multiple identities at different layers (e.g. a server has a hostname, a Netdata machine GUID, several MAC addresses, multiple IP addresses, container IDs, Kubernetes pod/namespace identifiers, SNMP-discovered LLDP chassis ID).

### User Request

Verbatim user request: *"create a pending SOW for the Unified multi-layered topology schema, which should allow merging topologies of the same kind (L2 + L2), but also merge topologies of different kinds (L2 + L3, or L2 + L3 + L7)"*.

This SOW captures the problem space, references existing prior planning (`TODO-UNIFIED-TOPOLOGY-SCHEMA.md`), and surfaces the decisions that must be locked before any implementation begins. It does not commit to a specific merge algorithm, identity-matching policy, or storage model — those are user decisions captured in `## Implications And Decisions` below.

### Assistant Understanding

Facts (verified):

- The current schema already exists at `src/go/pkg/topology/types.go`. Each `Actor` has a `Layer` field, a `Source` field, and a `Match` struct with extended identity fields: `ChassisIDs`, `MacAddresses`, `IPAddresses`, `Hostnames`, `DNSNames`, `SysObjectID`, `SysName`, `NetdataNodeID`, `NetdataMachineGUID`, `CloudInstanceID`, `CloudAccountID`, `ContainerIDs`, `PodNames`, `NamespaceIDs` (`types.go:7-22`). Each `Link` carries `Layer`, `Protocol`, `LinkType`, source/destination endpoints, direction, state, timestamps, and metrics (`types.go:42-55`).
- Topology data is produced today by four distinct sources (paths verified in repo):
  1. **SNMP L2** — Go, in `src/go/pkg/topology/engine/` and `src/go/plugin/go.d/collector/snmp_topology/`. Produces actors of type `device` and `endpoint` with chassis IDs, MACs, IPs, sysName/sysObjectID. LLDP, CDP, FDB, ARP, STP.
  2. **NetFlow / IPFIX / sFlow L3** — Rust, in `src/crates/netdata-netflow/`, plus Go plumbing in `src/go/`. Produces flow records with src/dst IP, ports, protocol, AS info, geolocation.
  3. **Network-viewer L7** — C, in `src/collectors/network-viewer.plugin/`. eBPF-derived process-to-process connection observations with PIDs, container IDs, hostnames.
  4. **Netdata streaming** — C, in `src/database/contexts/` and streaming subsystem. Produces parent ↔ child agent relationships with `NetdataNodeID`, `NetdataMachineGUID`.
- A 6797-line prior-planning artifact exists at `TODO-UNIFIED-TOPOLOGY-SCHEMA.md` with extensive notes on the design space, source coverage, IP tracking policy, ASN/geo enrichment options, and several decision items already taken or pending. This SOW supersedes the unstructured TODO once committed; the TODO stays in place as historical reference until the SOW closes.
- The codebase has helper code at `src/go/tools/topology-flow-merge/` and a per-poll merge implementation in `src/go/plugin/go.d/collector/snmp_topology/topology_output_merge.go` (sub-second scope: merging successive snapshots of the same SNMP topology). Cross-source / cross-layer merge does NOT exist today.
- The `Match` struct is the right shape for cross-layer correlation in principle: each source contributes whatever identity fields it knows, and a downstream merge step can correlate any two actors that share at least one identity field. This is the "extended match fields, match on any overlapping field" principle stated in `TODO-UNIFIED-TOPOLOGY-SCHEMA.md:69-72`.

Inferences:

- "Same-kind merge" (L2 + L2) is the easier case: multiple SNMP-managed switches each produce their own L2 view; merging them means deduplicating actors by identity match and unioning their links. Most algorithmic complexity is in identity normalization (MAC formatting, hostname canonicalization, FQDN vs short-name).
- "Cross-kind merge" (L2 + L3, or L2 + L3 + L7) is the harder case: actors are at different abstraction levels (a physical switch port at L2, an IP address at L3, a process at L7). Naive identity match works for some pairs (server's IP appears in both L3 flows and L2 ARP) but breaks for others (a process at L7 has no MAC; a switch at L2 has no PID). The schema must allow a parent-child or "lives-on" relationship rather than forcing every merge to a strict equality.
- Conflict resolution policy is a real design problem: two sources legitimately disagree (e.g. an LLDP neighbor advertises a chassis ID that disagrees with what the device's own SNMP poll reports because of stale cache). Whichever policy is chosen affects every downstream consumer.
- Performance and storage model interact: a memory-only merged view is simpler but limits historical queries and re-merge after configuration change; a persisted index enables more, at significant complexity cost.

Unknowns (real, blocking design decisions):

- Where does merge happen — agent-side per-source, agent-side cross-source, parent-side aggregating from multiple agents, or Cloud-side?
- What's the identity-matching algorithm — strict any-overlap equality, canonicalized equality, probabilistic scoring with thresholds, or learned per-deployment?
- What's the conflict-resolution policy when two sources disagree — newest wins, source-priority order, evidence-set union, or human-resolvable flag?
- What scale targets must the design meet — actors-per-deployment, links-per-deployment, sources-per-actor, merge latency budget?
- What's the storage and re-merge story — recompute on each request, persist a merged snapshot, persist an index of identities, or stream-update?
- How does L7 process-level granularity surface in the merged graph without exploding actor count? (TODO-UNIFIED-TOPOLOGY-SCHEMA.md:1028 notes "L7 Detailed vs Aggregated Views" as an open question.)
- How are stale entries aged out across sources whose freshness windows differ (LLDP cache ~120s, FDB ~5 min, NetFlow flow record duration, network-viewer eBPF connection lifetime)?

### Acceptance Criteria

(Final acceptance criteria are gated on user decisions in `## Implications And Decisions`. Until those decisions land, the criteria below are the high-level shape; they will be sharpened to specific verifiable outcomes once decisions are recorded.)

- A single, documented unified topology schema is in production use by all four current sources (SNMP L2, NetFlow L3, network-viewer L7, Netdata streaming), with the existing `Match`/`Actor`/`Link` types as the canonical contract or a clearly evolved version.
- A merge engine exists that takes N topology graphs (same-kind or cross-kind) and produces one merged graph following the chosen identity-match and conflict-resolution policies. Same-kind (L2 + L2) and cross-kind (L2 + L3, L2 + L3 + L7) cases each have explicit test coverage with expected merged output.
- Identity-matching and conflict-resolution behavior is unit-tested with targeted fixtures covering: actors that should merge, actors that should NOT merge despite an incidental field overlap, sources that legitimately disagree, and stale entries aged across sources with different freshness windows.
- A scale benchmark exists that exercises the merge engine at the chosen actors/links/sources targets and reports merge latency.
- The merged graph can be served as a topology function output without breaking existing UI consumers; if a schema migration is needed, the migration plan is documented.
- No customer-identifying data, community-member names, SNMP communities, bearer tokens, or PII appear in any committed artifact (SOW, code, code comments, tests, fixtures, commit messages, PR body).

## Analysis

Sources checked:

- `src/go/pkg/topology/types.go` — current `Match`, `Actor`, `Link` schema.
- `src/go/pkg/topology/engine/` — current SNMP L2 merge logic (within-source).
- `src/go/plugin/go.d/collector/snmp_topology/topology_output_merge.go` — per-poll snapshot merge (within-source).
- `src/go/tools/topology-flow-merge/` — standalone helper, not currently wired into the runtime path (see TODO-UNIFIED-TOPOLOGY-SCHEMA.md branch-cleanup audit).
- `src/crates/netdata-netflow/` — L3 source.
- `src/collectors/network-viewer.plugin/` — L7 source.
- Streaming subsystem (parent/child agent topology) — under `src/database/contexts/` and adjacent.
- `TODO-UNIFIED-TOPOLOGY-SCHEMA.md` — 6797 lines of prior thinking and partial decisions.
- Adjacent TODOs not yet absorbed: `TODO-streaming-topology.md`, `TODO-TOPOLOGY-ENRICHMENT.md`, `TODO-TOPOLOGY-FLOWS-INCOMPLETE-INTEGRATIONS.md`, `TODO-topology-flows-sync.md`, `TODO-topology-library.md`, `TODO-topology-library-phase2-direct-port.md`, `TODO-topology-netflow-metadata.md`.

Current state:

- Schema shape is good. The extended-match principle is already encoded in `Match`. What's missing is the merge layer that uses these identity fields across sources.
- Each source today produces its own topology output as a separate function. There is no merged endpoint.
- Where merge does exist (same-source, per-poll snapshot fusion in the SNMP topology engine), it is implementation-internal — no shared abstraction, no reuse path for cross-source.
- The codebase has hooks for layered presentation in the topology UI (the `PresentationActorType`, `PresentationLinkType` and friends in `types.go`), suggesting prior thinking about layered views, but no live data wiring.

Risks (cross-cutting):

- **Schema lock-in**: any merge engine that consumes the current `Match` shape effectively freezes that shape for downstream Cloud consumers. A future schema change costs two migrations (sources + merge + Cloud).
- **Identity correlation false positives**: matching on "any overlapping field" is dangerous if a field is non-unique (e.g. private RFC1918 IP that recurs across customer subnets, hostname `localhost`). Without canonicalization and field-quality weighting, merge can fuse unrelated actors.
- **Scale**: cross-layer merge expands actor count; L7 process granularity especially. If the design persists everything, storage grows; if it doesn't, historical queries degrade.
- **Conflict noise**: two sources legitimately disagreeing produces user-visible warnings unless conflict resolution is automatic. Policy choice affects perceived data quality.
- **Cross-version compatibility**: agents at different versions producing different schema versions. The merge engine must tolerate version drift gracefully.
- **Sensitive data exposure**: merged topology surfaces hostnames, IPs, container/pod names, sysName, sysDescr — all potentially customer-identifying. The merge engine output is a public topology surface that the user sees; the produced data must not leak across tenants in any multi-tenant deployment scenario.

## Pre-Implementation Gate

Status: needs-user-decision

Problem / root-cause model:

- Topology evidence is produced by four distinct sources at three distinct layers (L2/L3/L7) plus a streaming hierarchy. Each source has its own view, none is reconciled with the others, and there is no merge engine that takes evidence from multiple sources and produces a single coherent graph. The schema (`Match`/`Actor`/`Link`) is already shaped for cross-layer correlation, but the runtime layer that performs the correlation does not exist. Implementation cannot start until merge semantics, identity-matching policy, conflict resolution, and storage model are locked.

Evidence reviewed:

- See "Sources checked" above. Direct evidence of the gap: no production code path correlates an SNMP-discovered L2 device with a NetFlow-observed L3 endpoint or a network-viewer-observed L7 process; the four topology functions are independent and produce four independent graphs.

Affected contracts and surfaces:

- `src/go/pkg/topology/types.go` — schema may evolve.
- All four source producers (SNMP topology Go path, netflow Rust+Go, network-viewer C, streaming C) — output schema may need conformance changes.
- Topology functions exposed to the UI — possibly a new "merged" function or the existing per-source functions extended.
- Cloud-side consumers — the merged output schema becomes a Cloud contract.
- Test fixtures — new cross-source fixtures required.
- Documentation: `profile-format.md` is unrelated; topology UI documentation will need a section once merge ships.

Existing patterns to reuse:

- The `Match` extended-fields model in `types.go:7-22` is the right contract for identity. Reuse, possibly extend.
- Per-source within-source merge logic in `topology_output_merge.go` and the SNMP topology engine — the same shape (deduplicate by identity, union links) likely generalizes; a candidate to lift to a shared package.
- `src/go/tools/topology-flow-merge/` exists but is not wired in. Decide whether it's the seed of the cross-source merge engine or whether it gets retired.
- The Netdata streaming hierarchy already establishes a parent/child actor relation — useful as a precedent for the "lives-on" cross-layer relationship.

Risk and blast radius:

- Schema evolution touches every consumer; needs a versioning story.
- Cross-source merge is a new public-facing function; UI breakage risk if rolled out without a feature flag or staged rollout.
- Identity-matching false positives are user-visible and erode trust faster than missing data; conservative defaults are safer.
- Performance regressions on large deployments are likely if storage model isn't sized correctly.

Sensitive data plan:

- Same baseline as SOW-0001's plan: no community member names, no customer names, no SNMP communities, no bearer tokens, no SNMPv3 credentials, no customer-identifying IPs, hostnames, sysName/sysDescr/ifAlias/ifDescr, LLDP remote names, port descriptions, chassis IDs, management addresses, container/pod/namespace names.
- Topology fixtures derived from real environments must be sanitized: replace customer-pointing strings with neutral labels (`switch-a`, `endpoint-1`, `service-x`).
- Pre-commit checklist: same `rg -P` rules adopted for SOW-0001 (TBD after SOW-0001 cleanup pass), extended for the additional layer-3 / layer-7 surface (process names, container IDs, k8s pod/namespace names, ASN/geo data).
- Cross-tenant data isolation in the merge engine itself is a runtime concern: the merge function output must respect Cloud tenant boundaries; this is an explicit acceptance check.

Implementation plan:

To be filled after the user decisions below are recorded. The plan will likely have these phases (illustrative, not committed):

1. Lock the schema (extend `Match`/`Actor`/`Link` if needed; document version semantics).
2. Build a shared `topology/merge` package implementing the chosen identity-match + conflict-resolution policy. Unit-test thoroughly with same-kind and cross-kind fixtures.
3. Wire the merge engine behind a new topology function (`topology:unified` or similar). Keep per-source functions intact; the merged function is additive.
4. Add Cloud-side consumer integration plus migration notes for existing UI surfaces.
5. Add scale benchmark + observability counters (rows merged, conflicts detected, identity-match hit/miss rates).
6. Iterate on real-deployment validation: at least two distinct deployments (different vendor mix, different layer mix) should produce coherent merged graphs.

Validation plan:

- Unit tests on the merge package with fixture matrices covering same-kind and cross-kind, identity overlaps and disagreements, age-out scenarios.
- Integration test that wires a synthetic L2 + L3 + L7 input set and verifies the merged graph matches a hand-curated expected output.
- Scale benchmark at the chosen targets.
- Real-deployment validation in at least two environments.

Artifact impact plan:

- `AGENTS.md`: no expected change.
- Runtime project skills: no expected change.
- Specs under `.agents/sow/specs/`: a new spec documenting the unified-topology contract and identity-match algorithm is likely warranted on close (not at open).
- End-user/operator docs: topology UI section(s) updated when the new function ships.
- End-user/operator skills: none.
- SOW lifecycle: this SOW absorbs `TODO-UNIFIED-TOPOLOGY-SCHEMA.md` once user decisions are recorded; the TODO stays in place as historical reference until this SOW closes.

Open decisions: see `## Implications And Decisions` below — six decisions are outstanding.

## Implications And Decisions

The decisions below are unresolved and block implementation. Each is presented with options, pros/cons/implications/risks, and a recommendation that the user can accept or override.

### Decision 1 — Where does the merge happen?

**Options:**
- **A. Agent-side, per-source.** Each source produces an already-merged-within-itself graph. No cross-source merge.
- **B. Agent-side, cross-source on the same node.** The local agent merges all sources it produces. Cross-agent merge happens elsewhere (parent or Cloud).
- **C. Parent-side aggregation.** Streaming parents merge children's contributions. Cloud receives an aggregated view.
- **D. Cloud-side.** All raw evidence flows up, Cloud merges. Maximum flexibility, maximum bandwidth and storage cost.
- **E. Hybrid.** Local same-source merge (A) + parent same-kind merge (C) + Cloud cross-kind merge (D).

**Implications/Risks:** D leaks raw evidence to Cloud (cardinality concerns), C requires every parent to know all children's sources, A produces nothing useful for cross-source queries, B is the cleanest for single-host deployments but can't unify a fleet. E is the most flexible but has the largest blast radius.

**Recommendation: B for now, plan E as the long-term shape.** Start by merging on the local agent for sources the local agent produces; defer cross-agent merge until single-agent merge is solid. This minimizes Cloud schema lock-in and lets the merge engine evolve based on real local-agent feedback.

### Decision 2 — Identity-matching algorithm

**Options:**
- **A. Strict any-overlap equality on raw `Match` fields.** Two actors merge if any one of their `Match` slices intersects.
- **B. Canonicalized equality.** MACs lowercased / colon-stripped, hostnames lowercased / FQDN-normalized, IPs canonicalized, before equality check.
- **C. Field-quality-weighted scoring.** Each `Match` field has a discriminator weight (high for chassis ID, low for hostname `localhost`). Merge above a threshold.
- **D. Probabilistic / learned per deployment.** A model decides; tunable per environment.

**Implications/Risks:** A produces false positives on ambient identifiers (`localhost`, RFC1918 reuse). B handles 90% of false positives at low complexity. C handles ambiguity well but introduces a tuning knob users must understand. D is overkill for opening this work.

**Recommendation: B as MVP; C as a follow-up SOW once real deployments produce telemetry on false-positive rates.** B is implementable, debuggable, and predictable.

### Decision 3 — Conflict-resolution policy when sources disagree

**Options:**
- **A. Newest wins (timestamp-based).** Last evidence supersedes earlier.
- **B. Source-priority order.** A defined priority — e.g. SNMP managed > LLDP-derived inferred > NetFlow-implied > network-viewer-implied. Highest priority wins.
- **C. Evidence-set union, no resolution.** Both values are kept, exposed to the consumer as a multi-valued field.
- **D. Human-resolvable flag.** Conflicts produce a UI warning, user picks.

**Implications/Risks:** A loses provenance; B requires getting the priority right and is hard to change later; C grows attribute payloads without bound but never silently drops information; D produces UX friction.

**Recommendation: C for `Match` fields, B for `Attributes`/`Derived` fields.** Identity should be inclusive (preserve all evidence so future merges can still match); attributes should be resolvable (one displayed value at a time). Provenance in either case is preserved as a per-value `source` tag.

### Decision 4 — Storage model

**Options:**
- **A. Memory-only, recompute every request.** Simplest. No persistence.
- **B. Persisted merged snapshot, recomputed on schedule.** One canonical view, served from cache.
- **C. Persisted identity index.** Identity → actor lookup is persisted; the merged graph is computed on demand from raw source evidence using the index.
- **D. Stream-updated.** Each source emits deltas; the merge engine maintains a live merged graph.

**Implications/Risks:** A doesn't scale to large fleets; B trades freshness for cost; C is the right shape for query-heavy workloads but is a bigger build; D is a real-time pipeline with all the operational complexity that implies.

**Recommendation: A for MVP within a single agent (minimal complexity, fast to ship), with the design ensuring the merge engine is deterministic so future migration to B or C is straightforward.**

### Decision 5 — Scale targets

**Options (illustrative):**
- **A. Modest.** ≤ 1000 actors, ≤ 5000 links, ≤ 4 sources per actor.
- **B. Mid.** ≤ 10000 actors, ≤ 50000 links, ≤ 8 sources per actor.
- **C. High.** ≤ 100000 actors, ≤ 1M links, ≤ 16 sources per actor.

**Implications/Risks:** Picking too small understates real enterprise networks; picking too large bloats the design. The right pick depends on whether L7 processes are first-class actors (then C) or aggregated to host level (then B).

**Recommendation: B as initial target, with the design able to scale to C without re-architecture.** Validates against real enterprise mid-tier deployments and leaves room for L7-detail growth.

### Decision 6 — L7 process granularity

**Options:**
- **A. L7 process is a first-class actor.** Each process gets its own actor; potentially explodes count.
- **B. L7 aggregates to host actor with process-level attributes / sub-table.** One actor per host; processes are children or attributes.
- **C. Two views: detailed and aggregated.** UI toggles between per-process and per-host.

**Implications/Risks:** A is most informative but punishing at scale; B loses detail; C is the most flexible but requires two code paths. (TODO-UNIFIED-TOPOLOGY-SCHEMA.md:1028 notes this as an open item.)

**Recommendation: B for MVP, C as a follow-up once UI and scale targets are validated.**

## Plan

Filled after Decisions 1-6 are recorded. Default skeleton (assuming the recommendations above):

1. Lock and document the schema (extend `Match` if a decision requires it; otherwise leave as-is). Add a per-value `source` provenance tag mechanism to support Decision 3.
2. Implement a `topology/merge` package on the agent side with the chosen identity-match (Decision 2) and conflict-resolution (Decision 3) policies. Unit tests cover same-kind and cross-kind matrices.
3. Wire the merge engine behind a new topology function on the local agent. Per-source functions remain available unchanged.
4. Sanitized fixture set covering at least: a 2-switch L2 same-kind merge, an L2 + L3 cross-kind merge, an L2 + L3 + L7 cross-kind merge with a host-level L7 aggregation per Decision 6.
5. Scale benchmark at the chosen target (Decision 5).
6. Documentation: add a "Topology unified view" section to user-facing docs; add a project-level spec under `.agents/sow/specs/` describing the unified contract and the merge algorithm.
7. Real-deployment validation across two distinct environments before declaring done.

## Execution Log

### 2026-05-01

- SOW opened in `pending/` per user request following completion of the third review round on SOW-0001.
- Existing prior planning artifact (`TODO-UNIFIED-TOPOLOGY-SCHEMA.md`, 6797 lines) noted as historical context; this SOW is the canonical replacement once user decisions are locked.
- Six open decisions recorded; no implementation work begins until they are resolved.

## Validation

Pending — gated on locked decisions and implementation.

## Outcome

Pending.

## Lessons Extracted

Pending until validation.

## Followup

Adjacent TODOs to absorb or supersede when this SOW progresses:

- `TODO-streaming-topology.md` — streaming-source contributions to the merged graph.
- `TODO-TOPOLOGY-ENRICHMENT.md` — geo / ASN / vendor enrichment as cross-cutting attributes.
- `TODO-TOPOLOGY-FLOWS-INCOMPLETE-INTEGRATIONS.md` — flows-side integration gaps surfaced during the original split-PR cleanup.
- `TODO-topology-flows-sync.md` — flows-side sync semantics.
- `TODO-topology-library.md` — shared-library packaging considerations.
- `TODO-topology-library-phase2-direct-port.md` — phase-2 port plan (likely a separate SOW once this one progresses).
- `TODO-topology-netflow-metadata.md` — netflow metadata fields for cross-layer correlation.
- `src/go/tools/topology-flow-merge/` — decide retirement vs absorb-as-merge-package-seed.
- Possible child SOWs after this one progresses: identity-match scoring (Decision 2C as follow-up), persisted merge index (Decision 4C as follow-up), L7 detailed view (Decision 6C as follow-up), Cloud-side merge (Decision 1D/E as follow-up).
