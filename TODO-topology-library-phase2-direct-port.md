# TODO-topology-library-phase2-direct-port

## 0) Auth Token Location (Mandatory)
- Bearer token for authenticated topology function calls is stored at:
  - `/var/lib/netdata/bearer_tokens/`

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
- [x] A0.5. Execute install order (backend then frontend).
  - Executed:
    - `./install.sh` (backend) on 2026-02-24
    - `cd ~/src/dashboard/cloud-frontend && sudo ./agent.sh` (frontend) on 2026-02-24
  - Result:
    - both completed successfully
    - backend installer reported non-fatal warning: `git fetch -t` failed (`Permission denied (publickey)`).
- [x] A0.6. Probable-link visual distinction in UI.
  - Updated force-graph link classification to treat `metrics.attachment_mode=probable_*` as probable, even if explicit state is absent.
  - Updated probable link color to darker cyan for clearer contrast against strict/high-confidence links.
  - File:
    - `~/src/dashboard/netdata-cloud-frontend.git/src/domains/functions/components/graph/forceGraph.js`
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
  - Current execution note (2026-02-24):
    - parity evidence commands are blocked in this workstation because fixture root is missing:
      - `/tmp/topology-library-repos/enlinkd/features/enlinkd/tests/src/test/resources/linkd`
    - unit/integration go test suites for `engine` and `snmp` pass.
- [x] A11. Probable connectivity contract hardening (`strict + delta only`).
  - New required behavior:
    - probable pass starts from strict-unlinked endpoints only,
    - adds at most one new probable attachment per strict-unlinked endpoint,
    - never mutates or duplicates strict links,
    - never adds extra links for endpoints already linked in strict.
  - Missing-path fallback (when strict-unlinked endpoint has zero segment candidates):
    - use reporter evidence (`device_ids`, `if_indexes`, `if_names`) and FDB observations,
    - if still unresolved, attach endpoint via deterministic `portless probable` segment anchored to one managed reporter device.
  - Required output markers:
    - probable links must set `state=probable`,
    - metrics include `inference=probable`, `confidence=low`, and explicit `attachment_mode`.
  - Required tests:
    - strict links are subset of probable links,
    - probable does not create additional links for strict-linked endpoints,
    - strict-unlinked endpoints receive one probable link when any reporter evidence exists,
    - no extra device-device segment paths are introduced by probable fallback.
  - Implemented (2026-02-24):
    - probable assignment now iterates all known endpoints (not only endpoints already present in segment candidate map),
    - strict and probable phases are split:
      - strict resolves deterministic candidates first,
      - probable runs only for endpoints still unlinked after strict.
    - reporter-hint probable candidate selection added:
      - uses `learned_device_ids`, `learned_if_indexes`, `learned_if_names`,
      - consumes FDB reporter evidence as fallback when label hints are absent.
    - deterministic `probable_portless` fallback added for strict-unlinked endpoints with no segment candidates:
      - creates synthetic `bridge-domain:probable:*` segment anchored to one managed reporter hint,
      - emits one probable endpoint attachment and one bridge anchor edge.
    - probable metadata is explicit:
      - `state=probable`,
      - `metrics.inference=probable`,
      - `metrics.confidence=low`,
      - `metrics.attachment_mode=probable_segment|probable_portless`.
  - Backend tests:
    - `cd src/go && go test ./pkg/topology/engine -count=1` passed
    - `cd src/go && go test ./plugin/go.d/collector/snmp ./pkg/topology/engine/... -count=1` passed
  - Live validation (`nodes_identity=ip`):
    - strict:
      - actors: `segment=6 endpoint=74 device=8`
      - unlinked: `endpoint=48`
      - stats: `links_total=43`, `links_probable=0`
    - probable:
      - actors: `segment=9 endpoint=74 device=8`
      - unlinked: `endpoint=1`
      - stats: `links_total=98`, `links_probable=48`
    - strict subset check:
    - strict link signatures in probable: `42/42` (none missing)
    - strict-linked endpoint mutation check:
      - probable links on strict-linked endpoints: `0`
  - Follow-up hardening (2026-02-24):
    - add a final probable salvage pass for endpoints that remain unlinked after normal strict+probable assignment due overlap suppression edge-cases,
    - salvage emits exactly one probable attachment per still-unlinked endpoint using direct owner evidence first, then deterministic portless probable anchor,
    - preserve `strict + delta` behavior by touching only still-unlinked endpoints.
  - Implemented (2026-02-24):
    - overlap suppression now distinguishes managed-vs-unmanaged identity overlap:
      - managed overlap behavior unchanged (replace endpoint edge with managed device edge),
      - unmanaged overlap stays suppressed in strict but is allowed in probable and marked as probable.
    - probable-only overlap recovery now marks emitted direct-owner links as:
      - `state=probable`,
      - `metrics.inference=probable`,
      - `metrics.confidence=low`,
      - `metrics.attachment_mode=probable_direct`.
  - Added regression test:
    - `TestToTopologyData_ProbableConnectivityRecoversUnmanagedOverlapSuppression`
  - Verification:
    - `cd src/go && go test ./pkg/topology/engine -count=1` passed
    - `cd src/go && go test ./plugin/go.d/collector/snmp ./pkg/topology/engine/... -count=1` passed
  - Install verification:
    - backend installed with `./install.sh` (non-fatal warning remains: `git fetch -t` SSH key),
    - frontend installed after backend with `cd ~/src/dashboard/cloud-frontend && sudo ./agent.sh`.
