# SOW-0008 - MikroTik SNMP Per-Second Gaps

## Status

Status: completed

Sub-state: closed as diagnosis-complete; root cause is inherent RouterOS SNMP behavior for the tested topology OIDs.

## Requirements

### Purpose

Diagnose and fix the root cause that prevents true per-second SNMP collection. The only acceptable outcome is Netdata sustaining per-second SNMP collection for the affected device class; interval increases, partial disabling, local-only tuning, or other workarounds are not acceptable.

### User Request

The user reported that a previously reliable per-second MikroTik SNMP job now shows many gaps after topology and NetFlow work. The user asked to diagnose whether the cause is:

- the SNMP plugin can no longer collect per second;
- topology polling is too frequent and interferes;
- the Netdata plugin itself is broken.

If Netdata is at fault, create this SOW before implementation.

### Assistant Understanding

Facts:

- The affected device is a MikroTik CCR2004-16G-2S+ monitored by the `go.d` SNMP collector.
- The SNMP job is configured for `update_every: 1`.
- The separate `snmp_topology` job is configured for a slower cadence, not per-second.
- The observed gaps are real tier-0 database gaps, not a UI rendering artifact.
- Netdata logs show the SNMP job repeatedly skipping samples because the previous collection run is still in progress.

Inferences:

- The primary failure mode is collection overrun: successful SNMP runs take around 9-10 seconds, so a 1-second job skips intermediate ticks.
- Router raw capacity is unlikely to be the main bottleneck: RouterOS resource/profile output showed low CPU and low SNMP CPU during observation.
- The hidden `_topology_*` metrics are not directly collected by the regular SNMP job because the SNMP collector strips them before normal metric collection.
- The exact slow path is now proven: topology refresh has a separate cadence, but it opens concurrent SNMP sessions to the same device and performs long table walks that make the device delay normal per-second GETs.
- A direct isolated SNMP test with local Netdata stopped confirmed the RouterOS SNMP agent can spend about 8-10 seconds on topology table walks even with very small bulk repetitions. During normal Netdata operation, those walks block or delay the regular metrics GET path and produce per-second gaps.

Unknowns:

- No remaining unknown blocks this SOW. The exact RouterOS internal implementation detail is not externally observable, but the external behavior is proven: a single topology GETBULK request against a previously slow bridge FDB root can still take about 9.5 seconds after an idle gap.

### Acceptance Criteria

- The MikroTik SNMP job can sustain per-second `snmp.device_prof_ifTraffic` and `snmp.device_prof_ifOperStatus` without repeated skip/resume logs under normal local office load.
- Topology data remains available through the topology loop and is not polled by the per-second metrics loop.
- Validation records a 120-second, 120-point tier-0 query with no repeated all-null runs for the interface contexts.
- Validation records Netdata namespace logs without repeated `previous run is still in progress` messages for the affected job.
- Durable artifacts contain only redacted endpoint/secret evidence.

Final acceptance note:

- The original "Netdata must sustain per-second metrics while topology runs" acceptance path is closed by user decision because the verified cause is RouterOS SNMP behavior, not a fixable Netdata plugin defect in this SOW. Netdata can sustain per-second metrics when those unsafe RouterOS topology walks are not issued.

## Analysis

Sources checked:

- Netdata local MCP metric queries for `snmp.device_prof_ifTraffic`, `snmp.device_prof_ifOperStatus`, `snmp.device_prof_stats_timings`, `snmp.device_prof_stats_snmp`, `snmp.device_prof_stats_metrics`, and `snmp.device_prof_stats_errors`.
- Netdata namespace logs through the local `systemd-journal` function.
- RouterOS SSH commands for resource and live profile state.
- SNMP collector scheduler code: `src/go/plugin/framework/jobruntime/job_common.go`.
- SNMP profile selection/filtering code: `src/go/plugin/go.d/collector/snmp/profile_sets.go`, `src/go/plugin/go.d/collector/snmp/topology_profile_filter.go`, `src/go/plugin/go.d/collector/snmp/ddsnmp/topology_classify.go`.
- MikroTik profile and topology profile fragments under `src/go/plugin/go.d/config/go.d/snmp.profiles/default/`.
- GoSNMP fork used by this tree: `src/go/go.mod` replaces `github.com/gosnmp/gosnmp` with `github.com/ilyam8/gosnmp` at version `v0.0.0-20250912202722-388b2cb5192e`.

Current state:

- Tier-0 `snmp.device_prof_ifTraffic` queried as last 120 seconds / 120 points showed repeated all-null runs of about 8 seconds followed by short data bursts.
- `snmp.device_prof_ifOperStatus` showed matching all-null runs in the same per-second pattern.
- Netdata namespace logs for the redacted MikroTik job showed repeated messages:
  - `skipping data collection: previous run is still in progress ... interval 1s`
  - `data collection resumed after 9.580477889s (skipped 9 times)`
  - `data collection resumed after 10.152793637s (skipped 10 times)`
- `snmp.device_prof_stats_snmp` showed successful runs doing about 25 GET requests and 410 OIDs per data sample, with zero walk requests in the checked 120-second window.
- `snmp.device_prof_stats_errors` reported zero `snmp`, `processing_scalar`, and `processing_table` errors in the checked 120-second window.
- RouterOS resource output showed low CPU load and enough free memory during observation.
- RouterOS live profile samples showed low SNMP CPU during observation.
- Temporary instrumentation identified the exact interaction:
  - normal per-second SNMP samples are fast when topology is not walking the same device, usually about 150-300 ms for about 24 GET requests and about 406 OIDs;
  - while `snmp_topology` is refreshing topology, it performs long table walks against the same device;
  - during those topology walks, the regular SNMP job's scalar GET of only 2 OIDs can take about 8.8 seconds;
  - those delayed scalar GETs directly align with the existing skip/resume logs and database gaps.
- A first candidate fix that added a per-endpoint request gate and capped topology max-repetitions was installed and validated live, but it did not satisfy acceptance:
  - regular metrics still logged skip/resume sequences around 9 seconds;
  - topology still logged multi-second walks on bridge/STP/FDB-style OIDs;
  - regular 2-OID scalar GETs and cached table GETs were still delayed while topology collection was active.
- Direct read-only SNMP tests with local Netdata stopped showed:
  - ordinary scalar GETs returned in about 0.01 seconds;
  - cached-table-equivalent GETs for MikroTik optical and health OIDs returned in about 0.01-0.03 seconds;
  - topology table walks against the STP/bridge table subtree intermittently took about 8-10 seconds even with max-repetitions set to 1, 2, 3, 5, or 10;
  - therefore smaller bulk repetitions alone cannot guarantee per-second metrics for this device.
- GoSNMP does not implement a global or per-target same-device concurrency gate in the request path:
  - `NewHandler` creates a fresh `GoSNMP` value for each caller;
  - `Connect` opens a socket on that handler instance;
  - the only `GoSNMP` mutex found is used by `Close`;
  - `Get` and `GetBulk` call `send` directly, and `send` calls `sendOneRequest` without taking a target lock;
  - `sendOneRequest` writes one packet then waits for its response on that handler's socket.
- GoSNMP walks are sequential within one handler instance: the walk loop calls `GetBulk`, waits for the response, processes it, advances the OID, and then sends the next request. It does not fire an asynchronous burst of concurrent GETBULK requests from one walk.

Risks:

- Treating this only as a configuration tuning issue would sacrifice the user's required per-second interface metrics.
- Removing too much from the regular SNMP profile may drop useful non-topology metrics for existing users.
- Leaving heavy profile sections in the 1-second path causes gaps and misleading rate spikes after skipped samples.
- Changing profile filtering can affect all SNMP devices that rely on shared topology or LLDP profile fragments.

## Pre-Implementation Gate

Status: ready

Problem / root-cause model:

- The SNMP collector scheduler intentionally skips ticks when a prior run is still active. Evidence: `src/go/plugin/framework/jobruntime/job_common.go:94` sends ticks through a non-blocking channel, and `src/go/plugin/framework/jobruntime/job_common.go:106` logs the `previous run is still in progress` warning after repeated skips.
- The affected job is overrunning its 1-second interval. Evidence: Netdata namespace logs show resume times around 9-10 seconds for the redacted MikroTik job.
- The overrun is not explained by SNMP errors. Evidence: `snmp.device_prof_stats_errors` stayed at zero for SNMP and processing dimensions in the checked 120-second window.
- The overrun is not explained by full topology walks in the checked window. Evidence: `snmp.device_prof_stats_snmp` showed zero walk requests and about 410 GET OIDs per successful sample.
- The separate topology collector is not configured per-second. Evidence: local `snmp_topology` config is `update_every: 60` and `refresh_every: 30s`; Netdata namespace logs for `snmp_topology` appeared around minute cadence.
- Hidden topology metrics are intended to be stripped from the regular SNMP collector. Evidence: `src/go/plugin/go.d/collector/snmp/topology_profile_filter.go:10` documents that `selectCollectionProfiles` filters topology metrics out of normal collection, and `src/go/plugin/go.d/collector/snmp/profile_sets.go:16` calls it during profile setup.
- The MikroTik profile was expanded by the topology work. Evidence: `src/go/plugin/go.d/config/go.d/snmp.profiles/default/mikrotik-router.yaml:1` extends `_std-lldp-mib.yaml` plus topology fragments, and commit `ebd373c8f0` added those profile extends. This is investigative evidence only; it does not prove topology is the slow path.
- The slow path is not regular metric processing, transforms, or cached table GETs in isolation. Evidence: with local Netdata stopped, direct scalar and cached-table-equivalent GETs to the redacted device completed in milliseconds.
- The slow path is the topology walk class. Evidence: with local Netdata stopped, direct walks of the STP/bridge subtree intermittently took about 8-10 seconds even with max-repetitions as low as 1.
- The current collector contract is broken for per-second SNMP jobs because topology refresh can issue best-effort slow walks against the same endpoint without a hard guarantee that regular metric collection remains under 1 second.

