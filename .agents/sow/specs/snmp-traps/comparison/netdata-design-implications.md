# Netdata SNMP Trap Design — Phase B Implications

## 0. Document Metadata

- **Companion to**: `comparison/netdata-stress-test.md` (the stress-test findings).
- **Purpose**: synthesise the actionable design implications from the stress-test for the team that will write the SOW for implementation.
- **Posture**: decision-grade. Each item is either a commit (proceed), revisit (re-design), defer (push to a later SOW), or abandon (drop). No "interesting to consider" items.
- **Citation convention**: `netdata.md §N L<line>` for the target design; `<system>.md §N` or `<system>.md L<line>` for cohort evidence; `comparison/<artefact>.md §N` for Phase A matrix.
- **Authored by**: assistant under Phase B stress-test instructions.

---

## §1 Commit-to (proceed as designed)

These design choices survive the stress test. Cohort evidence supports them; the stress-test surfaced no fatal contradiction. The team should implement these as written.

### C1. Document-primitive choice (trap → journal row)

- **Source**: `netdata.md §2 L36-49`.
- **Cohort validation**: 6 of 16 systems converged on log-document primitive (`comparison/design-forks.md` Fork 1(d)): Datadog, Dynatrace, Splunk SC4SNMP, Logstash, SolarWinds (current Log Viewer/Analyzer), LogicMonitor (LogSource path).
- **Why it's right**: clean separation of forensic store from alerting; reuses existing Logs UI; avoids the per-row alarm-lifecycle storage tax that OpenNMS/Zenoss/CheckMK pay.
- **Caveat**: storage policy must be spelled out (see §2 R1).

### C2. Cardinality discipline (structural reject at config-load)

- **Source**: `netdata.md §4 L81-110`.
- **Cohort validation**: lens §8.4 Label Explosion Tension is the universal warning; Telegraf has explicit field-vs-tag discipline (`telegraf.md L355`). Netdata's choice to **reject** rather than warn is stronger.
- **Why it's right**: this is the single most important design decision the file makes. It prevents the SolarWinds `Orion.TrapVarbinds`-per-varbind cost (`solarwinds.md L419`) and the Zabbix high-cardinality-leak risk (`zabbix.md §5 L343-358`). Operators get clear errors at config-load instead of cardinality explosion at scale.

### C3. PDU-level source identification (RFC 3584 / v1 `agent-addr`)

- **Source**: `netdata.md §5 L136`.
- **Cohort validation**: LogicMonitor EA 36.100+ pattern (`logicmonitor.md L436-441`) is the cohort's cleanest. OpenNMS (`opennms.md L242-247`) is comparable. **8 of 16** systems get this wrong, including Datadog and Splunk SC4SNMP, which makes Netdata adopting it competitive differentiation.
- **Why it's right**: NAT/VRF-correctness, the lens §11.4 pitfall, is real. Devices behind NAT or sending through proxies are common in enterprise.

### C4. PLUGINSD/stdio decoupled cold-path emission

- **Source**: `netdata.md §5 L129-149`.
- **Cohort validation**: Telegraf's single-goroutine ceiling (`telegraf.md L1251`) shows what happens when receive and emit share a thread. OpenNMS `isBlockWhenFull` is the opposite design choice (`opennms.md L131`).
- **Why it's right**: stdout pipe backpressure cannot stop trap reception. Standard Netdata pattern.

### C5. Plugin-self metrics (`snmp.trap.events`, `snmp.trap.errors`)

- **Source**: `netdata.md §12 L525-561`.
- **Cohort validation**: OpenNMS 11 JMX counters, Telegraf internal metrics, Datadog `datadog.snmp_traps.*` (`comparison/operator-features.md T2-07`) — first-class self-telemetry is above cohort median.
- **Why it's right**: lens §13 "Pipeline self-monitoring" calls this out as a Tier-2 must-have. The cohort gap on this is real; Netdata closes it.

### C6. (MOVED TO REVISIT) — see R9 below

INFORM acknowledgement was previously listed as "commit-to" on the strength of gosnmp 1.42's INFORM handling. Iter-1 reviewers correctly flagged that this contradicts Risk 3 (which says INFORM requires real-device integration testing). It must not be both "commit as designed" and "verification needed." Moved to R9.

### C7. Hub-local correlation architecture

- **Source**: `netdata.md §0 L7`, `netdata-snmp-hub-architecture.md`.
- **Cohort validation**: lens §6.4 calls "Single Manager Bottleneck" an anti-pattern. All centralized cohort systems (OpenNMS, Zenoss, Centreon, SolarWinds) hit DB/JVM scale ceilings. This is real differentiation.
- **Why it's right**: cohort scaling pain is centralization; Netdata's hub model eliminates it.
- **Caveat**: per-hub throughput must actually deliver (see §2 R2).

### C8. Plugin-self metric for `deduplicated` count

- **Source**: `netdata.md §10 L429`.
- **Cohort validation**: this gives operators continuous visibility into trap-storm signals — better than the cohort's "kernel UDP overflow alert" pattern alone.
- **Why it's right**: every cohort system that has dedup has had operator confusion about whether dedup is hiding signal (lens §11.7). Netdata's continuous metric is the right answer.

### C9. Operator overrides via plugin config (per-OID label additions)

- **Source**: `netdata.md §7.5 L275-292`.
- **Cohort validation**: the operator overlay-on-vendor-knowledge pattern is necessary; Centreon CLAPI EXPORT (`centreon.md §4 L1092`) shows operators do customize.
- **Why it's right**: cross-cutting concerns (compliance scope, tenant, datacenter) cannot live in vendor profile YAML. The plugin-config layer is correct.
- **Caveat**: see §2 R5 for the two-artefact synchronization concern.

