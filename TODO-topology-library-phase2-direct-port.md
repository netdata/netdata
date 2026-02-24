# TODO-topology-library-phase2-direct-port

## 0) Install Order (Mandatory)
- Install backend first:
  - `./install.sh`
- Only after backend install completes, install frontend:
  - `cd ~/src/dashboard/cloud-frontend && sudo ./agent.sh`

## 1) TL;DR
- Objective remains the same: behavior-faithful SNMP L2 topology in Go, deterministic and production-safe.
- Architecture remains the same: backend does all correlation/naming/semantics; frontend is a visualizer only.
- This cleanup removes duplicated/contradictory historical logs and keeps only canonical requirements, key decisions, current status, and next execution plan.

## 1.1) Active Checklist (Now)
- [x] A0. Enforce install order in this TODO and execution flow.
  - Backend install:
    - `./install.sh`
  - Frontend install (only after backend):
    - `cd ~/src/dashboard/cloud-frontend && sudo ./agent.sh`
- [x] A0.1. Port rendering update for detected type semantics.
  - Bullets:
    - hover includes detected port type,
    - color reflects detected type with LLDP/CDP priority.
  - Popover:
    - port badges use detected type colors,
    - colors intensified with dark text for contrast.
- [x] A0.2. Tooltip cleanup for segment/link readability.
  - Segment participants:
    - actor display name + port + actor id in parentheses,
    - no repeated segment name per participant.
  - Link meta:
    - explicit structured rows (source/target/protocol/direction/metrics/id).
- [x] A0.3. Validate multi-MAC handling for SNMP managed actors.
  - Confirmed/fixed:
    - device actor identities now include interface MAC aliases (not only chassis MAC),
    - regression test added for interface MAC alias propagation.
- [x] A0.4. Align idle port popover color with idle port bullet color.
  - Updated popover idle color token to match bullet token.
- [ ] A1. Validate latest backend+frontend install on live office topology (hard refresh, multiple refresh cycles).
  - Evidence required:
    - screenshot/log excerpt of expected actor/link stability across refreshes,
    - no duplicated managed actors by MAC identity.
- [ ] A2. Verify port role rendering contract from backend payload.
  - Evidence required:
    - `if_statuses` includes `topology_role`, `topology_role_confidence`, `topology_role_sources`,
    - UI reflects payload values without local role recomputation.
- [ ] A3. Re-check managed device MAC completeness on known problematic devices.
  - Evidence required:
    - payload identities include expected primary MAC aliases,
    - no split actors caused by missing managed MAC.
- [ ] A4. Re-check FDB endpoint placement quality (switch-facing suppression + reporter-matrix owner rule).
  - Evidence required:
    - no obvious transit endpoints incorrectly attached to inter-switch ports,
    - ambiguous ownership remains suppressed (not forced).
- [ ] A5. Confirm segment hygiene rules remain intact after latest changes.
  - Evidence required:
    - no one-sided segments,
    - no segment path duplicated in parallel with direct LLDP/CDP edge.
- [ ] A6. Keep parity + regression suites green after any fix.
  - Required commands:
    - `cd src/go && go test ./pkg/topology/engine -count=1`
    - `cd src/go && go test ./plugin/go.d/collector/snmp ./pkg/topology/engine/... -count=1`
    - `cd src/go && go run ./tools/topology-parity-evidence --mode suite`
    - `cd src/go && go run ./tools/topology-parity-evidence --mode verify`
    - `cd src/go && go run ./tools/topology-parity-evidence --mode phase2`