- [x] A10. Fix IP + probable mode so inferred endpoints are connected and rendered as probable (darker cyan).
  - Reported issue (2026-02-24):
    - In `nodes_identity=ip` + `non_lldp_cdp_connectivity=probable`, inferred endpoints are still left unlinked and/or not marked as probable in output.
  - Target behavior:
    - in probable mode, inferred endpoint attachments should emit links whenever segment candidates exist,
    - emitted inferred endpoint links should carry probable markers for frontend darker-cyan rendering.
  - Implementation (2026-02-24):
    - probable is now layered strictly on top of strict:
      - strict-linked endpoints remain strict and unchanged,
      - probable applies only to endpoints that remain unlinked after strict pass.
      - source-label gate removed, so all strict-unlinked endpoints can receive probable assignment.
      - probable-only segments now emit a single anchor device->segment bridge edge (minimal patch) to avoid creating extra device-device segment paths in probable mode.
    - probabilistic endpoint selection now falls back to deterministic first candidate when scoring cannot disambiguate.
    - exactly one probable segment attachment is emitted per previously-unlinked endpoint.
  - Validation:
    - `go test ./pkg/topology/engine -count=1` passed,
    - `go test ./plugin/go.d/collector/snmp ./pkg/topology/engine/... -count=1` passed.
    - parity tools:
      - `go run ./tools/topology-parity-evidence --mode suite` passed,
      - `go run ./tools/topology-parity-evidence --mode verify` passed,
      - `go run ./tools/topology-parity-evidence --mode phase2` passed.
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
- [x] A12. Investigate the single remaining unlinked device in live `nodes_identity=ip` + `probable` mode.
  - Evidence required:
    - exact actor id / identities / labels,
    - whether it has any candidate segment or direct-owner evidence,
    - exact suppression path in `projectSegmentTopology()` causing it to remain unlinked,
    - fix recommendation if behavior is unintended.
  - Findings (from `/tmp/topology-live-ip-probable-after2.json`):
    - Remaining unlinked endpoint actor:
      - `actor_id=mac:9c:6b:00:7b:98:c7`, `display_name=nova-nic2`, `ip=10.20.4.22`.
    - It has direct-owner evidence (`attached_by=single_port_mac`, attached on `XS1930/swp01`).
    - The same physical node is already represented by managed device actor `mac:9c:6b:00:7b:98:c6` (`display_name=nova`) and has emitted links.
  - Root cause in backend:
    - During endpoint emission, identity overlap is evaluated first.
    - For managed overlap, endpoint-to-segment emission is intentionally suppressed to avoid duplicate identity edges:
      - `src/go/pkg/topology/engine/topology_adapter.go:850`
      - `src/go/pkg/topology/engine/topology_adapter.go:857`
      - `src/go/pkg/topology/engine/topology_adapter.go:891`
    - `nodes_identity=ip` collapse does not merge these two actors because their IPs differ (`172.22.0.1` vs `10.20.4.22`):
      - `src/go/pkg/topology/engine/topology_adapter.go:2509`
      - `src/go/pkg/topology/engine/topology_adapter.go:2514`
      - `src/go/pkg/topology/engine/topology_adapter.go:2519`
  - Conclusion:
    - This is a duplicate-identity suppression artifact (not missing discovery).
    - The endpoint remains visible as an actor but has no emitted link because the managed device representation already owns connectivity.
  - Fix implemented (2026-02-24):
    - backend now tracks endpoint IDs suppressed due managed-overlap replacement.
    - in `nodes_identity=ip` mode, unlinked endpoint actors matching those suppressed managed-overlap identities are pruned from actor list.
    - stat `actors_unlinked_suppressed` now reports this count.
  - Post-install live verification (2026-02-24, local function call):
    - Function call:
      - `POST /api/v3/function?function=topology:snmp`
      - payload: `{"nodes_identity":"ip","non_lldp_cdp_connectivity":"probable"}`
    - Result:
      - `unlinked_count=0` (no unlinked actors in emitted topology graph).
      - `actors_unlinked_suppressed=1` in `data.stats`.
    - Specific endpoint check requested by user:
      - `10.20.4.15` is present as actor `mac:50:2c:c6:a6:fc:35`.
      - It is linked via one probable FDB edge (`metrics.inference=probable`, `attachment_mode=probable_segment`).
  - Files:
    - `src/go/pkg/topology/engine/topology_adapter.go`
    - `src/go/pkg/topology/engine/topology_adapter_test.go`