### C10. PLUGINSD-driven external collector loaded by Netdata

- **Source**: `netdata.md §5 L128-130`.
- **Cohort validation**: standard Netdata pattern; the NetFlow plugin uses the same. Battle-tested in the existing Netdata codebase.
- **Why it's right**: there is no reason to invent a new process-management layer for trap reception.

### C11. Public-source strategy for stock vendor profile coverage

- **Source**: `netdata.md §8 L298-356`.
- **Cohort validation**: `comparison/profile-inventory.md §1-15` shows that the cohort's open sources (OpenNMS XML, LibreNMS MIB tree, Centreon SQL, public IETF MIBs, pysnmp MIB collection) are sufficient bootstrap material. Iter-2 reviewer correctly noted that raw module count is not curated coverage — the **strategy is sound**; the **delivered coverage** is a separate question (see R3).
- **Why it's right**: building the catalogue from public, open-licensed sources avoids closed-bundle opacity (Datadog `dd_traps_db.json.gz`) and the operator-pain it creates (`datadog-agent.md §17`).
- **Caveat**: the *strategy* commit is "use public sources + transform tools at build time." The *coverage number* (5,000 curated trap definitions across top-10 vendors) is the measurable target tracked in R3.

---

## §2 Revisit (rework before implementation)

These choices in `netdata.md` require redesign before implementation begins.

### R1. Journal storage cost model