- [ ] A7. Investigate `mega` duplicate actor behavior and verify exact connectivity evidence.
  - Evidence required:
    - actor ids, MAC/IP identities, and actor kinds for all `mega*` actors in live payload,
    - all links/segments touching those actors with ports/protocol/source,
    - explicit root-cause statement for why they are/are not merged today.
  - Investigation (2026-02-24):
    - In `/tmp/topology-l2-now.pretty.json` there are two actors named `mega`:
      - device actor `mac:e4:3d:1a:33:10:1a` (LLDP peer identity),
      - endpoint actor `mac:e4:3d:1a:33:10:1b` (FDB/ARP identity).
    - Connectivity in same payload:
      - LLDP edge: `xs1930.plaka:4 -> mega:d85ed31312ff` (actor `...10:1a`),
      - BRIDGE segment edge: `mega:d85ed31312ff -> ...segment:0` (actor `...10:1a`),
      - no emitted link for actor `...10:1b` (unlinked endpoint).
    - Root-cause:
      - overlap correlation uses hardware identity first (MAC/chassis),
      - endpoint `...10:1b` does not overlap managed/known device hardware IDs for `...10:1a`,
      - shared IP alone is intentionally not used as hard merge key.
  - Enlinkd behavior check (code evidence, 2026-02-24):
    - Enlinkd does not create a separate graph vertex per MAC endpoint when that MAC resolves to the same managed node via unique IP mapping.
    - `IpNetToMediaTopologyServiceImpl` maps ARP entry to `OnmsNode` only when IP resolves to exactly one node:
      - `features/enlinkd/service/impl/src/main/java/org/opennms/netmgt/enlinkd/service/impl/IpNetToMediaTopologyServiceImpl.java:120`
      - `features/enlinkd/service/impl/src/main/java/org/opennms/netmgt/enlinkd/service/impl/IpNetToMediaTopologyServiceImpl.java:137`
      - ambiguity guard (`Multiple Nodes`) avoids forced merge:
      - `features/enlinkd/service/impl/src/main/java/org/opennms/netmgt/enlinkd/service/impl/IpNetToMediaTopologyServiceImpl.java:128`
    - Topology graph deduplicates by node vertex id (`nodeId`) and reuses existing node vertex:
      - `features/enlinkd/adapters/updaters/bridge/src/main/java/org/opennms/netmgt/enlinkd/BridgeOnmsTopologyUpdater.java:136`
      - `features/enlinkd/adapters/updaters/bridge/src/main/java/org/opennms/netmgt/enlinkd/BridgeOnmsTopologyUpdater.java:152`
      - MAC-only unresolved entries go to cloud/segment vertex, not a second node vertex:
      - `features/enlinkd/adapters/updaters/bridge/src/main/java/org/opennms/netmgt/enlinkd/BridgeOnmsTopologyUpdater.java:163`
  - OpenNMS map behavior verification (2026-02-24):
    - "Collapses per IP" is only partially true:
      - ARP/FDB entries are attached to a node when IP resolves to exactly one monitored interface:
        - `features/enlinkd/service/impl/src/main/java/org/opennms/netmgt/enlinkd/service/impl/IpNetToMediaTopologyServiceImpl.java:120`
        - `features/enlinkd/service/impl/src/main/java/org/opennms/netmgt/enlinkd/service/impl/IpNetToMediaTopologyServiceImpl.java:137`
      - ambiguous IPs are not merged (tagged as `Multiple Nodes`):
        - `features/enlinkd/service/impl/src/main/java/org/opennms/netmgt/enlinkd/service/impl/IpNetToMediaTopologyServiceImpl.java:128`
      - graph dedup in renderer path is by node vertex id:
        - `features/enlinkd/adapters/updaters/bridge/src/main/java/org/opennms/netmgt/enlinkd/BridgeOnmsTopologyUpdater.java:136`
    - Non-IP/unresolved addresses are grouped in a per-segment cloud vertex:
      - cloud creation from unresolved MACs:
        - `features/enlinkd/service/api/src/main/java/org/opennms/netmgt/enlinkd/service/api/TopologyService.java:42`
      - cloud label text:
        - `features/enlinkd/service/api/src/main/java/org/opennms/netmgt/enlinkd/service/api/Topology.java:104`
      - cloud attached to segment graph:
        - `features/enlinkd/adapters/updaters/bridge/src/main/java/org/opennms/netmgt/enlinkd/BridgeOnmsTopologyUpdater.java:255`
    - "No unlinked endpoints shown" is consistent with this graph model:
      - bridge topology graph is built from `TopologyShared` segments and their participants:
        - `features/enlinkd/service/impl/src/main/java/org/opennms/netmgt/enlinkd/service/impl/BridgeTopologyServiceImpl.java:821`
      - updater renders nodes/segments/cloud edges from those shared structures, not separate unlinked endpoint actors:
        - `features/enlinkd/adapters/updaters/bridge/src/main/java/org/opennms/netmgt/enlinkd/BridgeOnmsTopologyUpdater.java:129`
  - Segment review (`xs1930`/`mega`/`Broadcom`, 2026-02-24):
    - The problematic bridge segment currently has **three** participants:
      - `mac:70:49:a2:65:72:cd` (`xs1930.plaka`)
      - `mac:d8:5e:d3:16:92:5d` (`Broadcom P210tep ...`)
      - `mac:e4:3d:1a:33:10:1a` (`mega`)
    - `mac:e4:3d:1a:33:10:1b` is not on that segment (it is unlinked in current payload).
    - Therefore merging/collapsing only the two `mega` identities does not remove this segment by itself; additional correlation/pruning is required for the `Broadcom` participant if treated as same device artifact.