- [ ] A13. Investigate nondeterministic visibility/linking for `10.20.4.15` (reported "comes and goes", sometimes unlinked).
  - Facts to verify per-snapshot:
    - actor presence,
    - link presence,
    - link state/metrics (`probable` tagging),
    - snapshot timestamp correlation (`collected_at`).
  - Initial evidence (2026-02-24):
    - repeated local calls (`20` calls, `nodes_identity=ip`, `non_lldp_cdp_connectivity=probable`) showed:
      - `unlinked_count=0` in every sampled snapshot,
      - actor `10.20.4.15` present and linked in every sampled snapshot.
    - This indicates no immediate nondeterminism in sampled backend snapshots; user-reported behavior may be tied to mode/filter changes or data refresh boundaries.
- [ ] A14. Make probable/low-confidence link styling fully consistent.
  - Problem statement:
    - some low-confidence paths appear bright cyan while others are dark cyan.
  - Evidence:
    - frontend styles probable links dark cyan only when inferred from `metrics.inference=probable` or `metrics.attachment_mode=probable_*`:
      - `~/src/dashboard/netdata-cloud-frontend.git/src/domains/functions/components/graph/forceGraph.js:534`
      - `~/src/dashboard/netdata-cloud-frontend.git/src/domains/functions/components/graph/forceGraph.js:540`
      - `~/src/dashboard/netdata-cloud-frontend.git/src/domains/functions/components/graph/forceGraph.js:650`
    - backend currently emits some low-confidence bridge/overlap links without probable markers:
      - probable-only segment anchor bridge edge has no `state`/`inference` marker:
        - `src/go/pkg/topology/engine/topology_adapter.go:818`
        - `src/go/pkg/topology/engine/topology_adapter.go:826`
      - managed-overlap edge uses `attachment_mode=managed_device_overlap` only:
        - `src/go/pkg/topology/engine/topology_adapter.go:880`
        - `src/go/pkg/topology/engine/topology_adapter.go:890`
    - frontend normalized graph link details currently omit `state` (only link row keeps it):
      - `~/src/dashboard/netdata-cloud-frontend.git/src/domains/functions/topologyUtils.js:383`
      - `~/src/dashboard/netdata-cloud-frontend.git/src/domains/functions/topologyUtils.js:406`
- [ ] A15. Introduce `Map Type` selector and mode semantics (replace current `Non LLDP/CDP Connectivity`).
  - Requested UX labels/options:
    - `LLDP/CDP/Managed Devices Map`
    - `High Confidence Inferred Map`
    - `All Devices (Low Confidence)`
  - Requested behavior:
    - `LLDP/CDP/Managed Devices Map`: only LLDP/CDP inferred + managed devices view.
    - `High Confidence Inferred Map`: existing strict behavior, but hide unlinked inferred endpoints.
    - `All Devices (Low Confidence)`: existing probable behavior.
  - Existing implementation to replace:
    - backend param currently `non_lldp_cdp_connectivity` (`strict|probable`):
      - `src/go/plugin/go.d/collector/snmp/func_topology.go:27`
      - `src/go/plugin/go.d/collector/snmp/func_topology.go:49`
    - frontend button currently labeled `Non LLDP/CDP Connectivity`:
      - `~/src/dashboard/netdata-cloud-frontend.git/src/domains/functions/components/topology/index.js:488`
- [ ] A16. Graph geometry updates for large-port actors.
  - Requested:
    - link visual length should account for node radius growth (avoid links being visually "consumed" by enlarged actors),
    - actor label should be below actor, centered.
  - Current evidence:
    - links are rendered center-to-center:
      - `~/src/dashboard/netdata-cloud-frontend.git/src/domains/functions/components/graph/forceGraph.js:1774`
      - `~/src/dashboard/netdata-cloud-frontend.git/src/domains/functions/components/graph/forceGraph.js:1776`
    - label currently rendered to the right of node:
      - `~/src/dashboard/netdata-cloud-frontend.git/src/domains/functions/components/graph/forceGraph.js:1892`