Evidence reviewed:

- `src/go/plugin/framework/jobruntime/job_common.go:94`
- `src/go/plugin/framework/jobruntime/job_common.go:106`
- `src/go/plugin/framework/jobruntime/job_common.go:119`
- `src/go/plugin/go.d/collector/snmp/profile_sets.go:16`
- `src/go/plugin/go.d/collector/snmp/topology_profile_filter.go:10`
- `src/go/plugin/go.d/collector/snmp/topology_profile_filter.go:38`
- `src/go/plugin/go.d/collector/snmp/ddsnmp/topology_classify.go:12`
- `src/go/plugin/go.d/config/go.d/snmp.profiles/default/mikrotik-router.yaml:1`
- `src/go/plugin/go.d/config/go.d/snmp.profiles/default/_std-lldp-mib.yaml:7`
- `src/go/plugin/go.d/config/go.d/snmp.profiles/default/_std-topology-fdb-arp-mib.yaml:4`
- `src/go/plugin/go.d/config/go.d/snmp.profiles/default/_std-topology-lldp-mib.yaml:53`
- Netdata namespace logs, redacted: affected job skipped 9-10 one-second ticks per long run.
- Netdata MCP, redacted: per-second tier-0 queries showed all-null runs matching skip logs.
- RouterOS SSH, redacted: device CPU/memory and SNMP CPU did not show overload during observation.

Affected contracts and surfaces:

- SNMP profile YAML behavior under `src/go/plugin/go.d/config/go.d/snmp.profiles/default/`.
- SNMP regular metrics collector behavior under `src/go/plugin/go.d/collector/snmp/`.
- SNMP topology collector behavior under `src/go/plugin/go.d/collector/snmp_topology/`.
- SNMP troubleshooting documentation and metadata if the final behavior changes recommendations or defaults.
- Runtime metrics and chart continuity for `snmp.device_prof_ifTraffic` and `snmp.device_prof_ifOperStatus`.

Existing patterns to reuse:

- Existing topology metric classification in `ddsnmp/topology_classify.go`.
- Existing regular-collector filtering in `snmp/topology_profile_filter.go`.
- Existing topology-only profile filtering in `snmp_topology/profile_filter.go`.
- Existing collector scheduler skip/resume logs in `jobruntime/job_common.go`.
- Existing SNMP profile stats contexts for validation.

Risk and blast radius:

- A profile-only change is narrow but can affect all MikroTik RouterOS and SwOS devices.
- A generic classifier/cadence change can affect all SNMP devices and must have focused tests.
- A runtime split that keeps some tables off the 1-second path is more robust but has larger code and documentation blast radius.
- Any local operational mitigation must avoid writing SNMP communities or private endpoints into durable artifacts.
- Trying to solve this only by lowering topology `max_repetitions` is insufficient; direct tests showed 8-10 second stalls even with max-repetitions set to 1.
- Trying to solve this only with a Go-side per-endpoint mutex/priority gate is insufficient if a topology request already sent to the device can occupy or delay the RouterOS SNMP agent for several seconds.
- The GoSNMP library itself is not serializing all same-device traffic across Netdata jobs. Evidence: `src/go/go.mod:7` selects the local fork; `github.com/ilyam8/gosnmp` `interface.go:187` creates independent handlers, `gosnmp.go:374` opens the per-handler connection, `gosnmp.go:294` uses the handler mutex only in `Close`, and `marshal.go:274` / `marshal.go:293` show one write followed by response wait in the same request.

Sensitive data handling plan:

- Do not write SNMP communities, trap communities, exact private endpoints, SSH details, contact/location strings, claim IDs, or personal data into durable artifacts.
- Use `[PRIVATE_ENDPOINT]` for endpoint evidence and redacted job names where needed.
- Keep raw logs and command outputs out of the SOW unless sanitized.
- Code comments and docs must describe behavior generically, not this office device.

Implementation plan:

1. Isolate which profile sections, tables, OIDs, transforms, virtual metrics, or framework paths account for the 9-10 second successful collection runtime.
2. Add temporary targeted instrumentation to the SNMP collector if existing stats are insufficient; rebuild with `./build-install-go.d.plugin.sh`.
3. Use the instrumentation against the live affected device to identify the exact slow operation(s).
4. Implement the root-cause fix that restores true per-second SNMP collection without interval increases or local workarounds.
5. Add focused unit/regression tests for the corrected behavior.
6. Validate locally with 120-second / 120-point tier-0 metric queries and Netdata namespace logs.
7. Update docs/specs/skills if collector behavior, profile authoring rules, or troubleshooting guidance changes.

Validation plan:

- Query `snmp.device_prof_ifTraffic` and `snmp.device_prof_ifOperStatus` as last 120 seconds / 120 points / tier 0.
- Query `snmp.device_prof_stats_timings`, `snmp.device_prof_stats_snmp`, `snmp.device_prof_stats_metrics`, and `snmp.device_prof_stats_errors`.
- Query Netdata namespace logs for the redacted SNMP job and `previous run is still in progress`.
- Run the narrow Go tests for SNMP profile filtering and topology collector profile filtering.
- Search for the same failure pattern in other SNMP profile extensions touched by topology work.

Artifact impact plan:

- AGENTS.md: likely unaffected unless this reveals a project-wide SOW/process rule gap.
- Runtime project skills: update `project-snmp-profiles-authoring` if profile authoring rules must prevent topology-heavy data from regular metric loops.
- Specs: add or update an SNMP/topology behavior spec if this work changes collector contracts.
- End-user/operator docs: update SNMP troubleshooting/docs if recommendations or topology polling behavior change.
- End-user/operator skills: likely unaffected unless public SNMP/query skills need new diagnostic workflow.
- SOW lifecycle: keep this SOW pending/open until user decision; move to current/in-progress before implementation.
- SOW lifecycle update: moved to current/in-progress after the user authorized root-cause instrumentation and local rebuilds.

Open-source reference evidence:

- `prometheus/snmp_exporter @ c12d07d5a60db3fd5a2cffaa02202e88d70a8b4d`: `config/config.go:124` exposes per-module `max_repetitions`, `retries`, and `timeout`; `generator/README.md:121` documents default `max_repetitions: 25` and says it may need reduction for buggy devices; `scraper/gosnmp.go:107` uses `BulkWalkAll` for SNMPv2/v3 walks.
- `DataDog/datadog-agent @ 4bb5357ef2350cd91a84df21cfb74f09cb95e8d1`: `pkg/collector/corechecks/snmp/internal/checkconfig/config.go:60` documents the too-high repetition risk; `config.go:64` defaults `bulk_max_repetitions` to 10; `config.go:141` separates OID batch size from table bulk repetitions.
- Official protocol/tool references checked: MikroTik RouterOS SNMP documentation confirms RouterOS exposes IF-MIB, IP-MIB, BRIDGE-MIB, and OID-based interface metrics; Net-SNMP GETBULK documentation confirms `max-repetitions` controls how many repeated OID instances are requested in one response; GoSNMP upstream documentation confirms `BulkWalkAll` retrieves a subtree using GETBULK.

Open decisions:

- None. The user decision is recorded below.

## Implications And Decisions

1. Root-cause and outcome decision:
   - Selection: true root-cause investigation and fix only.
   - Evidence: existing logs prove skipped samples, but existing stats do not prove which OID/profile/code path consumes the runtime.
   - Implication: temporary instrumentation and local rebuilds are allowed if needed.
   - Rejected paths: interval increase, local-only config workaround, partial disabling without root-cause proof, or accepting non-per-second collection.
   - Risk accepted: live local Netdata may be rebuilt/restarted during investigation, causing temporary local monitoring gaps.

2. External validation before implementation:
   - Selection: test candidate SNMP request policies outside Netdata before wiring any production code change.
   - Evidence: direct Net-SNMP tests already showed that simple `max_repetitions=1` tuning is not sufficient, and Netdata code review showed topology walks use a separate client and unstable map-order table iteration.
   - Implication: use read-only external SNMP probes with local Netdata stopped to compare scalar 1-second GET latency while topology-like walks run under different request policies.
   - Rejected path: adding or changing Netdata plugin behavior before an external policy proves it can preserve the 1-second metrics SLA.
   - Risk accepted: local Netdata may be temporarily stopped during isolated read-only tests; the production router must not be rebooted, reset, or reconfigured.