## 1.2) Decisions (Costa, 2026-02-24)
- [x] D1. Internal options to support in backend/UI state:
  - `collapse_actors_by_ip` (default: `true`)
  - `eliminate_non_ip_inferred` (default: `true`)
  - `probabilistic_connectivity` (default: `true`)
- [x] D2. Public UX must expose only two controls:
  - `Nodes Identity: mac | ip` (default: `ip`)
    - `ip` => collapse actors by IP + eliminate non-IP inferred devices/endpoints.
  - `Non LLDP/CDP Connectivity: strict | probable` (default: `probable`)
    - `probable` => connect non-LLDP/CDP inferred actors to the most probable segment.
- [x] D3. Segment cleanup implication accepted:
  - when non-IP inferred elimination removes participants, segments with <= 1 participants may be removed.
- [x] D4. Clarification on elimination/connectivity scope:
  - non-IP elimination applies to **all inferred devices/endpoints**, including inferred LLDP/CDP actors.
  - `probable` connectivity applies **only** to non-LLDP/CDP inferred endpoints.
  - LLDP/CDP-discovered direct connectivity remains authoritative and unaffected by probable inference.

## 2) Objective (Non-Negotiable)
- Build and maintain **Enlinkd-faithful** topology behavior for Netdata SNMP topology, scoped to **L2 + enrichment**.
- Keep focus on:
  - correctness,
  - deterministic output,
  - testability,
  - maintainability,
  - production reliability.
- Scope lock: this TODO tracks the active workstream for SNMP topology behavior and reliability.

## 3) Architecture (Canonical)

### 2.1 Data flow
- SNMP collector/discovery -> topology cache snapshot -> topology engine (`l2_pipeline.go`) -> shaping/serialization (`topology_adapter.go`) -> `topology:snmp` function output -> UI rendering.

### 2.2 Responsibility split
- Backend (collector + engine + adapter):
  - parsing and normalization,
  - correlation and deduplication,
  - actor/link/port identity and canonical naming,
  - topology semantics (ownership, inferred/managed role, filtering decisions).
- Frontend:
  - render what backend returns,
  - optional display filters/toggles only,
  - no correlation/dedup/matching logic.

### 2.3 Snapshot contract
- Topology output must be from an atomic completed snapshot (no partial-gap rendering).
- Deterministic IDs/names and stable structure across refreshes for unchanged inputs.

## 4) Scope

### 3.1 In scope
- L2 topology only:
  - LLDP,
  - CDP,
  - BRIDGE/FDB,
  - ARP/ND enrichment,
  - Q-BRIDGE VLAN enrichment,
  - STP enrichment/correlation,
  - Cisco VTP VLAN context enrichment.
