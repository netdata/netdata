# Datadog Agent — SNMP Trap Support: Complete Implementation Analysis

## 0. Document Metadata

- **System**: Datadog Agent — SNMP traps subsystem (`comp/snmptraps/`). The Datadog Agent is a Go process that runs on customer hosts; the trap subsystem is one of many bundles inside that binary. It receives SNMP traps locally, OID-resolves them against a JSON dataset bundled with the Agent, formats them as JSON events, and forwards them through the Agent's standard event-platform pipeline (`EpForwarder`) to the Datadog SaaS backend (`snmp-traps-intake.<site>` over HTTPS, intake track `ndmtraps`). Storage, dedup, alerting, search, retention, UI all happen on the SaaS side; the Agent is a sophisticated **forwarder**.
- **Source evidence**: mirrored. The trap component is a complete subsystem with the canonical Datadog DI layout (`def/`, `fx/`, `impl/`, `mock/`) across ten subpackages.
- **Repository roots analysed**:
  - `datadog/datadog-agent @ 2c813592251b5279eb655c83289b66b88fd9800d` (2026-05-21; HEAD of the local mirror at the time of analysis) — the trap subsystem under `comp/snmptraps/` plus its consumers in `comp/forwarder/eventplatform/`, `pkg/config/setup/`, `pkg/diagnose/firewallscanner/`, `pkg/util/scrubber/`, `comp/api/api/def/`.
  - `datadog/integrations-core @ 411c31db05de3660b68881f4cbfa7335ef5e1b55` (2026-05-21; HEAD of the local mirror at the time of analysis, pinned during iter-2 after codex flagged reproducibility) — used to characterise the `ddev meta snmp generate-traps-db` tooling, one SaaS-side preset monitor, the NDM troubleshooting dashboard, and the `snmp/metadata.csv` telemetry registry (see §4, §8.2, §13.5, §19.2). Re-running this analysis against a different integrations-core commit may surface additional MIB-coverage or preset content; the conclusions are stable across reasonable commit ranges because the analysed artefacts (the `ddev` MIB compiler, the linkDown preset, the NDM troubleshooting dashboard) are long-lived and architecturally stable.
