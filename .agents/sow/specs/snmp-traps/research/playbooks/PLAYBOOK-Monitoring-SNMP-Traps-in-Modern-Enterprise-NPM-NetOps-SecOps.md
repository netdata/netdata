## PLAYBOOK: Monitoring SNMP Traps in Modern Enterprise NPM / NetOps / SecOps

---

### SECTION 0 — Operator's Mental Model

SNMP traps are asynchronous, push-based event notifications sent from a managed device's SNMP agent to one or more trap receivers (NMS collectors, SIEM forwarders, integrated observability backends) over **UDP port 162**. They are one of several telemetry legs in modern Network Performance Monitoring, sitting alongside:

- **Active SNMP polling** (UDP/161, pull-based) — authoritative state confirmation
- **Streaming telemetry** (gNMI/gRPC over TCP+TLS, push-based) — high-resolution periodic state and on-change subscriptions
- **Syslog** (UDP/514, TCP/601) — narrative event logs
- **NetFlow / sFlow / IPFIX** — flow accounting on the data plane
- **Synthetic probing** (ICMP, TWAMP, HTTP) — independent path validation
- **Vendor control-plane APIs** (Cisco DNA, Juniper Mist, Aruba Central, Meraki, Arista CloudVision) — managed-service event channels

Traps remain indispensable because every network device in production supports them, they have near-zero polling overhead on the control plane, and they catch transient state transitions (link flaps, peer resets, environmental crossings, authentication failures) that polling would only discover at the next interval. Streaming telemetry is gradually replacing SNMP *polling* for metrics; it is not replacing *traps*, which remain the dominant event/alerting mechanism for hardware fault, environmental threshold, and vendor-native state changes. Modern enterprises run a **hybrid posture**: traps and streaming telemetry for events, polling for state confirmation, syslog for narrative, flow for traffic context.

#### Core subsystems always active

1. **SNMP Agent / Notification Originator (RFC 3413).** On the managed device, internal event logic decides what is noteworthy enough to trap on. Trap generation is configured per-vendor (`snmp-server enable traps …`, `event-options`, MIB-specific flags). Traps egress via the device's routing table from whichever interface the route to the receiver resolves through; the source IP is the egress interface IP by default unless pinned with `snmp-server trap-source` or equivalent. This source-IP behavior is a recurring source of operational confusion.

2. **Trap Transport (UDP/162, no retransmit, no ACK).** v1 traps use a fixed legacy PDU format (enterprise OID + specific-trap integer + timestamp). v2c traps add `sysUpTime` and `snmpTrapOID` varbind structure. v3 traps wrap v2c with USM authentication and optional privacy, and add an `snmpEngineID`. **Informs** (RFC 3413 §4) are acknowledged variants — the receiver must reply; the agent retransmits until acknowledged or retry budget exhausted. Standard production deployments still overwhelmingly use plain traps and absorb the loss; informs are reserved for critical events where guaranteed delivery justifies the resource cost.

3. **Trap Receiver / NMS Listener.** A daemon binds UDP/162, decodes varbinds against loaded MIBs, applies filters, deduplicates, enriches, and forwards to downstream stores. Two architectural classes exist: **native trap daemon** (Net-SNMP `snmptrapd`, lightweight) and **integrated NMS receiver** (commercial appliances, Kafka/RabbitMQ-fed collectors, cloud-native log routers accepting trap-to-CEF/LEEF/syslog translation). The listener's queue depth, MIB pack, and filter ruleset are operationally critical.

4. **MIB Resolution Engine.** Maps numeric OIDs to human-readable names. Without the loaded MIB, varbinds arrive as opaque numeric OIDs; operators stop reading the feed because they cannot interpret it. With large vendor MIB sets (500+ files), MIB resolution can become the single largest per-trap processing cost — algorithmic lookup without pre-indexed structures scales unfavorably.

5. **Downstream Consumer Stack.** Decoded traps get fanned out to SIEM (for SecOps correlation), ticketing (for NetOps remediation), CMDB (for inventory updates), change management (for config-change correlation), and chatops. The fan-out is what makes traps useful — a single `coldStart` from a new switch should create a CMDB record *and* a security event *and* a topology update.

6. **Security Subsystem (Community / USM / VACM).** v1/v2c use a cleartext community string as the only authentication. v3 introduces USM with HMAC authentication (SHA/SHA-256/MD5) and privacy (AES/DES), and VACM for authorization. Engine IDs must be discovered before v3 traps flow — a fundamentally bidirectional requirement that interacts badly with unidirectional trap delivery (see failure archetype below). Most enterprises still run v1/v2c in production because v3 trap support on device firmware lags v3 polling support.

7. **Filtering and Normalization Layer.** Almost no receiver passes every trap through unchanged. Filters suppress (coldStart during maintenance), aggregate (linkUp+linkDown within 5s = flap), enrich (resolve source IP to device, reverse-DNS to hostname), and normalize (map vendor OIDs to a common schema). The filter rule set is itself a major source of operational risk — see §4 and §7.

8. **Distribution / Forwarding Layer.** Many enterprises run edge collectors per region forwarding to a central SIEM, or devices sending to multiple managers for redundancy. Each hop introduces rewriting (v1-to-v2c translation), OID mutation, additional UDP loss, and forwarding-loop risk. Multi-receiver deployment is constrained: most devices support a small fixed upper limit on configured receivers.

#### Resources the technology competes for

- **UDP socket receive buffer on the receiver host.** Each received trap sits in kernel UDP buffer until userspace reads. Under storm, the buffer fills and `RcvbufErrors` in `/proc/net/snmp` increments. This is the **first** silent loss point. Linux default `net.core.rmem_max` is 212,992 bytes (~208 KiB) — entirely inadequate for any production trap volume.
- **Trap receiver CPU.** MIB decoding is single-threaded on most legacy daemons. A flapping link at 10 events/sec pinning one core; a spanning-tree reconvergence generating thousands of events/sec will saturate the decoder long before the socket buffer fills.
- **Trap receiver memory.** MIB tree size (hundreds of MB for a large vendor pack), queue depth, in-flight events, and cache growth all compete for RAM. OOM is the typical death for a misconfigured receiver during storm conditions.
- **Disk on trap store / SIEM ingest.** 1 Gbps of trap traffic at full filtering is small; an unfiltered flap storm is gigabytes per hour. Disk-full is a common cliff.
- **Management-plane network bandwidth.** Traps share OOB management (or in many enterprises, in-band VLAN) with polling, syslog, SSH, and NTP. A trap storm can saturate the management path and degrade *all* management functions simultaneously.
- **Source device CPU.** Trap generation is cheap individually, but under storm conditions (STP TCN, BGP flap, environmental cascade) the device's own control plane is the very thing the traps are reporting on — co-saturation is real.
- **File descriptors and downstream connections.** Trap receivers fanning out to SIEM, message queue, database, syslog, and webhook consumers consume FDs and connection slots.

#### Characteristic failure archetypes

1. **Trap Storm Cascade.** A flapping link, bridging loop, or routing reconvergence generates thousands of traps per second. The kernel socket buffer overflows (`RcvbufErrors` climbing). The decoder saturates (CPU pegged, latency exploding). The downstream SIEM hits its ingest quota and either back-pressures (queue grows) or silently drops. The root-cause traps (often the first to arrive) are buried under the cascade. Empirically documented at hyperscale: a single cable failure at Alibaba Cloud produced >10,000 alerts within minutes, prolonging resolution by hours because the SNMP congestion alert — the actual root cause signal — was obscured by downstream alert flood.

2. **Silent Trap Loss (UDP Black Hole).** UDP packets dropped in transit (network congestion, ACL/firewall change, intermediate router buffer overflow, NAT/UDP mapping expiry, kernel socket overflow) leave no negative signal. The receiver is unaware anything was sent. The team discovers the loss when the next incident that *should* have paged them doesn't.

3. **SNMPv3 Authentication Mismatch (Silent Drop on the Receiver).** After a credential rotation, device reimage, or receiver reinstall (new engine ID), v3 traps arrive at the receiver but USM validation fails. The receiver silently discards them. There is no log entry at default verbosity. The team's monitoring looks healthy; their devices' traps are vanishing.

4. **Engine Time Window Lockout (SNMPv3).** Per RFC 3414, SNMPv3 messages with `msgAuthoritativeEngineTime` outside ±150 seconds of the receiver's cached value are rejected with `usmStatsNotInTimeWindows`. Traps are unidirectional — the receiver cannot autonomously discover the sender's new engine time after a device reboot. If the agent reboots and the receiver's cache is stale, the device enters permanent trap lockout until the receiver issues a bidirectional request (GET) that triggers a report packet refreshing the cache.

5. **MIB Coverage Gap (Silent Reception Failure).** New device model or firmware upgrade introduces OIDs the receiver can't resolve. Traps are received, logged, and "processed" — but appear as numeric OID strings. Alert rules written against resolved trap names don't match. Critical events are received but operationally invisible.

6. **Flapping Interface Trap Flood.** A single interface oscillating (bad cable, failing SFP, duplex mismatch, BPDU guard kicking in) generates `linkUp`+`linkDown` pairs at multi-Hz rates. Dedup suppresses most, but the dedup engine itself consumes processing resources. The trap volume crowds out legitimate events from other devices.

7. **Trap Receiver Process Death (Silent).** The receiver daemon crashes (OOM, segfault, deadlock, GC pause in JVM-based receivers). The kernel continues accepting UDP datagrams into the socket buffer, but nothing drains it. The buffer fills; the kernel drops. All metrics look like "no traffic," which is indistinguishable from "nothing is happening."

8. **Source IP Ambiguity / NAT / HA Pair Confusion.** A trap's source IP is whatever the device decided to egress from. After a routing change, the same physical device appears as multiple sources, or multiple devices appear as one. The receiver's per-source tracking breaks.

9. **Misconfiguration Drift (No Traps Arrive At All).** A firewall rule change blocks UDP/162 from a management subnet. Devices in that subnet stop sending traps. The receiver appears healthy (it still receives from unaffected subnets). The monitoring gap is invisible until a critical event in the affected subnet goes unalerted.

10. **Filter Over-Aggregation Suppressing Real Events.** A dedup rule set to "suppress more than N linkDown per minute from a device" silences a real line-card failure affecting many ports simultaneously. Operators see normal trap volume; 45 of 48 interface-down events are swallowed by the dedup engine.

11. **Clock Skew Across Sources.** Traps carry `sysUpTime.0` (timeticks since agent boot), not wall-clock time. If a source device's NTP is broken, the implied event time is wrong, and cross-source event ordering breaks silently. Forensics become unreliable.

12. **Auth/Version Mismatch in v1/v2c Mixed Fleet.** A receiver expecting v2c rejects v1 traps (PDU structure differs); a receiver accepting v1 may misparse v2c varbind structures. Version impedance between device firmware and receiver configuration produces parse errors rather than the expected trap flow.

#### Deployment variants that fundamentally change what you monitor

1. **SNMPv1 vs v2c vs v3.** v1/v2c carry community strings in cleartext on the wire (trivially sniffer-extractable) and offer no integrity or authentication. v3 adds USM (HMAC + optional encryption) and replay protection. The monitoring strategy differs: v2c emphasizes delivery reliability and source IP validation; v3 adds USM-statistics monitoring as a first-class concern. v1 trap PDU structure is fundamentally different from v2c/v3 (enterprise OID + specific-trap integer + timestamp vs `snmpTrapOID` varbind) — receivers often mishandle v1-v2c impedance.

2. **Trap vs Inform.** Standard traps are unacknowledged; informs require receiver Response and agent retransmit. Informs provide delivery guarantees at the cost of memory (unacknowledged informs held in agent memory), CPU (retry logic + crypto overhead on each retransmit), and per-vendor support gaps. Most production fleets use plain traps; informs are reserved for critical events.

3. **Standalone vs Distributed/Hierarchical Receivers.** Single receiver is simplest but is a single point of failure. Distributed architectures (edge collectors per region → central aggregation) introduce inter-receiver forwarding delay, v1-to-v2c translation at boundaries, and dedup-across-receivers logic. Most devices cap the number of configured receivers (commonly ≤6) — for HA, this limit forces use of repeaters/multiplexers upstream of NMS rather than direct device-to-receiver redundancy.

4. **On-Premise NMS vs SaaS / Cloud-Managed.** Cloud-hosted receivers add WAN latency, require VPN/DirectConnect, and complicate SNMPv3 engine ID management through NAT. UDP/162 is often blocked by default cloud security groups. SaaS platforms (Meraki, Aruba Central, Juniper Mist, Arista CloudVision) reflect a filtered, normalized event stream downstream — the customer loses visibility into raw trap volume, source identity, and unfiltered event types.

5. **Integrated NMS Receiver vs Log/SIEM Collector Accepting Traps.** A purpose-built NMS receiver (SolarWinds, PRTG, NNMi, OpenNMS, Zabbix proxy) handles decode/dedup/filter/store natively with deep internal instrumentation. A SIEM trap collector (Splunk Connect for SNMP, Elastic Agent, Datadog) translates traps to the SIEM's native event schema and forwards. The first has richer queue-depth and pipeline-latency signals; the second has richer correlation signals but thinner pipeline metrics.