- End-to-end behavior from collection -> function output.

### 3.2 Out of scope
- L3 routing topology (OSPF/ISIS).
- `topology_view=l3` and `topology_view=merged` for SNMP topology in this workstream.
- NetFlow/flows.
- UI visual styling work.

## 5) Key Decisions (Costa, Canonical Summary)

### 4.1 Scope and protocol decisions
- SNMP topology scope is **L2 + enrichment only**.
- L3 removed for this workstream:
  - no ISIS,
  - no OSPF,
  - no `topology_view=l3`,
  - no `topology_view=merged`.
- Future discovery protocol expansion is allowed (more protocols can be added later), but immediate execution remains controlled by current scope/priority.

### 4.2 Collection/config decisions
- One agent-wide topology across all SNMP jobs.
- Topology scheduling/frequency is topology-plane config, not per-device frequency.
- Protocol precedence:
  - effective protocols per job = global enabled protocols ∩ per-job allow-list.
- If per-job allow-list includes protocols disabled globally:
  - silently ignored (no warning/error).
- Default per-job topology allow-list when absent:
  - `"*"` (all globally enabled protocols).
- Avoid forcing per-job manual profiles/autoprobe flags when defaults already provide required behavior.

### 4.3 Identity/correlation decisions
- In L2, MAC is the primary identity key.
- Different MACs -> different actors, even if names/IPs overlap.
- SNMP-managed device identity enrichment must use all reliable SNMP sources (bridge base MAC, FDB self evidence, interface MAC fallbacks).
- Parser correctness policy:
  - dedicated parser per field semantic type,
  - strict separation of parsing vs semantic meaning,
  - invalid/sentinel values must not create topology side effects.

### 4.4 STP decisions
- STP must never create actors.
- STP is enrichment/correlation signal only.
- STP can influence confidence/association, not introduce synthetic devices.

### 4.5 VLAN/segment decisions
- VLAN is an observed topology dimension/overlay, not a default physical actor.
- Segments represent inferred broadcast-domain structure where direct neighbor identity is incomplete.

### 4.6 Backend/UI contract decisions
- Backend is sole owner of canonical actor/link/port names and IDs.
- UI must not infer/correlate/deduplicate.
- Canonical naming must be consistent across all entities.

### 4.7 FDB ownership/dedup decisions
- LLDP/CDP switch-facing evidence has precedence for suppressing transit FDB endpoint misplacement.
- Reporter-matrix owner rule is required for ambiguous FDB ownership.
- Managed-identity overlap behavior:
  - if overlap resolves to exactly one managed device -> emit deterministic replacement edge,
  - if 0 or >1 managed matches -> suppress ambiguous forced mapping.

### 4.8 Process decision
- 2026-02-24:
  - Maintain an explicit short **Active Checklist** at the top of this TODO to drive next execution steps.

## 6) Current Implementation Status (Canonical)

### 5.1 Completed implementation highlights
- Topology role and link-mode metadata exposed per interface status.
- Role model currently includes:
  - `switch_facing`,
  - `host_facing` (strict),
  - `host_candidate`.
- STP role contribution is corroborated-only (requires managed-alias FDB evidence).
- Bridge-link extraction refactored to precompute once and reuse.
- Managed-alias FDB evidence integrated into switch-facing gating and endpoint owner inference.

### 5.2 Evidence pointers (source of truth)
- Backend implementation:
  - `src/go/pkg/topology/engine/topology_adapter.go`
  - `src/go/pkg/topology/engine/l2_pipeline.go`
  - `src/go/plugin/go.d/collector/snmp/topology_cache.go`
  - `src/go/plugin/go.d/collector/snmp/func_topology.go`
- Regression tests:
  - `src/go/pkg/topology/engine/topology_adapter_test.go`
- Parity/runtime evidence:
  - `src/go/pkg/topology/engine/parity/evidence/parity-summary.json`
  - `src/go/pkg/topology/engine/parity/evidence/phase2-parity-report.json`
  - `src/go/pkg/topology/engine/parity/evidence/phase2-gap-report.md`