- **Stress-test reference**: W1 (blocker), M4, H4, Q2.
- **What's wrong**: §11 promises "no new storage tier" but does not specify retention, rotation, namespace, corruption recovery, or per-row byte cost. At 10k traps/sec sustained, the journal grows at ~5-20 MB/sec — 86-432 GB/day. This is a real storage commitment that the design hides behind systemd-journal opacity.
- **Recommended approach**:
  1. State the journal namespace explicitly (e.g., `netdata-snmptrap.journal`).
  2. Default retention 30 days; configurable; with a hard cap on disk consumption (operator chooses GB or % of disk).
  3. Document the corruption recovery path: on journal write failure, the listener keeps accepting traps and queues to a fallback file; alerts the operator.
  4. Compute and publish expected per-row byte cost; recommend disk planning per trap rate.
  5. Decide whether to compress the JSON varbind payload (gzip per-row vs systemd's built-in compress).
- **Cohort evidence**: SolarWinds 90-min DELETE pain (`solarwinds.md L419`); Centreon truncation silent failure (`centreon.md L530-536`); Zenoss `event_archive_purge_interval_days=90` (`zenoss.md §6`); CheckMK `history_lifetime=365 days` default (`checkmk.md §6`). Every cohort system that stores traps in a structured store has solved this; Netdata can't skip the question.

### R2. Throughput claim must be measured, not asserted

- **Stress-test reference**: W2 (blocker), A1, A2.
- **What's wrong**: §1 L16 and §9 L362-380 claim "10s of thousands of events/sec, sufficient for any reasonable site size" and "10× to 60× cohort numbers" with zero benchmark evidence. The Dynatrace c5.large vendor numbers max out at 2.5k/sec; Splunk SC4SNMP is 1.5k/sec on 16-core; Telegraf single-goroutine raw decode is ~10k/sec ceiling.
- **Recommended approach**:
  1. Replace "10s of thousands/sec" with a measured target after prototype.
  2. Commit to publishing benchmarks on 3 reference platforms (4-core/8GB, 16-core/32GB, 32-core/64GB) under three workloads (pure decode, decode+journal-write, decode+journal-write+enrichment).
  3. The "30k rows/sec per writer thread" number must be sourced (commit hash, test file, hardware, workload) or removed.
- **Cohort evidence**: see W2.
- **Risk if not done**: the design ships with a marketing claim it can't back; operators will measure and the credibility of the cohort-win audit collapses.

### R3. Coverage commitment

- **Stress-test reference**: W10, A6, H3.
- **What's wrong**: §17 L630 ships "stock vendor pack for top-5 vendors (Cisco, Juniper, Arista, Aruba, Palo Alto)" without committing to coverage depth. §8 promises "8,000-12,000 unique MIB modules" but that's the raw deduped count, not the curated/tested count. Datadog's verified count is 3,652 from a marketed 11,000+.
- **Recommended approach**:
  1. Define the per-vendor coverage criterion: top N alert-class OIDs per vendor, sourced from the LibreNMS handler corpus + OpenNMS event XML.
  2. Day-1 commitment: 5 vendors × 50 OIDs minimum (= 250 OIDs with category + severity + symbolic name + display_hint).
  3. Day-90 commitment: 10 vendors × 100 OIDs minimum.
  4. Document the curation process: which Phase A conversion tools produce which output.
- **Risk if not done**: ship with a thin OOB catalogue, get the same Stage-1/Stage-2 critique the cohort earns.

### R4. Dedup model under partitioning

- **Stress-test reference**: W3 (blocker), I2.
- **What's wrong**: §5 L126 + §9 L366-370 partition by source-IP hash to break the 30k/sec single-writer cap. §10 L394 dedup cache is in-memory, per-process. Sharding listeners → writers by source-IP creates a dedup cache split that allows true duplicates to escape if devices change shard buckets.
- **Recommended approach**:
  1. Pick one: commit to single-writer until measured proven insufficient, defer partitioning to a follow-up SOW.
  2. OR: spell out the partitioning model — dedup-cache lives in a shared in-process LRU (sharded internally, but globally visible to all listener threads in one process); the bottleneck shifts to LRU contention; measure.
  3. Don't promise both lock-free hot-path dedup AND multi-writer partitioning without specifying how state is shared.
- **Cohort evidence**: OpenNMS dedup in PostgreSQL (single source of truth, `opennms.md §5`); Centreon dedup in single Perl daemon (`centreon.md L356`); Zenoss dedup in ZEP (`zenoss.md L329-330`). **No cohort system has both sharded receive AND lock-free dedup.**

### R5. Profile YAML / plugin config split

- **Stress-test reference**: W5, I4.
- **What's wrong**: §7 says profile YAML is "vendor knowledge only"; §7.5 puts per-OID metric opt-in, severity/category overrides, and labels in plugin config. The two artefacts can drift; nothing in the design ensures consistency. Operators routing an alert on a Cisco OID must edit three places.
- **Recommended approach**:
  1. Keep the split (vendor read-only + operator overlay is sound).
  2. Add a config-validation tool that, given an OID, prints the effective state (resolved category, severity, labels) across profile + plugin config; document this as part of the operator workflow.
  3. Generate plugin-config templates from profile YAML for common alert patterns (per-OID metric opt-in for the top-50 alert-class OIDs per vendor) so the default operator workflow is "enable a vendor's alert template, not write 50 lines of YAML."
- **Cohort evidence**: OpenNMS one-file-per-vendor (`opennms.md §13`); Datadog one bundled artefact (`datadog-agent.md §4 L275-298`); SNMPTT one `EVENT` block per OID. None split.

### R6. INFORM Response routing under multi-listener

- **Stress-test reference**: M3, Q8.
- **What's wrong**: §6 supports multiple listeners. INFORM Response must go back to the originating device via the listener that received it. The design doesn't say this.
- **Recommended approach**:
  1. State that INFORM Response uses the receive-side UDP socket (source port matches the bound listener).
  2. Document a fallback: if the device is behind NAT relative to the hub (lens §11.4), INFORM may fail; the operator's choice is to switch to UDP traps for that device.

### R7. Closed category set — replace with extensible set

- **Stress-test reference**: W6.
- **What's wrong**: §3 L74-75 declares "Category set is closed — the 9 slugs above are the canonical taxonomy. Operators cannot extend this set."
- **Recommended approach**:
  1. Drop the "closed" claim. Categories are advisory; operators can extend.
  2. OR: expand `custom` into 3-4 sub-categories (`custom_security`, `custom_state_change`, `custom_diagnostic`, `custom_other`) so operators have named hooks.
  3. Document that operator-added categories survive upgrades (recommended: persist in operator's plugin config).
- **Cohort evidence**: OpenNMS, Zenoss, LibreNMS, Centreon, LogicMonitor all allow operator extension.

### R8. Profile inline `varbinds:` block — MIB-loaded wins over profile inline

- **Stress-test reference**: W7 (corrected).
- **What's wrong**: §7 L189-211 allows profile to inline-define varbinds; §7 L215 resolution order is profile → MIB → raw. **Iter-1 reviewers corrected**: profile-wins is the SNMPTT/Zenoss pattern that earns the cohort's maintenance pain; the cohort norm (OpenNMS, Datadog) is MIB-authoritative.
- **Recommended approach**:
  1. Loaded MIB **overrides** profile inline `varbinds:` when both are present. Profile inline is the fallback for OIDs lacking MIB coverage.
  2. The build-time conversion tools (§8) produce inline `varbinds:` from MIB sources, ensuring they agree at ship time.
  3. Shipping a stock profile with inline `varbinds:` that contradicts the bundled MIBs must be a build error (CI check).
  4. Operator can override the loaded-MIB-wins default with an explicit per-OID flag (e.g., `varbind_override: true`) for known vendor-MIB bugs.

### R9. INFORM under multi-listener and v3 boot-counter persistence

- **Stress-test reference**: W9 (re-classified blocker), Q8, M3.
- **What's wrong**: original §1 commit C6 was internally inconsistent with Risk 3 (INFORM verification needed). Plus the v3 INFORM Response requires `snmpEngineBoots` persistence across plugin restart (RFC 3414 §2.2.2); the design's "plugin lifecycle follows the Netdata agent's lifecycle" (`netdata.md §5 L130`) means boot counter resets every Netdata restart, breaking devices that cached the old counter.
- **Recommended approach**:
  1. Persist per-job `snmpEngineBoots` to `${NETDATA_LIB_DIR:-/var/lib/netdata}/snmp-trap/{job_name}/engine-boots` and increment on each plugin start.
  2. Document the recovery behaviour for INFORM Response after plugin restart.
  3. Add integration test with a real Cisco device sending v2c INFORM and v3 INFORM; verify Response PDU shape via tcpdump.
  4. State that INFORM Response uses the receive-side UDP socket under multi-listener configs.
  5. If gosnmp 1.42 (or chosen Rust crate) does not implement INFORM correctly, drop INFORM from first release; explicit non-goal.

### R10. Storm protection — two-mode token-bucket per-source rate-limit

- **Stress-test reference**: W8 (made into an action, was lost as Risk 5 only).
- **What's wrong**: `netdata.md §10 L388` defers per-source rate-limiting to the kernel UDP overflow alert and §16 L621 claims first-wins dedup "solves the same problem more elegantly." It doesn't: a storm of *unique* OIDs from one source is not suppressed by dedup. Lens §11.1 names this as the #1 failure mode.
- **Recommended approach** (two explicit modes — iter-2 codex correctly flagged that journaling rate-limited traps protects alert load only, not receiver CPU / disk / journal growth):
  1. Remove the "more elegantly" claim from `netdata.md §16 L621` — it's wrong.
  2. Mode A (default) — **alert-only throttling**: rate-limited traps still write to the journal with `SNMP_TRAP_RATE_LIMITED=true` and do NOT increment metric counters that drive alerts. Use case: an operator wants forensic completeness; the throttle prevents alert-fatigue but preserves the data.
  3. Mode B (storm shedding) — **journal sampling**: rate-limited traps are sampled (e.g., 1 in N) into the journal; the rest are counted only via `snmp.trap.errors.rate_limited_shed` and aggregated into per-period summary entries. Use case: storm conditions where journal-write rate itself is the bottleneck.
  4. Operator selects mode per-source or globally via plugin config; default Mode A.
  5. Token-bucket parameters: configurable burst + sustained-rate per source IP / per device.
  6. Operator-visible metrics: `snmp.trap.errors.rate_limited` (Mode A counter) and `snmp.trap.errors.rate_limited_shed` (Mode B counter).

### R11. Per-device vnode cardinality

- **Stress-test reference**: W11 (originally W9), Risk 10.
- **What's wrong**: §12 L530-537 instantiates per-device vnode for `snmp.trap.events`. With 5,000 SNMP devices the hub sustains ~100,000 dimensions on the events chart alone. The lens §8 Label Explosion Tension warns against this.
- **Recommended approach**:
  1. Default: group `snmp.trap.events` by (vendor, category) on a per-hub vnode; per-device detail in labels (Logs UI), not chart dimensions.
  2. Operator can opt-in to per-device vnodes via plugin config for sites with <500 devices.
  3. Document the dimension budget per chart: e.g., max 1,000 dimensions per chart context; beyond that, regroup.
  4. Stress-test with 5,000 synthetic devices before first release.

### R12. v3 USM hot-registered engine-ID — security gating mandatory

- **Stress-test reference**: §1 #5 (blocker), W9 (blocker).
- **What's wrong**: §5 L153 hot-registers `(engineID, username)` pairs from raw PDU bytes without operator opt-in. Any device can spoof any engineID and inject traps.
- **Recommended approach**:
  1. Default: pre-configured engineIDs only (matching CheckMK).
  2. Hot-registration is opt-in via plugin config.
  3. When hot-registration enabled, pair is logged to journal with `SNMP_USM_HOT_REGISTERED=true` for operator audit; operator must confirm via dyncfg before pair is considered trusted.
  4. Hot-registered pairs expire after configurable TTL (default 24h) and require re-confirmation.
  5. Spell out the TOCTOU between hot-registration and Secrets lookup: pair-creation happens after Secrets binding lookup, not before.

### R13. AGPL-3.0 / GPL-3.0-or-later licensing claim — legal review before commit

- **Stress-test reference**: W14.
- **What's wrong**: `netdata.md §8 L349-351` asserts that GPL-2.0 / AGPL-3.0 obligations "do not propagate to our derived YAML." This is a legal claim. **Factual correction**: LibreNMS is GPL-**3.0**-or-later (per `comparison/profile-inventory.md §3 L30` and §4 L39), not GPL-2.0 as netdata.md states. The conversion-tool approach reads AGPL-3.0 OpenNMS event-XML definitions and GPL-3.0-or-later LibreNMS sources, transforms them with curated severity/category logic, and emits Netdata YAML. AGPL-3.0 §1 (definition of source code) and §13 (network use), plus GPL-3.0 §5 (conveying modified source) and the derivative-work definitions, are non-trivial.
- **Recommended approach**:
  1. Remove the licensing assertion from §8 until legal review.
  2. Engage Netdata legal counsel; document the conclusion in the SOW.
  3. First-release alternative if AGPL-3.0 review surfaces risk: prioritize Apache-2.0 sources (Centreon SQL, Datadog integrations-core tooling, public IETF MIBs) for initial coverage; defer OpenNMS XML conversion until licensing is resolved.
  4. Track the AGPL-3.0 alternative options as a contingency.

### R14. MIB hot-reload — defer to Phase 2; ship precompiled MIB index for first release

- **Stress-test reference**: W6 / H1.
- **What's wrong**: `netdata.md §7 L253` inotify-driven runtime MIB compile. Iter-1 reviewers (codex, kimi) flagged that documentation alone does not resolve operational pain — the cohort universally avoids live hot-reload (`splunk-sc4snmp.md §4`, `datadog-agent.md §4 L410`, `zenoss.md §4`, etc.).
- **Recommended approach**:
  1. First release: ship a precompiled MIB index (Datadog's choice — single artifact bundled with Netdata).
  2. Operator-supplied MIBs in `/etc/netdata/snmp-mibs/` require a discrete `netdata-snmp-mib-compile` step (or `dyncfg`-triggered reload), not silent inotify-driven recompile.
  3. Document that inotify-driven hot-reload is a Phase 2 deliverable, evaluated against the cohort's failure modes.

### R15. Hub-down resilience — 2-hub redundancy + Cloud-side dedup

- **Stress-test reference**: W15 / §1 #13.
- **What's wrong**: hub-down = Site-X trap blind spot. The design does not surface this trade-off.
- **Recommended approach**:
  1. Document operator pattern: configure each device with **2+ trap destinations** so each trap reaches at least two hubs.
  2. 2-hub redundancy creates duplicate journal entries; Cloud-side presentation must dedup across hubs (deferred D9).
  3. Add a Cloud-side alert: when a hub stops reporting `snmp.trap.events`, fire a "hub unreachable" alert tied to the existing agent-health mechanism.

### R16. Pipeline self-telemetry — full failure-mode coverage on `snmp.trap.errors`

- **Stress-test reference**: W17, W19a.
- **What's wrong**: §12 L551 lists three error dimensions (`unknown_oid`, `decode_errors`, `deduplicated`). Missing: `template_unresolved` (incremented at §7 L238 but not declared); `auth_failures`, `allowlist_dropped`, `usm_failures` (cohort lens §11.4 silent-mismatch failure mode); `rate_limited`, `rate_limited_shed` (per R10).
- **Recommended approach**: declare the full dimension set on `snmp.trap.errors`:
  - `unknown_oid`, `decode_errors`, `deduplicated` (already present)
  - `template_unresolved` (per §7 L238)
  - `auth_failures` (v3 USM auth failed)
  - `allowlist_dropped` (UDP peer not in allowlist OR community/username mismatch)
  - `usm_failures` (engine-ID resolution or USM-discovery failure)
  - `rate_limited`, `rate_limited_shed` (per R10)
- Each dimension carries `hub` + `source_device` labels (where source is identifiable).

### R17. `SNMP_TRAP_JSON` shape — adopt array-of-varbinds with optional symbolic name

- **Stress-test reference**: W18.
- **What's wrong**: §11 L517 keys JSON object by symbolic name; §13 L583 admits the shape stability is an open question. Symbolic-name keying has collision risk (two MIBs reusing the same symbol) and breaks operator `jq` queries when MIB updates rename symbols.
- **Recommended approach**: ship `SNMP_TRAP_JSON` as an **array of objects** preserving wire-order positional identity: `[{"oid": "1.3.6.1.4.1.9.9.315.1.2.1.1.1", "name": "cpsIfViolationMacAddress", "type": "OctetString", "value": "aa:bb:cc:dd:ee:ff", "display_hint": "1x:"}, ...]`. Name is optional; OID is always present.

### R18. Allowlist layers + template-syntax safety + dimension_from_varbind enforcement

- **Stress-test reference**: W19 (three allowlist layers), W21 (template single-pass + opaque varbind values), W22 (`dimension_from_varbind` cardinality check).
- **What's wrong**: §6 L161 conflates IP allowlist with community allowlist; §7 L223-243 template syntax doesn't specify recursion; §7.5 L296 cardinality check requires MIB knowledge that may not be present.
- **Recommended approach**:
  1. Define three allowlist layers: UDP-peer IP (pre-decode), community/v3-username (after ASN.1 header), PDU-source `agent-addr`/`snmpTrapAddress` (after full decode).
  2. State template substitution is single-pass and varbind values are opaque (no re-expansion of `{`/`}` characters).
  3. `dimension_from_varbind` cardinality check: reject any varbind reference for an OID without loaded MIB coverage, unless the varbind name appears on a built-in allowlist of known-bounded enum-valued varbinds.

### R19. Dedup default key — identifier varbinds only, not "all non-timestamp"

- **Stress-test reference**: W20.
- **What's wrong**: §10 L394 default `dedup_key_varbinds` = "all non-timestamp varbinds." Volatile counter varbinds (`ifInErrors`, BGP counters) trivially differ per-event, bypassing dedup.
- **Recommended approach**:
  1. Change the default to `(source_device, trap_OID)` only — coarse but reliable.
  2. Profiles declare an explicit `dedup_key_varbinds:` for OIDs where index-varbinds make dedup more precise (e.g., `[ifIndex]` for linkDown/linkUp).
  3. Document the trade-off in §10: coarse default = false positives (different real events suppressed); operator opts in to finer-grained keys via profile.

### R21. Journal-injection / control-char sanitization — first release

- **Stress-test reference**: W23.
- **What's wrong**: §7 template substitution writes varbind values into MESSAGE without sanitization. A device that sends `\n` or `\0` in an octet-string varbind can inject arbitrary trusted journal fields. CWE-117 log-injection class.
- **Recommended approach**:
  1. Template substitution sanitizes varbind values: strip / replace control characters (< 0x20) before MESSAGE insertion.
  2. `SNMP_TRAP_JSON` uses RFC 8259-compliant JSON encoder for varbind values.
  3. Self-test: synthetic trap with control characters in varbind values; verify journal entry has sanitized MESSAGE.
  4. Add `snmp.trap.errors.sanitized` dimension (per W19a + R16).

### R22. BER decode limits — first release

- **Stress-test reference**: W24.
- **What's wrong**: §5 hot-path BER decode has no stated limits. A malicious 65KB PDU with deeply-nested SEQUENCE types or many varbinds amplifies per-packet cost, blocking the listener.
- **Recommended approach**:
  1. Max UDP recv: 8 KB (configurable).
  2. Max varbinds per PDU: 256.
  3. Max BER decode depth: 8.
  4. Per-PDU decode time budget: 1ms.
  5. Add `snmp.trap.errors.malformed_decode_ddos` dimension (per W19a + R16).

### R20. Paired-clear semantics — first release relies on operator pattern; profile field is Phase 3

- **Stress-test reference**: W4, M1 (decision consolidated per iter-2 codex finding).
- **What's wrong**: iter-1 left M1 saying "in-scope," W4 saying "defer to follow-up SOW," and Phase 4 (Month 12+) committing to it. Three conflicting signals.
- **Recommended approach**: **defer the profile-field implementation to Phase 3 (Month 6-12).** First release surfaces the gap to operators via:
  1. Add to `netdata.md §14 Non-Goals`: "Paired-clear semantics (auto-clear on `linkUp` for an open `linkDown`) — first release does not pair traps in the journal; operators reconcile state via `ifOperStatus` polling."
  2. Recommend an operator alert pattern: alert when `linkDown` journal entry has no matching `linkUp` within N minutes AND `ifOperStatus` poll returns `down`.
  3. Phase 3 deliverable: profile `clear_trap_oid:` field that links Up to Down at the schema level; alert engine consumes the pairing.

---

## §3 Defer (push to a later SOW)

These features in `netdata.md` should be removed from the first release scope and tracked as follow-up work.

### D1. Northbound trap re-emission

- **Source**: `netdata.md §13 L577`.
- **Why defer**: it's already deferred in the design. Cohort coverage is split (7/16). Northbound = manager-of-managers integration; valuable for large enterprises but not blocking for first release.
- **When to revisit**: after first release ships and customer demand surfaces.

### D2. DTLS / TLS-TM

- **Source**: `netdata.md §6 L164`, §13 L585.
- **Why defer**: zero cohort systems support this (`comparison/operator-features.md T3-04`). The design already says "in scope if mature libraries exist." Rust and Go SNMP libraries do not have mature TLS-TM support today. Pretending this is in scope creates a checkbox that won't be ticked.
- **Recommended action**: move from "in scope conditionally" to "explicit non-goal for first release; tracked for future."

### D3. Topology-aware suppression

- **Source**: `netdata.md §16 L621` ("Hub-local enrichment makes this cheap").
- **Why defer**: the claim "cheap" is unsupported. The hub has topology, yes, but suppression rules require operator authoring (cohort gap, `comparison/operator-features.md T3-06`). First-release: ship topology annotation in the journal (`ND_TOPOLOGY_*` fields), defer suppression to a follow-up.
- **Recommended action**: move §16's "topology-aware suppression" claim to §13 Open Questions and surface it as a real follow-up SOW.

### D4. Profile-to-Secrets binding UX

- **Source**: `netdata.md §13 L576`.
- **Why defer**: already deferred. UX detail TBD. First release ships listener-level v3 USM with explicit user table; per-engine-ID Secrets binding via dyncfg is a polish item.

### D5. (REVISED) MIB upload via API + inotify hot-reload — both deferred

- **Source**: `netdata.md §7 L253` (inotify), §13 L579 (UI upload).
- **Why defer**: iter-1 reviewers (codex, kimi) flagged a contradiction with H1: stress-test labels inotify hot-reload as risky / cohort-unprecedented, but the original implications text called it "day-1." Both are deferred. First release ships a precompiled MIB index plus a discrete `dyncfg`-triggered MIB reload (operator-initiated, not file-drop-triggered). See R14 for the Phase 2 approach to operator-supplied MIB integration.

### D6. Conversion tools — staged delivery

- **Source**: `netdata.md §17 L635`.
- **Why defer**: the design lists 6 conversion tools as one step. Each is a multi-week project (H2 in stress-test). Realistic sequencing:
  - First release: public MIB tree → YAML (most volume), OpenNMS XML → YAML (highest-quality severity/category metadata).
  - 6-month follow-up: Centreon SQL → YAML, LibreNMS handler corpus → YAML.
  - 12-month follow-up: Zenoss ZenPack, SNMPTT.
- **Recommended action**: restructure §17 to reflect realistic effort.

### D7. Per-OID metric label-cardinality caps

- **Source**: `netdata.md §13 L572`.
- **Why defer**: already deferred. Trust operators in first release; add caps if real-world usage shows abuse.

### D8. `metric_filter` profile field for polled-equivalent linking

- **Source**: `netdata.md §13 L574`.
- **Why defer**: already deferred. The journal-as-source covers the forensic case; the polled-equivalent linkage is presentation polish.

### D9. Cloud-side cross-hub aggregation for traps

- **Stress-test reference**: H5.
- **Why defer**: the Cloud presentation layer needs schema reconciliation across hubs (different MIB versions). First release: hub-local presentation; Cloud cross-hub view is a follow-up.

### D10. Multi-tenant Cloud RBAC for trap data

- **Source**: `netdata.md §13 L580`.
- **Why defer**: out of scope per §0 already.

---

## §4 Abandon (drop entirely)

These features the design has but should be removed outright.

### A1. The "closed category set" claim

- **Source**: `netdata.md §3 L74-75`.
- **Why abandon**: see R7. Closing the set forecloses operator extensibility for no benefit; the cohort universally allows extension.
- **Replacement**: extensible category set + operator-defined categories persist in plugin config.

### A2. The "10× to 60× cohort numbers" claim

- **Source**: `netdata.md §9 L380`.
- **Why abandon**: see W2 + R2. The claim is unsupported; the cohort numbers it compares against are mostly per-minute (Dynatrace 17-150k/min, Splunk 1.5k/sec on 16-core). The math is wrong and the assertion is hubris.
- **Replacement**: a measured per-platform benchmark plan published with the first release.

### A3. The "1-second" alerting rule (Rule 6)

- **Source**: `netdata.md §1 L21` "Real-time (1-second from PDU arrival to alert evaluation). Likely the only real-time system among the cohort."
- **Why abandon**: factually wrong. `comparison/feature-matrix.md L611-629` shows OpenNMS, Zenoss, CheckMK, Sensu, Zabbix, SolarWinds, Centreon (median ~1s), LibreNMS (synchronous handler, cron-delayed delivery) all do real-time evaluation. Netdata is **not unique** in real-time alerting.
- **Replacement**: drop the "only real-time system" claim. Say: "Real-time at metric-update cadence (1 second), competitive with the alarm-engine cohort and faster than the SaaS-log-monitor cohort."

### A4. The "comprehensive vendor packs OOB" implied count of 8-12k unique modules

- **Source**: `netdata.md §8 L324`.
- **Why abandon**: see H3 + A6. The 8-12k number is the raw deduped MIB module count, not curated trap coverage. Datadog's verified count is 3,652; their marketing is 11,000+. Netdata should not repeat Datadog's marketing-vs-verified ambiguity.
- **Replacement**: a curated count (5,000 OIDs with severity + category + symbolic name + display_hint, derived from the cohort sources).

### A5. The "dozens of thousands of events/sec, sufficient for any reasonable site size" framing

- **Source**: `netdata.md §1 L16`.
- **Why abandon**: handwaves "any reasonable site size." A site with 5,000 SNMP devices and 2 traps/sec per device steady-state plus 1-hour fan-storm peak of 100/device-sec → 10,000/sec steady, 500,000/sec peak. "Reasonable" sites can easily exceed the claimed ceiling.
- **Replacement**: state explicit boundaries: "first release supports sites with up to N steady-state traps/sec on Y-CPU/Z-GB hub hardware; storm-peak survival is via dedup + kernel UDP buffer; beyond this, deploy more hubs."

### A6. The marketing language about being "the only real-time system among the cohort"

- **Source**: `netdata.md §1 L21`.
- **Why abandon**: see A3.

---

## §5 Implementation sequencing recommendation

Restructured from `netdata.md §17` (which is currently a flat 12-step list with unrealistic granularity).

### Phase 1: Vertical slice (Day 0 - Month 3)

**Goal**: an operator drops Netdata on a hub, points a device's trap destination at it, and sees decoded traps in Logs UI within the hour.

1. Reception (UDP/162 with `CAP_NET_BIND_SERVICE` and unprivileged fallback ports per `netdata.md §6`).
2. BER decode with gosnmp 1.42 (Go) or `pdu-snmp` (Rust) — prototype both, benchmark, choose. The language decision (§5 L114-121) must be made before Phase 2.
3. PDU-level source identification per LogicMonitor EA 36.100+ (RFC 3584 / v1 `agent-addr`).
4. Journal-write (one writer thread; no partitioning).
5. Bundled MIB index with public-MIB-derived inline `varbinds:` for the IETF base MIBs only (IF-MIB, BGP4-MIB, RFC1213-MIB, SNMPv2-MIB) plus 5 vendor profiles (Cisco IF, Juniper IF, Arista IF, Aruba IF, Palo Alto IF — 50 OIDs each minimum).
6. Plugin-self metrics (`snmp.trap.events`, `snmp.trap.errors`).
7. Logs UI integration (operators can query by `SNMP_TRAP_OID`, `SNMP_TRAP_CATEGORY`, `SNMP_SOURCE_IP`, `SNMP_DEVICE_HOSTNAME`).

**Done when**: a Cisco/Juniper/Arista trap from a real device is decoded, journaled, visible in Logs UI, and the per-device chart updates within 1 second.

### Phase 2: Operator-grade (Month 3 - Month 6)

8. Custom MIB integration — operator-initiated reload via dyncfg (NOT inotify). Atomic-rename safety + explicit dependency-resolution behaviour (see R6 / R14). Inotify hot-reload deferred per R14.
9. Plugin config dyncfg integration (per-OID metric opt-in, per-OID label additions, severity overrides).
10. First-wins dedup (default 1s window per Centreon; explicit per-OID override; document paired-clear semantics explicitly per W4).
11. Stock vendor pack expanded to top-10 vendors × 100 OIDs minimum.
12. Public MIB → YAML conversion tool (`netdata-snmp-profile-convert mib`).
13. OpenNMS XML → YAML conversion tool (`netdata-snmp-profile-convert opennms`).

### Phase 3: Differentiation (Month 6 - Month 12)

14. Topology annotation when topology is co-located on the hub (`ND_TOPOLOGY_*` fields).
15. Polling-context enrichment when polling is co-located on the hub.
16. Multi-writer journal partitioning **only if Phase 1+2 benchmarks show single-writer is insufficient** for the target sustained rate. If single-writer holds, defer.
17. Coverage expansion: Centreon SQL → YAML, LibreNMS handlers → YAML.
18. Cloud-side cross-hub aggregation for traps presentation.

### Phase 3 addition (Month 6-12)

18a. Paired-clear semantics as a profile field (`clear_trap_oid:`) — per R20. First release uses operator-pattern reconciliation; Phase 3 adds schema-level pairing.

### Phase 4: Polish + late-binding decisions (Month 12+)

19. Topology-aware suppression (Drools-equivalent in Netdata alert engine, or as a separate plugin).
20. ~~Paired-clear semantics~~ (moved to Phase 3 per R20).
21. DTLS / TLS-TM if Rust/Go libraries mature.
22. Northbound trap re-emit.
23. MIB upload via dyncfg UI.

### Cross-phase: continuous

- Per-vendor coverage expansion (community-driven via PRs).
- Benchmark publication on every release.

---

## §6 Risks the team must know

Load-bearing assumptions that, if wrong, invalidate the design.

### Risk 1: The systemd-journal writer throughput claim

- **Assumption**: 30k rows/sec per writer thread is real and reproducible on commodity hub hardware.
- **What's at stake**: §9 partitioning argument; §1's "dozens of thousands/sec" claim.
- **Verification before commit**: bench the existing Netdata Rust journal writer (named at §5 L118) under SNMP-trap workload (~500-byte MESSAGE + 12 fields + 500-2,000-byte JSON varbind payload). If actual measured rate is <10k/sec, the entire throughput model collapses.
- **Mitigation if wrong**: the journal is not the right storage. Drop "every trap always" → "every alerted trap always + periodic samples." But this contradicts the lens §11.12 Trap Retention principle.

### Risk 2: Pysmi / smiparser MIB compilation cost

- **Assumption**: hot-reload of MIB files via inotify is operationally safe and fast.
- **What's at stake**: §7 "operator drops MIB file" UX.
- **Verification**: measure pysmi compile of 100 vendor MIBs from a cold start; measure transitive dependency resolution. If compile takes >5s per MIB on a 5-vendor pack, "hot-reload" is misleading; the operator waits.
- **Mitigation if wrong**: ship a precompiled MIB index (Datadog's choice); operator-provided MIBs require a discrete recompile step.

### Risk 3: gosnmp / Rust SNMP library INFORM correctness

- **Assumption**: §6 L162 INFORM works. Verification: gosnmp 1.42 (which Netdata's polling plugin uses) handles INFORM Response correctly. Telegraf has untested INFORM (`telegraf.md L134`).
- **What's at stake**: protocol compliance for v3-INFORM-mandated environments (financial, healthcare).
- **Verification**: integration test with a real Cisco device sending v2c INFORM and v3 INFORM; verify Response PDU shape via tcpdump.
- **Mitigation if wrong**: drop INFORM from first release; explicit non-goal.

### Risk 4: Cross-language IPC for enrichment

- **Assumption**: §5 L119-121 `netipc` for cross-language state access is mature and fast enough for per-trap enrichment lookup.
- **What's at stake**: enrichment hot path. `netdata-existing-netipc.md` says netipc UDS ping-pong is 183-231k req/s — well above per-trap budget. But the existing netipc service catalogue is **1 service** (cgroups-snapshot); building a new "snmp-poller-state" service is real work.
- **Verification**: prototype the netipc service for SNMP polling state; measure per-trap enrichment overhead.
- **Mitigation if wrong**: enrichment becomes async (the journal lands with un-enriched data; enrichment patches the journal in a follow-up event). This breaks the §11 "journal is the source of truth at trap-receive time" claim.

### Risk 5: Storm survival without per-source rate-limit

- **Assumption**: §10 dedup + kernel UDP buffer + UDP-overflow alert is sufficient.
- **What's at stake**: behaviour under unique-OID storm (a misbehaving device sending varied OIDs at high rate).
- **Verification**: stress test with a synthetic generator (per `comparison/fixture-inventory.md §1.1` OpenNMS udpgen pattern) emitting 50k unique trap OIDs/sec from a single source. Measure: journal-write rate sustained, dedup metric increment rate, lost trap count.
- **Mitigation if wrong**: per-source token-bucket rate-limit becomes Phase 2 mandatory.

### Risk 6: The "existing Netdata alert engine" really alerts on traps

- **Assumption**: §0 L6 alert engine "handles" trap alerting.
- **What's at stake**: §1 Rule 6 "Real-time alerting."
- **Verification**: the existing Netdata alert engine supports OID-aware filtering on per-OID metric instances? If `snmp.trap.cisco_port_security` is a NIDL chart, alerts on its dimensions need NIDL-aware UX. Is that there today?
- **Mitigation if wrong**: alert-on-trap requires alert-engine work; that becomes Phase 1 sequencing.

### Risk 7: Cohort knowledge transfer is real

- **Assumption**: §8 OpenNMS / Centreon / Zenoss / LibreNMS / public-MIB conversion tools deliver curated profile YAML.
- **What's at stake**: §8 "Coverage target" = "LibreNMS-to-Datadog band."
- **Verification**: prototype one conversion (OpenNMS XML → YAML); show that the output is operator-actionable, severity/category-curated, not just OID-resolved.
- **Mitigation if wrong**: §8 becomes "ship raw MIB-derived YAML"; severity defaults all `info`; operator does curation. The day-1 visibility claim becomes weaker.

### Risk 8: Hub-local design without cross-hub correlation

- **Assumption**: `netdata-snmp-hub-architecture.md` operators don't need cross-site trap correlation.
- **What's at stake**: customers with multi-site enterprises (MSP, large enterprise) expect a manager-of-managers view.
- **Verification**: review customer ask in support channels; cohort evidence — every centralized cohort system (OpenNMS, Centreon, SolarWinds, etc.) has central correlation as a feature.
- **Mitigation if wrong**: Cloud-side aggregation must cover the use case (D9 in §3).

### Risk 9: Journal corruption operator-visible recovery

- **Assumption**: systemd-journal failure modes are handled.
- **What's at stake**: hub trap pipeline survives a corrupted journal file without losing in-flight traps.
- **Verification**: simulate journal file corruption (e.g., `truncate /var/log/journal/.../system.journal`); verify the trap listener continues to accept traps and falls back to a fresh journal file with operator alert.
- **Mitigation if wrong**: the listener has a separate disk-backed queue (small, fast SSD-only) to bridge journal-write outages. Adds storage tier the design promised to avoid.

### Risk 10: 5,000 vnodes per hub for the events context

- **Assumption**: §12 L537 per-device vnode for `snmp.trap.events`.
- **What's at stake**: hub TSDB cardinality. 5,000 devices × 9 categories = 45,000 dimensions on one chart × 1Hz emission = 45,000 dimension-seconds/sec just for this one chart.
- **Verification**: stress test with 5,000 synthetic devices; measure agent memory + TSDB write rate + chart-render latency.
- **Mitigation if wrong**: group the events chart by site/vendor, not per-device; per-device-detail only via labels (Logs UI), not chart dimensions.

---

End of Phase B implications. Companion stress-test in `netdata-stress-test.md`.