3. Close decision:
   - Selection: close this SOW as diagnosis-complete with no Netdata implementation.
   - Evidence: a single read-only GETBULK request against a previously slow bridge FDB topology root still took about 9.489 seconds after local Netdata was stopped and the router had 10 seconds idle time.
   - Implication: spreading topology requests over time cannot fully solve the issue because the first expensive request itself can block for multiple seconds.
   - Rejected path: implement Netdata tuning for `max_repetitions`, pacing, short topology timeout, or topology concurrency as a claimed fix in this SOW.
   - Final conclusion: this behavior is inherent to RouterOS SNMP for the tested OID class, or at least not externally correctable by Netdata transport policy without avoiding those topology walks.

## Plan

1. Move this SOW to current/in-progress.
2. Add targeted instrumentation only if existing stats cannot identify the slow path.
3. Rebuild and install `go.d.plugin` locally using `./build-install-go.d.plugin.sh` when instrumentation or a candidate fix is ready.
4. Validate with live local Netdata and redacted evidence.
5. Update durable artifacts if collector behavior or profile authoring rules change.

## Execution Log

### 2026-05-05

- Created this pending SOW after diagnosis found Netdata-side per-second SNMP collection overruns.
- User clarified that workarounds are not acceptable; only a verified root cause and true per-second SNMP collection are acceptable.
- User authorized temporary logs/instrumentation and rebuilding `go.d.plugin` with `./build-install-go.d.plugin.sh`.
- User clarified that no permanent destructive action is allowed. The production router must not be rebooted, reset, or modified destructively.
- Moved SOW to current/in-progress before code instrumentation.
- Confirmed the separate `snmp_topology` job is not configured per-second; the regular SNMP metric collector still overruns its own 1-second cadence.
- Confirmed existing runtime stats prove about 25 sequential SNMP GET requests and about 410 OIDs per successful sample, but do not identify the exact slow table/OID subset.
- Planned temporary SNMP collector instrumentation that logs only generic timing evidence: profile source, table name, operation name, OID counts, request counts, response counts, duration, and error state.
- Installed the temporarily instrumented `go.d.plugin`; local Netdata restarted cleanly.
- Verified the router and regular metric collector can sustain per-second collection when topology is not walking the device: regular runs were about 150-300 ms with about 24 GET requests and about 406 OIDs.
- Verified the root-cause interaction after a topology refresh:
  - `snmp_topology` table walks against the same device took multi-second intervals;
  - regular per-second scalar GETs of only 2 OIDs took about 8.8 seconds during the topology walk window;
  - the regular job immediately logged skipped 1-second ticks and resumed after about 9 seconds.
- Installed and validated a first candidate fix with a per-endpoint priority gate and topology max-repetitions cap. It did not pass live validation: the affected regular job still skipped about 9 one-second ticks while topology walks were active.
- Temporarily stopped only the local Netdata service and ran direct read-only SNMP timing checks against the redacted device, then restarted local Netdata in the same shell. The router was not modified.
- Confirmed direct scalar GETs and cached-table-equivalent GETs are fast in isolation.
- Confirmed topology-class walks can still take about 8-10 seconds in isolation, including with max-repetitions set to 1. This proves the topology walk class itself is unsafe to run against a device that must sustain 1-second metrics.
- Checked the GoSNMP fork used by this tree. No global or per-target same-device lock exists in the GET/GETBULK send path; walks are synchronous request/response loops, not asynchronous bursts from the client library.
- User changed the local topology cadence to a much longer interval. Follow-up tier-0 checks for the affected node showed the last 120 seconds / 120 points flowing continuously: interface traffic and operational-status points had no empty/partial annotations, regular SNMP stats showed zero walk requests, regular table timing stayed under 300 ms, and Netdata namespace logs had no skip/resume entries for the affected job in the checked recent window.
- User challenged the premature improvement plan and asked whether larger `max_repetitions` had been tested. A direct read-only timing matrix with local Netdata stopped showed the problem is not a generic "topology walks are always slow" condition:
  - the legacy ARP table was much faster with larger repetitions: `max_repetitions=25` and `50` completed in under 300 ms, while `1` took several seconds and one first-pass run timed out after 20 seconds;
  - bridge/Q-bridge FDB table behavior varied by run order and repetition; one first-pass low-to-high run made `max_repetitions=1` slow, while a repeat run made `max_repetitions=25` slow for the bridge FDB table but not the Q-bridge FDB table;
  - most LLDP, interface, STP, VLAN, and small topology tables completed in milliseconds for repetitions from 1 to 50;
  - `max_repetitions=100` consistently failed quickly on these tests and is not a safe direction for this device.
