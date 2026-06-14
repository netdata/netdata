# Netdata SNMP Trap Design — Phase B Stress Test

## 0. Document Metadata

- **System under test**: `.agents/sow/specs/snmp-traps/netdata.md` — proposed Netdata Agent SNMP trap reception, decode, enrichment, metric generation, and journal-storage subsystem (642 lines).
- **Posture**: adversarial. The target is treated as a junior engineer's first draft. The job is to break it, not validate it.
- **Lens**: every Netdata claim is cross-checked against the 16-system Phase A corpus (per-system specs + 6 comparison artefacts + the foundational `../domain/snmp-traps-in-observability.md` lens).
- **Citation convention**: `netdata.md §N L<line>` for the target; `<system>.md §N` or `<system>.md L<line>` for cohort evidence; `./<artefact>.md §N` for the Phase A matrix.
- **Reviewer convergence**: see §10. Run after the file was drafted in whole.
- **Authored by**: assistant under Phase B stress-test instructions.

---

## §1 Executive summary — top concerns

Severity ladder: **blocker** (design will not work as written), **major** (design will work but with a serious operator-visible defect), **minor** (precision/coverage problem), **nit** (wording).

1. **(blocker) The "every trap, always, no exceptions" claim is internally false.** `netdata.md §2 L38` and §11 L437-441 promise "every trap, always, with no exceptions, produces a full structured journal entry." But §10 L394-398 specifies that fingerprinted duplicates within the dedup window get **no journal write, no per-event metric increment** — only a counter increment and a later summary entry that does NOT preserve the suppressed PDU's varbind payload. The forensic-store claim is therefore false as written; suppressed duplicates' varbind detail is lost. Either drop the "no exceptions" wording or do not suppress journal writes (only suppress metric emission). See §3.0.
2. **(blocker) The journal is not a sufficient forensic store.** The design hangs the entire "no new storage tier" claim on systemd-journal (`netdata.md §2`, §11). The cohort's only convergent finding on journal/log-as-store is that retention, indexing, and journal corruption are all real concerns — Centreon's `log_traps.output_message VARCHAR(2048)` truncation (`centreon.md L530`) and SolarWinds' 90-minute DELETE on `Orion.TrapVarbinds` (`solarwinds.md L419`) both show that the "just write each trap as a row" choice carries a hidden retention/cleanup tax. The design names systemd-journal but does not state retention defaults, rotation policy, journal corruption recovery, journal namespace, search-index strategy, or what happens when `/var/log/journal/` fills. See §3.1.
3. **(blocker) The throughput target — "10s of thousands/sec sustained on a single hub" — is unsupported.** `netdata.md §1` and §9 claim "10× to 60× cohort numbers" without a single benchmark. Dynatrace's c5.large ActiveGate publishes 17k–150k traps/**minute** (`dynatrace.md L233-236`) — that is **283-2,500/sec**, not 10s of thousands/sec. Splunk SC4SNMP's 1,500/sec vendor number is on **16 cores / 64 GB** (`splunk-sc4snmp.md L699`). Telegraf's single-goroutine ceiling is ~10k/sec on raw decode only (`telegraf.md §10`). Netdata's claim asserts the cohort's combined ceiling, **plus journal-writes**, on commodity hub hardware. See §4.1.
4. **(blocker) "Single writer per journal file caps at ~30k rows/sec" is asserted without a citation.** `netdata.md §5 L126` and §9 L362. No Netdata benchmark is referenced; the journal writer is named "the existing Netdata journal writer," but the file gives no commit, no test, no number provenance. The whole partitioning argument that follows depends on this number. See §4.1.
5. **(blocker) v3 USM dynamic engine-ID discovery is a security vulnerability, not an operator convenience.** `netdata.md §5 L153` adopts Splunk's pre-parse hack from `splunk-sc4snmp.md §3.5` and hot-registers `(engineID, username)` pairs. Any device on the network — including an attacker — can claim any engineID via crafted SNMPv3 PDU bytes, get hot-registered, and inject traps that the journal records as authoritative. There is a TOCTOU between discovery and Secrets lookup. CheckMK explicitly requires engine_ids configured per credential (`checkmk.md L151`); Datadog FNV-128 hashes engine ID from hostname (`datadog-agent.md L228-243`); SC4SNMP requires `DISCOVER_ENGINE_ID` opt-in (`splunk-sc4snmp.md L194-205`). The design adopts the workaround without explaining the spoofing window, persistence of hot-registered pairs, expiry, rotation, or operator opt-in/default. Additionally, plugin restart resets `snmpEngineBoots` (RFC 3414 §2.2.2), breaking INFORM Response acceptance until devices time out their cached state. See §7.2.
6. **(major) Source-IP-hash partitioning to scale writer threads is a risky assumption for dedup state.** `netdata.md §5 L126` and §9 L366-370. Dedup cache (§10 L394) is in-memory; the target does not explicitly say whether it is global, per-listener-thread, or per-writer-shard. If the design later shards listeners → writers by source-IP hash to break the 30k/sec ceiling **and** dedup state shards with the writer, the same `(source, OID, key)` from a device whose IP changes shards mid-storm escapes dedup. The design must commit to one dedup-state model before partitioning. See §3.2.
7. **(major) "Likely the only real-time system among the cohort" is factually wrong.** `netdata.md §1 L21` Rule 6. `./feature-matrix.md L611-629` shows OpenNMS, Zenoss, CheckMK, Sensu, Zabbix, SolarWinds all do real-time evaluation; Centreon partial (~1s); LibreNMS partial (synchronous handler, cron-delayed delivery). **8 of 16 systems do real-time alarm-engine evaluation.** Netdata is competitive on this axis, not unique. See §3.7.
8. **(major) "Repeated identical-trap" suppression hides flap signatures, not paired clears.** Earlier framing of this finding incorrectly claimed `linkUp`/`linkDown` collapse — they don't because they have different trap-OIDs (`1.3.6.1.6.3.1.1.5.3` vs `1.3.6.1.6.3.1.1.5.4`) and trap-OID is in the dedup key. The **real** risk: a flapping BGP peer sending `bgpBackwardTransition` every 800ms within a 5s dedup window produces **1 journal entry** for 6 identical traps; operators get silence for a real failing optic / failing peer. The lens (`../domain/snmp-traps-in-observability.md §11.7`) is explicit: aggressive dedup hides early hardware failure. Plus the design has no paired-clear mechanism at all — missing the cohort's gold-standard pattern (`opennms.md §5 L274-279` clear-key, `zenoss.md §5 L329-330` clear_fingerprint_hash, `logicmonitor.md L477` auto-close). See §3.3.
9. **(major) "MIB compiler runs inside the plugin via inotify hot-reload" is novel, untested, and risky.** `netdata.md §7 L253` proposes inotify on `/etc/netdata/snmp-mibs/` triggering MIB recompile and in-memory swap. The cohort experience: SC4SNMP needs `pysmi` and rollout-restart (`splunk-sc4snmp.md §4`); CheckMK uploads via WATO + PySMI compile (`checkmk.md §4`); Datadog requires `ddev meta snmp generate-traps-db` and an Agent restart (`datadog-agent.md L410`); Zenoss runs `zenmib` via SubprocessJob (`zenoss.md §4`); Dynatrace requires EEC restart (`dynatrace.md §4`). **Zero** cohort systems do live MIB hot-reload inside the trap process. Smiparser / pysmi runtime cost on 28k MIB files is non-trivial. See §7.1.
10. **(major) Profile YAML "vendor knowledge only" rule contradicts cohort norm; the two-artefact split breaks Rule 2 (simpler).** `netdata.md §7` says profile YAML "does NOT define metric emission (that lives in plugin configuration — see §7.5)." But the cohort norm — OpenNMS event XML, Datadog `dd_traps_db`, SNMPTT, Zenoss `EventClass`, LogicMonitor LogSource — is that **per-OID metric/alert emission is declared in the same artefact the vendor knowledge lives in**. Splitting profile (vendor) from plugin config (operator emit) creates two artefacts that operators must keep in sync; the design provides no consistency check between them. Rule 2 ("Simpler — for the operator this must be the simplest possible engine") is **directly violated** — enabling alerting on a single Cisco port-security OID requires edits across three locations (profile/vendor knowledge, plugin config `metrics:`, plugin config `labels:`). See §3.5.
11. **(major) "Per-device vnode following existing SNMP polling pattern" is a cardinality bomb.** `netdata.md §12 L537` instantiates "Per-device vnode (each monitored device is a Netdata virtual node)." With a target of "any reasonable site size" and the design's appetite for unknown OIDs from unprofiled devices (§7 L253), a site with 5,000 SNMP devices means 5,000 vnodes × 2 always-on chart contexts × ~10 dimensions = 100,000 dimensions sustained on the hub just for the per-device events chart. The cohort wisdom (`../domain/snmp-traps-in-observability.md §11.7` Silent Deduplication Side-Effect, §8 Label Explosion Tension) is precisely **not to do this**. See §5.1.
12. **(major) AGPL-3.0 licensing claim is an unreviewed legal assertion, not engineering.** `netdata.md §8 L349-351` asserts "License obligations on others' source files (GPL-2.0 for LibreNMS, AGPL-3.0 for OpenNMS) do not propagate to our derived YAML." This is a legal question, not an engineering one. AGPL-3.0 copyleft + the "derivative work" definition apply to transformations of source. Transforming 17,442 OpenNMS event-XML definitions (including severity, reduction-key, clear-key, varbind-decode mappings) into Netdata profile YAML is arguably derivative. Datadog's parallel approach is Apache-2.0-tooled producing closed-source output from open MIBs (`./profile-inventory.md §6`) — a different licensing posture. Netdata's claim needs legal review before commit. See §3.8.
13. **(major) Hub-down = fleet blind spot during outage.** The hub model (`netdata-snmp-hub-architecture.md` § "What each SNMP hub contains") commits to "no central correlation tier" and "Cloud aggregates for presentation; not for correlation." When Site A's hub crashes, Site B's operators see nothing from Site A — and Site A's devices' traps are silently dropped at the kernel UDP layer of the unreachable hub. SaaS-cohort systems (Datadog, Dynatrace, LogicMonitor, Splunk SC4SNMP) have resilient SaaS-side ingestion during collector downtime; the trap reaches the central tier even if the local collector is unhealthy. The hub model deliberately drops this resilience. The design does not surface the trade-off. See §3.9.

---

## §2 Strengths (cohort-validated)

These are claims in `netdata.md` that hold up under cohort comparison. Every one is cited to specific cohort evidence; none is "good in principle."

### S1. Journal-as-document primitive (partial vindication)

`netdata.md §2 L32-49`: the choice to treat every trap as a structured journal row maps to the SaaS-cohort convergence. `./design-forks.md` Fork 1(d) lists six systems — Datadog, Dynatrace, Splunk SC4SNMP, Logstash, SolarWinds (current), LogicMonitor — that converged on the log-document primitive. `datadog-agent.md L20` is explicit: traps are a `logs` data type, not metrics. `dynatrace.md L36-39`: Grail log event. So the document-primitive choice is **cohort-validated by 6 of 16 systems**. **Caveat**: every one of these six is a forwarder-to-SaaS; none of them runs the document store on-prem on the same hub. The strength is the primitive choice, not the on-prem-journal storage choice (see §3.1).

### S2. Profile-derived OOB coverage *aspiration* is cohort-comparable