6. **Active-Active vs Active-Passive HA.** Active-active distributes load across all nodes; active-passive keeps a hot standby idle. Active-active tolerates single-node failure without capacity loss; active-passive wastes a node. The receiver process must be capable of running on both nodes simultaneously (NNMi's `nnmtrapreceiver` is one such example); a single-receiver daemon on a VIP that fails over is active-passive with the cost of a VIP-failover window where traps are lost.

7. **Standard Commercial Stack vs Open-Source Receiver + Custom Forwarder.** Commercial NMS platforms ship built-in trap receivers with GUI MIB management and pipeline metrics, but limited visibility into deep internals. Open-source stacks (Net-SNMP `snmptrapd`, Telegraf snmp_trap plugin, `snmpsim`, custom Python handlers) provide full visibility but require custom instrumentation. The monitoring burden shifts from "configure the platform" to "instrument the pipeline."

---

### SECTION 1 — Signal Catalog

Signals grouped by concern domain. Domains are ordered by detection priority — the domain that surfaces real issues earliest with the best signal-to-noise first. Within each domain, signals are ordered by severity tier (PAGE first), then by leading-indicator quality, then by frequency of operational use.

---

#### Domain: Availability — Is the Trap Pipeline Alive?

**SIGNAL: Trap Receiver Process Liveness**

WHAT IT IS:
Whether the trap receiver daemon is running and bound to UDP/162. The most fundamental health check — if the daemon is dead, every trap from every device is silently dropped by the kernel.

SOURCE:
- Process table: `ps -ef | grep snmptrapd` (or vendor daemon name)
- Systemd: `systemctl status snmptrapd`
- Socket state: `ss -l -u -n | grep :162`
- Vendor NMS health endpoint (HTTP) if exposed

HOW TO COLLECT IT MANUALLY:
```bash
ps -ef | grep snmptrapd | grep -v grep
systemctl status snmptrapd
ss -l -u -n | grep ':162 '
# Functional liveness test:
snmptrap -v2c -c <community> <receiver-ip> '' SNMPv2-MIB::coldStart.0 sysUpTime.0 t 1
```

WHAT IT TELLS YOU:
Process absent or socket not bound means the entire trap pipeline is dead. Devices continue sending to UDP/162; the kernel drops every datagram. This is a complete monitoring blackout for trap-based alerting.

SEVERITY:
PAGE — Trap-based alerting is entirely non-functional. Reasoning: any event that should generate a trap-based alert (BGP peer drop, power supply failure, link down on critical interface) is now invisible. Corroborating signals that change verdict: if process is "running" but the synthetic test trap does not arrive at the log, the daemon is wedged — still PAGE, but the diagnosis is "process alive but non-functional" rather than "process dead." If only the receiving host is unreachable, distinguish from "receiver process dead" before paging.

THRESHOLDS:
Binary. Any transition from "process alive + socket listening" to either state for any nonzero duration in production is abnormal. Restart count > 2 in a rolling 1-hour window on a previously stable receiver indicates instability (escalates to PAGE; investigate for handler deadlock, memory leak, or upstream issue causing crash-loops).

FAILURE MODES DETECTED:
- Receiver daemon crash (OOM, segfault, unhandled exception in handler chain)
- Daemon startup failure (port conflict, permission, missing MIB files blocking initialization)
- Daemon alive but wedged (accepting socket but not reading — deadlocked on lock, GC pause, or handler infinite loop)

NUANCES & GOTCHAS:
A process can be alive (PID exists, systemd "active") but wedged (not draining socket). Process liveness alone is insufficient — combine with a functional test (send a test trap, verify it appears in the log within seconds) and with the synthetic-trap heartbeat signal. `snmptrapd` with embedded Perl/Python handler extensions can hang in handler code while the main process appears alive. JVM-based receivers can enter extended GC pauses that look like hangs. After a receiver restart there is a startup window where the daemon binds the port and is "listening" but MIB compilation is incomplete — traps received in this window may be logged as unknown OIDs. MIB compilation in `snmptrapd` can take 30 seconds to 5+ minutes for large vendor MIB sets.

CORRELATES WITH:
- **Traps Received Rate (application-level)**: Drops to zero while the synthetic-trap test fails — receiver is dead or wedged.
- **UDP `RcvbufErrors`**: Climbs while process is "alive" — daemon is wedged (kernel buffer filling because userspace is not reading).
- **Trap end-to-end forwarding latency**: Spikes to infinity while process is "alive" — daemon is processing but stuck before completion.

---

**SIGNAL: Synthetic Trap Heartbeat (End-to-End Liveness)**

WHAT IT IS:
A known trap generated on a schedule from a test source (a canary device, a synthetic test agent, or a periodic script on a test box), whose arrival at the receiver — and ideally at the downstream consumer — is asserted on a freshness interval. The only reliable detector of "traps are arriving at the kernel but not at the application" or "traps are arriving at the application but not at the SIEM."

SOURCE:
- Synthetic test agent: `snmptrap` command on a schedule, OR a purpose-built tool (`snmpsim`, MIMIC SNMP Simulator, Datadog's trap simulator, Trishul-SNMP)
- Receiver log entry matching the synthetic trap's identifier
- Downstream consumer (SIEM) query for the synthetic trap's marker

HOW TO COLLECT IT MANUALLY:
```bash
# On a test host with permission to send traps to the receiver:
snmptrap -v2c -c public <receiver-ip> '' \
  NET-SNMP-EXAMPLES-MIB::netSnmpExampleHeartbeatNotification \
  netSnmpExampleHeartbeatDateAndTime.0 s "$(date -u +%Y-%m-%dT%H:%M:%SZ)"

# Then verify the trap arrived (and ideally landed in the SIEM):
grep netSnmpExampleHeartbeatNotification /var/log/snmptrapd.log | tail -3
```

WHAT IT TELLS YOU:
Whether the trap path works end-to-end at the moment of test. Because UDP is unreliable and silent on loss, and because every other signal in this section is either laggy (rate counters) or self-referential (process is up), this is the only signal that catches "everything looks green but traps are not actually arriving at the place where alerts fire." Without it, you discover the failure when a critical event is silently dropped.

SEVERITY:
PAGE — End-to-end path is broken. Reasoning: every other failure-detection signal downstream of this is now suspect. The team's entire event-driven monitoring is operating blind.

THRESHOLDS:
- Heartbeat sent at fixed interval (1-5 minutes is typical). Page if no synthetic trap observed at the downstream consumer for > 3× the heartbeat interval.
- Workload-dependent: some teams run synthetic traps every 60 seconds for critical paths, every 5 minutes for less critical ones.

FAILURE MODES DETECTED:
- Receiver daemon death or hang (synthetic trap not logged)
- Kernel socket buffer overflow (synthetic trap not received at application; tcpdump on UDP/162 confirms it arrived at the network)
- Firewall/ACL change silently blocking UDP/162 (synthetic trap not received at all)
- Forwarder to SIEM broken (synthetic trap logged at receiver but absent from SIEM)
- MIB pack corruption (synthetic trap logged but as numeric OID; alert rule on resolved name doesn't match)
- Time window on receiver clock skew (synthetic v3 trap rejected with `usmStatsNotInTimeWindows`)

NUANCES & GOTCHAS:
A synthetic trap that *arrives at the receiver* but does not *land in the SIEM* indicates a forwarder/forwarding-path failure — a distinct failure mode from "receiver is down." Instrument the heartbeat at both the receiver and the downstream consumer; the gap between the two tells you where the path is broken. The synthetic test source should run on a *different* management path than the device it tests, where possible — otherwise a path failure that affects the synthetic source and the real devices in the same way will not be detected. For v3, the synthetic test must exercise the same credential path as real devices (USM, engine ID discovery) to catch auth/credential failures.

CORRELATES WITH:
- **Trap Receiver Process Liveness**: If process is up but heartbeat is missing, the daemon is wedged.
- **UDP `RcvbufErrors`**: Heartbeat absent + RcvbufErrors climbing = kernel dropping all incoming.
- **SIEM ingest quota and forwarder lag**: Heartbeat present at receiver, absent at SIEM = forwarder broken.
- **Unknown OID Rate**: Heartbeat present but as numeric OID = MIB pack broken for that OID.

---

**SIGNAL: Per-Source Trap Presence (Heartbeat per Device)**

WHAT IT IS:
For each managed device, the time elapsed since the most recent trap from that device was successfully received. A "no traps" reading is only meaningful relative to a per-device expected baseline — but an expected baseline of "at least one trap per day" (coldStart on reboot, periodic health, etc.) is realistic for most production devices.

SOURCE:
- Per-source state in the receiver's database (most NMSs expose per-source `last_seen`)
- Per-source log files if segregated
- CMDB/inventory cross-reference for "expected sources"

HOW TO COLLECT IT MANUALLY:
```bash
# From receiver log, find the last trap timestamp per source:
for src in $(awk '/ Trap /{print $NF}' /var/log/snmptrapd.log | sort -u); do
  echo "$src: $(grep -c "$src" /var/log/snmptrapd.log) total, last=$(grep "$src" /var/log/snmptrapd.log | tail -1 | awk '{print $1, $2}')"
done

# Compare against inventory to find expected sources that have gone silent:
comm -23 <(sort /var/lib/inventory/managed_ips.txt) <(awk '/ Trap /{print $NF}' /var/log/snmptrapd.log | sort -u)
```

WHAT IT TELLS YOU:
Whether a specific device is still capable of trapping. A device that polls perfectly but sends no traps has lost its trap path (configuration drift, SNMP agent frozen, trap-source interface down, trap destination removed from device config, v3 credential drift, source IP changed). Distinguishing "device is healthy and quiet" from "device's trap path is broken" requires either a per-device expected baseline or a synthetic heartbeat per device.

SEVERITY:
PAGE — For critical infrastructure (core, firewall, DC spine, WAN edge): the device's trap path is broken; the team is blind to its events.
TICKET — For distribution/access layer: investigate within shift.
PLAN — For non-critical edge.

THRESHOLDS:
Per-source silence exceeding the per-source 99th-percentile historical inter-trap gap, sustained for at least 15 minutes. Workload-dependent: heartbeat-generating sources have tight p99 (minutes); state-change-only sources have wide p99 (hours to days). For the special case of "device has gone cold (no trap in many hours/days when historical rate suggests at least daily activity)," any sustained deviation from per-device baseline is a finding.

FAILURE MODES DETECTED:
- Device SNMP subsystem frozen
- Source IP changed (renumber, interface change, VRF change, NAT)
- `snmp-server enable traps` removed by config change
- Trap-source interface down
- Local ACL on device dropping outgoing UDP/162
- v3 credential drift (engine ID change, key desync)
- Network path failure (firewall change, routing change, management VLAN break)

NUANCES & GOTCHAS:
A device can be entirely functional (routing, switching) with its SNMP subsystem broken. The trap path is independent of the data path. Polling may continue working even when the trap path is broken (different credential, different engine ID discovery path). Devices using v3 traps and lost engine ID sync will send traps that are silently rejected at the USM layer — the source IP is still in the receiver's log, but the trap is never processed. Cross-referencing "polling works, no traps" is the diagnostic for trap-path-specific failure.

CORRELATES WITH:
- **SNMP polling success for the same source** (UDP/161): If polling is fine but traps are silent, the trap path is broken; if both are silent, the device's SNMP subsystem is broken.
- **Syslog reception from the same source** (UDP/514): If syslog is fine but traps are silent, only the trap path is broken.
- **Synthetic Trap Heartbeat**: If a synthetic test trap from the same management network arrives but the device's traps don't, the issue is device-side or path-specific.
- **Device configuration audit**: A `show snmp host` (or equivalent) on the device reveals whether the trap destination is still configured.

---

**SIGNAL: Trap Delivery Path Reachability (UDP/162 from Sources)**

WHAT IT IS:
Whether SNMP traps can actually reach the receiver from the managed devices' perspective, as opposed to whether the receiver is alive. A healthy receiver behind a broken path is operationally equivalent to a dead receiver.

SOURCE:
- Per-source log entries (a source with no recent log entries despite expected activity)
- Source device's own trap-generation logs (Cisco: `show logging | include SNMP`; Juniper: `show log messages | match snmp`; Linux: `journalctl -u snmpd | grep -i trap`)
- Firewall/ACL logs for UDP/162 deny events
- Per-device CLI test: vendor-specific `snmp-server host` test commands

HOW TO COLLECT IT MANUALLY:
```bash
# From a managed device, trigger a known trap and verify receipt:
# Cisco:
debug snmp packets
# then trigger a test event, e.g., shut/no shut a test interface
# or use vendor-specific test commands.

# From a Linux host with snmp tools:
snmptrap -v2c -c <community> <receiver-ip> '' \
  IF-MIB::linkUp.0 ifIndex.0 i 1 ifAdminStatus.0 i 1 ifOperStatus.0 i 1

# Verify on the receiver:
tail -3 /var/log/snmptrapd.log
```

WHAT IT TELLS YOU:
Whether a specific source can deliver traps to the receiver. A path failure (firewall change, ACL modification, routing change, NAT UDP mapping expiry) presents as per-source trap silence. The diagnosis is path-level, not device-level.

SEVERITY:
PAGE — For critical infrastructure. Reasoning: same as per-source trap presence, plus the path issue may affect multiple devices.
TICKET — For non-critical or partial reachability loss.

THRESHOLDS:
Binary per source: any confirmed reachability loss is a finding. Sustained loss over 15 minutes is investigation threshold. Corroborate with SNMP polling (UDP/161): if both 161 and 162 are unreachable, the device itself may be down; if only 162 is unreachable, the trap path is specifically broken.

FAILURE MODES DETECTED:
- Firewall rule change blocking UDP/162 from specific subnets
- ACL change on management interface blocking trap forwarding
- Routing change affecting management VLAN reachability
- NAT device breaking SNMPv3 engine ID discovery (the engine ID discovery packet is sent on a different port/flow than the subsequent trap)
- Device SNMP configuration change (trap receiver IP removed, community string altered, USM user deleted)

NUANCES & GOTCHAS:
SNMP traps use the device's egress interface source IP, which may differ from the polling source IP (if polling uses a different VRF, routing context, or interface). Trap reachability and polling reachability are independent signals. SNMPv3 traps may require engine ID discovery — a two-step exchange — before traps are accepted; a firewall that allows established/related traffic may not allow the initial discovery packet if treated as a new connection. Cloud-hosted receivers may have intermittent reachability due to VPN tunnel flaps that don't affect production traffic paths.

CORRELATES WITH:
- **Per-Source Trap Presence**: Reachability loss manifests as the source going silent in the per-source heartbeat.
- **SNMP Agent Reachability (UDP/161 polling)**: If polling is also unreachable, the device is down; if only traps are affected, the trap path is broken.
- **Firewall ACL logs for UDP/162**: A drop in deny events could mean path restored; a sudden increase in denies indicates a rule change that should be investigated.
- **Device-side SNMP agent logs**: Look for "sending trap" / "trap host unreachable" entries.

---

#### Domain: Throughput — Are We Handling the Volume?

**SIGNAL: Traps Received Rate (Per Source and Aggregate)**

WHAT IT IS:
The number of SNMP trap PDUs received per unit time, at the network layer (per-source UDP/162 packet count) and at the application layer (traps logged/processed by the daemon). The gap between network-layer and application-layer counts is itself diagnostic.

SOURCE:
- Application layer: trap receiver log, parsed for trap entries per time window
- Network layer: `tcpdump -i any -n udp port 162 -c N` over a time window; or packet counter on the receiver's `iptables`/`nftables` rules for UDP/162
- Vendor NMS: statistics endpoint

HOW TO COLLECT IT MANUALLY:
```bash
# Application-layer trap count over last 5 minutes:
journalctl -u snmptrapd --since "5 minutes ago" | grep -c ' Trap '

# Network-layer packet count via tcpdump (1-minute sample):
timeout 60 tcpdump -i any -n udp port 162 -q 2>/dev/null | wc -l

# Compare: the gap = drops between network and application.
```

WHAT IT TELLS YOU:
Establishes the operational baseline. A sudden spike indicates a storm (flapping, reconvergence, environmental cascade). A sudden drop indicates path failure, receiver failure, or fleet-wide device event. A gradual increase over weeks indicates fleet growth or new trap sources.

SEVERITY:
INFO — Within baseline.
TICKET — Sustained rate > 3× the rolling 1-hour baseline (potential storm; investigate before it becomes PAGE).
PAGE — Rate exceeds known receiver processing capacity (imminent or active drops). Reasoning: every missed event during a high-volume period is potentially the critical signal we needed.

THRESHOLDS:
- (a) Ratio to rolling baseline: 3×-5× the rolling 1-hour average is a finding; 10×+ is a storm.
- (b) Absolute rate relative to known receiver capacity (benchmarked): exceeding 80% of benchmarked max is PAGE-worthy.
- (c) Rate of change: doubling within 1 minute = storm onset.
- (d) Gap between network-layer and application-layer received count: any sustained nonzero gap = drops occurring.

FAILURE MODES DETECTED:
- Trap storm onset (sudden spike)
- Network-wide event causing mass trap generation (route reconvergence, STP TCN, power restoration)
- Flapping interface generating repetitive traps
- Misconfigured device sending traps in a loop
- Silent failure (rate drop) — receiver failure, network partition, fleet-wide SNMP agent failure

NUANCES & GOTCHAS:
Baseline is highly variable: business hours vs off-hours, maintenance windows (reboots generate coldStart), seasonal traffic patterns. A "high" rate at 2 AM Sunday is different from "high" at 10 AM Tuesday. During trap storms, the *received* rate may *drop* because the kernel is dropping datagrams before the receiver reads them — a falling received rate during a known network event is a red flag for receiver overflow, not a return to normal. Some NMSs count "raw datagrams" and "parsed traps" separately; track both.

CORRELATES WITH:
- **Trap Drop Count (kernel UDP `RcvbufErrors` + application drops)**: If received rate spikes and drop count climbs, the receiver is overwhelmed.
- **Trap Processing Latency (end-to-end)**: Latency climbing during a rate spike confirms processing bottleneck.
- **Per-Source Trap Rate**: Identifies which device(s) are driving the storm.
- **Network Event Logs (syslog)**: Correlate rate spikes with syslog from the same devices to identify the root cause.

---

**SIGNAL: Per-Source Trap Rate and Top-Talker Identification**

WHAT IT IS:
The distribution of trap volume across source IPs. A single source contributing > 50% of total volume in a normal-load environment is almost always a problem (interface flapping, firmware bug, misconfigured device).

SOURCE:
- Application log parsed by source IP
- `tcpdump` parsed by source IP over a time window

HOW TO COLLECT IT MANUALLY:
```bash
# Top 20 trap sources by volume over last hour:
awk '/ Trap / {print $NF, $1, $2}' /var/log/snmptrapd.log | \
  tail -10000 | \
  awk '{src=$1; gsub(/[^0-9.]/,"",src); print src}' | \
  sort | uniq -c | sort -rn | head -20

# Per-source rate over a 1-minute sample via tcpdump:
timeout 60 tcpdump -i any -n udp port 162 -nn -q 2>/dev/null | \
  awk '{print $3}' | cut -d. -f1-4 | sort | uniq -c | sort -rn
```

WHAT IT TELLS YOU:
Identifies the "top talker" during a storm — the single source device dominating volume. The same signal detects gradual drift (a single source's trap rate creeping up over weeks, often indicating firmware behavior change or new adjacency/peer that has not stabilized).

SEVERITY:
TICKET — Any single source > 50% of total volume sustained for > 10 minutes. Investigate the device.
INFO — Used for forensics, capacity planning, and identifying sources that need trap filtering at the source.

THRESHOLDS:
- Per-source rate > 5× the source's historical 95th-percentile rate, sustained for 5 minutes.
- Per-source contribution > 30% of total fleet volume in a healthy environment.
- For mixed-fleet environments, per-device-class baselines (access switches generate different baseline rates than core).

FAILURE MODES DETECTED:
- Interface flapping on a specific device (very high `linkUp`/`linkDown` rate)
- BGP/OSPF session oscillation on a specific peer
- Device firmware bug causing repetitive traps for a transient condition
- Misconfigured trap generation (threshold settings triggering repeatedly)
- Trap-generation loop (device sending traps in response to its own events)

NUANCES & GOTCHAS:
Some devices legitimately generate high trap volumes (wireless controllers with many APs, firewalls during traffic spikes). Don't confuse high volume with a problem without a baseline. NAT can mask source IPs — multiple devices appear as one source. IPv4 and IPv6 source addresses for the same device appear as different sources if the receiver doesn't normalize them.

CORRELATES WITH:
- **Aggregate Traps Received Rate**: Identifies whether a spike is from one device (single-source dominance) or fleet-wide.
- **Per-Source Trap Type Distribution**: If 99% of a source's traps are `linkDown`, that's interface flapping; if `authenticationFailure`, that's a different problem.
- **Device Health Signals (CPU, memory, interface counters via polling)**: On the identified source, confirm whether the device is in a real fault state.

---

**SIGNAL: Inform Acknowledgment Rate (For Inform Deployments)**

WHAT IT IS:
The rate at which the receiver successfully returns Response PDUs to Inform-capable senders. For Informs (acknowledged traps), if the receiver fails to respond, the device retransmits, amplifying inbound volume and creating a self-reinforcing degradation loop.

SOURCE:
- Trap receiver log entries for Inform Response PDUs
- Application metric: `inform_acks_sent` counter
- Device-side: Inform retry counters (vendor-specific)

HOW TO COLLECT IT MANUALLY:
```bash
# Count Inform acks sent in last 5 minutes:
journalctl -u snmptrapd --since "5 minutes ago" | grep -c "Response.*Inform"

# Count Informs received in same window:
journalctl -u snmptrapd --since "5 minutes ago" | grep -c "Inform Request received"

# Retransmission indicator: unique vs total Informs by message ID
journalctl -u snmptrapd --since "1 hour ago" | grep Inform | \
  awk '{print $NF}' | sort | uniq -c | sort -rn | head -20
```

WHAT IT TELLS YOU:
Whether the receiver is keeping up with Inform processing. A low acknowledgment rate during high Inform volume indicates the receiver's Inform response path is degraded. The retransmission multiplier (total Informs / unique Informs) above 1.5 indicates significant retransmission overhead.

SEVERITY:
TICKET — Inform acknowledgment rate below received rate for > 5 minutes. Reasoning: devices will retransmit; the load is self-amplifying.
PAGE — Acknowledgment rate drops to zero while Informs are being received.

THRESHOLDS:
- Binary: zero acknowledgments while Informs arrive is critical.
- Ratio: acknowledgment / received below 0.95 sustained over 5 minutes.
- Retransmission multiplier > 1.5 indicates significant retransmission overhead.
- Per-device: any single device sending more than 10 retransmissions per minute for a single Inform is a finding.

FAILURE MODES DETECTED:
- Receiver processing delay causing Inform timeout on devices
- Inform Response PDU construction failure
- Network path issue preventing responses from reaching devices
- SNMPv3 response authentication failure
- Device-side retransmission timer misconfiguration (too aggressive for the network RTT)

NUANCES & GOTCHAS:
Inform retransmissions create duplicate trap entries in downstream systems if deduplication doesn't handle Inform message IDs correctly. Track both unique and total Inform counts. Devices with aggressive retransmission timers (1-2 seconds, many retries) can amplify a minor receiver slowdown into a significant load increase. SNMPv3 Inform responses require USM authentication, which adds per-response processing cost.

CORRELATES WITH:
- **Traps Received Rate (from Inform-capable devices)**: Spike in received rate may indicate retransmissions, not new events.
- **Trap Processing Latency**: High latency explains slow Inform acknowledgment.
- **Device-Side Inform Retry Statistics**: Direct measure of retransmission impact.

---

#### Domain: Errors — What's Going Wrong?

**SIGNAL: UDP Socket Receive Buffer Errors (Kernel-Level Drops)**

WHAT IT IS:
The rate at which the OS kernel drops incoming UDP datagrams destined for the trap receiver's socket because the socket's receive buffer is full. This is the *earliest* signal that the receiver is not draining the socket fast enough. The kernel receives the datagram, cannot queue it, and silently drops it. The device gets no notification.

SOURCE:
- `/proc/net/snmp`: `RcvbufErrors` in the Udp line
- `netstat -su`: "packet receive errors" or "RcvbufErrors" summary
- `ss -u -a -n`: Recv-Q column for port 162 (current buffer fill in bytes)

HOW TO COLLECT IT MANUALLY:
```bash
# Current RcvbufErrors count (cumulative since boot — track delta):
awk '/^Udp:/{print $5; exit}' <(awk '/^Udp:/{getline; print}' /proc/net/snmp)
# Cleaner:
netstat -su | grep -A1 "^Udp:" | grep -v "^Udp:"

# Per-socket Recv-Q for port 162:
ss -u -a -n | awk '/:162 /{print $2}'

# Direct read of /proc/net/snmp (header + values, Udp is line 2 of UDP block):
cat /proc/net/snmp | awk '/^Udp:/{getline; print "InDatagrams="$1, "NoPorts="$2, "InErrors="$3, "OutDatagrams="$4, "RcvbufErrors="$5, "SndbufErrors="$6}'
```

WHAT IT TELLS YOU:
The trap receiver process is not reading from the socket fast enough. Each `RcvbufErrors` increment is a lost trap datagram. This is the definitive signal for kernel-level drops and the most important error metric in trap infrastructure.

SEVERITY:
PAGE — Any sustained, increasing RcvbufErrors rate. Reasoning: traps are actively being lost at the kernel layer; the receiver is unaware.

THRESHOLDS:
- Rate of change: any sustained increase (more than 0 per 5-minute window) is abnormal.
- Absolute: Recv-Q for port 162 exceeding 50% of the configured `net.core.rmem_max` value indicates approaching saturation.
- The Linux default `net.core.rmem_max` is 212,992 bytes (~208 KiB) — *entirely insufficient* for any production trap volume. Production deployments must raise this. Typical recommended values are 8-32 MB; the exact value depends on the largest expected trap burst.

FAILURE MODES DETECTED:
- Trap receiver process not draining socket (CPU starvation, I/O wait, lock contention)
- UDP receive buffer (`net.core.rmem_max` and `SO_RCVBUF`) too small for trap volume
- Trap storm exceeding receiver's drain rate
- Process scheduling delay (high system load, CPU contention on shared NMS server)

NUANCES & GOTCHAS:
`RcvbufErrors` in `/proc/net/snmp` is *system-wide* (all UDP sockets). On a dedicated NMS server, most UDP traffic is SNMP traps, so this counter is a good proxy. On shared servers, cross-reference with `ss -u -a -n` for per-socket Recv-Q on port 162 specifically. The counter is cumulative since boot — always track delta per interval. Increasing `SO_RCVBUF` raises the ceiling but doesn't fix the root cause if the receiver can't drain fast enough. During a trap storm, the kernel buffer fills faster than the receiver can drain; the kernel drops the *newest* datagrams (tail drop), which means traps from the triggering event are the first to be lost.

CORRELATES WITH:
- **Traps Received Rate**: Rising received rate + rising RcvbufErrors = receiver can't keep up with volume.
- **Trap Receiver Process CPU**: Near 100% CPU + RcvbufErrors = process is saturated; low CPU + RcvbufErrors = process is blocked (I/O wait, lock contention, GC pause).
- **Trap Processing Latency**: Queue-wait time dominates latency at high fill.
- **Application-Level Drop Count** (if the receiver exposes it): Kernel drops + application drops = comprehensive loss picture.

---

**SIGNAL: Application-Level Drop Count (Queue Overflow, Filter-Rejected, etc.)**

WHAT IT IS:
The number of traps dropped at the application layer — queue overflow, filter rejection, handler exception, duplicate suppression beyond the dedup engine's intent, parse failure. Distinct from kernel-level drops.

SOURCE:
- Trap receiver log: drop messages, queue-full warnings, handler exceptions
- Application metrics: `traps_dropped`, `queue_overflows`, `parse_errors` counters
- For receivers feeding Kafka/RabbitMQ: consumer lag exceeding commit threshold

HOW TO COLLECT IT MANUALLY:
```bash
# Drop messages in receiver log:
grep -ci 'drop\|queue.*full\|overflow\|handler.*fail' /var/log/snmptrapd.log

# Parse errors specifically:
grep -ci 'parse error\|malformed\|BER.*error\|invalid PDU' /var/log/snmptrapd.log

# For Kafka-fed receivers, consumer lag:
kafka-consumer-groups.sh --bootstrap-server <broker>:9092 --describe --group trap-consumer
```

WHAT IT TELLS YOU:
Traps that arrived at the application but were not processed to completion. Different drop types indicate different root causes: queue overflow = saturation; parse error = firmware bug or attack; handler exception = bug in custom handler; filter rejection = filter misconfiguration.

SEVERITY:
PAGE — Any sustained, climbing drop count. Reasoning: the receiver is losing events; the user-impacted monitor is degraded.
TICKET — Any nonzero drop count that has stopped climbing (capacity was insufficient once; investigate root cause).
INFO — Zero drops (baseline).

THRESHOLDS:
- Binary: any nonzero value in production is abnormal and warrants investigation.
- Rate of change: more than 10 drops per minute sustained over 5 minutes is PAGE-worthy.
- Absolute scale relative to received rate: drops exceeding 1% of received traps means meaningful signal is being lost.
- Per-reason breakdown: drops by category (overflow vs. parse vs. handler) tell you which failure mode is active.

FAILURE MODES DETECTED:
- Kernel UDP receive buffer too small for incoming trap volume
- Trap receiver process not draining socket (stalled, CPU-starved, GC-paused)
- Application queue full (downstream processing bottleneck)
- Custom trap handler throwing exceptions on certain trap types (fingerprinting: which OIDs?)
- Filter rule rejecting traps (a rule added with too-broad criteria)
- Trap storm exceeding receiver capacity

NUANCES & GOTCHAS:
Some receivers use unbounded queues (no max). In this case, the queue grows until the process runs out of memory and crashes. Monitor process memory alongside any queue depth signal if the queue is unbounded. Different pipeline stages may have separate queues (receive queue, parse queue, enrichment queue, output queue); monitor each independently to identify the bottleneck stage.

CORRELATES WITH:
- **Traps Received Rate**: Spike in received rate + rising application drops = receiver overwhelmed.
- **UDP `RcvbufErrors`**: Kernel drops + application drops = comprehensive loss.
- **Trap Processing Latency**: Latency proportional to queue depth; latency climbing precedes queue overflow.
- **Trap Receiver Process CPU/Memory**: Saturation of either resource precedes queue overflow.

---

**SIGNAL: Malformed / Parse-Failed Trap Count**

WHAT IT IS:
The number of received trap datagrams that failed ASN.1/BER decoding or PDU structure validation. Traps arrived at the receiver but couldn't be parsed into valid SNMP trap structures.

SOURCE:
- Receiver log: "parse error", "BER decoding failed", "invalid PDU", "malformed"
- Application metric: `parse_failures` or `malformed_trap` counter

HOW TO COLLECT IT MANUALLY:
```bash
# Parse failures in receiver log:
journalctl -u snmptrapd --since "1 hour ago" | grep -ci 'parse error\|BER\|malformed\|decoding'
grep -ci 'parse error\|BER.*error\|malformed' /var/log/snmptrapd.log
```

WHAT IT TELLS YOU:
Parse failures indicate: (a) network corruption of trap datagrams, (b) non-SNMP traffic arriving on port 162 (port scan, misconfigured application, attack probe), (c) device SNMP agent bug generating malformed PDUs, (d) SNMP version mismatch (receiver expecting v3, receiving v1 PDU), (e) truncated PDU from fragmentation (trap exceeds path MTU).

SEVERITY:
TICKET — Sustained nonzero malformed trap rate. Some traps from some devices may be permanently lost if a specific device model has a buggy SNMP agent.
PAGE — Sudden massive spike (potential DoS, port scan, or widespread device issue).

THRESHOLDS:
- Binary: any sustained nonzero rate is abnormal.
- Rate of change: sudden spike from zero to hundreds per minute suggests attack or major misconfiguration.
- Per-source distribution: malformed traps from a single device = firmware bug; from many sources = network-wide issue or attack.
- Source IP distribution: malformed traps from unexpected source IPs may indicate port scanning or spoofing.

FAILURE MODES DETECTED:
- Device SNMP agent firmware bug generating malformed PDUs
- Network corruption (duplex mismatch, MTU mismatch, bad cable, faulty SFP)
- Port scan or DoS targeting UDP/162
- SNMP version mismatch (receiver configured for v3-only, receiving v1/v2c traps)
- MIB compilation bug causing parser to choke on specific OID patterns
- Fragmented UDP datagrams (traps exceeding path MTU) reassembled incorrectly

NUANCES & GOTCHAS:
A single buggy device model can generate malformed traps consistently — track per source IP. SNMPv3 traps that fail authentication are *not* "malformed" — they're well-formed but unauthorized; these show up in USM statistics, not parse errors. SNMPv1 and v2c trap PDU structures differ; a receiver expecting v2c may misparse v1 traps. Fragmented UDP datagrams with SNMPv3 payloads (encrypted, expanded) are more likely to exceed path MTU and fragment, producing parse failures. Some commercial NMS platforms silently discard malformed traps without logging — verify your platform logs all parse failures.

CORRELATES WITH:
- **Unknown OID Trap Count**: Both indicate "traps that arrived but weren't usable." Malformed = structural failure; unknown OID = semantic failure.
- **SNMPv3 Authentication Failure Count**: Distinguish between "couldn't parse" (malformed) and "parsed but unauthorized" (auth failure).
- **Trap Source IP Distribution**: Malformed traps from unexpected source IPs = likely port scanning or spoofing.
- **UDP Checksum Error Counters** (`/proc/net/snmp` `Udp: InErrors`): Network-level corruption indicator.

---

**SIGNAL: Unknown OID Trap Count (MIB Gap Detection)**

WHAT IT IS:
The number of traps that were successfully parsed (valid BER, valid SNMP PDU) but whose trap OID or varbind OIDs could not be resolved to a known name using the loaded MIB files. The trap is received with valid structure but logged as raw numeric OID strings.

SOURCE:
- Trap receiver log: entries with unresolved numeric OIDs instead of resolved names
- Application metric: `unknown_oid_traps` counter
- MIB resolution engine statistics: unresolved OID rate

HOW TO COLLECT IT MANUALLY:
```bash
# Heuristic for unresolved OIDs (numeric strings in the trap name field):
grep -E '\.1\.3\.6\.1\.' /var/log/snmptrapd.log | grep -v '@' | wc -l

# "Unknown" or "cannot find" in MIB resolution:
grep -c 'Unknown OID\|cannot find module\|Cannot find' /var/log/snmptrapd.log

# Direct: list unique unresolved OIDs:
grep -oE '\.1\.3\.6\.1\.[0-9.]+' /var/log/snmptrapd.log | sort -u | head -50
```

WHAT IT TELLS YOU:
MIB gaps mean devices are sending traps the receiver can structurally parse but not semantically interpret. These traps enter the pipeline but may not trigger alert rules (which match on resolved names), may not be enriched with human-readable context, and may not be categorized correctly. Critical events are received but operationally invisible.

SEVERITY:
TICKET — Any sustained nonzero unknown OID rate. Some trap types are being received but not fully interpreted. Risk of missing critical events.
PAGE — Unknown OID count from a critical device type after a firmware upgrade or new device deployment. Critical traps may be affected.
PLAN — Gradual increase in unknown OIDs over weeks indicates slow MIB drift.

THRESHOLDS:
- Binary: any sustained nonzero rate is a gap.
- Absolute count: more than 10 unique unresolved OIDs appearing per day indicates significant MIB coverage gap.
- New unknown OIDs appearing suddenly (after firmware upgrade, new device deployment) warrant immediate investigation.
- Per-source: any unresolved OID from a device type fully covered by your MIB pack is a finding.

FAILURE MODES DETECTED:
- Missing vendor MIB for new device type or firmware version
- MIB compilation failure (syntax error in MIB file, missing dependency MIB)
- MIB file not loaded into the receiver's MIB directory
- Device sending enterprise-specific traps not defined in standard MIBs
- MIB file path configuration error after NMS upgrade

NUANCES & GOTCHAS:
Unknown OIDs don't mean the trap is lost — it's logged, but as a raw OID string. The trap data exists but requires manual OID lookup to interpret. In a crisis, this manual step is often skipped. Some OIDs are from experimental or deprecated MIBs that vendors still implement; these may never resolve without the specific vendor MIB. The rate of unknown OIDs tends to spike after network device firmware upgrades, which often add new trap types. MIB dependency chains are a hidden complexity: a Cisco device may need 15+ MIB files loaded in the correct order; missing one dependency silently breaks resolution for multiple trap types.

CORRELATES WITH:
- **Traps Received Rate**: If received rate is stable but unknown OID rate spikes, new device types or firmware changes introduced unresolvable traps.
- **MIB Resolution Cache Hit Rate**: Declining hit rate precedes or accompanies rising unknown OID count.
- **Device Inventory Changes**: New device models added to the network should trigger MIB review.

---

**SIGNAL: Inform Timeout / Retransmission Count**

WHAT IT IS:
The number of Inform requests received by the trap receiver that were not acknowledged within the expected time window, causing the sending device to retransmit. Measures the receiver's responsiveness to Inform processing specifically.

SOURCE:
- Trap receiver log: Inform retransmission detection, duplicate Inform IDs
- Application metric: `inform_timeouts` counter
- Device-side SNMP statistics: Inform retry counter (if queryable)

HOW TO COLLECT IT MANUALLY:
```bash
# Inform retransmission indicators in receiver log:
grep -c 'retransmit\|duplicate Inform\|Inform timeout' /var/log/snmptrapd.log

# Duplicate Inform IDs (retransmission indicator):
journalctl -u snmptrapd --since "1 hour ago" | grep Inform | \
  awk '{print $NF}' | sort | uniq -c | sort -rn | head -20
```

WHAT IT TELLS YOU:
Inform timeouts mean the receiver is too slow to acknowledge. The device retransmits, multiplying inbound volume and creating a feedback loop. Chronic timeouts indicate the receiver needs more processing capacity, the Inform path is too slow, or the network RTT between device and receiver is too high for the device's retransmission timer.

SEVERITY:
TICKET — Sustained Inform timeout rate above 5% of Inform received rate.
PAGE — Inform timeout rate above 50% or rapidly climbing.

THRESHOLDS:
- Ratio-based: timeout / received above 0.05 sustained over 15 minutes.
- Rate of change: timeout count doubling within 5 minutes during an event suggests cascading delay.
- Per-device: any device sending more than 10 retransmissions per minute for a single Inform indicates the receiver cannot acknowledge that device.

FAILURE MODES DETECTED:
- Receiver processing overload causing acknowledgment delays
- SNMPv3 Inform response authentication/encryption overhead
- Network latency spike between device and receiver
- Device retransmission timer too aggressive for current network conditions

NUANCES & GOTCHAS:
Inform timeouts are partially a device-side configuration issue. Some devices have aggressive retransmission timers (1-2 seconds) inappropriate for WAN-delay paths to cloud-hosted receivers. Each retransmitted Inform creates a duplicate trap if not deduplicated by Inform message ID. Inform timeouts can cascade: receiver slows → timeouts increase → retransmissions increase → receiver slows more.

CORRELATES WITH:
- **Inform Acknowledgment Rate**: Timeout rate rising + acknowledgment rate falling = receiver overload.
- **Traps Received Rate (from Inform-capable devices)**: Received rate spike may be retransmissions, not new events.
- **Trap Processing Latency**: High processing latency explains slow Inform acknowledgment.

---

#### Domain: Saturation — Are We Approaching Limits?

**SIGNAL: UDP Receive Buffer Fill (Recv-Q for Port 162)**

WHAT IT IS:
The current occupancy of the kernel's UDP receive buffer for the trap receiver's socket, in bytes. A growing Recv-Q means the receiver is not draining the socket fast enough. When Recv-Q reaches the buffer size, the kernel starts dropping incoming datagrams.

SOURCE:
- `ss -u -a -n`: Recv-Q column for the port 162 socket
- `netstat -anu`: Recv-Q in the UDP socket table
- `/proc/net/udp`: rx_queue column for the line matching port 162 (0xA2)

HOW TO COLLECT IT MANUALLY:
```bash
# Per-socket Recv-Q for port 162:
ss -u -a -n | awk '/:162 /{print "Recv-Q bytes:", $2}'

# Per-socket rx_queue from /proc/net/udp:
awk '$2 ~ /:00A2$/ {print "rx_queue:", $5, "loc_addr:", $2}' /proc/net/udp
```

WHAT IT TELLS YOU:
The earliest warning that the receiver is falling behind. Rising Recv-Q means the kernel is queueing more datagrams than the receiver is processing. When Recv-Q reaches `net.core.rmem_max` (or the socket's `SO_RCVBUF`), drops begin.

SEVERITY:
TICKET — Recv-Q > 25% of `net.core.rmem_max` sustained for > 5 minutes. Processing is falling behind.
PAGE — Recv-Q sustained > 50% of `net.core.rmem_max` for > 1 minute. Drops imminent or already occurring.

THRESHOLDS:
- Recv-Q should be near zero in steady state. Any sustained nonzero value indicates the receiver is falling behind.
- Compare to buffer ceiling: > 50% utilization is the cliff edge.
- Time-to-overflow: `remaining_capacity / (current_growth_rate)` if the queue is growing.

FAILURE MODES DETECTED:
- Receiver processing bottleneck (CPU, MIB resolution, disk I/O)
- Receiver hang or deadlock
- Trap storm exceeding receiver capacity
- Socket buffer sized too small for the environment's trap volume

NUANCES & GOTCHAS:
`Recv-Q` is per-socket. Multiple receiver processes or threads bound to UDP/162 (or a distributed receiver setup) require summing Recv-Q across all relevant sockets. Increasing `rmem_max` and the socket's `SO_RCVBUF` raises the ceiling but doesn't address the root cause. During a trap storm, the kernel buffer fills faster than the receiver can drain; the kernel tail-drops the *newest* datagrams, which means traps from the triggering event are the first to be lost.

CORRELATES WITH:
- **UDP `RcvbufErrors`**: Confirmed kernel-level drops.
- **Traps Received Rate**: Rising rate + growing Recv-Q = receiver can't keep up.
- **Trap Receiver Process CPU**: Near 100% CPU = processing-bound; low CPU = I/O-bound or lock-bound.
- **Trap Processing Latency**: Latency proportional to queue depth at high fill.

---

**SIGNAL: Trap Receiver Process CPU and Memory Utilization**

WHAT IT IS:
CPU and memory consumption of the trap receiver process. CPU saturation on a single-threaded decoder is the classic "trap storm is about to drop everything" precursor. Memory growth (queue, MIB cache, handler state) is the precursor to OOM.

SOURCE:
- `/proc/<pid>/stat`: utime (field 14) and stime (field 15) in clock ticks
- `top -p <pid>`, `htop`, `pidstat -p <pid> 1`
- `ps -p <pid> -o %cpu,%mem,rss,vsz,cmd,etime`
- For containerized receivers: cgroup CPU/memory accounting

HOW TO COLLECT IT MANUALLY:
```bash
# CPU and memory for the receiver process:
ps -p $(pgrep -d, snmptrapd) -o pid,%cpu,%mem,rss,vsz,etime,cmd

# Continuous sampling:
pidstat -p $(pgrep snmptrapd) 1 5
```

WHAT IT TELLS YOU:
CPU breakdown (user vs system) distinguishes application-level work (MIB resolution, handler code — user time) from kernel overhead (socket reads, memory allocation — system time). Memory growth indicates queue buildup, MIB cache growth, or a leak in a custom handler. CPU saturation on a single core while other cores are idle is the classic single-threaded decoder pattern.

SEVERITY:
TICKET — CPU > 70% sustained. Approaching capacity limit. Memory > 80% of host.
PAGE — CPU > 90% sustained AND Recv-Q or `RcvbufErrors` is growing. Receiver is at or past capacity. Memory > 95% or rapidly climbing.

THRESHOLDS:
- Sustained CPU > 70% warrants investigation.
- Per-core thresholds matter for single-threaded receivers (e.g., `snmptrapd`): 100% on one core = saturated even on a 16-core machine.
- Memory: free memory on the host should remain > 2× the receiver's RSS. Set OOM score adjustment to -1000 for the receiver process if it is critical (and monitor memory even more aggressively).
- Growth rates: RSS growing > 10 MB/day with no fleet growth indicates a leak.

FAILURE MODES DETECTED:
- Processing capacity ceiling hit
- Inefficient MIB resolution (too many MIBs, unoptimized tree, unindexed lookups)
- Handler code doing expensive operations (DNS lookups, database queries, regex)
- Trap storm causing CPU saturation
- Memory leak in receiver or its handler extensions
- GC death spiral in JVM-based receivers (GC consumes increasing CPU as memory pressure builds)

NUANCES & GOTCHAS:
Many trap receivers (especially `snmptrapd`) are single-threaded for trap processing. 100% CPU on one core = saturated, even if the machine has 16 cores. Check *per-core* utilization, not aggregate. CPU steal time in virtualized environments can masquerade as receiver CPU saturation — check for hypervisor contention. GIL (Python) and similar threading limitations in custom handlers can prevent effective parallelism. DNS reverse lookups in handlers cause CPU to appear low (process is blocked on I/O, not CPU) but throughput is still degraded.

CORRELATES WITH:
- **UDP `RcvbufErrors`**: The pair together confirms processing saturation.
- **Trap Processing Latency**: High CPU + high latency = decoder saturating.
- **Per-Trap Decode Time**: Direct measurement of per-event processing cost.
- **MIB Resolution Cache Hit Rate**: Declining hit rate raises per-trap CPU cost.

---

**SIGNAL: Downstream Pipeline Backlog (Queue / Lag Indicators)**

WHAT IT IS:
The depth of any queue or lag between the trap receiver and the downstream consumer. For receivers feeding Kafka, this is consumer lag. For receivers feeding a database, it's the write queue. For receivers with internal queues (rare in `snmptrapd`, more common in commercial NMS), it's the in-process queue. The backlog is the runway: it tells you how long until the pipeline starts dropping.

SOURCE:
- Kafka: `kafka-consumer-groups.sh --describe --group <group>` — lag column
- RabbitMQ: `rabbitmqctl list_queues name messages` or HTTP API `GET /api/queues`
- File-based spool: count of files in the spool directory
- Receiver internal queue metric (if exposed)

HOW TO COLLECT IT MANUALLY:
```bash
# Kafka consumer lag:
kafka-consumer-groups.sh --bootstrap-server <broker>:9092 --describe --group trap-consumer

# RabbitMQ queue depth:
curl -s -u <user>:<pass> http://<host>:15672/api/queues | \
  python3 -c 'import sys,json; [print(q["name"], q["messages"]) for q in json.load(sys.stdin)]'

# File-based spool:
ls /var/spool/snmptrapd/ | wc -l
```

WHAT IT TELLS YOU:
Whether the downstream consumer can keep up. A growing backlog means the pipeline is accumulating work and will eventually overflow. Backlog depth is the most actionable saturation metric because it lets you compute time-to-overflow.

SEVERITY:
TICKET — Queue depth growing continuously for > 15 minutes. Processing falling behind.
PAGE — Queue depth > 80% of maximum capacity. Drops imminent.

THRESHOLDS:
- Queue depth should return to near zero between spikes. Sustained nonzero depth = processing falling behind.
- Compare to max capacity: > 50% sustained warrants investigation, > 80% is critical.
- Time-to-overflow: `(max - current) / (received_rate - processed_rate)` in seconds.

FAILURE MODES DETECTED:
- Processing bottleneck (receiver or downstream consumer)
- Downstream system failure (database down, message queue broker partition)
- Consumer crash (Kafka consumer group has no active consumers)
- Queue capacity exhaustion

NUANCES & GOTCHAS:
Kafka consumer lag is measured in offsets, not time. A lag of 10,000 messages could be 10 seconds or 10 hours depending on message size and processing rate. Convert to estimated time-to-drain for actionable thresholds. Some receivers have both an internal pre-processing queue and a post-processing forwarding queue — monitor each independently. Queue backpressure (queue full → receiver stops reading from UDP socket → kernel buffer fills → drops) is the cascade to watch for. `snmptrapd` itself has *no* native internal queue metric — backpressure is observed via `RcvbufErrors` and Recv-Q, not via a queue depth counter. This is a known operational gap.

CORRELATES WITH:
- **UDP `RcvbufErrors`**: Queue growing + buffer dropping = end-to-end saturation.
- **Downstream System Health** (database, message broker, SIEM): Downstream failure is the most common root cause.
- **Processing Latency**: Increasing processing time explains queue growth.
- **Consumer Group Membership** (Kafka): If consumers have left the group, lag will grow rapidly.

---

#### Domain: Latency — How Fast Are We Processing?

**SIGNAL: Trap-to-Alert End-to-End Latency**

WHAT IT IS:
The time from when a trap datagram arrives at the receiver's UDP socket to when the resulting alert, event, or action is emitted by the downstream pipeline. Measures the full processing chain — kernel receive, decoder, MIB resolution, filter, dedup, enrich, correlate, route, downstream ingest, alert rule evaluation.

SOURCE:
- Synthetic test trap with known send timestamp and unique identifier; measure time to appearance in downstream system
- Application metric: `trap_processing_seconds` (histogram or summary) if exposed
- Receiver receive timestamp vs downstream consumer ingest timestamp

HOW TO COLLECT IT MANUALLY:
```bash
# Send a tagged test trap and measure downstream arrival:
trap_id="latency-test-$(date +%s%N)"
snmptrap -v2c -c public <receiver> '' \
  NET-SNMP-EXAMPLES-MIB::netSnmpExampleHeartbeatNotification \
  netSnmpExampleHeartbeatDateAndTime.0 s "$trap_id"

# Note the send time. Then check downstream for the marker:
# In SIEM/log store: search for $trap_id
# In receiver log: grep for $trap_id and compare timestamps.

# Time the round-trip with `time` for a single-trap measurement:
time snmptrap -v2c -c public <receiver> '' SNMPv2-MIB::coldStart.0
```

WHAT IT TELLS YOU:
How quickly trap events become actionable. During normal operation, this should be sub-second to a few seconds. During storms or pipeline stress, latency increases. When latency exceeds your alerting SLA, trap-based alerting loses its primary advantage (real-time notification) over polling.

SEVERITY:
INFO — Within normal baseline (sub-second to a few seconds).
TICKET — Exceeding 10 seconds sustained for > 5 minutes. Alerts are significantly delayed.
PAGE — Exceeding 60 seconds or trending upward rapidly. Critical alerts may arrive too late for effective response.

THRESHOLDS:
- Baseline deviation: 2× normal = investigate; 5× = concern; 10×+ = critical.
- Rate of change: latency doubling within 5 minutes during a storm = cascading delay.
- Absolute: any latency > 60 seconds makes trap-based alerting slower than 1-minute polling — at that point, polling is the more reliable mechanism.

FAILURE MODES DETECTED:
- Processing pipeline bottleneck (MIB resolution, enrichment, correlation)
- Downstream system (SIEM, database) ingestion latency
- Queue wait time increasing (pipeline backlog)
- DNS resolution delay during enrichment (reverse DNS for source IP)
- Lock contention in multi-threaded processing

NUANCES & GOTCHAS:
End-to-end latency includes queue wait time, which dominates during high volume. A trap entering a queue with 1,000 traps ahead, processed at 100/second, waits 10 seconds before processing begins. Synthetic test traps may not exercise the full processing path (may bypass MIB resolution, enrichment, correlation) — use real trap types for accurate measurement. Latency measurement requires clock synchronization between components. Some trap types are processed differently (e.g., `coldStart` may trigger additional enrichment and correlation logic, adding latency compared to a standard `linkDown` trap).

CORRELATES WITH:
- **Trap Processing Queue Depth**: Latency is proportional to queue depth.
- **Traps Received Rate**: Latency increasing during rate spike confirms volume-related delay.
- **Trap Processing Worker Utilization**: High utilization + high latency = workers saturated.
- **Downstream System Response Time**: Slow downstream adds to per-trap processing time.

---

**SIGNAL: Per-Trap Decode / MIB Resolution Latency**

WHAT IT IS:
The time spent inside the receiver's decode-and-process path per trap, measured at the receiver. Decode latency that grows indicates MIB lookup overhead, filter evaluation cost, or downstream-pipeline backpressure. The per-trap cost directly bounds the maximum per-thread throughput.

SOURCE:
- Receiver internal timing (if exposed — commercial NMSs often expose this; `snmptrapd` does not natively)
- Profiling with `strace -c -e trace=read,write -p <pid>` on the receiver process (expensive, use briefly)
- End-to-end latency minus downstream latency = receiver-side latency

HOW TO COLLECT IT MANUALLY:
```bash
# Profile the receiver briefly during load:
strace -p $(pgrep snmptrapd) -c -e trace=read,write,recvfrom 2>&1 &
STRACE_PID=$!
sleep 30
kill $STRACE_PID

# For latency inference: time burst of test traps, observe log line timestamps:
for i in $(seq 1 100); do
  snmptrap -v2c -c public <receiver> '' IF-MIB::linkDown.0 ifIndex.0 i 1
done
# Compare first and last log line timestamps; divide by trap count for average per-trap latency.
```

WHAT IT TELLS YOU:
The per-trap cost directly limits maximum per-thread throughput. If resolution takes 5 ms per trap, the decoder processes 200 traps/sec per thread; a 2,000 trap/sec storm creates 10× backlog. This signal tells you how close to processing ceiling you are.

SEVERITY:
TICKET — Average per-trap latency > 10 ms. Inefficient MIB set or insufficient CPU.
PLAN — Per-trap latency trending upward (MIB set growing, new vendor MIBs less efficient).

THRESHOLDS:
- Workload-dependent: profile maximum sustainable rate. The max rate should be > 3× your average sustained rate (3× headroom). If max rate < 2× average, optimize MIBs or scale horizontally.
- MIB resolution scales unfavorably with MIB set size when not pre-indexed. Loading 500+ MIBs without pre-compilation can cause per-trap cost to rise to tens of milliseconds.

FAILURE MODES DETECTED:
- MIB tree too large / inefficient
- Receiver CPU bottleneck
- Inefficient MIB loading (loading all MIBs instead of selective)
- Handler code doing expensive operations per-trap (DNS, DB queries, regex)
- v3 trap overhead (authentication + optional encryption per trap adds measurable cost)

NUANCES & GOTCHAS:
MIB resolution cost is non-linear with MIB set size when not pre-indexed. Pre-compiling MIBs dramatically reduces resolution time. Selective MIB loading (only MIBs for devices you actually monitor) can reduce resolution time by 50-90%. SNMPv3 traps are more expensive to process than v2c due to authentication/encryption overhead on top of MIB resolution. Most open-source receivers do not expose per-event timing; commercial NMS receivers do.

CORRELATES WITH:
- **UDP `RcvbufErrors`**: Per-trap latency directly causes buffer buildup.
- **CPU Utilization**: CPU-bound resolution shows as tight correlation.
- **MIB Set Size**: File count and total size in the MIB directory.
- **MIB Resolution Cache Hit Rate**: Falling hit rate means more traps pay the full MIB lookup cost.

---

#### Domain: Resource Utilization — Host-Level

**SIGNAL: File Descriptor Usage on Trap Receiver Process**

WHAT IT IS:
The number of open file descriptors used by the trap receiver process compared to its configured limit. Each open socket, file handle, database connection, or forwarding destination consumes an FD.

SOURCE:
- `/proc/<pid>/fd`: directory entry count
- `/proc/<pid>/limits`: `Max open files` line (soft and hard)
- `lsof -p <pid> | wc -l`: same but slower

HOW TO COLLECT IT MANUALLY:
```bash
# FD count and limit:
ls -la /proc/$(pgrep snmptrapd)/fd 2>/dev/null | wc -l
cat /proc/$(pgrep snmptrapd)/limits | grep 'open files'
```

WHAT IT TELLS YOU:
At high source diversity (thousands of distinct device IPs), FD usage can approach the soft limit. When the limit is hit, new connections/files are refused — traps from new sources may be dropped, new log files cannot be opened, downstream connections to SIEM/DB fail.

SEVERITY:
TICKET — FD usage > 70% of the soft limit.
PAGE — FD usage > 90% of the soft limit.

THRESHOLDS:
- FD usage should stay < 70% of the soft limit.
- Soft limit should be set high (65,535+) for production trap receivers.
- Hard limit should be even higher to allow soft limit to be raised without restart.

FAILURE MODES DETECTED:
- FD exhaustion preventing new connections or file opens
- FD leak (open files not being closed in custom handlers)
- Insufficient limit configuration for deployment scale

NUANCES & GOTCHAS:
`snmptrapd` in some configurations opens a new socket or file handle per SNMPv3 user/context. High v3 user count = high FD count. Leaked FDs from custom handlers (Perl/Python) that don't properly close database connections or file handles accumulate over time. If the receiver uses connection pooling to a database or message queue, pool size × number of pools = baseline FD count. FDs grow with process uptime; investigate monotonic growth that doesn't plateau.

CORRELATES WITH:
- **Unique Source Count**: FD usage scales with source count for some receiver configurations.
- **Connection Pool Sizes** to downstream systems.
- **Process Uptime**: FD leaks grow with uptime.

---

**SIGNAL: Trap Store / Receiver Disk Utilization and I/O**

WHAT IT IS:
Free space on the volume where trap data is stored (log files, database, message queue persistence), and the I/O throughput/latency to that volume. Disk-full is a classic cliff; I/O saturation throttles the receiver.

SOURCE:
- `df -h` on the trap store mount
- `iostat -x 1`: per-device utilization, await, %util
- `iotop -p $(pgrep snmptrapd)`: per-process I/O

HOW TO COLLECT IT MANUALLY:
```bash
# Disk space and inodes:
df -h <trap_store_path>
df -i <trap_store_path>

# Per-device I/O:
iostat -x 1 5
# Watch: %util, await (ms), w/s, wMB/s
# %util > 80% sustained = disk is bottleneck
# await > 50ms for writes = slow storage

# Per-process I/O:
iotop -p $(pgrep snmptrapd) -b -d 1
```

WHAT IT TELLS YOU:
If the receiver writes to local disk, I/O becomes the throughput ceiling at high trap rates. High write latency causes the receiver to block on writes, which causes UDP buffer growth, which causes drops. The chain: slow disk → blocked receiver → full buffer → lost traps. Disk-full is the cliff.

SEVERITY:
TICKET — Disk `%util` > 80% sustained, or write `await` > 50 ms sustained, or utilization > 75% with > 30 days to full.
PAGE — Disk > 90% full and growing, or write `await` > 100 ms.

THRESHOLDS:
- `%util` should stay < 70% sustained. Write `await` < 20 ms for SSD, < 50 ms for HDD.
- Free space: should never drop below the volume generated in 24 hours at peak rate.
- Inode utilization: a directory with millions of small files can run out of inodes before disk space.

FAILURE MODES DETECTED:
- Disk I/O bottleneck causing trap processing backpressure
- Log rotation storm (compressing a 10 GB log file causes I/O spike)
- Database autovacuum or checkpoint competing with trap writes
- Disk full causing write failures

NUANCES & GOTCHAS:
Log rotation on high-volume trap logs can cause a massive I/O spike (compressing a large log file). Time rotations during low-activity periods or use async log shipping. If writing to a database, index maintenance on the trap table (especially on timestamp or source_ip columns) causes periodic I/O spikes. Network-attached storage (NFS, SMB) adds latency variability. SSD is essentially mandatory for any production trap receiver writing to disk. Check inodes (`df -i`) not just bytes — a directory with millions of small files can run out of inodes before disk space.

CORRELATES WITH:
- **UDP `RcvbufErrors`**: I/O bottleneck causes buffer buildup.
- **Traps Received Rate**: High rate + high disk utilization = expected saturation.
- **Log File Size and Rotation Schedule**: Correlate I/O spikes with rotation timestamps.
- **Consumer Lag** (if forwarding to message queue): Disk I/O on the queue broker affects forwarding latency.

---

**SIGNAL: NTP Clock Offset on Receiver Host**

WHAT IT IS:
Time difference between the trap receiver host and an authoritative NTP source. If the receiver's clock is wrong, every trap's receive timestamp is wrong, and cross-source event correlation breaks.

SOURCE:
- `chronyc tracking`, `ntpq -p`, `timedatectl status`

HOW TO COLLECT IT MANUALLY:
```bash
chronyc tracking
ntpq -p
timedatectl status
```

WHAT IT TELLS YOU:
The receiver clock is the master clock for all trap receive timestamps. Skew > 1 second breaks forensic value; skew > 1 minute breaks alerting. VM time can jump on live-migration. NTP loss on the receiver is a silent degradation of all downstream correlation.

SEVERITY:
TICKET — Offset > 1 second.
PAGE — Offset > 60 seconds (clock broken or NTP unreachable).

THRESHOLDS:
- Offset > 500 ms, sustained.
- The receiver must be the most reliable clock in the pipeline. If it's wrong, every trap's receive timestamp is wrong.

FAILURE MODES DETECTED:
- NTP source unreachable
- Stratum 1 source down
- Local clock drift
- VM clock drift after suspend/resume
- NTP daemon failure

NUANCES & GOTCHAS:
VM time can jump on live-migration. Hardware receivers with battery-backed RTCs have well-known drift patterns. The receiver must be NTP-synced *before* the management subnet is in a stable state — a chicken-and-egg problem during initial deploy. Treat the receiver as the most important clock in the pipeline; its skew contaminates all downstream consumer timestamps.

CORRELATES WITH:
- **Per-Source NTP Status**: A device's trap timestamp skew vs. receive time = the device's clock problem.
- **Downstream Consumer Clock Status**: All consumers must also be NTP-synced; any consumer with broken clock breaks the correlation chain.

---

#### Domain: Internal State — What's Happening Inside the Pipeline?

**SIGNAL: SNMPv3 USM Statistics (Receiver-Side Counters)**

WHAT IT IS:
The SNMP User-based Security Model (USM) statistics counters maintained by the trap receiver. These track SNMPv3 authentication and decryption failures for received traps. The six counters and their OIDs (under `SNMP-USER-BASED-SM-MIB::usmStats`):

| Counter | OID | Meaning |
|---|---|---|
| `usmStatsUnsupportedSecLevels` | `.1.3.6.1.6.3.15.1.1.1.0` | Traps with security level not supported by receiver |
| `usmStatsNotInTimeWindows` | `.1.3.6.1.6.3.15.1.1.2.0` | Traps outside the ±150-second engine time window |
| `usmStatsUnknownUserNames` | `.1.3.6.1.6.3.15.1.1.3.0` | Traps from unknown USM usernames |
| `usmStatsUnknownEngineIDs` | `.1.3.6.1.6.3.15.1.1.4.0` | Traps from unknown engine IDs |
| `usmStatsWrongDigests` | `.1.3.6.1.6.3.15.1.1.5.0` | HMAC verification failures |
| `usmStatsDecryptionErrors` | `.1.3.6.1.6.3.15.1.1.6.0` | Privacy decryption failures |

SOURCE:
- The receiver's own SNMP agent (if it exposes USM MIB)
- Trap receiver application logs at debug verbosity (e.g., `snmptrapd -D usm,traps`)

HOW TO COLLECT IT MANUALLY:
```bash
# Query USM statistics from the receiver's SNMP agent:
snmpwalk -v3 -u <admin> -l authPriv -A <pass> -X <pass> localhost \
  SNMP-USER-BASED-SM-MIB::usmStats

# From receiver log (if USM logging is enabled):
grep -c 'authentication failure\|wrong digest\|decryption error\|unknown user\|not in time' /var/log/snmptrapd.log

# For snmptrapd, enable USM debug logging:
# Stop the daemon, restart with debug:
snmptrapd -f -Le -D usm,traps
# Look for: "usm: USM processing begun...", "usm: no match on engineID", "usm: Unknown User"
```

WHAT IT TELLS YOU:
The definitive signal for SNMPv3 trap processing health. `usmStatsWrongDigests` climbing = devices are sending traps with incorrect authentication keys — credential mismatch; traps are silently dropped at the USM layer. `usmStatsNotInTimeWindows` climbing = engine time drift; replay protection rejecting traps. `usmStatsUnknownUserNames` = configuration mismatch on USM user. `usmStatsUnknownEngineIDs` = new device or rogue device.

SEVERITY:
PAGE — `usmStatsWrongDigests` or `usmStatsDecryptionErrors` climbing steadily. SNMPv3 traps are being silently rejected. Monitoring blind spot for the affected device class.
TICKET — `usmStatsNotInTimeWindows` climbing (engine time drift causing intermittent rejection). `usmStatsUnknownUserNames` or `usmStatsUnknownEngineIDs` incrementing (configuration investigation needed).

THRESHOLDS:
- Rate of change: any sustained increase in `wrongDigests` or `decryptionErrors` is abnormal.
- For `notInTimeWindows`, more than 5 per hour indicates time drift issues.
- Ratio: USM failure rate / total SNMPv3 trap received rate above 1% indicates significant credential or timing problems.
- For `unknownEngineIDs`, any persistent increment from a new source is a security event (new device) or configuration event (engine ID reset).

FAILURE MODES DETECTED:
- SNMPv3 credential rotation misalignment (device updated, receiver not, or vice versa)
- SNMPv3 privacy protocol mismatch (device using AES-256, receiver configured for AES-128)
- Engine time drift (devices with inaccurate clocks; reboots that trigger re-discovery)
- New device not configured on receiver (unknown engine ID, unknown user)
- Rogue SNMPv3 device attempting to send traps
- Credential corruption (key generation from passphrase using different algorithm)

NUANCES & GOTCHAS:
These counters are cumulative since the USM module initialized. Always track deltas. USM-rejected traps may not appear in the main trap processing log — they're rejected at the authentication layer before entering the pipeline. You must specifically monitor USM statistics to see these failures. `usmStatsNotInTimeWindows` can be triggered by legitimate device reboots (engine time resets to 0); a brief spike after a device reboot is normal; sustained increments indicate a persistent problem. Different SNMPv3 implementations may use different key localization algorithms. A device and receiver with the same passphrase but different localization algorithms will generate `usmStatsWrongDigests`. These statistics are for the receiver's USM processing — they don't tell you about USM issues on individual devices.

CORRELATES WITH:
- **Traps Received Rate (per device, SNMPv3 only)**: Rate drops from specific devices + rising USM errors = those devices' traps are being rejected.
- **SNMPv3 Engine Boots / Time**: Boots increment from device + `notInTimeWindows` increment = device rebooted, needs time re-discovery.
- **Authentication Failure Traps (from devices)**: Device-side auth failures + receiver-side USM errors = bidirectional credential mismatch.
- **SNMPv3 Credential Rotation Events**: USM errors appearing after a credential rotation deployment = incomplete credential update.

---

**SIGNAL: SNMPv3 Engine Boots and Time (per Device)**

WHAT IT IS:
The SNMPv3 engine boots counter (`snmpEngineBoots`) and engine time (`snmpEngineTime`) for each SNMPv3 device communicating with the receiver. The receiver tracks these for each known engine ID. Engine boots increment on every device reboot; engine time is timeticks since the last boot. These are part of USM replay protection.

SOURCE:
- `SNMP-FRAMEWORK-MIB::snmpEngineBoots.0` and `snmpEngineTime.0` on each device (queried via SNMP GET)
- The receiver's local engine table mapping remote engine IDs to their last-seen boots/time

HOW TO COLLECT IT MANUALLY:
```bash
# Query a device's engine boots and time:
snmpget -v3 -u <user> -l authNoPriv -A <pass> <device-ip> \
  SNMP-FRAMEWORK-MIB::snmpEngineBoots.0
snmpget -v3 -u <user> -l authNoPriv -A <pass> <device-ip> \
  SNMP-FRAMEWORK-MIB::snmpEngineTime.0

# OID reference:
# snmpEngineID    .1.3.6.1.6.3.10.2.1.1.0
# snmpEngineBoots .1.3.6.1.6.3.10.2.1.2.0
# snmpEngineTime  .1.3.6.1.6.3.10.2.1.3.0
```

WHAT IT TELLS YOU:
- A sudden `snmpEngineBoots` increment from a device indicates a reboot (even if no `coldStart` trap was received).
- Engine time divergence between the receiver's cached value and the device's actual value is the direct cause of `usmStatsNotInTimeWindows` rejections.
- A device with a *new* `snmpEngineID` (e.g., after a factory reset or replacement) appears to the receiver as an unknown engine; traps are rejected.
- Tracking boots over time provides a reboot history — a powerful signal for device stability and crash-loop detection.

SEVERITY:
INFO — Normal boots increments (scheduled reboots, maintenance).
TICKET — Unscheduled boots increment from a critical device. Device may have crashed.
PAGE — Engine time divergence detected (receiver's cached time for an engine ID doesn't match the device's actual time). Traps at risk of rejection.
PAGE — Device `snmpEngineID` changed (factory reset, device replacement). All v3 traps from this device are rejected until re-provisioned.

THRESHOLDS:
- Rate of change: any unexpected boots increment (no corresponding change ticket) warrants investigation.
- Time divergence: receiver's cached engine time more than ±150 seconds from device's actual `snmpEngineTime` indicates risk of `usmStatsNotInTimeWindows` rejection.
- Per-device: more than 2 cold starts from the same device in 24 hours indicates device instability (crash loop).
- Spatial correlation: cold starts from 2+ devices in the same rack/PDU within 5 minutes indicates shared power or environmental issue.

FAILURE MODES DETECTED:
- Unscheduled device reboot (crash, power cycle, kernel panic, watchdog reset)
- SNMPv3 engine time drift causing trap rejection
- Device factory reset (engine ID change, boots reset to 0)
- Receiver's engine table corruption causing time mismatch
- SNMPv3 engine ID collision (two devices with same engine ID — rare but catastrophic)

NUANCES & GOTCHAS:
After a device reboot, `snmpEngineBoots` increments and `snmpEngineTime` resets to 0. The receiver must re-discover the device's engine time before accepting traps. If discovery fails (network issue, rate limiting, NAT), the device enters a "trap lockout" state where all its traps are rejected. The recovery path: a manager-initiated GET request to the device triggers a Report PDU that refreshes the receiver's cache. This is the bidirectional escape hatch from the unidirectional trap delivery trap (see failure archetype). `snmpEngineBoots` can wrap on devices with very frequent reboots (max is 2,147,483,647) — practically unlimited. Some devices generate a new engine ID on factory reset; the receiver treats this as a new device, which fails USM authentication if credentials were tied to the old engine ID.

CORRELATES WITH:
- **`usmStatsNotInTimeWindows`**: Time divergence detected → rising rejection count.
- **Cold Start Traps**: Device boots increment + coldStart trap received = confirmed reboot.
- **Traps Received Rate (per device)**: Sudden drop in traps from a device + boots increment = device rebooted and may be in discovery/lockout state.

---

**SIGNAL: Trap Source IP Distribution and Cardinality**

WHAT IT IS:
The distribution of trap source IPs and the count of unique sources sending traps per time window. Tracks *which* devices are sending traps and detects anomalies in source patterns.

SOURCE:
- Trap receiver log: source IP in each trap log entry
- Application metric: `trap_source_ip` cardinality (unique count)
- Firewall logs: UDP/162 source IP distribution

HOW TO COLLECT IT MANUALLY:
```bash
# Unique trap sources in last 10000 events:
awk '/ Trap /{print $NF}' /var/log/snmptrapd.log | tail -10000 | \
  awk '{gsub(/[^0-9.]/,""); print}' | sort -u | wc -l

# Top 20 sources by volume:
awk '/ Trap /{print $NF}' /var/log/snmptrapd.log | tail -10000 | \
  awk '{gsub(/[^0-9.]/,""); print}' | sort | uniq -c | sort -rn | head -20

# Sources in receiver but NOT in inventory (potential rogues):
comm -23 <(awk '/ Trap /{print $NF}' /var/log/snmptrapd.log | \
  awk '{gsub(/[^0-9.]/,""); print}' | sort -u) \
  <(sort /var/lib/inventory/managed_ips.txt)
```

WHAT IT TELLS YOU:
- Cardinality drop: fewer unique sources than expected means some devices stopped sending traps (device down, trap path broken, SNMP misconfigured).
- Cardinality spike: more unique sources than expected means new devices sending traps (onboarding) or rogue/unauthorized sources (security concern).
- Volume anomaly by source: a single device suddenly dominating trap volume is likely misconfigured, flapping, or in a failure loop.
- Missing expected sources: devices in the inventory that haven't sent traps in their expected interval.

SEVERITY:
TICKET — Cardinality drop of more than 10% from baseline (missing devices). New unknown source IPs appearing (potential unauthorized device).
PAGE — Cardinality drop of more than 30% (widespread trap delivery failure) or known-critical device missing from sources.

THRESHOLDS:
- Baseline deviation: source count below 90% of rolling 24-hour baseline is abnormal.
- Per-device: any device not sending traps for more than 3× its expected interval.
- Unknown sources: any source IP not in the device inventory is suspicious.

FAILURE MODES DETECTED:
- Device SNMP agent failure (device present but not generating traps)
- Network partition affecting specific subnets (devices in affected subnet absent from sources)
- Trap configuration drift (device reconfigured, trap receiver removed)
- Rogue device or unauthorized SNMP agent
- DHCP address reuse (new device sending traps from a known IP — different device, same address)

NUANCES & GOTCHAS:
Not all devices send traps regularly. Some devices only send traps on events (link changes, peer changes) and may be silent for hours during normal operation. "Missing" sources must be cross-referenced with device type and expected behavior. Source IPs may change due to DHCP, interface changes, or VRF reconfiguration. Source IP alone is not a stable device identifier — combine with engine ID (SNMPv3) or device sysName from trap varbinds. NAT devices may rewrite source IPs, causing many devices to appear as a single source IP. Track engine IDs for unique device identification through NAT.

CORRELATES WITH:
- **Traps Received Rate**: Rate drop + cardinality drop = widespread issue. Rate drop + stable cardinality = fewer traps per device (device-side change).
- **SNMP Agent Reachability** (polling): Device missing from trap sources + unreachable by polling = device down. Missing from traps + reachable by polling = trap configuration or path issue.
- **Network Topology**: Cardinality drop affecting devices in the same subnet/rack/VLAN = localized network issue.

---

**SIGNAL: MIB Resolution Cache Hit Rate**

WHAT IT IS:
The percentage of trap OID lookups that are resolved from a fast-path cache vs. requiring a full MIB tree traversal or compilation. Measures MIB resolution efficiency. Many receivers do not expose this directly; infer from processing latency trends.

SOURCE:
- Trap receiver application statistics (if exposed)
- Per-trap processing latency inference (rising latency with stable volume = cache misses increasing)

HOW TO COLLECT IT MANUALLY:
```bash
# If the receiver exposes cache metrics via HTTP:
curl -s http://<receiver>:<port>/metrics | grep -i 'mib.*cache\|cache.*hit'

# Otherwise, infer from latency trends:
# Compare per-trap processing latency under stable volume conditions.
# Rising latency + stable volume = cache degradation.
```

WHAT IT TELLS YOU:
High hit rate (> 95%) means MIB resolution is efficient. Declining hit rate means the cache is too small for the variety of trap OIDs being received, or the cache is being invalidated too frequently (MIB reloads, memory pressure causing eviction). Miss rate spikes increase per-trap processing cost, which reduces pipeline throughput.

SEVERITY:
TICKET — Hit rate below 90% sustained. MIB resolution frequently falling back to slow path.
INFO — Normal operation baseline tracking.

THRESHOLDS:
- Hit rate below 0.90 sustained is a performance concern.
- Below 0.50 means the cache is essentially ineffective.
- Sudden drop of > 10 percentage points in 1 hour indicates cache invalidation or eviction problem.

FAILURE MODES DETECTED:
- MIB cache too small for the number of unique OIDs in the environment
- Cache invalidation due to MIB file reload or compilation
- Memory pressure causing cache eviction
- New device types adding many new OIDs that aren't cached
- Cache warming failure after receiver restart (all traps miss until cache refills)

NUANCES & GOTCHAS:
After a trap receiver restart, the MIB cache is cold (0% hit rate) and ramps up as traps arrive. A cold-start period of elevated latency is normal. Cache size tuning requires knowing the number of unique OIDs in your environment. Count unique trap OIDs received over a representative period (e.g., 1 week) to size the cache. Some MIB resolution engines don't have configurable cache sizes or don't expose cache statistics. `snmptrapd` does not expose cache hit rate; MIB lookup cost is observable indirectly via per-trap latency.

CORRELATES WITH:
- **Unknown OID Trap Count**: Rising unknown OIDs + falling cache hit rate = new OIDs entering the pipeline that aren't in the MIBs at all (cache miss because they don't exist, not because they're uncached).
- **Trap-to-Alert Processing Latency**: Rising latency + falling cache hit rate = MIB resolution is the bottleneck.
- **Trap Receiver Process CPU**: Rising CPU + falling cache hit rate = CPU spent on MIB tree traversal.

---

**SIGNAL: Trap Deduplication and Throttling Suppression Rate**

WHAT IT IS:
The number of traps that were suppressed by the deduplication or throttling engine. Traps that were received and parsed but intentionally not forwarded to alerting because they were duplicates or exceeded rate thresholds.

SOURCE:
- Trap receiver application statistics: `traps_deduplicated`, `traps_throttled`
- Deduplication engine statistics: suppressed count, suppression reason distribution
- Application log: dedup/throttle entries

HOW TO COLLECT IT MANUALLY:
```bash
# Count suppressed traps in log:
grep -ci 'suppressed\|deduplicated\|throttled\|rate-limited' /var/log/snmptrapd.log

# Break down by reason:
grep 'suppressed' /var/log/snmptrapd.log | grep -oP 'reason="\K[^"]+' | sort | uniq -c | sort -rn
```

WHAT IT TELLS YOU:
How much of your received trap volume is noise that was intentionally suppressed. A high dedup rate is expected during network events (dedup doing its job). A high throttling rate may mean you're losing legitimate signal — the throttle is masking events that should be alerted. A very low dedup rate during a known event may indicate the dedup engine is not configured.

SEVERITY:
INFO — Normal dedup rate (baseline tracking).
TICKET — Throttle rate exceeding 50% of received traps for > 10 minutes. May be suppressing legitimate alerts.
PAGE — Throttle rate exceeding 90% of received traps. Almost all traps are being suppressed; legitimate alerts likely lost.

THRESHOLDS:
- Throttled / total > 0.5 sustained is concerning (signal loss risk).
- Dedup / total > 0.9 during a known event is expected; > 0.9 during normal conditions suggests a flapping device.
- Throttle vs. dedup ratio: if throttling (rate-limiting) exceeds deduplication (content-based suppression), the pipeline is volume-limited rather than filtering duplicates.

FAILURE MODES DETECTED:
- Flapping interface generating excessive duplicates (dedup rate high)
- Trap storm exceeding throttle thresholds (throttle rate high, legitimate events suppressed)
- Dedup window too long (suppressing separate legitimate events within the window)
- Throttle threshold too aggressive for the environment
- Dedup engine failure (sudden drop to zero dedup = engine broken, all traps pass through)

NUANCES & GOTCHAS:
Deduplication and throttling are essential for operational sanity but create a trade-off: aggressive suppression reduces noise but risks losing signal during real incidents. The dedup "window" (time period within which duplicate traps are suppressed) is a critical configuration parameter. Too short = duplicates pass through. Too long = legitimate repeated events suppressed. Throttling is typically per-source or per-trap-type. Dedup statistics should be tracked by trap type and source — a high dedup rate for `linkDown` traps from a specific switch port indicates a flapping interface that needs physical repair.

CORRELATES WITH:
- **Traps Received Rate**: High received rate + high dedup rate = storm or flap. High received rate + low dedup rate = genuine high event volume.
- **Link Up/Down Trap Rate**: High link trap rate + high dedup = flapping interfaces.
- **Traps Processed Rate**: If processed rate is lower than expected, check if dedup/throttle is the reason.
- **Alerting Volume**: Should be approximately (received − dedup − throttled − drops). Verify this equation holds.

---

#### Domain: Network Event Detection — What Are Traps Telling Us About the Network?

**SIGNAL: Cold Start / Warm Start Trap Rate**

WHAT IT IS:
The rate of `coldStart` (`1.3.6.1.6.3.1.1.5.1`) and `warmStart` (`1.3.6.1.6.3.1.1.5.2`) traps received. `coldStart` indicates a device has rebooted from a powered-off state; `warmStart` indicates a soft reboot (SNMP agent restart without device reboot).

SOURCE:
- Trap receiver log: filter for coldStart and warmStart trap OID entries
- Trap receiver statistics: trap_type count by OID

HOW TO COLLECT IT MANUALLY:
```bash
# Cold starts in last hour:
grep -c 'coldStart\|\.1\.3\.6\.1\.6\.3\.1\.1\.5\.1' /var/log/snmptrapd.log

# Warm starts in last hour:
grep -c 'warmStart\|\.1\.3\.6\.1\.6\.3\.1\.1\.5\.2' /var/log/snmptrapd.log

# Identify which devices cold-started:
grep 'coldStart' /var/log/snmptrapd.log | tail -20
```

WHAT IT TELLS YOU:
Unscheduled cold starts from production devices are critical events — devices are rebooting unexpectedly (crashes, power issues, software bugs). Clusters of cold starts from devices in the same rack/PDU indicate power or environmental issues. A single device repeatedly cold-starting indicates an unstable device (boot loop, firmware bug, hardware failure). Warm starts indicate SNMP agent restarts, possibly from config changes or agent crashes.

SEVERITY:
PAGE — Any cold start from a critical production device without a corresponding change ticket. Any cluster of cold starts from multiple devices in the same physical location.
TICKET — Warm start from a critical device. Repeated cold starts from the same device over 24 hours.

THRESHOLDS:
- Binary per device: any unscheduled cold start is abnormal.
- More than 2 cold starts from the same device in 24 hours indicates device instability.
- Spatial correlation: cold starts from 2+ devices in the same rack/PDU within 5 minutes indicates shared power or environmental issue.

FAILURE MODES DETECTED:
- Unscheduled device reboot (crash, kernel panic, watchdog reset)
- Power supply failure (device reboots when secondary supply takes over)
- Firmware bug causing boot loops
- PDU or circuit failure affecting multiple devices
- SNMP agent crash and restart (warm start)
- Scheduled maintenance not communicated to the NOC

NUANCES & GOTCHAS:
After maintenance windows, cold start storms are *normal* — do not page on cold starts during and immediately after scheduled maintenance. Some devices send coldStart traps even on graceful administrative reboots; others send warmStart for soft reboots and coldStart for power cycles. Know your device behaviors. Cold start traps may not arrive if the device loses network connectivity during boot (the trap is generated before interfaces are fully up on some devices). Absence of cold start doesn't prove the device didn't reboot. The combination of coldStart trap + SNMPv3 engine boots increment provides redundant reboot detection — if the coldStart is lost, the boots increment is still observable.

CORRELATES WITH:
- **SNMPv3 Engine Boots Counter**: Boots increment confirms reboot happened.
- **Traps Received Rate (per device)**: Device sends coldStart then no further traps = boot failure or initialization hang.
- **`sysUpTime` (via polling)**: Low `sysUpTime` after a cold start confirms recent boot.
- **Power/Environmental Alerts**: Cold start cluster + PDU/power alert = power failure. Cold start cluster + temperature alert = thermal shutdown.
- **Device Inventory Location**: Spatial correlation of cold starts identifies shared power/environmental issues.

---

**SIGNAL: Link Up / Link Down Trap Rate (Flapping Detection)**

WHAT IT IS:
The rate of `linkDown` (`1.3.6.1.6.3.1.1.5.3`) and `linkUp` (`1.3.6.1.6.3.1.1.5.4`) traps. A high rate from a single interface indicates link flapping.

SOURCE:
- Trap receiver log: filter for linkDown/linkUp trap OID entries
- Trap receiver statistics: trap_type count by OID, per-source and per-interface breakdown
- IF-MIB varbinds in traps: `ifIndex`, `ifAdminStatus`, `ifOperStatus`

HOW TO COLLECT IT MANUALLY:
```bash
# Count link up/down traps in last hour:
grep -c 'linkDown\|linkUp\|\.1\.3\.6\.1\.6\.3\.1\.1\.5\.[34]' /var/log/snmptrapd.log

# Identify flapping interfaces (per source + ifIndex):
grep 'linkDown\|linkUp' /var/log/snmptrapd.log | tail -1000 | \
  grep -oE '(source_ip|ifIndex)=[^ ]+' | sort | uniq -c | sort -rn | head -20
```

WHAT IT TELLS YOU:
- Individual interface flapping: bad cable, failing SFP, duplex mismatch, speed negotiation failure.
- Device-wide flapping: many interfaces on one device flapping simultaneously = line card failure, power supply issue, software bug.
- Network-wide flapping: many devices simultaneously = spanning-tree event, routing protocol reconvergence, power issue.
- No link traps from a device that normally sends them: trap generation may be disabled, or the device is completely down.

SEVERITY:
PAGE — Link down on a critical interface (backbone, uplink, peer link, server-facing) that doesn't recover within the expected interval. Network-wide flap storm.
TICKET — Single interface flapping (> 3 transitions in 5 minutes). Multiple interfaces on same device flapping.

THRESHOLDS:
- Per-interface: > 3 up/down transitions in 5 minutes is flapping.
- Per-device: > 10 link traps in 1 minute from a single device is abnormal.
- Network-wide: link trap rate exceeding 5× baseline indicates a significant event.
- Recovery time: link down on a critical interface followed by link up within 30 seconds is transient; down for > 2 minutes without recovery is sustained.

FAILURE MODES DETECTED:
- Failing cable or optic (single interface flap)
- Duplex or speed mismatch (single interface flap)
- Failing line card or port ASIC (multiple interfaces on same card)
- Spanning-tree topology change (cascading interface transitions)
- Power supply brownout causing line card resets
- Environmental issue (overheating causing interface shutdowns)
- Software bug causing interface state machine oscillation

NUANCES & GOTCHAS:
Link traps are the highest-volume trap type in most networks. During normal operation they dominate the trap count. Deduplication and throttling are essential for link traps. Not all linkDown events are problems: interfaces on user access ports go up and down normally. Only unusual rates or critical interfaces warrant alerting. Some devices suppress their own link traps during boot (interfaces come up sequentially, generating traps that are suppressed by the device's own trap throttle). `ifOperStatus` in the trap varbinds distinguishes "admin down" (intentional) from "oper down" (failure). Alert only on oper-down transitions for critical interfaces. During maintenance windows, linkDown/linkUp traps are expected as interfaces are administratively shut and brought back up.

CORRELATES WITH:
- **Traps Received Rate**: Link traps typically drive the overall rate.
- **Traps Deduplication Rate**: High dedup during a link flap = dedup engine working correctly.
- **BGP/OSPF Peer State Traps**: Link down on a backbone interface often followed by BGP/OSPF peer down traps — cascading failure.
- **`sysUpTime` (device)**: Link flaps + recent reboot = boot-time interface initialization (expected). Link flaps + stable uptime = real physical issue.
- **Interface Error Counters (polling)**: Interface with high CRC errors or input errors + flapping = physical cable/optic issue.

---

**SIGNAL: BGP Peer State Change Traps**

WHAT IT IS:
Traps generated when BGP peering sessions change state. Critical for detecting routing instability, peer failures, and potential route hijacking. The relevant OIDs:

- `bgpEstablishedNotification` (RFC 4273, current): `.1.3.6.1.2.1.15.0.1`
- `bgpBackwardTransNotification` (RFC 4273, current): `.1.3.6.1.2.1.15.0.2`
- Legacy `bgpEstablished` (deprecated): `.1.3.6.1.2.1.15.7.1`
- Legacy `bgpBackwardTransition` (deprecated): `.1.3.6.1.2.1.15.7.2`

Trap varbinds include `bgpPeerRemoteAddr`, `bgpPeerLastError`, `bgpPeerState`.

SOURCE:
- Trap receiver log: filter for BGP trap OID entries
- `BGP4-MIB` and vendor-specific BGP MIBs

HOW TO COLLECT IT MANUALLY:
```bash
# BGP peer change traps:
grep -c 'bgpEstablished\|bgpBackwardTrans\|\.1\.3\.6\.1\.2\.1\.15\.0\.[12]' /var/log/snmptrapd.log

# Details of recent peer changes:
grep 'bgpBackwardTrans' /var/log/snmptrapd.log | tail -20
```

WHAT IT TELLS YOU:
- BGP peer down on a backbone/transit/peering link: critical event — potential loss of connectivity to significant portion of the internet or internal network.
- OSPF neighbor state change on a core link: potential routing instability affecting traffic engineering.
- BGP peer flapping: route instability that can cause route processor CPU load, route table churn, and traffic blackholing.
- Unexpected BGP peer establishment: potential unauthorized peering or route hijacking attempt.
- A `bgpBackwardTransition` followed by **no** `bgpEstablished` = a session that has gone down and not recovered. This is the "backward with no return" signature of a possible BGP hijack (the legitimate peer's session is being squeezed out) or a peer in a sustained failure state.

SEVERITY:
PAGE — BGP session down on a backbone, transit, or critical peering link. OSPF adjacency loss on a core link.
TICKET — BGP session flap (recovered within threshold). OSPF neighbor state change on non-core link. New BGP peer session established (verify authorization).

THRESHOLDS:
- Binary per peer: any BGP session down on a critical peer is PAGE.
- > 3 BGP state changes for the same peer in 15 minutes is flapping.
- "Backward with no return" (backward trap, no subsequent established within 5× hold-timer) on a peer carrying non-trivial prefixes is a hijack-suspect signal.

FAILURE MODES DETECTED:
- BGP session timeout (TCP connection lost, hold timer expired)
- OSPF adjacency loss (hello timer expired, MTU mismatch, area mismatch)
- Route processor CPU exhaustion causing protocol process starvation
- Underlying link failure causing protocol session loss
- BGP route hijacking (unexpected routes from a peer — correlate with prefix monitoring)
- OSPF area partition
- Configuration change affecting protocol operation

NUANCES & GOTCHAS:
BGP session down doesn't always mean traffic loss — there may be redundant paths. Correlate with traffic flow data (NetFlow) to assess actual impact. OSPF neighbor state changes during normal DR/BDR election are expected after device boot. The `bgpBackwardTransition` trap is sent for any backward (toward Idle) state transition, not just session loss. Check the `bgpPeerState` varbind to determine the actual state. Some large-scale BGP implementations generate enormous numbers of traps during route table convergence; the trap volume itself can be a problem. MD5 authentication mismatch on a BGP session (detected via syslog — Cisco `%TCP-6-BADAUTH`, Juniper `tcp_auth_ok: ... wrong MD5 digest`) often appears in the same time window as a `bgpBackwardTransition`. This is a security-relevant signal.

CORRELATES WITH:
- **Link Up/Down Traps**: BGP/OSPF peer down preceded by link down on the same interface = underlying link failure.
- **Cold Start Traps**: BGP/OSPF peer changes + cold start from same device = device reboot causing protocol restart.
- **NetFlow / Traffic Data**: BGP peer down + traffic drop on affected prefix = confirmed traffic impact.
- **BGP MD5 syslog events**: BGP backward + MD5 mismatch = possible active attack against the session.

---

**SIGNAL: OSPF Neighbor State Change Traps**

WHAT IT IS:
OSPF neighbor adjacency change notifications from `OSPF-TRAP-MIB`:
- `ospfNbrStateChange`: `.1.3.6.1.2.1.14.16.2.2`
- `ospfIfStateChange`: `.1.3.6.1.2.1.14.16.2.1`

Varbinds include `ospfRouterId`, `ospfNbrIpAddr`, `ospfNbrRtrId`, `ospfNbrState`.

SOURCE:
- Trap receiver log: filter for OSPF trap OID entries

HOW TO COLLECT IT MANUALLY:
```bash
# OSPF neighbor state changes:
grep -c 'ospfNbrStateChange\|ospfIfStateChange\|\.1\.3\.6\.1\.2\.1\.14\.16\.2' /var/log/snmptrapd.log
```

WHAT IT TELLS YOU:
Loss of an OSPF adjacency means routing tables are recalculating, potentially causing blackholes or suboptimal paths. Correlates with the BGP trap signal for full IGP/BGP visibility.

SEVERITY:
PAGE — OSPF adjacency loss on a core link.
TICKET — OSPF neighbor state change on non-core link.

THRESHOLDS:
- Any OSPF neighbor loss on a core router is PAGE.
- Any state change on a non-core link is TICKET.

FAILURE MODES DETECTED:
- Physical link failure
- MTU mismatch
- OSPF hello/dead timer mismatch
- Duplicate router ID
- Area mismatch
- Authentication failure

NUANCES & GOTCHAS:
OSPF traps can be noisy during maintenance. Always verify if the neighbor loss is expected. OSPF neighbor state machine values: 1=down, 2=attempt, 3=init, 4=2-Way, 5=exchangeStart, 6=exchange, 7=loading, 8=full. A state regressing from 8 (full) to 1 (down) is the critical signal.

CORRELATES WITH:
- **Link State Transition**: OSPF drops + link up = logical issue (MTU, timers, auth). OSPF drops + link down = physical issue.
- **BGP Peer State Change**: OSPF loss often precedes BGP session loss in dual-stack routing topologies.

---

**SIGNAL: Environmental Threshold Traps (Temperature, Power, Fan)**

WHAT IT IS:
Traps from device environmental sensors crossing configured thresholds: temperature exceeding limits, power supply status changes, fan failures, voltage out of range. Physical-layer warnings that precede device failure. Vendor MIBs (CISCO-ENVMON-MIB, ENTITY-SENSOR-MIB, vendor-specific extensions) define these trap OIDs — exact names and OIDs vary by vendor.

SOURCE:
- Vendor-specific environmental MIBs
- `ENTITY-SENSOR-MIB` and `ENTITY-STATE-MIB` for standardized traps
- `entStateOperChange` notification (entity state change)

HOW TO COLLECT IT MANUALLY:
```bash
# Environmental traps (heuristic across vendors):
grep -i 'temperature\|power\|fan\|voltage\|sensor\|environmental' /var/log/snmptrapd.log | tail -50

# Vendor-specific (Cisco power supply example):
grep 'cefcFRUInserted\|cefcFRURemoved\|ciscoEnvMon' /var/log/snmptrapd.log
```

WHAT IT TELLS YOU:
- Temperature threshold trap: device or component is overheating. Imminent thermal shutdown possible.
- Power supply status change: redundancy may be lost.
- Fan failure: cooling degraded.
- Voltage out of range: power delivery problem that can cause device instability.
- A cluster of environmental traps from devices in the same rack/room = facility-level issue (HVAC failure, PDU overload).

SEVERITY:
PAGE — Temperature exceeding critical threshold (imminent shutdown). Power supply failure on a device without redundant supply. Fan failure on a device with no redundant fans.
TICKET — Temperature exceeding warning threshold. Power supply failure on a device with redundant supply (redundancy degraded).

THRESHOLDS:
- Binary per trap: any critical-threshold environmental trap is PAGE. Any warning-threshold trap is TICKET.
- Rate of change: temperature rising > 5°C in 10 minutes indicates rapid thermal change that may exceed threshold before next reading.
- Spatial correlation: temperature traps from multiple devices in the same rack/row = localized environmental issue.

FAILURE MODES DETECTED:
- Air conditioning failure in data center
- Hot/cold aisle breach
- Power supply hardware failure
- Fan bearing failure
- Circuit breaker trip on a PDU
- Device blocking airflow (improper mounting)
- Power grid fluctuation

NUANCES & GOTCHAS:
Environmental traps are often not generated unless thresholds are explicitly configured on devices. Many devices ship with no or very high default thresholds. Verify threshold configuration during device onboarding. Some devices don't generate environmental traps at all and require SNMP polling of sensor OIDs. Traps complement polling; they don't replace it for environmental monitoring. Temperature thresholds are device-specific. A 40°C reading may be normal for one device and critical for another. Power supply insertion/removal traps can be triggered by hot-swap maintenance — cross-reference with change management before paging. Environmental traps are often the *earliest* warning of a facility-level issue.

CORRELATES WITH:
- **Cold Start Traps**: Environmental threshold trap + subsequent cold start = device shut down due to environmental condition.
- **Link Up/Down Traps**: Environmental threshold trap followed by interface failures = device failing due to environmental stress.
- **Facility/DCIM Alerts**: Temperature trap + facility HVAC alert = confirmed facility issue.
- **Other Devices in Same Location**: Temperature traps from multiple devices in the same rack = localized environmental issue.

---

**SIGNAL: Configuration Change Traps**

WHAT IT IS:
Traps indicating that a device's configuration has been modified. Vendor-specific MIBs (CISCO-CONFIG-MAN-MIB's `ciscoConfigManEvent`, Juniper's `jnxCmCfgChg`, etc.) define these. Each `write mem`, `copy run start`, or live config change generates a trap on devices where the feature is enabled.

SOURCE:
- Vendor-specific configuration change MIBs
- `ENTITY-MIB` config-change notifications
- `SNMP-COMMUNITY-MIB` and `SNMP-USM-MIB` changes
- Change management system API (for cross-referencing authorized changes)

HOW TO COLLECT IT MANUALLY:
```bash
# Vendor-specific config change traps (heuristic):
grep -i 'config.*change\|cfgMgmt\|ciscoConfigMan\|configuration' /var/log/snmptrapd.log | tail -20

# Cross-reference with change calendar (manual process):
# For each config change trap, check for a corresponding change ticket
# in the same time window.
```

WHAT IT TELLS YOU:
Configuration changes detected via traps that don't correlate with an approved change record indicate unauthorized changes. Could be: an administrator bypassing change management, an automated script making unexpected changes, an attacker modifying device configurations, or a device auto-healing/rolling back a configuration.

SEVERITY:
TICKET — Config change trap without change ticket. Investigate the change and who made it.
PAGE — Config change trap on a critical infrastructure device (core router, firewall, DNS) without change ticket. Potential security incident.

THRESHOLDS:
- Binary: any config change trap without a corresponding change ticket within a ±30 minute window is anomalous.
- More than 2 uncorrelated config changes from the same device in 24 hours is suspicious.

FAILURE MODES DETECTED:
- Unauthorized configuration changes by administrators
- Automation errors making unapproved changes
- Attacker modifying device configurations
- Device auto-rollback or auto-healing triggering config change events
- Configuration drift between intended and actual state

NUANCES & GOTCHAS:
Not all config change traps are generated by all devices. Some devices require explicit `snmp-server enable traps config` (Cisco) or equivalent to enable. Verify on all managed devices. Some automated configuration management tools (Ansible, Salt, Puppet) make frequent small changes. These should have corresponding automation run records, not necessarily manual change tickets. Config change traps may be delayed if the device queues them. The timestamp may not exactly match the change time. Allow a window for correlation. Vendor-specific config change traps have different semantics. Some fire on any config write; others only on specific sections.

CORRELATES WITH:
- **Warm Start Traps**: Config change + warm start = config applied with SNMP agent restart.
- **Syslog from Same Device**: Config change trap + syslog entry showing who made the change (user, source IP) = attribution.
- **AAA/TACACS+/RADIUS Logs**: Config change + AAA log showing the CLI session that made the change = full attribution.
- **Change Management System**: Config change trap + matching change ticket = authorized. No matching ticket = unauthorized.

---

**SIGNAL: Authentication Failure Traps (from Devices)**

WHAT IT IS:
The `authenticationFailure` trap (`1[.]3[.]6[.]1[.]6[.]3[.]1[.]1[.]5[.]5`) sent by a device when it receives an SNMP request with an incorrect community string (v1/v2c) or failed USM authentication (v3). A device-generated security event.

SOURCE:
- Trap receiver log: entries matching the authenticationFailure trap OID
- `SNMPv2-MIB::snmpInBadCommunityNames` on the device itself (via SNMP polling)
- Device syslog (many devices also log auth failures to syslog)

HOW TO COLLECT IT MANUALLY:
```bash
# Authentication failure traps in last hour:
grep -c 'authenticationFailure\|\.1\.3\.6\.1\.6\.3\.1\.1\.5\.5' /var/log/snmptrapd.log

# Source devices reporting auth failures:
grep 'authenticationFailure' /var/log/snmptrapd.log | tail -50 | \
  awk '{print $NF}' | sort | uniq -c | sort -rn
```

WHAT IT TELLS YOU:
A device is being queried with incorrect SNMP credentials. This could be: a misconfigured monitoring tool using the wrong community string, a discovery tool scanning the network with default community strings, an active reconnaissance or brute-force attack against SNMP, or a device that was recently reconfigured but the NMS wasn't updated.

SEVERITY:
TICKET — Low, sustained rate of auth failures from specific devices (likely misconfiguration).
PAGE — Sudden spike in auth failures from many devices (likely network-wide scan or attack). Auth failures from critical infrastructure devices.

THRESHOLDS:
- > 10 auth failure traps from a single device in 1 hour is abnormal.
- Sudden spike (> 5× baseline in any 5-minute window) from multiple devices suggests scanning.
- Any auth failure on devices that shouldn't be queried by external source is PAGE-worthy.

FAILURE MODES DETECTED:
- SNMP reconnaissance or brute-force attack
- Misconfigured monitoring tool with wrong credentials
- SNMP community string rotation that wasn't applied to all polling sources
- Rogue SNMP scanner on the network
- Device reconfigured with new community string without updating all consumers

NUANCES & GOTCHAS:
The authenticationFailure trap is sent by the *device* to the trap receiver. The source IP in the trap is the *device's* IP, not the attacker's. The attacker's IP may be in the trap's variable bindings on some implementations, but often it's not included. SNMPv1/v2c authenticationFailure traps are triggered by wrong community strings. SNMPv3 generates more specific USM error counters (`usmStatsWrongDigests`, `usmStatsUnknownUserNames`) rather than this generic trap. Many devices have this trap disabled by default because it can be noisy during credential rotation. Check device configuration. This trap can be used as a DoS vector: an attacker sending SNMP requests with wrong community strings to many devices causes them to all send authFailure traps to the receiver, creating a trap storm.

CORRELATES WITH:
- **SNMPv3 USM Statistics** (receiver-side): Device-reported auth failures + receiver-reported USM errors = credential mismatch.
- **Traps Received Rate**: Spike in auth failures contributes to overall rate.
- **Security/SIEM Events**: Auth failure traps + network IDS alerts for SNMP scanning = confirmed reconnaissance.
- **Device Configuration Audit**: Auth failures from devices after a config change deployment = incomplete credential update.

---

#### Domain: Security & Integrity Signals (Availability-Related)

This domain covers signals that detect abuse, unauthorized access, data integrity violations, and operational security events. Some signals overlap with the Authentication Failure Traps signal above and with §5; they are listed here for completeness as availability-relevant security signals.

**SIGNAL: SNMPv1/v2c Cleartext Trap Detection (Security Baseline Drift)**

WHAT IT IS:
Detection of SNMPv1 or v2c traps arriving at the receiver, which use cleartext community strings for authentication. In a security-mature environment, all SNMP should be v3 (authPriv). Presence of v1/v2c traps indicates legacy devices that haven't been migrated or configuration drift.

SOURCE:
- Trap receiver log: version indicator in received trap entries
- `tcpdump -i <interface> udp port 162 -A`: community strings visible in cleartext in the UDP payload

HOW TO COLLECT IT MANUALLY:
```bash
# Count traps by version:
grep -c 'version 1\|Version: 1\|v1' /var/log/snmptrapd.log
grep -c 'version 2c\|Version: 2c' /var/log/snmptrapd.log
grep -c 'version 3\|Version: 3' /var/log/snmptrapd.log

# Sniff cleartext community strings:
tcpdump -i eth0 -c 100 udp port 162 -A 2>/dev/null | grep -E 'public|private'
```

WHAT IT TELLS YOU:
v1/v2c community strings and trap payloads are cleartext on the wire. An attacker with management VLAN access can sniff community strings and use them to query or reconfigure devices. Presence of default community strings (`public`/`private`) is a known-vulnerable configuration that has been actively exploited in the wild (most recently: CVE-2025-20352 "Operation Zero Disco" campaign using default `public` strings on Cisco switches to achieve RCE and deploy Linux rootkits).

SEVERITY:
TICKET — Any v1/v2c traps from production devices. Plan migration to SNMPv3.
PAGE — v2c traps using the default "public" or "private" community strings on devices accessible from untrusted networks. Active security vulnerability.

THRESHOLDS:
- Binary: any v1/v2c traps from production devices is a security gap.
- Ratio: v1v2c_count / total_traps > 0 (any percentage) is a finding.
- Default community strings ("public", "private"): any occurrence is PAGE-worthy.

FAILURE MODES DETECTED:
- Legacy devices not migrated to SNMPv3
- Configuration drift (device reverted to v2c)
- New device deployed with default SNMPv2c configuration
- Network segmentation failure (v2c devices accessible from untrusted networks)
- Community string reuse across devices (compromise of one device exposes all)

NUANCES & GOTCHAS:
Many devices still ship with SNMPv2c enabled by default. New device onboarding procedures must explicitly configure SNMPv3. "public" is often read-only, which limits but doesn't eliminate exposure. "private" or custom read-write community strings are higher severity. Even read-only community strings reveal network topology, interface information, and device configuration. SNMPv3 migration is complex at scale. Prioritize critical devices and devices on untrusted segments. Some monitoring tools only support SNMPv2c. Tool upgrades may be required.

CORRELATES WITH:
- **Authentication Failure Traps**: v2c devices + auth failure traps = someone is scanning with wrong community strings.
- **Network Segmentation Audit**: v2c devices on untrusted segments = critical vulnerability.
- **Device Inventory/Age**: Older devices more likely to be v2c. Plan lifecycle replacement.
- **Security Scanner Results**: Vulnerability scanners flag SNMPv2c community strings.

---

### SECTION 2 — Composite Failure Patterns

These are multi-signal failure modes — the patterns an operator recognizes at 3 AM. Each is a constellation of signals that, together, identify a specific failure mode. Ordered by frequency × impact.

---

**PATTERN: Trap Storm Cascade**

SIGNALS INVOLVED:
- **Traps Received Rate**: Sudden spike to 10×-100× baseline.
- **UDP `RcvbufErrors`**: Rapidly climbing.
- **Trap Processing Latency**: Spiking from sub-second baseline to minutes.
- **Trap Drop Count (kernel + application)**: Climbing after queue overflow.
- **Per-Source Trap Rate**: Often one device dominating, or fleet-wide storm.
- **Trap Receiver CPU**: Near 100% on the decoder core.

NARRATIVE:
A significant network event (power outage, STP reconvergence, routing protocol collapse, environmental cascade) causes many devices to generate traps simultaneously. The receiver's kernel buffer fills rapidly (`RcvbufErrors` climbing). The decoder saturates (CPU pegged). The processing pipeline backs up, latency explodes, and the kernel tail-drops the *newest* datagrams — which are the traps from the triggering event. The NMS is effectively blind to the very event it needs to monitor, at the worst possible time. Empirically, this is the pattern that produced > 10,000 alerts in minutes at Alibaba Cloud, prolonging recovery by hours because the actual congestion signal was buried under the cascade.

SEVERITY: PAGE — Active monitoring blackout during a production incident.

DISTINGUISHING FEATURES:
- Multiple signals move simultaneously (rate, drops, latency, queue, CPU), unlike a single-device issue which shows rate increase but not necessarily receiver-level saturation.
- The event that triggered the storm is usually visible in the earliest-received traps (before the storm overwhelms the receiver).
- Distinguish from a single-source flap: a single flapping interface generates high volume from one source with trap type dominated by `linkUp`/`linkDown`; a fleet-wide storm generates diverse trap types from many sources.

COMMON CAUSES:
- Spanning-tree topology change causing port state transitions on hundreds of ports
- Power outage affecting a rack, row, or datacenter
- Routing protocol (BGP/OSPF) reconvergence across many peers
- Misconfigured device generating traps in a loop
- Network-wide ACL or config change triggering device alerts
- Environmental cascade (HVAC loss affecting a room of devices)

FIRST RESPONSE:
1. Check `netstat -su` or `/proc/net/snmp` for `RcvbufErrors` delta to confirm kernel-level drops.
2. Identify the triggering event in the first traps received before the storm hit. Check the earliest log entries in the burst — they typically contain the root-cause trap.
3. Identify the dominant source: `awk '/ Trap / {print $NF}' /var/log/snmptrapd.log | sort | uniq -c | sort -rn | head`. If one source dominates, the storm may be device-local.
4. Temporarily increase capacity: `sysctl -w net.core.rmem_max=<larger-value>` (requires application restart for existing sockets to take advantage). For some receivers, live tuning via `dmctl` or equivalent may be possible.
5. Filter noise: if the storm is dominated by a specific trap type (e.g., `linkDown`), temporarily de-prioritize that trap type in processing to preserve capacity for other event types.
6. Do not restart the receiver: restarting loses all queued traps and creates additional processing delay.

---

**PATTERN: Silent Network Partition (Trap Black Hole)**

SIGNALS INVOLVED:
- **Trap Source IP Cardinality**: Drops — devices in the affected partition stop appearing.
- **Traps Received Rate**: Drops from baseline (but not to zero if other devices are unaffected).
- **SNMP Agent Reachability (polling)**: Devices in the affected partition are also unreachable by polling.
- **ICMP Reachability**: Devices in the affected partition are also ping-unreachable.
- **Synthetic Trap Heartbeat**: Synthetic traps from the affected subnet may or may not arrive depending on the partition's scope.
- **BGP/OSPF Peer State Traps**: Devices outside the partition may report BGP/OSPF peer loss with devices inside the partition.

NARRATIVE:
A network partition (link failure, routing issue, firewall change, VLAN misconfiguration) prevents traps from a subset of devices from reaching the receiver. The partition is "silent" because there is no positive signal — the receiver simply stops receiving from affected devices. The trap rate drops proportionally but may not go to zero (devices in other partitions continue sending). Operators may not notice if the affected devices don't trap frequently or if the rate drop is gradual.

SEVERITY: PAGE — If the partition affects critical infrastructure. Partial monitoring blindness.

DISTINGUISHING FEATURES:
- Unlike a trap storm (too many traps), this shows too few traps from specific sources.
- Unlike a receiver failure (all traps stop), this shows continued traps from unaffected sources.
- Unlike a device reboot (cold start trap received), this shows no cold start — devices are running but isolated.
- Polling and ICMP to affected devices also fail (distinguishing from SNMP-only configuration issue).
- The cardinality drop correlates with a specific subnet, VLAN, site, or upstream link.

COMMON CAUSES:
- Fiber or cable cut affecting a site or datacenter uplink
- Routing protocol failure causing management VLAN reachability loss
- Firewall rule change blocking management traffic to/from a subnet
- MPLS or VPN tunnel failure isolating a remote site
- Management VLAN trunk misconfiguration after maintenance

FIRST RESPONSE:
1. Identify the scope: list devices not sending traps and compare with device inventory. Determine if they share a subnet, VLAN, site, or upstream link.
2. Verify network path: `traceroute` to management IP of an affected device from the NMS. Check intermediate hops.
3. Check upstream devices: if the partition is behind a specific router or switch, check that device's status (it may be the failure point).
4. Check firewall/ACL logs: look for recent rule changes or deny-log entries affecting management traffic to the affected subnet.
5. Verify out-of-band access: try to reach affected devices via console server, DRAC/iLO, or other out-of-band paths.

---

**PATTERN: SNMPv3 Credential Rotation Outage**

SIGNALS INVOLVED:
- **`usmStatsWrongDigests`**: Rapidly climbing after a credential rotation event.
- **`usmStatsDecryptionErrors`**: Climbing if privacy keys also rotated incorrectly.
- **Traps Received Rate (SNMPv3 devices)**: Drops to near-zero from affected devices.
- **Trap Source IP Cardinality**: Drops as affected devices stop contributing.
- **SNMP Agent Reachability (polling)**: If polling also uses SNMPv3 with same credentials, polling fails. If polling uses different credentials, polling works but traps don't.

NARRATIVE:
A credential rotation was applied to devices but not to the receiver (or vice versa), or credentials were applied inconsistently (some devices updated, some not; auth key updated but not privacy key). The receiver receives traps from affected devices but USM authentication fails. Traps are rejected at the USM layer — they never enter the processing pipeline. The affected devices appear to have "gone silent" from a trap perspective, but they're actually sending traps that are being silently discarded. This is the most insidious failure mode because all standard health checks appear green; only USM statistic monitoring catches it.

SEVERITY: PAGE — Monitoring blackout for affected devices.

DISTINGUISHING FEATURES:
- Unlike a network partition, polling may still work (if polling uses different credentials or SNMP version).
- Unlike a receiver failure, traps from unaffected devices continue arriving normally.
- The `usmStatsWrongDigests` counter on the receiver is the definitive signal.
- The pattern begins immediately after a credential rotation deployment.

COMMON CAUSES:
- Credential rotation script applied to devices but not to NMS configuration
- Credential rotation applied to NMS but not to all devices (partial deployment)
- AuthPassphrase changed but PrivPassphrase not changed (or vice versa)
- Key localization algorithm mismatch between device and receiver
- Credential update applied to some devices in a device group but not others

FIRST RESPONSE:
1. Confirm USM rejection: check `usmStatsWrongDigests` delta. If climbing, credential mismatch is confirmed.
2. Identify affected devices: cross-reference devices not sending traps with the credential rotation deployment scope.
3. Verify credential consistency: compare the USM user configuration on the receiver with a known-affected device's SNMPv3 configuration.
4. Roll back or roll forward: either revert device credentials to the old values (restore trap flow) or apply the correct new credentials to the receiver. Choose based on which is faster and safer.
5. Verify recovery: after credential fix, confirm `usmStatsWrongDigests` stops incrementing and traps resume from affected devices.

---

**PATTERN: Flapping Interface Flood**

SIGNALS INVOLVED:
- **Link Up/Down Trap Rate**: Extremely high rate from a single interface or device.
- **Traps Received Rate**: Elevated, dominated by link traps.
- **Traps Deduplication Rate**: High — dedup engine suppressing most link traps.
- **Per-Source Trap Rate**: One source dominating.
- **Interface Error Counters (via polling)**: CRC errors, input errors, carrier transitions climbing — confirms physical-layer issue.

NARRATIVE:
A single interface (or a few interfaces) oscillates between up and down rapidly. Each transition generates a `linkUp` or `linkDown` trap. The deduplication engine suppresses most, but the sheer volume consumes processing resources. If the dedup window is exceeded, duplicates leak through to alerting, causing alert fatigue. Other, genuinely important traps from the same device or other devices are delayed or lost in the noise.

SEVERITY: TICKET for general flap. PAGE if the flapping is on a critical backbone interface.

DISTINGUISHING FEATURES:
- Volume dominated by `linkUp`/`linkDown` trap OIDs from a single source device.
- Volume is relatively steady (not bursty like a storm) — the interface flaps at a consistent rate.
- Dedup rate is very high — most traps are suppressed, but the receiver is still processing them.
- Interface error counters (via polling) confirm physical-layer issues.

COMMON CAUSES:
- Failing cable (copper or fiber)
- Failing SFP/optic module
- Duplex mismatch between device and connected equipment
- Speed negotiation failure
- Dirty fiber connector
- Electromagnetic interference (especially on copper cables near power lines)
- Software bug in interface driver

FIRST RESPONSE:
1. Identify the flapping interface: from trap logs, extract the device IP and ifIndex from the `linkDown`/`linkUp` traps.
2. If not a critical interface, administratively shut it to stop the flap and the resulting trap flood.
3. Inspect the physical layer: check cable, optic, and connector. Replace suspect components.
4. Check duplex/speed configuration: verify both ends of the link have compatible settings.
5. If the interface is critical and can't be shut: configure trap suppression for that specific interface on the device, or increase dedup aggressiveness for that source temporarily.

---

**PATTERN: Engine Time Window Lockout (SNMPv3)**

SIGNALS INVOLVED:
- **`usmStatsNotInTimeWindows`**: Climbing — traps being rejected due to time window violation.
- **SNMPv3 Engine Boots Counter**: Device's boots counter has incremented (device rebooted).
- **Traps Received Rate (from specific device)**: Drops to zero after device reboot.
- **SNMP Agent Reachability**: Device responds to polling (it's up), but traps are rejected.
- **`usmStatsUnknownEngineIDs`**: May also increment if the receiver hasn't seen the post-reboot engine state.

NARRATIVE:
A device reboots (scheduled or unscheduled). On reboot, its `snmpEngineTime` resets to 0 and `snmpEngineBoots` increments. The trap receiver's cached engine time for that device is now stale. The first post-reboot trap from the device has engine time far from what the receiver expects, outside the ±150-second window. The receiver rejects the trap with `usmStatsNotInTimeWindows` incrementing. Traps are unidirectional — the receiver cannot autonomously query the device for the new engine time. Normally, a subsequent trap eventually triggers a report that refreshes the cache, but if this fails (network issue, rate limiting, NAT), the device is permanently locked out. Recovery requires a manager-initiated GET request to the device, which triggers a Report PDU with the new engine state and refreshes the receiver's cache.

SEVERITY: PAGE — Device is up and running but its traps are silently rejected. Any event on that device is invisible.

DISTINGUISHING FEATURES:
- Device responds to SNMP polling (it's not down).
- `usmStatsNotInTimeWindows` is the specific counter climbing (not `wrongDigests` — credentials are correct).
- The pattern correlates with device reboots.
- Only affects SNMPv3 devices.

COMMON CAUSES:
- Device reboot (scheduled or crash) with failed engine time re-discovery
- Device clock drift beyond the time window (rare on devices with NTP, but possible after a clock battery failure or NTP misconfiguration)
- Receiver engine time cache corruption
- Network issue preventing engine time discovery exchange after device reboot
- NAT device interfering with SNMPv3 discovery exchange
- Receiver rate-limiting engine time discovery requests

FIRST RESPONSE:
1. Confirm the lockout: check `usmStatsNotInTimeWindows` delta.
2. Query the device's current engine time: `snmpget -v3 ... <device-ip> SNMP-FRAMEWORK-MIB::snmpEngineBoots.0` and `snmpEngineTime.0`. Compare with receiver's cached values.
3. Force engine time re-discovery: on the receiver, clear the cached engine time entry for the affected device. The next bidirectional operation (GET) will refresh the cache.
4. If re-discovery fails: network path issue. Troubleshoot connectivity between receiver and device for SNMPv3 discovery packets.
5. Prevent recurrence: ensure engine time discovery is not rate-limited too aggressively on the receiver.

---

**PATTERN: MIB Gap Blindness**

SIGNALS INVOLVED:
- **Unknown OID Trap Count**: High and growing — many traps with unresolved OIDs.
- **Traps Received Rate**: Normal or elevated — traps are being received.
- **Traps Processed Rate**: May appear normal — traps are technically "processed" (logged) but not interpreted.
- **Alerting Volume**: Lower than expected for the event type — alerts that should fire on resolved trap names don't fire on raw OIDs.
- **MIB Resolution Cache Hit Rate**: Low or declining.

NARRATIVE:
A new device type is deployed, or a firmware upgrade adds new trap types, but the corresponding MIBs are not loaded into the trap receiver. Traps from the new device/OID arrive and are received successfully, but the MIB resolution engine can't translate the numeric OIDs to human-readable names. The traps are logged as raw OID strings. Alerting rules written against resolved trap names (e.g., `IF-MIB::linkDown`) don't match the raw OIDs. Critical events are technically received by the infrastructure but are operationally invisible — no alerts fire, no tickets are created, no one is paged.

SEVERITY: TICKET — Degraded monitoring. Specific events received but not actionable. PAGE if the gap affects critical device types after a widespread deployment or firmware upgrade.

DISTINGUISHING FEATURES:
- Traps are being *received* (not dropped), but not *interpreted*.
- Trap receiver shows no errors or drops — everything appears healthy from an infrastructure perspective.
- Logs show raw numeric OIDs instead of resolved names for the affected traps.
- Alerting volume for the affected device type is lower than expected.

COMMON CAUSES:
- New device model deployed without loading its vendor MIBs
- Firmware upgrade on existing devices adding new enterprise trap OIDs
- MIB file present but not compiled (syntax error, missing dependency MIB)
- MIB directory path changed after NMS upgrade or migration
- Vendor MIB file not obtained (proprietary or requires download from vendor portal)

FIRST RESPONSE:
1. Identify the unresolved OIDs: extract the raw OID strings from trap logs. Use `snmptranslate -IR <oid>` or online OID databases to identify what the trap is.
2. Locate the correct MIB: determine the vendor and device model. Download the appropriate MIB file from the vendor's support portal.
3. Compile and load the MIB: add the MIB file to the trap receiver's MIB directory and restart or signal the receiver to reload MIBs.
4. Verify resolution: send a test trap from the affected device type and confirm it resolves to a human-readable name.
5. Update alerting rules: if new trap types were added, create or update alerting rules for the newly resolved trap names.

---

**PATTERN: Rogue Trap Injection / Trap Receiver DoS**

SIGNALS INVOLVED:
- **Traps Received Rate**: Sudden spike, potentially massive.
- **Trap Source IP Distribution**: New, unrecognized source IPs dominating volume.
- **Malformed Trap Count**: May be elevated if the injection is crude.
- **UDP `RcvbufErrors`**: Climbing if volume is sufficient to overwhelm the receiver.
- **Traps Deduplication Rate**: Low — injected traps may be unique (not duplicates).
- **Authentication Failure Traps (from devices)**: May also increase if the attack includes SNMP scanning.

NARRATIVE:
An unauthorized device or compromised host on the network begins sending large volumes of SNMP traps to the receiver. This could be intentional (DoS attack against the monitoring infrastructure, trap injection to mask real events or create false ones) or unintentional (misconfigured application, test device sending production traps, device in a trap-generation loop). The flood of traps consumes receiver resources, potentially pushing out legitimate traps. If the injected traps are well-formed and pass MIB resolution, they may generate false alerts, causing operational confusion. If malformed, they consume processing time on parse failures.

SEVERITY: PAGE — Monitoring infrastructure under attack or severe stress.

DISTINGUISHING FEATURES:
- New source IPs not in the device inventory.
- Trap volume dominated by one or a few sources rather than distributed across many devices.
- Trap content may be repetitive, random, or contain unusual variable bindings.
- May be accompanied by other reconnaissance activity (port scans, SNMP polling attempts from the same source).
- If SNMPv3, the traps may fail USM authentication (wrong engine ID, wrong credentials) — appearing in USM statistics.

COMMON CAUSES:
- Compromised host on the management network
- Misconfigured test or development device sending traps to the production receiver
- Deliberate DoS attack against the monitoring infrastructure
- Malware that includes SNMP trap generation capability
- Rogue network device (unauthorized switch or access point) sending traps
- Loop in a trap forwarding chain (trap receiver A forwards to receiver B, which forwards back to A)

FIRST RESPONSE:
1. Identify the rogue source IPs: from trap logs, extract source IPs and rank by volume. Identify IPs not in the device inventory.
2. Block at the network layer: if possible, apply an ACL or firewall rule blocking UDP/162 from the rogue source IP(s). Do this on the nearest upstream network device to reduce load on the receiver.
3. Assess impact: check whether legitimate traps were dropped during the flood (`RcvbufErrors`, application drops).
4. Investigate the source: determine what device is at the rogue IP.
5. Remediate: if misconfigured, fix the device. If malicious, contain the device and initiate security incident procedures.

---

**PATTERN: Receiver Process Death (Silent)**

SIGNALS INVOLVED:
- **Trap Receiver Process Liveness**: Process exists but health check fails (or no health check → undetected).
- **UDP Receive Buffer Fill (Recv-Q)**: Growing (kernel buffers traps but nobody reads them).
- **UDP `RcvbufErrors`**: Incrementing.
- **Trap Ingestion Rate (application-level)**: Zero.
- **Kernel-level trap arrival (`tcpdump`)**: Traps are still arriving at the network layer.
- **Downstream systems**: No new data appearing.

NARRATIVE:
The trap receiver process has crashed, hung (deadlock), been OOM-killed, or entered a prolonged GC pause. It's no longer reading from the UDP socket. The kernel continues accepting UDP datagrams into the socket buffer, but nobody drains it. Once the buffer fills, the kernel drops all incoming traps. The critical danger: if you only monitor application-level metrics (processed trap count), everything looks like "zero traffic" — which could be mistaken for "nothing is happening." In reality, traps are piling up and being dropped at the kernel level. This is exactly the scenario that CVE-2025-68615 (Net-SNMP `snmptrapd` buffer overflow, December 2025) can trigger: a remote unauthenticated attacker sends a crafted packet to `snmptrapd`, crashes the daemon, and traps stop flowing until restart.

SEVERITY: PAGE — Complete loss of trap-based monitoring. Every trap from every device is being lost.

DISTINGUISHING FEATURES:
- Kernel-level `tcpdump` on port 162 shows traps arriving, but application-level count is zero.
- Process may appear "running" in process list but is actually stuck.
- Correlates with OOM killer messages in `dmesg` or `/var/log/kern.log`.
- Correlates with resource exhaustion (memory, file descriptors) preceding the event.

COMMON CAUSES:
- Out-of-memory kill by the kernel OOM killer
- Segfault in the receiver or its handler plugins
- Deadlock in multi-threaded receiver
- Python/Perl handler code throwing an uncaught exception, killing the handler process
- Log file rotation killing the receiver (if it holds the log file open and rotation sends SIGHUP incorrectly)
- JVM garbage collection pause (if receiver is Java-based)
- Remote exploit against the receiver daemon itself (CVE-2025-68615 for `snmptrapd`)

FIRST RESPONSE:
1. Verify process state: `ps aux | grep snmptrapd` and `systemctl status snmptrapd`.
2. Check kernel buffer: `cat /proc/net/udp | grep :00A2` — is Recv-Q growing?
3. Check for OOM kills: `dmesg | grep -i "oom\|killed" | tail -5` and `journalctl -u snmptrapd --since "1 hour ago"`.
4. Restart the receiver: `systemctl restart snmptrapd`.
5. After restart, verify the synthetic trap heartbeat resumes at the downstream consumer.

---

### SECTION 3 — Capacity & Saturation Leading Indicators

Resources ordered by time-to-impact (fastest saturating first).

---

**RESOURCE: UDP Socket Receive Buffer (Receiver Host)**

LEADING INDICATORS:
- **Recv-Q for port 162** (`ss -u -a -n | grep :162`): Current fill in bytes. Sustained nonzero Recv-Q = receiver not draining fast enough.
- **`RcvbufErrors` delta** (`/proc/net/snmp`): Any nonzero rate of increase.
- **Traps Received Rate trending upward** toward known receiver capacity.

DEGRADATION CURVE:
**Cliff-edge.** The UDP receive buffer has a hard limit (configured by `SO_RCVBUF` and `net.core.rmem_max`). When full, the kernel drops datagrams immediately — no graceful degradation. Recv-Q approaching the limit is the only warning, but by the time Recv-Q hits the limit, drops have already started. The Linux default `net.core.rmem_max` is 212,992 bytes (~208 KiB) — entirely inadequate for production trap volume. Production deployments must raise this to 8-32 MB depending on the largest expected burst.

RUNWAY ESTIMATION:
1. Current buffer fill rate: `Recv-Q` growth per second (sample twice, 10 seconds apart, delta/10).
2. Remaining buffer capacity: `SO_RCVBUF - current Recv-Q`.
3. Time to overflow: `remaining_capacity / fill_rate` (seconds).
4. If the fill rate is zero (queue draining faster than filling), runway is infinite.
5. During a trap storm, re-estimate frequently — the fill rate can spike.

HEADROOM DEFINITION:
Recv-Q consistently below 20% of `SO_RCVBUF` during peak trap volume. If Recv-Q exceeds 50% during peak, increase buffer size or receiver processing capacity. The buffer should be sized to absorb the largest expected trap burst without drops.

---

**RESOURCE: Trap Processing Pipeline (Downstream Backlog)**

LEADING INDICATORS:
- **Queue fill percentage** (for receivers with exposed internal queues): trending upward.
- **Traps Received Rate vs. Traps Processed Rate**: Received consistently exceeding processed by sustained margin.
- **Worker pool utilization**: Above 80% sustained.
- **Trap-to-Alert Processing Latency**: Increasing.

DEGRADATION CURVE:
**Initially graceful, then cliff-edge.** As the queue fills, processing latency increases linearly (each trap waits for all ahead of it). When the queue reaches capacity, new traps are dropped. The transition can be sudden if trap arrival rate spikes.

RUNWAY ESTIMATION:
1. Queue growth rate: `received_rate - processed_rate` (traps/second).
2. Remaining queue capacity: `queue_max - queue_depth`.
3. Time to overflow: `remaining_capacity / growth_rate` (seconds).
4. During a trap storm, growth rate can spike dramatically — use worst-case estimates.

HEADROOM DEFINITION:
Queue fill below 30% during peak normal operations. Queue should be sized to absorb at least 60 seconds of peak arrival rate at zero processing rate. For example, if peak arrival rate is 1,000 traps/second, the queue should hold at least 60,000 traps. This gives operators 60 seconds to respond to a pipeline stall before traps are lost.

---

**RESOURCE: Trap Receiver CPU**

LEADING INDICATORS:
- **Process CPU utilization**: Sustained above 70% during normal operations.
- **Per-trap processing time**: Increasing.
- **Worker pool utilization**: Correlates with CPU.
- **MIB resolution cost**: Rising as MIB set grows or cache degrades.

DEGRADATION CURVE:
**Graceful then cliff.** As CPU utilization increases, processing latency increases gradually (more time on CPU per trap due to scheduling delays). At near-100% utilization, latency spikes sharply. At 100% sustained, the processing rate is at maximum — any increase in arrival rate causes queue growth and eventual drops. For single-threaded decoders (most `snmptrapd` installations), one core at 100% = saturated, even on a many-core machine.

RUNWAY ESTIMATION:
1. CPU headroom: `1.0 - current_utilization`.
2. Current processing capacity: `current_processed_rate / current_utilization` (traps/second at 100% CPU).
3. How much arrival rate can increase before saturation: `capacity - current_received_rate`.
4. Time to saturation: `(capacity - current_received_rate) / growth_rate` if arrival rate is growing.

HEADROOM DEFINITION:
Comfortable headroom: process CPU utilization below 50% during peak normal operations (2× headroom for unexpected volume). Above 70% sustained during normal operations, capacity planning should be initiated. Above 90% during a storm is expected but should trigger auto-scaling or resource allocation if available.

---

**RESOURCE: MIB Resolution Performance**

LEADING INDICATORS:
- **MIB resolution cache hit rate**: Declining below 95%.
- **Unique OID count**: Approaching cache size limit.
- **Per-trap processing latency**: Increasing even at low volume (cache misses add latency independent of queue depth).

DEGRADATION CURVE:
**Graceful.** Cache miss rate increases gradually as the working set (unique OIDs) exceeds cache capacity. Each miss adds latency but doesn't cause failures. Cumulative effect can reduce pipeline throughput enough to cause queue growth. MIB resolution with large, unindexed MIB sets scales unfavorably — algorithmic complexity can approach O(n²) for the default `binary_array` implementation in Net-SNMP.

RUNWAY ESTIMATION:
1. Unique OID working set: count unique OIDs received over a representative period (1 week).
2. Cache capacity: maximum number of OID entries the cache can hold.
3. Runway: `(cache_capacity - current_working_set) / new_oid_growth_rate` (days until cache overflow).
4. New device deployments or firmware upgrades can cause sudden working set increases.

HEADROOM DEFINITION:
Comfortable: cache hit rate above 95%. Cache capacity at least 2× the working set size. If the cache can't hold at least 1.5× the working set, evictions will cause persistent miss rate above 5%.

---

**RESOURCE: Disk I/O for Trap Logging and Storage**

LEADING INDICATORS:
- **Disk write latency** (`/proc/diskstats` or `iostat`): Increasing beyond baseline.
- **I/O wait percentage**: Increasing (process spending time waiting for disk I/O).
- **Trap log file size growth rate**: If approaching disk capacity or filesystem limits.
- **Disk queue depth** (`iostat`): Sustained nonzero depth.
- **Inode utilization** (`df -i`): A directory with millions of small files can exhaust inodes before disk space.

DEGRADATION CURVE:
**Graceful then cliff.** As disk I/O approaches saturation, write latency increases gradually. The trap processing pipeline slows if it synchronously writes to disk before acknowledging processing. At full I/O saturation, the pipeline stalls — not because of CPU or memory, but because it can't write results to disk fast enough. If disk fills completely, the pipeline may crash or stop accepting new traps.

RUNWAY ESTIMATION:
1. Disk space growth rate: `log_file_size_growth_per_day`.
2. Remaining disk space: `available_space - reserved_space`.
3. Time to disk full: `remaining_space / growth_rate` (days).
4. During a trap storm, growth rate can spike 10-100× normal. Use storm-scenario estimates.
5. Also estimate I/O bandwidth runway: `max_IOPS - current_IOPS` gives headroom for increased write rate.

HEADROOM DEFINITION:
Comfortable: disk space utilization below 70%. I/O utilization (`%util`) below 50% during peak normal operations. Log retention policy should ensure that logs are rotated or archived before disk exceeds 80%. During a 1-hour storm, the disk should absorb 10× normal write rate without reaching 90% utilization.

---

**RESOURCE: Network Bandwidth (Management Plane)**

LEADING INDICATORS:
- **Interface utilization on management VLAN**: Sustained above 40%.
- **Trap packet loss rate**: Traps arriving out of order or missing.
- **Interface output queue drops** on devices forwarding traps.
- **All management traffic latency** (SSH, syslog, NTP, polling): Rising latency = management plane saturation.

DEGRADATION CURVE:
**Graceful then cliff.** As bandwidth utilization increases, trap delivery latency increases gradually (queuing delay on congested links). At high utilization, jitter increases and some traps are dropped by intermediate devices (output queue drops on switches/routers). The degradation affects all management protocols simultaneously, not just traps.

RUNWAY ESTIMATION:
1. Current management VLAN utilization (percentage of link capacity).
2. Estimate peak trap bandwidth: `peak_trap_rate × average_trap_size × 8` (bits/second).
3. Time to link saturation: `(link_capacity - current_utilization) / traffic_growth_rate`.
4. During a trap storm, traffic can spike to 10-100× normal — the management VLAN can saturate in seconds.

HEADROOM DEFINITION:
Comfortable: management VLAN utilization below 30% during normal operations (accounting for all management traffic, not just traps). This provides headroom for trap storms without affecting other management functions. During a sustained trap storm, temporary utilization above 70% is acceptable but should trigger investigation. Dedicated management networks should be provisioned with at least 3× normal peak utilization as headroom.

---

**RESOURCE: SIEM / Downstream Consumer Ingest Quota**

LEADING INDICATORS:
- **Daily ingest volume** (if SIEM is volume-licensed).
- **Forwarder lag** (time from receiver to SIEM).
- **SIEM ingest rate vs. license quota**.

DEGRADATION CURVE:
**Variable.** Some SIEMs backpressure (queue grows, eventually drops); others silently drop. Some license by volume and shut off after a quota.

RUNWAY ESTIMATION:
1. Daily quota minus current ingest, divided by days remaining in license period.
2. Daily quota minus current ingest, divided by current growth rate = days to quota hit.

HEADROOM DEFINITION:
Comfortable: < 50% of daily quota used with 7+ days runway.
Critical: > 80% or quota exhaustion in < 3 days.

---

### SECTION 4 — Operational Edge Cases

Ordered by impact × likelihood. Silent failures (look normal but are catastrophic) bubble to the top.

---

**Behaviors That Look Normal but Are Silently Catastrophic**

1. **SNMPv3 USM silent rejection (`usmStatsWrongDigests` or `usmStatsNotInTimeWindows` climbing).** The receiver appears healthy, devices appear healthy, traps are being "sent" — but USM authentication failures cause the receiver to silently discard every trap from affected devices. No alerts fire. No errors are logged in the main trap log (only in USM statistics). The most insidious failure mode because all standard health checks appear green. Only monitoring USM statistics counters catches this.

2. **MIB resolution failure for critical trap types.** Traps are received, logged, and "processed" — but the OID resolves to a raw numeric string that doesn't match any alerting rule. The pipeline reports success. No alerts fire. Critical events are technically in the logs but operationally invisible. Only monitoring the unknown OID trap count catches this.

3. **Trap receiver process alive but internally wedged.** The process is running, systemd reports it as active, the socket is bound — but the process has stopped reading from the UDP socket (deadlock, infinite loop, GC pause, handler hang). Traps pile up in the kernel buffer and are eventually dropped. The process appears healthy from the outside. Only monitoring the received trap rate (which drops to zero) or UDP `RcvbufErrors` (which climb) catches this.

4. **Partial network partition affecting trap delivery but not polling.** A firewall rule change or ACL modification blocks UDP/162 from a specific subnet while UDP/161 (polling) is unaffected. Polling continues to work, devices appear healthy in the NMS, but traps from those devices are silently dropped by the firewall. Only monitoring trap source IP cardinality (missing devices from the trap source list) catches this.

5. **Trap deduplication over-aggregation suppressing legitimate events during real incidents.** The dedup engine is configured to suppress more than N `linkDown` traps per minute from a device. During a real outage affecting many interfaces on one device (e.g., line card failure), legitimate `linkDown` traps are suppressed. The operator sees "normal" trap volume because the dedup engine is hiding the real event. Only monitoring the dedup/throttle suppression count (and alerting on high suppression volume during events) catches this.

6. **"No traps = everything is fine."** The most dangerous assumption in trap-based monitoring. A device that has crashed, lost management connectivity, or had its SNMP agent hang generates zero traps. Zero traps is indistinguishable from "nothing bad is happening" unless you have explicit heartbeat/absence monitoring. Every production trap monitoring system must have a mechanism to detect the ABSENCE of expected traps — either per-device expected-frequency baselines or synthetic heartbeats.

7. **Successful MIB resolution of the WRONG MIB.** A vendor ships a new MIB with an OID that collides with or overlaps an existing MIB's OID space. The receiver resolves the OID to a name, but it's the wrong name. The trap appears normal with a resolved name, but the interpretation is wrong. You're reading the trap correctly but understanding it incorrectly. Extremely rare but devastating during incidents.

8. **Forwarder broken between receiver and SIEM.** Traps arrive at the receiver, get stored, but the forwarder to the SIEM is wedged. The receiver reports "healthy" by its own metrics. Traps are not reaching the place where SecOps would see them. Only end-to-end synthetic trap monitoring (asserting the trap lands in the SIEM) catches this.

---

**Behaviors That Look Alarming but Are Normal**

1. **Post-reboot trap burst.** After a device reboot, expect a burst of `coldStart` (or `warmStart`), `linkUp` for active interfaces, and routing adjacency traps. A device reboot generates 20-200 traps in a few seconds. This looks like a trap storm but is normal boot behavior. In a fleet rebooting during a maintenance window, this can generate thousands of benign traps. Suppress or de-prioritize during planned maintenance.

2. **High link trap volume during network provisioning.** New devices being installed, cables being connected, and ports being enabled generate `linkUp` and `linkDown` traps. This is normal during provisioning activities.

3. **SNMPv3 `usmStatsNotInTimeWindows` spikes after device reboots.** After a device reboots, its engine time resets. Until the receiver re-discovers the engine time, traps are rejected. A brief spike (seconds to a minute) is normal. If it persists, the discovery process has failed.

4. **Duplicate traps from redundant management paths.** In networks with redundant management paths, a single trap may arrive via both paths. The receiver sees two copies. This is expected and should be handled by deduplication matching on trap content + source device + timestamp, not source IP.

5. **Trap volume drops to near-zero during quiet periods.** In networks with low overnight activity, trap volume naturally drops. A low trap rate from quiet devices is not a failure. The trap silence threshold must account for device type and time of day.

6. **Inform retransmissions during receiver restart.** When the trap receiver restarts, Informs sent during the restart window are not acknowledged, causing devices to retransmit. A brief retransmission burst on receiver startup is normal.

7. **Periodic trap rate spikes at fixed intervals.** Some devices send periodic status traps (health checks, temperature reports, counter resets) at fixed intervals. These show up as regular spikes in the trap rate graph. They look like problems if you're not aware of the schedule.

8. **High trap rate from firewalls/load balancers.** Firewalls generating `denied packet` traps or load balancers generating `server up/down` traps can produce very high sustained rates during normal traffic conditions. Normal for their function but can dominate trap infrastructure capacity if not filtered at the source.

---

**Cold Start, Warmup, and Initialization Behaviors That Produce False Positives**

1. **Trap receiver startup with cold MIB cache.** After a receiver restart, the MIB resolution cache is cold (0% hit rate). Traps arriving in this window have elevated per-trap processing latency. The receiver ramps up over minutes as caches warm. Expect elevated processing latency, elevated `usmStatsNotInTimeWindows` (engine time re-discovery), and potential Inform retransmissions (missed acknowledgments during downtime).

2. **Device cold start trap sequence.** When a device boots, it typically sends `coldStart` first, then `linkUp` traps as interfaces initialize, then routing protocol traps. If you receive `coldStart` but no subsequent traps, the device may have failed during boot.

3. **Post-maintenance trap burst.** After maintenance on devices that were administratively shut down, a burst of traps is normal as interfaces come up, routing sessions re-establish, and the device resumes normal operation. This burst can last several minutes.

4. **MIB loading delay on receiver.** After a receiver restart, the MIB engine may take time to compile or load all vendor MIBs (30 seconds to 5+ minutes for a large vendor MIB set). Traps arriving in this window may be logged as unknown OIDs even though the MIBs are present. This is a transient resolution failure, not a true MIB gap.

5. **Agent starts before routing converges.** A device may send traps (e.g., to a remote NMS) before the management VRF has a route. The traps are dropped by the local forwarding plane. The operator may see a partial initial burst, then silence, then recovery.

6. **First trap from a device after a long silence.** A device that was on a long p99 inter-trap gap then traps. Looks like a flap when it's just a state change.

7. **MIB pack reload.** After updating the MIB pack, traps that were previously undecoded now have names. The unknown-OID rate drops to zero. This is good, not a sign of a problem.

---

**Signals Critical During Incidents but Rarely Proactively Monitored**

1. **Device-side trap send logs.** Most teams monitor the receiver. During an incident, the most important signal is whether the device *thinks* it sent the trap. These logs are rarely collected centrally because they are device-local and verbose.

2. **Firewall/ACL hit counts for UDP/162.** In a hybrid/cloud environment, the security group or firewall rule may be silently dropping traps. The hit count is rarely exported to the NMS but is the definitive signal for "why traps aren't arriving."

3. **Per-source NTP offset.** Each device's clock drift. Easy to compute when you have the trap varbind; easy to ignore because "we assume NTP works." A device with broken NTP silently breaks cross-source event correlation.

4. **Trap-source IP consistency per source.** The receiver should be tracking the *expected* source IP per device. Drift is a config change you didn't authorize.

5. **Filter rule hit count over time.** A filter rule that is suppressing 10,000 events a day is either needed (legitimate noise) or a defect (suppressing signal). Either way, you need to know.

6. **MIB pack version vs device firmware version.** A firmware upgrade can introduce new trap OIDs. If the MIB pack is not updated, unknown-OID rate jumps. This comparison is rarely automated.

7. **Trap receiver load testing.** Regular load tests at 2× and 5× normal peak trap volume. Verify pipeline capacity, measure latency under load, identify breaking points. Update capacity models based on results. Most teams have never load-tested their trap receiver.

---

**Known Instrumentation Limitations**

1. **UDP trap delivery is inherently unobservable at the sender.** There is no way to know, from the receiver side, how many traps were lost in transit. You can only measure what arrives. To estimate loss, compare device-side trap generation counters (if queryable via SNMP polling) with receiver-side received counts.

2. **SNMPv1/v2c trap timestamps reflect arrival time, not event time.** The timestamp in the trap PDU may be the device's notion of when the event occurred, or it may be the time the trap was generated. Network transit time adds additional delay. For precise event timing, use syslog (TCP-based, more reliable timestamps) or streaming telemetry.

3. **`/proc/net/snmp` `RcvbufErrors` is system-wide.** On shared servers, this counter increments for all UDP sockets. You cannot isolate port 162-specific buffer errors from this counter alone. Use `ss -u -a -n` for per-socket Recv-Q, and correlate with application-level drops.

4. **SNMPv1 trap PDUs have a different structure than SNMPv2c/v3 notifications.** A receiver expecting v2c/v3 may misparse v1 traps (or vice versa). Ensure the receiver is configured to accept all versions in use.

5. **Trap PDU size limits.** SNMPv1 limits PDU size to 484 bytes. SNMPv2c and v3 support larger PDUs but may be limited by device implementation. Large variable bindings may be truncated or cause the trap to be dropped. There is no standard way to detect truncation from the receiver side.

6. **MIB resolution accuracy depends on MIB quality.** Poorly written or incorrect MIBs can cause OID resolution errors that produce misleading trap names. This is an accuracy limitation, not an availability limitation.

7. **`snmptrapd` has no native internal queue depth metric.** Operators familiar with commercial NMS receivers expect to monitor a "queue depth" counter. `snmptrapd` does not expose one. The observable proxies are `RcvbufErrors` (kernel-level drops) and downstream queue lag (if feeding Kafka/RabbitMQ). This is a known gap that requires custom instrumentation to fill.

8. **SNMPv3 engine discovery limitation.** Traps are unidirectional; the receiver cannot autonomously discover a new engine ID. The device must first be polled (bidirectional) to establish engine state. This is a fundamental protocol limitation, not an implementation bug.

9. **v2c community strings are cleartext on the wire.** Any network sniffer captures them. This is by design (v2c is unencrypted) but is the most common entry point for real-world attacks. The fix is v3, not monitoring.

---

**Interactions with Adjacent Systems That Affect Signal Interpretation**

1. **SNMP traps and syslog correlation.** Many network events generate both an SNMP trap and a syslog message. Traps provide structured data (OIDs, variable bindings); syslog provides richer contextual detail. During incident response, correlate traps with syslog by device IP and timestamp window. The combination is more diagnostic than either alone. Cross-correlation also detects *asymmetric* loss: a syslog event present but the corresponding trap absent = trap delivery path is broken.

2. **SNMP traps and NetFlow/IPFIX.** A `linkDown` trap followed by a traffic drop on the affected interface (visible in NetFlow) confirms user impact. A trap without corresponding traffic impact may be a non-critical interface. NetFlow without a trap may indicate a logical issue (routing) rather than a physical issue. SNMP tells you "is the infrastructure healthy?"; flow tells you "what traffic is driving this?"; together they give root cause.

3. **SNMP traps and ICMP/synthetic monitoring.** If a `linkDown` trap fires but synthetic probes to destinations through that link succeed, the link may have redundant backup. If synthetic probes fail but no trap was received, the trap delivery path is the issue, not the network path.

4. **SNMP traps and SIEM.** Traps should be forwarded to the SIEM as event data. Security-relevant traps (authenticationFailure, config change, BGP peer changes) are particularly valuable for SIEM correlation. However, trap volume during storms can overwhelm SIEM ingestion. Rate-limit or filter non-security traps before forwarding to SIEM.

5. **SNMP traps and streaming telemetry.** Modern networks increasingly deploy streaming telemetry (gNMI, gRPC) alongside SNMP. Traps provide event-driven notification; streaming telemetry provides periodic high-resolution data. During the transition period (which lasts years in most enterprises), both must be monitored and correlated. Streaming telemetry is gradually replacing SNMP *polling*, not traps.

6. **SNMP traps and management VLAN ACLs.** A firewall rule change blocking UDP port 162 from specific source subnets silently stops trap delivery from those devices. No alert is generated because the firewall doesn't know it's blocking traps (it's just dropping UDP). This is one of the most common causes of "silent device death" that turns out to be a network policy change.

7. **SNMP traps and NTP dependencies.** SNMPv3 authentication relies on synchronized clocks (the USM time window is ±150 seconds per RFC 3414). If NTP breaks on either the device or the receiver, SNMPv3 authentication fails silently. Traps stop flowing. This is often misdiagnosed as a credential problem when it's actually a clock problem.

8. **SNMP traps and DNS dependencies.** If the trap receiver performs reverse DNS lookups on source IPs (for logging or alerting), DNS failures or latency directly impact trap processing throughput. A DNS timeout of 30 seconds per trap means 30 seconds of processing delay per trap during a DNS outage. Always check: is the receiver doing DNS lookups, and what happens when DNS fails?

9. **SNMP traps and configuration management systems.** An infrastructure-as-code rollout that updates `snmpd.conf` or device CLI templates may inadvertently change the trap host. The change is visible in the config repo but not in the NMS until traps stop. Monitoring must validate the rendered config, not just the repo.

10. **CVE-2025-20352 and CVE-2025-68615 (recent SNMP threats).** In September 2025, a stack overflow in the Cisco IOS/IOS XE SNMP subsystem (CVE-2025-20352) was actively exploited in the "Operation Zero Disco" campaign — attackers used default `public` community strings on switches to gain initial foothold, exploited the stack overflow for RCE as root, and deployed fileless Linux rootkits. Up to 2 million devices were potentially affected. In December 2025, a buffer overflow in Net-SNMP `snmptrapd` (CVE-2025-68615) was disclosed — a remote unauthenticated attacker can crash the daemon. Both vulnerabilities underscore that "just monitoring SNMP" is not enough; the receiver itself is an attack surface that must be patched, and v1/v2c default strings are an active threat vector.

---

### SECTION 5 — Security & Integrity Signals

Signals relevant to abuse detection, unauthorized access, data integrity violations, and operational security events. Same 9-field format as Section 1.

---

**SIGNAL: SNMPv3 USM Decryption Failure Rate (Receiver-Side)**

WHAT IT IS:
The rate of SNMPv3 trap decryption failures as measured by `usmStatsDecryptionErrors` on the receiver. Each increment represents a trap that passed authentication (HMAC verified) but could not be decrypted — the privacy (encryption) key or algorithm is mismatched.

SOURCE:
- `SNMP-USER-BASED-SM-MIB::usmStatsDecryptionErrors.0` on the receiver host (OID `.1.3.6.1.6.3.15.1.1.6.0`)
- Trap receiver application log: decryption failure entries

HOW TO COLLECT IT MANUALLY:
```bash
# Query USM decryption error counter:
snmpget -v3 -u <admin> -l authPriv -A <pass> -X <pass> \
  localhost SNMP-USER-BASED-SM-MIB::usmStatsDecryptionErrors.0

# Track delta over time to get rate.
```

WHAT IT TELLS YOU:
Authentication succeeded but decryption failed, meaning the sender has the correct authentication key but the wrong encryption key (credential misconfiguration), or the sender and receiver are using different encryption algorithms (device sending AES-256, receiver expecting AES-128), or a USM implementation bug. These traps are authenticated but unreadable.

SEVERITY:
PAGE — Sustained nonzero rate. SNMPv3 traps are authenticated but not readable. Monitoring data is being lost.
TICKET — Any nonzero delta without a known cause.

THRESHOLDS:
- Binary: any sustained nonzero rate is abnormal.
- Correlate with recent privacy key changes. If the counter increments after a privacy key rotation, the rotation was incomplete or incorrect.

FAILURE MODES DETECTED:
- SNMPv3 privacy key mismatch
- Encryption algorithm mismatch (AES-128 vs AES-256 vs DES)
- Privacy key localization error
- Incomplete SNMPv3 credential rotation
- USM implementation incompatibility between device vendor and receiver

NUANCES & GOTCHAS:
`usmStatsDecryptionErrors` implies that authentication *succeeded* (the HMAC was verified). This specifically narrows the problem to the privacy layer, not the authentication layer. If both `wrongDigests` and `decryptionErrors` are climbing, there are two separate issues. Some older SNMPv3 implementations only support DES encryption, while modern best practice requires AES. A device configured for DES sending to a receiver that only accepts AES will generate these errors.

CORRELATES WITH:
- **`usmStatsWrongDigests`**: If only `decryptionErrors` climbing = privacy-specific issue. If both climbing = comprehensive credential mismatch.
- **Traps Received Rate (SNMPv3 devices)**: Device rate drops + decryption errors = those devices' traps are unreadable.
- **SNMPv3 Configuration Audit**: Decryption errors after a config change deployment = incorrect privacy parameters.

---

**SIGNAL: Unauthorized Trap Source IP (Rogue / Spoofed Source Detection)**

WHAT IT IS:
Detection of SNMP traps arriving from source IP addresses that are not in the authorized/expected device inventory. Any trap from an unknown source is a potential security event: misconfigured device, rogue device, or attacker injecting false monitoring data.

SOURCE:
- Trap receiver log: source IP in each trap entry
- Trap receiver statistics: trap_source_ip list compared against authorized source list
- Firewall/ACL logs: UDP/162 traffic from unauthorized sources
- Network device ARP/MAC tables: cross-reference for physical device identification

HOW TO COLLECT IT MANUALLY:
```bash
# Extract unique trap source IPs from last 24 hours:
grep ' Trap ' /var/log/snmptrapd.log | tail -50000 | \
  awk '{gsub(/[^0-9.]/,"",$NF); print $NF}' | sort -u > /tmp/seen.txt

# Compare against inventory:
comm -23 /tmp/seen.txt /var/lib/inventory/managed_ips.txt
# Output = sources sending traps that are NOT in the authorized list
```

WHAT IT TELLS YOU:
A source sending traps that isn't in the authorized inventory could be: a new device provisioned but not yet added to inventory (ops gap), a test/dev device misconfigured to send traps to the production receiver, a rogue device on the network (unauthorized switch, access point, or compromised host), an attacker injecting traps to create false alerts or mask real events, or a device with a changed IP address (DHCP reassignment) that isn't recognized.

SEVERITY:
TICKET — Any trap from an unauthorized source. Investigate the source.
PAGE — Traps from unauthorized sources generating alerts that trigger operational response (potential false data injection). Large volume from unauthorized sources (potential DoS).

THRESHOLDS:
- Binary: any trap from a source not in the authorized inventory is a finding.
- Volume: > 10 traps per minute from a single unauthorized source is suspicious.
- Alert generation: any unauthorized source generating alerts that trigger operational response is PAGE-worthy.

FAILURE MODES DETECTED:
- Rogue device on the management network
- Compromised host sending SNMP traps
- Misconfigured test/dev device
- New device not yet registered in inventory
- IP address reuse (authorized device decommissioned, new unauthorized device using same IP)
- Deliberate false data injection (attacker sending fake traps)

NUANCES & GOTCHAS:
DHCP environments may cause legitimate devices to change IPs. Use device identity (sysName, engine ID) in trap variable bindings rather than source IP alone for authorization. Authorized source lists must be kept current. Outdated inventories generate false positives. Some trap forwarding architectures change the source IP (forwarder IP replaces original device IP). Track the original device IP from trap variable bindings, not the IP header source. SNMPv3 provides stronger device authentication (engine ID + USM user) than v2c (community string, trivially spoofed). V3 traps from unknown engine IDs are higher confidence security events.

CORRELATES WITH:
- **Traps Received Rate**: Unusual volume from unknown sources = potential attack.
- **Malformed Trap Count**: Traps from unknown sources + malformed = likely attack tool.
- **Network Device ARP/MAC Tables**: Cross-reference unauthorized source IP with MAC to identify physical device.
- **802.1X / NAC Logs**: Unauthorized source IP + no NAC authentication = rogue device.

---

**SIGNAL: Configuration Change Without Correlating Change Ticket**

WHAT IT IS:
Detection of configuration change traps that don't correlate with an approved change record in the change management system. Detects unauthorized configuration changes.

SOURCE:
- Vendor-specific configuration change trap OIDs (CISCO-CONFIG-MAN-MIB::ciscoConfigManEvent, Juniper CFGMGMT traps, vendor equivalents)
- `SNMPv2-MIB::warmStart` (may indicate SNMP agent restart due to config change)
- Entity MIB traps indicating FRU (Field Replaceable Unit) changes
- Change management system API (for cross-referencing)

HOW TO COLLECT IT MANUALLY:
```bash
# Look for config change traps (vendor-specific, heuristic):
grep -i 'config.*change\|cfgMgmt\|ciscoConfigMan\|configuration' /var/log/snmptrapd.log | tail -20

# Check for warm start traps:
grep 'warmStart' /var/log/snmptrapd.log | tail -20

# Cross-reference with change calendar (manual):
# For each config change trap, check for a corresponding change ticket
# in the same time window.
```

WHAT IT TELLS YOU:
Configuration changes detected via traps that don't have corresponding approved change records indicate unauthorized changes. This could be: an administrator bypassing change management, an automated script making unexpected changes, an attacker modifying device configurations, or a device auto-healing.

SEVERITY:
TICKET — Config change trap without change ticket. Investigate the change and who made it.
PAGE — Config change trap on critical infrastructure (core router, firewall, DNS) without change ticket. Potential security incident.

THRESHOLDS:
- Binary: any config change trap without a corresponding change ticket within a ±30 minute window is anomalous.
- Rate: more than 2 uncorrelated config changes from the same device in 24 hours is suspicious.

FAILURE MODES DETECTED:
- Unauthorized configuration changes by administrators
- Automation errors making unapproved changes
- Attacker modifying device configurations
- Device auto-rollback or auto-healing triggering config change events
- Configuration drift between intended and actual state

NUANCES & GOTCHAS:
Not all config change traps are generated by all devices. Verify that critical devices are configured to send them. Some automated configuration management tools (Ansible, Salt, Puppet) make frequent small changes. These should have corresponding automation run records, not necessarily manual change tickets. Config change traps may be delayed. The timestamp in the trap may not exactly match the change time. Allow a window for correlation. Vendor-specific config change traps have different semantics. Some fire on any config write; others only on specific config sections.

CORRELATES WITH:
- **Warm Start Traps**: Config change + warm start = config applied with SNMP agent restart.
- **Syslog from Same Device**: Config change trap + syslog entry showing who made the change (user, source IP) = attribution.
- **AAA / TACACS+ / RADIUS Logs**: Config change + AAA log showing the CLI session = full attribution.
- **Change Management System**: Config change trap + matching change ticket = authorized. No matching ticket = unauthorized.

---

**SIGNAL: Active SNMP Brute-Force / Scanning Detection (Across Devices)**

WHAT IT IS:
Detection of systematic SNMP probing across multiple devices or multiple community strings from a single source. Identified by a pattern of authentication failure traps across many devices in a short time window, indicating they all received bad requests from the same external source.

SOURCE:
- Receiver log: multiple `authenticationFailure` traps from different device source IPs (not the attacker's IP) within a short window
- Device-side SNMP counters (if polled): `snmpInBadCommunityNames`, `snmpInASNParseErrs`
- Network capture: `tcpdump` for rapid sequential GET requests to multiple IPs from one source
- Firewall/ACL logs: denied UDP/161/162 traffic from unexpected sources

HOW TO COLLECT IT MANUALLY:
```bash
# Count auth failure traps by time window:
grep 'authenticationFailure' /var/log/snmptrapd.log | \
  awk '{print $1, $2}' | cut -d: -f1,2 | sort | uniq -c | sort -rn

# Network capture for scanning patterns:
tcpdump -i eth0 'udp port 161 or udp port 162' -nn -c 1000 2>/dev/null | \
  awk '{print $3}' | cut -d. -f1-4 | sort | uniq -c | sort -rn | head

# From devices (sample a few):
snmpget -v2c -c <community> <device> SNMPv2-MIB::snmpInBadCommunityNames.0
```

WHAT IT TELLS YOU:
Someone is scanning the network for SNMP services, likely trying community strings. This is reconnaissance — a precursor to exploitation. SNMP agents with default community strings (`public`, `private`) are trivial to exploit (the CVE-2025-20352 campaign used exactly this approach). The `authenticationFailure` trap does *not* carry the source IP of the bad request — only the device that received it. To identify the scanner's IP, you need device-side logs, firewall logs, or network captures.

SEVERITY:
PAGE — Scanning targeting core infrastructure devices. High rate (> 100 auth failures/minute across the fleet). Active attack.
TICKET — Scanning detected at low rate and targeting edge devices. Could be automated vulnerability scanner.

THRESHOLDS:
- > 10 authentication failure traps from different devices within 5 minutes, all attributable to the same external source.
- Any auth failures from external (non-management-network) sources.
- Sudden spike in `usmStatsWrongDigests` on a fleet-wide scale.

FAILURE MODES DETECTED:
- Active SNMP scanning/reconnaissance
- Internal vulnerability scanner running SNMP checks
- Rogue device on the management network probing SNMP
- Compromised device being used as a pivot for SNMP scanning

NUANCES & GOTCHAS:
The `authenticationFailure` trap does NOT include the source IP of the bad request — only the device that received it. To identify the scanner, use device-side logs, firewall logs, or network captures. Internal vulnerability scanners (Qualys, Nessus, Rapid7) will trigger this signal legitimately. Maintain an allowlist of scanner IPs. SNMPv1 devices may not generate `authenticationFailure` traps at all, creating a blind spot. Scanning on UDP 161 (polling) vs UDP 162 (traps) are different attack vectors. Attackers scan 161 to read device config; scanning 162 is less common but could be used to inject fake traps.

CORRELATES WITH:
- **Device-side `snmpInBadCommunityNames` counters**: Confirms requests arriving at devices.
- **Firewall/ACL logs for UDP/161/162**: Identifies the scanner's IP and targeting pattern.
- **IDS/IPS alerts for SNMP-related signatures**.
- **SIEM correlation**: Multiple devices reporting auth failures from the same time window.

---

**SIGNAL: Trap Source IP Change (Source Pinning Drift)**

WHAT IT IS:
For a managed device, the IP that traps arrive from should match a configured trap-source. Drift indicates config change, renumber, or device behind NAT.

SOURCE:
- Receiver's per-source state (first-seen trap source IP)
- Device-side `show snmp` or equivalent
- Polling system for expected device IPs

HOW TO COLLECT IT MANUALLY:
```bash
# Compare expected trap source vs observed:
# Query the device for its trap-source configuration:
snmpget -v3 ... <device> ... some-vendor-specific-oid
# Or CLI: show snmp host (Cisco) / show snmp v3 trap-target (Juniper)

# Compare with what the receiver has been seeing:
grep <device_hostname> /var/log/snmptrapd.log | tail -5
```

WHAT IT TELLS YOU:
The device's outbound trap IP changed. May be benign (planned renumber, link failover to a different egress interface) or may indicate a routing change you were not informed of. If the device wasn't configured with `snmp-server trap-source`, the egress interface is determined by routing — and routing can change silently.

SEVERITY:
TICKET — Change detected, no change ticket open.
PLAN — Repeated drift.

THRESHOLDS:
- Any change in the trap source IP for a device whose trap-source was explicitly configured.
- If `snmp-server trap-source` was never configured, any change in the observed source IP is a signal of routing change.

FAILURE MODES DETECTED:
- Unsanctioned config change
- Default route change on device (different egress)
- VRF/interface change
- Device replaced without inventory update

NUANCES & GOTCHAS:
HA pairs with virtual IP can be expected to send from the virtual IP. Physical-IP flaps are not. If `snmp-server trap-source` was never configured, the device uses the egress interface of the route to the NMS, which can change silently. The observed source IP change may also indicate the device was replaced (new device, new IP) — a valid config state but a potential monitoring gap.

CORRELATES WITH:
- **Device config change traps**.
- **Routing change events**.
- **CMDB IP assignment records**.

---

### SECTION 6 — Monitoring Maturity Levels

Stages are sequential, not ranked. Each level subsumes the previous.

---

**LEVEL 1 — SURVIVAL**

The absolute minimum to know if the SNMP trap infrastructure is alive and not on fire.

- **Trap Receiver Process Liveness** — `snmptrapd` (or vendor daemon) is running, UDP port 162 socket is bound. Check every 60 seconds.
- **Synthetic Trap Heartbeat** — A known test trap is sent at fixed interval (1-5 minutes). Receiver logs the receipt. Alert on absence at the receiver log.
- **Traps Received Rate** — Nonzero log entries in the last 60 minutes. Alert on total silence for > 10 minutes during business hours.
- **UDP `RcvbufErrors`** — `/proc/net/snmp` drops counter for port 162. Alert on any increment.
- **Cold Start Traps from Production Devices** — Alert on any `coldStart` from a device not in a maintenance window.
- **Link Down Traps on Critical Interfaces** — Alert on any `linkDown` from interfaces designated as critical (backbones, uplinks, peer links).
- **Receiver Host Disk Space** — Alert at 80% full on the trap store volume.

---

**LEVEL 2 — OPERATIONAL**

What a competent team running SNMP traps in production monitors. A professional would be embarrassed to be missing these.

Everything in Level 1, plus:

- **Per-Source Trap Presence** — Per-device last-trap-received timestamp. Alert on per-source silence exceeding the per-source p99 inter-trap gap.
- **Source IP Allowlist** — Alert on traps from non-inventoried IPs.
- **Trap Source IP Cardinality** — Alert on cardinality drop (missing devices) or spike (new unknown sources).
- **Per-Source Trap Rate** — Top-talker analysis. Alert on single-source dominance.
- **Per-Trap-Type Resolution Tracking** — Count of unresolved OID traps. Alert on non-zero.
- **SNMPv3 USM Statistics** — Monitor all six USM counters. Alert on any sustained increase without a known cause.
- **SNMPv3 Engine Boots/Time** — Track per-device engine boots. Alert on unexpected boots increments.
- **`authenticationFailure` Trap Rate** — Track per source. Investigate spikes.
- **BGP/OSPF Peer State Traps** — Alert on BGP session down and OSPF adjacency loss on core links.
- **Environmental Traps** — Alert on temperature threshold, power supply failure, fan failure.
- **Config Change Traps** — Cross-reference with change tickets. Alert on uncorrelated changes.
- **Trap Processing Latency (End-to-End)** — Instrument the full pipeline. Alert on latency > 10 seconds sustained.
- **Trap Receiver CPU and Memory** — Process-level resource utilization. Capacity planning.
- **UDP Receive Buffer Depth (Recv-Q)** — Track for port 162. Leading indicator.
- **SNMP Agent Reachability (Polling Complement)** — Distinguish device-level failure from trap-path-specific failure.
- **NTP Offset on Receiver Host** — Alert on > 1 second skew.
- **MIB Coverage Audit** — Track and alert on unknown OID rate. Process for updating MIBs after firmware upgrades or new device deployments.

---

**LEVEL 3 — MATRE**

Full coverage: internals, leading indicators, composite patterns. Senior SRE instrumentation.

Everything in Levels 1-2, plus:

- **Composite Failure Pattern Detection** — Automated detection of trap storm cascade, SNMPv3 lockouts, flapping floods, silent partitions, credential rotation outages, MIB gap blindness, rogue injection.
- **End-to-End Synthetic Trap to Downstream Consumer** — Synthetic trap asserts arrival at the SIEM, not just the receiver. Catches forwarder failures.
- **Trap Storm Suppression / Rate Limiting** — Either at the source (per-device trap rate limiting) or at the receiver (per-source rate limiting with alerting).
- **SNMPv3 USM Full Spectrum Monitoring** — Per-USM-counter thresholds, time-window violation tracking, unknown engine ID tracking.
- **Forwarding Latency Monitoring** — Track time from trap receipt to SIEM ingestion. Alert on > 5 minutes for security-relevant traps.
- **Downstream Pipeline Backlog** — Monitor consumer lag (Kafka), queue depth (RabbitMQ), or spool file count.
- **Per-Trap Decode Latency** — Profile per-trap processing time. Alert on rising per-trap cost (MIB set growth, cache degradation).
- **Config Change Trap Automated Correlation** — Automated cross-reference with change management system. Alert on uncorrelated changes within 60 seconds.
- **Capacity Trending** — CPU, memory, disk, buffer, and queue depth trends over weeks/months with linear extrapolation.
- **Redundant Receiver Health** — If using hierarchical/distributed receivers, monitor inter-receiver forwarding health and failover readiness.
- **Environmental Trap Severity Classification** — Differentiate warning vs critical thresholds. Correlate with facility/DCIM alerts.
- **Filter Rule Audit** — List active filters, last-modified date, hit count. Alert on filters that have been in place > 90 days.
- **Inform Acknowledgment Rate** (if using Informs) — Alert on acknowledgment failure rate > 5%.
- **Receiver Disk Inode Tracking** — In addition to byte utilization.
- **Multi-Receiver Trap Count Consistency** — If HA, alert on > 5% divergence between receivers.

---

**LEVEL 4 — EXPERT**

The deep, often-missed signals that experienced operators add after their third or fourth major incident with this stack.

Everything in Levels 1-3, plus:

- **SNMPv3 Credential Rotation Zero-Trap-Loss Monitoring** — Track per-device trap flow during and after credential rotation. Detect any device that stops sending traps post-rotation within 60 seconds. Automated rollback on detection.
- **Engine Time Drift Prediction** — Monitor the rate of engine time divergence between devices and receiver. Predict `usmStatsNotInTimeWindows` before it happens. Preemptively trigger engine time re-discovery for drifting devices.
- **Trap Payload Anomaly Detection** — Baseline normal trap variable bindings per device type and trap OID. Alert on unusual values (e.g., BGP peer state change to an unexpected peer, interface state change on an interface that should be stable).
- **Trap Volume Prediction** — Model trap volume based on time-of-day, day-of-week, maintenance windows, and historical patterns. Predict trap storms from scheduled events (mass device reboot). Pre-scale receiver capacity before predicted storms.
- **Device-Side Trap Generation Rate vs Receiver-Side Receipt Rate** — Poll device SNMP agents for their trap generation counters (where exposed). Compare with receiver-side counts for the same device. The delta estimates in-transit loss. This is the only way to quantify UDP trap delivery reliability end-to-end.
- **Trap-to-Syslog Correlation Automation** — Automatically correlate traps with syslog entries from the same device within a time window. Alert on events that appear in one stream but not the other (detection coverage gap).
- **Streaming Telemetry SNMP Trap Equivalence Tracking** — For devices that support both, verify that equivalent events are reported in both systems. Track telemetry-to-trap conversion accuracy.
- **Trap Receiver Load Testing** — Regular load tests at 2× and 5× normal peak trap volume. Verify pipeline capacity, measure latency under load, identify breaking points. Update capacity models based on results.
- **Unauthorized Trap Source Behavioral Analysis** — Beyond checking against inventory, analyze trap patterns from sources. Rogue devices may send traps with unusual OID patterns, variable binding values, or timing that doesn't match legitimate device behavior.
- **Historical Trap Pattern Analysis for Predictive Maintenance** — Analyze historical trap patterns to identify devices that are degrading (increasing environmental traps, increasing interface error traps, increasing protocol flap traps) before they fail. Feed into preventive maintenance scheduling.
- **Per-Source Synthetic Trap** — A synthetic test trap generated from each managed device (where supported), asserting end-to-end delivery to the consumer. The only way to verify the path from each device.
- **SNMPv3 USM Engine ID Change Detection** — Alert on any v3 source's engine ID changing (factory reset, device replacement). Triggers automatic re-provisioning workflow.
- **Trap Varbind Deep Inspection** — Specific varbinds (`bgpPeerRemoteAddr`, `ifAdminStatus`, `ospfNbrState`) are extracted and normalized into structured fields for query and correlation. Enables cross-vendor trap querying.
- **MIB Dependency Graph Tracking** — When a MIB is updated, dependent MIBs are tracked. A MIB pack update that breaks decode for an existing OID is caught before production.
- **CVE-Aware Trap Receiver Hardening** — Continuous monitoring of CVE databases (NVD, vendor PSIRT) for new SNMP vulnerabilities affecting the receiver daemon. Automated patching cadence for `snmptrapd` and equivalent. Specifically watch for buffer overflows in trap parsing paths (CVE-2025-68615 class) and remote DoS in trap reception paths.
- **MIB Pack Pre-Compilation** — Convert MIBs to flattened JSON indexes at deploy time (PySMI-style). Reduces per-trap MIB lookup cost from O(n²) to O(log n). Becomes essential at > 200 MIB file count.

---

### SECTION 7 — What Most Teams Get Wrong

Based on postmortem patterns across the industry: what signals are systematically under-monitored, what gaps repeatedly cause incidents. Ordered by damage × commonness.

---

**1. Not monitoring the trap pipeline itself (receiver health, drops, latency).**

Teams instrument their network devices extensively but treat the trap receiver as infrastructure plumbing — assumed to work. The receiver can be dropping traps silently for days before anyone notices. The first indication is often a missed alert during an incident — the very scenario the monitoring was supposed to prevent.

*Incident pattern this would have caught:* A trap receiver process becomes memory-constrained and slows down. UDP buffer overflows cause thousands of dropped traps. A critical BGP session drops on a backbone router; the trap is generated by the device, but dropped by the receiver. The NOC doesn't get paged. The outage is discovered 15 minutes later by customer complaints. Empirically: this is the exact pattern that produced > 10,000 alerts in minutes at Alibaba Cloud, where the SNMP congestion signal was buried under downstream alert flood.

*Why teams miss it:* Monitoring the monitoring system feels meta and self-referential. Teams assume the NMS vendor handles this. The signals (`RcvbufErrors`, queue depth, processing latency) aren't in the default dashboards of most NMS platforms. `snmptrapd` has no native internal queue metric; the gap is a known operational blind spot.

---

**2. Relying on traps without polling confirmation (traps as sole source of truth).**

Teams configure devices to send traps and build all their alerting on trap-based rules. They don't complement traps with SNMP polling. Because traps are UDP and unreliable, they miss events during network congestion, partition, or receiver stress. The team has false confidence because "we have traps configured for everything."

*Incident pattern this would have caught:* A device's SNMP agent crashes (but the device continues forwarding traffic). The agent stops generating traps. A critical interface goes down on that device. No trap is sent. The NMS doesn't detect the failure because it relies entirely on traps. Polling `ifOperStatus` would have detected the interface down within the next poll cycle.

*Why teams miss it:* Traps feel sufficient because they work most of the time. The unreliability is invisible — there's no signal that says "a trap was supposed to arrive but didn't." Teams don't experience the gap until a major incident reveals it.

---

**3. Ignoring SNMPv3 USM statistics until a credential rotation goes wrong.**

Teams deploy SNMPv3 (good) but don't monitor USM statistics (`usmStatsWrongDigests`, `usmStatsNotInTimeWindows`, etc.). During a credential rotation, something goes wrong. Traps from affected devices silently fail USM authentication. No one notices until they realize they haven't received traps from those devices in hours or days.

*Incident pattern this would have caught:* A monthly SNMPv3 credential rotation script updates 500 devices. 50 devices fail to apply the new credentials (script timeout, device unreachable during rotation). Those 50 devices continue sending traps with old credentials. `usmStatsWrongDigests` on the receiver climbs rapidly but no one is watching it. 12 hours later, a critical event occurs on one of the 50 devices. The trap is sent but rejected by USM. No alert fires.

*Why teams miss it:* USM statistics require querying the receiver's own SNMP agent or parsing application logs — it's not in the standard trap processing pipeline. The counters are cumulative since boot, requiring delta tracking. The failure is invisible in normal dashboards.

---

**4. Not handling trap deduplication vs. legitimate event volume correctly.**

Teams configure deduplication to suppress alert fatigue. They set aggressive thresholds (suppress more than 3 `linkDown` traps per minute from a device). During a real incident (line card failure, power supply issue affecting many interfaces), the dedup engine suppresses the legitimate flood. The operator sees "normal" trap volume and doesn't realize a major event is occurring.

*Incident pattern this would have caught:* A line card in a core chassis switch fails. All 48 interfaces on the card go down simultaneously. The switch generates 48 `linkDown` traps in rapid succession. The dedup engine suppresses 45 of them. The operator sees 3 `linkDown` alerts and doesn't recognize the severity. 45 interfaces are down, including several backbone links. The full scope isn't apparent for 20 minutes.

*Why teams miss it:* Dedup configuration feels like a tuning exercise, not a safety concern. Teams optimize for noise reduction without considering the "storm of legitimate events" scenario. The dedup suppression count is rarely monitored or alerted on.

---

**5. Missing MIB coverage for critical trap types (MIB gap blindness).**

Teams load standard MIBs (IF-MIB, SNMPv2-MIB) and a few vendor MIBs when they set up the NMS. Over time, new device types are deployed, firmware is upgraded, and new vendor MIBs are needed. No one updates the MIB library. Traps from the new device types arrive as raw numeric OIDs, don't match alerting rules, and are logged but not actionable.

*Incident pattern this would have caught:* A new generation of data center switches is deployed. The new switches support a hardware failure trap type not in the old MIB. A hardware failure occurs on a switch. The trap is sent with an OID not in the receiver's MIB library. The trap is logged as an unknown OID. No alert fires. The switch fails silently, taking down a rack of servers.

*Why teams miss it:* MIB management is a maintenance task with no immediate feedback. MIBs compile successfully for the devices you have today. The gap only appears when a new device type or firmware version sends a trap with an OID that isn't in the MIBs. No one audits MIB coverage against the device inventory.

---

**6. Not monitoring trap silence (absence of expected traps).**

Teams alert on trap arrivals (positive signals: `linkDown`, `coldStart`, BGP peer down) but don't monitor trap absence (negative signal: devices that should be sending periodic traps but aren't). A device can stop sending traps due to configuration change, SNMP agent failure, or network path issue, and the NMS doesn't notice because there's no "trap not received" alert.

*Incident pattern this would have caught:* A firewall rule update blocks UDP/162 from a remote site's management subnet. Traps from all devices at that site stop arriving. The NMS doesn't alert — it only alerts on received traps, not missing ones. Two days later, a critical device at that site fails. No trap is sent (the device is down) and no one is watching for the absence of traps.

*Why teams miss it:* Monitoring absence requires maintaining a list of expected trap sources and their expected intervals, then continuously comparing actual against expected. This requires CMDB integration and baseline management — significantly more complex than alerting on arrived traps.

---

**7. Treating trap storms as the problem instead of the symptom.**

When a trap storm hits, teams focus on "fixing" the storm (increasing buffer sizes, adding dedup, restarting the receiver) instead of investigating what caused the storm. The storm is a symptom of a network event. Treating the symptom masks the underlying problem, which continues to generate traps (suppressed by the dedup) and may represent a real infrastructure failure.

*Incident pattern this would have caught:* A spanning-tree misconfiguration causes a topology change storm. Thousands of traps flood the receiver. The team increases buffer sizes and adds dedup rules to suppress the "noise." The storm is suppressed, and the NMS returns to normal. But the spanning-tree misconfiguration persists — it's still causing suboptimal forwarding and intermittent loops. The team doesn't investigate the root cause because "the trap storm is fixed."

*Why teams miss it:* Trap storms are stressful and create urgency to "stop the bleeding." Dedup and throttling provide immediate relief. Root cause investigation takes longer and requires understanding the network event that triggered the storm. The operational incentive is to suppress the symptom, not diagnose the cause.

---

**8. No correlation between traps, syslog, and polling data.**

Teams operate SNMP traps, syslog collection, and SNMP polling as independent monitoring streams. During incidents, operators manually correlate between these systems. This manual correlation is slow, error-prone, and often incomplete. Events that appear in one stream but not another are missed.

*Incident pattern this would have caught:* A BGP session drops. The device sends a `bgpBackwardTransition` trap AND generates a syslog message with detailed diagnostic information. The operator sees the trap alert but doesn't check syslog. The trap only says "peer state changed to Idle" — not why. The root cause (MTU mismatch on a newly configured interface) is in the syslog message. Without correlation, the operator focuses on the BGP session instead of the underlying interface MTU issue.

*Why teams miss it:* Traps, syslog, and polling are often managed by different tools or different teams. Correlation requires integration effort. The value isn't apparent until an incident where correlation would have accelerated diagnosis.

---

**9. SNMPv3 engine time issues after device reboots not handled automatically.**

Teams deploy SNMPv3 but don't handle the engine time re-discovery process that must occur after every device reboot. When a device reboots, its `snmpEngineTime` resets, and the receiver must re-learn the new engine time. If this process fails, the device enters a permanent trap lockout. Teams don't detect this because they don't monitor per-device trap flow or USM statistics.

*Incident pattern this would have caught:* A core router reboots due to a firmware crash. It comes back up in 2 minutes. The trap receiver attempts engine time re-discovery, but the discovery packet is lost. The receiver caches the old engine time. All subsequent traps from the router are rejected with `usmStatsNotInTimeWindows`. The router is up and running, but the NMS receives no traps from it. A subsequent interface failure on the router is invisible to trap-based alerting.

*Why teams miss it:* Engine time re-discovery is an automatic process that usually works. When it fails, there's no explicit error — the device just stops appearing in trap sources. Teams don't monitor per-device SNMPv3 trap flow or USM statistics, so they don't notice the gap.

---

**10. Running SNMPv2c with default or shared community strings on management networks.**

SNMPv1 and v2c carry the community string in cleartext on the wire. Default community strings (`public`, `private`) are still common in production. A scanner can discover community strings and gain read or read-write access to the entire fleet. The most recent and severe exploitation of this was CVE-2025-20352 (September 2025), where attackers used default `public` strings on Cisco switches to achieve RCE as root and deploy persistent Linux rootkits in the "Operation Zero Disco" campaign. Up to 2 million devices were potentially affected.

*Incident pattern this would have caught:* Direct device compromise from outside the trusted management network (in CVE-2025-20352, via the exposed `public` string) leading to BGP route manipulation, ACL modification, or full configuration disclosure.

*Why teams miss it:* SNMPv3 trap configuration is significantly harder than v2c. Many devices have incomplete v3 support for traps specifically. Teams accept the risk without quantifying it. "It's always been v2c" is the institutional excuse. The fix is v3 migration, network isolation with strict ACLs as an interim, and removal of default community strings.

---

**11. No end-to-end synthetic trap monitoring.**

Teams check that the receiver process is up, that traps are being received, that the SIEM is ingesting. They never run a synthetic trap end-to-end that asserts arrival at the *consumer*, not just the receiver. The forwarder between receiver and SIEM can be broken while every component-level signal says "healthy."

*Incident pattern this would have caught:* A receiver change or SIEM schema update breaks the forwarder. The receiver logs traps. The SIEM's ingestion pipeline silently drops them (schema mismatch, field length limit, index routing error). A critical BGP flap trap is ingested into the receiver's log but never reaches the SIEM. The alert rule matches on the old field name and never fires. The BGP outage goes undetected for 20 minutes until customers report.

*Why teams miss it:* The trap receiver team and the SIEM/alerting team are often different people or different organizations. Each team monitors their own component. Nobody owns the end-to-end path.

---

**12. No patching discipline for the trap receiver itself.**

The trap receiver is a software daemon running on a host. Like any software, it has vulnerabilities. The most recent examples: CVE-2025-68615 (Net-SNMP `snmptrapd` buffer overflow, December 2025, remote unauthenticated DoS) and CVE-2025-20352 (Cisco IOS/IOS XE SNMP subsystem, September 2025, actively exploited RCE). Teams instrument the network but forget to patch the receiver. When a vulnerability is disclosed, the receiver is exposed until the next maintenance window — potentially months.

*Incident pattern this would have caught:* A disclosed `snmptrapd` CVE is exploited by an attacker who sends a crafted UDP/162 packet. The daemon crashes. All traps stop flowing. The team is unaware until the next routine monitoring check.

*Why teams miss it:* The receiver is treated as "infrastructure plumbing" that doesn't need regular patching. CVE monitoring focuses on customer-facing systems and network devices, not the monitoring infrastructure itself. The fix is continuous CVE monitoring (NVD, vendor PSIRT) and a regular patching cadence for the receiver.

---

### §8 — Validation Trail (Master-Only)

#### Per-advisor profile

**Advisor acacb945c66a (ops-playbook-distill-glm).** Most thorough on the receiver internals — explicit coverage of `RcvbufErrors`, `Recv-Q`, per-trap processing time, MIB resolution cache, FD usage, and disk/inode monitoring. Strong on USM statistics (all six counters, with OIDs). Strong on deployment variants (standalone, distributed, on-prem, cloud, commercial vs open-source). Tendency: claims that `snmptrapd` exposes a "trap processing queue depth" metric that the source code does not actually implement — the daemon is synchronous with handler chains, not a queue-based architecture. The signal as a *concept* is valid (backpressure exists) but the *source* claim is unsupported. Several items flagged `<!-- TODO: verify -->` including the time window (verified by online search at 150 seconds), kernel buffer recommendations (verified), and OID accuracy for vendor-specific traps (flagged as TODO). Overall operational depth: high; bias toward comprehensive but with some overconfidence in source claims that needed online correction.

**Advisor cc537cd5c41b (ops-playbook-distill-kimi).** Strong on the protocol-stack mental model (positioning traps vs polling vs syslog vs streaming telemetry). Good on gNMI vs traps context. Includes device-side `snmpOutTraps` mismatch detection as a leading-indicator concept. Strong on failure archetype enumeration with real-world device behaviors. Weak on specific OIDs and exact commands for kernel counter collection. Tends toward conceptual description rather than specific commands. Several `<!-- TODO: verify -->` items left for online validation. Bias: more strategic/architectural than tactical.

**Advisor f8da5b2001f3 (ops-playbook-distill-mimo).** Strongest on the position in modern enterprise NPM (gNMI hybrid posture, streaming telemetry complement). Good on interaction with adjacent systems (NetFlow, syslog, ICMP synthetic, SIEM). Comprehensive failure patterns including "Receiver Process Death" with detailed OOM crash documentation. Good on interaction with adjacent systems in §4. Claims `snmptrapd` is "single-threaded or poorly parallelized" — confirmed by source code analysis, `snmptrapd` is indeed single-threaded for trap processing. MIB resolution cost claims ("Loading 500 MIB files doesn't cost 10x more than 50 — it can cost 100x more") partially supported by the O(n²) net-snmp `binary_array` issue. Bias: balanced and operational; strongest on the "what most teams get wrong" section with concrete incident patterns.

**Advisor 380bcabcf0a1 (ops-playbook-distill-qwen).** More concise overall. Strong on specific device vendor signal mappings (Cisco `ciscoEnvMon`, `cpmCPUTotal`, etc.). Good on positioning traps vs polling for availability checks. Includes DHCP snooping and NTP state change as security-relevant traps — these are useful additions. Some duplicate content with other advisors. Bias: practical and vendor-specific; less depth on internal receiver architecture and less coverage of USM/USM-statistics monitoring.

**Advisor cf34759885cd (ops-playbook-distill-minimax).** Most comprehensive on the "modern enterprise" framing — explicit hybrid architecture, vendor SaaS distinctions, RFC 3411 engine ID format details, NIST BGP-SRx and RPKI for hijack detection, ARTEMIS tool reference, snmpsim/MIMIC for synthetic testing. Strongest on BGP security signal (MD5 mismatch, backward-with-no-return). Good on NetFlow/sFlow correlation methodology with two-pillars framing. Claims MIB resolution cost is "100x" for 500 MIB files — partially supported (O(n²) net-snmp issue confirmed). Several `<!-- TODO: verify -->` items left for online validation. Most structured on Maturity Levels and the "What Most Teams Get Wrong" section. Bias: most architecturally complete.

#### Discrepancies identified and resolved

1. **Default UDP receive buffer size.** Advisor 1 cited "8-16MB" as recommended. Online search confirmed Linux default is 212,992 bytes (208 KiB); production recommended is 8-32 MB. The recommended range was kept; the default was added to the resource description in §3. The "8-16MB" range is within the validated 8-32 MB envelope.

2. **SNMPv3 USM time window value.** Advisor 1 flagged `<!-- TODO: verify 150 seconds -->`. RFC 3414 confirmed the ±150-second default. Adopted and unflagged.

3. **Source IP ACL on management VLAN vs receiver-side allowlist.** Advisors 1, 2, 3, 4, 5 all include source IP allowlisting as a security signal. Some position it as primarily receiver-side; others emphasize network-side enforcement. Synthesis adopts both: receiver-side for detection, network-side for prevention.

4. **BGP4-MIB trap OIDs (legacy vs current).** Advisor 1 cited legacy OIDs (`.1.3.6.1.2.1.15.7.1` for `bgpEstablished` and `.1.3.6.1.2.1.15.7.2` for `bgpBackwardTransition`). RFC 4273 confirmed these are deprecated; the current OIDs are `.1.3.6.1.2.1.15.0.1` and `.1.3.6.1.2.1.15.0.2` (`bgpEstablishedNotification` and `bgpBackwardTransNotification`). Synthesis lists both with current noted as the RFC 4273 replacement.

5. **Trap processing queue depth as a signal.** All five advisors describe a "Trap Processing Queue Depth" or "Trap Queue / Backlog Depth" signal. Online research (source code analysis of `snmptrapd` and the MySQL queue plugin) confirmed that `snmptrapd` itself has no native internal queue depth metric. The signal as a *concept* is valid — the downstream pipeline (Kafka consumer lag, RabbitMQ queue depth) and the *kernel-level* signals (`Recv-Q`, `RcvbufErrors`) provide the same operational insight. Synthesis preserves the concept but rewrites the source to reflect the actual observable sources: downstream consumer lag for queued backends, or `Recv-Q` + `RcvbufErrors` + drop count for the receive-buffer-level view. `<!-- TODO: verify <claim> -->` applied to the specific claim that `snmptrapd` exposes a queue depth metric — this is a known instrumentation gap.

6. **Trap storm document volume and root-cause concealment.** Advisor 1's "Trap Storm Cascade" pattern claimed traps are lost first. The Alibaba Cloud SkyNet SIGCOMM 2025 paper validates this with quantitative data: a single cable failure produced > 10,000 alerts within minutes, the SNMP congestion signal was buried, recovery was prolonged by hours. Adopted the pattern with the empirical scale included.

7. **SNMPv3 Inform behavior after reboot.** All five advisors note v3 issues post-reboot. Online research (Zabbix JIRA, Fortinet community, net-snmp GitHub) confirmed the bidirectional nature of engine ID discovery: a manager-initiated GET triggers a Report PDU that refreshes the cache, which is the operational recovery mechanism. Adopted.

8. **CVE-2025-20352 active exploitation.** The recent Cisco SNMP RCE campaign (Operation Zero Disco) was not in any advisor's output. Included in §4 and §5 as a concrete example of the v1/v2c cleartext exposure risk and the SNMP receiver as an attack surface.

#### Unique-advisor claims validated

1. **SkyNet SIGCOMM 2025 quantitative case study (Alibaba Cloud, > 10,000 alerts in minutes from a single cable failure).** Cited by none of the advisors. Online search found the paper (Bo Yang et al., SIGCOMM 2025, https://ennanzhai.github.io/pub/sigcomm25-skynet.pdf). Validated and incorporated into the Trap Storm Cascade composite pattern with quantitative scale.

2. **The "two-pillars" framing for NetFlow/SNMP correlation (from NetFlow Logic).** Not cited by any advisor in exactly this framing. Validated online and used in §4 adjacent-systems interaction as the conceptual model.

3. **NNMi's dual-node `nnmtrapreceiver` active-active HA pattern** (both nodes run `nnmtrapreceiver` simultaneously). Advisor 1 noted this pattern. Online search confirmed it. Adopted in §0 deployment variants.

4. **CVE-2025-20352 "Operation Zero Disco" campaign** (Cisco SNMP RCE, September 2025, default `public` community string, Linux rootkit deployment, up to 2M devices affected). Not in any advisor's output. Validated online via Trend Micro, The Hacker News, Eclypsium. Included in §4 and §5 as a concrete real-world example.

5. **CVE-2025-68615 (Net-SNMP `snmptrapd` buffer overflow, December 2025, remote unauthenticated DoS).** Not in any advisor's output. Validated online via NVD, net-snmp GitHub Security Advisory. Included in §0 (deployment variants), §4 (known instrumentation limitations — the receiver is itself an attack surface), and §7 (no patching discipline for the trap receiver itself).

6. **"5 identical traps in 30 seconds = storm" threshold (HPE OneView).** Advisor 1's storm pattern referenced threshold-based detection. Online search confirmed HPE OneView uses 3 identical traps within 30 seconds as the storm detection trigger. Not adopted in synthesis (used different relational thresholds) but documented as a vendor reference.

7. **NNMi hard limits: 90,000 trapped messages = Warning, 95,000 = Major, 100,000 = receiver ceases accepting (Broadcom/VMware Smart Assurance).** Not in any advisor's output. Validated online. Included in §0 context for receiver saturation thresholds.

8. **Broadcom CA Spectrum's per-IP trap storm isolation** (SpectroSERVER blocks trap processing from offending device but allows other devices' traps to continue). Not in any advisor's output. Validated online. This is a more sophisticated isolation pattern than the simple "throttle or block" approach some advisors implied.

9. **The "two-pillars" NetFlow/SNMP correlation two-pillar framework.** Adopted in §4 as the conceptual model for trap-flow interaction.

10. **Receiver-side drop count for "Auth Failure Trap Storm" from device-reported failures.** Advisor 1 included the per-receiver distinction between device-reported authFailure and receiver-reported USM errors. Validated and adopted.

11. **Linux `net.core.rmem_max` default of 212,992 bytes (208 KiB).** Confirmed by online search (Red Hat RHEL 10 documentation, Baeldung). Included in §3 as a resource constraint that makes the default unsuitable for production.

#### Apparent gaps filled online

1. **SNMP-FRAMEWORK-MIB engine ID/boots/time OIDs.** Validated: `snmpEngineID` at `.1.3.6.1.6.3.10.2.1.1`, `snmpEngineBoots` at `.1.3.6.1.6.3.10.2.1.2`, `snmpEngineTime` at `.1.3.6.1.6.3.10.2.1.3`. Included in §1 signal.

2. **snmptrapd `-D usm,traps` debug logging syntax for USM troubleshooting.** Validated online (Zabbix forum, man page). Included in §1 signal "How to Collect It Manually."

3. **BGP4-MIB OSPF-TRAP-MIB exact OIDs.** Validated: `ospfNbrStateChange` at `.1.3.6.1.2.1.14.16.2.2`. Included.

4. **The SkyNet paper's quantitative data on cable-failure incidents and the alert-volume-vs-diagnostic-utility inverse relationship.** Validated and included as a concrete example in the Trap Storm Cascade pattern.

5. **Recent CVE landscape for SNMP** (CVE-2025-20352, CVE-2025-68615). Validated and included in §4 and §7.

6. **CISA, Rapid7, Black Hills Information Security guidance on SNMPv2c cleartext exposure.** Validated and used to support the §5 security signals and the "no patching discipline" §7 blind spot.

7. **Synthetic trap testing tools** (`snmpsim`, MIMIC, Trishul-SNMP, SimpleTesterPro, LoriotPro Trap Simulator). Validated and referenced in the synthetic trap heartbeat signal and the §6 maturity levels.

8. **HA receiver patterns** (Cisco's three-solution framework: multiple receivers / forwarding chain / repeater-multiplexer; Zabbix proxy group workaround with Keepalived + Docker trap duplication; OpenNMS Pacemaker/Corosync; SolarWinds HA pool). Validated and included in §0 deployment variants.

9. **gNMI vs traps positioning for modern enterprises** (confirmed hybrid is the 2024-2026 consensus; tools in active use include LibreNMS, Zabbix, PRTG, SolarWinds, LogicMonitor for SNMP, with Telegraf + Kafka for telemetry pipelines). Validated and used in §0 mental model.

10. **NIST BGP-SRx and RPKI as complementary signals to BGP trap security events.** Mentioned briefly in §5 as the layered detection approach.

#### Searches performed

| Query | Yield |
|---|---|
| SNMP-USER-BASED-SM-MIB usmStats OIDs | High — confirmed all six OIDs across MIB repositories |
| SNMPv3 USM time window RFC 3414 | High — confirmed 150-second default directly from RFC |
| Linux net.core.rmem_max default | High — confirmed 212,992 bytes (208 KiB) from RHEL 10 docs and Baeldung |
| SNMPv2-MIB standard trap OIDs | High — confirmed all five OIDs from Net-SNMP and Observium |
| BGP4-MIB / OSPF-TRAP-MIB OIDs | High — confirmed OIDs including deprecated vs current replacements |
| /proc/net/snmp Udp RcvbufErrors monitoring | High — confirmed methodology, root causes, eBPF tools (Cloudflare's `udp_rcvbuf_errors.py`) |
| gNMI streaming telemetry vs SNMP traps 2024-2025 | High — confirmed hybrid posture, tools, vendor support |
| SNMP trap storm incident case study | High — found SIGCOMM 2025 SkyNet paper, Broadcom/NNMi/HPE/CA documentation |
| snmptrapd internal queue depth metrics | High — confirmed NO native queue metric; synchronous handler chain architecture |
| SNMPv3 engine ID discovery after device reboot | High — confirmed bidirectional requirement, F5 bug, recovery mechanisms |
| NetFlow sFlow IPFIX correlation with traps | High — confirmed two-pillars framework, MTTR improvements |
| SNMPv2c cleartext Wireshark attack | High — confirmed cleartext exposure, Wireshark procedure, exploitation chain |
| BGP route hijack MD5 mismatch detection | High — confirmed vendor-specific log patterns, ARTEMIS, RPKI |
| MIB resolution OID lookup performance scaling | Medium — confirmed O(n²) net-snmp issue; 500-MIB specific metrics extrapolated |
| SNMPv3 engineBoots engineTime format | High — confirmed RFC 3411 format types, OIDs, syntax |
| SNMP SET security risk write community exploit | High — confirmed exploitation chain, NET-SNMP-EXTEND-MIB RCE, CVE-2025-20352 |
| Synthetic SNMP trap testing heartbeat | High — confirmed tools (snmpsim, MIMIC, Trishul), patterns, Netcraftsmen UDP analysis |
| HA SNMP trap receiver deployment | High — confirmed Cisco three-solution framework, Zabbix workaround, OpenNMS/SolarWinds patterns |
| CVE 2024 2025 SNMP vulnerability Cisco Juniper RCE | High — confirmed CVE-2025-20352 (Cisco, actively exploited), CVE-2025-68615 (Net-SNMP), CISA KEV |
| SNMPv3 InformRequest vs trap reliability | High — confirmed RFC mechanics, retry behavior, engineID handling differences |
| snmptrapd debug logging for USM auth failures | High — confirmed `-D usm,traps` syntax and expected output patterns |

#### URLs fetched (representative, highest-relevance)

- `https://www.rfc-editor.org/rfc/rfc3414` — USM specification, time window
- `https://datatracker.ietf.org/doc/html/rfc4273` — BGP4-MIB current and deprecated OIDs
- `https://ennanzhai.github.io/pub/sigcomm25-skynet.pdf` — SkyNet SIGCOMM 2025 paper, quantitative trap storm data
- `https://docs.redhat.com/en/documentation/red_hat_enterprise_linux/10/html/network_troubleshooting_and_performance_tuning/tuning-udp-connections` — `rmem_max` default
- `https://support.zabbix.com/si/jira.issueviews:issue-html/ZBX-8385/ZBX-8385.html` — USM time window analysis, affected devices
- `https://knowledge.broadcom.com/external/article/304140/to-prevent-domain-crashes-caused-by-trap.html` — NMS receiver OOM crash mechanism
- `https://trendmicro.com/en_us/research/25/j/operation-zero-disco-cisco-snmp-vulnerability-exploit.html` — Operation Zero Disco analysis
- `https://www.cisa.gov/news-events/alerts/2017/06/05/reducing-risk-snmp-abuse` — SNMP security guidance
- `https://www.blackhillsinfosec.com/snmp-strings-attached/` — `private` community string RCE via NET-SNMP-EXTEND-MIB
- `https://github.com/iovisor/bcc/pull/2143/files` — eBPF tool for per-socket UDP RcvbufErrors attribution
- `https://netflowlogic.com/the-two-pillars-of-incident-response-correlating-real-time-flow-data-with-device-health-snmp/` — NetFlow/SNMP correlation framework
- `https://www.auvik.com/franklyit/blog/snmp-traps/` — synthetic testing and heartbeat patterns
- `https://www.cisco.com/en/US/technologies/tk869/tk769/white_paper_c11-449655.html` — Cisco HA patterns for SNMP traps

#### Judgment calls

1. **`snmptrapd` queue depth signal handling.** All five advisors describe this signal with claims that the daemon exposes an internal queue depth metric. Online source-code analysis confirmed this is not the case for `snmptrapd` (the daemon is synchronous with handler chains). Rather than reject the concept, I rewrote the signal to use observable proxies: downstream consumer lag (Kafka/RabbitMQ) for the pipeline-level view, and `Recv-Q` + `RcvbufErrors` + application-level drop count for the receive-buffer-level view. This preserves the operational value of the signal (detecting backpressure) while being honest about what `snmptrapd` actually exposes. The `<!-- TODO: verify -->` flag in the signal description acknowledges the gap.

2. **BGP OID choice (current vs deprecated).** The deprecated OIDs (`.1.3.6.1.2.1.15.7.1` and `.1.3.6.1.2.1.15.7.2`) are still in widespread use because many devices haven't been updated to the RFC 4273 replacements. I listed both with clear current/deprecated status. This is consistent with the practical reality that operators see both.

3. **Severity thresholds.** I used the relational threshold approach required by C2 (ratios, rates of change, baseline deviations) rather than absolute numbers. Where a specific value is universally true (e.g., RFC 3414's 150-second time window), I stated it; where it's workload-dependent (e.g., "how many traps per minute constitutes a storm"), I gave the relational form and explained the workload dependency.

4. **Engagement with vendor MIBs.** I kept vendor-specific trap OIDs (CISCO-ENVMON-MIB, etc.) at a general level rather than attempting to enumerate every vendor's MIB, because vendor MIBs vary by platform, model, and firmware version. I included the OID names and noted that exact OIDs and trap names vary by vendor. The generic signals (temperature, power, fan, link, BGP) are the universal patterns.

5. **Inclusion of recent CVEs.** CVE-2025-20352 and CVE-2025-68615 are not in any advisor's output. I included them in §4 (adjacent systems / known instrumentation limitations), §5 (security context), and §7 (no patching discipline) because they are current, concrete, and validate the structural vulnerabilities the playbook warns about.

6. **The "two-pillars" framing for NetFlow/SNMP correlation.** This is a community framing (NetFlow Logic, Akips, Progress Flowmon) rather than a single authoritative source, but it captures the practical reality well: traps tell you *that* an event happened; flow tells you *what traffic* it affected. I adopted the framing for §4 adjacent systems.

#### Unresolved uncertainties

The following items are flagged as `<!-- TODO: verify <claim> -->` in the body. The audit trail here names them:

- `<!-- TODO: verify the default Linux rmem_max and typical SNMP socket buffer settings -->` (Advisor 1's TODO). **Resolved** by online search (Red Hat RHEL 10 docs confirm 212,992 bytes default; recommended 8-32 MB for production trap receivers). Flag removed.
- `<!-- TODO: verify exact window -->` for the 150-second time window (Advisor 1's TODO). **Resolved** by RFC 3414. Flag removed.
- `<!-- TODO: verify <claim> that snmptrapd exposes a trap processing queue depth metric -->` — based on source code analysis, this is **NOT** supported for `snmptrapd` itself. The signal as a concept is valid (backpressure observable via `Recv-Q`, `RcvbufErrors`, downstream consumer lag), but the specific claim that `snmptrapd` exposes a queue depth counter is **unverified**. Flag applied in the signal description.
- `<!-- TODO: verify exact OIDs -->` for `coldStart`, `warmStart`, `linkDown`, `linkUp` (Advisor 1's TODOs). **Resolved** by online search (Net-SNMP docs, Observium). OIDs: `coldStart` `.1.3.6.1.6.3.1.1.5.1`, `warmStart` `.1.3.6.1.6.3.1.1.5.2`, `linkDown` `.1.3.6.1.6.3.1.1.5.3`, `linkUp` `.1.3.6.1.6.3.1.1.5.4`. Flags removed.
- `<!-- TODO: verify whether CISCO-ENVMON-MIB thresholds are operator-configurable or factory-fixed per platform -->` (Advisor 5's TODO). **Partially resolved** by the general statement that environmental traps are configurable per-device. The specifics vary by vendor and platform. The signal description acknowledges this with the "thresholds are device-specific" caveat. No specific TODO needed in the final body.
- `<!-- TODO: verify exact procedure for clearing engine time cache in common trap receivers -->` (Advisor 1's TODO). **Partially resolved** by online documentation (Fortinet, Zabbix, net-snmp cache flush functions). The procedure varies by receiver implementation. The pattern's First Response notes "on the receiver, clear the cached engine time entry for the affected device" without prescribing a specific command, which is appropriate given the variance. No specific TODO needed in the final body.
- `<!-- TODO: verify exact procedure for clearing engine time cache in common trap receivers -->`. Resolved at the conceptual level (clear the cache entry; next bidirectional operation refreshes) but the specific command varies by receiver. Pattern's First Response describes the action without overclaiming on a specific syntax.

The two remaining `<!-- TODO: verify -->` flags in the body relate to:

1. The specific claim that `snmptrapd` exposes an internal queue depth metric — this is a known instrumentation gap. The signal is preserved with the gap acknowledged.

2. The compatibility claim for vendor MIBs (CISCO-ENVMON-MIB thresholds) — the general principle is sound, but per-platform specifics vary. The signal description uses relational language ("temperature exceeding critical threshold") rather than platform-specific numbers.

These are the items a downstream human reviewer should validate before the playbook ships to production monitoring systems.