- Updated working theory: the root cause is specific topology table walk interactions, especially bridge/FDB and legacy ARP areas, with RouterOS behavior depending on OID root, repetition size, and likely agent/table cache/order state. A final fix must be based on exact slow walk identification and reproduced Netdata-side `max_repetitions` behavior, not generic pacing assumptions.
- Code review found Netdata topology table walk order is not stable: `walkTables` iterates over a Go map of table OIDs. A Netdata topology refresh can therefore hit topology roots in different orders across runs.
- A direct read-only randomized-order reproduction with local Netdata stopped, using `max_repetitions=25` for all topology roots, reproduced the stall class:
  - one round spent about 9.5 seconds in the STP port table;
  - another round spent about 3.9 seconds in the IP address table followed by about 5.8 seconds in the bridge FDB table;
  - the same roots were fast in other rounds.
- Updated root-cause statement: the local gaps are caused by Netdata running best-effort topology walks against the same SNMP endpoint as the 1-second metrics job. RouterOS exhibits order/state-sensitive multi-second stalls on specific topology table roots even with the default `max_repetitions=25`; Netdata's current topology loop has no mechanism to isolate or abort those stalls before they delay the per-second metrics job.
- User requested the next investigation step: prove candidate SNMP request policies outside Netdata before any code is wired. Recorded this as the active decision.
- Built and ran a temporary external GoSNMP harness under `.local/snmp-homework/` using the same GoSNMP module path and replacement version as Netdata. The harness runs a 1-second scalar GET loop while a separate client performs topology-like GETBULK walks. Raw logs remain under `.local/snmp-homework/results/` and are not durable project artifacts.
- Baseline with local Netdata stopped and no topology walks:
  - first suite: 20 scalar GET samples, zero errors, zero 1-second SLA violations, max 22 ms;
  - low-repetition suite: 15 scalar GET samples, zero errors, zero 1-second SLA violations, max 8 ms;
  - edge suite: 15 scalar GET samples, zero errors, zero 1-second SLA violations, max 16 ms.
- External policy matrix results with all topology roots:
  - `max_repetitions=25`, no pause: failed. One run had a 9.658 s bridge FDB walk and one scalar GET at 8.711 s; another run had 9.471 s interface walk, 9.579 s bridge FDB walk, and two scalar GET SLA violations.
  - `max_repetitions=50`, no pause: failed. Interface and bridge FDB walks were about 9.3 s and 9.6 s; scalar GET loop had three SLA violations, max 9.026 s.
  - `max_repetitions=10`, no pause: failed. Interface and bridge FDB walks were about 9.5 s and 9.7 s; scalar GET loop had three SLA violations, max 8.895 s.
  - `max_repetitions=5`, no pause: failed. Interface and bridge FDB walks were about 9.3 s and 9.8 s; scalar GET loop had two SLA violations, max 8.974 s.
  - `max_repetitions=2`, no pause: failed. Interface, bridge FDB, and legacy ARP walks took about 9.8 s, 9.8 s, and 12.2 s; scalar GET loop had four SLA violations, max 9.034 s.
  - `max_repetitions=1`, no pause: failed. Bridge FDB, legacy ARP, and Q-BRIDGE FDB walks took about 9.9 s, 24.8 s, and 9.8 s; scalar GET loop had six SLA violations, max 9.323 s.
  - `max_repetitions=1`, 10 ms pause between GETBULK requests: failed. Interface, bridge FDB, legacy ARP, Q-BRIDGE FDB, and STP roots all had multi-second walk times; scalar GET loop had twelve SLA violations, max 9.472 s.
  - `max_repetitions=25`, 250 ms pause between GETBULK requests: failed. The walk stretched to about 85.8 s and scalar GET loop had eleven SLA violations, max 9.337 s.
  - `max_repetitions=25`, topology timeout 800 ms, zero topology retries: failed. The topology client timed out many roots quickly, but the scalar GET loop still had a max latency around 8.7 s. This was reproduced in a separate edge run, so it was not only contamination from the previous long test.
  - `max_repetitions=100`: not viable. The GoSNMP harness received zero PDUs for every topology root and therefore collected no topology data; prior Net-SNMP direct tests also showed this direction failing quickly.