`netdata.md §8 L302` cites Datadog vendor claim "11,000+ MIBs" but the verified count via copy of `dd_traps_db.json.gz` is **3,652 MIBs** (the design's own citation). `netdata.md §8` proposes ~28,200 raw MIB files → 8,000-12,000 deduped modules, transforming OpenNMS XML + Centreon SQL + LibreNMS PHP handlers + Datadog `dd_traps_db` + pysnmp/pysmi public MIBs + LibreNMS bundled tree. `./profile-inventory.md §1` shows OpenNMS 17,442 event definitions (AGPLv3); `./profile-inventory.md §3` LibreNMS 4,770 MIBs / 2,245 with notifications; `./profile-inventory.md §6` notes Datadog "vendor claim" of 11,000+ vs verified 3,652. **Important caveats: (a) Netdata's 8-12k is the raw deduped MIB module count, not curated/tested coverage; (b) Datadog's verified 3,652 represents *curated* trap definitions, not raw modules; (c) the conversion tools (§8 L334-348) do not yet exist (see H2 in §7). The aspiration is cohort-comparable; the delivery target must be measured, not asserted.** **Confidence medium.**

### S3. Source identification via PDU-level fields (RFC 3584)

`netdata.md §5 L136` cites LogicMonitor EA 36.100+ pattern (`logicmonitor.md L436-441`): v1 → `agent-addr`, v2c/v3 → `snmpTrapAddress.0` varbind. This is the cohort's cleanest pattern. **8 of 16** systems either fail this or do not exercise it (CheckMK explicit drop `checkmk.md L305`, Datadog explicit drop `datadog-agent.md L491`, Splunk SC4SNMP `splunk-sc4snmp.md L444`, Sensu, Telegraf, LibreNMS, Zabbix Perl bridge, Dynatrace `dynatrace.md L361`). LogicMonitor (`logicmonitor.md L438-441`) and OpenNMS (`opennms.md L242-247`) are the gold-standard. Netdata's adoption of this pattern is cohort-validated. **Confidence high.**

### S4. (REMOVED — moved to W8)

Iter-1 reviewers flagged this as soft-pedalling: a "shared cohort blind spot" is not a strength. The original framing has been removed; see W8 for the proper weakness treatment.

### S5. PLUGINSD/stdio decoupling for stdout backpressure

`netdata.md §5 L129-149` decouples the hot path (UDP recv + decode + counter increment + journal write) from the cold path (PLUGINSD `BEGIN`/`SET`/`END` emission). This is the right pattern for the cohort: `telegraf.md §10` documents single-goroutine throughput collapse when downstream backs up; `opennms.md L131 (isBlockWhenFull=true)` is the contrasting back-pressure choice. Netdata's choice — never let the metric pipe back-pressure into the trap-receive path — is closer to Telegraf's metric buffer + accumulator design (`./feature-matrix.md L937`). **Cohort-validated.**

### S6. INFORM acknowledgement in scope

`netdata.md §6 L162` includes INFORM. The cohort is split: OpenNMS, Zenoss, Centreon, Zabbix support INFORM (`./feature-matrix.md L122-141`); CheckMK explicitly does NOT (`checkmk.md L152` — "We do not support them (yet)"); Dynatrace explicitly does NOT (`dynatrace.md L209-213`). The lens (`../domain/snmp-traps-in-observability.md §2`) flags INFORM as "reliability for v2c/v3"; financial/healthcare compliance asks for it. Netdata including it is **better than CheckMK and Dynatrace, on par with OpenNMS/Zabbix**.

### S7. Plugin-self metrics (`snmp.trap.events`, `snmp.trap.errors`)

`netdata.md §12 L525-561` exposes per-hub pipeline-health metrics: `unknown_oid`, `decode_errors`, `deduplicated`. The cohort has this only sometimes: OpenNMS 11 JMX counters (`opennms.md L271`), Telegraf via internal metrics, Datadog `datadog.snmp_traps.*` (`./operator-features.md T2-07`), Zabbix `zabbix[process,snmp trapper,*]` template (`zabbix.md L536`). Most others — CheckMK, Centreon, LibreNMS, Nagios+SNMPTT, Sensu, SolarWinds, Dynatrace, LogicMonitor — have nothing or log-scrape-only. Netdata's first-class self-telemetry is **above cohort median**. **Confidence high.**

### S8. Cardinality discipline rule (§4)

`netdata.md §4 L81-110`: MAC, source-IP-of-auth-attempt, username, RAID slot ID, packet content are FORBIDDEN as metric labels; they live in journal MESSAGE / JSON varbinds. This is exactly what the lens (`../domain/snmp-traps-in-observability.md §8.4`) and `../domain/snmp-traps-in-observability.md §4 Sampling/Aggregation Caveats` warn about. **Cohort behaviour is split**: Telegraf has explicit cardinality discipline (`telegraf.md L355`); Zabbix history_log/_text avoids labels by design (`zabbix.md L343-358`); SolarWinds creates a row per varbind in `Orion.TrapVarbinds` (`solarwinds.md L413`). Netdata's structural-reject-at-config-load (`netdata.md §4 L110`) is **stronger than any cohort default**. **Confidence high.**

### S9. Hub-local correlation (no central tier) — differentiation, but with a real trade-off

`netdata.md §0 L7` and `netdata-snmp-hub-architecture.md` together commit to "no central correlation tier." The hub-with-co-located-topology-and-polling pattern is the differentiator. `./feature-matrix.md L880-898`: every other system either has a central DB (OpenNMS, Zenoss, Centreon, LibreNMS, Nagios, SolarWinds), a central SaaS (Datadog, Splunk, Dynatrace, LogicMonitor), or pushes correlation to the operator (Telegraf, Logstash, Cribl). Telegraf/Logstash/Cribl are *also* distributed without central correlation, so "no central tier" is not unique — the specific combination of (UDP/162 reception + polling + topology + alert engine + log store + correlation) co-located on one hub is unique. Lens (`../domain/snmp-traps-in-observability.md §6.4 Anti-Pattern Single Manager Bottleneck`) supports the choice. **Caveats**: (a) per-hub throughput must actually deliver (see §1 #3); (b) the hub-down = fleet blind spot trade-off is unstated in the design (see §1 #13).

### S10. Categorical taxonomy mapped to MIB-derived knowledge

`netdata.md §3 L62-72` lists 9 categories (config_change, security, auth, license, mobility, state_change, diagnostic, custom, unknown). Cohort comparison: `opennms.md §13` event catalogue uses similar groupings; `librenms.md §5` handler classes map identically (BGP, OSPF, IF-MIB, environmental, security). Netdata's category set is **derivable from the OpenNMS + LibreNMS taxonomy union**, not invented in isolation. **Caveat**: see §3.4 — the `state_change` bucket is too coarse.

---

## §3 Weaknesses (cohort-evidence-contradicted)

Structured findings — each one has severity, target evidence (file/section/line), cohort-evidence that contradicts the choice, recommended fix.

### W0. (blocker) "Every trap, always, no exceptions" contradicts dedup behaviour

- **Target evidence**: `netdata.md §2 L38` "Every trap, always, with no exceptions, produces a full structured journal entry"; §11 L441 "every trap, always, no exceptions, lands in the journal with all its varbinds"; §10 L394-398 hot-path dedup: "Fingerprint present → suppress: **no journal write**, no per-event metric increment. Increment an in-memory per-period suppression counter."
- **Contradiction**: the two statements cannot both be true. Suppressed duplicates do not produce a full structured journal entry. The §10 L405-414 periodic summary entry carries only counters per-trap-OID — the suppressed PDU's varbind payload (the high-cardinality MAC, source IP, username) is **lost**.
- **Cohort contradiction**: OpenNMS (`opennms.md §5`) separates alarm dedup (`alarms` table keyed by reduction-key) from event storage — every non-discarded event is written to the `events` table; the explicit `<logmsg dest="discardtraps">` path drops events with a counted reason and is the only way an event is suppressed from the events table. Zenoss (`zenoss.md §5 L329-330`) upserts a single row keyed on `fingerprint_hash` with `event_count` and `last_seen` — counter increment is also a write that preserves last-trap state. **No cohort system claims forensic completeness while suppressing journal writes without an explicit, counted discard path.**
- **Recommended fix**: either (a) drop "no exceptions" from §2 L38 and §11 L441, and explicitly say "dedup suppresses journal writes for repeated identical traps; suppressed traps appear only in counters and periodic summary entries"; or (b) write all traps to the journal but with a `SNMP_TRAP_DEDUPED=true` flag so suppressed traps don't fire alerts but their varbind payload is preserved for forensics. Option (b) is closer to the design's stated forensic-completeness goal.

### W1. (blocker) Journal-storage cost model is absent

- **Target evidence**: `netdata.md §2 L36-49` "the journal is the universal store … no new storage tier"; §11 L437-441 "every trap, always"; §12 L562 operator-opted-in per-OID metric chart.
- **Cohort contradiction**:
  - `solarwinds.md L419`: DELETE on `TrapVarbinds` "on the order of 90 minutes" on large deployments. Per-row growth is the storage failure mode.
  - `centreon.md L530-536`: `log_traps.output_message VARCHAR(2048)` truncates silently or fails the INSERT depending on SQL mode; operator gets no signal either way. Per-row storage carries a per-row schema-discipline cost.
  - `zenoss.md §6 retention`: `event_archive_interval_minutes=3 days`, `event_archive_purge_interval_days=90`. Even Zenoss with a relational store cannot afford to keep traps forever.
  - `splunk-sc4snmp.md §10` 1,500/sec vendor ceiling on 16-core/64GB — the storage tier is the cost.
  - `checkmk.md §6` `history_lifetime=365 days` default on a per-host event-store with SQLite/file/Mongo dispatch.
- **What's missing in netdata.md**: no statement of journal retention policy, no rotation/vacuum strategy, no statement of what happens when the journal namespace fills, no statement of expected per-row storage cost (the `SNMP_TRAP_JSON` field at §11 L492 is a serialized JSON of all varbinds + name/oid/type/value tuples — that is hundreds of bytes per row at minimum; at 10k traps/sec sustained with a 500-2000 byte per-row payload the write rate is **5-20 MB/sec to the journal**, ~432 GB/day uncompressed at the low end and ~1.7 TB/day uncompressed at the high end; systemd-journal's built-in compression reduces this by 3-5×), no statement of how `journalctl --fields` performance behaves when there are millions of journal entries with stable but content-rich `MESSAGE` fields.
- **Recommended fix**: §11 must specify (a) per-trap journal namespace (e.g., `netdata-snmptrap.journal`), (b) retention defaults (recommend 30d default, configurable), (c) rotation policy, (d) journal-corruption recovery path (does the agent restart the listener? does it drop in-flight traps? what's the operator-visible signal?), (e) expected per-row byte cost. Without these, "no new storage tier" is a marketing claim, not a design.

### W2. (blocker) Throughput claim has zero evidence

- **Target evidence**: `netdata.md §1 L16` "dozens of thousands of events/sec, sufficient for any reasonable site size"; §9 L380 "10× to 60× cohort numbers"; §5 L126 "~30k rows/sec per writer thread."
- **Cohort contradiction**:
  - `dynatrace.md L233-236`: c5.large ActiveGate: default 45k/min v2c (~750/sec), 30k/min with logs enabled (~500/sec). High-performance: 150k/min (~2.5k/sec). These are vendor-published. Netdata's claim is **4×-40× higher** on commodity hardware. The Phase A summary at `netdata.md §9 L378` is right to label these "0.5-0.75k/sec" — but then dismisses them.
  - `splunk-sc4snmp.md L699`: 1,500 traps/sec on 16-core/64GB single-host. Netdata claims 10k+/sec on a Netdata hub (= one VM/host).
  - `telegraf.md L1251`: "A single-goroutine listener is a known throughput ceiling" — Telegraf's single-goroutine raw decode ceiling is ~10k/sec on best hardware, before any enrichment or storage.
  - `opennms.md §5 L131`: bounded async queue with `isBlockWhenFull=true`; throughput collapses gracefully into kernel UDP drops, not into in-memory growth. The OpenNMS team explicitly documented this back-pressure choice rather than promising high sustained rates.
- **Recommended fix**: replace "10s of thousands of events/sec" with a concrete benchmark plan. Pick three reference platforms (e.g., 4-core/8GB, 16-core/32GB, 32-core/64GB) and commit to publishing measured numbers under three workloads: pure decode no journal-write, decode+journal-write, decode+journal-write+enrichment. Don't claim "10× cohort" before measuring; the existing Rust journal writer "~30k rows/sec" number must be sourced or removed.

### W3. (major) Single-writer-per-file partitioning is a risky assumption for dedup state

Originally classified blocker in iter-1. Codex iter-2 correctly noted the "dedup escape hatch" depends on an unstated implementation assumption (dedup state per-writer vs global); the target only specifies *writer* partitioning. Downgraded to major risky-assumption. The risk remains material — if implementation defaults to per-shard dedup, the escape hatch is real. The fix in R4 forces the design to commit to a single dedup model before partitioning.

- **Target evidence**: `netdata.md §5 L122-126` "single writer per journal file. The journal writer caps at ~30k rows/sec per writer thread. To exceed that, partition: either run multiple writer threads each owning its own journal file, or shard listeners → writers by source-IP hash"; §10 L394-398 in-memory dedup cache (LRU 100k entries) checked in hot path.
- **Cohort contradiction**:
  - `opennms.md §5`: alarm-level reduction-key dedup happens in PostgreSQL with a unique constraint — single source of truth, serialized through the DB. Centralized dedup is the cohort pattern.
  - `centreon.md L356-360`: MD5 digest dedup with `duplicate_trap_window=1s`, single Perl daemon — single in-memory dedup hashmap.
  - `zenoss.md L329-330`: `fingerprint_hash` checked at ZEP via DB unique key, single source of truth.
- **Why this breaks**: if the design later partitions listener-to-writer by source-IP hash to get past 30k/sec, the dedup cache must either be (a) global — defeating the partitioning goal (re-introducing cross-shard contention/locking), (b) per-shard — meaning that if a device's IP changes hash-buckets (e.g., listener restart, network re-IP, NAT remapping), the dedup state doesn't follow, and a true duplicate from the same OID + key varbinds escapes; (c) hash-based on `(source_ip, OID, key_varbinds)` rather than just source-IP — at which point the partitioning is no longer a function of source IP alone.
- **Recommended fix**: pick one. Option A: commit to single-writer until measured proven insufficient, defer partitioning to a SOW. Option B: design dedup as a separate process/thread with explicit invalidation rules under sharding. Option C: dedup at metric-update-time, not hot-path-time — but this contradicts the §10 L427 claim that "the metric `snmp.trap.errors.deduplicated` increments per suppressed trap." Don't promise both lock-free hot-path dedup AND multi-writer partitioning without specifying how state is shared.

### W4. (major) Dedup hides repeated identical traps + paired-clear semantics are absent

- **Target evidence**: `netdata.md §10 L394-398` "first-wins" + "Cache entries expire after the dedup window (default e.g., 5 seconds, configurable globally and per-OID)"; §7 L208 `dedup_key_varbinds: [cpsIfViolationMacAddress, cpsIfViolationVlan]`.
- **Clarification**: `linkUp` (`1.3.6.1.6.3.1.1.5.4`) and `linkDown` (`1.3.6.1.6.3.1.1.5.3`) have different trap-OIDs; the §10 fingerprint `hash(source_device, trap_OID, key_varbinds)` does NOT collapse them. The risk is NOT linkUp/linkDown collapse.
- **The real risk**: repeated *identical* traps within the dedup window are hidden. A flapping BGP peer sending `bgpBackwardTransition` (`1.3.6.1.2.1.15.7.2`) every 800ms within a 5s default window produces **1 journal entry for 6+ identical traps**; operators get silence for what is a real failing peer/optic. The lens (`../domain/snmp-traps-in-observability.md §11.7 Silent Deduplication Side-Effect`) is explicit: "Aggressive deduplication (e.g., 5-minute window) suppresses a flapping interface that is actually an early hardware failure signature."
- **Cohort contradiction on paired-clear**:
  - `opennms.md §5 L274-279`: alarm-layer reduction-key + `clear-key` for paired-clear (e.g., `ccmGatewayRecovered` clears `ccmGatewayFailed`). Cohort gold-standard.
  - `zenoss.md §5 L329-330`: ZEP `clear_fingerprint_hash` computed from `device|component|eventClass|eventKey` — paired traps with matching identifiers but different severity clear automatically.
  - `logicmonitor.md L477` LogSource: "Automatically close alerts when a related 'clear' trap comes in."
  - `./feature-matrix.md L522-541` Clear-pair semantics: only OpenNMS, Zenoss, LogicMonitor LogSource fully support; CheckMK partial via `match_ok`; **most others operator-built**. Netdata's design has **no paired-clear mechanism at all** — `netdata.md §14 L591` explicit non-goal "Built-in alarm-lifecycle state machine (open/ack/clear) — alert-engine territory."
- **Why netdata.md gets this wrong**: §3 L79 admits paired-clear in passing ("A future release may add a `metric_filter` profile field..."), but the dedup mechanism in §10 actively hides flap signatures. The lens §11.7 calls this out as the failure mode for aggressive dedup.
- **Recommended fix**:
  1. Document the 5s-window dedup behaviour and surface the suppressed-count as a per-trap-OID metric so operators see flap signatures even when journal entries are collapsed.
  2. Reduce default dedup window to **1s** (matching Centreon, `centreon.md L356` `duplicate_trap_window`).
  3. Add an Open Question (§13) about whether paired-clear semantics belong in a profile field (`clear_trap_oid:`) and explicitly defer to a follow-up SOW — but say it now so reviewers cannot infer the design handles paired-clear correctly.
  4. Document explicitly that `linkUp`/`linkDown` do NOT collapse because trap-OID is in the fingerprint key.

### W5. (major) Profile YAML / plugin config split creates two-artefact synchronization

- **Target evidence**: `netdata.md §7 L166-211` profile YAML is "vendor knowledge only"; §7.5 L255-296 plugin config has the per-OID metric opt-in, severity/category overrides, labels.
- **Cohort contradiction**:
  - `opennms.md §4 L191-194`: single XML per vendor (`Cisco.events.xml`, `Juniper.events.xml`, etc.) that bundles UEI + severity + `<alarm-data>` + `<varbindsdecode>` + the reduction-key template. **One file, one source of truth per vendor.**
  - `datadog-agent.md §4 L275-298`: `dd_traps_db.*` is JSON/YAML covering MIBs **and** the lookup tables for resolving OIDs in event payloads — one bundled artifact.
  - `librenms.md §5 L330-345`: per-handler PHP class registered in `config/snmptraps.php` (the OID → handler map). One file is the canonical mapping; the handler class is the canonical behaviour. Operators add a handler by adding a class **and** an entry — but the registry is one file.
  - `nagios-snmptt.md §5`: SNMPTT `EVENT` block carries the OID + name + category + severity + FORMAT + EXEC in one file. Single file.
  - **None of the cohort splits vendor knowledge from operator-emit choice across two artefacts.**
- **Why netdata.md gets this wrong**: §7.5 L294 says "Operators do NOT copy entire profiles to enable metrics or add labels. The profile remains the vendor's curated knowledge; per-installation choices are surgical edits in plugin config." This is design-pure, but operationally awkward: an operator wanting to alert on Cisco port-security violations must (a) install profile YAML for Cisco port-security (or rely on stock), (b) edit plugin config to add the OID to `metrics:`, (c) edit plugin config again to add labels via `labels:` block. Three lookups across two files. **No cohort system requires this.**
- **Recommended fix**: either (a) accept the split and provide a single "what do I do to enable alerting on OID X?" recipe in the design that clearly walks all three places, (b) merge into one artefact with two sections (vendor `<core>` is read-only, operator `<override>` is mutable), or (c) keep the split and ship a config-generation tool that, given an OID, prints the plugin-config diff the operator should apply.

### W6. (major) "Closed category set" forecloses operator extensibility

- **Target evidence**: `netdata.md §3 L74-75` "Category set is closed — the 9 slugs above are the canonical taxonomy. Operators cannot extend this set; new slugs are added via Netdata releases when genuinely new content types emerge."
- **Cohort contradiction**:
  - `opennms.md §13` ships 17,442 event definitions and the operator can add more — categories are not closed.
  - `zenoss.md §4 L241` `EventClass` tree is operator-extensible; ZenPacks routinely add new classes (e.g., `/Status/Storage/RAID`).
  - `librenms.md §5` 177 handler classes — operators add new classes via a PR; not closed at runtime, but extensible by community.
  - `centreon.md §6 L444-470`: the `traps_vendor` and `traps` tables are operator-mutable.
  - `logicmonitor.md §5`: EventSource is portal-customizable; the alert lifecycle is per-source.
- **Why netdata.md gets this wrong**: closing the set is justified by §3 L75 with "For cross-cutting concerns ... operators use labels." But the cohort uses categories AND labels — categories for the per-device chart dimension, labels for cross-cutting concerns. Forcing operators to use only `custom` for, e.g., "industrial OT trap from PLC fleet" puts all PLC traps in one undifferentiated `custom` bucket on the per-device chart.
- **Recommended fix**: drop the "closed" claim. Either (a) make the category set advisory + extensible with a release-note warning that operator-added categories won't survive an upgrade unless declared in plugin config, or (b) keep it closed but expand `custom` into 2-3 sub-categories: `custom_security`, `custom_state_change`, etc. — give operators 3-4 named hooks rather than one.

### W7. (major) Profile inline `varbinds:` field duplicates MIB knowledge; precedence direction contradicts cohort norm

- **Target evidence**: `netdata.md §7 L189-209` `varbinds:` block with `oid`, `name`, `type`, `display_hint`; L215 resolution order: **profile inline → loaded MIB → raw fallback** (profile wins); L221 "operators get full decoding for top vendors without ever installing a MIB file."
- **Cohort contradiction**:
  - `datadog-agent.md §4 L394` ships `dd_traps_db.json.gz` precompiled. One compiled artifact; no inline override path in the per-OID definition. **Single source of truth.**
  - `opennms.md §4 L170-184`: per-trap event XML carries varbind metadata via the matched MIB at OpenNMS Compiler step; the per-event XML does not duplicate `oid`/`name`/`type` for every varbind. Compiler-extracted, post-compilation. **MIB is authoritative; profile entries are MIB-derived.**
  - `splunk-sc4snmp.md §4 L240-261`: MIB server is a separate container with lazy-compile semantics; pull-on-demand model.
  - The cohort systems that **do** allow operator override (SNMPTT, Zenoss transforms) are precisely the ones with the most drift and maintenance pain (`nagios-snmptt.md §4 L259-272`, `zenoss.md §5 L301-313`).
- **Why netdata.md gets this wrong** — two distinct problems:
  1. Two sources of truth: profile inline `varbinds:` + loaded MIB. When firmware updates a varbind type, profile wins silently — operator may decode against stale schema. (Iter-1 stress-test framed this correctly.)
  2. **Iter-1 reviewer correction**: the original stress-test recommended "operator wins" precedence. That contradicts the cohort norm. Operator wins is the SNMPTT/Zenoss pattern that earns the cohort's maintenance pain. **MIB-loaded should win.**
- **Recommended fix** (corrected from iter-1):
  - Loaded MIB **overrides** profile inline `varbinds:` when both are present; profile inline is the fallback for OIDs lacking MIB coverage.
  - Profile-conversion tools (§8) produce inline `varbinds:` from MIB sources at build time; shipping a profile with inline `varbinds:` that contradicts the bundled MIBs is a CI build error.
  - Operator can suppress the loaded-MIB-wins default with an explicit per-OID profile flag (e.g., `varbind_override: true`) for known vendor-MIB bugs.

### W8. (major) Storm/backpressure decision delegates entirely to kernel UDP

- **Target evidence**: `netdata.md §10 L388-390` "Netdata already ships built-in alerts for UDP receive-buffer overflow on all listeners. That covers the kernel-level overflow case. We do NOT duplicate that as a plugin feature"; §14 L593 explicit non-goal.
- **Cohort contradiction**:
  - `../domain/snmp-traps-in-observability.md §11.1 Trap Storm`: Trap Storm / Thundering Herd is the **#1 failure mode in the lens**. The lens recommends "circuit breakers that shed non-critical trap types under load; per-source token bucket rate limiting." Netdata's response is "we don't do that — the kernel alerts."
  - `cribl.md §10 L497`: kernel SO_RCVBUF + PQ + Drop function — three layers, even in a closed-source product positioned as a pipeline-only.
  - `opennms.md §5 L130-131`: explicit `isBlockWhenFull=true` back-pressure choice, allowing the kernel to be the buffer of last resort but the agent to push back deliberately.
  - `datadog-agent.md §5 L248-249`: bounded channel size 100, but explicit drop accounting via `EventPlatformEventsErrors[network-devices-snmp-traps]`.
- **Why netdata.md gets this wrong**: the existing Netdata UDP-overflow alert is reactive — operator sees "kernel buffer overflowed" after the fact. The cohort wisdom is that mitigation must happen before the buffer overflows, not after. The §10 L427 design relies on dedup as the storm-protection mechanism, but dedup only suppresses *known* duplicates; a storm of *unique* OIDs from one source (the lens §3 "Single device generating >80% of all trap volume") is not suppressed by dedup.
- **Recommended fix**: add a token-bucket per-source rate-limit as a deferrable Open Question (§13) but acknowledge in the Cohort-Win Audit (§16) that this is a real gap, not solved by Netdata's stack today. Don't claim "first-wins plugin-level dedup with periodic summary entries solves the same operational problem more elegantly" — it doesn't solve the unique-OID storm.

### W9. (blocker) v3 USM hot-registered engine-ID is a security vulnerability + boot-counter resets break INFORM

- **Target evidence**: `netdata.md §5 L152-153` "Subclass the SNMP transport to peek at the raw bytes pre-parse, ASN.1-decode the SNMPv3 header to extract `engineID`+`username`, hot-register the pair, retry parse"; §13 L576 open question 7 (UX TBD).
- **Cohort contradiction**:
  - `splunk-sc4snmp.md L194-205`: `DISCOVER_ENGINE_ID` is **opt-in** and explicitly gated. SC4SNMP's pattern requires operator consent to accept unknown engine IDs.
  - `checkmk.md L151`: engine_ids must be pre-configured per credential. Explicit rejection of dynamic discovery.
  - `datadog-agent.md L228-243`: FNV-128 hash of hostname → engine ID. Deterministic and not operator-supplied.
  - `./operator-features.md T3-03`: 7 cohort systems support multi-user; **none** documented as accepting fully-unknown engineIDs by default.
- **Security implications netdata.md does not state**:
  1. **Engine-ID spoofing**: any device on the network can craft an SNMPv3 PDU claiming any engineID. If the netdata listener hot-registers any pair on first contact, an attacker injects a fake `(engineID, username)` pair before the legitimate device sends its trap. The journal records the attacker's trap as authoritative.
  2. **TOCTOU between discovery and Secrets lookup**: the design (§7.5 L260) says community/keys come from Secrets. If hot-register happens before Secrets lookup, the Secrets binding may not match.
  3. **Hot-registered pair persistence**: undocumented. Does it survive plugin restart? Across `dyncfg` reloads? If yes, an attacker who registers an engineID during a brief gap persists in the table indefinitely.
  4. **snmpEngineBoots resets on plugin restart** (RFC 3414 §2.2.2): the boot counter must persist to stable storage and increment monotonically. The design (`netdata.md §5 L130`) says "plugin lifecycle follows the Netdata agent's lifecycle" — every Netdata restart resets the counter. Devices that cached the old boot counter will reject the plugin's INFORM Response PDUs as replay attacks until they time out (minutes to hours). This breaks the §6 INFORM-in-scope claim for v3.
- **Recommended fix**: elevate from minor to blocker. Require:
  1. Hot-registration is **opt-in via plugin config** (default: pre-configured engineIDs only, matching CheckMK/Splunk default).
  2. When hot-registration is enabled, the pair is logged to the journal with `SNMP_USM_HOT_REGISTERED=true` for operator audit, and the operator must explicitly confirm via dyncfg before the pair is considered trusted.
  3. Per-job `snmpEngineBoots` is persisted to disk (`${NETDATA_LIB_DIR:-/var/lib/netdata}/snmp-trap/{job_name}/engine-boots`) and survives plugin restart; document the restart-recovery behaviour for INFORM Response.
  4. Hot-registered pairs expire after a configurable TTL (default 24h) and require re-confirmation.

### W10. (minor) "Stock vendor pack for top-5 vendors" lacks a coverage commitment

- **Target evidence**: `netdata.md §17 L629-630` "Profile YAML loader + stock vendor pack for top-5 vendors (Cisco, Juniper, Arista, Aruba, Palo Alto)"; §13 L583 says "top 8-10."
- **Cohort contradiction**: `datadog-agent.md L387, 394` ships 3,652 verified MIBs out of the box; `librenms.md L208-211` ships 4,770. Top-5 vendor packs on day one — without committing to coverage depth — risks shipping a profile that misses common OIDs from supported vendors (the lens §11.5 Firmware Schema Drift).
- **Recommended fix**: state coverage metrics: e.g., "first release ships X profiles covering Y OIDs across Cisco/Juniper/Arista/Aruba/Palo Alto, including the top 50 alert-class OIDs per vendor from the LibreNMS handler corpus."

### W11. (minor) "Real-use evidence" missing for the hub model

- **Target evidence**: `netdata-snmp-hub-architecture.md` § "Implication for the eventual Netdata trap design discussion" — hub IS the trap processor.
- **Cohort gap**: No cohort system ships exactly this model. OpenNMS Minion is closest, but Minions forward to a central core. The hub model is unvalidated by cohort experience. The design assumes correlation lives where the data lives — but does not specify what cross-hub correlation use cases are dropped.
- **Cohort contradiction**:
  - `solarwinds.md §4` Network Atlas / Orion Maps support cross-site topology; centralised maps are an operator expectation in enterprise NMS.
  - `dynatrace.md §4 L546-558`: SmartScape entities span the cluster; cross-tenant entities link via the SaaS.
- **Recommended fix**: state explicitly which cross-hub correlation use cases the hub model **deliberately drops** (e.g., a flap-storm at site A is invisible to operators looking at site B's hub UI), and what the Cloud-presentation layer compensates with.

### W12. (minor) No mention of NSCD/DNS dependency for source identity

- **Target evidence**: §5 L138 "Enrich: device identity (sysName, vendor); topology position if co-located; recent polling state if available."
- **Cohort contradiction**:
  - `librenms.md §6 L339-345`: hostname/IP/ipv4_addresses/ipv6_addresses lookup is against the LibreNMS device table — not DNS.
  - `sensu.md L313-318`: reverse DNS via `Resolv.getname` — known operational pain.
- **Why netdata.md gets this wrong**: enrichment via the SNMP-polling plugin's state (per `netdata-existing-netipc.md`) is fast and reliable when the device is already polled. But for newly-discovered or NAT'd devices, "device identity" may need a DNS lookup or fallback. The hot path budget is microseconds; a synchronous DNS lookup is ms-to-seconds.
- **Recommended fix**: state explicitly that enrichment is best-effort and never blocks; if the device is not in the polling-derived cache, the journal records the source UDP peer IP and the `agent-addr`/`snmpTrapAddress` from the PDU and tags the entry with `SNMP_DEVICE_UNKNOWN`.

### W13. (major) "Likely the only real-time system among the cohort" is factually wrong

- **Target evidence**: `netdata.md §1 L21` Rule 6: "Real-time (1-second from PDU arrival to alert evaluation). **Likely the only real-time system among the cohort.**"
- **Cohort contradiction**: `./feature-matrix.md L611-629` Real-time evaluation table:
  - OpenNMS ✓ (`opennms.md L264` alarm engine on event bus; cosmicClear in Drools).
  - Zenoss ✓ (`zenoss.md L34` zeneventd pipeline → ZEP).
  - CheckMK ✓ (`checkmk.md L158` in-process synchronous).
  - Centreon partial (`centreon.md L200` median ~1s latency via 2s polling).
  - Zabbix ✓ (`zabbix.md L572-583` trigger evaluates on item value arrival).
  - Sensu ✓ (event arrives at backend immediately).
  - SolarWinds ✓ (`solarwinds.md L487` pub/sub Event alert condition).
  - LibreNMS partial (`librenms.md L521-525` alert state synchronous; transport via cron).
  - LogicMonitor partial (`logicmonitor.md L498` SaaS Alert Rule engine; round-trip).
  - Dynatrace partial (`dynatrace.md L526-528` Davis problems via log event rules).
- **Summary**: 6 of 16 systems do fully real-time evaluation; 4 more partial. Netdata is **competitive on this axis, not unique**.
- **Why this matters**: a stress-test that misses a headline marketing falsehood in the design's opening rules is not adversarial enough. This is a direct, checkable claim contradicted by the Phase A artefacts the design itself cites.
- **Recommended fix**: rewrite Rule 6 to "Real-time at metric-update cadence (1 second); competitive with the alarm-engine cohort and faster than the SaaS-log-monitor cohort." Drop "Likely the only" wording.

### W14. (major) AGPL-3.0 / GPL-3.0-or-later licensing claim needs legal review (and LibreNMS license is misstated in the design)

- **Target evidence**: `netdata.md §8 L349-351` "We are transforming knowledge at development time, not redistributing other systems' files. The output (our profile YAML) is original work informed by reading public documentation, MIB definitions, and open-source classifications. License obligations on others' source files (GPL-2.0 for LibreNMS, AGPL-3.0 for OpenNMS) do not propagate to our derived YAML."
- **Factual error in the design**: `./profile-inventory.md §3 L30` and `./profile-inventory.md §4 L39` both state LibreNMS license is **GPL-3.0-or-later**, not GPL-2.0 as netdata.md claims. The factual misstatement aggravates the unreviewed legal claim.
- **Why this matters**: this is a **legal claim, not engineering**. AGPL-3.0 has copyleft + derivative-work provisions; whether transforming 17,442 OpenNMS event-XML definitions (including severity, reduction-key, clear-key, varbind-decode mappings) into Netdata YAML produces a derivative work is a question for legal counsel, not the design author.
- **Cohort comparison**:
  - Datadog (`./profile-inventory.md §6`): Apache-2.0 tooling reads public MIBs and produces closed-source `dd_traps_db.json.gz`. The tooling is openly licensed; the bundled output is closed. Different posture from Netdata's claim.
  - Centreon (`./profile-inventory.md §2`): Apache-2.0 trap-engine, openly published SQL data file. Apache-2.0 reading + Apache-2.0 output = simple chain.
  - OpenNMS AGPLv3 (`./profile-inventory.md §1`): the event-XML corpus is the AGPLv3 part Netdata is transforming. AGPL-3.0 §13 (network use) and §1 (the source code definition) are non-trivial.
- **Recommended fix**: defer the licensing claim to legal review. Until reviewed:
  1. Remove the assertion that "license obligations do not propagate."
  2. State that the conversion approach is under legal review.
  3. Bring legal counsel into the design conversation.
  4. Consider Apache-2.0 / MIT alternatives for first-release coverage (Centreon SQL, public MIB tree, datadog/integrations-core tooling under Apache-2.0).

### W15. (major) Hub-down = fleet blind spot during outage

- **Target evidence**: `netdata-snmp-hub-architecture.md` § "Implication for the trap subsystem design" "Be self-sufficient per site"; netdata.md §0 L7 "no central correlation tier."
- **Why this matters**: when Site A's hub crashes, Site A's devices' traps go to the kernel UDP buffer on the unreachable hub and are dropped. Operators at Site B see nothing from Site A. The Cloud presentation layer aggregates *what hubs report* — it cannot see what they never received.
- **Cohort contradiction** (narrowed to systems with explicit receiver HA/failover):
  - `logicmonitor.md L184`: failover pairs, per-Collector — explicit collector-down failover documented.
  - `solarwinds.md L140-141, L145`: APE per location + central VIP HA pattern — explicit receiver failover.
  - `splunk-sc4snmp.md §3.6`: HPA scaling + multi-pod ingestion behind a Kubernetes UDP Service — one pod crash doesn't take down ingestion (architectural property, not an explicit failover claim).
  - `dynatrace.md L246-250`: documents the *operator pattern* of multi-target devices behind ActiveGate group — failover is operator's responsibility, not a vendor feature.
  - **Datadog**: forwarding to SaaS does NOT prove collector-down trap survival; if the Agent host is down, the device's trap is lost at the wire. (Original iter-1 W15 overstated this; corrected.)
  - **Conclusion**: explicit collector/receiver HA exists in only ~3-4 cohort systems (SolarWinds APE, LogicMonitor failover pairs, Splunk SC4SNMP via k8s). The hub model's "hub-down = blind" trade-off is shared with most of the cohort; what makes it noteworthy is that the **co-located correlation tier** also goes down with the hub, whereas in SaaS-cohort systems the correlation tier stays up.
- **Why netdata.md gets this wrong**: the hub model deliberately drops cross-hub resilience. The design (`netdata-snmp-hub-architecture.md` § "The user value of this architecture") frames "Site isolation" as a positive — but the negative ("Site A hub goes down, all of Site A's traps lost") is not surfaced.
- **Recommended fix**:
  1. State the trade-off explicitly: hub-down at Site X = trap blind spot at Site X for the outage duration.
  2. Recommend operator pattern: configure devices with **2+ trap destinations** so each trap reaches at least two hubs (lens §10 mandates "Trap Receiver Infrastructure with Monitoring-of-the-Monitor").
  3. Add a Netdata Cloud alert that fires when a hub stops reporting trap-rate metrics (the existing `snmp.trap.events` counter goes silent), so operators learn quickly when a hub is down.
  4. Document that 2-hub redundancy creates duplicate journal entries at both hubs; Cloud-side presentation must dedup across hubs (deferred D9).

### W16. (major) MIB version conflict resolution across hubs is undefined

- **Target evidence**: `netdata.md §7 L253` "Operator-provided **MIB files** (raw `.mib` / `.txt` SMIv1/v2 files for vendors not covered by stock profiles) live in `/etc/netdata/snmp-mibs/`. The plugin watches the directory via inotify; compiles new MIBs on file change; updates the in-memory MIB index without restart."
- **Why this matters**: in a multi-hub deployment, two hubs may run different versions of the same vendor MIB. A trap from Cisco device X arrives at Hub A (with Cisco-MIB v23.1) and at Hub B (with Cisco-MIB v21.0). Hub A produces `ifAdminStatus` (named); Hub B produces `1.3.6.1.2.1.2.2.1.7` (numeric). Cloud queries across both hubs see different field names for the same OID. Lens (`../domain/snmp-traps-in-observability.md §11.5 Firmware Trap Schema Drift`) calls this a top failure mode.
- **Cohort contradiction**:
  - `datadog-agent.md §4`: single `dd_traps_db.json.gz` shipped with the Agent; same Agent version = same MIB set across the fleet.
  - `dynatrace.md §4 L302-307`: bundled extension has a "fixed predefined OID set" — same set everywhere.
  - `splunk-sc4snmp.md §4`: shared mibserver container — single source of truth across pods.
- **Why netdata.md gets this wrong**: per-hub local MIB store is the architectural choice (`netdata-snmp-hub-architecture.md`), but the cross-hub query reconciliation problem is not addressed.
- **Recommended fix**:
  1. Stock profiles ship a Netdata-version-pinned MIB index — all hubs running the same Netdata version produce the same field names for stock-covered OIDs.
  2. For operator-supplied MIBs in `/etc/netdata/snmp-mibs/`, document that cross-hub field-name reconciliation is the operator's responsibility (or defer Cloud-side reconciliation to D9).
  3. Add a Netdata-Cloud-side advisory: when a hub's loaded MIB version differs from the fleet baseline, surface a warning.

### W17. (minor) Internal inconsistency in `snmp.trap.errors` dimensions

- **Target evidence**: `netdata.md §7 L238` template-unresolved increments `snmp.trap.errors.template_unresolved`; §12 L551 `snmp.trap.errors` dimensions are listed as `unknown_oid`, `decode_errors`, `deduplicated` only.
- **Inconsistency**: the design increments a dimension (`template_unresolved`) that is not listed in the dimension declaration.
- **Recommended fix**: add `template_unresolved` to §12 L551, or remove the increment from §7 L238.

### W18. (minor) `SNMP_TRAP_JSON` keyed by symbolic name has stability + collision risks

- **Target evidence**: `netdata.md §11 L517` "Keyed by varbind symbolic name when known"; §13 L583 "shape stability" open question.
- **Cohort contradiction**:
  - `datadog-agent.md §5 L521-527`: dual encoding (`variables[]` array of positional varbinds AND top-level named keys). The array preserves order/identity; the names are convenience.
  - `solarwinds.md §6.3 L395`: one row per varbind in `Orion.TrapVarbinds` keyed by `(TrapID, OID)`. OID-keyed, not name-keyed.
- **Risks**: (a) two different MIBs may define the same symbolic name for different OIDs (e.g., `ifIndex` is reused but in different scopes); name-keyed JSON collapses them silently. (b) Firmware/MIB updates change a varbind's symbolic name → operator's `jq` queries break.
- **Recommended fix**: ship JSON as an **array of `{oid, name?, type, value, ...}`** preserving the wire-order positional identity. Symbolic name is optional; OID is always present.

### W19a. (major) `snmp.trap.errors` dimensions miss authn/allowlist/USM-fail counters

- **Target evidence**: `netdata.md §6 L161` per-source community/IP allowlist; §12 L551 `snmp.trap.errors` dimensions = `unknown_oid`, `decode_errors`, `deduplicated` only.
- **Cohort contradiction**:
  - `opennms.md §5`: AuthenticationFailureLogger logs each dispatcher's auth result at DEBUG. There's no metric, but there's a log signal.
  - `splunk-sc4snmp.md L194-205`: `DISCOVER_ENGINE_ID` produces a counter for engine-ID discovery events.
  - Lens (`../domain/snmp-traps-in-observability.md §11.4` Community String / v3 Credential Mismatch Silence): the cohort's #4 failure mode is *silent* credential mismatch — operators report "no traps from device X" with no clue why. The fix is metric-level signal.
- **Why netdata.md gets this wrong**: traps that fail allowlist (UDP peer not in allowlist), traps that fail v3 USM auth, traps from unknown communities — all currently silent. The §12 `snmp.trap.errors` chart should have dimensions for these.
- **Recommended fix**: add `auth_failures`, `allowlist_dropped`, `usm_failures` (and possibly `unknown_community`) dimensions to `snmp.trap.errors` at §12 L551. Each is incremented when a PDU is rejected at the corresponding layer.

### W19. (minor) Pre-decode allowlist vs post-decode source identification

- **Target evidence**: `netdata.md §6 L161` "Per-source community/IP allowlist as the first filter — drops unwanted traffic before decode"; §5 L136 source identification happens after decode via `agent-addr` / `snmpTrapAddress` varbind.
- **Inconsistency**: §6 conflates "community allowlist" and "IP allowlist" as one filter. Only **IP** can be pre-decode (UDP peer is known before BER parse). **Community** requires at least minimal ASN.1 parsing — for v1/v2c the community is in the PDU header; for v3 the username requires SNMPv3 header parse. So "community allowlist pre-decode" is incorrect.
- **Recommended fix**: define three allowlist layers:
  1. **UDP-peer IP allowlist** — pre-decode, cheapest. Drops at the wire.
  2. **Community/v3-username allowlist** — after minimal ASN.1 header parse (cheap; no MIB lookup needed). Drops before full PDU decode.
  3. **PDU-source allowlist** — after full decode, per `agent-addr`/`snmpTrapAddress`. Validates the *logical* device identity, useful for NAT environments where multiple devices share a peer IP.
- State all three, with priority. Each layer increments a distinct dimension on `snmp.trap.errors` (see W19a).

### W20. (major) Volatile varbinds in default dedup key trivially bypass dedup

- **Target evidence**: `netdata.md §10 L394` "Key varbinds are profile-specified (default: all non-timestamp varbinds)."
- **Cohort contradiction**: Zenoss (`zenoss.md §5 L329-330`) dedupid is `device|component|eventClass|eventKey|severity` — small set of high-stability identifiers, not "all varbinds." OpenNMS reduction-key templates (`opennms.md §5 L274-279`) are operator-curated subsets of varbinds.
- **Why netdata.md gets this wrong**: "all non-timestamp varbinds" includes volatile counters (e.g., `ifInErrors`, `bgpPeerInUpdates`) and per-event identifiers (e.g., `eventCounter`). Two BGP backward-transition traps 1s apart have different `bgpPeerInUpdates` values; they hash to different fingerprints; dedup never triggers. A flapping BGP peer's storm is not dedup'd despite the operator's intent.
- **Recommended fix**: the dedup default must be **identifier varbinds only**, not all varbinds. The profile defines `dedup_key_varbinds:` explicitly (already in §7 L208 for ciscoPsmTrapSrvUnauthorized). The default for OIDs without an explicit `dedup_key_varbinds:` should be the most-stable subset — either (a) `(source_device, trap_OID)` only (matches Centreon's coarse model), or (b) `(source_device, trap_OID, *index-named varbinds*)` where index-named is per the MIB's INDEX clause. **"All non-timestamp varbinds" is the wrong default.**

### W21. (minor) Template substitution must escape `{...}` in varbind values

- **Target evidence**: `netdata.md §7` restricted Go-template helpers such as `{{value "varname"}}`; `description: 'Port-security violation: MAC {{value "cpsIfViolationMacAddress"}} ...'` etc.
- **Issue**: if a varbind value contains literal template delimiters (a hostile/curated device, or a MIB octet-string containing user content), unguarded recursive rendering may produce nested template expansion. The design says "Hot-path substitution is bounded-size buffer fill" (§7 L245) but does not specify single-pass vs recursive.
- **Cohort contradiction**: SNMPTT (`nagios-snmptt.md §4`) FORMAT substitutions are single-pass and treat varbind values as opaque. Centreon (`centreon.md §5`) substitutes positionally with no recursive expansion.
- **Recommended fix**: state explicitly that template execution is single-pass, varbind values are treated as opaque strings, and template delimiters inside varbind values are not interpreted.

### W23. (major) Journal field injection via newline / null / field-separator in varbind values

- **Target evidence**: `netdata.md §7` restricted Go-template helpers such as `{{value "varname"}}`; §11 journal entry example with MESSAGE constructed from templated varbind values; §7 only states "MESSAGE capped at 512 chars."
- **Issue**: a malicious or buggy device can send a trap whose octet-string varbind value contains `\n` (newline), `\r`, `\0` (null), or systemd-journal field-separator sequences. Unsanitized substitution into `MESSAGE=...` allows the attacker to inject arbitrary trusted fields (e.g., `\nPRIORITY=0` to elevate alert priority, `\nSYSLOG_IDENTIFIER=...` to spoof origin). CWE-117 (Log Injection) class.
- **Cohort contradiction**:
  - `librenms.md §5 L285`: explicitly notes that "multi-line varbind values and lines without a space are silently mishandled" — LibreNMS's parser is non-defensive. Netdata is targeting a higher quality bar; it must defend.
  - `centreon.md L143-148`: writes 4096-byte atomic lines but does not sanitize newlines in payload.
  - Lens (`../domain/snmp-traps-in-observability.md §11.4`): silent credential mismatch + lens §11.7 silent-dedup are operational anti-patterns; journal-injection is a security anti-pattern.
- **Recommended fix**:
  1. Template substitution sanitizes varbind values before MESSAGE insertion: strip / replace `\n`, `\r`, `\0`, and any byte < 0x20 with a printable substitute (e.g., `\x0a` literal).
  2. `SNMP_TRAP_JSON` uses a strict JSON encoder that escapes control characters per RFC 8259.
  3. Plugin self-test: emit a synthetic trap with control characters in varbind values; verify the journal entry has the expected sanitized MESSAGE and no injected fields.
  4. Add `snmp.trap.errors.sanitized` dimension that increments per trap whose varbind values required sanitization (operator signal for malformed-payload device discovery).

### W24. (major) DoS via unbounded BER decode — per-packet amplification

- **Target evidence**: `netdata.md §5 L134-135` "UDP recv into a reusable buffer (no allocation). BER decode in place (no copy)."
- **Issue**: a UDP datagram is up to 65,507 bytes. An attacker can craft a maximally large PDU with hundreds of varbinds, deeply nested SEQUENCE constructed types, or pathologically-encoded length fields. The hot path claims "zero allocation" but a malicious PDU may require unbounded CPU (deep recursion / quadratic ASN.1 parsing) or scratch buffer growth. The cohort's storm discussion focuses on *volume*, not *per-packet* amplification.
- **Cohort contradiction**:
  - `telegraf.md §10`: single-goroutine ceiling is on PDU decode + OID translation; a malicious PDU could exhaust the budget on one packet, blocking all subsequent traps.
  - `splunk-sc4snmp.md §3.5`: pysnmp's ASN.1 decode is Python; max-size PDU would amplify the cost.
  - `opennms.md §5`: SNMP4J handles BER decode; malformed PDUs are caught and dropped, but the cost is paid per-packet.
- **Recommended fix**: state explicit decode-time limits in §5:
  1. Max UDP recv size: e.g., 8 KB (well above typical trap size but bounded).
  2. Max varbinds per PDU: e.g., 256.
  3. Max BER decode depth: e.g., 8.
  4. Per-PDU decode time budget: e.g., 1ms; over-budget packets are dropped and logged.
  5. Add `snmp.trap.errors.malformed_decode_ddos` dimension that increments per dropped packet exceeding any limit.

### W22. (major) `dimension_from_varbind` cardinality check requires MIB knowledge

- **Target evidence**: `netdata.md §7.5 L296` "If `dimension_from_varbind` references a varbind that can take unbounded values (MAC, IP, username), the plugin REJECTS the config at load with a clear error."
- **Issue**: how does the plugin know which varbinds are bounded vs unbounded? This is MIB-derived knowledge. If the operator configures `dimension_from_varbind: someVendorVarbind` for an OID whose MIB is not loaded, the plugin cannot validate. Result: silent acceptance, cardinality explosion at runtime.
- **Cohort contradiction**: Telegraf (`telegraf.md L355`) cardinality discipline is per-tag vs per-field, but Telegraf has no runtime cardinality check on tag values — operators are warned, not blocked. None of the cohort implements compile-time cardinality enforcement.
- **Recommended fix**: the plugin must either (a) reject `dimension_from_varbind` for any OID without loaded MIB coverage, or (b) maintain a built-in allowlist of known-bounded varbind names (e.g., `ifOperStatus`, `bgpPeerState`, `ccmHistoryEventTerminalType` — enum-valued varbinds from common MIBs) that can be checked without MIB load. (a) is safer; (b) is more operator-friendly. The design must pick.

---

## §4 Risky assumptions

### A1. "~30k rows/sec per writer thread" is an existing Netdata benchmark

- **Assumption**: `netdata.md §5 L126`, §9 L362.
- **What would invalidate it**: the actual benchmark is older than the design, was measured on different hardware, was measured under different workload, or doesn't exist.
- **Suggested verification**: cite the commit, the test file, the hardware, and the workload. If the number was measured under "30k rows/sec of journal entries with 200-byte MESSAGE and 4 standard fields," the SNMP-trap workload (MESSAGE up to 512 bytes + ~12 fields + ~500 byte JSON varbind payload) is **2-4× heavier per row**, lowering the ceiling proportionally.

### A2. Two writer threads → 60k/sec

- **Assumption**: §9 L368-370 "two writer threads achieve ~60k entries/sec."
- **What would invalidate it**: contention on the systemd-journal-writer side (filesystem fsync, FS journal, kernel inode locks) when two writers append to different journal files in the same filesystem.
- **Suggested verification**: actual measurement with two writers on the same filesystem, then on separate filesystems, to characterize the FS-contention cost.

### A3. "Both Rust and Go are workable; defer the call until prototype benchmarks"

- **Assumption**: §5 L114-121.
- **What would invalidate it**: one language has a critical capability the other lacks (e.g., Rust `cap_net_bind_service` integration; Go gosnmp's INFORM correctness; Rust SNMP4J-equivalent maturity). The design's "defer to prototype" is a punt, not a decision.
- **Suggested verification**: Phase B should propose a concrete benchmark plan with named libraries (gosnmp 1.42 (used by Netdata polling) for Go; `pdu-snmp` or `snmp4j-jni` for Rust). The choice cannot be "either is workable" without a prototype — that's a not-a-decision.

### A4. inotify hot-reload of MIBs is operationally safe

- **Assumption**: §7 L253 "watches the directory via inotify; compiles new MIBs on file change; updates the in-memory MIB index without restart."
- **What would invalidate it**: a half-written file during operator `cp foo.mib /etc/netdata/snmp-mibs/` triggers a compile with the unfinished file; the compile fails halfway and leaves the in-memory index inconsistent.
- **Suggested verification**: define the file-completeness check (atomic rename only? size + mtime stable for 1s? `.tmp` → final rename pattern?). Cohort: Datadog uses `ddev meta snmp generate-traps-db` as a discrete build step (`datadog-agent.md L400-408`); LogicMonitor uses portal upload with a validation step (`logicmonitor.md L320-339`).

### A5. systemd-journal handles "tens of thousands of rows/sec sustained"

- **Assumption**: implicit throughout §9, §11.
- **What would invalidate it**: systemd-journal benchmarks (publicly available on Lennart Poettering's blog from 2012, GitHub issues, kernel mailing lists) show throughput in the 5-15k/sec range on commodity SSD with default config; 100k/sec is possible only with `Storage=volatile` (RAM-only), `Compress=no`, etc. Netdata's deployment target (Linux/glibc/musl) means relying on the user's journald config.
- **Suggested verification**: cite the actual systemd-journal throughput benchmark Netdata measured. If Netdata's "Rust systemd-journal writer" is a Netdata-internal writer that uses the systemd binary format on disk **but writes directly** (bypassing journald), say so — that's a different architecture with different failure modes.

### A6. ~28,000 MIB files dedupe to 8-12k unique modules

- **Assumption**: `netdata.md §8 L324` "Deduped: estimated 8,000-12,000 unique MIB modules."
- **What would invalidate it**: actual dedup. Each "MIB module" is identified by module name (top-level `DEFINITIONS ::= BEGIN`). The same module can exist in multiple sources with different OIDs, different versions, different revisions, different vendor-extension content. A simple `grep -l 'DEFINITIONS' | sort -u` won't dedupe correctly; conflicting versions of the same module name from different vendors is the most common case.
- **Suggested verification**: run the actual dedup tool (which doesn't exist in the design yet) and report the count.

### A7. 5-second dedup window is appropriate

- **Assumption**: §10 L398 "default e.g., 5 seconds."
- **What would invalidate it**: cohort numbers — Centreon defaults to 1s (`centreon.md L356`); OpenNMS's reduction-key dedup has no window (alarm-scoped); Zenoss dedup has no window (event_summary lifetime is). 5s is the upper end of the cohort, not a sensible default.
- **Suggested verification**: pick a deliberate default. Recommend 1s like Centreon for the same OID + source + key-varbinds tuple; longer windows (e.g., 60s) for "trap storm protection" deserves a separate config knob.

---

## §5 Missing features

Features the cohort considers essential that `netdata.md` does not address.

### M1. Clear-pair semantics (auto-clear `linkDown` ↔ `linkUp`)

- **Cohort coverage**: 4 of 16 ship paired-clear: OpenNMS (`opennms.md §5` `clear-key`), Zenoss (`zenoss.md §5` `zEventClearClasses` + `clear_fingerprint_hash`), LogicMonitor LogSource auto-close, CheckMK `match_ok` cancelling rules. 12 of 16 are operator-built.
- **Why it matters**: the lens (`../domain/snmp-traps-in-observability.md §6.5 Absence of Clear Events`) calls this a Stage-4 maturity indicator. Without paired-clear, a missed `linkUp` (UDP loss) leaves the interface "down" in the journal forever (`../domain/snmp-traps-in-observability.md §11.2`).
- **Recommendation**: **deferred to Phase 2 / Phase 3** (alignment with implications-file Phase 4). Surface loud in §14 Non-Goals of `netdata.md` for first release. The W4 dedup change (1s default + suppressed-count metric) plus operator paired-clear via the existing alert engine (e.g., alert when `linkDown` is open and no `linkUp` arrives within 5 min, then reconcile via `ifOperStatus` poll) is the first-release workaround. The profile field `clear_trap_oid:` is a Phase 2/3 deliverable.

### M2. Topology-aware suppression

- **Cohort coverage**: 0 of 16 ship this as a built-in feature (`./operator-features.md T3-06`).
- **Why it matters**: the lens (`../domain/snmp-traps-in-observability.md §6.4 Topology-Aware Correlation`) calls this a Stage-4 indicator. Netdata is uniquely positioned because the hub already has topology (`netdata-snmp-hub-architecture.md`).
- **Recommendation**: in-scope for the cohort-win audit. `netdata.md §16 L621` already claims "**Hub-local enrichment makes this cheap.**" — but the design provides no implementation detail. Add a §13 Open Question: how does the alert engine suppress downstream alerts when upstream is in alarm? What state does the journal carry to enable this?

### M3. INFORM-acknowledgement under the hub design

- **Cohort coverage**: 4 fully (OpenNMS, Zenoss, Centreon, Zabbix); CheckMK ✗; Dynatrace ✗.
- **Why it matters**: `netdata.md §6 L162` lists INFORM as in-scope. But INFORM under the hub design means: the hub must respond with a Response PDU on a separate UDP port back to the originating device. If the hub is behind a NAT relative to the device (per `../domain/snmp-traps-in-observability.md §11.4 NAT-Obscured Trap Source`), the INFORM Response goes to the wrong place.
- **Recommendation**: state the operational expectation: INFORM requires direct routing back to the device; not all hub deployments will support it.

### M4. Per-row cost forensics (the storage cost the design hides)

- **Cohort coverage**: SolarWinds 90-min DELETE pain (`solarwinds.md L419`); Centreon 2,048-byte truncation (`centreon.md L530-536`); Nagios+SNMPTT MyISAM no-index full-table scan (`nagios-snmptt.md L1098`).
- **Why it matters**: §11 `SNMP_TRAP_JSON` is full structured varbind payload as single-line JSON. Per-row cost is ~500-2000 bytes for typical Cisco/Juniper traps. At 10k traps/sec sustained, this is ~5-20 MB/sec write rate. At default systemd-journal compression, ~1-5 MB/sec on disk. Per hour, 3.6-18 GB. Per day, 86-432 GB. Per 30 days, 2.6-13 TB. **The design has no statement of expected storage cost.**
- **Recommendation**: §11 must record an estimated storage budget and recommended retention.

### M5. Northbound trap re-emit

- **Cohort coverage**: 7 of 16 ship native northbound (OpenNMS, Zenoss, LibreNMS, Cribl, SolarWinds, Centreon, Sensu); 9 of 16 do not.
- **Why it matters**: enterprise customers with layered NMS hierarchies (manager-of-managers pattern, `./operator-features.md T3-01`). The design (`netdata.md §13 L577`) defers this to a separate SOW.
- **Recommendation**: keep deferred, but document the Cloud-aggregated-for-presentation choice as a partial substitute for centralized observability.

### M6. Operator alert UX path

- **Cohort coverage**: 13 of 16 ship some form of operator alert authoring UX (`./operator-features.md T2-03`).
- **Why it matters**: the design says §0 L6 "Alerting (existing Netdata alert engine) ... handled by existing Netdata subsystems." But the cohort norm is that trap-driven alerting requires authoring per-OID rules — the existing Netdata alert engine doesn't have OID-aware authoring UX today.
- **Recommendation**: validate that the existing Netdata alert engine's alert authoring UX supports OID-aware filtering (e.g., "alert when `snmp.trap.cisco_port_security` rises with label `interface=Gi0/1`"). If it does, cite the existing UI page. If it doesn't, this is a real gap.

### M7. Audit logging of trap-rule changes

- **Cohort coverage**: 8 of 16 ship audit logging (OpenNMS, Zenoss, CheckMK, Centreon, Zabbix, Dynatrace, LogicMonitor, SolarWinds).
- **Why it matters**: SOX / ITGC compliance asks for who-changed-what-when. `netdata.md §0 L6` defers to "existing Netdata Cloud spaces/rooms."
- **Recommendation**: verify the existing Netdata dyncfg audit log records changes to trap configuration. If not, surface this as a gap.

### M8. Test fixture corpus (per-vendor `.trap` fixtures, PCAPs)

- **Cohort coverage**: Zenoss `trapdump.pcap` (`./fixture-inventory.md §2.2`), LibreNMS 80 templated text fixtures (`./fixture-inventory.md §6`), Splunk SC4SNMP pysnmp v1/v2c/v3 tests (`./fixture-inventory.md §12`).
- **Why it matters**: a new trap-receiver implementation needs test fixtures. The design has no §17.7-equivalent fixture-corpus commitment.
- **Recommendation**: §17 should explicitly include "ingest the Zenoss PCAP, LibreNMS text fixtures, OpenNMS smoke-test fixtures into the Netdata test suite" as a Day-1 deliverable.

---

## §6 Internal inconsistencies

### I1. "Single writer per file" vs "scale beyond one thread by partitioning"

- **§5 L126**: "Single writer per journal file. The journal writer caps at ~30k rows/sec per writer thread."
- **§9 L366-370**: "To exceed 30k entries/sec total, scale horizontally with isolation: Multiple journal files, each with its own writer thread."
- **Contradiction**: §5 establishes the cap as a fact about a single writer; §9 treats it as a knob. If §9 is true, then "single writer per file" is not the limit — the limit is per-file. The design conflates "writer" with "writer thread" and with "writer file."
- **Recommended resolution**: clarify that the bottleneck is per-file fsync rate, not per-writer-thread; multiple writer threads contending on one file is the wrong scaling strategy; multiple files (each with one writer) is the right one.

### I2. "Hot path: zero allocation" vs "Compute fingerprint: hash(source_device, trap_OID, key_varbinds)"

- **§5 L125**: "single thread per listener, zero-allocation."
- **§10 L394**: "Compute fingerprint per trap: hash(source_device, trap_OID, key_varbinds). Key varbinds are profile-specified (default: all non-timestamp varbinds)."
- **Contradiction**: hashing a variable-length set of varbinds (default = all non-timestamp varbinds, which can be 0-50+ varbinds with octet-string values up to 256 bytes each) requires either (a) per-trap allocation to build a fingerprint input, or (b) a streaming-hash interface that doesn't allocate but is more complex to implement. The design doesn't specify which.
- **Recommended resolution**: state which hash function (XXH3 fits the per-packet budget; SHA-1 like Zenoss does not), how the input buffer is reused, and what the max fingerprint-input size is.

### I3. "Per-device vnode" vs "Per hub instance" for errors context

- **§12 L530 Context 1 `snmp.trap.events`**: "Instance | Per device (one instance per source device the hub knows about)"
- **§12 L549 Context 2 `snmp.trap.errors`**: "Instance | Per hub (one instance per Netdata hub agent)"
- **Inconsistency**: per-device vnode for events, per-hub for errors. Operators querying errors won't know which device caused them.
- **Recommended resolution**: keep per-hub for errors but add `source_device` label always (already done at L553 "possibly source_device where source is identifiable"). Strengthen the "possibly" to "always when known."

### I4. "Profile YAML defines vendor knowledge ONLY" vs "Operator overrides via dyncfg"

- **§7 L168**: "Profile YAML defines vendor-curated knowledge ONLY. It does NOT define journal field names (the journal always captures all varbinds — see §11), and it does NOT define metric emission (that lives in plugin configuration — see §7.5)."
- **§7 L211**: "name overrides the MIB symbolic name (use when MIB isn't loaded for this OID)."
- **§7 L253**: "Operator-provided overrides live in `/etc/netdata/snmp-profiles/` and take precedence."
- **Inconsistency**: profile YAML is "vendor knowledge only" but also is "operator-overridable" via the `/etc/netdata/snmp-profiles/` path. The category override is in §7.5 (plugin config), but the symbolic name override is in profile YAML. Operators have two places to override.
- **Recommended resolution**: pick one. Either operator overrides go in plugin config (and `/etc/netdata/snmp-profiles/` is read-only), or both can override and the precedence is spelled out.

### I5. "MESSAGE field capped at 512 chars" vs "high-cardinality detail (packet content)"

- **§4 L87**: "Description template → `MESSAGE` field | **No restriction** — use any varbind"
- **§7 L245**: "MESSAGE capped at 512 chars; truncated with marker if exceeded."
- **Inconsistency**: §4 promises "no restriction"; §7 imposes 512 chars. Operators templating a `SNMP_TRAP_JSON` content into MESSAGE (e.g., to make a tcpdump-style summary) will hit the cap silently.
- **Recommended resolution**: clarify that "no restriction" means "no metric-label cardinality restriction"; the MESSAGE byte cap is real. Mention the truncation marker explicitly: `... [+N bytes truncated]`.

### I6. "INFORM in scope" vs "DTLS/TLS-TM conditional"

- **§6 L162**: "INFORM acknowledgement support (Cisco/Juniper devices often configure INFORM by default)."
- **§6 L164**: "DTLS / TLS-TM: in scope for first release if mature libraries exist in the chosen language. Phase A finding: zero cohort systems support this (universal gap)."
- **Mild inconsistency**: INFORM is in scope unconditionally; DTLS/TLS-TM is conditional on library maturity. Both are protocol features; the choice rationale is asymmetric.
- **Recommended resolution**: state the criterion uniformly. INFORM-Response over UDP is well-established in gosnmp 1.42 (Netdata polling already uses); DTLS-TM is rare. Both should be conditional on library support, with INFORM being a near-certain yes and DTLS being a likely no.

### I7. "Cohort-Win Audit (what this design wins relative to the cohort)" vs absent mention of OpenNMS event-XML coverage

- **§16 L617-624**: lists 5 cohort gaps the design wins on.
- **Missing**: the design does not claim to win on cohort-typical features it actually loses (Datadog `dd_traps_db` precompiled bundle, OpenNMS 17,442 event definitions, SolarWinds 250k OIDs). The §8 OOB strategy promises to be Datadog-comparable but the §16 audit does not call out the head-to-head.
- **Recommended resolution**: add an honest "where we are not yet at cohort par" section: pre-existing alert-pack catalogues from OpenNMS / Datadog / SolarWinds will take time to match.

---

## §7 Hidden complexity

### H1. The MIB compiler

- **Target claim**: §7 L253 inotify hot-reload + compile + in-memory MIB index swap.
- **Hidden complexity**: SMIv1/v2 MIBs have transitive `IMPORTS` chains (`netdata.md §8 L325` mentions pysnmp's MIB compilation chain). An operator drops `foo.mib` into `/etc/netdata/snmp-mibs/` that imports `bar.mib` which imports `baz.mib`. baz isn't there. The compile must (a) fail with a clear error, (b) compile partial, (c) queue for retry. The cohort's experience with this is operationally painful: OpenNMS doc warns explicitly "the operator must locate, upload, and compile each dependency MIB themselves" (`opennms.md §4 L207-208`). Zenoss `zenmib` SubprocessJob can fail.
- **Cohort evidence that this is hard**: `opennms.md §4 L207-208`; `splunk-sc4snmp.md §4 L240-261` (mibserver as a separate container); `centreon.md §4 L281-286`; `datadog-agent.md §4 L400-408` (build-step, not runtime).
- **Recommended treatment**: state the dependency-resolution behaviour explicitly. Recommend bundling pysmi (or equivalent) as a vendored library, and document the fallback to OID-only decode when a dependency is missing.

### H2. The conversion tools

- **Target claim**: §8 L334-348 OpenNMS XML → YAML, Centreon SQL → YAML, Zenoss ZenPack `objects.xml` → YAML, LibreNMS PHP handlers → YAML, MIB tree → YAML, SNMPTT `.conf` → YAML.
- **Hidden complexity**: each conversion is a multi-week project. OpenNMS event XML has 17,442 entries across 230 files. The XML schema includes `<mask>` (id/generic/specific/varbind matchers), `<uei>`, `<alarm-data>` (reduction-key, clear-key, alarm-type), `<varbindsdecode>` (enum-to-string maps). Mapping `<alarm-data>` semantics (which depend on a stateful alarm engine) into Netdata's stateless YAML is not lossless. LibreNMS PHP handlers are Turing-complete; "heuristic — handler names map to OID families" (§8 L337) is going to miss the handler's actual behaviour.
- **Cohort evidence**: `opennms.md §13`, `zenoss.md §13`, `librenms.md §5` (181 OID→class registry; handlers update `ports.ifOperStatus` directly — non-portable to YAML), `nagios-snmptt.md §4 L259-272` (`snmpttconvertmib`).
- **Recommended treatment**: §17 sequencing should reflect realistic effort: each conversion tool is its own work item (2-4 weeks each), not "Conversion tools (OpenNMS, Centreon, LibreNMS, Zenoss, public MIBs)" as a single step 7.

### H3. The `dd_traps_db`-equivalent

- **Target claim**: §8 L323-327 28,200 raw MIB files → 8-12k unique modules → "dd_traps_db-equivalent built end-to-end from public sources."
- **Hidden complexity**: Datadog's `dd_traps_db.json.gz` is a build-time artefact. The Datadog compiler (`datadog-agent.md §4 L400-408`, `ddev meta snmp generate-traps-db`) does multi-stage parsing: read MIB → resolve imports → emit JSON. The Netdata design implies the same flow exists "for free" because pysmi and pysnmp do the work. But Datadog has a closed-source 3,652-MIB-curated build target ((`./profile-inventory.md §6` says verified count 3,652, vendor claims 11,000+). Netdata's 8-12k is the **raw deduped count**, not the curated/tested count.
- **Cohort evidence**: `datadog-agent.md §4 L394` "more than 11,000 MIBs" is a marketing number; the verified count is 3,652. `./profile-inventory.md §6`: "the actual file ships inside the closed Omnibus installer."
- **Recommended treatment**: state the target as "5,000 OIDs curated for severity + category + symbolic name + display_hint" or similar. Don't promise 8-12k unique modules with no quality criterion.

### H4. The journal vacuum / rotation / corruption recovery story

- **Target claim**: §11 L437-441 "every trap, always, no exceptions, lands in the journal with all its varbinds. This holds whether or not the OID is in the MIB index. The journal entry is the source of truth."
- **Hidden complexity**: systemd-journal has a known set of failure modes — `MaxFileSec`, `MaxRetentionSec`, journal corruption recovery via `journalctl --verify` and `journalctl --rotate`. The Netdata Rust journal writer (per §5 L118) is "the existing Netdata journal writer" — but the operator-facing behaviour (what happens when the journal file is corrupted? does the listener keep accepting traps and queue to a fresh file? does it drop?) is unstated.
- **Cohort evidence**: `zenoss.md §6 L390` event_archive 3 days → archive; 90 days purge — even Zenoss with a stateful DB has explicit retention. systemd-journal doesn't have "archive" semantics; rotation is by size/time only.
- **Recommended treatment**: §11 must spell out the rotation policy, the recovery path, and the operator-visible alert when journal is corrupted.

### H5. The Cloud-presentation layer for traps

- **Target claim**: `netdata-snmp-hub-architecture.md` "Cloud aggregates for presentation; not for correlation."
- **Hidden complexity**: aggregation across hubs requires a unified schema. Each hub writes `SNMP_TRAP_JSON` in its own ad-hoc shape (varbind names from the locally-loaded MIBs); two hubs running different MIB versions may produce different field names for the same OID. The Cloud query must reconcile. This isn't free.
- **Cohort evidence**: `dynatrace.md §3.5` says ActiveGate-side traps reach Grail and are presented uniformly because Grail enforces a schema. Netdata Cloud doesn't have this enforcement today.
- **Recommended treatment**: a §0-level statement: "Cloud-side trap aggregation across hubs is a follow-up SOW; first release shows traps per-hub only."

### H6. Per-OID metric naming

- **Target claim**: §12 L564 "Naming convention: `snmp.trap.<vendor>_<short_name>` (e.g., `snmp.trap.cisco_config_changes`)."
- **Hidden complexity**: a trap from a multi-vendor MIB (e.g., `IF-MIB::linkDown`) doesn't have a "vendor." The naming convention breaks for IETF MIBs. Some MIBs are owned by one vendor but contain traps that fire on third-party gear (Cisco shipping IETF-MIB extensions).
- **Recommended treatment**: state the fallback: `snmp.trap.<mib_module>_<symbol>` when no vendor binding exists; document the exception list.

### H7. The dyncfg story for plugin config

- **Target claim**: §7.5 L257 "The plugin's own configuration (in /etc/netdata/..., dyncfg-editable)."
- **Hidden complexity**: dyncfg is a Netdata-internal config plane (per the project knowledge skill). Per-OID metric opt-in via dyncfg means each metric is a dyncfg resource. The cohort's experience with per-OID UIs is that they get unwieldy fast (Centreon's traps table at hundreds of rows). For tens of opt-in OIDs the design is fine; for thousands it isn't.
- **Recommended treatment**: state the scale: "first release supports up to N per-OID opt-in metrics; beyond N, the operator works in the underlying YAML files."

---

## §8 Open questions (Phase B couldn't resolve)

### Q1. Is the "existing Netdata Rust journal writer" the right primitive?

The design names it (`netdata.md §5 L118`). I couldn't verify which Netdata commit, library, or test contains this writer's actual benchmark. The "30k rows/sec" number is asserted but uncited. **Phase B can't tell if this is real or aspirational without source pointer.**

### Q2. Is the systemd-journal storage tier really "no new storage tier"?

The journal is on the same filesystem as the rest of Netdata's data. If Netdata's storage budget on a hub is X GB and traps + their JSON varbind payload at 10k traps/sec is 5-20 MB/sec, the journal consumes 432 GB/day at the high end — that's a significant fraction of any reasonable hub deployment's disk budget. **The design doesn't acknowledge this.**

### Q3. Does the existing Netdata alert engine support OID-aware alert authoring?

§0 L6 defers alerting to "the existing Netdata alert engine." The cohort universally requires per-OID rule authoring; the cohort's experience is that the alert authoring UX is critical (`./operator-features.md T2-03`). I couldn't verify from the design whether the existing engine has OID-aware UX.

### Q4. What does Cloud do with traps when hubs disagree on MIB versions?

The hub model commits to "each site has its own MIBs." A trap from a Cisco device might arrive at hub A with `ifAdminStatus` (named) and at hub B (which lacks the Cisco MIB on disk) with `1.3.6.1.2.1.2.2.1.7` (numeric). Cloud queries across both. **The design doesn't address this.**

### Q5. What happens to in-flight traps on plugin restart?

§5 L128-130 "the plugin lifecycle follows the Netdata agent's lifecycle." During a restart, the dedup cache is lost (in-memory, §10 L394), the UDP socket closes, kernel queue drops on overflow. **The design doesn't specify the operator-visible recovery behaviour.**

### Q6. How does enrichment via netipc behave when the polling plugin is down?

`netdata-existing-netipc.md` describes netipc as the cross-plugin RPC. If the polling plugin is unhealthy, the trap listener's enrichment lookup fails. Does the trap fall through to journal-with-just-source-IP, or does it block? **The design says "in-process state (Go) or netipc (Rust)" but doesn't specify failure modes.**

### Q7. What is the storage cost of the 28,200 raw MIB tree on the hub?

The design proposes shipping comprehensive vendor profile coverage. Where do the MIBs live on disk? Are they bundled (then how big is the agent install)? Are they downloaded on first use? **Unstated.**

### Q8. INFORM Response under multi-listener?

§6 L158 supports multiple listeners. Each listener accepts INFORM. The Response PDU must go back to the originating device, via the listener that received it. **The design doesn't explicitly state INFORM Response uses the same listener's UDP socket as the receive path.**

### Q9. v3 USM key rotation interaction with the hot-registered engine-ID pattern?

§5 L153 hot-registers engine-ID + username from the wire. When the operator rotates the USM key, what happens? Is the hot-registered pair invalidated? **The design doesn't say.**

### Q10. Is the existing Logs UI capable of substring-searching the templated MESSAGE field?

§11 L495 promises "fully searchable in the journal (Logs UI substring search, journalctl _MESSAGE_MATCH=aa:bb:cc". I couldn't verify from the design whether Netdata's existing Logs UI does substring search on MESSAGE or only field-equality matches.

---

## §9 Phase B integrity log

For each of the 25 input files (target + 6 comparison artefacts + foundational lens + hub-architecture premise + 16 per-system specs), filename, line count, and exact last non-empty line (regenerated mechanically via `wc -l` + `awk 'NF{l=$0} END{print l}'`).

Target:

1. **`netdata.md`** (642 lines, last line: "End of design proposal. Awaiting Phase B stress-test.")

Comparison artefacts (6):

2. **`./feature-matrix.md`** (1,375 lines, last line: "Read `logicmonitor.md` in whole (1138 lines, last line: \"preserving cross-system framing alignment.\")")
3. **`./design-forks.md`** (398 lines, last line: "Read `logicmonitor.md` in whole (1138 lines, last line: \"preserving cross-system framing alignment.\")")
4. **`./profile-inventory.md`** (179 lines, last line: "Read `logicmonitor.md` in whole (1138 lines, last line: \"preserving cross-system framing alignment.\")")
5. **`./fixture-inventory.md`** (252 lines, last line: "Read `logicmonitor.md` in whole (1138 lines, last line: \"preserving cross-system framing alignment.\")")
6. **`./alerting-models.md`** (247 lines, last line: "Read `logicmonitor.md` in whole (1138 lines, last line: \"preserving cross-system framing alignment.\")")
7. **`./operator-features.md`** (305 lines, last line: "Read `logicmonitor.md` in whole (1138 lines, last line: \"preserving cross-system framing alignment.\")")

Foundational lens + hub-architecture premise (2):

8. **`../domain/snmp-traps-in-observability.md`** (1,038 lines, last line: "---")
9. **`netdata-snmp-hub-architecture.md`** (114 lines, last line: "End of document.")

Per-system specs (16):

10. **`opennms.md`** (1,405 lines, last line: "**accepted** — convergence declared after 5 iterations. Document is decision-grade for the comparative analysis. Surviving precision items noted above will not be revised further in this SOW; if any are uncovered later as material errors during the cross-system comparison phase, this file can be reopened as a regression per the SOW process.")
11. **`zenoss.md`** (1,322 lines, last line: "Final verdict: this document accurately represents Zenoss's SNMP trap implementation as of `zenoss-prodbin @ bc1ca09` (7.4.0 develop), `zenoss-zep @ dd7c68e`, `pynetsnmp @ b747af1`, `zenoss-protocols @ a612fee`, and `zenoss-protobufs @ 3c527aa`. Brutal-honesty findings (the IPv6 hack at `net.py:65`, the `egpNeighorLoss` typo at `handlers.py:161`, the `replay.py:88` typo, the OID-map interval inconsistency, the silent-drop of v3-with-bad-credentials at default INFO level, the SNMPv3Action session resource pattern, the `disable-event-deduplication` config-doc confusion, the v1 agent-addr 0.0.0.0 override, the missing PBDaemon-overflow Events Console event, the sparse day-1 vendor mapping count) are all source-verified and documented in §17.")
12. **`checkmk.md`** (1,369 lines, last line: "**accepted** — convergence declared after 3 iterations. Document is decision-grade for the comparative analysis. Surviving precision items have been applied or noted in the iteration logs; further iteration would surface only editorial micro-precision items that do not affect the analytical conclusions.")
13. **`centreon.md`** (1,755 lines, last line: "Surviving findings are precision-improvement shape: line-range precision (±5 lines), wording-tone preferences, secondary-source mentions (the broader `web.yml` CI workflow). These do not affect the document's utility for the Netdata trap-design discussion in the upcoming comparative-analysis document.")
14. **`zabbix.md`** (1,596 lines, last line: "The qwen reviewer's persistent 30-minute timeouts (5/5 attempts across iter-2 to iter-5) are recorded as a reviewer-infrastructure issue, not a content issue; the other 5 reviewers' verdicts are dispositive. Five contributing reviewers, three clean accepts plus one no-blocker no-major plus one with internal-consistency majors-already-fixed, satisfies the SOW convergence threshold.")
15. **`librenms.md`** (1,394 lines, last line: "**accepted** — convergence declared after 5 iterations. Document is decision-grade for the comparative analysis. Surviving precision items (additional handler / test / utility coverage in the missed-content appendices) will not be revised further in this SOW; if any are uncovered later as material errors during the cross-system comparison phase, this file can be reopened as a regression per the SOW process.")
16. **`nagios-snmptt.md`** (1,751 lines, last line: "- The single remaining ambiguity (LOGONLY field position) is upstream's, not this analysis's; both interpretations are acknowledged explicitly in §5 Phase 7, §9, and §20.")
17. **`sensu.md`** (1,299 lines, last line: "End of document.")
18. **`telegraf.md`** (1,371 lines, last line: "This spec is delivered as **converged at iter-3**: structurally compliant, factually accurate, all reviewer-flagged in-scope findings addressed across iter-1/iter-2/iter-3, with documented model-side reviewer instability (qwen across all three iterations; glm/kimi/mimo at iter-3) as the only reason the formal \"≥3 outright accept\" target was not met.")
19. **`logstash.md`** (1,244 lines, last line: "**accepted** — convergence declared after 3 iterations. Document is decision-grade for the comparative analysis of how Logstash implements SNMP traps. The trap-as-document-in-Elasticsearch model is faithfully described without projecting NMS-style features; every architectural claim is source-cited (with explicit `Inferred` / `Vendor-documented, not source-verified` / `Unverified` labels where appropriate). The SNMP4J backend attribution was upgraded from inferred to source-verified mid-iter-3 via the missed-content discovery at `elastic/built-docs :: raw/en/logstash/8.14/plugins-integrations-snmp.html:98-101`.")
20. **`datadog-agent.md`** (1,560 lines, last non-empty line: "- The spec's evidence base, line counts, claim/source consistency, and reviewer-pass log are now coherent with the source tree at `datadog/datadog-agent @ 2c813592` and `datadog/integrations-core @ 411c31db`.")
21. **`splunk-sc4snmp.md`** (1,377 lines, last line: "These are accepted as deferred polish items rather than blockers; the document remains source-faithful and template-complete.")
22. **`cribl.md`** (1,258 lines, last line: "Not run — convergence declared at iter-2 per the SOW stop rule.")
23. **`solarwinds.md`** (1,114 lines, last line: "None of the surviving items affect the analytical claims of the file. All vendor-cited claims trace to URLs in §19; all operator-reported claims are explicitly tagged.")
24. **`dynatrace.md`** (1,135 lines, last line: "**Iter-5 not required.** This document is at convergence per the SOW stop rule.")
25. **`logicmonitor.md`** (1,138 lines, last line: "- **§0.1 brutal-honesty preface**: deviates from the template's strict §0 = Metadata convention by carrying analytical content. The deviation is intentional and documented by other per-system specs (Dynatrace, Datadog Agent) using the same pattern; preserving cross-system framing alignment.")

---

## §10 Reviewer convergence record

### Iter-1 — 2026-05-23

Six-reviewer pass: codex, glm (5.1), kimi (k2.6), mimo (v2.5-pro), minimax (m2.7-coder), qwen (3.6-plus). All six returned exit-0 within the 30-min budget; the stress-test was reviewed against `netdata.md` + 6 Phase A comparison artefacts + foundational lens + hub-architecture premise + 16 per-system specs.

| Reviewer | netdata-stress-test.md | netdata-design-implications.md |
|---|---|---|
| codex | reject | reject |
| glm | accept-with-fixes | accept-with-fixes |
| kimi | accept-with-fixes | accept-with-fixes |
| mimo | accept-with-fixes | accept-with-fixes |
| minimax | accept-with-fixes | accept |
| qwen | accept-with-fixes | accept-with-fixes |

Material findings applied (iter-1 → iter-2):

1. Added W0 (blocker): "every trap, always, no exceptions" contradicts dedup behaviour (codex, kimi).
2. Fixed W4: was wrongly framed as `linkUp`/`linkDown` collapse; corrected to "repeated identical traps within dedup window hide flap signatures + paired-clear semantics absent" (codex, minimax, kimi).
3. Reclassified W9 from minor to blocker: v3 USM hot-registered engine-ID + boot-counter reset is a security vulnerability (qwen, codex, kimi, glm).
4. Added W13 (major): "Likely the only real-time system among the cohort" is factually wrong (codex, kimi, qwen).
5. Added W14 (major): AGPL-3.0 / GPL-2.0 license claim needs legal review (qwen, glm, kimi).
6. Added W15 (major): hub-down = fleet blind spot during outage (minimax).
7. Added W16 (major): MIB version conflict across hubs undefined (qwen).
8. Added W17 (minor): `template_unresolved` not in declared error dimensions (codex).
9. Added W18 (minor): `SNMP_TRAP_JSON` symbolic-name-keyed shape has collision risk (codex).
10. Added W19 (minor): pre-decode allowlist vs post-decode source identification (codex).
11. Removed S4 (per-source rate-limit was soft-pedalled as strength) — see W8.
12. Softened S2: distinguish raw-deduped MIB count from curated trap coverage; flag Datadog's verified-3,652 number explicitly.
13. Softened S9: hub-local is differentiation but with explicit hub-down trade-off; acknowledge Telegraf/Logstash/Cribl are also distributed.
14. Corrected W7 fix recommendation: MIB-loaded wins over profile inline `varbinds:` (cohort norm), opposite to original "operator wins."
15. Integrity log §9 regenerated mechanically (24 → 25 inputs corrected; `librenms.md`, `zabbix.md`, `datadog-agent.md` last lines made exact).

In `netdata-design-implications.md`:

16. C6 (INFORM) moved to Revisit R9 (was internally inconsistent with Risk 3).
17. Added R9 (INFORM + boot-counter persistence), R10 (storm rate-limit), R11 (per-device vnode cardinality), R12 (v3 USM security gating), R13 (AGPL-3.0 legal review), R14 (defer inotify; ship precompiled MIB index for first release), R15 (hub-down operator pattern).
18. D5 revised — both inotify hot-reload and UI MIB upload deferred (resolves codex/kimi-flagged contradiction between H1 risk + D5 "day-1").
19. Phase 2 sequencing item 8 changed from "inotify hot-reload" to "dyncfg-triggered MIB reload."

### Iter-2 — 2026-05-23

Six-reviewer pass after iter-1 fixes. Five reviewers returned within budget; minimax timed out at 30min (process killed; partial stdout, no exit file). Per the SOW convention this is recorded as reviewer-infrastructure failure, not a content issue — the other five are dispositive.

| Reviewer | netdata-stress-test.md | netdata-design-implications.md |
|---|---|---|
| codex | accept-with-fixes | reject |
| glm | accept-with-fixes | accept-with-fixes |
| kimi | accept-with-fixes | accept-with-fixes |
| mimo | accept-with-fixes | accept-with-fixes |
| minimax | (timeout — infra failure) | (timeout — infra failure) |
| qwen | accept-with-fixes | accept-with-fixes |

Material findings applied (iter-2 → iter-3):

1. W0 citation precision: "OpenNMS keeps every event" softened to "OpenNMS separates alarm dedup from event storage; explicit `discardtraps` path is the only event-table suppression mechanism."
2. W1 storage-cost math reconciled: removed self-contradictory ">100 MB/sec" estimate; consistent with 5-20 MB/sec used elsewhere.
3. W3 downgraded from blocker to major risky-assumption. §1 #6 follow-on text revised accordingly.
4. W14 corrected: LibreNMS license is **GPL-3.0-or-later** (per Phase A inventory), not GPL-2.0 as the design states. The factual error reinforces the legal-review requirement.
5. W15 SaaS-cohort resilience claim narrowed: only systems with explicit receiver HA/failover (SolarWinds APE, LogicMonitor failover pairs, Splunk SC4SNMP via k8s) cited; Datadog Agent forwarding to SaaS does NOT prove collector-down survival — corrected.
6. M1 paired-clear consolidated to "deferred to Phase 3" per implications R20; eliminates W4/M1/sequencing inconsistency.
7. §9 integrity log: replaced Zenoss `...` ellipsis with exact last line.
8. Added W19a (major): `snmp.trap.errors` missing `auth_failures`, `allowlist_dropped`, `usm_failures` dimensions (codex missed-vector + lens §11.4).
9. Added W20 (major): default `dedup_key_varbinds: all non-timestamp` includes volatile counters that bypass dedup (glm finding).
10. Added W21 (minor): template substitution must be single-pass with opaque varbind values (kimi finding — injection risk).
11. Added W22 (major): `dimension_from_varbind` cardinality check needs MIB knowledge or built-in allowlist (qwen finding).
12. W19 fix corrected: community/v3-username allowlist is post-ASN.1-header, not pre-decode (codex correction).

In `netdata-design-implications.md`:

13. R10 (storm rate-limit) given explicit two-mode semantics: Mode A (alert-only throttle, still journals) vs Mode B (sample to journal + shed). Resolves codex critique that "rate-limited still journals" only protects alert load.
14. C11 reframed from "coverage parity" commit to "public-source strategy" commit. Measured coverage tracked in R3.
15. R13 corrected: LibreNMS GPL-3.0-or-later, not GPL-2.0.
16. Added R16 (full pipeline-self-telemetry dimension set), R17 (`SNMP_TRAP_JSON` array shape), R18 (3-layer allowlist + template safety + `dimension_from_varbind` enforcement), R19 (dedup default key change), R20 (paired-clear sequencing).
17. Phase 3 sequencing updated: 18a paired-clear profile field added; Phase 4 #20 marked moved.

### Iter-3 — 2026-05-23

Six-reviewer pass after iter-2 fixes. **All six reviewers returned, all six verdicts on both files are `accept-with-fixes`**. No outright rejects. Trajectory: iter-1 → iter-2 → iter-3 shifted from `reject` to `accept-with-fixes` across the board.

| Reviewer | netdata-stress-test.md | netdata-design-implications.md |
|---|---|---|
| codex | accept-with-fixes | accept-with-fixes |
| glm | accept-with-fixes | accept-with-fixes |
| kimi | accept-with-fixes | accept-with-fixes |
| mimo | accept-with-fixes | accept-with-fixes |
| minimax | accept-with-fixes | accept-with-fixes |
| qwen | accept-with-fixes | accept-with-fixes |

Material findings applied (iter-3 → iter-4 / convergence):

1. W1 storage-cost math corrected (kimi): 20 MB/sec × 86400 = ~1.7 TB/day at high end; 432 GB/day is the low-end. Added systemd compression 3-5× note.
2. Added W23 (major, kimi missed-vector): journal field injection via control characters in varbind values (CWE-117). Sanitization required at template substitution + JSON encoding.
3. Added W24 (major, kimi missed-vector): DoS via unbounded BER decode — per-packet amplification. Explicit decode-time limits required (max UDP recv 8KB, max varbinds 256, max depth 8, max decode time 1ms).
4. In `netdata-design-implications.md`: added R21 (journal-injection sanitization) and R22 (BER decode limits) corresponding to W23 / W24.

Remaining findings NOT applied (deliberate, with rationale):

- **Minimax #1**: "Only real-time system" claim still in `netdata.md §1 L21`. This is a finding about the design document, not the stress-test. The stress-test correctly surfaces it (W13); the implications correctly say abandon (A3). Updating `netdata.md` is out of scope for Phase B (Phase B produces stress-test outputs, not edits to the design). The next SOW that takes `netdata.md` into implementation will apply this.
- **Minimax #2-4, #7**: similar — findings about `netdata.md` (the design), not the stress-test products. Surfaced correctly in W0, W9, W20, R10. Phase B's job is done.
- **Kimi #4 (R15 heartbeat mechanism)**: R15 step 3 already specifies a Cloud-side alert when a hub stops reporting `snmp.trap.events` — that's the heartbeat. Wording could be sharper but not material.
- **Codex/glm precision items**: integrity log §9 was regenerated mechanically; remaining nits are wording-preference, not material.

### Convergence verdict

**Iter-3 convergence achieved**: 6 of 6 reviewers at `accept-with-fixes` on both files; 0 rejects; 0 unaddressed blockers in the stress-test products themselves. The two new W23/W24 findings from iter-3 are now applied, addressing all material missed-attack-vector findings. Surviving items are either (a) wording-preference precision nits, or (b) findings about `netdata.md` (the design under stress test) that this Phase B has correctly surfaced and that the next implementation SOW will apply.

Phase B stress test is **complete**. Both files are decision-grade for the team to act on. Iter-4/iter-5 not required per the SOW convergence threshold of `≥3 accept-with-fixes with no remaining major/blocker in the artefacts themselves`.