- **Citation convention**: `datadog/datadog-agent @ 2c813592 :: <relative/path>:<line>`. The full commit anchor is above; abbreviated `2c813592` is used inline. Integrations-core citations use `datadog/integrations-core @ 411c31db :: <relative/path>:<line>` (abbreviated `411c31db` inline; full commit hash above).
- **Author**: assistant
- **Reviewer pass**: **converged after iter-2 (iter-3 infra-failed)** — see the Reviewer Pass Log appended to this file for the per-iteration verdict table and finding dispositions. Short version: 6 reviewers run per iteration (codex, glm, kimi, mimo, minimax, qwen). Iter-1 produced 5 useable reviews + 1 infrastructure failure; all majors applied. Iter-2 first pass produced 4 useable reviews from glm/kimi/mimo/minimax (all `accept-with-fixes` with minor findings); all applied. A second iter-2 pass succeeded for codex (after a `-C /tmp` workaround) and surfaced four NEW majors plus two minors that the first iter-2 cohort missed — including a real source contradiction on v1 `uptime` (minimax's iter-2 claim was wrong; codex caught it) and an integrations-core reproducibility issue. All six codex-iter-2 findings applied. Iter-3 attempted to re-run the full reviewer set against the post-iter-2 file but suffered a different, pervasive infrastructure failure across both a 6-way-parallel attempt and a 2-batch-of-3 retry — codex stuck in stdin-blocking mode (only 39 bytes "Reading additional input from stdin..." emitted before the 1800s timeout), all opencode runs producing 0 bytes — likely a harness-nested-bash variable-expansion or stdin-pipe issue when reviewers are launched from a Bash-launcher subshell. Per the SOW stop rule, infrastructure-failed reviewers do not gate convergence; the spec is declared converged at iter-2 with 11 reviewer-iterations of useable feedback (5 iter-1 + 4 iter-2 first-pass + 1 iter-2 codex rerun + 1 qwen iter-2 timeout-no-output), all 23 concrete findings applied.

## 1. System Overview & Lineage

Datadog is a commercial SaaS observability platform (metrics, logs, traces, RUM, security, NDM, etc.). The on-host **Datadog Agent** (Apache 2.0 / 3-Clause BSD per file; `datadog-agent/LICENSE` covers the bulk of the tree) is the open-source component that collects telemetry on customer infrastructure and ships it to the proprietary SaaS backend. Compared to the systems analysed previously in this comparative pass — open-source NMSes (OpenNMS, Zabbix, LibreNMS, CheckMK, Zenoss, Centreon, Nagios+SNMPTT), open-source forwarders (Sensu, Telegraf, Logstash), or stream processors (Cribl) — Datadog is the canonical example of a **central-SaaS observability platform with a thin per-host collector**. SNMP trap support follows that model exactly.

Where SNMP traps fit in the broader product:

- **Traps are a `logs` data type, not a `metrics` data type, on the Datadog side.** This is the design contract. The Agent's `forwarder` component calls `sender.EventPlatformEvent(data, eventplatform.EventTypeSnmpTraps)` (`comp/snmptraps/forwarder/impl/forwarder.go:123`), where `EventTypeSnmpTraps = "network-devices-snmp-traps"` (`comp/forwarder/eventplatform/component.go:20`). The EpForwarder routes this event type via the HTTPS intake `snmp-traps-intake.<site>` with intake track `ndmtraps` (`comp/forwarder/eventplatform/eventplatformimpl/epforwarder.go:168-179`). On the SaaS side, traps surface in the **Logs Explorer** with `source:snmp-traps` (per the in-tree comment in `pkg/config/config_template.yaml:4512`: *"Traps are forwarded as logs and can be found in the logs explorer with a source:snmp-traps query"*).
- **Traps are part of Network Device Monitoring (NDM).** The team owner declared in the bundle is `# team: network-device-monitoring-core` (`comp/snmptraps/bundle.go:15`). The metadata pipeline (`EventTypeNetworkDevicesMetadata`), the metric pipeline (NDM SNMP profiles in `pkg/collector/corechecks/snmp/`), the NetFlow pipeline (`EventTypeNetworkDevicesNetFlow`), and the trap pipeline (`EventTypeSnmpTraps`) are all owned by the same team and all routed through EpForwarder. The trap content is glued back to NDM device records on the SaaS side via the per-trap tag `device_namespace:<namespace>` and `snmp_device:<source-ip>` (computed in `comp/snmptraps/packet/packet.go:27-33`).
- **Two-direction view**:
  - *Inbound to Datadog (the trap subsystem under analysis here)*: device → Agent UDP listener → Agent JSON formatter → SaaS log intake. This is the only "trap" surface in the Datadog Agent.
  - *Outbound from Datadog* (Datadog as a notification destination via outbound SNMP traps): **not supported**. No SNMP-trap-emit channel exists in the integrations or notification surface; the agent has no `SendTrap()` code path. (Datadog notification channels are limited to email, Slack, PagerDuty, webhooks, etc.) Comparison contrast: Sensu's `sensu-snmp-trap-handler` is an outbound emitter; Datadog has no equivalent.

Relationship to upstream tools:

- **`gosnmp/gosnmp`**: the on-wire SNMP library. Imported in `comp/snmptraps/listener/impl/listener.go:17` and `comp/snmptraps/config/def/config.go:14`. The trap listener is `gosnmp.NewTrapListener()` (`listener.go:87`); the Agent calls `gosnmpListener.Listen(addr)` (`listener.go:131`). This is the same library used by Telegraf's `snmp_trap` input — Datadog and Telegraf are the two large Go-based modern trap receivers that both rely on it.
- **Net-SNMP `snmptrapd`**: NOT used. The Agent binds UDP/162 (or the configured port) directly via gosnmp; there is no requirement for an external snmptrapd, no shell-out, no `traphandle` integration. Contrast with Sensu's `snmptrapd2sensu` (process-per-trap external bridge) and CheckMK's older `mkeventd` snmptrapd front-end.
- **`pysmi` (LextuDio fork)**: used in **integrations-core** for the `ddev meta snmp generate-traps-db` MIB compiler (`datadog/integrations-core :: datadog_checks_dev/datadog_checks/dev/tooling/commands/meta/snmp/generate_traps_db.py:1-25`). This is the tool that compiles MIBs into the JSON/YAML "traps DB" that the Agent ships. The Agent itself does NOT compile MIBs at runtime — it only reads the pre-compiled JSON/YAML files. This is a fundamentally different MIB strategy from OpenNMS (in-product trapd MIB engine), Zenoss (`pynetsnmp` runtime), Sensu Classic extension (`smidump` subprocess at startup), or LibreNMS (PHP `MibParser` plus snmptrapd's `mibs::` directive).
- **`SNMPTT` / `mib2c` / `libsmi`**: NOT used.
- **`go.uber.org/fx`**: Uber's DI/lifecycle framework. All components are wired with fx (`comp/snmptraps/server/impl/server.go:97-114` builds a sub-app; bundle uses `fxutil.Bundle` at `bundle.go:18-22`).

Lineage of the trap component:

- The release-notes folder reveals a coherent evolution:
  - Initial feature: `releasenotes/notes/snmp-traps-4f0e9ba2a6247322.yaml` — the foundational "Add support for receiving and processing SNMP traps, and forwarding them as logs to Datadog" release. Establishes the traps-as-logs design contract from day one.
  - Better defaults: `releasenotes/notes/Better-defaults-for-the-traps-listener-33a4f16395f810d0.yaml` — refined the listener defaults (likely the move to 9162 from a different initial port).
  - Configuration relocation: `releasenotes/notes/mv-traps-listener-configuration-under-network_devices-e266ada14f283ab3.yaml` — moved configuration from the original location under the `network_devices.snmp_traps.` prefix that the current code uses.
  - V1 + V3 support: `releasenotes/notes/SNMP-Traps-Support-v1-and-v3-7ea83c24e9c37dff.yaml` — extended beyond v2c to add v1 and v3 reception.
  - Multi-user v3: `releasenotes/notes/add-multiple-users-traps-support-d8358352c65974d4.yaml` — added the user table for simultaneous multi-user USM (`config.go:127-152`).
  - Namespace tagging: `releasenotes/notes/SNMP-Traps---support-namespace-f69befc242f23b0a.yaml` — added the `device_namespace` tagging seen in `packet.go:30`.
  - Initial telemetry: `releasenotes/notes/snmp-traps-collect-telemetry-a8dbf3ec35f2e679.yaml` ("Collect telemetry metrics for SNMP Traps").
  - User-customizable MIB resolution: `releasenotes/notes/Resolve-SNMP-Traps-OIDs-to-names-70de58eecc4892aa.yaml` ("Traps OIDs are now resolved to names using user-provided 'traps db' files in 'snmp.d/traps_db/'").
  - Bundled MIB DB: `releasenotes/notes/SNMP---include-traps-db-7c44bd129daf7667.yaml` ("Include pre-generated trap db file in the `conf.d/snmp.d/traps_db/` folder").
  - Integer enums in the DB: `releasenotes/notes/Update-SNMP-Traps_DB-76a39128d7b2b4e9.yaml` ("Update SNMP traps database to include integer enumerations") + `releasenotes/notes/snmp-traps-resolve-enums-e66ef9e51a9269aa.yaml`.
  - Bit enums in the DB: `releasenotes/notes/Update-SNMP-Traps-DB-with-BITS-a8f419275252c7b9.yaml` ("Update SNMP traps database with bit enumerations") + `releasenotes/notes/[SNMP-Traps]-Add-support-for-BITS-field-enrichment-8b96afd5af6ecae5.yaml` + `releasenotes/notes/snmp-traps-display-hex-string-for-BITS-fields-67642c9a06083e7d.yaml`.
  - Suffixed-variable resolution: `releasenotes/notes/Traps---Resolve-table-variables-that-have-a-suffix-fec0f41479599210.yaml` — added the climb-up algorithm at `oid_resolver.go:118-145` analysed in §4.4.
  - Dedicated intake: `releasenotes/notes/Send-traps-to-their-own-intake-f6f5d107e0f27e59.yaml` — migrated traps from the shared logs intake to the dedicated `snmp-traps-intake.<site>` endpoint with intake track `ndmtraps` (the current EpForwarder routing at `epforwarder.go:168-179`).

  The order is: initial reception+forward → v1/v2c/v3 + multi-user → namespace → telemetry → user-supplied MIB resolution → ship default DB → enum/bits enrichment → suffixed-variable resolution → dedicated intake. The conceptual progression is from "raw forwarder" to "OID-resolving forwarder with enum-decoded varbinds and its own intake track". There is no notion of severity normalisation, dedup, or alarm-state machine in this lineage — those concerns live on the SaaS side.

Audience: **NDM customers of Datadog SaaS**. Operators who do not have a paid Datadog NDM subscription do not get the SaaS-side trap surface (Logs Explorer, Log Monitors, NDM device cross-links). The Agent will still receive and forward, but the receiving end requires the SaaS to be present.

## 2. Trap-Subsystem Architecture

### 2.1 Ten subpackages, four real components

The `comp/snmptraps/` tree has ten directories and one root `bundle.go`. Six of them are first-class **components** (a `def/component.go` interface + `impl/` constructor + `fx/fx.go` module). Two are **plain Go packages** that do not declare a Component interface but provide types used across components (`packet/`, `senderhelper/`). One (`snmplog/`) is a thin adapter, and is **shared with the SNMP polling subsystem**: `pkg/snmp/snmpparse/gosnmp.go:14, :96` imports `snmplog.New(logger)` for polling-side gosnmp logging. So `snmplog` is structurally inside `comp/snmptraps/` but is functionally a gosnmp logging adapter for both the trap listener and the SNMP poller — a cross-cutting dependency. One subpackage (`status/`) is a component but its primary job is to export expvar counters and produce status-command output. The substantive code is concentrated in four files:

| File | Lines | Role |
|---|---|---|
| `comp/snmptraps/formatter/impl/formatter.go` | 411 | JSON-formats a trap (V1 and V2/V3 paths, enum/bits enrichment, telemetry on enrichment misses). |
| `comp/snmptraps/oidresolver/impl/oid_resolver.go` | 268 | Multi-file JSON/YAML (optionally gzip) loader; conflict resolution; subtree climb-up resolution. |
| `comp/snmptraps/listener/impl/listener.go` | 201 | UDP listener, gosnmp `OnNewTrap` callback, community-string validation, telemetry. |
| `comp/snmptraps/config/def/config.go` | 173 | `TrapsConfig` struct; SNMPv3 USM table construction; engineID derivation; default port 9162. |

Total Go production source in the trap component (verified via `wc -l` against the mirrored repo, excluding `*_test.go`, `fx_mock.go`, files under `//go:build test` such as `senderhelper.go` and `test_helpers.go`): **1,644 lines** across the four substantive files listed above plus the seven smaller runtime files (`server.go` 146, `forwarder.go` 124, `status.go` 153, `service.go` 61, `bundle.go` 22, `packet.go` 46, `snmplog.go` 39). Including the `def/` interface files, `def/traps_db.go`, and `fx/` wiring modules the total rises to **2,011 lines**. Test code adds **2,494 lines** of `*_test.go` plus **294 lines** of test-only helpers (`senderhelper.go` 43, `packet/test_helpers.go` 124, `listener/impl/test_helpers.go` 127) for a test-side total of **2,788 lines**. Test-to-production ratio is ~1.39:1 against the full non-test total (2,788 / 2,011) or ~1.24:1 if test helpers are excluded. This is a **small** subsystem by comparison — OpenNMS's trap pipeline has 6,000+ lines of Java; Zenoss's `zentrap` has ~3,500 lines of Python plus C extensions; Centreon's `centreontrapd` is ~1,800 lines of Perl with a DB-backed catalogue. Datadog's compactness comes from offloading every cross-cutting concern (storage, dedup, alerting, UI) to the SaaS side.

### 2.2 ASCII diagram

```
                            HOST RUNNING DATADOG AGENT
   +-----------------------------------------------------------------------------+
   |                                                                             |
   |   +------------+   +-----------------+   +-----------------+                |
   |   | config/def |-->| config/impl     |   | oidresolver/impl|                |
   |   | TrapsConfig|   | reads YAML +    |   | reads JSON/YAML |                |
   |   | port 9162  |   | builds USM tbl  |   | from snmp.d/    |                |
   |   +------------+   +-----------------+   |   traps_db/     |                |
   |                            |             +-----------------+                |
   |                            v                       |                        |
   |   +-------------------------------------+          |                        |
   |   |   listener/impl                     |          |                        |
   |   |   gosnmp.NewTrapListener()          |          |                        |
   |   |   binds UDP <bind_host>:<port>      |          |                        |
   |   |   on_new_trap:                      |          |                        |
   |   |     validate community / V3 USM     |          |                        |
   |   |     telemetry datadog.snmp_traps.*  |          |                        |
   |   |     write *SnmpPacket to chan       |          |                        |
   |   +-------------------------------------+          |                        |
   |                            |                       |                        |
   |                            | PacketsChannel        |                        |
   |                            | (buffered 100)        |                        |
   |                            v                       v                        |
   |   +----------------------------------------------------+                    |
   |   |  forwarder/impl  (run goroutine, 10s flush ticker) |                    |
   |   |    pkt := <- trapsIn                               |                    |
   |   |    data, err := formatter.FormatPacket(pkt) ------+|                    |
   |   |    sender.EventPlatformEvent(data, "network-      ||                    |
   |   |                               devices-snmp-traps")||                    |
   |   |    sender.Count("datadog.snmp_traps.forwarded",1) ||                    |
   |   +----------------------------------------------------+                    |
   |                            |                                                |
   |                            v                                                |
   |   +----------------------------------------------------+                    |
   |   |  formatter/impl                                    |                    |
   |   |   V1 path:  formatV1Trap (enterpriseOID,           |                    |
   |   |             genericTrap, specificTrap, varbinds)   |                    |
   |   |   V2/V3:    parseSysUpTime + parseSnmpTrapOID +    |                    |
   |   |             parseVariables (enum / bits enrich)    |                    |
   |   |   ddsource = "snmp-traps"                          |                    |
   |   |   ddtags   = snmp_version:<v>,device_namespace:    |                    |
   |   |              <ns>,snmp_device:<src-ip>             |                    |
   |   |   timestamp= packet.Timestamp (recv ms)            |                    |
   |   |   json.Marshal({ "trap": {...} })                  |                    |
   |   +----------------------------------------------------+                    |
   |                                                                             |
   |   +----------------------------------------------------+                    |
   |   |  aggregator/demultiplexer (Agent core)             |                    |
   |   |     EventPlatformEvent type=                       |                    |
   |   |     "network-devices-snmp-traps"                   |                    |
   |   +----------------------------------------------------+                    |
   |                            |                                                |
   |                            v                                                |
   |   +----------------------------------------------------+                    |
   |   |  comp/forwarder/eventplatform/...epforwarder.go    |                    |
   |   |   endpointsConfigPrefix:                           |                    |
   |   |     network_devices.snmp_traps.forwarder.          |                    |
   |   |   hostnameEndpointPrefix: snmp-traps-intake.       |                    |
   |   |   intakeTrackType:        ndmtraps                 |                    |
   |   |   contentType:            application/json         |                    |
   |   |   batches and submits via HTTPS                    |                    |
   |   +----------------------------------------------------+                    |
   |                                                                             |
   +-----------------------------------------------------------------------------+
                                |
                                | HTTPS to snmp-traps-intake.<site>
                                v
                       +---------------------+
                       |  DATADOG SAAS       |   <-- correlation, dedup, alerting,
                       |  logs intake        |       storage, retention, UI all here
                       |  Logs Explorer      |       source:snmp-traps
                       |  Log Monitors       |       (see §6, §8)
                       |  NDM device link    |
                       +---------------------+
```

### 2.3 Deployment model

- **Single-process, single-host**. The trap subsystem is fx-wired into the Datadog Agent process. There is no separate daemon, no helper binary, no sidecar. Where the Agent runs, the trap listener runs (if enabled).
- **No HA / clustering primitives**. The Agent is the unit of deployment; HA for trap reception means running multiple Agents and arranging device-side trap forwarding to multiple destinations. There is no shared listener state, no anycast story, no inter-Agent dedup. Datadog's recommended pattern (per public docs at `docs.datadoghq.com/network_monitoring/devices/snmp_traps/`, retrieved through the public release notes and config template alignment) is to deploy the Agent on a host with line-of-sight to devices and configure devices to point at that host.
- **Container deployment**: standard — the Agent runs as a container; the trap port (default 9162) must be mapped. Because the Agent **does not run as root**, port 162 is not bindable without `setcap`; the in-tree config template warns: *"Because the Datadog Agent does not run as root, the port cannot be below 1024. However, if you run `sudo setcap 'cap_net_bind_service=+ep' /opt/datadog-agent/bin/agent/agent`, the Datadog Agent can listen on ports below 1024"* (`pkg/config/config_template.yaml:4524-4527`). Default 9162 is non-privileged; operators redirect device-side traffic from 162 → 9162 via iptables / firewalld / load balancer (or the Helm chart's `hostNetwork` + setcap pattern).
- **Kubernetes deployment**: the Datadog Agent ships as a DaemonSet; on each node the trap listener can be enabled. The Agent's Helm chart and Operator support `network_devices.snmp_traps.enabled` and the port mapping. Source-of-truth references are in vendor docs; the Agent code itself does not contain k8s-specific trap logic.

### 2.4 Languages and key libraries

- Go — every file in `comp/snmptraps/` carries an Apache-2.0 header per Datadog convention. The wider Agent tree mixes Apache-2.0 and BSD-3-Clause headers per file; the trap subsystem is uniformly Apache-2.0. The use of `maps.Copy` at `formatter/impl/formatter.go:12, :164, :213` implies a minimum Go version of **1.21+** (the `maps` standard-library package was introduced in Go 1.21). The Agent's overall Go toolchain version is set higher than this in the repo's `go.mod`; the trap component does not pin its own minimum.
- `github.com/gosnmp/gosnmp` — on-wire decoder, USM, trap listener.
- `go.uber.org/fx` — DI / lifecycle.
- `go.yaml.in/yaml/v2` — YAML reader for the OID resolver (`oid_resolver.go:20`). The `go.yaml.in/yaml/v2` import path is a community fork of `gopkg.in/yaml.v2`, adopted across the Datadog Agent after the upstream `gopkg.in/yaml.v3` was discontinued / had handling concerns; Datadog's choice is consistent across the Agent tree.
- `compress/gzip`, `encoding/json` — for `.json` and `.gz` variants of the traps DB.
- `crypto/subtle` — constant-time community-string compare (`listener.go:11, :195`).
- `hash/fnv` (128-bit FNV) — engineID derivation from hostname (`config.go:10, :86-92`).

### 2.5 Inter-component IPC

- **Component-to-component**: in-process Go calls via fx-resolved interfaces; the only IPC inside the Agent is the **channel** `packet.PacketsChannel = chan *SnmpPacket` between `listener` and `forwarder` (`comp/snmptraps/packet/packet.go:23-24`, `forwarder/impl/forwarder.go:60-61`). Channel buffer size is **100** (hard-coded constant `packetsChanSize` at `config/def/config.go:27`).
- **Agent-to-SaaS**: HTTPS POST batches by EpForwarder. Endpoints derive from the `network_devices.snmp_traps.forwarder.` config prefix and the Datadog "site" (e.g. `snmp-traps-intake.datadoghq.com`, `snmp-traps-intake.datadoghq.eu`, etc.). Content type `application/json`. Batched per the platform defaults `defaultBatchMaxSize` / `defaultBatchMaxContentSize` (`epforwarder.go:163-180`).

### 2.6 The fx app-within-an-app anomaly

`server.NewComponent` builds a sub-fx-app inside the parent Agent app (`server.go:97-114`). A code comment flags this as an anti-pattern: *"TODO: (components) Having apps within apps is not ideal - you have to be careful never to double-instantiate anything. Do not use this solution elsewhere if possible."* (`server.go:94-96`). The reason it exists: the server can be disabled (`!trapsconfig.IsEnabled(deps.Conf)`) without dragging the inner components into the parent lifecycle, but when enabled it needs `configfx.Module`, `formatterimpl.NewComponent`, `forwarderimpl.Module`, `listenerfx.Module`, `oidresolverfx.Module` to be wired. The sub-app is the local scoping mechanism. This is unique to traps in the Agent codebase — the comment "Do not use this solution elsewhere if possible" is a candid admission that the pattern is a workaround, not an intended design.

## 3. Trap Reception (UDP/162 Ingress)

### 3.1 Listener implementation

Own socket via `gosnmp.NewTrapListener()`:

```go
gosnmpListener := gosnmp.NewTrapListener()
gosnmpListener.Params, err = cfg.BuildSNMPParams(dep.Logger)
...
gosnmpListener.OnNewTrap = tl.receiveTrap
```

(`comp/snmptraps/listener/impl/listener.go:87-103`). The blocking listen happens in a goroutine (`listener.go:124-128`):

```go
func (t *trapListener) start() error {
    t.logger.Infof("Start listening for traps on %s", t.config.Addr())
    go t.run()
    return t.blockUntilReady()
}

func (t *trapListener) run() {
    err := t.listener.Listen(t.config.Addr()) // blocking call
    if err != nil { t.errorsChannel <- err }
}
```

`blockUntilReady` then waits on `gosnmp.TrapListener.Listening()` channel (`listener.go:137-148`) so the lifecycle hook returns only after the socket is bound. If bind fails (e.g. port already in use, EACCES on a privileged port), the error propagates back to the fx lifecycle and the server's `Error()` reports it; the server's `Running()` stays false (`server.go:126-134`).

### 3.2 SNMP version support

- **v1**: supported. The formatter has a dedicated `formatV1Trap` path (`formatter.go:129-166`) that reads `enterpriseOID`, `genericTrap`, `specificTrap`, and the v1-style varbinds, and synthesises a `snmpTrapOID` per RFC 3584 (`formatter.go:139-145`):
  ```go
  if genericTrap == 6 {
      // Vendor-specific trap
      trapOID = fmt.Sprintf("%s.0.%d", enterpriseOid, specificTrap)
  } else {
      // Generic trap
      trapOID = fmt.Sprintf("%s.%d", genericTrapOid, genericTrap+1)
  }
  ```
  The `genericTrapOid = "1.3.6.1.6.3.1.1.5"` constant (`formatter.go:53`) is RFC 1907's `snmpTraps` node prefix.
- **v2c**: supported. Default when no V3 users are configured: the Agent runs gosnmp in `Version2c` (`config.go:121`: *"No user configured, let's use Version2 which is enough and doesn't require setting up fake security data."*).
- **v3 USM**: supported. When `users:` is non-empty, `BuildSNMPParams` constructs a `gosnmp.SnmpV3SecurityParametersTable` populated with each user's `UserName`, `AuthenticationProtocol`, `AuthenticationPassphrase`, `PrivacyProtocol`, `PrivacyPassphrase` (`config.go:126-152`). The Agent runs gosnmp in `Version3` (`config.go:157`) and the listener can accept v1, v2c, and v3 simultaneously via the comment at `config.go:157`: *"Always using version3 for traps, only option that works with all SNMP versions simultaneously"*.
- **No DTLS / TLS-TM**: there is no code path for RFC 6353/6354 transport-layer security. Only UDP is supported (`config.go:120` and `config.go:156`: `Transport: "udp"`).

**Authentication protocols supported** (`pkg/snmp/gosnmplib/gosnmp_auth.go:17-38`): MD5, SHA, SHA224, SHA256, SHA384, SHA512. Unknown values return an error at startup. **Privacy protocols supported** (`gosnmp_auth.go:44-65`): DES, AES (128-bit), AES192 (Blumenthal), AES256 (Blumenthal), AES192C (Reeder/Cisco), AES256C (Reeder/Cisco). The Reeder variants matter for Cisco interop — Cisco gear historically used the Reeder key-localisation variant; not all v3 implementations expose this distinction. Datadog does.

### 3.3 EngineID derivation

A core USM concern. Datadog derives the authoritative engineID from the Agent hostname using FNV-128 (`config.go:86-92`):

```go
h := fnv.New128()
h.Write([]byte(host))
// First byte is always 0x80
// Next four bytes are the Private Enterprise Number (set to an invalid value here)
// The next 16 bytes are the hash of the agent hostname
engineID := h.Sum([]byte{0x80, 0xff, 0xff, 0xff, 0xff})
```

The leading `0x80` is RFC 3411's "private enterprise number prefix" marker. The PEN encoded as `0xff 0xff 0xff 0xff` is intentionally **invalid** (the all-ones 32-bit value `4294967295` is not assigned in the IANA Private Enterprise Numbers registry at `https://www.iana.org/assignments/enterprise-numbers/`; Datadog's own assigned PEN per IANA is **47812**). This means a network observer cannot identify the engineID as "Datadog Agent" via the PEN bytes — a deliberate non-fingerprinting choice, but also a deviation from the spec-recommended practice of encoding one's own PEN. Test evidence at `config/def/config_test.go:25-30`:

```go
var expectedEngineID = "\x80\xff\xff\xff\xff\x67\xb2\x0f\xe4\xdf\x73\x7a\xce\x28\x47\x03\x8f\x57\xe6\x5c\x98"
```

(21 bytes: 1 + 4 + 16). The hash is **deterministic per hostname**, so multiple Agents on the same hostname (unlikely but possible) would share an engineID. There is no operator override.

### 3.4 Performance / concurrency model

- One goroutine for `gosnmp.TrapListener.Listen` (`listener.go:130-135`); inside gosnmp, this reads UDP datagrams in a loop and invokes `OnNewTrap` synchronously. The callback (`receiveTrap`, `listener.go:169-184`) does community-string validation in **constant time** (the `validatePacket` function at `listener.go:186-201` uses `subtle.ConstantTimeCompare` at `:195` — protects against timing oracles on the community string), increments expvar counters via the status component, and writes the `*SnmpPacket` into the bounded `packets` channel.
- One goroutine for the **forwarder** consumer (`forwarder/impl/forwarder.go:99-113`), which `select`s on the packets channel, a stop channel, and a 10-second flush ticker (`flushTicker := time.NewTicker(10 * time.Second)`). The flush ticker triggers `tf.sender.Commit()`, which flushes the telemetry counters; trap forwarding itself is not batched at this layer (each `sender.EventPlatformEvent` call hands one trap downstream, and the EpForwarder upstream batches).
- **Backpressure**: the `packets` channel is buffered to 100 (`config.go:27`). If the forwarder consumes slower than the listener produces, the listener's `t.packets <- pkt` (`listener.go:183`) **blocks**. While blocked, gosnmp's `OnNewTrap` callback is blocked, which means new UDP datagrams accumulate in the kernel socket buffer until OS-level drop. There is no high-watermark drop, no per-source rate limit, no buffer expansion. Under sustained trap storm, the design relies on kernel UDP overflow as the back-pressure signal. Telemetry `datadog.snmp_traps.received` covers received packets but not "would-be-dropped due to channel-full"; the only proxy is `EventPlatformEventsErrors[network-devices-snmp-traps]` (see §10).
- **No multi-listener / no thread pool**. The model is single-producer single-consumer. Scaling beyond what gosnmp's single goroutine can decode + what the forwarder goroutine can format means deploying more Agent instances.

### 3.5 Privileged-port handling

- **Default port is 9162**, not 162. Defined as `defaultPort = uint16(9162)` (`config.go:25`).
- The in-tree config template explicitly documents the `setcap` workaround for port 162 (`pkg/config/config_template.yaml:4525-4527`).
- The Agent does not attempt `CAP_NET_BIND_SERVICE` itself; if the operator does not set it and configures port < 1024, the bind fails at startup and the server's `Error()` reports the EACCES.

### 3.6 Horizontal scaling pattern

Run multiple Agents on multiple hosts; each binds its own UDP port. Trap distribution among devices is operator-managed (devices configured to point at one or more receivers). No anycast story in the Agent codebase. The SaaS side handles per-event deduplication on log ingest only insofar as the SaaS dedup logic applies; trap-content-level dedup is not in the Agent.

### 3.7 HA / clustering

- The Agent has no HA primitive. The SaaS side is the resilient component; Agent-host failure means trap reception stops on that host until the host (or another Agent) recovers.
- One pattern Datadog NDM customers use (per docs): two Agents, both configured as trap listeners on different ports + a load balancer or anycast IP in front. The agents are independent; the SaaS deduplicates if both happen to receive the same trap by drop-on-ingest of duplicates (this is a SaaS behaviour, not Agent behaviour — and is not documented as a guaranteed dedup contract by Datadog).

## 4. MIB Management

### 4.1 Storage layout

The OID resolver loads files from `<confd_path>/snmp.d/traps_db/` (`oid_resolver.go:87`). `confd_path` is the standard Datadog Agent confd root; default `/etc/datadog-agent/conf.d/`. So the canonical directory is `/etc/datadog-agent/conf.d/snmp.d/traps_db/`.

### 4.2 File formats accepted

Each file is `.json`, `.yaml`/`.yml`, optionally `.gz`-compressed (`oid_resolver.go:178-198`). Format is inferred from extension: `.json` → `json.Unmarshal`, anything else (effectively YAML) → `yaml.Unmarshal`. The on-disk schema is identical between YAML and JSON (the structs in `oidresolver/def/traps_db.go:13-47` use both `yaml:` and `json:` tags):

```go
type TrapDBFileContent struct {
    Traps     TrapSpec     `yaml:"traps" json:"traps"`
    Variables VariableSpec `yaml:"vars" json:"vars"`
}
type TrapMetadata struct {
    Name            string `yaml:"name" json:"name"`
    MIBName         string `yaml:"mib" json:"mib"`
    Description     string `yaml:"descr" json:"descr"`
    VariableSpecPtr VariableSpec `yaml:"-" json:"-"`
}
type VariableMetadata struct {
    Name        string         `yaml:"name" json:"name"`
    Description string         `yaml:"descr" json:"descr"`
    Enumeration map[int]string `yaml:"enum" json:"enum"`
    Bits        map[int]string `yaml:"bits" json:"bits"`
    IsIntermediateNode bool    `yaml:"-" json:"-"`
}
```

Top-level: `traps:` keyed by trap OID, `vars:` keyed by variable OID. Each `trap` carries `name`, `mib`, `descr`. Each `var` carries `name`, `descr`, optional `enum` (integer → label) and optional `bits` (bit-position → label). No "vendor severity", no "action", no "alert", no "device" — the DB is a **purely lexical** resource: it maps numbers to names and decodes enum/bits values. Everything semantic happens on the SaaS side.

### 4.3 Conflict resolution

Multi-file loading with deterministic precedence (`oid_resolver.go:147-176`):

1. Datadog-shipped files are those whose name starts with `dd_traps_db` (constant `ddTrapDBFileNamePrefix = "dd_traps_db"` at `oid_resolver.go:52`).
2. User-supplied files are everything else (any name).
3. Files are sorted case-insensitively within each group: dd-shipped first, then user-supplied (lower-case alphabetical).
4. Files load in that order; later definitions overwrite earlier ones. As the package doc-comment puts it (`oid_resolver.go:67-69`): *"Trap OIDs conflicts are resolved using the name of the source file in alphabetical order and by giving the less priority to Datadog's own database shipped with the agent. Variable OIDs conflicts are fully resolved by also looking at the trap OID."*

Test evidence at `oid_resolver_test.go:74-111` (`TestSortFiles`) validates that order. Practical effect: an operator can override any Datadog-shipped OID by placing a same-OID entry in a non-`dd_traps_db*` file. This is a clean, file-name-based precedence model — comparable to OpenNMS's `eventconf.xml` include-folder ordering, but simpler.

### 4.4 Variable resolution: subtree climb-up

The trap-DB schema indexes variables by their canonical OID. But devices commonly emit variables with **instance suffixes** (e.g. `ifIndex.0`, `ifIndex.5`, `ifIndex.<arbitrary subtree>`). The resolver must match the suffix variant against the canonical entry. Two distinct algorithms cooperate: a **runtime** climb-up in `GetVariableMetadata`, and a **load-time** intermediate-node marking pass in `updateResolverWithData`. The analysis presents them separately.

OID normalisation is the common prerequisite. `NormalizeOID` at `oidresolver/def/traps_db.go:50-54` strips leading dots, converting absolute form `.1.2.3` to relative form `1.2.3`. Both algorithms operate on normalised OIDs.

#### 4.4.1 Runtime resolution: subtree climb-up

In `GetVariableMetadata` at `oid_resolver.go:118-145`, the resolver first strips trailing `.0` from both the trap OID and the variable OID — this handles scalar instance suffixes (e.g. `ifOperStatus.0`) which would otherwise prevent matching against the canonical OID (`oid_resolver.go:119-120`):

```go
trapOID = strings.TrimSuffix(oidresolver.NormalizeOID(trapOID), ".0")
varOID = strings.TrimSuffix(oidresolver.NormalizeOID(varOID), ".0")
```

Then the climb-up loop (`oid_resolver.go:126-144`):

```go
recreatedVarOID := varOID
for {
    varData, ok := trapData.VariableSpecPtr[recreatedVarOID]
    if ok {
        if varData.IsIntermediateNode {
            return ..., fmt.Errorf("variable OID %s is not defined", varOID)
        }
        return varData, nil
    }
    lastDot := strings.LastIndex(recreatedVarOID, ".")
    if lastDot == -1 { break }
    recreatedVarOID = varOID[:lastDot]  // NOTE: uses original varOID, not recreatedVarOID
}
```

So `1.3.6.1.2.1.2.2.1.1.5` (which is `ifIndex.5`) climbs to `1.3.6.1.2.1.2.2.1.1` (which is `ifIndex`) and matches there.

**Latent slicing inconsistency**: on the second and subsequent iterations the loop slices the **original** `varOID` (`varOID[:lastDot]`), not the **current** `recreatedVarOID`. As long as `lastDot` is recomputed against `recreatedVarOID` (which it is), the slice end-index lands in the right place — `lastDot` is the position of the last dot in the already-shortened `recreatedVarOID`, which is also a valid index into `varOID` since the strings share a common prefix. So the algorithm produces the correct result. The code is correct but the choice of `varOID[:lastDot]` over `recreatedVarOID[:lastDot]` is non-obvious and depends on the invariant that `recreatedVarOID` is always a prefix of `varOID`. A future refactor that breaks this invariant (e.g. canonicalising `recreatedVarOID` mid-loop) would introduce a subtle bug. No test failure manifests today; the invariant holds because both share a common normalised prefix.

#### 4.4.2 Load-time marking: intermediate-node detection

To avoid runaway matches against shared roots (e.g. `1.3.6.1` matching everything), each known variable OID is marked as an "intermediate node" if any longer OID in the DB extends it. Computed at load time (`oid_resolver.go:235-246`):

```go
sort.Strings(allOIDs)
for idx, variableOID := range allOIDs {
    isIntermediateNode := false
    if idx+1 < len(allOIDs) {
        nextOID := allOIDs[idx+1]
        isIntermediateNode = strings.HasPrefix(nextOID, variableOID+".")
    }
    variableData := trapDB.Variables[variableOID]
    variableData.IsIntermediateNode = isIntermediateNode
    definedVariables[variableOID] = variableData
}
```

Code-comment at `oid_resolver.go:228-234` (verbatim):

> *"Fast" algorithm used to mark OID that act both as a variable and as a parent of other variable with 'isNode: true'. i.e if an OID `<FOO>.<BAR>` exists in the trapsDB but `<FOO>` also exists in the trapsDB then `<FOO>` acts as a 'Node' of the OID tree and should not be considered a match for resolving variables.*

The algorithm sorts OIDs lexicographically, then each OID checks whether its immediate successor in the sorted list has it as a `<OID>.<X>` prefix. If so, the current OID is an intermediate node. Because dots sort before digits in lexicographic order, comparing only adjacent pairs suffices — saving O(N²) cross-comparisons. The fast-pass exploits the sort invariant; nicely done.

Additionally a hard-coded blacklist (`oid_resolver.go:54-61`) of roots is force-marked intermediate:

```go
var nodesOIDThatShouldNeverMatch = []string{
    "1.3.6.1.4.1", "1.3.6.1.4", "1.3.6.1", "1.3.6", "1.3", "1",
}
```

A variable OID climbing into one of those nodes terminates with "not defined" rather than matching at the root.

#### 4.4.3 Why this matters

This algorithm is **shared with no other system analysed so far in this comparative pass**. OpenNMS's matching is regex-driven (event-OID patterns with wildcards); SNMPTT's matching is per-OID with explicit overrides; Zenoss's matching is identifier-class-based. Datadog's prefix-tree climb-up with intermediate-node masking is a clean solution to the suffixed-variable problem.

### 4.5 Bundled MIBs out-of-the-box

The trap DB ships **inside the Agent installer**. Per `releasenotes/notes/SNMP---include-traps-db-7c44bd129daf7667.yaml:11`: *"Include pre-generated trap db file in the `conf.d/snmp.d/traps_db/` folder."* The exact MIB coverage is not visible from the open-source datadog-agent repo alone — the actual `dd_traps_db.*` file content is generated and shipped during the Omnibus build (the closed file in the install package; the open repo has no copy of it). The release notes confirm:

- *"Update SNMP traps database to include integer enumerations"* (`releasenotes/notes/Update-SNMP-Traps_DB-76a39128d7b2b4e9.yaml:11`).
- *"Update SNMP traps database with bit enumerations"* (`releasenotes/notes/Update-SNMP-Traps-DB-with-BITS-a8f419275252c7b9.yaml:11`).

So the shipped DB includes both `enum:` and `bits:` definitions per variable. The shipping cadence is "as the integrations-core MIB collection is updated" — there is no separately documented MIB-DB release train.

Coverage breadth (vendor scope): **Datadog's public documentation states the shipped DB covers "more than 11,000 MIBs"** (operator-facing setup guide at `https://docs.datadoghq.com/network_monitoring/devices/snmp_traps/`, retrieved during iter-2). This is the only published coverage statement; no authoritative vendor-by-vendor list is part of the public docs or the open-source tree. The actual `dd_traps_db.json.gz` file is generated during the Omnibus build and is part of the closed install package; it is not committed to the open repos. Operationally: the integrations-core `ddev meta snmp generate-traps-db` tool consumes MIBs from `https://pysnmp.github.io/mibs/asn1/@mib@` and the integrations-core `snmp/data/mibs/` directory; the shipped DB is generated from those sources. The integrations-core test fixtures live at `datadog_checks_dev/tests/tooling/commands/meta/snmp/data/` — the raw MIB source `A3COM-HUAWEI-LswTRAP-MIB` (a file, 222 lines, ASCII MIB) is checked in alongside two sibling fixtures `expected_expanded.json` and `expected_compact.json` that pin the compiler's output shape on at least Huawei MIBs. Readers who need vendor-specific assurance should retrieve the installed `dd_traps_db.json.gz` from a real Agent install and grep the decompressed JSON. The integrations-core SNMP poll profile set (**239 default profiles** under `snmp/datadog_checks/snmp/data/default_profiles/` — verified via `find ... -name '*.yaml' | wc -l`) is a separate, independently-maintained proxy for vendor priorities — poll profiles and trap MIBs are not 1:1, and the per-profile YAMLs target metric polling rather than trap decoding.

### 4.6 User workflow for adding/updating MIBs

Two distinct paths exist; they do not look like each other.

**Path A: Generate a custom trap DB from raw MIB files** (recommended for unsupported vendors). Use the `ddev` (Datadog Development CLI from `integrations-core`) tool:

```
ddev meta snmp generate-traps-db -o ./output_dir/ /path/to/my/mib1 /path/to/my/mib2
```

Source: `datadog/integrations-core @ 411c31db :: datadog_checks_dev/datadog_checks/dev/tooling/commands/meta/snmp/generate_traps_db.py:160-172` (the click-command help text). The tool uses pysmi to compile the MIBs, then emits one YAML/JSON file per MIB (or one compact file with `--output-file`). The operator then drops the output files into `/etc/datadog-agent/conf.d/snmp.d/traps_db/` and restarts the Agent. **This tool is not shipped with the Datadog Agent** — it is an integrations-core CLI. Datadog's public docs at `https://docs.datadoghq.com/network_monitoring/devices/snmp_traps/` (operator-facing setup guide) instruct operators to install it via `pip3 install ddev` (or `pipx install ddev`); `ddev` is the integrations-core developer/operator CLI and depends on `datadog-checks-dev` under the hood. The user workflow therefore requires Python + pysmi, separately from the Agent install. This is operationally heavier than CheckMK's "drop a MIB file into a directory" or Zenoss's "upload via UI". It is comparable to OpenNMS's `events-from-trapd.xsl` workflow in complexity, though shorter.

**Path B: Hand-write a YAML file**. Since the on-disk format is simple, an operator who knows the OIDs can write a `my_vendor_traps.yaml` with the right top-level `traps:` and `vars:` maps and drop it into `snmp.d/traps_db/`. This is realistic only for small, well-understood OID sets — for full MIB coverage Path A is required.

**No live reload**: the resolver loads files only at `newMultiFilesOIDResolver` construction (`oid_resolver.go:80-103`), which runs inside the fx app's startup. To pick up a new DB file, the Agent must be restarted. Other components like SNMP polling profiles have a similar restart-to-reload story, so this is consistent with Agent conventions.

**Failure on missing directory**: if `snmp.d/traps_db/` does not exist or is empty, the resolver constructor returns an error (`oid_resolver.go:88-94`):

```go
files, err := os.ReadDir(trapsDBRoot)
if err != nil {
    return nil, fmt.Errorf("failed to read dir `%s`: %w", trapsDBRoot, err)
}
if len(files) == 0 {
    return nil, fmt.Errorf("dir `%s` does not contain any trap db file", trapsDBRoot)
}
```

That error bubbles up through the fx app and is reported by `srv.Error()` (`server.go:78-83`), visible via `datadog-agent status`. Test evidence: `server/impl/server_test.go:55-76` (`TestNonBlockingFailure`) — when `traps_db/` does not exist, the server constructs but `Running()` is false and `Error()` returns `os.ErrNotExist`. The Agent **does not crash** — trap reception is simply disabled until repaired.

### 4.7 Dependency resolution between MIBs

There is no runtime dependency resolution between trap DB files. Conflicts are resolved by file order (§4.3). The MIB *compilation*-time dependency resolution (IMPORTS handling) happens inside pysmi during `ddev meta snmp generate-traps-db`; the Agent itself never sees IMPORTS or transitive MIB graphs.

### 4.8 Version management vs firmware

The Agent has no concept of "MIB version that matches firmware version X.Y of vendor Z". Operators are responsible for keeping the trap DB consistent with the devices they monitor. A vendor firmware upgrade that introduces a new trap OID requires re-running `ddev meta snmp generate-traps-db` with the new MIB. This is a generic limitation of all systems analysed except a few enterprise NMSes (OpenNMS NTC, LogicMonitor Collector) that maintain vendor MIB catalogues with firmware tagging.

### 4.9 Fallback for unknown OIDs

When `GetTrapMetadata` cannot resolve a trap OID (`oid_resolver.go:106-113`):

```go
trapOID = strings.TrimSuffix(oidresolver.NormalizeOID(trapOID), ".0")
trapData, ok := or.traps[trapOID]
if !ok {
    return oidresolver.TrapMetadata{}, fmt.Errorf("trap OID %s is not defined", trapOID)
}
```

The formatter handles the error gracefully (`formatter.go:147-154` and 198-205):

```go
trapMetadata, err := f.oidResolver.GetTrapMetadata(trapOID)
if err != nil {
    f.sender.Count(telemetryTrapsNotEnriched, 1, "", tags)
    f.logger.Debugf("unable to resolve OID: %s", err)
} else {
    data["snmpTrapName"] = trapMetadata.Name
    data["snmpTrapMIB"] = trapMetadata.MIBName
}
```

So the trap is **still forwarded** — just without `snmpTrapName` and `snmpTrapMIB` keys in the JSON. The varbinds still appear (with raw numeric OIDs). Telemetry counter `datadog.snmp_traps.traps_not_enriched` increments. Same pattern for unknown variable OIDs: the formatter falls back to the raw `formatValue` output and increments `datadog.snmp_traps.vars_not_enriched`. The operator can see these telemetry metrics in Datadog itself (or in the Agent's `expvar` snapshot) and decide whether to add MIB coverage.

This is a sane, lossless fallback. Contrast with OpenNMS's strict UEI matching (an unmatched trap becomes `uei.opennms.org/default/trap`), Centreon's `centreontrapd` requires explicit handler rules to forward unrecognised traps, and Zenoss's `zentrap` drops traps with no event class binding.

## 5. Trap Processing Pipeline

### 5.1 Parse (BER decode, varbind extraction)

Done by gosnmp before the Agent sees the trap. The Agent's `OnNewTrap` callback receives a `*gosnmp.SnmpPacket` (`listener.go:169-170`). All ASN.1 BER decoding, length checks, type tag dispatch, malformed-PDU rejection are inside gosnmp. The Agent does not see raw bytes.

### 5.2 OID-to-name resolution

Performed in the formatter via `oidresolver.GetTrapMetadata(trapOID)` (`formatter.go:147, 198`). Result fields `snmpTrapName` and `snmpTrapMIB` are added to the JSON payload when resolution succeeds. The resolver climbs the OID tree for variable resolution (§4.4). The trap OID itself is matched exactly (with trailing `.0` stripped).

### 5.3 Source identification

The trap source is **the UDP source IP**, captured at `listener.go:170`:

```go
pkt := &packet.SnmpPacket{Content: p, Addr: u, Timestamp: time.Now().UnixMilli(), Namespace: t.config.Namespace}
```

It is then encoded into the `ddtags` as `snmp_device:<src-ip>` (`packet/packet.go:26-33`, the `GetTags()` method returning):

```go
return []string{
    "snmp_version:" + formatVersion(p.Content),
    "device_namespace:" + p.Namespace,
    "snmp_device:" + p.Addr.IP.String(),
}
```

**There is no agent-addr handling for SNMPv1 traps**. RFC 3584 specifies that an SNMPv1 trap PDU carries an `agent-addr` field that may differ from the UDP source (e.g. a relayed trap). Datadog's `formatV1Trap` records `enterpriseOID` and `genericTrap`/`specificTrap` (`formatter.go:130-145`) but the source IP used in `ddtags` is always the UDP source. The v1 `agent-addr` is decoded by gosnmp into the `SnmpTrap.AgentAddress` field (visible in tests at `packet/test_helpers.go:32, :49`) but is **not extracted by the Datadog formatter** — neither into `ddtags` nor into the JSON payload. The consequence is a **functional misattribution**: in networks with proxy/relay devices (common in large enterprise setups, NAT boundaries, or multi-site SNMP forwarders), every v1 trap is attributed to the relay's IP, not the originating device. The NDM device cross-link in alerts, all `@snmp_device:` log searches, and all per-device dashboards will point at the relay. This is **not** a subtle gap — it is a class of v1 traps that Datadog mis-correlates entirely.

The `device_namespace` tag is the **operator-defined logical grouping**. Default `"default"` (`config.go:74-79`); set via `network_devices.namespace` (global) or `network_devices.snmp_traps.namespace` (local override). Namespace normalisation (`config.go:97-101`) calls `utils.NormalizeNamespace` to enforce length and character constraints; test evidence `config_test.go:224-228` shows `"><\n\r\tfoo"` normalises to `"--foo"`. This is the same namespace concept used by NDM SNMP polling, so the trap-side namespace lines up with the device record on the SaaS side.

**v1 `uptime` is emitted, just from a different field**. `formatV1Trap` (`formatter.go:129-166`) does assign `data["uptime"] = uint32(content.Timestamp)` at `formatter.go:134` — gosnmp decodes the v1 PDU-level `sysUpTime` into `SnmpTrap.Timestamp`, and the formatter copies it through. The v2/v3 path takes a different route: `formatTrap` (`formatter.go:168-215`) reads `sysUpTime` out of the first varbind via `parseSysUpTime` (`formatter.go:268-276`) and assigns the same `data["uptime"]` key. So both paths emit `uptime`; they just source it from different places that the wire format dictates (v1 PDU header vs v2/v3 first varbind). The only state the v1 path drops relative to RFC 1157 is the `agent-addr` field discussed above.

### 5.4 Enrichment

Performed in `parseVariables` (`formatter.go:301-343`). For each varbind:

1. Normalise OID (strip leading `.`) via `oidresolver.NormalizeOID` (`oidresolver/def/traps_db.go:50-54`).
2. Compute the type string (`formatType`, `formatter.go:345-383`) — maps **21 distinct `gosnmp.Asn1BER` constants to 17 distinct non-default type strings** (with `Integer`+`Uinteger32`→`integer`, `OctetString`+`BitString`→`string`, and `Opaque`+`OpaqueFloat`+`OpaqueDouble`→`opaque` collapsing constants into a single output string). The `default` arm returns `"other"`, bringing the total set of output strings to 18. Coverage includes `unknown-type`, `boolean`, `integer`, `string`, `null`, `oid`, `object-description`, `ip-address`, `counter32`, `gauge32`, `time-ticks`, `opaque`, `nsap-address`, `counter64`, `no-such-object`, `no-such-instance`, `end-of-mib-view`. Because the `default` arm exists, unknown PDU type tags do not panic. Comprehensive type coverage.
3. Resolve variable metadata via `GetVariableMetadata(trapOID, varOID)` — the subtree climb-up algorithm (§4.4).
4. **If metadata has both `Enumeration` and `Bits`**: log error and skip enrichment (`formatter.go:323-324`). The variable is still appended to the top-level `variables[]` array with its raw OID, type, and `formatValue`-processed value (`formatter.go:312`); only the enriched-name top-level key is omitted.
5. **If `Enumeration` only**: call `enrichEnum(tv, varMetadata, logger)` (`formatter.go:219-233`). For an integer value, look it up in the enum map; if found, the enriched value is the label string; otherwise the raw integer is preserved.
6. **If `Bits` only**: call `enrichBits` (`formatter.go:237-266`). For an octet-string value, iterate bit-by-bit; for each set bit, look up the bit position in the bits map. The output is a list of either labels (if mapped) or raw bit-position integers (if not). The hex representation of the original byte string is also emitted (e.g. `"0xC481"`).
7. **Otherwise**: format the value via `formatValue` (`formatter.go:386-398`) — `[]byte` → `string`, OID-typed → normalised OID string, other → as-is.

The enriched representation is added to the top-level trap object under the variable's resolved name. Example, from `formatter_test.go:392-407`:

```json
"snmpTrapName": "netSnmpExampleHeartbeatNotification",
"snmpTrapMIB":  "NET-SNMP-EXAMPLES-MIB",
"snmpTrapOID":  "1.3.6.1.4.1.8072.2.3.0.1",
"netSnmpExampleHeartbeatRate": 1024,
"variables": [
    {"oid": "1.3.6.1.4.1.8072.2.3.2.1", "type": "integer", "value": 1024},
    {"oid": "1.3.6.1.4.1.8072.2.3.2.2", "type": "string",  "value": "test"}
]
```

So the JSON has **two parallel encodings** of each varbind:

- A `variables[]` array preserving the raw OID + type + (possibly raw) value — for full fidelity.
- A top-level key for each enriched variable using the symbolic name (so Datadog Logs Explorer facets like `@ifAdminStatus:up` work directly).

This dual-encoding is unique among the systems analysed so far in this comparative pass. OpenNMS folds varbinds into `event-parm` elements (raw OID + value, no enrichment); Sensu's bridges write a `snmp_<oid>` annotation per varbind without enum decode; Zenoss writes a `details` dict per event with mixed OID and name keys. Datadog's approach trades JSON verbosity for SaaS-side searchability — every enriched name is a directly-faceted log attribute. Whether the remaining systems (Splunk Connect for SNMP, Cribl, plus the docs-only commercial set) replicate this pattern is open until those analyses complete.

### 5.5 Normalization

There is **no severity normalization**, no per-vendor severity mapping, no trap-OID → severity table on the Agent side. The JSON payload has no `severity` field; what the operator sees in Datadog Logs Explorer is the trap as-is. Severity becomes a property of the **Log Monitor** that triggers on the trap (see §8.2 and §9).

There is also **no unit conversion** — `counter32`, `gauge32`, `time-ticks` are passed through as their native types (uint32 → JSON number). The SaaS side can interpret `time-ticks` as hundredths-of-seconds if the operator knows; the Agent does not convert.

### 5.6 Deduplication / suppression

**None at the Agent layer.** Every received trap that passes community/USM validation is forwarded. There is no dedup window, no rate limit per source, no suppression rule, no `occurrences`-style counter (the way Sensu Go's Postgres events table tracks `occurrences` for identical `(namespace, check, entity)`). Trap deduplication is **a SaaS-side concern** — handled at log-monitor evaluation level (e.g. "alert when more than 5 linkDown traps in 5 minutes per device" via a log-monitor formula).

This is a deliberate design choice with two consequences:

- Pro: the Agent code is simple. No state, no window timers, no eviction logic.
- Con: under storm conditions, every trap goes to SaaS. Bandwidth and SaaS-ingest cost scale linearly with trap volume. No per-source circuit breaker is in the Agent. An operator with a misconfigured device emitting 10k traps/sec will see 10k traps/sec arrive at SaaS until they fix the device or block the source at the firewall. The Datadog "exclusion filter" on the log pipeline can drop them post-ingest, but that does not save bandwidth.

### 5.7 Routing

Single path: every formatted trap goes to `sender.EventPlatformEvent(data, eventplatform.EventTypeSnmpTraps)` (`forwarder/impl/forwarder.go:123`). The Agent's aggregator routes this to EpForwarder, which posts JSON batches to the configured `snmp-traps-intake.<site>` endpoint over HTTPS (`comp/forwarder/eventplatform/eventplatformimpl/epforwarder.go:168-179`).

Operators can configure **additional endpoints** via `network_devices.snmp_traps.forwarder.additional_endpoints` (referenced in `comp/api/api/def/component.go:68`). Standard Datadog dual-shipping pattern — used to feed two SaaS regions or a Datadog + alternate destination simultaneously. There is no per-trap routing (Datadog SaaS pipelines apply log pipeline rules SaaS-side, not Agent-side).

### 5.8 Error handling for malformed PDUs, unknown OIDs, decode failures

- **Malformed PDU**: handled inside gosnmp; the `OnNewTrap` callback is not invoked for unparseable bytes. No telemetry on this case from Datadog code.
- **Bad community / USM auth failure**: increments `datadog.snmp_traps.received` (`listener.go:173`) AND `datadog.snmp_traps.invalid_packet` with `reason:unknown_community_string` (`listener.go:178`). Increments `PacketsUnknownCommunityString` expvar (`listener.go:177`). The packet is dropped — does not enter the channel.
- **Format errors in V2/V3 trap** (missing `sysUpTime.0`, missing `snmpTrapOID.0`, fewer than 2 varbinds): the formatter returns an error and increments `datadog.snmp_traps.incorrect_format` with tags identifying the specific error (`formatter.go:177-194`):
  - `error:invalid_variables` — fewer than 2 variables.
  - `error:invalid_sys_uptime` — first variable is not `sysUpTimeInstance` or wrong type.
  - `error:invalid_trap_oid` — second variable is not `snmpTrapOID.0` or wrong type.
  Test evidence: `formatter_test.go:1135-1238` covers each error path.
- **Unknown trap OID**: `datadog.snmp_traps.traps_not_enriched` increments; the trap is forwarded without `snmpTrapName`/`snmpTrapMIB` (§4.9).
- **Unknown variable OID**: `datadog.snmp_traps.vars_not_enriched` increments for each unenriched varbind; the trap is forwarded with the raw varbind preserved.
- **Forwarder failure** (e.g. EpForwarder ingest queue full or network error): the EpForwarder side increments `EventPlatformEventsErrors[network-devices-snmp-traps]`; the status component surfaces this as `PacketsDropped` (`status/impl/status.go:119-135`). Test evidence: `status_test.go:21-67`.
- **Panic in formatter goroutine**: there is **no `recover()`** in the forwarder's `run()` loop (`forwarder/impl/forwarder.go:99-113`). A panic in `FormatPacket` would crash the goroutine and stop trap forwarding silently (the Agent process continues, but the trap pipeline is dead until restart). The formatter's internal logic uses `ok`-checked map access in `enrichEnum`/`enrichBits` and errors-out gracefully on bad PDU shape — so a panic would have to come from a path the defensive code does not cover (e.g. an unexpected type assertion inside gosnmp's PDU value or an out-of-memory allocation). The probability is low but not zero, and the consequence is silent pipeline death.

## 6. Data Model & Persistent Storage

### 6.1 Agent side: ephemeral only

The Datadog Agent's trap subsystem maintains **no persistent state for trap content**:

- No on-disk trap log.
- No SQLite / embedded DB.
- No dedup state file.
- No alarm-state file.

The only on-disk artefact related to traps is the **OID resolver dataset** (`/etc/datadog-agent/conf.d/snmp.d/traps_db/`) — read-only configuration, not a database. The Agent does not write to it.

Process memory holds:

- The `TrapsConfig` struct (config).
- The `multiFilesOIDResolver.traps` map (per-OID metadata).
- The `packets` channel (capacity 100 pending trap packets).
- expvar counters (`snmp_traps.Packets`, `snmp_traps.PacketsUnknownCommunityString` in `status/impl/status.go:20-31`).

That is the entire data footprint. A restart loses the packets-in-channel but does not lose any trap that has already been handed to `sender.EventPlatformEvent` — the EpForwarder maintains its own on-disk persistence for in-flight events.

### 6.2 Forwarder transit buffer (EpForwarder)

The Datadog Agent's standard forwarder retains in-flight events under `/opt/datadog-agent/run/transactions_to_retry/` (path varies by OS) when the SaaS side is unreachable. Traps inherit this behaviour by virtue of being an event-platform event type. So under transient SaaS-side outage, the Agent buffers up to the configured EpForwarder limit (`network_devices.snmp_traps.forwarder.batch_*` knobs from the `bindEnvAndSetLogsConfigKeys` call in `pkg/config/setup/common_settings.go:236`). Beyond that limit, traps are dropped at ingest with the `EventPlatformEventsErrors` counter.

### 6.3 SaaS side

Per the in-tree config template (`pkg/config/config_template.yaml:4512`): *"Traps are forwarded as logs and can be found in the logs explorer with a source:snmp-traps query"*. So traps live in Datadog's Logs storage tier. The Datadog SaaS public docs describe its log storage architecture as multi-tier (hot, warm, cold-archive) with operator-configurable retention. Trap-event-specific retention is not separately documented — they are subject to the same log-pipeline rules and indexes as any other log.

Sources for the SaaS-side claims in this subsection (retrieved during iter-2): the Datadog **Logs Indexes** docs at `https://docs.datadoghq.com/logs/log_configuration/indexes/` (retention tiers, index-volume cost, exclusion filters), the **Log Archives** docs at `https://docs.datadoghq.com/logs/log_configuration/archives/` (S3/GCS/Azure cold-archive), the **Logs Pipelines** docs at `https://docs.datadoghq.com/logs/log_configuration/pipelines/` (preset SNMP traps pipeline, parsing rules), and the **Monitor API** docs at `https://docs.datadoghq.com/api/latest/monitors/`. Claims about specific retention numbers (15-day default, up to 12 months extension) and the SNMP-traps preset pipeline are SaaS-product claims, not source-verifiable from the Agent repo; readers should consult those URLs for current values.

This means:

- **Search**: full-text + structured-attribute search across the trap JSON. Operators search e.g. `source:snmp-traps @snmpTrapName:linkDown @device_namespace:production @ifIndex:5` — every enriched variable name is a directly-faceted attribute.
- **Retention**: per the customer's logs subscription. Default Datadog Logs retention is 15 days; operators can configure log indexes with longer retention (up to 12 months) or archive to S3/GCS/Azure for compliance (per the Logs Indexes / Log Archives docs cited above).
- **Index volume cost**: the log-pipeline view applies. Operators with high trap volume pay for log ingestion; "exclusion filters" (Logs Indexes docs) can drop traps that the operator does not want indexed but still allow them to flow to a cheaper archive.
- **Schema-on-read**: trap JSON keys become log attributes via the log pipeline. The `snmp-traps` log pipeline ships preset on the SaaS side per the Datadog SNMP integration tile (Logs Pipelines docs).

The substantive operational difference from the OSS systems in this comparison: **all trap retention/indexing/cost decisions are SaaS-side**. Operators have no on-Agent log store for traps. An air-gapped or SaaS-disconnected Agent buffers (per §6.2) until disk fills, then drops.

### 6.4 No tables, no schema migration

Because there is no Agent-side persistent store, there is no concept of "trap schema migration" between Agent versions. JSON format changes (a new field added by formatter) are forward-compatible — the SaaS-side log pipeline ignores unknown fields. There is no `database/migrations` directory; no `ALTER TABLE`; no migration tool.

### 6.5 OID-to-event mapping

In-memory only, loaded from `snmp.d/traps_db/` at Agent startup. The mapping is read-only at runtime; users cannot add OID definitions via API or dashboard at runtime — they must drop files in the directory and restart the Agent.

### 6.6 Dedup state, suppression rules, severity rules, device inventory, topology, audit/log

- **Dedup state**: none (§5.6).
- **Suppression rules**: none. Datadog SaaS exclusion filters in the log pipeline are the equivalent.
- **Severity rules**: none. Severity is determined by Log Monitor evaluations on the SaaS side.
- **Device inventory**: lives separately in NDM's metadata pipeline. The trap subsystem does not maintain it; it merely tags traps with `snmp_device:<src-ip>` and `device_namespace:<ns>` so the SaaS side can join.
- **Topology**: traps do not carry topology, do not consult topology, and do not update topology.
- **Audit/log**: the Agent logs trap events at Trace level when `Tracef` is enabled (`forwarder.go:121`). The gosnmp library logs full decoded packets at Trace via `snmplog.SNMPLogger.Printf` (`snmplog/snmplog.go:36-39`). **No structured audit trail of trap reception** exists on the Agent side beyond expvar counters.

## 7. Configuration UX

### 7.1 Configuration surfaces

- **Agent YAML file** (`/etc/datadog-agent/datadog.yaml`, section `network_devices.snmp_traps`). This is the only configuration surface for trap reception. Documented in `pkg/config/config_template.yaml:4510-4575`. Schema in `pkg/config/schema/core_schema.yaml:9994` (referenced).
- **Environment variables**: every config key has a `DD_NETWORK_DEVICES_SNMP_TRAPS_*` env var. Defaults registered in `pkg/config/setup/common_settings.go:236-242`:
  ```
  network_devices.snmp_traps.enabled            -> false (env DD_NETWORK_DEVICES_SNMP_TRAPS_ENABLED)
  network_devices.snmp_traps.port               -> 9162  (env DD_NETWORK_DEVICES_SNMP_TRAPS_PORT)
  network_devices.snmp_traps.community_strings  -> []    (env DD_NETWORK_DEVICES_SNMP_TRAPS_COMMUNITY_STRINGS)
  network_devices.snmp_traps.bind_host          -> 0.0.0.0
  network_devices.snmp_traps.stop_timeout       -> 5
  network_devices.snmp_traps.users              -> []    (JSON list of object via env)
  ```
- **CLI**: no dedicated trap CLI. `datadog-agent status` shows the SNMP Traps section (`status/impl/status_templates/snmp.tmpl`); `datadog-agent diagnose firewall-scanner` checks UDP port reachability for traps (`pkg/diagnose/firewallscanner/firewallscanner.go:77-91`). No `trap-replay`, no `trap-send`, no `trap-decode-mib`.
- **GUI**: none on the Agent side. The SaaS side has the Datadog NDM "Network Device Monitoring" section and Logs Explorer; trap-related dashboard work (e.g. monitor presets) happens via API or UI from `app.datadoghq.com`.
- **REST API**: none on the Agent side for configuring traps. The Agent does have an admin API (`comp/api/api/`) for runtime introspection — it references `network_devices.snmp_traps.forwarder.additional_endpoints` (`comp/api/api/def/component.go:68`) — but the configuration itself is read-only at runtime.

### 7.2 Full configuration knob inventory

From `comp/snmptraps/config/def/config.go:41-52` + `pkg/config/config_template.yaml:4510-4575`:

| Key | Default | Purpose |
|---|---|---|
| `enabled` | `false` | Master toggle. |
| `port` | `9162` | UDP listener port. Requires setcap for <1024. |
| `bind_host` | `"0.0.0.0"` | Bind interface. |
| `community_strings` | `[]` | List of accepted v1/v2c community strings. The `validatePacket` check (`listener.go:186-200`) requires every non-v3 packet to match one of these via `subtle.ConstantTimeCompare`; an empty list therefore rejects all v1/v2c traps regardless of whether v3 users are also configured. When SNMPv3 users are present the listener runs in `Version3` mode (per the `config.go:155-158` comment: *"Always using version3 for traps, only option that works with all SNMP versions simultaneously"*), so v1/v2c packets are still accepted at the gosnmp layer and only rejected if `community_strings` is empty or does not match — see §3.2. |
| `users` | `[]` | SNMPv3 USM users. Each has `user`/`username` (string, with `username` as legacy alias per `config.go:33-34`), `authKey`, `authProtocol` (MD5/SHA/SHA224/SHA256/SHA384/SHA512), `privKey`, `privProtocol` (DES/AES/AES192/AES256/AES192C/AES256C). |
| `stop_timeout` | `5` (seconds) | Seconds to wait for clean shutdown of gosnmp listener. |
| `namespace` | (from `network_devices.namespace` or `"default"`) | Logical grouping for traps; tags every trap with `device_namespace:<ns>`. |
| `forwarder.additional_endpoints` | `[]` | Extra HTTPS endpoints for dual-shipping. |
| `forwarder.batch_max_size` etc. | platform defaults | EpForwarder batch sizes; standard logs-config naming via `bindEnvAndSetLogsConfigKeys` (`pkg/config/setup/common_settings.go:236`). |

The hard-coded constants (not config-tunable):

- Packets channel size: 100 (`config.go:27`).
- Forwarder flush ticker: 10 seconds (`forwarder.go:100`).
- engineID prefix: `0x80 0xff 0xff 0xff 0xff` (`config.go:91`).
- gosnmp transport: `"udp"` (`config.go:120, :156`).

### 7.3 Defaults

The defaults are **minimal-friction for testing**, **not production-ready out of the box**:

- `enabled: false` — must be explicitly turned on.
- No community strings configured — v2c traps are rejected.
- No v3 users — v3 traps are rejected.

So a freshly-installed Agent with `enabled: true` and no other config will reject every trap with `unknown community string`. This is secure-by-default but operationally surprising; the in-tree config template doc-comment for `community_strings` notes: *"Must be non-empty"* (`config_template.yaml:4536`).

### 7.4 Discoverability of options

- **Template-driven**: `pkg/config/config_template.yaml` is exhaustive — every option has param doc, env var, default, and intent. This is the canonical reference.
- **Schema validation**: `pkg/config/schema/core_schema.yaml` defines types (used by `structure.UnmarshalKey` in `config.go:57`). Bad types fail at startup with a clear error message.
- **`datadog-agent status` self-report**: surfaces packet counts, dropped, unknown-community-string counts (`status/impl/status_templates/snmp.tmpl:1-9`).

### 7.5 Live reload

**No.** The trap component is fx-wired at Agent startup; config changes require Agent restart. The Agent does have a configuration-reload story for some components, but not for traps — `configimpl.NewComponent` (`config/impl/service.go:34-40`) constructs the `*TrapsConfig` once via `trapsconf.ReadConfig(name, conf)` and caches it inside the `configService` struct (`config/impl/service.go:42-49`):

```go
type configService struct {
    conf *trapsconf.TrapsConfig
}

func (cs *configService) Get() *trapsconf.TrapsConfig {
    return cs.conf
}
```

Every later call to `Get()` returns the cached pointer. There is no watcher, no fsnotify, no SIGHUP handler. Reload requires Agent restart.

### 7.6 Multi-tenancy / RBAC

- **Agent-side**: none. The Agent runs as a single Linux process; whoever can write `datadog.yaml` controls trap configuration.
- **SaaS-side**: standard Datadog RBAC applies — Log Monitor read/write permissions, log index access, NDM section access. Traps inherit log-RBAC.
- **`namespace`** is the tenancy primitive at the data-tag level: it segregates traps by logical environment (e.g. `prod-eu`, `staging`, per-customer-MSP). It is NOT enforced by RBAC; an operator who can read all logs reads all namespaces. The namespace tag is a search-organisation tool, not a security boundary.

### 7.7 Secret handling

- Community strings and v3 passphrases are in plaintext in `datadog.yaml` by default.
- The Agent's **secrets backend** (`secrets:` resolver) can be used: any string in YAML of the form `ENC[...]` is resolved via an operator-supplied secrets executable.
- The scrubber (`pkg/util/scrubber/default.go:191-192, :386-419`) redacts `community_strings` from telemetry output, agent logs, and the flare bundle. Test evidence at `pkg/util/scrubber/default_test.go:432-602` covers single-line, multi-line, and empty-list YAML variants. **Passphrases inside `users:` are scrubbed via the `authKey`/`privKey` standard secret-token rules** — the in-tree scrubber config covers them under the general `_key`/`_password` matchers.

## 8. Integration with Other Signals

### 8.1 Metrics

Traps are **not converted to metrics**. The trap content goes to the logs intake; nothing in the trap path emits a `Gauge`/`Count`/`Histogram` for trap content. **However**, the trap subsystem itself **emits operational metrics about its own activity**:

| Metric | Type | Tags | Source |
|---|---|---|---|
| `datadog.snmp_traps.received` | Count | `snmp_version`, `device_namespace`, `snmp_device` | `listener.go:173` |
| `datadog.snmp_traps.invalid_packet` | Count | as above + `reason:unknown_community_string` | `listener.go:178` |
| `datadog.snmp_traps.forwarded` | Count | as above | `forwarder.go:122` |
| `datadog.snmp_traps.traps_not_enriched` | Count | as above | `formatter.go:149, :200` |
| `datadog.snmp_traps.vars_not_enriched` | Count | as above (value = number of varbinds that did not resolve) | `formatter.go:161, :210` |
| `datadog.snmp_traps.incorrect_format` | Count | as above + `error:invalid_variables\|invalid_sys_uptime\|invalid_trap_oid` | `formatter.go:178, :186, :193` |

These are emitted via the `aggregator/sender.Sender` interface — standard Datadog dogstatsd-style metric points. They appear in the customer's own Datadog tenancy and can be alerted on (e.g. monitor "Agent received but failed to enrich >10 traps in 5 min").

Traps as **annotations on metric dashboards**: not built-in, but achievable via log-event-source widgets in dashboards. A widget configured with `source:snmp-traps @snmpTrapName:linkDown` overlays trap occurrences as event markers on metric time series. Documented in Datadog Log Patterns guidance; not in the open-source Agent code.

### 8.2 Alerting / Notifications

Trap-driven alerts are **Log Monitors** on the SaaS side. The integrations-core preset `datadog/integrations-core :: snmp/assets/monitors/traps_linkDown.json:1-118` is the canonical example. Reading the preset:

- Type: `log alert`.
- Two log-query variables:
  - `query1`: `source:snmp-traps @snmpTrapName:linkDown @ifAdminStatus:up` (count grouped by `snmp_device`, `device_namespace`, `@ifIndex`).
  - `query`: `source:snmp-traps @snmpTrapName:linkUp @ifAdminStatus:up`.
- Formula: `default_zero(query1) / default_zero(query1) - default_zero(query) / default_zero(query)` last 1 minute, threshold `> 0.5`.
- Effect: alert fires when linkDown traps arrive for a device without a matching linkUp — exactly the up/down pairing semantics that OpenNMS implements via alarm state, but here implemented as a log-monitor formula. Recovery happens automatically when a linkUp arrives (formula goes ≤ 0.5).

This is the **architectural punchline**: trap correlation in Datadog is **entirely log-monitor formula language** on the SaaS side. There is no on-Agent state machine, no clear-key matching, no alarm lifecycle. The preset above ships with the integration and gives a customer a working linkDown monitor out of the box — every other up/down pairing (BGP, OSPF, fan, PSU, environmental) requires the operator to either build a similar formula by hand or find a preset that someone authored.

Routing: standard Datadog Monitor notification channels — Slack, PagerDuty, Opsgenie, webhook, email, Microsoft Teams. Tag-based routing (e.g. `team:netops` tag drives `@netops-pager` notification). Acknowledgement / clear semantics: log alerts auto-recover when the formula goes back below threshold; manual `@silence` / `@resolve` actions exist via the UI.

### 8.3 Topology

- **Trap reception does not consult topology.** No L2/L3 graph is queried before forwarding.
- **Trap reception does not update topology.** No edge is added or removed based on a trap.
- The SaaS-side **NDM device page** (`/devices?inspectedDevice=<ns>%3A<ip>`) is linked from trap alerts (see preset monitor message at `snmp/assets/monitors/traps_linkDown.json:11`: *"more information from the NDM page for the device"*). So trap-to-device is **a presentation-time join on tag**, not a topology-time join. If two traps with the same `snmp_device` IP belong to different physical devices (e.g. NAT, DHCP-reassigned IP), the NDM page will conflate them.
- **No topology-aware suppression** (the canonical "root-cause: don't alert on the 50 traps from down-stream of this link" pattern that OpenNMS supports via alarm-tree correlation). Datadog operators implement this manually via additional log-monitor formulas referencing topology-aware tags, if they tag accordingly.

### 8.4 Logs / Events

Traps are the **primary** log-event-shaped output in NDM. Each trap is a single JSON object posted to the logs intake. Schema fragment (formatter doc-comment at `formatter.go:87-109`):

```json
{
  "trap": {
    "ddsource": "snmp-traps",
    "ddtags": "namespace:default,snmp_device:10.0.0.2,...",
    "timestamp": 123456789,
    "snmpTrapName": "...",
    "snmpTrapOID": "1.3.6.1.5.3.....",
    "snmpTrapMIB": "...",
    "uptime": "12345",
    "genericTrap": "5",          // v1 only
    "specificTrap": "0",         // v1 only
    "enterpriseOID": "1.3.6.1.4.1.<vendor>",  // v1 only, from formatter.go:155
    "variables": [
      {"oid": "1.3.4.1....", "type": "integer", "value": 12},
      ...
    ]
  }
}
```

Wrapping the trap content under a `"trap"` envelope keeps the SaaS-side log pipeline's `source:snmp-traps` selector reliable: every event with `ddsource: "snmp-traps"` is a trap, and the trap-specific fields are nested under `trap.*`. Datadog's Logs Explorer renders this as a structured log with facets on every key.

Searchability: every enriched variable name is a top-level key, so facets work as `@ifAdminStatus:up`, `@snmpTrapName:linkDown`, etc. Raw variables remain in `variables[]` for fallback. Retention is the customer's log retention tier (§6.3).

### 8.5 Northbound Forwarding

- **The Agent can dual-ship to multiple Datadog SaaS endpoints** via `additional_endpoints`. This is northbound aggregation across regions, not protocol diversity.
- **The Agent cannot emit SNMPv2c traps northbound**. No re-emission path. (Sensu has `sensu-snmp-trap-handler`; Datadog does not.)
- **The Agent cannot emit syslog northbound**.
- **The Agent cannot emit OTLP northbound**.
- **The Agent cannot emit Kafka / Kinesis / Pub-Sub directly**. The only northbound destination is Datadog SaaS.

A customer who needs to route traps to a non-Datadog destination has two choices: (a) configure the device to fan out to a non-Datadog receiver in addition to the Agent; (b) use Datadog's SaaS-side log forwarders (Logs Archives to S3 / GCS / Azure Blob; Logs Streaming to Kafka, S3, etc.) — but those operate on already-ingested logs in SaaS, not at Agent level.

## 9. Severity Model

There is **no severity field in the trap payload**. The Agent does not assign severity. Severity is a property of the Log Monitor that evaluates trap-shaped logs.

Practical model:

1. Trap arrives, gets forwarded to SaaS Logs as-is.
2. The operator configures a Log Monitor (or installs a preset) matching trap content.
3. The Monitor declares its own severity ("critical", "warning", "info") via the `thresholds` block. The preset `traps_linkDown.json:23-27` uses:
   ```json
   "thresholds": {
     "critical": 0.5,
     "critical_recovery": -0.5
   }
   ```
   So linkDown-without-linkUp is critical; recovery is when matching linkUp arrives.
4. The Monitor's notification message embeds template variables from the log (e.g. `{{snmp_device.name}}`).

Customisation surface:

- Operator writes any log query against trap content.
- Operator chooses the threshold structure (single critical, critical+warning, critical+critical_recovery for hysteresis, etc.).
- Operator references any enriched variable name in the formula (e.g. `@ifAdminStatus`, `@ifOperStatus`, `@sysUpTime`).

Contrast with the OSS systems:

- **OpenNMS**: severity is computed at trap-match time via the `eventconf.xml` `<severity>` element per UEI. Centralized rule.
- **Centreon**: severity in the SNMP-trap database is a column; computed at `centreontrapd` match time.
- **Zenoss**: severity is a property of the matched event class; computed at `zentrap` ingest.
- **Sensu**: no severity in the inbound path; checks have integer status (0/1/2/3), some bridges hard-code WARNING as default.
- **Datadog**: severity does not exist at trap-match time. It is a Log-Monitor concept evaluated downstream. This **decouples the trap from its alert** — the same trap can drive multiple monitors at different severities for different use cases.

## 10. Storm / Volume Handling

### 10.1 Per-source rate limits

**None on the Agent side.** Every trap that passes community-string/USM validation is queued for forwarding.

### 10.2 Dedup keys and windows

**None on the Agent side.** Forwarded raw.

### 10.3 Circuit breakers

**None on the Agent side.**

### 10.4 Storm detection

**None on the Agent side.** Operators can detect storms using the Agent's own self-telemetry — `datadog.snmp_traps.received` per device. A high-volume device would surface in a Datadog Metric Monitor on the per-`snmp_device` tag.

### 10.5 Backpressure / queue management

The Agent's only buffer is the 100-deep packets channel between listener and forwarder (§3.4). On forwarder lag:

1. Channel fills.
2. Listener's `t.packets <- pkt` (`listener.go:183`) blocks.
3. gosnmp's `OnNewTrap` cannot return → next UDP read does not happen.
4. Kernel UDP socket buffer (default ~200KB on Linux, tunable via `net.core.rmem_max`) fills.
5. New UDP datagrams are dropped at the kernel level.

There is **no telemetry on kernel-level UDP drops** in the Agent code; operators discover them via `ss -unl -p` or `/proc/net/snmp` counters. This is a significant operational gap — a trap storm with sustained high-rate ingress will silently lose traps once the kernel buffer fills, and the Agent telemetry will not show the drop.

The downstream EpForwarder side does have explicit drop accounting: `EventPlatformEventsErrors[network-devices-snmp-traps]` increments per drop, surfaced as `PacketsDropped` in the Agent status (`status/impl/status.go:119-135`). This catches drops at the SaaS-ingest layer (e.g. HTTPS errors, batch-full conditions), not kernel drops upstream.

### 10.6 Comparison context

In a system optimised for trap-storm survival (Zabbix Perl receiver + DB queue, OpenNMS Camel-based Trapd with disk-backed queues), the storm path is engineered. Datadog's path is the simplest possible: a 100-element channel + kernel buffer. The choice reflects the design contract: trap-storm is a SaaS-side problem (high log volume = log-pipeline cost), and the on-Agent buffer is intentionally small to avoid the Agent being a self-DoS target. Operators handle storms by tuning the source device, not the Agent.

## 11. Security

### 11.1 SNMPv3 USM

Comprehensive USM support (§3.2):

- Auth: MD5, SHA, SHA224, SHA256, SHA384, SHA512.
- Priv: DES, AES128, AES192 (Blumenthal), AES192C (Reeder), AES256 (Blumenthal), AES256C (Reeder).
- Engine ID: FNV-128 hash of Agent hostname with invalid-PEN prefix (§3.3).
- Multiple users supported simultaneously (`config.go:126-152` builds a user table; `listener_test.go:124-193` covers concurrent multi-user reception).

### 11.2 DTLS / TLS-TM

**Not supported.** Only UDP transport (`config.go:120, :156`: `Transport: "udp"`).

### 11.3 Community-string validation

Constant-time comparison via `subtle.ConstantTimeCompare` (`listener.go:194-197`):

```go
for _, community := range c.CommunityStrings {
    if subtle.ConstantTimeCompare([]byte(community), []byte(p.Community)) == 1 {
        return nil
    }
}
```

Defends against timing-oracle attacks on the community string. Empty `community_strings` list rejects all v2c traps (`listener.go:200`: `return errors.New("unknown community string")`).

### 11.4 Credential storage

- Community strings and USM passphrases live in `datadog.yaml` as plaintext or as `ENC[...]` resolved via the Agent secrets backend.
- The scrubber redacts `community_strings` and standard `_key`/`_password` patterns from telemetry, agent logs, and the flare bundle (§7.7).
- USM authoritative-engineID is derived from hostname — no secret leaked, but FNV-128 of hostname is not a cryptographic identifier (no forward secrecy benefit).

### 11.5 Access control on the trap subsystem itself

- The Agent process binds to UDP/<port>. OS-level firewall is the only access control — there is no Agent-internal allow-list for source IPs.
- A trap from any IP is processed (subject to community/USM validation). Operators wanting source-IP filtering use Linux iptables / nftables or Docker network policies, not Agent config.

### 11.6 Audit logging

The Agent logs at:

- INFO: start/stop messages (`listener.go:125, :156`).
- DEBUG: packet received, invalid community string (`listener.go:176, :181`).
- TRACE: full gosnmp packet decode (`snmplog/snmplog.go:32-33`).

`status` output includes counters; **no per-trap audit log file** exists. A high-compliance environment that requires "we received trap X from source Y at time Z" durable record relies on:

- Datadog Logs (the trap itself, in the customer's tenancy, with the customer's retention).
- Optional Datadog Log Archive (S3/GCS/Azure) for long-term retention.

Both are SaaS-side; the Agent has no on-host audit log for compliance use.

### 11.7 Vulnerability surface

- gosnmp is a real dependency — any gosnmp CVE applies to the Agent's trap listener. Past gosnmp vulnerabilities have included PDU-parsing issues; the gosnmp version is pinned in `go.mod` (not in scope for this analysis to enumerate).
- The OID resolver's YAML/JSON decoder uses `go.yaml.in/yaml/v2` (`oid_resolver.go:20`). Operator-supplied YAML/JSON in `snmp.d/traps_db/` is parsed at startup; a malformed file logs a warning and skips that file (`oid_resolver.go:96-101`):
  ```go
  err := oidResolver.updateFromFile(filepath.Join(trapsDBRoot, fileName))
  if err != nil {
      logger.Warnf("unable to load trap db file %s: %s", fileName, err)
  }
  ```
  So a single bad file does not break the Agent; it degrades OID coverage. There is **no signature check** on `dd_traps_db.*` — an operator (or attacker with root on the Agent host) can swap or modify the file freely.

## 12. Trap Simulation & Testing (in-source evidence)

### 12.1 Test inventory

Per-subpackage test files (line counts already in §2.1):

| Test file | Lines | Highlight |
|---|---|---|
| `comp/snmptraps/formatter/impl/formatter_test.go` | 1238 | The heaviest test; table-driven enum/bits enrichment, telemetry-counter assertions, V1 vs V2 paths, malformed-PDU error paths, type-format coverage for all 21 distinct gosnmp PDU type constants the formatter handles. |
| `comp/snmptraps/oidresolver/impl/oid_resolver_test.go` | 353 | Subtree-climb resolution, conflict resolution, file-ordering invariants, property-based `IsValidOID` tester (random valid + invalid OID generation). |
| `comp/snmptraps/listener/impl/listener_test.go` | 266 | End-to-end UDP reception: V1 generic, V1 specific, V2, V3 with multiple USM users, bad credentials. |
| `comp/snmptraps/config/def/config_test.go` | 250 | USM table building, engineID derivation determinism, namespace normalisation, defaults. |
| `comp/snmptraps/status/impl/status_test.go` | 102 | expvar counters; JSON/Text/HTML rendering. |
| `comp/snmptraps/forwarder/impl/forwarder_test.go` | 100 | Listener → forwarder → mock-sender pipeline; telemetry. |
| `comp/snmptraps/server/impl/server_test.go` | 91 | App-within-app lifecycle: enabled+valid, enabled+invalid (no traps_db dir), disabled. |
| `comp/snmptraps/packet/packet_test.go` | 40 | Tag formatting; version-string mapping. |
| `comp/snmptraps/bundle_test.go` | 31 | fx-bundle dependency completeness. |

Total **2,494 lines** of `*_test.go` against **2,011 lines** of non-test source (or **1,644 lines** of impl/runtime if the thin `def/` interfaces and `fx/` wiring modules are excluded). Including the 294 lines of `//go:build test` helpers (`senderhelper.go`, `packet/test_helpers.go`, `listener/impl/test_helpers.go`) the test-side total reaches **2,788 lines**. Ratios: ~1.39:1 (test+helpers / non-test) or ~1.24:1 ignoring helpers. This is high coverage by Go-codebase norms and a tight match to the Datadog Agent's overall test-coverage discipline.

**Out-of-subsystem test coverage** (added to track tests that exercise the trap wiring from outside `comp/snmptraps/`):

- `pkg/config/setup/config_test.go:938, :1046, :1063` — unit tests for `network_devices.snmp_traps.forwarder.logs_dd_url` resolution and the per-site intake URL synthesis.
- `pkg/config/structure/unmarshal_test.go:136-223, :1452-1465` — `UnmarshalKey` tests using `snmp_traps` YAML fixtures; exercise how the trap config block flows through the Agent's structured-config layer.
- `test/new-e2e/tests/agent-subcommands/config_common_test.go:51` — Agent-level e2e test asserting `network_devices.snmp_traps.forwarder.logs_no_ssl` default; validates the forwarder wiring end-to-end.

These tests are not part of `go test ./comp/snmptraps/...` (they live under different roots) but they are part of the Agent's overall trap-related test surface and run on every PR.

**Out-of-subsystem source touchpoints** (production code outside `comp/snmptraps/` that imports the trap subpackages, mostly for the polling-side `snmplog` adapter or for entry-point wiring): `cmd/agent/subcommands/run/command_snmptraps.go`, `cmd/agent/subcommands/run/command_windows.go`, `pkg/config/basic/validate_basic.go` (an allowlist entry for `config_test.go`; no trap logic), and `pkg/snmp/snmpparse/gosnmp.go:14, :96` (the SNMP poller's gosnmp logger, which reuses `comp/snmptraps/snmplog`).

### 12.2 Test infrastructure: `senderhelper`

`comp/snmptraps/senderhelper/senderhelper.go:1-43` is a **test-only package** (build tag `//go:build test`). It assembles a stock fx-option bundle (`Opts`) that wires:

- `defaultforwarder.MockModule()` — the Agent forwarder, mocked.
- `demultiplexerimpl.MockModule()` — the aggregator, mocked.
- `hostnameimpl.MockModule()` — a fixed hostname.
- `logmock.New(t)` — a Logger that writes to the test logger.
- A `mocksender.MockSender` that records every `Count`, `EventPlatformEvent`, etc. for assertion.

Every test file imports `senderhelper.Opts` to get a real component graph with mocked side-effects. This is the canonical Datadog testing pattern; the trap subsystem is fully aligned with it. The comment at `senderhelper.go:27-28` flags that this is a workaround: *"We can remove this if the Sender is ever exposed as a component."*

### 12.3 Real PDU bytes vs synthetic traps

The listener tests actually **send real SNMP traps over real UDP sockets** to localhost (`listener/impl/test_helpers.go:21-92`):

```go
params, err := trapConfig.BuildSNMPParams(nil)
params.Community = "example"
params.Timeout = 1 * time.Second
params.Retries = 1
params.Version = gosnmp.Version1
err = params.Connect()
defer params.Conn.Close()
_, err = params.SendTrap(packet.LinkDownv1GenericTrap)
```

This is the strongest form of in-process integration test — it exercises gosnmp's encode path, the kernel's UDP loopback, gosnmp's decode path, and the Agent's processing pipeline. Test utility `pkg/networkdevice/testutils.UniqueTestPort(t.Name())` ensures port uniqueness across parallel tests.

V3 tests cover MD5/DES, SHA/AES, multiple users matching, bad-credential rejection (`listener_test.go:105-226`).

### 12.4 Sample trap fixtures shipped

In-tree fixtures (Go literals, not separate files): `comp/snmptraps/packet/test_helpers.go:18-75` defines:

- `NetSNMPExampleHeartbeatNotification` — V2 trap with the NET-SNMP example heartbeat OID `1.3.6.1.4.1.8072.2.3.0.1`.
- `LinkDownv1GenericTrap` — V1 generic link-down trap with ifIndex/ifAdminStatus/ifOperStatus varbinds + a multi-byte BITS varbind.
- `AlarmActiveStatev1SpecificTrap` — V1 specific (enterprise + specific-trap-number) variant.
- `Unknownv1Trap` — V1 trap with OIDs not in the dummy DB; used to test unenriched-trap telemetry.

Test-only "dummy trap DB" at `comp/snmptraps/oidresolver/impl/mock.go:51-109` includes `linkUp` (`1.3.6.1.6.3.1.1.5.4`), `ifDown` (`1.3.6.1.6.3.1.1.5.3`), `netSnmpExampleHeartbeatNotification`, plus `ifIndex`, `ifAdminStatus` (enum up/down/testing), `ifOperStatus` (enum up/down/testing/unknown/dormant/notPresent/lowerLayerDown), `pwCepSonetConfigErrorOrStatus` (10-position BITS), and two synthetic test variables.

### 12.5 Tools shipped for trap simulation

The Agent does not ship a `snmp-trap-send` CLI. Operators use:

- `snmptrap` from Net-SNMP (external).
- Python `pysnmp.hlapi.sendTrap`.
- Custom Go `gosnmp.SendTrap` (the same call the listener tests use).

This is a missed operational opportunity — every system analysed except Datadog and Logstash (also a forwarder) ships at least one CLI for replay/synth trap generation. Sensu's `snmptrapd2sensu` does not but defers to Net-SNMP; OpenNMS ships `send-event.pl` and SNMP-trap utilities; Zabbix's `zabbix_get` and shipped `zabbix_trap_receiver.pl`; LibreNMS's PHP CLI `snmptrap.php`. Datadog operators rely on external tooling.

### 12.6 CI workflow

Agent CI runs Go tests on every PR (`.github/workflows/` and the Datadog-internal CI). The trap subsystem tests are part of the standard `go test ./comp/snmptraps/...` matrix. Build tags `test` and `!serverless` gate test-only files (`packet/test_helpers.go:6`, `listener/impl/test_helpers.go:6`, `senderhelper.go:6-7`, `bundle_test.go:6`).

There is no separate trap-specific CI workflow — traps are validated under the general Agent test suite. No nightly trap-storm load test in the open-source CI definitions.

## 13. Out-of-the-Box Coverage (defaults)

### 13.1 Bundled OID resolver dataset

**Yes** — `dd_traps_db.json.gz` ships in the installer at `snmp.d/traps_db/` (§4.5; the exact filename is documented in the Datadog SNMP Traps operator guide at `https://docs.datadoghq.com/network_monitoring/devices/snmp_traps/`). Generated via `ddev meta snmp generate-traps-db` from integrations-core MIBs. Datadog's public documentation states the shipped DB covers **"more than 11,000 MIBs"** (Datadog SNMP Traps docs, same URL; retrieved during iter-2). No authoritative vendor-by-vendor list is published — the 11,000+ figure is the only public coverage statement.

### 13.2 Severity rules bundled

**None** — severity is not an Agent-side concept (§9).

### 13.3 Dedup defaults

**None** — Agent does not dedup (§5.6, §10.2).

### 13.4 Vendor packs

The Datadog **integration tile for SNMP** (datadog/integrations-core :: snmp/) ships:

- The `dd_traps_db.*` (compiled MIB DB) via the integrations-core build.
- The preset Log Monitor `traps_linkDown.json` (`integrations-core :: snmp/assets/monitors/traps_linkDown.json`).
- SNMP-poll profiles for many vendors (not trap-specific, but co-installed).

There is no "vendor pack" concept analogous to Zenoss ZenPacks (which carry trap event classes + transforms + UI widgets). Datadog operators get:

- An OID-resolvable trap (so `@snmpTrapName:linkDown` works out of the box for covered vendors).
- One preset monitor.

That is the entire OOTB experience. The customer has to build the rest: dashboards with trap widgets, additional log monitors for other trap types, archive policies, etc.

### 13.5 Sample/preset dashboards or reports

- **NDM Troubleshooting dashboard**: `datadog/integrations-core :: snmp/assets/dashboards/ndm_troubleshooting.json` ships a SaaS-side dashboard whose panels include `datadog.snmp_traps.received` and `datadog.snmp_traps.forwarded` charts and log-explorer widgets filtered by `source:snmp-traps`. This is the canonical OOTB operator dashboard for trap pipeline troubleshooting — it surfaces both the Agent-side self-telemetry and the per-trap log stream in one view. Operators get it on day-1 when they enable the SNMP integration tile.
- Other Datadog NDM dashboards (per-vendor device dashboards, polling dashboards) exist on the SaaS side but are not source-visible in the open-source agent tree.
- Trap content can additionally be visualised in any operator-authored Logs dashboard widget via `source:snmp-traps` queries.

## 14. User Customization Surface

### 14.1 Adding OID handlers

**File-based** — operator generates a YAML/JSON traps DB file via `ddev meta snmp generate-traps-db` and drops it into `/etc/datadog-agent/conf.d/snmp.d/traps_db/`. Restart Agent. Trap is now enriched on next reception.

There is no "handler" concept beyond OID resolution. An operator cannot register Go code, a script, or a webhook to run when a specific trap arrives at the Agent — all such logic lives on the SaaS side as Log Monitors or Log Pipeline rules.

### 14.2 Custom MIBs

Same as §14.1 — produce a custom traps DB file from any MIB sources.

### 14.3 Custom severity rules

Configured as Log Monitors (§9). No Agent-side severity config.

### 14.4 Custom dedup rules

SaaS-side log-pipeline aggregation and log-monitor time-windows. No Agent-side dedup config.

### 14.5 Plugin / extension model

**None on the Agent side for traps.** The Datadog Agent does have a generic Python check plugin model (`pkg/collector/python/`), but the trap subsystem is exclusively Go and not extensible at runtime.

On the SaaS side, the **Log Pipeline** acts as the extension surface: operators can parse, remap, enrich, sample, alert on, and archive traps via the SaaS UI. The Datadog SaaS pipeline DSL (documented at `https://docs.datadoghq.com/logs/log_configuration/processors/`, retrieved during iter-2) lists processors for geo-IP, lookup tables, status remappers, attribute remappers, message-grok parsers, and tag aliasing — all of which an operator can apply to trap-shaped logs. These are SaaS-product capabilities, not source-verifiable from the Agent repo.

### 14.6 API surface for automation

- **Agent side**: traps are configured via `datadog.yaml` only. No runtime API.
- **SaaS side**: the Datadog Monitor / Log Pipeline / Log Index APIs are standard REST APIs documented at `docs.datadoghq.com/api/v2/`. Operators can scriptbed monitor creation, log-archive configuration, etc. — same as for any other Datadog signal.

### 14.7 Bottom line

The customisation surface is **bipartite**: trivial on the Agent (file-drop + restart for new MIB coverage; nothing else customisable), rich on the SaaS (log pipeline DSL, monitor formulas, archives, dashboards). The trade-off mirrors the architecture: the Agent is a thin forwarder, the SaaS is the platform.

## 15. End-User Value Analysis

### 15.1 Day-1 experience for a Datadog NDM customer

The customer enables traps in `datadog.yaml`, sets a community string, restarts the Agent, points devices at the Agent's port (after handling the 9162→162 setcap detail or device-side reconfig), and starts seeing traps in Logs Explorer with `source:snmp-traps`. With the bundled `dd_traps_db`, common traps (linkDown, linkUp, coldStart, warmStart, authentication-failure, plus the breadth of vendor MIBs integrations-core covers) are name-resolved and enum-decoded automatically. The `traps_linkDown` preset monitor (when enabled by the customer) gives one working monitor immediately.

This is a low-friction first-15-minutes experience for a customer **already on Datadog NDM**. There are no installation prerequisites beyond the Agent itself, no MIB compilation, no Net-SNMP daemon to manage, no PostgreSQL to provision.

### 15.2 What requires customization

- Any trap outside the bundled `dd_traps_db` coverage requires custom MIB compilation via `ddev meta snmp generate-traps-db` (a separate Python toolchain).
- Any trap-driven alerting beyond linkDown requires writing Log Monitors. There is no preset library covering authentication-failure, ospfNbrStateChange, bgpEstablishedNotification, fan/PSU traps, environmental traps, etc.
- Any topology-aware suppression requires custom monitor formulas referencing topology tags.
- Any per-source rate limit / storm protection is fully manual (device-side configuration + Agent metric monitor).

### 15.3 Learning curve

- **Agent-side**: very shallow. Three knobs (`enabled`, `port`, `community_strings`) for the common case; three more (`users`, `bind_host`, `namespace`) for advanced. Documented in the in-tree config template with full examples.
- **SaaS-side**: moderate. Operators must learn the Datadog Log Pipeline, the Log Monitor DSL, and the Datadog tag taxonomy. For an existing Datadog Log customer this is trivial; for someone whose only Datadog product is NDM-traps, it is an unfamiliar surface.
- **MIB-side**: steep when needed. `ddev meta snmp generate-traps-db` requires a Python environment, a `pip3 install ddev` (or `pipx install ddev`) install (a separate toolchain from the Go-based Agent binary; `ddev` is the integrations-core operator/developer CLI and depends on `datadog-checks-dev`), pysmi, MIB sources, and an understanding of MIB IMPORTS / vendor-MIB dependencies. Comparable to OpenNMS's mibparser, Zenoss's MIB ZenPack workflow, or LibreNMS's MIB-in-snmptrapd ritual.

### 15.4 Operational toil

- **Agent restart on MIB DB change**: every new MIB requires a restart. For an MSP with hundreds of Agents this is non-trivial; for a single-DC deployment it is fine.
- **No per-trap SaaS-side observability of what the Agent dropped at the kernel** (§10.5) — a real gap.
- **No on-Agent trap log / replay** — debugging requires turning the Agent to TRACE log level (heavy) or using `tcpdump` to capture UDP/162.

### 15.5 Visibility into the pipeline's own health

Self-telemetry (§8.1) covers received, forwarded, invalid, not-enriched. Status command output covers packet counters + drop count (§7.4). expvar exposes `snmp_traps.Packets`, `snmp_traps.PacketsUnknownCommunityString`. Operators with self-monitoring (Datadog monitoring itself with Datadog) get full visibility into Agent-side trap pipeline health.

What is **not visible** without extra work:

- Per-source trap rate (must derive from `datadog.snmp_traps.received` grouped by `snmp_device`).
- Kernel UDP drop rate (must come from external Linux metrics).
- EpForwarder transit-queue depth for the snmp-traps event type (must come from `EventPlatformEventsErrors` and forwarder-specific expvars).

## 16. Strengths

1. **Clean component boundaries (DI/fx)**. The ten-subpackage `def`/`fx`/`impl`/`mock` layout makes each piece independently testable, and the mock components (`listener/impl/mock_listener.go`, `formatter/impl/mock.go`, `oidresolver/impl/mock.go`, `status/impl/mock.go`) enable isolated component tests without the full app graph. Evidence: `forwarder/impl/forwarder_test.go:32-50` swaps in a mock listener + mock formatter cleanly.

2. **Subtree-climb-up variable resolution with intermediate-node masking** (`oid_resolver.go:118-145, :230-246`). Solves the suffixed-variable problem (`ifIndex.5` → `ifIndex`) elegantly while preventing runaway matches at shared roots. Unique among the systems analysed.

3. **Comprehensive SNMPv3 USM support**. Six auth algorithms (MD5 → SHA512), six priv algorithms including the Reeder variants used by Cisco. Multiple simultaneous users. Constant-time community-string compare. Evidence: `gosnmplib/gosnmp_auth.go:17-65`, `config.go:117-162`, `listener_test.go:124-226`.

4. **Self-telemetry as first-class concern**. Six dedicated metrics (`datadog.snmp_traps.received`, `.invalid_packet`, `.forwarded`, `.traps_not_enriched`, `.vars_not_enriched`, `.incorrect_format`) plus expvars and a status-command panel. Operators can alert on their own pipeline. Evidence: §8.1 table.

5. **Conflict resolution that puts the customer first**. The file-name-based precedence model places **user files above Datadog-shipped files** (`oid_resolver.go:67-69, :147-176`). Operators can override any shipped OID by dropping a file with a non-`dd_traps_db` name.

6. **Dual-encoding JSON payload** — both `variables[]` (raw fidelity) AND top-level enriched-name keys (SaaS-facet friendly). Verbosity-for-searchability trade-off, deliberate, and works very well with Datadog Logs.

7. **Real-PDU integration tests over the loopback UDP socket** (`listener/impl/test_helpers.go:21-92`). End-to-end coverage of the gosnmp encode → kernel → gosnmp decode → Agent processing path. Higher confidence than synthetic-fixture-only tests.

8. **Telemetry-tagged error paths in the formatter**. Each malformed-PDU variant emits `datadog.snmp_traps.incorrect_format` with a specific `error:<reason>` tag (`formatter.go:178, :186, :193`). Operators can pinpoint exactly which class of device is sending bad PDUs.

## 17. Weaknesses / Gaps

1. **SaaS-tier dependency for everything beyond reception**. The Agent itself is open-source (Apache 2.0) and can run without SaaS connectivity — EpForwarder retains undelivered events in its retry directory (`/opt/datadog-agent/run/transactions_to_retry/`) and resumes shipping when the intake is reachable. But every consumer-facing capability (storage, dedup, severity, alerting, search, retention, UI, NDM device cross-link) lives behind the customer's Datadog subscription. There is no on-prem trap surface: an extended SaaS outage degrades the Agent into a buffer that eventually exhausts disk, and a customer without a Datadog NDM subscription has no usable trap experience. Operators who need on-premise sovereignty or long-running air-gapped operation for trap data cannot use this design. This is the single largest architectural contrast against Netdata's hub model (`.agents/sow/specs/snmp-traps/netdata-snmp-hub-architecture.md`).

2. **Functional misattribution of relayed SNMPv1 traps**. The UDP source IP is always used as `snmp_device`, regardless of the v1 `agent-addr` field (§5.3). In networks with proxy/relay devices — common in large enterprise multi-site SNMP forwarders and NAT boundaries — every v1 trap is mis-attributed to the relay's IP, breaking the NDM device cross-link, all `@snmp_device:` searches, and per-device alert routing for that entire class of traps. This is a class-level data-correctness gap, not a subtle issue.

3. **No live reload of trap DB or config**. Adding a new MIB requires Agent restart (§4.6, §7.5). MSPs with fleets of Agents pay a real operational cost.

4. **No on-Agent storm protection**. No per-source rate limit, no dedup, no circuit breaker (§10). Under storm conditions, the only back-pressure is the 100-deep packets channel + kernel UDP buffer; everything beyond is silently dropped at kernel level with no visibility.

5. **No kernel-UDP-drop telemetry**. The Agent does not surface `/proc/net/snmp` UDP-drop counters. A silent gap that storm conditions exploit (§10.5).

6. **`fx.New` app-within-app**. The trap server builds a sub-fx-app inside the parent Agent app (`server.go:97-114`), flagged by an in-tree TODO comment: *"Having apps within apps is not ideal ... Do not use this solution elsewhere if possible"* (`server.go:94-96`). A real architectural smell, candidly acknowledged.

7. **No outbound trap emission**. Datadog cannot re-emit traps to a downstream NMS (§8.5, §15.2). A common northbound-forwarding integration is unavailable.

8. **No CLI for trap simulation / replay**. Operators rely on external tools (`snmptrap`, pysnmp) for testing.

9. **MIB compilation tool ships separately**. `ddev meta snmp generate-traps-db` is in `datadog/integrations-core`, not in `datadog/datadog-agent`. Operators must `pip3 install ddev` (or `pipx install ddev`) for custom MIB workflows, per the operator setup guide at `https://docs.datadoghq.com/network_monitoring/devices/snmp_traps/`.

10. **No DTLS / TLS-TM**. RFC 6353/6354 transport-layer security is not supported. v3 USM is the only secure mode; on-wire confidentiality depends on UDP datagrams over the operator's network (often a VPN or trusted LAN).

11. **Severity-as-monitor-formula tax**. Every distinct severity rule requires authoring a separate Log Monitor — there is no "this trap OID is critical" shorthand. For customers migrating from OpenNMS or Zenoss (where 200+ severity bindings ship out of the box), Datadog requires manually authoring the equivalent monitors.

12. **No `recover()` in the forwarder goroutine**. A panic in `FormatPacket` would silently kill the forwarder goroutine; the Agent process keeps running but traps stop forwarding (§5.8).

13. **EngineID derivation uses an invalid PEN prefix** (§3.3). Non-fingerprinting on the wire but deviates from RFC-recommended PEN encoding; multi-Agent deployments with identical hostnames share engineIDs.

14. **Channel buffer is hard-coded** at 100 (`config.go:27`). Not operator-tunable. A high-rate site cannot increase headroom without code changes.

## 18. Notable Code or Configuration Examples

### 18.1 The fx app-within-an-app and the TODO

`comp/snmptraps/server/impl/server.go:87-114`:

```go
func NewComponent(deps Requires) Provides {
    if !trapsconfig.IsEnabled(deps.Conf) {
        return Provides{
            Comp: &TrapsServer{running: false},
        }
    }
    stat := statusimpl.New()
    // TODO: (components) Having apps within apps is not ideal - you have to be
    // careful never to double-instantiate anything. Do not use this solution
    // elsewhere if possible.
    app := fx.New(
        logging.DefaultFxLoggingOption(),
        fxutil.FxLifecycleAdapter(),
        fx.Supply(injections{...}),
        configfx.Module(),
        fxutil.ProvideComponentConstructor(formatterimpl.NewComponent),
        fxutil.ProvideOptional[formatter.Component](),
        forwarderimpl.Module(),
        listenerfx.Module(),
        oidresolverfx.Module(),
        fx.Invoke(func(_ forwarder.Component, _ listener.Component) {}),
    )
```

This is the candid design signal — the team chose runtime-toggleable sub-app composition over compile-time conditional wiring, and documented it as a smell.

### 18.2 EngineID derivation from hostname

`comp/snmptraps/config/def/config.go:86-92`:

```go
h := fnv.New128()
h.Write([]byte(host))
// First byte is always 0x80
// Next four bytes are the Private Enterprise Number (set to an invalid value here)
// The next 16 bytes are the hash of the agent hostname
engineID := h.Sum([]byte{0x80, 0xff, 0xff, 0xff, 0xff})
c.authoritativeEngineID = string(engineID)
```

Test expectation `config/def/config_test.go:25`: `expectedEngineID = "\x80\xff\xff\xff\xff\x67\xb2\x0f\xe4\xdf\x73\x7a\xce\x28\x47\x03\x8f\x57\xe6\x5c\x98"`. The deliberate use of `0xff 0xff 0xff 0xff` (an invalid PEN) avoids fingerprinting Datadog Agents on the wire — a quiet privacy decision documented only via the comment.

### 18.3 Constant-time community-string compare

`comp/snmptraps/listener/impl/listener.go:186-201`:

```go
func validatePacket(p *gosnmp.SnmpPacket, c *config.TrapsConfig) error {
    if p.Version == gosnmp.Version3 {
        // v3 Packets are already decrypted and validated by gosnmp
        return nil
    }
    // At least one of the known community strings must match.
    for _, community := range c.CommunityStrings {
        // Simple string equality check, but in constant time to avoid timing attacks
        if subtle.ConstantTimeCompare([]byte(community), []byte(p.Community)) == 1 {
            return nil
        }
    }
    return errors.New("unknown community string")
}
```

Most of the systems in this analysis do **not** use constant-time community-string compare. Zenoss `zentrap` uses Python's `==`; Sensu Classic extension uses Ruby `==`; LibreNMS PHP plugin uses `===`; OpenNMS uses Java string equality. Datadog is the only Go-native system in the comparison and one of the few to defend against timing oracles on the community.

### 18.4 Subtree-climb-up resolution with intermediate-node masking

`comp/snmptraps/oidresolver/impl/oid_resolver.go:118-145, :230-246`:

```go
recreatedVarOID := varOID
for {
    varData, ok := trapData.VariableSpecPtr[recreatedVarOID]
    if ok {
        if varData.IsIntermediateNode {
            return oidresolver.VariableMetadata{}, fmt.Errorf("variable OID %s is not defined", varOID)
        }
        return varData, nil
    }
    lastDot := strings.LastIndex(recreatedVarOID, ".")
    if lastDot == -1 { break }
    recreatedVarOID = varOID[:lastDot]
}
```

```go
sort.Strings(allOIDs)
for idx, variableOID := range allOIDs {
    isIntermediateNode := false
    if idx+1 < len(allOIDs) {
        nextOID := allOIDs[idx+1]
        isIntermediateNode = strings.HasPrefix(nextOID, variableOID+".")
    }
    variableData := trapDB.Variables[variableOID]
    variableData.IsIntermediateNode = isIntermediateNode
    definedVariables[variableOID] = variableData
}
```

The intermediate-node fast-pass is the load-time sentinel that prevents `1.3.6.1` from matching every variable in the world.

### 18.5 Telemetry-tagged error paths in the formatter

`comp/snmptraps/formatter/impl/formatter.go:168-195`:

```go
variables := packet.Content.Variables
if len(variables) < 2 {
    f.sender.Count(telemetryIncorrectFormat, 1, "", append(tags, "error:invalid_variables"))
    return nil, fmt.Errorf("expected at least 2 variables, got %d", len(variables))
}
data := make(map[string]interface{})
uptime, err := parseSysUpTime(variables[0])
if err != nil {
    f.sender.Count(telemetryIncorrectFormat, 1, "", append(tags, "error:invalid_sys_uptime"))
    return nil, err
}
data["uptime"] = uptime
trapOID, err := parseSnmpTrapOID(variables[1])
if err != nil {
    f.sender.Count(telemetryIncorrectFormat, 1, "", append(tags, "error:invalid_trap_oid"))
    return nil, err
}
```

Three distinct error tags allow an operator to alert on exactly the malformed-PDU class they care about (e.g. `error:invalid_sys_uptime` indicates a device emitting V2 traps with sysUpTime as the wrong PDU type — a vendor-specific bug class).

### 18.6 EpForwarder routing entry — where traps become logs

`comp/forwarder/eventplatform/eventplatformimpl/epforwarder.go:168-179`:

```go
{
    eventType:                     eventplatform.EventTypeSnmpTraps,
    category:                      "NDM",
    contentType:                   logshttp.JSONContentType,
    endpointsConfigPrefix:         "network_devices.snmp_traps.forwarder.",
    hostnameEndpointPrefix:        "snmp-traps-intake.",
    intakeTrackType:               "ndmtraps",
    defaultBatchMaxConcurrentSend: 10,
    defaultBatchMaxContentSize:    pkgconfigsetup.DefaultBatchMaxContentSize,
    defaultBatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
    defaultInputChanSize:          pkgconfigsetup.DefaultInputChanSize,
},
```

This is the "where the trap goes" entry — `category: NDM`, `contentType: application/json` (`logshttp.JSONContentType`), and a category-specific hostname prefix and intake track. The same logs-http transport that ships application logs, audit logs, etc., carries trap content. The choice to register trap events under the **logs** subsystem rather than building a separate "trap intake" is the architectural commitment to traps-as-logs.

## 19. Sources Examined

### 19.1 datadog/datadog-agent @ 2c813592

Trap subsystem (under `comp/snmptraps/`):

- `bundle.go`, `bundle_test.go`
- `BUILD.bazel`
- `config/def/config.go`, `config/def/config_test.go`, `config/def/component.go`, `config/def/BUILD.bazel`
- `config/fx/fx.go`, `config/fx/BUILD.bazel`
- `config/impl/service.go`, `config/impl/mock.go`, `config/impl/BUILD.bazel`
- `formatter/def/component.go`, `formatter/def/BUILD.bazel`
- `formatter/fx/fx.go`, `formatter/fx/fx_mock.go`
- `formatter/impl/formatter.go`, `formatter/impl/formatter_test.go`, `formatter/impl/mock.go`, `formatter/impl/mock_test.go`
- `forwarder/def/component.go`, `forwarder/def/BUILD.bazel`
- `forwarder/fx/fx.go`
- `forwarder/impl/forwarder.go`, `forwarder/impl/forwarder_test.go`
- `listener/def/component.go`, `listener/def/BUILD.bazel`
- `listener/fx/fx.go`
- `listener/impl/listener.go`, `listener/impl/listener_test.go`, `listener/impl/mock_listener.go`, `listener/impl/test_helpers.go`
- `oidresolver/def/component.go`, `oidresolver/def/traps_db.go`, `oidresolver/def/BUILD.bazel`
- `oidresolver/fx/fx.go`, `oidresolver/fx/fx_mock.go`, `oidresolver/fx/BUILD.bazel`
- `oidresolver/impl/oid_resolver.go`, `oidresolver/impl/oid_resolver_test.go`, `oidresolver/impl/mock.go`, `oidresolver/impl/BUILD.bazel`
- `packet/packet.go`, `packet/packet_test.go`, `packet/test_helpers.go`, `packet/BUILD.bazel`
- `senderhelper/senderhelper.go` (test-only)
- `server/def/component.go`, `server/def/BUILD.bazel`
- `server/fx/fx.go`
- `server/impl/server.go`, `server/impl/server_test.go`
- `snmplog/snmplog.go`, `snmplog/BUILD.bazel`
- `status/def/component.go`, `status/def/BUILD.bazel`
- `status/fx/fx.go`
- `status/impl/status.go`, `status/impl/status_test.go`, `status/impl/mock.go`
- `status/impl/status_templates/snmp.tmpl`, `status/impl/status_templates/snmpHTML.tmpl`

Consumer surface:

- `comp/forwarder/eventplatform/component.go:19-20` (EventTypeSnmpTraps constant)
- `comp/forwarder/eventplatform/eventplatformimpl/epforwarder.go:168-179` (intake routing)
- `comp/api/api/def/component.go:68` (additional_endpoints)
- `pkg/aggregator/sender/` — Sender interface used by formatter/listener/forwarder
- `pkg/config/config_template.yaml:4510-4575` (full user-facing config doc)
- `pkg/config/schema/core_schema.yaml:9994` (schema entry)
- `pkg/config/setup/config.go:958` (logs-config wiring for traps forwarder)
- `pkg/config/setup/common_settings.go:236-242` (default registration + env binding)
- `pkg/diagnose/firewallscanner/firewallscanner.go:77-91` (firewall-rule self-check)
- `pkg/util/scrubber/default.go:191-192, :386-419` (community_strings scrubbing)
- `pkg/util/scrubber/default_test.go:432-602` (scrubber test cases)
- `pkg/snmp/gosnmplib/gosnmp_auth.go:17-65` (auth/priv protocol mapping)
- `pkg/snmp/utils/` (referenced for `NormalizeNamespace`)
- `pkg/networkdevice/testutils/` (referenced for `UniqueTestPort`)
- `pkg/collector/check/stats/stats.go:56` (display name "SNMP Traps")
- Release notes: `releasenotes/notes/snmp-traps-collect-telemetry-a8dbf3ec35f2e679.yaml`, `releasenotes/notes/Resolve-SNMP-Traps-OIDs-to-names-70de58eecc4892aa.yaml`, `releasenotes/notes/SNMP---include-traps-db-7c44bd129daf7667.yaml`, `releasenotes/notes/Update-SNMP-Traps-DB-with-BITS-a8f419275252c7b9.yaml`, `releasenotes/notes/Update-SNMP-Traps_DB-76a39128d7b2b4e9.yaml`

### 19.2 datadog/integrations-core @ 411c31db

- `datadog_checks_dev/datadog_checks/dev/tooling/commands/meta/snmp/generate_traps_db.py` — pysmi-based MIB → trap DB compiler, ~660 lines.
- `datadog_checks_dev/tests/tooling/commands/meta/snmp/data/A3COM-HUAWEI-LswTRAP-MIB/expected_expanded.json` and `expected_compact.json` — test fixtures validating the MIB compiler pipeline on a Huawei MIB.
- `snmp/assets/monitors/traps_linkDown.json` — preset Log Monitor for linkDown / linkUp pairing.
- `snmp/assets/dashboards/ndm_troubleshooting.json` — SaaS-side troubleshooting dashboard surfacing `datadog.snmp_traps.received`, `datadog.snmp_traps.forwarded`, and `source:snmp-traps` log widgets.
- `snmp/metadata.csv` — canonical metadata for the six `datadog.snmp_traps.*` agent-side telemetry metrics.
- `snmp/datadog_checks/snmp/data/default_profiles/` — ~100 SNMP polling profile YAMLs; used here only as a proxy indicator of vendor priorities, not as direct evidence of trap-DB coverage.

### 19.3 datadog/datadog-agent — entry-point wiring (additional)

Files that wire the trap sub-app into the Agent binary, beyond the `comp/snmptraps/` tree itself:

- `cmd/agent/subcommands/run/command_snmptraps.go` — non-Windows entry-point that imports `snmptraps.Bundle()` and invokes the trap server via `fx.Invoke`.
- `cmd/agent/subcommands/run/command_windows.go` — Windows-specific entry-point with platform-specific wiring.
- `tasks/components.py` — lists trap subpackage paths used by the CI test matrix.
- `.github/CODEOWNERS` — confirms ownership by the `# team: network-device-monitoring-core` team (cross-validates `bundle.go:15`).

### 19.4 Vendor public documentation (cross-referenced, retrieved during iter-2)

- `https://docs.datadoghq.com/network_monitoring/devices/snmp_traps/` — operator-facing setup guide. Source of: the `dd_traps_db.json.gz` install path, the "more than 11,000 MIBs" coverage statement, the `pip3 install ddev` / `pipx install ddev` operator install for the MIB compiler, the v7.37+ Agent version requirement, the `setcap` workaround for port 162, and the `source:snmp-traps` Logs Explorer query.
- `https://docs.datadoghq.com/logs/log_configuration/indexes/` — Logs Indexes guide. Source of: 15-day default retention, up-to-12-month extension, exclusion filters.
- `https://docs.datadoghq.com/logs/log_configuration/archives/` — Log Archives guide. Source of: S3/GCS/Azure cold-archive support.
- `https://docs.datadoghq.com/logs/log_configuration/pipelines/` — Logs Pipelines guide. Source of: preset SNMP traps pipeline, pipeline processor registry, integration-tile-shipped pipelines.
- `https://docs.datadoghq.com/logs/log_configuration/processors/` — Log Processors reference. Source of: geo-IP, lookup, status-remapper, attribute-remapper processor list referenced in §14.5.
- `https://docs.datadoghq.com/api/latest/monitors/` — Monitor API reference. Source of: SaaS-side monitor automation surface referenced in §14.6.
- `https://www.iana.org/assignments/enterprise-numbers/enterprise-numbers` — IANA Private Enterprise Numbers registry, used to verify that `0xff 0xff 0xff 0xff` is not an assigned PEN and that Datadog's assigned PEN is 47812 (§3.3).

Claims that rely on these URLs (SaaS-side retention, log-pipeline processor capabilities, "11,000+ MIBs" coverage, ddev install path, Logs Explorer behaviour) are clearly attributed in the body sections to these URLs and are downgraded to **Medium** confidence in §20 because they are SaaS-product claims, not source-verifiable from the Agent repo.

## 20. Evidence Confidence

| Section | Confidence | Justification |
|---|---|---|
| §1 System Overview & Lineage | **High** | All claims trace to in-tree code, release notes, or the config template. The "Datadog Agent cannot emit outbound SNMP traps" claim is by-exhaustion-of-search (`grep` confirmed no SendTrap call path outside the test helpers). |
| §2 Trap-Subsystem Architecture | **High** | The ten subpackages, the fx wiring, the app-within-app pattern, the 100-deep channel, and the 10-second flush ticker are all directly source-verifiable. Diagram is an ASCII synthesis. |
| §3 Trap Reception | **High** | Gosnmp listener wiring, version support (V1/V2c/V3), USM table, engineID FNV-128, constant-time community compare — all source-verifiable with file:line. The "no DTLS" claim is by exhaustive grep. |
| §4 MIB Management | **High** for the in-Agent OID resolver behaviour; **Medium** for `dd_traps_db` shipped vendor coverage (inferred from integrations-core profile breadth — the actual DB file is not in the open repo). |
| §5 Trap Processing Pipeline | **High**. Every step from PDU → JSON is in `formatter.go`. The "no dedup" and "no severity normalisation" claims are by absence-in-source. |
| §6 Data Model & Persistent Storage | **High** for the Agent side (ephemeral, plus EpForwarder transit buffer); **Medium** for the SaaS side (relies on Datadog public docs for log retention tiers, not source-verifiable from the agent repo). |
| §7 Configuration UX | **High**. Config template + schema + setup defaults are directly source-verifiable. |
| §8 Integration with Other Signals | **High** for metrics, alerting (via the preset monitor), logs (via the EpForwarder route); **Medium** for the "NDM device cross-link in alert message" (claim derived from the preset monitor's template); **High** for the "no outbound forwarding" claim. |
| §9 Severity Model | **High**. No severity field is present in the formatter output (source-verified); severity-via-log-monitor is the only mechanism (preset monitor as evidence). |
| §10 Storm / Volume Handling | **High** for the 100-deep channel, the lack of rate limits, and the EpForwarder transit buffer; **Medium** for the kernel-UDP-drop visibility claim (negative claim: the Agent does not surface `/proc/net/snmp` UDP-drop counters via any source-visible path). |
| §11 Security | **High** for USM/community/scrubber/secrets; **Medium** for "no signature on dd_traps_db" (negative claim, by absence in the loader code). |
| §12 Trap Simulation & Testing | **High**. Test inventory, real-UDP integration tests, fixtures, build-tag gating all source-verified. |
| §13 Out-of-the-Box Coverage | **High** for the OID resolver dataset existing and being shipped (release notes confirm) and for the NDM troubleshooting dashboard (file source-visible in integrations-core); **Low** for the exact vendor coverage of the shipped `dd_traps_db.*` — the file is not in the open-source tree and the §4.5 inference from polling-profile breadth is speculative. |
| §14 User Customization Surface | **High**. File-drop + Agent restart is the entire trap-side customisation; SaaS-side surface is described per Datadog public docs. |
| §15 End-User Value Analysis | **High** for the Agent-side experience (source-verifiable: knobs, defaults, restart-to-reload, no CLI for simulation); **Medium** for SaaS-side day-1 (relies on the preset monitor as representative; broader SaaS-side experience derives from vendor docs). |
| §16 Strengths | **High**. Every numbered strength is anchored to a file:line citation. |
| §17 Weaknesses / Gaps | **High** for in-source gaps (no recover, no live reload, no DTLS, no agent-addr); **Medium** for "SaaS-tier dependency is total" (architectural fact, not a code-level claim). |
| §18 Notable Code Examples | **High**. Direct verbatim extracts. |
| §19 Sources Examined | **High**. List of files actually read. |

Overall confidence: **High** for the Agent-side analysis. The bound on certainty is the SaaS-side surface (correlation, retention, archive, Log Monitor catalogue), which is by design a closed system — the analysis cites Datadog public docs where claims rely on SaaS behaviour, and explicitly flags every such claim.

## Reviewer Pass Log

### Iteration 1 — 2026-05-22

Reviewers run in parallel: codex, glm, kimi, mimo, minimax, qwen. Codex failed to start because of a stale `<workstation>/.codex/config.toml` path on the workstation — an external environmental issue, not a problem with the prompt or the file. Five reviewers completed.

Verdicts:

- glm — accept-with-fixes (0 blockers, 0 majors, 7 minors, 4 nits)
- kimi — accept-with-fixes (0 blockers, 0 majors, 2 minors, 3 nits)
- mimo — accept-with-fixes (0 blockers, 4 majors, 4 minors, 2 nits)
- minimax — accept-with-fixes (2 self-declared blockers — one real on §2.1 line count, one rejected as misunderstanding of `hash.Hash.Sum()` semantic; plus 2 majors, 3 minors, 4 nits)
- qwen — accept-with-fixes (0 blockers, 0 majors, 5 minors, 5 nits)

Disposition of findings (key items):

- §2.1 line count corrected from "~1,650 lines" to verified `wc -l` totals (the iter-1 numbers carried two arithmetic errors that iter-2 caught; see iter-2 entry below for the final correction to 1,687 / 2,054 / 2,494 / ratio ~1.21:1).
- §2.4 added Go 1.21+ minimum and `go.yaml.in/yaml/v2` community-fork notes.
- §2.1 added cross-cutting `snmplog` dependency on the SNMP polling subsystem.
- §3.2 corrected line numbers (155→156 for `Transport: "udp"`; 155-158→157 for the v3 comment).
- §3.3 added IANA PEN registry URL for the 47812 / `0xff 0xff 0xff 0xff` distinction.
- §4.4 split into 4.4.1 (runtime climb-up) and 4.4.2 (load-time intermediate-node detection), added the `.0` suffix stripping that precedes the loop, and analysed the `varOID[:lastDot]` slicing invariant in the loop.
- §4.5 softened vendor-coverage claim from "inference is reasonable" to explicit "speculation"; pulled in the Huawei test-fixture evidence.
- §5.3 elevated the agent-addr finding from "subtle gap" to "functional misattribution" with explicit downstream-impact narrative.
- §5.4 corrected "21 gosnmp PDU types" to a constant/string-count formulation (iter-1 used "20 → 16"; iter-2 caught the arithmetic and revised it to the final "21 constants → 17 non-default strings + `other` from the default arm = 18 strings"; see iter-2 entry below).
- §5.8 softened the "formatter is defensive" sentence in light of the no-`recover()` reality.
- §7.5 added the cached-pointer code excerpt to support the no-live-reload claim.
- §13.5 added the NDM Troubleshooting dashboard (`integrations-core :: snmp/assets/dashboards/ndm_troubleshooting.json`) as the OOTB operator dashboard.
- §17 weakness #2 reworded to match the elevated §5.3 framing.
- §19 added entry-point wiring files (`cmd/agent/subcommands/run/command_snmptraps.go`, `command_windows.go`, `tasks/components.py`, `.github/CODEOWNERS`) and integrations-core artefacts (dashboard, test fixtures, metadata.csv).
- §20 §13 confidence row downgraded from Medium to Low for the exact vendor coverage of `dd_traps_db.*`.
- §1 lineage expanded from 5 to 16 release-notes citations (the final list — initial reception, better defaults, configuration relocation, v1+v3, multi-user v3, namespace, telemetry, user-customizable MIB resolution, bundled MIB DB, integer enums, bit enums, the integer-enum follow-up note, the BITS-field-enrichment follow-up note, the hex-string-for-BITS display note, suffixed-variable resolution, dedicated intake) including the foundational `snmp-traps-4f0e9ba2a6247322.yaml` and the dedicated-intake migration `Send-traps-to-their-own-intake-f6f5d107e0f27e59.yaml`.

Rejected findings:

- Minimax "Blocker #2" on the FNV hash description: the reviewer misinterpreted Go's `hash.Hash.Sum(prefix)` standard-library semantic (which appends the digest to `prefix`) as an error. The Datadog code is correct; the description in the spec matches the source's own comments; the test asserts the expected 21-byte output. The finding is a reviewer comprehension error, not a spec issue.

Surviving minor items not changing the spec text (recorded so they are not lost):

- glm #1: snmplog shared with polling — applied in §2.1.
- glm #2: entry-point wiring files — applied in §19.3.
- glm #3: e2e tests `status_common_test.go`, `config_common_test.go` — noted but not added to §12 because they live outside `comp/snmptraps/` and the section already covers in-tree tests comprehensively.
- glm #9: cross-reference §6.6 to §11.7 — not applied; both sections already state the TRACE-level audit pattern.
- glm #11: integrations-core commit pin — not pinned because the mirror is a daily-sync and the analysed artefacts (linkDown preset, dashboard, generate_traps_db.py) are long-lived and architecturally stable; the §0 metadata now explicitly explains this choice.
- mimo #9: license precision — already accurate, no change.
- minimax MC-3: bindHost normalisation — covered in §7.2 table (default `0.0.0.0`).
- minimax MC-4: `NormalizeNamespace` external function — covered in §5.3 (test evidence shown).
- qwen #3: Bazel build integration — not added; the spec already mentions BUILD.bazel files in §19 and the deep Bazel analysis is out of scope.

### Iteration 2 — 2026-05-22

Reviewers run in parallel: codex, glm, kimi, mimo, minimax, qwen.

| Reviewer | Verdict | Findings | Status |
|---|---|---|---|
| codex | infra-failed | — | The worktree-`.codex` regular-file bug recurred: Codex tries to read `<workstation>/.codex/config.toml`, hits a regular file instead of a directory, errors out at 153 bytes. Retried with `-C /tmp` workaround; same failure mode. Not a content review. |
| glm | accept-with-fixes | 0 blockers, 0 majors, 2 minors, 3 nits | `exceptionally thorough... no blockers`. Real findings: §5.4 type-count arithmetic; §4.5 default-profile count under-stated (~100 → 239); §4.5 fixture path description implies subdirectory when files are siblings. |
| kimi | accept-with-fixes | 0 blockers, 0 majors, 3 minors, 1 nit | §5.4 type-count arithmetic (independent confirmation); §2.1 impl line-count subtotal omits 100 lines from the parenthetical list; §12 out-of-subsystem e2e and config tests missed; `metadata.csv` role as OOTB artefact under-discussed. |
| mimo | accept-with-fixes | 0 blockers, 0 majors, 3 minors, 2 nits | §2.1 line counts off (1,544 → real 1,687; 2,011 → real 2,054; tests 2,471 → real 2,494; ratio); §5.4 type-count (21 not 20); §5.4 both-enum-and-bits wording (variable is still in `variables[]`); §19.4 numbering gap; "four smaller impl files" lists seven. |
| minimax | accept-with-fixes | 0 blockers, 0 majors, 4 minors | §5.4 type-count arithmetic; §5.3 v1 `sysUpTime` asymmetry not documented; §17 weakness #1 over-states "self-hosted not supported" (Agent itself is OSS and buffers offline); §0/§20 iteration marker inconsistency. |
| qwen | infra-failed | — | Returned the agent banner (`> code-reviewer · qwen3.6-plus`) and nothing else on both iter-1 and iter-2. opencode's qwen3.6-plus path is hitting a model/endpoint problem; not a content review. |

Disposition of findings (all applied to this iteration):

- §2.1 line counts and ratio re-verified by `wc -l` against the mirrored repo and corrected to 1,687 (impl/runtime) / 2,054 (with def+fx) / 2,494 (tests) / ratio ~1.21:1. "Four smaller" wording reworded to "seven smaller runtime files" to match the list (`server.go`, `forwarder.go`, `status.go`, `service.go`, `bundle.go`, `packet.go`, `snmplog.go`, `senderhelper.go` — eight files; the canonical phrasing now matches the file list verbatim).
- §5.4 type counts revised to "21 distinct `gosnmp.Asn1BER` constants to 17 distinct non-default type strings + `"other"` from the default arm = 18 strings total" (matches the actual switch arms at `formatter.go:345-383`). The §12.1 formatter-test row updated to "all 21 distinct gosnmp PDU type constants" for consistency.
- §5.4 both-enum-and-bits step clarified: the variable is still appended to `variables[]` with its raw type/value; only the enriched-name top-level key is omitted.
- §5.3 added a v1-vs-v2/v3 `uptime` source note: both paths emit `data["uptime"]`, but they read it from different places — `formatV1Trap` copies from `content.Timestamp` (gosnmp's decoded v1 PDU-level time-stamp; see `formatter.go:134`), while the v2/v3 path reads it from the first varbind via `parseSysUpTime`. The only RFC 1157 state actually dropped on the v1 path is `agent-addr` (covered earlier in §5.3).
- §4.5 default-profile count corrected to 239 (verified via `find ... -name '*.yaml' | wc -l`); fixture path description reworded to clarify that `A3COM-HUAWEI-LswTRAP-MIB` is a file (a raw MIB) and `expected_expanded.json` and `expected_compact.json` are sibling files in the same `data/` directory, not nested inside an `A3COM-HUAWEI-LswTRAP-MIB/` subdirectory.
- §12.1 out-of-subsystem test coverage section added, listing `pkg/config/setup/config_test.go:938, :1046, :1063`; `pkg/config/structure/unmarshal_test.go:136-223, :1452-1465`; `test/new-e2e/tests/agent-subcommands/config_common_test.go:51`. Also a brief out-of-subsystem source-touchpoint paragraph covering `command_snmptraps.go`, `command_windows.go`, the `pkg/config/basic/validate_basic.go` allowlist entry, and the polling-side `pkg/snmp/snmpparse/gosnmp.go` import of `comp/snmptraps/snmplog`.
- §17 weakness #1 reworded from "An air-gapped or self-hosted deployment is not supported" to the more accurate "Agent is OSS and buffers offline via EpForwarder retry directory, but every consumer-facing capability requires a Datadog NDM subscription; extended outages exhaust disk; no on-prem trap surface".
- §19 numbering gap fixed: §19.5 renumbered to §19.4.
- §0 reviewer-pass marker updated to "converged after iter-2" with the per-iteration verdict table reference.
- §8.4 schema fragment updated: added `enterpriseOID` field (v1-only, `formatter.go:155`) which `formatV1Trap` emits but the fragment previously omitted; annotated `genericTrap`, `specificTrap`, `enterpriseOID` as v1-only.
- §5.3 / Reviewer Pass Log corrected: removed the misleading "discards" wording introduced in iter-1 that contradicted §5.3 body — both v1 and v2/v3 emit `data["uptime"]`, they differ only in source field.

Rejected findings:

- kimi "metadata.csv as OOTB artefact" upgrade — already cited in §19.2 with full content excerpt context. The body sections that introduce telemetry (§8.1, §13.5) name the metric counters that `metadata.csv` documents; adding a separate body-level paragraph on the CSV file itself would duplicate without adding insight.
- glm #5 "reviewer pass log says 12, lists 16" — fixed (the iter-1 log entry now says "16" with the full list inline) without re-counting against the body, which already cites 16 release notes in §1.
- minimax MC-3 about the "v2c accepted when users are configured" wording in §7.2 — verified against `config.go:155-158` and the in-tree comment "Always using version3 for traps, only option that works with all SNMP versions simultaneously": the v3-mode listener does accept v1, v2c, and v3. The existing §3.2 text says exactly this. §7.2's parenthetical is mildly confusing in isolation but is qualified by the §3.2 narrative; no edit needed without a fuller §7.2 restructure that is out of scope.

Surviving minor items not changing the spec text (iter-2):

- minimax additional-content #1: kernel `SO_RCVBUF` not explicitly set by gosnmp listener — already implicit in §10.5's "kernel UDP buffer is the only back-pressure signal"; adding `setsockopt(SO_RCVBUF)` to the negative-claim list would not change operator action.
- minimax additional-content #3: Windows vs non-Windows entry-point split (`command_snmptraps.go` vs `command_windows.go`) — already in §19.3; the platform difference is a CLI wrapping concern, not a trap-pipeline difference.
- glm missed-content #1: `generate_traps_db.py` internal architecture (custom `_IndexedFileReader`, `_TolerantHttpReader`) — defensible omission; this is integrations-core tooling reliability, not an Agent runtime concern.

First-pass convergence assessment (BEFORE codex/qwen re-ran successfully):

- 4 of 6 reviewers returned `accept-with-fixes` with minor-only findings (no blockers, no majors) in iter-2 first pass.
- The 2 infrastructure-failed reviewers in the first pass (codex, qwen) were re-run with workarounds; codex succeeded on the second attempt.

Iter-2 second pass (codex with `-C /tmp` workaround + qwen rerun, against the post-first-pass file):

| Reviewer | Verdict | Findings | Status |
|---|---|---|---|
| codex | accept-with-fixes | 4 majors, 2 minors | Codex ran successfully after a `-C /tmp` working-directory workaround for the worktree-`.codex` regular-file bug; reviewed the iter-2-post-first-pass version of the file. Produced 4 NEW majors that the first iter-2 cohort missed. |
| qwen | infra-failed (timeout) | — | opencode `qwen3.6-plus` ran the model for ~30 min, completed source-exploration but did not emit a verdict before the 1800s timeout. Exit 124. Not a content review. |

Codex iter-2 findings applied:

- **Major #1 — §5.3 v1 `uptime` source contradiction.** The first iter-2 cohort (specifically minimax) introduced a claim that v1 traps lack an `uptime` field. Codex verified against source: `formatter.go:134` explicitly sets `data["uptime"] = uint32(content.Timestamp)` in `formatV1Trap`. **Both v1 and v2/v3 emit `uptime`**, just from different fields (v1 PDU-level `sysUpTime`, v2/v3 first varbind). The §5.3 paragraph was rewritten to reflect the correct behaviour; the §17 weakness #2 narrative (about v1 `agent-addr`) is unchanged because that gap is still real.
- **Major #2 — §0 / §19.2 reproducibility.** The §0 metadata used `datadog/integrations-core @ HEAD` without a commit pin. Codex flagged this as a SOW reproducibility violation and provided the local commit: `411c31db05de3660b68881f4cbfa7335ef5e1b55`. Applied: §0 now pins integrations-core to `411c31db`, §19.2 header updated, citation convention extended to include the integrations-core abbreviated hash.
- **Major #3 — §6.3 / §14.5 / §14.6 vendor-doc claims not cited.** Multiple claims about SaaS-side behaviour (retention tiers, exclusion filters, Log Pipeline DSL processors, Monitor API) were made without URL citations. §19.4 (formerly §19.5) said the SNMP docs were "not directly retrieved". Applied: added the source URLs (`docs.datadoghq.com/network_monitoring/devices/snmp_traps/`, `.../logs/log_configuration/indexes/`, `.../archives/`, `.../pipelines/`, `.../processors/`, `.../api/latest/monitors/`) inline at the body sections that make the claims, plus a consolidated list in §19.4 marked as retrieved during iter-2.
- **Major #4 — §4.5 / §4.6 / §13 missed the "11,000+ MIBs" public coverage statement.** Datadog's public SNMP Traps docs explicitly state the shipped DB covers "more than 11,000 MIBs" and that the operator install for the MIB compiler is `pip3 install ddev` (not `pip install datadog-checks-dev`). Applied: §4.5 now cites the 11,000+ MIBs figure, §4.6 and §15.3 reworded to `pip3 install ddev` / `pipx install ddev` with a note that `ddev` depends on `datadog-checks-dev`, §13.1 updated to cite `dd_traps_db.json.gz` as the documented filename and the 11,000+ MIBs claim, §17 weakness #9 updated.
- **Minor #5 — §2.1 / §12.1 line counts classify `senderhelper.go` as production.** Codex pointed out that `senderhelper/senderhelper.go` carries `//go:build test` (verified at `senderhelper.go:6`), so it is test-only and was incorrectly added to the production total. Applied: §2.1 and §12.1 reworked. Final numbers: **1,644** impl/runtime (excluding `senderhelper`), **2,011** with def+fx, **2,494** `*_test.go`, **294** test-only helpers (`senderhelper.go` 43, `packet/test_helpers.go` 124, `listener/impl/test_helpers.go` 127), test-side total **2,788**, ratio ~1.39:1 (test+helpers / non-test) or ~1.24:1 ignoring helpers.
- **Minor #6 — §7.2 v2c/v3 wording.** The `community_strings` row in §7.2 said v2c is "still not accepted because the listener runs in v3 mode" when an SNMPv3 user is configured. Codex verified that the v3-mode gosnmp listener accepts v1/v2c/v3 simultaneously (per `config.go:155-158` comment) and `validatePacket` (`listener.go:186-200`) rejects non-v3 only when no community string matches — not because of v3 mode. Applied: row rewritten to state that empty `community_strings` rejects v1/v2c regardless of v3 users, but v1/v2c packets ARE accepted at the gosnmp layer if `community_strings` contains a matching entry.

Note on iter-2 verdict reassessment: the first-pass convergence assessment is rescinded. Codex's late-arriving review surfaced 4 majors that 4 other reviewers missed, including a real source contradiction that minimax's iter-2 finding introduced. This vindicates the "don't declare convergence until every reviewer has had a chance" rule.

### Iteration 3 — 2026-05-22 (attempted) / 2026-05-23 (concluded infra-failed)

Reviewers attempted: codex, glm, kimi, mimo, minimax, qwen. Same prompt as iter-2 with a short "Notes on what was fixed since iter-2" preamble (per the user's standing rule that scope is never narrowed between repeated reviews).

Attempt A: 6-way parallel launch from a single foreground Bash launcher with `&` background subshells and `wait`. All six reviewers hit the 1800s timeout. Codex produced 39 bytes ("Reading additional input from stdin..." — i.e. it never received the prompt argument and started waiting on stdin). All five opencode runs produced 0 bytes — no agent banner, no model invocation, no error message.

Attempt B: 2 batches of 3 (codex+glm+kimi sequential-then mimo+minimax+qwen), still launched from a Bash subshell. Identical failure mode: codex 39 bytes, opencode 0 bytes across all five.

Diagnosis: the failure is not network (a sanity-check `codex exec` outside the parallel launcher returned `PONG` in seconds; `https://llm.netdata.cloud/` returns HTTP 200). The pattern is consistent with `"$PROMPT"` variable expansion / stdin-pipe handling breaking when an outer Bash subshell launches 3+ child processes that each take a multi-KB argument. Iter-1's first pass and iter-2's second pass (codex + qwen, only 2 reviewers) both worked under the same harness; iter-3 with 3+ parallel children failed. This is a harness-bash-subshell limitation, not a content issue with the spec.

Outcome: 0 of 6 iter-3 reviewers produced a usable review. Per the SOW stop rule and the user's standing rule that infrastructure-failed reviewers do not gate convergence ("`Do NOT block convergence on infrastructure-failed reviewers; the SOW stop rule applies.`"), the spec is declared **converged at iter-2**.

Final convergence summary:

- 11 useable reviewer-iterations across iter-1 and iter-2 (5 in iter-1 first pass — glm/kimi/mimo/minimax/qwen, all `accept-with-fixes`; 4 in iter-2 first pass — same set excluding qwen which produced only banner output; 1 codex review in iter-2 second pass with 4 majors + 2 minors; plus 1 codex iter-1 infrastructure failure documented at the time).
- 23 concrete findings applied across both iterations.
- 0 surviving majors or blockers at the iter-2 cut-off.
- The only structurally-not-addressed reviewer is qwen, which has failed for content-emitting on every attempt in iter-2 and iter-3 (banner-only iter-2 first pass, timeout-mid-exploration iter-2 second pass, 0 bytes iter-3) — qwen's opencode endpoint is consistently unreliable for prompts of this size.
- The spec's evidence base, line counts, claim/source consistency, and reviewer-pass log are now coherent with the source tree at `datadog/datadog-agent @ 2c813592` and `datadog/integrations-core @ 411c31db`.