- External reduced-root test:
  - Removing the obvious bridge/FDB/ARP roots was still not safe. A reduced set including interface, IP, STP, VLAN, and LLDP roots failed twice: one run had an `ifTable` walk around 3.1 s and scalar GET max 3.67 s; the next had `ifTable` and STP walks around 9.5 s each and scalar GET max 9.212 s.
- Updated conclusion after external homework: no tested transport policy that still walks the topology roots preserves the 1-second scalar GET SLA on this RouterOS device. Smaller bulks, larger bulks, per-bulk pauses, and short topology-side timeouts all fail. The only policies that can preserve per-second metrics are policies that avoid issuing unproven topology walks while 1-second collection is required, or policies that can learn and quarantine unsafe walks after causing initial damage.
- Searched current public MikroTik/RouterOS evidence for known SNMP issues:
  - Official MikroTik RouterOS SNMP documentation says SNMP gathers data from other RouterOS services, can log `timeout while waiting for program` / `SNMP did not get OID data within expected time`, may deny requests for that service for a while, and says slow/busy services should often be skipped by monitoring tools.
  - Official MikroTik RouterOS SNMP documentation lists IF-MIB, IP-MIB, and BRIDGE-MIB as RouterOS-supported MIBs, matching the class of topology roots involved in this investigation.
  - Public MikroTik forum reports include RouterOS 7.x SNMP polling times of 200-220 seconds on a CCR2004-16G-2S+ with RouterOS 7.14.1, intermittent SNMP refusal on RouterOS 7.14.2 with low CPU, and very slow SNMP walks on MikroTik enterprise interface-stat OIDs.
  - Checkmk has an old compatibility fix for a broken MikroTik RouterOS bulk-walk implementation in RouterOS v6.22 where consecutive duplicate OIDs could be returned.
  - RouterOS 7.22 public changelog evidence mentions an SNMP fix where bulk walk might skip the first OID. This is not the same as the observed stall, but it shows RouterOS bulk-walk behavior has had recent fixes.
  - No public source found an exact named MikroTik bug for "BRIDGE-MIB/FDB/IF-MIB GETBULK stalls concurrent scalar SNMP GETs for 8-10 seconds." The evidence supports a known RouterOS SNMP slow/busy-service class, not a public exact-match defect ID.
- User asked to test whether the behavior is internal RouterOS SNMP rate limiting rather than slow OID processing. Planned external read-only tests with long pauses between topology GETBULK requests and between topology roots. Prediction: if simple rate limiting is the cause, long request spacing should remove the 8-10 second stalls and scalar GET SLA violations.
- Rate-limit hypothesis quick check:
  - Stopped local Netdata, waited 10 seconds, then ran a single read-only walk of one previously slow bridge FDB topology root with `max_repetitions=25`. Result: 192 rows, exit 0, duration 9.528 seconds.
  - Stopped local Netdata again, waited 10 seconds, then ran one single GETBULK request against the same root with `max_repetitions=25`, not a full walk. Result: 25 rows, exit 0, duration 9.489 seconds.
  - Conclusion: the observed delay is not explained by simple recent-request burst rate limiting. A single request after an idle gap can still be delayed by about 9.5 seconds. This does not rule out RouterOS internal serialization, internal table refresh, or per-OID/service throttling, but it rules out "spread previous topology calls and the first expensive request becomes fast" for this root.
- User concluded this is inherent to RouterOS and asked to close the SOW. No source code, router configuration, or durable operational configuration was changed as part of closing.

## Validation

Acceptance criteria evidence:

- Root cause evidence complete.
- Netdata can sustain per-second metrics when topology does not issue unsafe RouterOS topology walks against the same endpoint: after topology cadence was moved away, tier-0 120-second / 120-point queries for the affected interface traffic and operational-status contexts were continuous, and Netdata logs had no affected-job skip/resume entries in the checked window.
- Full topology walking while preserving 1-second metrics is not achievable for this device with the tested request policies: `max_repetitions` values 1, 2, 5, 10, 25, and 50 all produced scalar GET SLA violations during topology walks; pauses and short topology-side timeouts also failed.
- A single GETBULK request after a 10-second idle gap still took about 9.489 seconds, proving the failure is not simple burst rate limiting.
- The user accepted the conclusion that this is inherent RouterOS behavior and requested SOW closure.

Tests or equivalent validation:

- Diagnostic validation completed:
  - tier-0 120-second / 120-point queries showed repeated all-null runs for interface traffic/status;
  - Netdata namespace logs showed skip/resume messages for the affected SNMP job;
  - SNMP error stats stayed at zero in the checked window;
  - RouterOS resource/profile data did not show router overload.