## 7) Analysis (Current State)
- Facts:
  - Parity suites are green for the tracked scope.
  - Topology adapter role/classification follow-up changes are implemented and covered by tests.
  - Backend + frontend install flow has been executed (frontend reinstalled after backend).
- Known non-fatal installer warnings observed during install:
  - `./netdata-installer.sh: line 554: shift: shift count out of range`
  - `git fetch -t` access failure in installer path (SSH permissions), install still completed.
- Risk:
  - This TODO had accumulated conflicting historical statements (for example L3 in-scope vs L3 removed).
  - Cleanup resolves conflicts by preserving latest accepted canonical decisions only.

## 8) Plan (Next Execution)
- P1. Continue live validation loops against office topology with current backend logic.
- P2. Address remaining correctness issues from observed live output using backend-first fixes only.
- P3. Keep parity evidence and targeted regression tests updated with each behavior change.
- P4. Only after behavior is stable, perform additional optional enhancements (future protocol expansion, additional toggles) under explicit decisions.

## 9) Implied Decisions (Recorded)
- Historical verbose gate/session logs are intentionally condensed; git history remains the audit source for detailed chronology.
- Contradictory historical text is removed in favor of latest accepted decisions.
- This file is now the canonical operational TODO for continuation.

## 10) Testing Requirements (Mandatory)
- For every behavior change:
  - targeted unit tests in topology engine,
  - SNMP collector + engine integration test pass,
  - parity evidence commands pass.
- Required command set:
  - `cd src/go && go test ./pkg/topology/engine -count=1`
  - `cd src/go && go test ./plugin/go.d/collector/snmp ./pkg/topology/engine/... -count=1`
  - `cd src/go && go run ./tools/topology-parity-evidence --mode suite`
  - `cd src/go && go run ./tools/topology-parity-evidence --mode verify`
  - `cd src/go && go run ./tools/topology-parity-evidence --mode phase2`
- Parsing/correlation changes must include edge-case tests (invalid/sentinel/ambiguous cases) with deterministic expected behavior.

## 11) Documentation Updates Required
- Keep this TODO synchronized with:
  - accepted design decisions,
  - architecture responsibilities,
  - current objective/scope,
  - latest canonical status.
- If function contract fields change, update docs for:
  - topology function payload schema,
  - backend/frontend responsibility boundaries,
  - operator-facing behavior notes (for example role semantics and filtering semantics).

## 12) Latest Task Entry (Cleanup)
- Timestamp (UTC): `2026-02-24`
- Task:
  - Clean up this TODO while preserving design, architecture, objective, and key decisions.
- Result:
  - Completed. File reduced to canonical operational content with conflicting historical statements removed.

- Timestamp (UTC): `2026-02-24`
- Task:
  - Add a short active checklist to continue execution quickly.
- Result:
  - Completed. Added section `1.1) Active Checklist (Now)` and recorded process decision in `4.8`.

- Timestamp (UTC): `2026-02-24`
- Task:
  - Enforce install-order note at top of TODO and implement UI/identity follow-ups:
    - port bullet hover/type coloring with LLDP priority,
    - popover port color intensity + contrast,
    - segment/link tooltip structure cleanup,
    - verify/fix multi-MAC handling for SNMP actors.
- Result:
  - Completed.
  - Added mandatory install-order section and executed installs in this order:
    - `./install.sh`
    - `cd ~/src/dashboard/cloud-frontend && sudo ./agent.sh`
  - Frontend graph/popover updates applied.
  - Backend multi-MAC actor identity propagation extended with interface MAC aliases.
  - Added regression test for device actor MAC alias propagation.

- Timestamp (UTC): `2026-02-24`
- Task:
  - Fix idle-port color mismatch between graph bullets and popover badges.
- Result:
  - Completed.
  - Idle popover badge now uses the same color token as idle bullets for visual parity.
