# SOW-0004 - Palo Alto NGFW go.d collector (XML API), read-only telemetry suite

## Status

Status: completed

Sub-state: Implementation complete, fixture-backed validation complete, draft PR opened, live PAN-OS validation assigned to the SRE validation process outside this implementation SOW.

## Requirements

### Purpose

Add Palo Alto Networks NGFW (PA-Series, VM-Series, Panorama-managed) monitoring to Netdata's `go.d.plugin`, fit-for-purpose for DevOps/SREs running production edge/core firewalls. v1 ships **read-only PAN-OS XML API telemetry** under the `panos.*` namespace: BGP, system, HA, environment, licenses, and IPsec. The collector is a single `panos` module designed to grow into additional read-only metricsets (GlobalProtect, certificates, sessions, interfaces, dataplane CPU) only when fixture evidence or sanitized real XML exists.

### User Request

Add Palo Alto firewall monitoring to `go.d.plugin`. v1 must deliver BGP metrics (operator need from production deployments) and the fixture-backed read-only XML API telemetry set the user approved on 2026-05-02: system, HA, environment, licenses, and IPsec. Use the PAN-OS XML API as transport. Use the official `github.com/PaloAltoNetworks/pango` SDK. Support both legacy Virtual Router routing engine and Advanced Routing Engine (ARE) for BGP. Leave the existing Palo Alto SNMP profiles untouched. Use a single `panos` collector module covering all PA metricsets. The collector is **independent of the unified `bgp` collector (PR #22168)** — `panos` emits its own `panos.bgp.*` chart contexts, designed per the NIDL framework, with no shared model or chart code.

### Assistant Understanding

Facts:

- PAN-OS does **not implement BGP4-MIB** — confirmed by official Palo Alto KB `kA10g000000PNtlCAG` and absence from supported-MIBs list across PAN-OS 8.0 → 12.1. Only BGP signals over SNMP are state-change traps (`1.3.6.1.4.1.25461.2.1.3.2.0.{1531..1537}`), not pollable.
- Existing Netdata SNMP profiles for Palo Alto at `src/go/plugin/go.d/config/go.d/snmp.profiles/default/{palo-alto.yaml, _palo-alto.yaml, palo-alto-cloudgenix.yaml}` and `metadata/paloalto.yaml` cover sessions, GlobalProtect utilization, FRU/fan/power, model identification — these stay untouched.
- The unified `bgp` collector (PR #22168, currently open) and the SNMP-BGP work (PR #22170) cover Linux BGP daemons and SNMP-MIB-speaking devices respectively. PA falls into neither — it has its own XML API. The `panos` collector is an independent module; it does not reuse or extend either surface.
- PAN-OS XML API exposes per-peer BGP data: state, uptime, established-counts, flap-counts, msg-total-in/out, msg-update-in/out, per-AFI/SAFI prefix-counter (incoming-total/accepted/rejected, outgoing-advertised), peer-group, VR/logical-router, last-error, remote-as, peer/local addresses. Both legacy VR (`show routing protocol bgp ...`) and ARE (`show advanced-routing bgp ...`) command trees expose this data.
- Mirror evidence confirms read-only XML API command and fixture coverage for system info, HA state, environmentals, licenses, and IPsec SAs. Evidence sources: Centreon mockoon PAN-OS fixture, Centreon API modes, and Elastic PANW module command references.
- Official PAN-OS XML API docs confirm `type=op` performs operational mode commands and `type=keygen` generates API keys. Official docs also list configuration, commit, log, report, import, export, and user-id request types; these are intentionally out of scope for a Netdata metrics collector.
- PAN-OS XML API does NOT expose: per-message-subtype counters (Withdraws/Notifications/Keepalives/RouteRefresh as separate counters), full last-reset details (Hard/Soft, age), RPKI route-correctness valid/invalid/notfound per family, EVPN VNI metrics. The `panos.bgp.*` chart design simply does not include these — there are no charts for data PA cannot emit.
- pango Go SDK (`github.com/PaloAltoNetworks/pango`) is ISC-licensed, official, and used by Elastic Beats `panw` Metricbeat module; its `client.Op(query, vsys, extras, ans)` method runs op commands and returns parsed XML responses. PAN-OS 10.1+ supported.
- PAN-OS XML API has a documented soft limit of 5 concurrent connections per management plane.
- PAN-OS 10.2+ introduced ARE (FRR-based internally) which is the default on new firewalls; older estates are still legacy VR. Both must be supported.
- Reference Go implementation: `elastic/beats @ 19617a623ad9`, `x-pack/metricbeat/module/panw/routing/bgp_peers.go`, already does PA BGP via XML API + pango. Elastic fixtures are reference-only and must not be copied into this GPL tree.
- Reference auth pattern (non-SDK): Centreon `centreon-plugins/src/network/panos/api/custom/api.pm` — keygen, key cache, 401 auto-retry. pango handles keygen and key reuse, but not automatic unauthorized retry; Netdata's collector must wrap op calls and retry keygen once when pango reports PAN-OS error code 16 (`Unauthorized`) and username/password are configured.
- Closest go.d.plugin collector patterns: `activemq` (XML parsing + dynamic per-entity charts), `typesense` (API-key header auth), `nginxplus` (multi-endpoint scrape with discovery).

Inferences:

- Single-collector architecture: one `panos` module containing all supported PA metricsets, all under the `panos.*` chart-context namespace. v1 metricsets emit `panos.bgp.*`, `panos.system.*`, `panos.ha.*`, `panos.environment.*`, `panos.license.*`, and `panos.ipsec.*` contexts.
- Chart design follows the NIDL framework (`docs/NIDL-Framework.md`): one instance type per context, related dimensions only, hierarchical separation via separate contexts. Likely starting set for v1: `panos.bgp.peer.state`, `panos.bgp.peer.uptime`, `panos.bgp.peer.messages` (in/out), `panos.bgp.peer.updates` (in/out), `panos.bgp.peer.prefixes_received` (total/accepted/rejected), `panos.bgp.peer.prefixes_advertised`, `panos.bgp.peer.flaps`, `panos.bgp.peer.established_count`. Per-VR aggregates: `panos.bgp.vr.peers_by_state`. Final list locked during chart design phase.
- Polling cost on PA management plane is undocumented by Palo Alto. Conservative default `update_every: 60s`. Operators can override.
- HA pair: each member is an independent endpoint; passive responds. Configured per-job; no auto-pair-discovery in v1.
- Partial-failure behavior: enabled metricsets are collected serially. A metricset failure is returned/logged with area and command context, while successful metricsets still emit metrics. `Check()` succeeds when at least one enabled metricset returns metrics; it fails when all enabled metricsets fail or return no usable telemetry.
- Multi-VR / multi-logical-router: enumerate with `show virtual-router` (legacy) or `show advanced-routing logical-router` (ARE), then iterate BGP per VR. Routing-engine detection: probe legacy command first, fall back to ARE on empty/error.
- pango SDK BGP coverage may not include wrappers for every `show advanced-routing bgp ...` command; raw `client.Op()` with hand-built XML cmd strings is the fallback (same approach Elastic Beats uses).

Unknowns:

- Exact ARE XML response field names for `show advanced-routing bgp peer detail` — research confirmed legacy VR fields and official ARE CLI command availability; local mirrored repos did not contain an ARE BGP XML fixture. Resolution: implement parser defensively from documented command structure, use synthetic minimal ARE fixtures for unit coverage, then replace/supplement with sanitized real ARE output or provision VM-Series eval if Phase 4 reveals gaps.
- Exact XML variants for hardware sensors across PA hardware, VM-Series, and cloud deployments. Resolution: implement tolerant parser coverage from observed fixtures and treat unsupported/empty optional metricsets as non-fatal when other metricsets collect.
- Whether vsys-scoping affects BGP queries (multi-vsys + multi-VR interaction). Legacy docs imply BGP is per-VR not per-vsys. Verify with fixture or live device.

### Acceptance Criteria

- [x] `panos` go.d collector module exists at `src/go/plugin/go.d/collector/panos/` and is registered.
- [x] BGP metricset queries PAN-OS XML API on both legacy VR and ARE-mode command trees and auto-detects routing engine. Local validation uses synthetic XML fixtures because no sanitized ARE output was available.
- [x] BGP metrics emit under `panos.bgp.*` chart contexts. Each context follows NIDL discipline: one instance type, related dimensions, same unit, hierarchical separation via separate contexts.
- [x] Multi-VR / multi-logical-router identity is carried via `vr` labels. Local tests cover multiple legacy VRs and one synthetic ARE logical-router wrapper.
- [x] Charts populated only with data PAN-OS XML API actually exposes; no charts for unsupported fields (Withdraws/Notifications/Keepalives/RouteRefresh counters, EVPN VNI, RPKI correctness).
- [x] Auth supports both API key and username/password keygen via pango SDK, with key reuse and one local unauthorized retry when username/password are configured.
- [x] Default `update_every: 60s` (configurable). Requests are serial in v1, so concurrent connections to a single firewall are ≤ 1.
- [x] Tests: unit tests with local synthetic XML fixtures for both legacy VR and ARE shapes. Real/sanitized ARE fixture capture remains pending before final production confidence.
- [x] `metadata.yaml`, `config_schema.json`, `README.md`, stock config, integration doc, spec, and stock alerts under `panos.*` are present.
- [x] Existing SNMP profiles `palo-alto.yaml`, `_palo-alto.yaml`, `palo-alto-cloudgenix.yaml`, `metadata/paloalto.yaml` untouched.
- [x] Independent of PRs #22168 and #22170 — no shared code, no chart-context overlap, no merge-order coupling.
- [x] System metricset queries `<show><system><info></info></system></show>` and emits `panos.system.*` metrics for uptime and operational/certificate status.
- [x] HA metricset queries `<show><high-availability><state></state></high-availability></show>` and emits `panos.ha.*` metrics for enabled state, local/peer state, peer connection, link status, sync status, and priority.
- [x] Environment metricset queries `<show><system><environmentals></environmentals></system></show>` and emits `panos.environment.*` metrics for temperatures, fans, voltage rails, power supplies, and sensor alarms.
- [x] Licenses metricset queries `<request><license><info></info></license></request>` and emits `panos.license.*` metrics for license count, status, and expiration days.
- [x] IPsec metricset queries `<show><vpn><ipsec-sa></ipsec-sa></vpn></show>` and emits `panos.ipsec.*` metrics for active SAs and per-tunnel remaining SA lifetime.
- [x] Config supports enabling/disabling each metricset explicitly, with all six metricsets enabled by default.
- [x] Non-BGP optional metricset tests use Netdata-owned synthetic XML fixtures derived from observed fixture shape, without copying third-party fixture payloads.
- [x] Docs, metadata, schema, stock config, alerts, and spec are expanded beyond BGP and stay consistent with code.

## Analysis

Sources checked:

- `docs/NIDL-Framework.md` (chart design discipline)
- `src/go/plugin/go.d/config/go.d/snmp.profiles/default/{palo-alto.yaml, _palo-alto.yaml, palo-alto-cloudgenix.yaml, _std-bgp4-mib.yaml}`
- `src/go/plugin/go.d/config/go.d/snmp.profiles/metadata/paloalto.yaml`
- `elastic/beats @ 19617a623ad9`, `x-pack/metricbeat/module/panw/`
- `centreon/centreon-plugins @ a4f99c776351`, `src/network/panos/api/`
- Datadog `palo-alto.yaml` SNMP profile, LibreNMS `panos.yaml`, Prometheus snmp_exporter `panos_fw` block
- Official PAN-OS docs (XML API request types, Supported MIBs PAN-OS 10.1/11.1, OpenConfig support, REST API scope, NetFlow templates, syslog formats)
- Official KBs `kA10g000000PNtlCAG` (BGP via SNMP not supported), `kA14u000000CqJjCAK` (BGP traps), `kA14u0000008XQ2CAM` (CLI commands and prefix-counter fields)
- PRs #22168 (native BGP daemon collector) and #22170 (SNMP BGP) — read for context only; this SOW does not depend on them.
- go.d collector reference patterns: `activemq`, `typesense`, `nginxplus`

Current state:

- `panos` collector module now exists in `src/go/plugin/go.d/collector/` and is registered.
- Unified `bgp` collector (#22168) is open, not merged. Its model/charts/alerts live inside the `bgp` package — not yet a shared package.
- SNMP profile coverage for PA exists but does not include BGP (intentionally — PAN-OS does not implement BGP4-MIB).

Risks:

- **R1 — ARE vs legacy routing engine.** Different command trees, possibly different XML field names. Mitigation: detect via probe (legacy first, fallback to ARE). Capture fixtures for both.
- **R2 — No live PAN-OS test environment.** Mitigation: use upstream XML only as reference, create minimal synthetic fixtures for parser coverage, supplement with customer-provided sanitized output. Provision VM-Series eval if fixtures insufficient.
- **R3 — pango SDK BGP coverage/auth gap.** SDK is config-tree-focused; op commands work via raw `client.Op()` but BGP-specific helpers may be absent. pango handles keygen but not unauthorized retry. Mitigation: hand-build XML cmd strings, mirror Elastic Beats approach, and wrap op calls with one local key refresh retry when possible.
- **R4 — Polling cost on PAN-OS management plane.** No published benchmark. Mitigation: conservative `update_every: 60s` default, ≤ 2 concurrent connections per job, document the 5-conn limit in README.
- **R5 — Multi-VR / multi-vsys interaction.** Mitigation: enumerate VRs explicitly per scrape, treat vsys as a config knob (not auto-discovery), document constraints.
- **R6 — NIDL chart design errors.** Mixing instance types, mixing units, missing per-instance dimensions, or wrong hierarchy collapses dashboard usability. Mitigation: design pass against `docs/NIDL-Framework.md` checklist before implementation; review chart list with user before coding.

## Pre-Implementation Gate

Status: ready

Problem / root-cause model:

- Production operators need BGP metrics from Palo Alto NGFW. PAN-OS does not implement BGP4-MIB (confirmed). The PAN-OS XML API is the only viable interface. Operators want one collector module per device type — a single `panos` collector that grows over time, all under the `panos.*` chart-context namespace. The collector is independent of other Netdata BGP work; chart design follows the NIDL framework.

Evidence reviewed:

- See Analysis → Sources checked.

Affected contracts and surfaces:

- New: `src/go/plugin/go.d/collector/panos/` (module dir).
- New: `panos.bgp.*`, `panos.system.*`, `panos.ha.*`, `panos.environment.*`, `panos.license.*`, and `panos.ipsec.*` chart contexts (NIDL-compliant; final list locked before implementation in the 2026-05-02 expansion log).
- New/updated: `panos/README.md`, `metadata.yaml`, `config_schema.json`, integrations/, stock alerts under `panos.*`.
- New (Go module dependency): `github.com/PaloAltoNetworks/pango` added to `src/go/go.mod` / `go.sum`.
- Untouched: `src/go/plugin/go.d/config/go.d/snmp.profiles/{default,metadata}/*palo*.yaml`, the unified `bgp` collector (#22168), the SNMP-BGP work (#22170).

Existing patterns to reuse:

- NIDL chart design: `docs/NIDL-Framework.md` (one instance type per context, related dimensions, hierarchical separation, thoughtful labels).
- pango SDK `client.Op()` pattern (Elastic Beats `panw/client.go`), with local unauthorized retry wrapper.
- Per-area Fetch functions returning errors with `errors.Join` (Elastic `eventFetchers []struct{name; fn}`).
- Golden XML fixture testing pattern. Upstream fixtures are reference-only unless license-compatible; Netdata tests should use synthetic or sanitized real XML.
- go.d collector layout: `collector.go` + `init.go` + `collect.go` + `charts.go` + `apiclient.go` + `metadata.yaml` + `config_schema.json` + `testdata/` + `integrations/`.
- API-key header auth: `typesense` collector pattern.
- Multi-endpoint discovery + dynamic dimensions: `nginxplus` + `activemq`.

Risk and blast radius:

- See R1–R6 in Analysis.
- Blast radius low: new collector, no existing module modified. SNMP profile work explicitly out of scope. Independent of #22168 / #22170.

Implementation plan:

1. **NIDL chart design pass.** Lock the v1 `panos.*` chart context list, with instance type, dimensions, units, labels per context. Verify against the NIDL checklist.
2. **Scaffold** `src/go/plugin/go.d/collector/panos/` with module skeleton, registration, config, init, basic Check.
3. **XML API client wrapper** using pango SDK (`client.Op()`); routing-engine detection probe; VR/LR enumeration helper; one unauthorized key-refresh retry when username/password are configured.
4. **Metricsets** implementation: BGP plus system, HA, environment, licenses, and IPsec; XML→domain mapping; `panos.*` chart emit per locked design.
5. **Tests** with local XML fixtures. Synthetic fixtures are acceptable until sanitized real output is available; third-party fixture payloads are reference-only.
6. **metadata.yaml, config_schema.json, README, stock alerts** under `panos.*`, integration listing.
7. **Manual validation** against a live firewall (eval VM-Series if needed).
8. **PR open** with `needs review`; cubic/copilot review loop per the `pr-reviews` skill.

Validation plan:

- Unit tests with local XML fixtures (legacy VR + synthetic ARE) for: routing-engine detect, VR/logical-router identity, BGP peer parse, XML→domain mapping.
- `go test ./plugin/go.d/collector/panos/...` clean.
- Manual: point at a live PA firewall; verify charts emit under `panos.bgp.*` contexts; spot-check NIDL compliance (instance dropdown shows expected entities, dimensions are related and same-unit per chart).
- Same-failure scan: confirm no chart-context overlap with `bgp.*` (PR #22168) or `snmp.bgp.*` (PR #22170).
- Reviewer findings: cubic + copilot review on PR; address per `pr-reviews` skill.

Artifact impact plan:

- AGENTS.md: no update expected — SOW is project-internal, framework rules already cover this work.
- Runtime project skills: no update expected — no new repeatable workflow introduced.
- Specs: new spec at `.agents/sow/specs/panos-collector.md` documenting the collector's scope, supported PAN-OS versions, routing-engine matrix, polling-cost defaults, chart-context contract.
- End-user/operator docs: new `src/go/plugin/go.d/collector/panos/README.md` and `metadata.yaml` covering setup (API key generation, least-privilege admin role), configuration, troubleshooting.
- End-user/operator skills: none expected. PA is not in `docs/netdata-ai/skills/` today.
- SOW lifecycle: this SOW now covers the fixture-backed read-only telemetry set: BGP, system, HA, environment, licenses, and IPsec. Future PA metricsets without XML fixtures or better covered by SNMP (GlobalProtect, certificates, sessions, interfaces, dataplane CPU) become separate SOWs.

Open decisions: all resolved.

- **D7 = A** — Use pango SDK as documented (PAN-OS 10.1+ minimum). pango handles keygen/key reuse/transport; raw `client.Op()` for BGP show commands; Netdata adds unauthorized retry wrapper.
- **D8 = B** — Provision VM-Series eval only if Phase 4 testing reveals fixture gaps. Default path: fixtures only.
- **D9 = B** — Panorama proxy deferred to a follow-up SOW. v1 = direct-to-firewall jobs.
- **D10 = B** — No chart-design review gate. Commit a NIDL-compliant draft, adjust during implementation if real issues surface.

## Implications And Decisions

User decisions captured:

1. **Scope** → **Option 2** (broader `panos` XML API collector, ship BGP first, expand later).
2. **SDK vs custom HTTP client** → **A: pango SDK** (`github.com/PaloAltoNetworks/pango`, ISC license).
3. **Routing engine support in v1** → **A: Both legacy VR and ARE.**
4. **SNMP profile change** → **A: Leave existing `palo-alto.yaml` SNMP profile untouched.**
5. **PAN-OS access for development/testing** → **C primary, B fallback** (use upstream/customer XML response fixtures; provision VM-Series eval only if fixtures insufficient).
6. **SOW workflow** → **A: Open SOW now**, fill Pre-Implementation Gate, then implement.
7. **Architecture for PA BGP** → **Single `panos` collector emitting its own `panos.bgp.*` chart contexts and `panos.<area>.*` for additional metricsets, NIDL-compliant. Independent of PRs #22168 and #22170 — no shared code, no chart-context overlap.**
8. **pango SDK version pinning** → **A: pango current; document min PAN-OS = 10.1.**
9. **Eval firewall trigger** → **B: provision only if Phase 4 reveals fixture gaps.**
10. **Panorama proxy in v1** → **B: deferred to follow-up SOW.**
11. **NIDL chart-design review gate** → **B: no gate; commit draft and adjust during implementation.**
12. **2026-05-02 telemetry expansion** → **Implement the fixture-backed read-only XML API metricsets now: system, HA, environment, licenses, and IPsec. Exclude GlobalProtect, certificates, sessions, interfaces, arbitrary XML command passthrough, and all config/commit/log/report/import/export/user-id operations from this SOW.**
13. **Metricset selection** → **All six metricsets enabled by default, with explicit per-metricset booleans so operators can disable unsupported/noisy areas on specific firewalls.**
14. **Partial failures** → **Collect enabled metricsets independently. Preserve successful metrics when one metricset fails; include metricset name and XML command context in errors.**
15. **Cardinality controls** → **Option 1.B approved.** Add default caps high enough for average installations while protecting Netdata from runaway per-entity chart creation: `max_bgp_peers: 512`, `max_bgp_prefix_families_per_peer: 4`, `max_bgp_virtual_routers: 256`, `max_environment_sensors: 512`, `max_licenses: 64`, and `max_ipsec_tunnels: 1024`. Caps must be paired with selectors, must report discovered/monitored/omitted counts where practical, and must not replace stale-chart obsoletion; obsoletion remains independent.
16. **SOW closure / live-device validation** → **Close this implementation SOW after fixture-backed implementation and draft PR creation.** On 2026-05-02 the user confirmed that live PAN-OS validation is handled by the SRE validation process, not by this implementation SOW. The SOW records the local validation gap honestly, but it is no longer a blocker for SOW closure.

## Plan

1. **Phase 0 — Resolve open decisions** (D7–D10 above).
2. **Phase 1 — NIDL chart design.** Draft v1 `panos.*` chart context list (instance type, dimensions, unit, labels, family) per `docs/NIDL-Framework.md`.
3. **Phase 2 — Module scaffold + auth.** New `panos` collector skeleton; pango SDK wiring; routing-engine probe; least-privilege admin-role docs.
4. **Phase 3 — Metricsets.** Existing BGP plus system, HA, environment, licenses, and IPsec; XML→domain mapping; `panos.*` chart emit per locked design.
5. **Phase 4 — Tests + fixtures.** Golden XML files for every metricset; unit-test parsers; integration test against live firewall if available.
6. **Phase 5 — Docs + alerts + integration listing.** README, metadata.yaml, config_schema.json, stock alerts under `panos.*`.
7. **Phase 6 — PR review loop.** Open PR; cubic/copilot review; address findings per `pr-reviews` skill.
8. **Phase 7 — Spec and follow-up mapping.** Update spec at `.agents/sow/specs/panos-collector.md`. Track remaining metricsets only where fixture evidence is missing or SNMP is the better surface.

## Execution Log

### 2026-05-01

- SOW created.
- Research completed via 4 parallel agents: PAN-OS monitoring interfaces (broad), mirrored-repos survey, go.d.plugin collector patterns, focused PAN-OS BGP MIB and XML API verification.
- Confirmed: BGP4-MIB not implemented on PAN-OS (KB `kA10g000000PNtlCAG`); only XML API path viable for BGP polling.
- User decisions 1–7 captured; D7–D10 still open.

### 2026-05-02

- Architecture clarified: `panos` collector is fully independent from PRs #22168 and #22170. Charts emit under `panos.bgp.*` (and future `panos.<area>.*`), NIDL-compliant. No shared package, no chart-context overlap with the unified `bgp` collector.
- Removed all references to `pkg/bgp/` extraction and chart-context coordination with #22168. Replaced with a Phase 1 NIDL chart design pass.
- Decisions D7=A, D8=B, D9=B, D10=B captured. Pre-Implementation Gate moved to `ready`. SOW moved from `pending/` to `current/`, status `in-progress`.
- Module renamed from `paloalto` to `panos` (matches PAN-OS naming; aligns with upstream tooling). All chart contexts now `panos.bgp.*`. SOW file renamed to `SOW-0004-20260501-panos-collector.md`.
- User approved implementation after review. Corrected implementation facts before coding: pango latest is `v0.10.2`, ISC-licensed, supports `client.Op()` and keygen/key reuse, but does not auto-retry unauthorized responses. Netdata must implement one local unauthorized retry. Elastic BGP XML is reference-only and not copied. Local mirrors do not contain ARE BGP XML fixtures, so initial ARE tests use synthetic minimal fixtures until sanitized real output is available.
- **Phase 1 — NIDL chart draft (v1, BGP metricset).** Per `docs/NIDL-Framework.md`. Adjust if real issues surface during implementation.

  Family: `bgp` (operator-facing dashboard family).

  Per-peer contexts. Instance identity = `(vr_or_logical_router, peer_address)`. Labels on every per-peer chart: `vr`, `peer_address`, `local_address`, `remote_as`, `peer_group`.

  | Context | Dimensions | Unit | Algorithm | Notes |
  |---|---|---|---|---|
  | `panos.bgp.peer.state` | `idle`, `connect`, `active`, `opensent`, `openconfirm`, `established` | `state` (boolean per dim, exactly one =1) | absolute | FSM state as one-hot |
  | `panos.bgp.peer.uptime` | `uptime` | `seconds` | absolute | from `status-duration` |
  | `panos.bgp.peer.messages` | `in`, `out` | `messages/s` | incremental | from `msg-total-in/out` |
  | `panos.bgp.peer.updates` | `in`, `out` | `messages/s` | incremental | from `msg-update-in/out` |
  | `panos.bgp.peer.flaps` | `flaps` | `flaps/s` | incremental | from `status-flap-counts` |
  | `panos.bgp.peer.established_transitions` | `established` | `transitions/s` | incremental | from `established-counts` |

  Per-peer-per-family contexts. Instance identity = `(vr, peer_address, afi, safi)`. Labels add `afi`, `safi`.

  | Context | Dimensions | Unit | Algorithm | Notes |
  |---|---|---|---|---|
  | `panos.bgp.peer.prefixes_received` | `total`, `accepted`, `rejected` | `prefixes` | absolute | from `prefix-counter.incoming-*` |
  | `panos.bgp.peer.prefixes_advertised` | `advertised` | `prefixes` | absolute | from `prefix-counter.outgoing-advertised` |

  Per-VR aggregate contexts. Instance identity = `(vr_or_logical_router)`. Labels: `vr`.

  | Context | Dimensions | Unit | Algorithm | Notes |
  |---|---|---|---|---|
  | `panos.bgp.vr.peers_by_state` | `idle`, `connect`, `active`, `opensent`, `openconfirm`, `established` | `peers` | absolute | counts of peers in each state per VR |
  | `panos.bgp.vr.peers_total` | `configured`, `established` | `peers` | absolute | health summary per VR |

  Excluded from v1 (PAN-OS XML API does not expose):
  - Withdraws/Notifications/Keepalives/RouteRefresh as separate counters
  - HasResetState details (Hard/Soft, age)
  - RPKI route correctness (valid/invalid/notfound)
  - EVPN VNI metrics
  - RIB route counts per family (deferred — `show ... bgp loc-rib` polling cost unverified)

  Diagnostics not as time-series (consider a Netdata function in a follow-up SOW): `last-error`, `peer-router-id`, negotiated `holdtime` / `keepalive`.

- **Phase 2–5 local implementation completed.**
  - Added collector code under `src/go/plugin/go.d/collector/panos/`.
  - Added pango dependency in `src/go/go.mod` / `src/go/go.sum`.
  - Registered the collector in `src/go/plugin/go.d/collector/init.go`.
  - Added stock config `src/go/plugin/go.d/config/go.d/panos.conf`.
  - Added stock alert `src/health/health.d/panos.conf`.
  - Added docs/artifacts: `README.md`, `metadata.yaml`, `config_schema.json`, `integrations/palo_alto_panos.md`.
  - Added spec `.agents/sow/specs/panos-collector.md`.
  - Added local test fixtures and tests for legacy VR, ARE fallback, dynamic charts, config serialization, and unauthorized key-refresh retry.
- User requested implementing the consensus reviewer improvements from Claude, GLM, MiniMax, Qwen, and Kimi. Accepted scope for the next pass:
  - Add first-success PAN-OS system-info diagnostics.
  - Add routing-engine detection logging and richer command/error context.
  - Add explicit no-BGP cache/reprobe behavior to avoid repeated probe storms.
  - Add grace before removing stale peer/prefix/VR charts.
  - Harden unauthorized retry handling where possible.
  - Add parser/helper/error tests and more realistic edge fixtures.
  - Keep real/sanitized ARE XML capture as the SOW close gate.
- Reviewer-driven hardening implemented:
  - Added one-time PAN-OS system-info logging after successful pango initialization, including hostname/model/software version/serial/HA state when reported.
  - Added routing-engine detection logs and command-context wrapping for BGP query failures.
  - Added nested PAN-OS `<msg><line>...</line></msg>` error parsing so API error details are preserved.
  - Added sanitized API-key error wrapping to avoid logging `key=...` values from SDK errors.
  - Added explicit no-BGP cache/reprobe behavior: if all probes succeed empty, skip full re-probing for 5 minutes.
  - Added stale chart grace: peer, prefix, and VR charts are removed only after 3 consecutive missing collections.
  - Capped the pango HTTP transport to 2 connections per firewall job.
  - Rejected embedded URL credentials and unmatched `tls_cert` / `tls_key`.
  - Added parser/helper tests, URL parser tests, nested PAN-OS error tests, no-BGP cache tests, chart-grace tests, and auth-refresh failure sanitization tests.
  - Updated README, metadata, and spec for diagnostics, connection behavior, no-BGP troubleshooting, and TLS guidance.
- Mirrored-repo fixture search completed:
  - Found Elastic Beats PANW Metricbeat legacy BGP fixture in `elastic/beats @ 19617a623ad9`, `x-pack/metricbeat/module/panw/_meta/testdata/bgp_peers.xml`.
  - Found Elastic Beats legacy BGP command and parser in `elastic/beats @ 19617a623ad9`, `x-pack/metricbeat/module/panw/routing/bgp_peers.go`.
  - Found Zabbix dynamic-routing-by-HTTP template using the same legacy BGP XML API command and expected fields in `zabbix/community-templates @ 48feaf2f785d`, `Network_Devices/Palo_Alto/template_palo_alto_firewall_dynamic_routing_by_http/6.4/template_palo_alto_firewall_dynamic_routing_by_http.yaml`.
  - Found Centreon PAN-OS XML API fixtures for keygen, HA, environmentals, licenses, system info, and IPsec in `centreon/centreon-plugins @ a4f99c776351`, `tests/network/paloalto/api/mockoon-paloalto-api.json`.
  - Did not find any Advanced Routing Engine BGP XML fixture or command evidence in the scoped PAN-OS-related mirror search. The ARE close gate remains open.
- Legacy BGP fixture hardening implemented after mirror search:
  - Enriched Netdata-owned synthetic fixture `src/go/plugin/go.d/collector/panos/testdata/legacy_bgp_peers.xml` to follow the observed legacy PAN-OS shape where `entry peer` is a peer name and `<peer-address>` carries the actual peer address.
  - Added synthetic fields observed in external fixtures/templates: `peer-router-id`, `password-set`, `passive`, `multi-hop-ttl`, BGP timers, `last-error`, peer capabilities, `policy-rejected`, and `outgoing-total`.
  - Added parser assertions proving Netdata uses `<peer-address>` for metric identity while preserving peer group, local address, remote AS, state, and prefix counters.
  - Did not copy Elastic fixture data verbatim due to license risk; only used the public shape evidence to improve a Netdata-owned synthetic test artifact.
- User approved expanding this SOW from BGP-only to the read-only PAN-OS XML API telemetry set with fixture evidence: system, HA, environment, licenses, and IPsec. The following remain excluded from this SOW: GlobalProtect, certificates, sessions, interfaces, arbitrary XML command passthrough, config, commit, log, report, import, export, and user-id operations.
- Official documentation re-check before expansion:
  - PAN-OS XML API request types page confirms `type=keygen` for API keys and `type=op` for operational mode commands; it also lists config, commit, report, log, import, export, user-id, and version request types, validating the explicit read-only metric collector boundary.
  - PAN-OS operational mode API page confirms API callers can run CLI operational commands as XML bodies using `type=op&cmd=<xml-body>`, and points operators to the API Browser for command syntax.
- Mirror evidence for expanded metricsets:
  - Centreon commands and fixture shapes exist for system, HA, environment, licenses, and IPsec in `centreon/centreon-plugins @ a4f99c776351`, `src/network/paloalto/api/mode/` and `tests/network/paloalto/api/mockoon-paloalto-api.json`.
  - Elastic PANW command evidence exists for environment components and other system areas in `elastic/beats @ 19617a623ad9`, `x-pack/metricbeat/module/panw/system/`.
- Expanded NIDL chart draft:
  - `panos.system.uptime`: global device uptime in seconds.
  - `panos.system.device_certificate_status`: global one-hot certificate status (`valid`, `invalid`).
  - `panos.system.operational_mode`: global one-hot operational mode (`normal`, `other`).
  - `panos.ha.enabled`: global HA enabled status (`enabled`).
  - `panos.ha.local.state` and `panos.ha.peer.state`: one-hot HA state (`active`, `passive`, `non_functional`, `suspended`, `unknown`).
  - `panos.ha.peer.connection_status`: peer connection status (`up`).
  - `panos.ha.state_sync`: state synchronization status (`synchronized`).
  - `panos.ha.links_status`: HA link status (`ha1`, `ha1_backup`, `ha2`, `ha2_backup`).
  - `panos.ha.priority`: local and peer priority values.
  - `panos.environment.temperature`: per temperature sensor, Celsius, labels `slot`, `sensor`, `sensor_type`.
  - `panos.environment.fan_speed`: per fan sensor, RPM, labels `slot`, `sensor`, `sensor_type`.
  - `panos.environment.voltage`: per voltage rail, volts, labels `slot`, `sensor`, `sensor_type`.
  - `panos.environment.sensor_alarm`: per environment sensor alarm status, labels `slot`, `sensor`, `sensor_type`.
  - `panos.environment.power_supply_status`: per PSU inserted/alarm status, labels `slot`, `sensor`, `sensor_type`.
  - `panos.license.count`: global license count (`total`, `expired`).
  - `panos.license.status`: per license one-hot status (`valid`, `expired`), labels `feature`, `description`.
  - `panos.license.time_until_expiration`: per license days until expiration, labels `feature`, `description`; `-1` means PAN-OS reports `Never`. Unparseable dates are reported as collection errors with the license name and raw value.
  - `panos.ipsec.tunnels`: global active IPsec SA count.
  - `panos.ipsec.tunnel.sa_lifetime`: per tunnel seconds until current SA expiry, labels `tunnel`, `gateway`, `remote`, `tunnel_id`, `protocol`, `encryption`.
  - BGP chart contexts remain as already implemented.
- Expanded metricset implementation completed locally:
  - Added `src/go/plugin/go.d/collector/panos/telemetry.go` with read-only XML API parsers/collectors for system, HA, environment, licenses, and IPsec.
  - Added metricset enable/disable booleans in config with all six metricsets enabled by default.
  - Changed collection to keep successful metricsets when another enabled metricset fails; errors now include metricset and command context.
  - Added dynamic charts for `panos.system.*`, `panos.ha.*`, `panos.environment.*`, `panos.license.*`, and `panos.ipsec.*`.
  - Added Netdata-owned synthetic fixtures for system, HA, environment, licenses, and IPsec under `src/go/plugin/go.d/collector/panos/testdata/`.
  - Expanded tests, metadata, README, config schema, stock config, integration doc, health alerts, and spec.
- Troubleshooting/error-handling review completed after the user's request to inspect all error conditions and silent failures:
  - Rechecked official PAN-OS XML API error-code behavior and pango's parser. Confirmed PAN-OS error details may appear in top-level `<msg><line>...` and nested `<result><msg>...` forms; the collector now preserves both.
  - Rechecked mirrored repos for PAN-OS error handling. No stronger open-source telemetry error-handling pattern was found than pango's parser behavior; pango itself treats `status="error"`/`status="failed"` and non-success codes as errors and preserves result-level messages.
  - Replaced silent missing/malformed integer/decimal/duration parsing with strict field-aware errors for values that back emitted charts. The collector no longer turns absent or malformed PAN-OS telemetry values into `0`.
  - Changed unrecognized license expiration dates from fake `-1` to explicit errors. `-1` now only means PAN-OS reported `Never`.
  - Changed unexpected empty/unknown successful XML payloads for enabled metricsets into explicit errors naming the expected XML section.
  - Kept partial-success semantics: valid charts/metrics continue to emit while missing/malformed per-entity environment, license, and IPsec values are reported and their unsafe value charts are omitted.
  - Expanded unauthorized/session-expired retry detection to cover PAN-OS code 22 and 403-style forbidden errors, and redacts password/pass query parameters in SDK errors.
  - Updated README, integration doc, metadata, and spec for the improved troubleshooting behavior.
- User approved cardinality default option 1.B:
  - `max_bgp_peers: 512`
  - `max_bgp_prefix_families_per_peer: 4`
  - `max_bgp_virtual_routers: 256`
  - `max_environment_sensors: 512`
  - `max_licenses: 64`
  - `max_ipsec_tunnels: 1024`
  - Caps must have selectors, must not silently truncate without troubleshooting signals, and stale-chart obsoletion remains independent of cardinality caps.
- Cardinality controls and final troubleshooting hardening implemented:
  - Added caps and selectors for BGP peers, BGP prefix families, BGP virtual routers, environment sensors, licenses, and IPsec tunnels.
  - Added collection metrics with `discovered`, `monitored`, `omitted_by_selector`, and `omitted_by_limit` dimensions for each capped entity family.
  - Added once-per-condition cap-hit warnings that name the cap and selector to adjust.
  - Kept summary metrics complete where possible: BGP VR aggregates use all parsed peers, license totals use all parsed licenses, and IPsec active tunnel count uses PAN-OS `<ntun>` when available.
  - Reused the per-cycle metrics map and dynamic-chart seen map to avoid avoidable hot-path allocations.
  - Hardened status parsing so missing/invalid environment alarm, power-supply inserted/alarm, HA enabled, and license expired-status fields no longer become silent false/valid metrics.
  - Added explicit IPsec `<ntun>` versus `<entry>` mismatch errors so users can distinguish complete active-tunnel counts from incomplete per-tunnel lifetime details.
  - Added tests for caps/selectors, dynamic-chart obsoletion after selector exclusion, invalid config caps/selectors, invalid license status, and IPsec `<ntun>` mismatch behavior.
- Project collector-writing skill review identified one required framework correction: new go.d collectors must use CollectorV2, not V1.
- CollectorV2 migration completed:
  - Changed `panos` registration from `Create` to `CreateV2`.
  - Added `src/go/plugin/go.d/collector/panos/charts.yaml` as the runtime chart template.
  - Added a metrix store bridge that maps the existing fixture-tested collection output into CollectorV2 metric series.
  - Preserved partial-success behavior: if at least one enabled metricset emits metrics, CollectorV2 `Collect()` logs any partial error but returns success so go.d does not abort the cycle and discard good metrics.
  - Added chart-template schema/semantic compile tests and V2 metrix-store tests for BGP peer/prefix metrics and IPsec tunnel identity including `tunnel_id`.
  - Added `tunnel_id` as a per-IPsec-tunnel chart label and updated README, metadata, spec, and SOW label descriptions.
- SOW closure decision captured:
  - User confirmed that live PAN-OS validation is handled by the SRE validation process outside this implementation SOW.
  - Draft PR #22389 was opened for the implementation.
  - SOW can close as implementation-complete with fixture-backed validation and the live-device validation gap documented.
- Closure hygiene after updating to latest `master`:
  - Replaced local mirror absolute-path evidence with durable `owner/repo @ commit` citations.
  - `.agents/sow/audit.sh` reports SOW-0004 status/directory consistency and mirror evidence as OK.
  - `.agents/sow/audit.sh` still reports unrelated AGENTS.md sensitive-data section warnings introduced by the newer SOW framework; those are project-framework hygiene, not PAN-OS collector implementation debt.

## Validation

Local validation completed:

- `go test -count=1 ./plugin/go.d/collector/panos` from `src/go` — pass.
- `go test -count=1 ./plugin/go.d/collector/panos` from `src/go` after troubleshooting/error-hardening pass — pass.
- `go test -count=1 ./plugin/go.d/collector/panos` from `src/go` after cardinality/status/IPsec mismatch hardening — pass.
- `go test -count=1 ./plugin/go.d/collector/panos` from `src/go` after CollectorV2 migration — pass.
- `go test -count=1 ./plugin/go.d/collector` from `src/go` — pass, validates collector registry import compiles.
- `go test ./plugin/go.d/pkg/collecttest` from `src/go` — pass.
- `go test -count=1 ./plugin/go.d/collector` from `src/go` after troubleshooting/error-hardening pass — pass.
- `go test -count=1 ./plugin/go.d/pkg/collecttest` from `src/go` after troubleshooting/error-hardening pass — pass.
- `go test -count=1 ./plugin/go.d/collector` from `src/go` after cardinality/status/IPsec mismatch hardening — pass.
- `go test -count=1 ./plugin/go.d/pkg/collecttest` from `src/go` after cardinality/status/IPsec mismatch hardening — pass.
- `go test -count=1 ./plugin/go.d/collector` from `src/go` after CollectorV2 migration — pass.
- `go test -count=1 ./plugin/go.d/pkg/collecttest` from `src/go` after CollectorV2 migration — pass.
- YAML parse check with Ruby for `src/go/plugin/go.d/collector/panos/metadata.yaml` and `src/go/plugin/go.d/collector/panos/testdata/config.yaml` — pass.
- JSON parse check with Ruby for `src/go/plugin/go.d/collector/panos/config_schema.json` and `src/go/plugin/go.d/collector/panos/testdata/config.json` — pass.
- XML parse check with Ruby for all PAN-OS test fixtures under `src/go/plugin/go.d/collector/panos/testdata/` — pass.
- SNMP profile diff check for `src/go/plugin/go.d/config/go.d/snmp.profiles/default/{palo-alto.yaml,_palo-alto.yaml,palo-alto-cloudgenix.yaml}` and `src/go/plugin/go.d/config/go.d/snmp.profiles/metadata/paloalto.yaml` — clean; untouched.
- `.agents/sow/audit.sh` — pass.
- `git diff --check` — pass.
- Personal-name scan over SOW/spec/collector/config/health artifacts — clean; no matches.
- YAML/JSON/XML parse checks after troubleshooting/error-hardening pass — pass.
- YAML/JSON/XML parse checks after cardinality/status/IPsec mismatch hardening — pass.
- YAML/JSON/XML parse checks after CollectorV2 migration — pass.
- `.agents/sow/audit.sh` after troubleshooting/error-hardening pass — pass.
- `git diff --check` after troubleshooting/error-hardening pass — pass.
- Personal-name scan after troubleshooting/error-hardening pass — clean; no matches.
- `.agents/sow/audit.sh` after cardinality/status/IPsec mismatch hardening — pass.
- `git diff --check` after cardinality/status/IPsec mismatch hardening — pass.
- Personal-name scan after cardinality/status/IPsec mismatch hardening — clean; no matches.
- Tab-character scan over PAN-OS metadata, integration doc, spec, and SOW after cardinality/status/IPsec mismatch hardening — clean.
- `.agents/sow/audit.sh` after CollectorV2 migration — pass.
- `git diff --check` after CollectorV2 migration — pass.
- Personal-name scan after CollectorV2 migration — clean; no matches.
- Tab-character scan over PAN-OS metadata, config schema, chart template, README, stock config, stock alert, spec, and SOW after CollectorV2 migration — clean.
- SNMP profile diff check after CollectorV2 migration — clean; untouched.

Acceptance evidence:

- Collector module and registration: `src/go/plugin/go.d/collector/panos/collector.go`, `src/go/plugin/go.d/collector/init.go`.
- CollectorV2 runtime contract: `src/go/plugin/go.d/collector/panos/collector.go`, `src/go/plugin/go.d/collector/panos/charts.yaml`, `src/go/plugin/go.d/collector/panos/metrix.go`, `TestCollector_ChartTemplateYAML`, `TestCollector_CollectV2WritesMetricStore`, `TestCollector_CollectV2WritesIPSecTunnelIdentity`.
- pango auth/transport and unauthorized retry: `src/go/plugin/go.d/collector/panos/apiclient.go`, `TestPangoAPIClient_OpRefreshesAPIKeyOnceOnUnauthorized`.
- pango initialization/key-refresh behavior and error sanitization: `src/go/plugin/go.d/collector/panos/apiclient.go`, `TestPangoAPIClient_OpRefreshesAPIKeyWhenInitializeFindsExpiredKey`, `TestPangoAPIClient_OpResetsInitializationWhenRefreshFails`.
- Legacy and ARE command-tree probing: `src/go/plugin/go.d/collector/panos/bgp.go`, `TestCollector_Collect_AdvancedBGPFallback`.
- External legacy BGP shape evidence: `elastic/beats @ 19617a623ad9`, `x-pack/metricbeat/module/panw/_meta/testdata/bgp_peers.xml`, and `zabbix/community-templates @ 48feaf2f785d`, `Network_Devices/Palo_Alto/template_palo_alto_firewall_dynamic_routing_by_http/6.4/template_palo_alto_firewall_dynamic_routing_by_http.yaml`.
- No-BGP cache/reprobe behavior: `src/go/plugin/go.d/collector/panos/bgp.go`, `TestCollector_Collect_NoBGPStateIsCached`.
- NIDL chart contexts and labels: `src/go/plugin/go.d/collector/panos/charts.go`, `TestCollector_Collect_LegacyBGP`.
- Stale chart grace behavior: `src/go/plugin/go.d/collector/panos/charts.go`, `TestCollector_Collect_RemovesChartsAfterGraceMisses`.
- Parser/helper coverage: `TestParserHelpers`, `TestParseAPIURL`, `TestParseBGPPeers_ErrorResponseWithNestedLines`.
- Troubleshooting/error-hardening coverage: `TestCollector_Collect_ReportsUnrecognizedSystemResponse`, `TestCollector_Collect_ReportsMalformedTelemetryValues`, `TestCollector_Collect_ReportsMalformedLicenseExpirationWithoutFakeNeverValue`, `TestCollector_Collect_ReportsMalformedIPSecLifetime`, `TestParseBGPPeers_ErrorResponseWithResultMessage`, `TestParseBGPPeers_MalformedNumericFieldFails`, `TestParseBGPPeers_MissingNumericFieldFails`, strict required/invalid parser checks in `TestParserHelpers`, and `TestPangoAPIClient_OpSanitizesPasswordInErrors`.
- Cardinality/status/IPsec mismatch coverage: `TestCollector_Collect_AppliesBGPCardinalityControls`, `TestCollector_Collect_AppliesTelemetryCardinalityControls`, `TestCollector_Collect_RemovesDynamicChartsAfterGraceMisses`, `TestCollector_Collect_ReportsMalformedLicenseStatusWithoutFakeValidValue`, and `TestCollector_Collect_UsesIPSecNTunWhenEntriesAreAbsent`.
- License-safe legacy fixture coverage: `src/go/plugin/go.d/collector/panos/testdata/legacy_bgp_peers.xml`, `TestParseBGPPeers`.
- Read-only metricset implementation: `src/go/plugin/go.d/collector/panos/telemetry.go`, `src/go/plugin/go.d/collector/panos/charts.go`, `TestCollector_Collect_ReadOnlyTelemetry`, `TestParseReadOnlyTelemetry`.
- Metricset selection and partial collection behavior: `src/go/plugin/go.d/collector/panos/collector.go`, `src/go/plugin/go.d/collector/panos/collect.go`, `TestCollector_Init`.
- Fixture-backed non-BGP parser coverage: `testdata/system_info.xml`, `testdata/ha_state.xml`, `testdata/environment.xml`, `testdata/licenses.xml`, `testdata/ipsec_sa.xml`.
- Docs/config/alert/spec artifacts: listed in execution log above.

Not validated locally:

- Live PAN-OS firewall collection. No firewall endpoint or sanitized real ARE/non-BGP XML output was available in this session.
- External PR reviewer loop is not completed in this SOW. Draft PR #22389 exists and remains the review/validation surface.

Real-use evidence:

- No live firewall was available to this implementation session.
- User confirmed on 2026-05-02 that live PAN-OS validation is handled by the SRE validation process outside this implementation SOW.
- Draft PR opened: <https://github.com/netdata/netdata/pull/22389>.

Reviewer findings:

- Pre-implementation second-opinion reviews from Claude, GLM, MiniMax, Qwen, and Kimi were run and their consensus hardening items were implemented before closure.
- GitHub PR reviewer loop is outside this SOW closure and remains attached to PR #22389.

Same-failure scan:

- SNMP profile diff checks stayed clean; the Palo Alto SNMP profile surface was not modified.
- Collector registry import compile test passed via `go test -count=1 ./plugin/go.d/collector`.

Sensitive data gate:

- Personal-name scan over SOW/spec/collector/config/health artifacts was clean.
- Durable artifacts do not include raw credentials, API keys, bearer tokens, SNMP communities, customer names, private endpoints, or live customer PAN-OS XML.
- XML fixtures are Netdata-owned synthetic fixtures; third-party fixture data was used only as shape evidence and not copied verbatim.

Artifact maintenance gate:

- AGENTS.md: no update required for this collector implementation; repository SOW and collector consistency rules already covered the workflow.
- Runtime project skills: `.agents/skills/project-writing-collectors/SKILL.md` was reviewed; its CollectorV2 requirement was applied to this collector.
- Specs: `.agents/sow/specs/panos-collector.md` was added and updated for supported metricsets, cardinality controls, obsoletion, and troubleshooting behavior.
- End-user/operator docs: `src/go/plugin/go.d/collector/panos/README.md`, `metadata.yaml`, `config_schema.json`, stock `panos.conf`, and `src/health/health.d/panos.conf` were added/updated.
- End-user/operator skills: no Netdata AI output/reference skills were affected by this collector implementation.
- SOW lifecycle: Status changed to `completed`; file moved from `.agents/sow/current/` to `.agents/sow/done/`; live-device validation is documented as external SRE validation rather than a blocking SOW item.

Specs update:

- `.agents/sow/specs/panos-collector.md` documents the shipped collector contract.

Project skills update:

- No additional project skill update required at SOW close. The existing collector-writing skill already captures the relevant CollectorV2, cardinality, obsoletion, docs, and validation rules.

End-user/operator docs update:

- PAN-OS collector README, metadata, config schema, stock config, and health alerts were added/updated with setup, metricset selection, cardinality, troubleshooting, and alert behavior.

End-user/operator skills update:

- No end-user/operator AI skills were affected.

Lessons:

- See Lessons Extracted below.

Follow-up mapping:

- Live PAN-OS validation: transferred to the SRE validation process by user decision on 2026-05-02; not tracked as an implementation SOW item.
- Panorama proxy: rejected from this SOW; v1 is direct-to-firewall only.
- GlobalProtect, certificates, sessions, interfaces, dataplane CPU, arbitrary XML passthrough, config, commit, log, report, import, export, and user-id operations: rejected from this SOW by scope decision; each needs a new user-approved SOW if pursued.
- BGP loc-rib route counts and diagnostic Netdata functions: rejected from this SOW because polling cost and UI contract were not validated for v1.
- OpenConfig/gNMI and future PAN-OS BGP4-MIB support: rejected from this SOW as speculative future product work, not implementation debt.

## Outcome

Completed and pushed through draft PR #22389. The collector covers BGP plus system, HA, environment, licenses, and IPsec using read-only PAN-OS XML API commands.

Local validation is fixture-backed and complete for this implementation SOW. Live PAN-OS validation remains intentionally outside this SOW and is handled by the SRE validation process per user decision on 2026-05-02.

## Lessons Extracted

- Palo Alto documents how to derive XML API operational commands, but does not publish a stable BGP XML response schema. The collector parser must stay tolerant and fixture-driven.
- pango is sufficient for transport/keygen/op calls, but it does not refresh expired keys automatically. The Netdata wrapper owns that retry behavior.
- PAN-OS BGP over SNMP is not a viable polling path today; preserving existing SNMP profiles untouched is the correct separation.

## Followup

No implementation follow-up remains in this SOW. Candidate future PAN-OS collector work is intentionally out of scope and requires a new user-approved SOW.