- [ ] A17. Add backend function filters for very large infrastructures.
  - Requested new params:
    - `Managed SNMP Device Focus` (default: `All Devices`, options: `All Managed SNMP Devices`, `All Devices`)
    - `Depth` (default: `All`, options: `0..10`, `All`)
  - Required behavior:
    - build full topology in backend,
    - then send only subgraph within selected hop depth per focus mode.
- [ ] A18. Remove visible initial graph animation; present fully settled layout.
  - Current evidence:
    - after warmup ticks, simulation restarts and keeps animating:
      - `~/src/dashboard/netdata-cloud-frontend.git/src/domains/functions/components/graph/forceGraph.js:869`
      - `~/src/dashboard/netdata-cloud-frontend.git/src/domains/functions/components/graph/forceGraph.js:874`
- [x] A8. Remove `L2|L3|Merged` selection from topology function and UI.
  - Evidence:
    - backend function still exposes `topology_view` with `l2/l3/merged`:
      - `src/go/plugin/go.d/collector/snmp/func_topology.go:26`
      - `src/go/plugin/go.d/collector/snmp/func_topology.go:40`
    - current TODO scope is strict L2 only.
  - Required outcome:
    - `topology:snmp` returns strict L2 topology only,
    - no `topology_view` param in method schema or UI filters.
- [x] A9. Remove L3 protocol handling from SNMP topology pipeline.
  - Evidence:
    - function currently normalizes and serves `l3` and `merged` views:
      - `src/go/plugin/go.d/collector/snmp/func_topology.go:105`
      - `src/go/plugin/go.d/collector/snmp/func_topology.go:107`
  - Required outcome:
    - topology stack is L2-only (LLDP/CDP/BRIDGE/FDB/ARP/STP/VLAN enrichments),
    - no OSPF/ISIS topology view routing in this workstream path.
  - Implementation status (2026-02-24):
    - [x] `topology:snmp` no longer exposes `topology_view`; now exposes:
      - `nodes_identity=ip|mac` (default `ip`)
      - `non_lldp_cdp_connectivity=strict|probable` (default `probable`)
    - [x] topology registry removed L3/merged snapshot routes.
    - [x] topology autoprobe no longer appends OSPF/ISIS topology profiles.
    - [x] backend policy pass added:
      - collapse actors by IP (when `nodes_identity=ip`),
      - eliminate non-IP inferred actors (when `nodes_identity=ip`),
      - remove sparse segments (`<=1` participants after filtering).
    - [x] probable connectivity:
      - enabled only for non-LLDP/CDP inferred endpoints,
      - marks emitted links with `state=probable`, `metrics.inference=probable`, `metrics.confidence=low`.
    - [x] frontend graph:
      - replaced old 3 visibility toggles with:
        - `Nodes Identity: ip|mac`
        - `Non LLDP/CDP Connectivity: strict|probable`
      - probable links rendered with darker cyan.

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
- [x] D5. Enforce strict L2 product behavior:
  - remove `L2|L3|Merged` option from topology function/UI,
  - remove L3 topology protocols/views from this SNMP topology path,
  - this topology feature must be strictly L2.
- [x] D6. Probable connectivity semantics (critical fix):
  - `probable` must be layered on top of `strict` only,
  - it must not alter already linked endpoints from strict mode,
  - it must add exactly one probable link for each endpoint that is unlinked in strict mode (goal: no unlinked endpoints left),
  - it must not create extra segment links for endpoints already linked in strict mode.
- [x] D7. Probable fallback scope (2026-02-24):
  - probable assignment now applies to any endpoint still unlinked after strict pass (not limited by learned source labels),
  - result contract is still one probable candidate per unresolved endpoint and zero strict-link mutation.
- [x] D8. Probable mode graph delta contract (2026-02-24):
  - probable mode must be computed as a delta on top of strict output,
  - compared to strict, probable must only add the minimum segment/link additions required to connect strict-unlinked actors,
  - probable must not introduce additional segment paths between actors already connected in strict mode.
- [x] D9. Implement probable fallback for strict-unlinked endpoints without segment candidates (2026-02-24).
  - Accept deterministic backend fallback:
    - first use reporter/port hints to map to an existing segment when possible,
    - otherwise create a single `portless probable` attachment anchored to one managed reporter device.
  - Keep hard contract:
    - strict graph unchanged,
    - one probable attachment max per strict-unlinked endpoint,
    - probable markers always present for fallback links.

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