- External Net-SNMP and GoSNMP timing validation completed with local Netdata stopped:
  - baseline scalar GET loop had zero errors and zero 1-second SLA violations;
  - topology-like GETBULK walks reproduced 8-10 second stalls and scalar GET SLA violations;
  - low, default, and higher `max_repetitions` values were tested and failed;
  - request pacing and short topology-side timeouts were tested and failed;
  - one single GETBULK request after an idle gap still took about 9.5 seconds.

Real-use evidence:

- Local Netdata MCP and local Netdata `systemd-journal` function were used.
- Direct read-only SNMP commands were run with local Netdata stopped to isolate device behavior without concurrent Netdata polling. Local Netdata was restarted immediately after the test.
- Temporary instrumentation and candidate code changes were removed before closure; source code remained unchanged at close.

Reviewer findings:

- No implementation was shipped, so no code review was required.
- External references were checked for context: MikroTik RouterOS SNMP documentation, public MikroTik forum reports, Checkmk RouterOS SNMP bulk-walk compatibility note, Prometheus `snmp_exporter`, and Datadog Agent SNMP configuration defaults.

Same-failure scan:

- Same-failure class was checked through public MikroTik reports and open-source SNMP collector references.
- Public evidence supports RouterOS SNMP slow/busy-service and bulk-walk issue classes, but no exact public defect ID was found for the specific bridge/FDB/interface 8-10 second stall observed here.

Sensitive data gate:

- Raw SNMP secrets, trap communities, exact private endpoints, contact/location strings, claim IDs, and SSH details were not written to this SOW.
- Private endpoint evidence is redacted as `[PRIVATE_ENDPOINT]`.

Artifact maintenance gate:

- AGENTS.md: no update needed. This SOW did not change repository workflow, responsibility boundaries, or project-wide guardrails.
- Runtime project skills: no update needed. The work produced a device-specific diagnosis, not a new reusable collector-authoring rule.
- Specs: no update needed. No Netdata product behavior or public collector contract changed in this SOW.
- End-user/operator docs: no update needed in this SOW. The user explicitly closed the investigation as inherent RouterOS behavior without requesting a Netdata operator guidance change.
- End-user/operator skills: no update needed. No public/operator AI skill behavior changed.
- SOW lifecycle: status updated to `completed`; file will be moved from `.agents/sow/current/` to `.agents/sow/done/`.

Specs update:

- No spec update. There was no shipped behavior change and no new Netdata contract selected.

Project skills update:

- No project skill update. The investigation used existing collector and SNMP guidance; no reusable workflow rule changed.

End-user/operator docs update:

- No end-user/operator docs update. The result is a closed local diagnosis; documentation of RouterOS-specific topology limitations is outside this SOW and was not requested.

End-user/operator skills update:

- No end-user/operator skill update. No public skill behavior changed.

Lessons:

- A single expensive RouterOS topology GETBULK request can take about 9.5 seconds after an idle gap, so request pacing is not sufficient to guarantee 1-second SNMP metrics.
- `max_repetitions` tuning is device/OID/order dependent on RouterOS and cannot be treated as a generic fix for this failure class.
- External request-policy testing should precede Netdata implementation when the suspected failure could be inherent to the SNMP agent.

Follow-up mapping:

- No follow-up SOW is required by the user. The SOW is closed as diagnosis-complete.

## Outcome

Completed as diagnosis-only.

Final conclusion:

- The per-second metric gaps were caused by RouterOS SNMP behavior when topology-class OIDs are queried.
- Netdata regular SNMP collection can sustain per-second metrics when those unsafe topology walks are not issued.
- Topology collection is a separate Netdata loop and client, but it still targets the same RouterOS SNMP agent.
- A single bridge/FDB topology GETBULK request after a 10-second idle gap took about 9.489 seconds, so the failure is not simple request burst rate limiting.
- No tested Netdata-side transport policy preserved the 1-second SLA while still walking the tested topology roots.
- No Netdata implementation was made or required in this SOW.

## Lessons Extracted

- Verify device-agent behavior externally before implementing collector changes when live evidence suggests an upstream SNMP agent stall.
- Do not assume smaller `max_repetitions` improves RouterOS behavior; for this device/OID class, values from 1 through 50 all failed in at least one tested scenario.
- A topology loop can be architecturally separate from metrics collection and still interfere through the monitored device's own SNMP agent.

## Followup

None.

## Regression Log

None yet.

Append regression entries here only after this SOW was completed or closed and later testing or use found broken behavior. Use a dated `## Regression - YYYY-MM-DD` heading at the end of the file. Never prepend regression content above the original SOW narrative.
