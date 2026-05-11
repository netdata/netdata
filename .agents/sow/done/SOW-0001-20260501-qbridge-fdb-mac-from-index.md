# SOW-0001 - SNMP topology: index-based extraction for not-accessible columns + observability + profile-engine macro

## Status

Status: completed

Sub-state: implementation, regression fix, PR creation, first review-thread fix, Sonar duplication cleanup, and SOW close completed. Live-device validation against the originally reported affected device was not independently runnable in this workspace; residual runtime confirmation remains a PR/local-testing concern.

## Requirements

### Purpose

Make the topology view show every L2 endpoint reachable through every managed switch on the network — laptops, IoT, printers, servers, AP-attached clients — across all common FDB protocol variations and vendor implementations, not just on the subset that happens to fit the path the code currently exercises. Today, on standards-compliant Q-BRIDGE-MIB-only switches, zero FDB endpoints surface. This is the dominant case in real enterprise LANs that skipped the legacy BRIDGE-MIB FDB.

The work bundles three confirmed bug sites that share one root cause (profile asks for SNMP fields marked `MAX-ACCESS not-accessible` as if they were readable columns), small operational-visibility improvements that surface alongside the fix, and a profile-engine macro that prevents the bug class from recurring.

### User Request

After landing the Netgear-switch profile fix (PR #22366) and seeing LLDP work but FDB stay empty, the user asked for a comprehensive plan covering all FDB variations — not a narrow patch.

Verbatim user request: *"Have you done a plan on what is needed to fix the issue and support all the variations of FDB properly?"*

Subsequent user direction (recorded as locked decisions below): single PR, multiple isolated commits per group, full SOW documenting every issue, external verification by 5 multi-agent reviewers, cross-check against LibreNMS source code in mirrored repos.

### Assistant Understanding

Facts (verified):

- Three SNMP table fields are walked in our profiles as if they were readable columns, but the source MIBs declare them `MAX-ACCESS not-accessible`. On strict-spec devices, the fields return no value; the runtime sinks then drop the rows because the relevant tag (MAC, IP, etc.) is empty.
  - A1: Q-BRIDGE-MIB FDB MAC (`dot1qTpFdbAddress`) — RFC 4363, used as INDEX of `dot1qTpFdbEntry`.
  - A2: IP-MIB modern ARP/ND — `ipNetToPhysicalIfIndex`, `ipNetToPhysicalNetAddressType`, `ipNetToPhysicalNetAddress` are all `not-accessible` per RFC 4293. Note that `ipNetToPhysicalPhysAddress` (the MAC itself) **is accessible** (`MAX-ACCESS read-create`); only the three index components are not-accessible.
  - A3: LLDP-MIB local management address — `lldpLocManAddrSubtype` and `lldpLocManAddr` are `not-accessible` per IEEE 802.1AB-2005, used as INDEX of `lldpLocManAddrEntry`.
- LibreNMS, OpenNMS, and `netdisco/snmp-info` all extract the MAC for Q-BRIDGE FDB **from the OID index, not from a column**. None of them read `dot1qTpFdbAddress` as a column. Verified directly in mirrored source:
  - `librenms/librenms @ 90115d62d82a`: `includes/discovery/fdb-table/bridge.inc.php:31-98`
  - `OpenNMS/opennms @ 032d82cc926f`: `features/enlinkd/adapters/collectors/bridge/src/main/java/org/opennms/netmgt/enlinkd/snmp/Dot1qTpFdbTableTracker.java:49-119`
  - `netdisco/snmp-info @ 613d360b629d`: `lib/SNMP/Info/Bridge.pm:131-178`
- The profile engine already supports `format: ip_address` on index-derived values (`index_tag_value.go:51-87`) and `format: mac_address` on column-derived values (`utils.go:61`, `value_processor.go:39`). Adding `format: mac_address` to the index-derived value path is a small additive change parallel to the existing `ip_address` case.
- The existing `index_transform` mechanism (`{start: N, drop_right: M}`) already exists at `metrics.go:184` and is applied at `table_row_processor.go:107`. A2/A3 will use this same mechanism in addition to the new `mac_address` formatter.
- Office field validation (sanitized): of four managed switches polled in a real LAN (vendors: MikroTik routerOS, MikroTik SwOS, Zyxel XS-class, Zyxel GS-class), three are backstopped by BRIDGE-MIB FDB (works), one is accidentally lenient by emitting `dot1qTpFdbAddress` as a column despite the spec (works by accident). None hit the strict-only-Q-BRIDGE failure mode. The originally reported failing device is in that strict-only failure mode.

Inferences:

- The Q-BRIDGE-MIB FDB profile, the modern IP-MIB ARP profile, and the LLDP local management address profile were likely shaped by mechanical extension of older profiles whose source MIB columns were accessible. Without per-author MIB-discipline awareness, the same mistake will recur.
- The "lenient vendor returns the not-accessible column" hypothesis was previously unverified; it is now empirically confirmed against a real device but does not change the implementation choice (column-derived and index-derived MACs are byte-identical when both exist, per RFC 4363 §4 — both encode the same MAC address bytes).
- Static FDB tables (`dot1qStaticUnicastTable`, `dot1dStaticTable`) are not just feature gaps; they have different semantics (filtering policy, multi-egress, allowed-port lists) that justify separate design rather than bundling here.

Unknowns:

- None block the locked decisions or implementation plan. Per-vendor support for proprietary FDB MIBs and the newer IEEE 802.1Q-2014 `IEEE8021-Q-BRIDGE-MIB` are deferred as listed in Followup.

### Acceptance Criteria

- A live SNMP poll of the originally reported affected device (Netgear GS110TP v3, sysObjectID `1.3.6.1.4.1.4526.100.4.19`) produces FDB endpoints whose count matches `dot1qTpFdbPort` row count from a fresh walk. Verification: `topology:snmp` function output + manual diff against the walk.
- ARP/ND entries on a strict-spec L3 device are ingested with non-empty IP, ifIndex, and address type. Verification: regression fixture + targeted test, plus opportunistic real-device validation.
- LLDP local management address is correctly populated on devices that implement the standards-compliant variant of `lldpLocManAddrTable`. Verification: regression fixture.
- Existing behavior on devices that already work (BRIDGE-MIB FDB, lenient Q-BRIDGE-MIB devices, hybrid devices, Cisco-style per-VLAN context) is preserved bit-for-bit. Verification: existing `snmp_topology` and parity test suites pass without weakening; commit 7 includes a "lenient vendor" regression fixture (a Q-BRIDGE FDB walk where the column happens to populate) that exercises the no-regression path.
- Engine macro `format: mac_address` is available for index-derived values. Output format: lowercase hex, colon-separated, two-digit per octet (`%02x` style), e.g. `aa:bb:cc:dd:ee:ff`. Octet validation: each component must be 0-255; on validation failure return empty string and let the row be dropped. Length-prefix tolerance: when the slice contains 7 components AND the first component equals 6 AND each of the remaining 6 components is a valid octet (0-255), treat as length-prefixed and use the trailing 6 components (defensive handling for F10).
- **MAC output parity (column-side and index-side)**: column-side `format: mac_address` at `ddsnmp/ddsnmpcollector/utils.go:54` currently uses `%02X` (uppercase) and is exercised by tests at `collector_table_test.go:934-939` and `collector_device_meta_test.go:276-278` that expect uppercase output. As part of commit 1, switch the column-side format string to `%02x` (lowercase) and update those tests so both paths produce byte-identical lowercase output. Add an explicit column-vs-index parity unit test that runs both formatters on the same input and asserts byte equality. Rationale: `net.ParseMAC` and `normalizeMAC` (`topology_hex_normalization.go:18,31`) both lowercase, and industry tooling (LibreNMS, `ip` command, `ifconfig`) renders MACs lowercase — converging to lowercase is the natural shape.
- Index-based extraction for A2 (modern ARP) handles RFC 4293 InetAddress encoding: a leading length octet (4 for IPv4, 16 for IPv6) preceding the address bytes. **The length octet is stripped at the profile level** via `index_transform: [{start: N, drop_right: M}]` on each affected tag (`arp_ip` tag uses positions after the length byte; `arp_addr_type` and `arp_if_index` use the earlier index positions). This keeps `formatIndexIPAddress` pure and documents the SNMP encoding where the MIB structure lives, rather than burying it in the formatter. (Alternative — extending `formatIndexIPAddress` to detect the prefix — was considered and rejected for keeping the formatter format-only.)
- VLAN attribution falls back to `fdbID == VLAN_ID` when the `dot1qVlanCurrentTable` mapping is absent. Specifically: in `topology_cache_fdb.go:46-49`, if `c.fdbIDToVlanID[entry.fdbID]` returns empty, set `entry.vlanID = entry.fdbID`. This is a NEW fallback being ADDED alongside the existing mapping lookup, mirroring LibreNMS `bridge.inc.php:78`. Verification: unit test.
- FDB rows referencing bridge ports without an `ifIndex` mapping are still ingested but tagged as unmapped. Implementation: an internal counter (not a chart) tracks unmapped-bridge-port FDB rows per poll cycle, plus a single rate-limited log line per poll cycle when the count is nonzero. Verification: unit test.
- Warn-on-drop: when FDB rows are dropped due to empty MAC at `topology_cache_fdb.go:11-13`, emit at most ONE warning log per poll cycle with a count of dropped rows (not one log per row). Verification: unit test simulating a strict-Q-BRIDGE poll.
- The profile-author guardrail exists as a project skill at `.agents/skills/project-snmp-profiles-authoring/SKILL.md` (note `project-` prefix per AGENTS.md:35,196,228 — runtime project skills MUST use this prefix) and as a new "Field accessibility" section in `src/go/plugin/go.d/collector/snmp/profile-format.md`. The profile-format.md section includes an audit recipe (a `grep` pattern for finding profile YAML symbols whose MIB declares `not-accessible`). Verification: file exists, links to profile-format.md from the skill, content reviewed in the multi-agent verification round.
- All eight commits in the PR pass the existing `snmp_topology`, `pkg/topology/engine`, and `ddsnmp` test suites without regression.
- Sensitive-data discipline: no community strings, bearer tokens, customer-identifying IPs, customer hostnames (sysName, sysDescr, ifAlias, ifDescr, LLDP remote names, LLDP port descriptions, chassis IDs, management addresses pointing to customer infrastructure), SNMPv3 usernames/auth/priv secrets, or community-member names appear in any committed file (SOW, profile YAMLs, code comments, tests, fixtures, commit messages, PR body, skill, profile-format.md update).

## Analysis

### Master gap & bug list (44 items)

#### A. Confirmed parsing bugs — strict-spec devices return zero / incomplete data (3)

| # | Site | Source MIB & spec | File:line | Effect |
|---|---|---|---|---|
| **A1** | Q-BRIDGE-MIB FDB MAC | RFC 4363 — `dot1qTpFdbAddress` is `MAX-ACCESS not-accessible`, INDEX of `dot1qTpFdbEntry` | `_std-topology-q-bridge-mib.yaml:21-25` (column read), drop at `topology_cache_fdb.go:10-13` | 0 FDB endpoints on strict-only-Q-BRIDGE devices (Netgear smart switches, parts of MikroTik SwOS, parts of Zyxel) |
| **A2** | IP-MIB modern ARP/ND index components | RFC 4293 — `ipNetToPhysicalIfIndex`, `ipNetToPhysicalNetAddressType`, `ipNetToPhysicalNetAddress` are all `MAX-ACCESS not-accessible`. The MAC itself (`ipNetToPhysicalPhysAddress`) is `read-create` (accessible). | `_std-topology-fdb-arp-mib.yaml:192-217` reads three not-accessible columns | ARP entries on strict L3 devices lose IP, ifIndex, address type. MAC↔IP correlation breaks. The MAC value continues to be readable. |
| **A3** | LLDP-MIB local management address | IEEE 802.1AB-2005 — `lldpLocManAddrSubtype` and `lldpLocManAddr` are `MAX-ACCESS not-accessible`, INDEX of `lldpLocManAddrEntry` | `_std-topology-lldp-mib.yaml:84-101` reads both not-accessible columns; no compat anchor (the *remote* table at `:184-319` has one — this one does not) | Local LLDP management address absent on strict devices |

#### B. Latent parsing bugs — would surface if we add these tables (4)

| # | Site | Why it'd repeat the pattern | Spec |
|---|---|---|---|
| **B1** | Q-BRIDGE static FDB `dot1qStaticUnicastTable` | `Address` AND `ReceivePort` both `not-accessible`. Only `AllowedToGoTo` is `read-write` | RFC 4363 |
| **B2** | Q-BRIDGE multicast FDB `dot1qTpGroupTable` | `GroupAddress` `not-accessible` | RFC 4363 |
| **B3** | IEEE8021-Q-BRIDGE-MIB FDB | Same shape, INDEX adds `ComponentId` (3-component INDEX) | IEEE 802.1Q-2018 |
| **B4** | IP-MIB modern `ipAddressTable` | `AddrType` and `Addr` both `not-accessible` | RFC 4293 |

#### C. Missing capabilities — not polled today (7)

| # | Capability | Why it matters | Scope decision |
|---|---|---|---|
| **C1** | BRIDGE-MIB static FDB (`dot1dStaticTable`) | RFC 1493 SMIv1 — columns `read-write`, accessible. Different semantics (filtering policy). | **Defer** |
| **C2** | Q-BRIDGE-MIB static FDB (`dot1qStaticUnicastTable`) | Needs index-based extraction (B1). Same semantics caveat as C1. | **Defer** |
| **C3** | IEEE8021-Q-BRIDGE-MIB FDB | Modern alternative for IEEE 802.1Q-2014-only switches. | **Defer** |
| **C4** | Modern `ipAddressTable` | For routers/firewalls running modern IP-MIB only. | **Defer** |
| **C5** | Vendor proprietary FDB MIBs: HUAWEI-L2MAM-MIB, EXTREME-FDB-MIB, DLINKSW-L2FDB-MIB, HP-ICF-BRIDGE (Aruba/HPE), AX-FDB-MIB (AlaxalA), JUNIPER-VLAN/L2ALD-MIB, ALCATEL-IND1-MAC-ADDRESS-MIB (Nokia/Alcatel). LibreNMS also has handlers for EdgeSwitch (Ubiquiti), FortiSwitch (Fortinet), TiMOS (Nokia SR OS), AOS6/AOS7 (Alcatel-Lucent OmniSwitch), and VRP (Huawei). | LibreNMS handlers exist as references. Each is its own design problem. | **Defer** |
| **C6** | CISCO-MAC-NOTIFICATION-MIB (trap-based) | Out-of-SNMP-poll scope | **Defer** |
| **C7** | LLDP-MED, LLDP-EXT-DOT3, LLDP-EXT-DOT1 | Phone/PoE/voice-VLAN/inventory metadata. Out of scope; opt-in by-vendor. | **Defer** |

#### D. Quality / attribution gaps — data appears, partially incomplete (6)

| # | Gap | Evidence | Scope decision |
|---|---|---|---|
| **D1** | No `fdbID == VLAN_ID` fallback when `dot1qVlanCurrentTable` is absent. Today `topology_cache_fdb.go:46-49` only consults `c.fdbIDToVlanID`; if empty, `entry.vlanID` stays empty. The fix ADDS a fallback line that sets `entry.vlanID = entry.fdbID` when the mapping returns nothing | LibreNMS `bridge.inc.php:78` precedent (`$vlan = $vlan_fdb_dict[$vlanIndex] ?? $vlanIndex;`) | **In this PR** |
| **D2** | FDB entries on unmapped bridge ports emit `IfIndex: 0` silently | `topology_observation_local_forwarding.go:29-42`. No counter or warning today | **In this PR** |
| **D3** | No detection of FDB truncation by SNMP agent (large tables on small devices) | No `*counts vs walked* ` validation against `dot1qFdbDynamicCount` | **Defer** |
| **D4** | No deduplication when same MAC is in LLDP remote AND FDB | Could produce two endpoint actors for one device | **Defer** |
| **D5** | Port aggregation (LACP/LAG): FDB → member port ifIndex, no LAG rollup | `bridgePortToIf` is 1:1; LibreNMS also does not roll up | **Defer** |
| **D6** | Cross-protocol freshness (LLDP age vs FDB age vs ARP age) not reconciled | Can produce ghost endpoints | **Defer** |

#### E. Code smells / blind spots (6)

| # | Issue | Where | Scope decision |
|---|---|---|---|
| **E1** | `macFromOIDIndexSuffix` lives in `_test.go`, not in production | `topology_snmprec_forwarding_test.go:546` | **In this PR** (helper logic folded into engine macro) |
| **E2** | Test fixture parser has its own MAC-from-index fallback that bypasses production path | `topology_snmprec_forwarding_test.go:305-336` masks A1 | **In this PR** (resolved by adding fixtures that exercise profile→engine→cache end-to-end) |
| **E3** | LLDP octet reassembly in `topology_management_address_normalization.go` is bespoke; no shared helper | Will duplicate when fixing A1/A3 | Engine macro replaces it (commit 1) |
| **E4** | No logging when FDB rows are dropped due to empty MAC | `topology_cache_fdb.go:11-13` — silent data loss | **In this PR** (rate-limited per poll cycle, not per row) |
| **E5** | Two anchors for `lldpRemManAddrTable` (primary `.1`/`.2` + compat `.3`) — unclear interaction with strict devices | `_std-topology-lldp-mib.yaml:184-319` — works in practice but warrants a comment | **In this PR** (one-line comment) |
| **E6** | No engine-level `index_format: mac` macro | ddsnmp engine missing feature | **In this PR** (engine macro) |

#### F. Vendor / firmware quirks (10)

| # | Quirk | LibreNMS handler | Status in our code | Scope decision |
|---|---|---|---|---|
| **F1** | MikroTik LLDP RemManAddr exposes columns `.1`/`.2` despite spec | (unknown) | Handled (primary anchor in our profile) | n/a |
| **F2** | Zyxel firmware emits malformed Q-BRIDGE indexes that need reshaping | `includes/discovery/fdb-table/zynos.inc.php:27-35` | Not handled | **Defer** |
| **F3** | TP-Link JetStream ifIndex offset of `+49152` between BRIDGE-MIB and IF-MIB | `includes/discovery/fdb-table/jetstream.inc.php:38` | Not handled | **Defer** |
| **F4** | Cisco IOS classic — per-VLAN BRIDGE-MIB via `community@<vlan>` | `includes/discovery/fdb-table/ios.inc.php:29` | Handled (`topology_vlan_context.go`) | n/a |
| **F5** | Some Aruba IAP firmware — truncated Q-BRIDGE indexes | `arubaos.inc.php` partial | Not handled | **Defer** |
| **F6** | Originally reported device firmware bug — 512-byte zero-filled `lldpLocSysCapSupported`/`Enabled` | (Netdata-specific finding) | Tolerated implicitly by `format: hex` | n/a |
| **F7** | Stacked switches (Cisco StackWise, HP IRF) — aggregated FDB on master, per-member on others | Not handled in LibreNMS either | Not handled | **Defer** |
| **F8** | Cisco SB / SF / SG Small Business — non-standard FDB shape | Cisco-SB-specific profile in our codebase | Partially handled | n/a |
| **F9** | Lenient vendors return `dot1qTpFdbAddress` as a column despite spec | LibreNMS doesn't read the column at all (index-only) | Decision 1B (drop column) makes this irrelevant | n/a |
| **F10** | Length-prefix byte in MAC index encoding — some agents (per LibreNMS comment, observed on Aruba CX, Comtrol) prepend a length octet (value 6) before the 6 MAC bytes, producing a 7-component index suffix | `includes/discovery/fdb-table/bridge.inc.php:80-96` | Not handled today | **In this PR** (defensive: when the slice has 7 components and the first equals 6, drop it; engine macro acceptance criterion above) |

#### G. Engine / ddsnmp gaps (4)

| # | Gap | Why it matters | Scope decision |
|---|---|---|---|
| **G1** | No `MAX-ACCESS` validation when authoring profiles | Lets the A-class bug be authored without warning | **In this PR** (skill + profile-format.md) |
| **G2** | No native multi-position index extraction with format coercion | Forces ugly per-octet enumeration; the same fix shape we need for A1, A2, A3 | **In this PR** (engine macro `format: mac_address` for index-derived values) |
| **G3** | No "fail-loud" when a column symbol returns no rows on a populated table | Silent data loss | **Defer** |
| **G4** | No per-poll-cycle stats on rows-fetched vs rows-dropped per metric | Hard to diagnose A-class bugs in production | **Partial** — covered by E4 (warn-on-drop) for the FDB sink |

#### H. Process / documentation gaps (4)

| # | Gap | Scope decision |
|---|---|---|
| **H1** | No MIB-authoring spec or project skill — every author can repeat the not-accessible mistake | **In this PR** — `.agents/skills/project-snmp-profiles-authoring/SKILL.md` (note `project-` prefix per AGENTS.md:35,196,228) |
| **H2** | No same-failure scan was performed at PR review time for this profile family | **In this PR** — reviewers ran the scan, results integrated; project skill encodes the rule for future PRs |
| **H3** | Test fixtures cannot trivially be derived from real SNMP walks (snmprec format conversion not documented) | **Defer** |
| **H4** | No reference table linking each topology MIB column we walk to its accessibility class | **In this PR** — `profile-format.md` MAX-ACCESS section, includes a `grep` audit recipe |

### LibreNMS verification matrix

For each issue, what LibreNMS does. File paths are relative to `librenms/librenms @ 90115d62d82a`.

| Issue | LibreNMS handles? | Code path | Approach |
|---|---|---|---|
| A1 (Q-BRIDGE FDB MAC) | Yes | `includes/discovery/fdb-table/bridge.inc.php:31-35` (walk), `:98` (parse), `:80-96` (length-prefix tolerance) | Walk `dot1qTpFdbPort` (column .2, accessible); extract MAC from index. No column read. |
| A2 (IP-MIB ARP `ipNetToPhysicalTable`) | Yes | `LibreNMS/Modules/ArpTable.php:104-118` | Walk `ipNetToPhysicalPhysAddress` and consume the **structured table keys** returned by `->table(1)` (a hierarchical ifIndex→addrType→address map; not a raw OID-suffix split). Shape repair at line 120. The MAC value comes from the column (which is accessible); IP/ifIndex/addrType come from the structured key path. |
| A3 (LLDP local mgmt addr) | No | n/a — table not polled | LibreNMS sidesteps by not reading this table at all. We choose the right thing: index extraction. |
| B1/C2 (Q-BRIDGE static FDB) | No | n/a — table not polled | Same gap |
| B3/C3 (IEEE8021-Q-BRIDGE-MIB) | Partial | `LibreNMS/OS/Traits/QBridgeMib.php:50-58` | Used only for VLAN names; FDB extraction stays on classic Q-BRIDGE-MIB |
| B4/C4 (modern `ipAddressTable`) | Partial | IPv4 still uses deprecated `ipAddrTable` (`LibreNMS/Modules/Ipv4Addresses.php:195`); IPv6 uses modern `ipAddressTable` (`LibreNMS/Modules/Ipv6Addresses.php:148`) | Mixed: deprecated for v4, modern for v6 |
| C5 (vendor proprietary FDBs) | Yes (≥11 vendors) | `includes/discovery/fdb-table/{arubaos,vrp,ios,aos6,aos7,zynos,jetstream,edgeswitch,fortiswitch,timos,...}.inc.php` | Per-vendor override files |
| D1 (VLAN fdbID==VLAN_ID fallback) | Yes | `bridge.inc.php:78` | `$vlan = $vlan_fdb_dict[$vlanIndex] ?? $vlanIndex;` |
| F2 (Zyxel malformed Q-BRIDGE index) | Yes | `zynos.inc.php:27-35` | Reshape pass before normal parsing |
| F3 (TP-Link JetStream offset) | Yes | `jetstream.inc.php:38` | Hardcoded `+49152` ifIndex offset |
| F4 (Cisco per-VLAN context) | Yes | `ios.inc.php:29` | `SnmpQuery::context($vlan_raw, 'vlan-')->walk('BRIDGE-MIB::dot1dTpFdbPort')` |
| F10 (length-prefix byte) | Yes | `bridge.inc.php:80-96` | Defensive: detect and strip the 7-byte index encoding |
| Bridge-port → ifIndex | Yes | `bridge.inc.php:49-54` (build), `:108` (fallback to `basePort==ifIndex`) | Walks `dot1dBasePortIfIndex`; falls back when missing |
| Stack / multi-component bridge | No | n/a | Same gap as ours |
| LACP / port-channel rollup | No | n/a | Same gap as ours |
| Test coverage / fixtures | Limited | `tests/data/timos_fdb-table.json`, `tests/snmpsim/zynos_gs1900-fdb.snmprec` | Two relevant fixtures |

Cross-references:

- `netdisco/snmp-info` `Bridge.pm:160-178` — `_qb_fdbtable_index` (lines 160-165) decodes MAC from index; `qb_fw_mac` (lines 167-178) walks `qb_fw_port` and applies the decoder. Never reads the not-accessible column.
- OpenNMS `features/enlinkd/adapters/collectors/bridge/src/main/java/org/opennms/netmgt/enlinkd/snmp/Dot1qTpFdbTableTracker.java:64` (column collection: port + status only), `:113` (decodes address from row index).

**Conclusion**: index-based extraction is the universal industry approach for A1. A2 and A3 are RFC-backed analogs with the same mechanical fix shape, but the three reference implementations do not directly cover them — A2 has a partial parallel in LibreNMS `ArpTable.php` (which uses the structured-table-key approach, semantically equivalent), A3 has no reference precedent (LibreNMS skips the table). Decision 1B is the right call for all three; A1 is independently proven, A2/A3 are RFC-driven.

### Risks (cross-cutting)

- **Engine macro added in this PR**: contained scope. The format handler extends `formatIndexTagValue` with `case "mac_address":` parallel to the existing `case "ip_address":`. Octet validation (each component 0-255) and length-prefix tolerance (drop a leading length-of-6 octet when the slice has 7 components) are part of the formatter, mirroring `formatIndexIPAddress` patterns and LibreNMS behavior. Engine has the existing `format: ip_address` precedent that exercises the same code path.
- **A2/A3 implementation completeness**: the new `format: mac_address` formatter is necessary but not sufficient. A2 and A3 need correct `index` + `index_transform` declarations in the profile YAML to slice the OID suffix to the right components before formatting. For A2 in particular, the IP component of the OID index is RFC 4293 InetAddress-encoded (a length octet followed by address bytes); the length octet is **stripped at the profile level** via `index_transform` so the `format: ip_address` handler stays pure (format-only). The engine macro adds `format: mac_address` for index values; profile-level slicing handles SNMP encoding peculiarities.
- **Removing the column read** (Decision 1B): index-derived MAC and column-derived MAC are byte-identical when both exist (RFC 4363 §4 — both encode the same MAC bytes). On lenient devices that today populate the column, behavior stays correct. On strict devices, FDB starts working. No device regresses. The lenient-vendor regression fixture in commit 7 verifies this.
- **VLAN fallback (D1)** could over-attribute if a device uses `fdbID != VLAN_ID` mapping but doesn't expose `dot1qVlanCurrentTable`. LibreNMS accepts this trade-off; the fallback is a documented best-effort.
- **Warn-on-drop (E4)** could be noisy. Implementation MUST be rate-limited to one log line per poll cycle with a count, not one per dropped row. This is in the acceptance criteria.

## Pre-Implementation Gate

Gate state at implementation start: decisions locked; implementation authorized after validation.

Problem / root-cause model:

- Three SNMP table fields are walked in our profiles as if they were readable columns, but the source MIBs declare them `MAX-ACCESS not-accessible`. Strict-spec devices correctly return no value for those columns. Our runtime sinks then drop the rows because the relevant tag (MAC, IP, etc.) is empty. The bug class repeats because there is no project-level rule requiring profile authors to consult MIB MAX-ACCESS, and no engine-level macro making the right way (index extraction with format coercion) ergonomic.

Evidence reviewed:

- RFCs 1493, 4188, 4293, 4363; IEEE 802.1AB-2005; IEEE 802.1Q-2018 (downloaded into `/tmp/sow-verify/`).
- Profile YAMLs: `_std-topology-q-bridge-mib.yaml`, `_std-topology-fdb-arp-mib.yaml`, `_std-topology-lldp-mib.yaml`, `_std-topology-stp-mib.yaml`, `_std-topology-cisco-vtp-mib.yaml`.
- Runtime sinks: `topology_cache_fdb.go`, `topology_cache_metric_dispatch.go`, `topology_cache_tags.go`, `topology_observation_local_forwarding.go`, `topology_vlan_context*.go`, `topology_observation_local_identity.go`.
- Engine: `ddsnmp/ddprofiledefinition/{metrics.go,validation.go,selector.go}`, `ddsnmp/ddsnmpcollector/{table_row_processor.go,index_tag_value.go,utils.go,value_processor.go,cross_table_lookup.go}`.
- `librenms/librenms @ 90115d62d82a` source (file:line citations in matrix above).
- `netdisco/snmp-info @ 613d360b629d`: `lib/SNMP/Info/Bridge.pm` and `lib/SNMP/Info/IEEE802_Bridge.pm`.
- `OpenNMS/opennms @ 032d82cc926f`: `features/enlinkd/.../Dot1qTpFdbTableTracker.java`.
- Live SNMP walks from one originally reported affected device and four office-network managed switches (3 vendors).
- Two rounds of external multi-agent reviews (5 reviewers per round: Codex, GLM, MiniMax, Qwen, Kimi). Round 2 produced 20 fixes that have been applied to this SOW.

Affected contracts and surfaces:

- Profile YAMLs: `_std-topology-q-bridge-mib.yaml`, `_std-topology-fdb-arp-mib.yaml`, `_std-topology-lldp-mib.yaml`.
- Runtime: `topology_cache_fdb.go`, `topology_cache_tags.go`, `topology_observation_local_forwarding.go`.
- Engine: `ddsnmp/ddsnmpcollector/index_tag_value.go` (extending `formatIndexTagValue`).
- New skill: `.agents/skills/project-snmp-profiles-authoring/SKILL.md`.
- Updated docs: `src/go/plugin/go.d/collector/snmp/profile-format.md` (new "Field accessibility" section + audit recipe).
- Test data: new snmprec fixtures under `src/go/plugin/go.d/collector/snmp_topology/testdata/`.
- No public/operator docs change beyond release notes.
- No public CLI / UI / schema change.
- No `AGENTS.md` change required (skill follows the `project-*` convention; no legacy registration needed).

Existing patterns to reuse:

- LLDP management-address octet decomposition in `_std-topology-lldp-mib.yaml:206-239` and the runtime reassembly in `topology_management_address_normalization.go:13-35` (`reconstructLldpRemMgmtAddrHex`) — supersedable once the engine macro lands; kept as production reference until then.
- VLAN-context plumbing in `topology_vlan_context.go` is robust and need not change.
- Test helper `macFromOIDIndexSuffix(parts []string)` at `topology_snmprec_forwarding_test.go:546` — its decode logic moves into production as part of the engine macro (commit 1).
- LibreNMS `bridge.inc.php` "fdbID == vlanID" fallback at `:78` — directly mirrored.
- Existing `formatIndexIPAddress` at `index_tag_value.go:58-87` — the new `formatIndexMACAddress` parallels it (validation, length-prefix tolerance, error path returning empty string).

Risk and blast radius:

- A1+A2+A3 fixes: low. Additive (column read removed, index extraction added). No device regresses (RFC 4363 §4 guarantees byte equivalence; lenient-vendor fixture in commit 7 verifies).
- Engine `format: mac_address` for index-derived values: low. Mirrors existing `format: ip_address`; tests reuse the same harness.
- D1 VLAN fallback: low. Documented best-effort matching LibreNMS.
- D2 IfIndex tracking: low. Internal counter + one log line per poll cycle. No chart, no public metric.
- E4 warn-on-drop: low. Rate-limited per poll cycle.
- Static FDB additions: deferred — different semantics, separate SOW.
- Per-vendor SOWs: deferred, no risk in this PR.

Sensitive data handling plan:

- All durable artifacts in this SOW (the SOW itself, profile YAMLs, code, code comments, tests, fixtures, commit messages, PR body, the new skill, the profile-format.md update) must contain zero raw sensitive data: no community member or customer names, no SNMP communities, no bearer tokens, no SNMPv3 usernames / authentication / privacy secrets, no customer-identifying IPs (private RFC1918 IPs are acceptable when used as illustrative examples in profile-format.md, never in fixtures derived from real walks), no customer-identifying device strings (real-world `sysName`, `sysDescr`, `ifAlias`, `ifDescr`, LLDP remote `sysName`, LLDP remote port descriptions, real chassis IDs, customer-pointing management addresses), no proprietary incident details.
- Use placeholders (`[REDACTED]`, "the user", "the reporter", "originally reported affected device"), public product names (e.g. "Netgear GS110TP v3" is a publicly sold product, not PII), and file:line citations.
- Test fixtures derived from real device walks must be sanitized: replace customer hostnames in port descriptions or sysName with neutral labels (`endpoint-1`, `port-A`, etc.), and strip or stub LLDP remote-name fields and LLDP port descriptions. Sanitization happens before the fixture is staged.
- Pre-commit checklist (run before each commit and before opening the PR). Uses `rg -P` (ripgrep with PCRE) — POSIX `grep -E` does not support negative lookahead and would silently fail. The checklist scans **both staged and unstaged** content, plus commit messages and the PR body separately.
  ```bash
  # Helper: stream all uncommitted changes (staged + unstaged)
  git_diff_all() { { git diff --cached; git diff; }; }

  # 1. credentials and tokens in any uncommitted content
  git_diff_all | rg -P '(community|bearer|token|secret|password|auth.?key|priv.?key|snmpv3)\s*[:=]'

  # 2. customer-identifying IPv4 outside RFC1918, and IPv6 global-unicast-like
  #    addresses (2000::/3). The OID-context exclusion
  #    (^| [^0-9.])(?!1\.[0-3]\.6\.1\.) avoids flagging SNMP OIDs like
  #    1.3.6.1.4.1.x. The lookahead chain after that excludes private IPv4
  #    ranges. The IPv6 pattern is case-insensitive and intentionally excludes
  #    ULA fc00::/7.
  git_diff_all | rg -P '(?<![0-9.])(?!1\.[0-3]\.6\.1\.)(?!10\.)(?!172\.(1[6-9]|2[0-9]|3[01])\.)(?!192\.168\.)\b[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\b'
  git_diff_all | rg -P '(?i)\b(?:2[0-9a-f]{3}|3[0-9a-f]{3}):[0-9a-f:]+\b'

  # 3. customer-identifying device strings (replace with neutral labels in fixtures)
  git_diff_all | rg -iP '(sysName|sysDescr|ifAlias|ifDescr|chassis.?id|mgmt.?addr|lldp.?rem.?(sys.?name|port.?desc))'

  # 4. proper names that surfaced in chat or Slack during work on this SOW.
  #    Maintain the personal-name list in a local environment file (.env or
  #    AGENTS.local.md, both gitignored). Never embed names in this SOW.
  #    Example: NAME_PATTERN="Firstname1|Firstname2|Surname1"; export NAME_PATTERN
  git_diff_all | rg -iP "$NAME_PATTERN"

  # 5. commit messages — three coverage paths because each catches a different point:
  #    a) prepare-commit-msg hook: scan $1 (the in-progress message file)
  #       before commit goes through.
  #    b) post-commit verification of the most recent commit:
  git log -1 --format='%B' | rg -iP '(community|bearer|token|secret|password|auth.?key|priv.?key|snmpv3)\s*[:=]|sysName|sysDescr|ifAlias|ifDescr|chassis.?id|mgmt.?addr'
  git log -1 --format='%B' | rg -iP "$NAME_PATTERN"
  #    c) range scan across all commits not yet merged to upstream/master,
  #       just before opening the PR:
  git log upstream/master..HEAD --format='%B' | rg -iP '(community|bearer|token|secret|password|auth.?key|priv.?key|snmpv3)\s*[:=]|sysName|sysDescr|ifAlias|ifDescr|chassis.?id|mgmt.?addr'

  # 6. PR body — apply the same patterns to the body before `gh pr create` /
  #    `gh pr edit --body-file`. If using a body file, run the patterns
  #    against that file directly.
  ```
  Each `rg` invocation MUST return zero lines before commit / PR open. If `rg` is unavailable, the regex set requires it and the checklist cannot be satisfied with `grep` alone — install ripgrep first.

Implementation plan: see "Plan" section below — 8 commits, single PR off `upstream/master`, branch `snmp-qbridge-fdb-mac-from-index`. Validation before implementation found one required engine prerequisite: current `index_transform` cannot express "start at N and keep the rest" (`validation.go` rejects `start > end`, and `applyIndexTransform` requires an explicit in-bounds `end`). The user accepted extending `index_transform` so `start > 0` with omitted/zero `end` means "through the last index component"; `start: 0, end: 0` keeps its existing "first component only" behavior.

Validation plan:

- Unit tests for: engine `format: mac_address` formatter (octet validation 0-255, length-prefix tolerance, output format `aa:bb:cc:dd:ee:ff`); A1+A2+A3 profile→engine→cache flow on synthetic strict-spec fixtures; D1 VLAN fallback; D2 unmapped-bridge-port counter; E4 rate-limited warn-on-drop.
- Snmprec fixtures: at least one strict-spec Q-BRIDGE FDB fixture (sanitized derivative of the user-reported walk), one strict-spec modern ARP fixture (synthetic, RFC 4293 InetAddress-encoded), one strict-spec LLDP local mgmt addr fixture (synthetic), one **lenient-vendor** Q-BRIDGE FDB fixture (column populated; verifies no-regression on the path that already works).
- Existing test suites must pass without weakened assertions: `snmp_topology`, `pkg/topology/engine`, `ddsnmp`.
- Manual validation: live poll against the originally reported affected device showing FDB endpoint count matches walked row count.
- Same-failure scan output: documented in this SOW under `## Validation` after commit 7. No further not-accessible-as-column sites detected by the multi-agent review.
- Profile-format.md and skill content: reviewed in the multi-agent verification round before any commits.

Artifact impact plan:

- AGENTS.md: update required. The new runtime project skill follows the `project-*` convention, and the Project Skills Index must stop saying no `project-*` skills exist.
- Runtime project skills: new `.agents/skills/project-snmp-profiles-authoring/SKILL.md` (in this PR).
- Specs under `.agents/sow/specs/`: no expected change. The skill + profile-format.md cover the authoring rule.
- End-user/operator docs: `profile-format.md` gets a new "Field accessibility" section with the rule + audit recipe.
- End-user/operator skills: none.
- SOW lifecycle plan at implementation start: move from `pending/` to `current/` after user authorization, then close after merge or explicit user request. Actual closure is recorded in the execution log and validation gate.

Open decisions: none. All five locked below.

## Locked Decisions

### Decision 1 — MAC extraction approach for not-accessible columns

**Locked: Option B — drop column read entirely, extract from index only.**

Reasoning: For A1, three reference implementations (LibreNMS `bridge.inc.php`, SNMP::Info `Bridge.pm`, OpenNMS `Dot1qTpFdbTableTracker.java`) all use index-only extraction; none reads the not-accessible column. For A2 and A3, the same approach is RFC-backed and mechanically identical, though direct industry parallels are fewer (LibreNMS handles A2 via a structured-table-key path semantically equivalent to index extraction; A3 is sidestepped by not polling the table). The "lenient vendor returns the column" hypothesis is empirically true for at least one vendor (Zyxel GS-class) but does not change the choice — column-derived and index-derived MACs are byte-identical (RFC 4363 §4), so removing the column read produces identical bytes on lenient devices and starts working on strict devices. Two paths multiplied across 3 sites = unjustified maintenance burden.

### Decision 2 — Authoring guardrail format and path

**Locked: project skill at `.agents/skills/project-snmp-profiles-authoring/SKILL.md` (note `project-` prefix per AGENTS.md:35,196,228 — runtime project skills MUST use this prefix). Plus a new "Field accessibility" section in `src/go/plugin/go.d/collector/snmp/profile-format.md` containing the rule and a `grep` audit recipe. The skill links to profile-format.md.**

Reasoning: The user requested skill-shape rather than spec-shape. profile-format.md is the canonical authoring reference (2046 lines, no current MAX-ACCESS mention) and is the right home for the rule itself. The skill points authors at it. The `project-` prefix follows the project convention; using a different prefix would require an explicit registration entry in AGENTS.md, which is unjustified for a new skill.

### Decision 3 — Quality / observability improvements in this PR

**Locked: Option A — include all three.**

- D1 VLAN fallback: resolve `fdbID == VLAN_ID` as a late fallback when producing observations, after the `dot1qVlanCurrentTable` mapping has had a chance to populate. Do not eagerly store `entry.vlanID = entry.fdbID` in `updateFdbEntry`, because Q-BRIDGE FDB rows are processed before the VLAN mapping table in the current profile order and an eager fallback can block a later correct mapping.
- D2 IfIndex tracking: when `parseIndex(c.bridgePortToIf[bridgePort])` returns 0, increment an internal per-poll counter; emit at most ONE log line per poll cycle if the counter is nonzero. No new chart or public metric.
- E4 warn-on-drop: when `topology_cache_fdb.go:11-13` drops rows due to empty MAC, emit at most ONE rate-limited warning per poll cycle with a count of dropped rows (not one log per row).

Reasoning: Each is file-local, additive, cheap, and ensures the next bug of this class is visible immediately rather than requiring forensic walks.

### Decision 4 — Engine macro G2 in this PR; static FDB / IEEE8021-Q-BRIDGE-MIB / modern ipAddressTable / vendor proprietary deferred to child SOWs

**Locked: engine macro IN this PR. Static FDB, IEEE8021-Q-BRIDGE-MIB, modern `ipAddressTable`, vendor-proprietary FDB MIBs all DEFERRED — child SOWs opened before this SOW closes.**

Reasoning: Engine macro is a small additive change (extends `formatIndexTagValue` with `case "mac_address":` parallel to the existing `case "ip_address":`, plus octet validation and length-prefix tolerance). Static FDB has different semantics (filtering policy, multi-egress) and warrants its own design SOW. IEEE8021-Q-BRIDGE-MIB and modern `ipAddressTable` have no real-device evidence in this SOW justifying immediate work. Vendor-proprietary FDBs are each their own design problem.

### Decision 5 — `index_transform` variable-tail support

**Locked: Option A — extend `index_transform` semantics.**

Reasoning: Q-BRIDGE MAC extraction and variable-length IP/LLDP management-address indexes need "slice from this index component through the end". Current `index_transform` can only select explicit inclusive ranges or drop a fixed number of right-side components. Duplicating tags for every possible IPv4/IPv6 or normal/length-prefixed shape would be brittle and order-sensitive because tag insertion does not overwrite an existing non-empty tag. Extending the engine is low risk because `start > 0, end == 0, drop_right == 0` is currently invalid, while existing `start: 0, end: 0` keeps its current first-component meaning.

## Plan

Single PR, branch `snmp-qbridge-fdb-mac-from-index` off `upstream/master`. Eight commits, isolated per group:

| # | Commit | Files touched | Issues addressed |
|---|---|---|---|
| 1 | `engine: add format mac_address for index-derived tag values; converge column-side to lowercase` | `ddsnmp/ddsnmpcollector/index_tag_value.go` (new `formatIndexMACAddress` + switch case, lowercase output), `ddsnmp/ddsnmpcollector/table_row_processor.go` and `ddsnmp/ddprofiledefinition/validation.go` (extend `index_transform` so `start > 0` with omitted/zero `end` keeps the tail), `ddsnmp/ddsnmpcollector/utils.go:54` (flip `%02X` → `%02x` for parity), `ddsnmp/ddsnmpcollector/collector_table_test.go:934-939` and `collector_device_meta_test.go:276-278` (update existing assertions from uppercase to lowercase), unit tests including an explicit column-side ↔ index-side parity test | G2, E1 (folds in `macFromOIDIndexSuffix` decode logic), F10 (length-prefix tolerance with octet validation), MAC parity (column-side and index-side both produce lowercase `aa:bb:cc:dd:ee:ff`), variable-tail index extraction needed by A1/A2/A3 |
| 2 | `profile + runtime: q-bridge fdb mac from index` | `_std-topology-q-bridge-mib.yaml` (replace octet enumeration with `format: mac_address` + `index_transform`), `topology_cache_tags.go` (rename/cleanup tag constants if needed), `topology_cache_fdb.go` (consume the new tag value), unit tests | A1 |
| 3 | `profile + runtime: ip-mib modern arp from index` | `_std-topology-fdb-arp-mib.yaml` — replace not-accessible column reads with `index` + `index_transform` extraction. For `arp_if_index` and `arp_addr_type`: single `index: N` lookups. For `arp_ip`: `index_transform: [{start: <after-length-byte>}]` to skip the RFC 4293 InetAddress length octet *at the profile level*, then `format: ip_address` consumes a clean 4 / 16 octet sequence (formatter stays format-only; SNMP-encoding logic stays in the profile YAML where the MIB structure is documented). `topology_cache_stp_arp.go` and any tag consumer; unit tests including an IPv4 + IPv6 strict-spec fixture pair. | A2 |
| 4 | `profile + runtime: lldp local management address from index` | `_std-topology-lldp-mib.yaml`, `topology_management_address.go` and/or `topology_management_address_normalization.go` (consumer), unit tests | A3, E5 (one-line comment near the dual anchors of `lldpRemManAddrTable`) |
| 5 | `runtime: fdbID == VLAN_ID fallback when mapping table absent` | `topology_observation_local_forwarding.go` (late fallback to `entry.fdbID` only when no mapped VLAN ID exists at observation-output time), `topology_cache_fdb.go` (keep cache mutation compatible with late mapping), unit tests | D1 |
| 6 | `runtime: warn-on-drop + unmapped-bridge-port counter` | `topology_cache_fdb.go:11-13` (rate-limited per-poll-cycle warning with count), `topology_observation_local_forwarding.go:29-42` (per-poll-cycle counter for `IfIndex == 0` cases + single log line when nonzero), unit tests verifying rate-limiting | D2, E4 |
| 7 | `tests: snmprec fixtures + regression tests` | new fixtures under `src/go/plugin/go.d/collector/snmp_topology/testdata/`: strict-spec Q-BRIDGE FDB (sanitized from real walk), strict-spec ARP (synthetic), strict-spec LLDP local mgmt (synthetic), **lenient-vendor Q-BRIDGE FDB (column populated, verifies no-regression)**; forwarding tests that exercise all four | regression coverage for A1+A2+A3 |
| 8 | `docs/skill: snmp profile authoring guardrail` | new `.agents/skills/project-snmp-profiles-authoring/SKILL.md` (links to profile-format.md), new "Field accessibility" section in `src/go/plugin/go.d/collector/snmp/profile-format.md` with the MAX-ACCESS rule + a `grep` audit recipe, update `AGENTS.md` Project Skills Index | H1, H4 |

Order matters: commit 1 (engine) lands before 2-4 (profiles that consume the new format). 5-6 (observability) before 7 (tests can exercise them). 8 (docs) last so the skill reflects the final shape of the engine macro and profile pattern.

Pre-commit checklist (Pre-Implementation Gate § Sensitive data plan) MUST pass before each commit and before PR open.

## Execution Log

### 2026-05-01

- Created branch `snmp-qbridge-fdb-mac-from-index` off `upstream/master`.
- Drafted initial narrow SOW; user pushed back asking for full survey across all FDB variations.
- Re-investigated topology profiles, runtime sinks, ddsnmp engine, RFC 1493/4188/4293/4363 and IEEE 802.1AB-2005 / 802.1Q-2018.
- Sent narrower SOW to 5 external reviewers (round 1: Codex, GLM, MiniMax, Qwen, Kimi). Codex independently identified A2 (IP-MIB ARP). User locked four decisions.
- Verified user's office network: 3 of 4 managed switches backstopped by BRIDGE-MIB FDB; 1 lenient. Confirmed bug-affected population is "strict + Q-BRIDGE-only + no BRIDGE-MIB".
- LibreNMS verification subagent ran; results integrated.
- Sanitized SOW and `AGENTS.local.md`: no community member names, no SNMP communities, no bearer tokens.
- Sent expanded SOW to the 5 reviewers (round 2). Findings consolidated:
  - 4 of 5 confirmed Bridge.pm citation off (242-271 → 160-178). Fixed.
  - Codex flagged Decision 2 skill path conflict with AGENTS.md (`project-*` convention). Fixed (path now `project-snmp-profiles-authoring`).
  - Codex flagged catalog count "28" wrong (actual 43 + F10 = 44). Fixed.
  - Codex flagged LibreNMS `ArpTable.php` mischaracterization (uses structured table keys, not raw index). Fixed.
  - Codex flagged LibreNMS `ipAddressTable` claim wrong (IPv4 deprecated, IPv6 modern). Fixed.
  - 4 of 5 flagged length-prefix byte edge case (F10). Added to catalog and engine macro acceptance.
  - 5 of 5 flagged pre-commit checklist as too vague. Replaced with concrete grep commands.
  - Codex + MiniMax flagged A2 phrasing — `ipNetToPhysicalPhysAddress` IS accessible. Fixed.
  - MiniMax + Codex flagged D1 description ambiguity (must say "ADD a fallback", not "fix existing lookup"). Fixed in acceptance criteria + Decision 3 + commit 5.
  - MiniMax flagged D4/D5/D6 explicit "Defer". Fixed (table now has scope column with explicit "Defer").
  - Kimi + Qwen + GLM flagged engine macro output format (lowercase hex, colon-separated, octet validation). Added to acceptance criteria.
  - Kimi flagged lenient-vendor regression fixture missing from commit 7. Added.
  - Codex flagged commit 5 file list incomplete (D1 also touches `topology_cache_fdb.go`). Fixed.
  - Codex flagged Decision 1 wording overclaim. Narrowed.
  - Codex + GLM flagged D2/E4 metric type undefined. Specified as internal counter + rate-limited log line, no chart.
  - Codex flagged vendor followup incomplete. Added EdgeSwitch, FortiSwitch, TiMOS, AOS6/7, VRP.
  - Codex flagged OpenNMS direct citation missing. Added.
  - Codex flagged sensitive-data checklist insufficient. Expanded to cover SNMPv3 creds, sysName/sysDescr/ifAlias/ifDescr, LLDP remote names/port descriptions, chassis IDs, mgmt addresses; added concrete grep one-liners.
  - GLM flagged E4 rate-limit "per-cycle, not per-row" missing from acceptance. Added.
  - GLM flagged profile-format.md MAX-ACCESS section should include audit recipe. Added.
  - GLM flagged engine `mac_address` output format parity (column-side vs index-side). Both produce `aa:bb:cc:dd:ee:ff`; verification in commit 1 unit tests.
- All 20 fixes applied. SOW v3 produced.
- Sent v3 to the same 5 reviewers (round 3). Findings consolidated:
  - 5 of 5 flagged **MAC output parity**: column-side `utils.go:54` uses `%02X` (uppercase); SOW spec requires `%02x` (lowercase). Existing tests at `collector_table_test.go:934-939` and `collector_device_meta_test.go:276-278` expect uppercase. Resolution (user pick A): converge to lowercase — flip column-side `utils.go:54` to `%02x`, update both test files, add explicit column-vs-index parity test. Folded into commit 1.
  - 4 of 5 flagged **pre-commit checklist gaps** (commit messages not in `git diff --cached`). Resolution: added `git log -1 --format='%B' | rg ...` post-commit verification, range scan via `git log upstream/master..HEAD`, and a separate PR-body check.
  - 2 of 5 (Codex, GLM) flagged **broken pre-commit IP regex** — used `(?!...)` PCRE lookahead under `grep -E` (POSIX ERE) → silently parses as a literal capture group → check passes spuriously. Resolution: switched the entire pre-commit checklist from `grep -E` to `rg -P` (ripgrep with PCRE).
  - Codex flagged **OID false positives** in the public-IP regex (SNMP OIDs like `1.3.6.1.4.1.x` look like dotted-quad IPs). Resolution: added `(?!1\.[0-3]\.6\.1\.)` exclusion and a non-digit-or-dot left-context guard so the regex only flags genuine dotted-quad addresses, not OID fragments.
  - Qwen flagged **A2 length-prefix path ambiguity** (does `index_transform` skip the byte, or does the formatter detect it?). Resolution: profile-level slicing via `index_transform` chosen — formatter stays pure (format-only). Explicit in commit 3 scope.
  - 2 of 5 (MiniMax, Codex wording) flagged **F10 detection rule should explicitly require remaining-6-octets validation**. Resolution: acceptance criterion now reads "7 components AND first equals 6 AND each remaining 6 components is a valid octet (0-255)".
  - GLM, Codex separately confirmed all 20 v2 fixes are present at the cited SOW lines.
- All round-3 fixes applied to v4. Pre-commit checklist now uses `rg -P` (POSIX-incompatible — install ripgrep first) and has 6 sub-checks (uncommitted credentials, public IPv4 with OID exclusion, public IPv6, device strings, names from local env, commit messages, PR body).
- SOW v4 ready for implementation.
- Validation pass before implementation found two plan corrections:
  - `index_transform` needs variable-tail support (`start > 0` with omitted/zero `end`) before A1/A2/A3 can be represented cleanly. User accepted option A: extend engine semantics.
  - D1 VLAN fallback must be late at observation-output time, not an eager cache mutation, so a later real `dot1qVlanCurrentTable` mapping cannot be blocked.
- Moved SOW from `pending/` to `current/`, changed status to `in-progress`, and started implementation.

### 2026-05-02

- Investigated local testing report that derived endpoints disappear briefly during some refreshes while SNMP devices and LLDP links remain visible.
- Found the registered topology cache was used as the refresh write buffer and was cleared before replacement data was ready.
- Changed refresh lifecycle to collect into an unregistered scratch cache and publish with `replaceWith()` only after full ingest/finalization.
- Added a regression test that blocks refresh mid-collection and verifies the published snapshot still exposes prior FDB and ARP-derived endpoint data.
- Addressed PR review thread `PRRT_kwDOAKPxd85_EfxH`: the IPv6 sensitive-data checklist regex contradicted its own `fc00::/7` exclusion and missed uppercase global-unicast IPv6 text. Fixed the checklist to scan `2000::/3` with `(?i)` case-insensitive matching and to exclude ULA.
- Addressed SonarCloud duplication signal by collapsing duplicated Q-BRIDGE actual-profile tests into one table-driven test while preserving normal and length-prefixed MAC-index coverage.
- User requested SOW close; marked status `completed` and moved the SOW to `done/`.
- Addressed PR review thread `PRRT_kwDOAKPxd85_Eovf`: LLDP local management addresses were temporarily formatted as `ip_address`, which would drop valid non-IP LLDP management-address subtypes before topology normalization. Added index-derived `format: hex`, switched `lldpLocManAddr` to hex preservation, and added IPv4 plus non-IP-length actual-profile coverage.
- Addressed PR review thread `PRRT_kwDOAKPxd85_Etpy`: `SOW_AUDIT_SENSITIVE_FULL_HISTORY=1` scanned fewer file types than `SOW_AUDIT_SENSITIVE_CHANGED=1`. Aligned full-history sensitive-data scanning with the changed-file code/config/documentation selector.

## Validation

Acceptance criteria evidence:

- A1 Q-BRIDGE FDB MAC extraction is implemented in the actual shipped topology profile: `_std-topology-q-bridge-mib.yaml` derives `dot1q_fdb_mac` from the row index with `format: mac_address` and no `symbol.OID` for `dot1qTpFdbAddress`.
- A2 IP-MIB ARP/ND index extraction is implemented in the actual shipped topology profile: `_std-topology-fdb-arp-mib.yaml` derives `arp_if_index` from `index: 1`, `arp_addr_type` from `index: 2` with top-level mapping, and `arp_ip` from `index_transform: [{start: 3}]` plus `format: ip_address`.
- A3 LLDP local management address extraction is implemented in the actual shipped topology profile: `_std-topology-lldp-mib.yaml` anchors `lldpLocManAddrTable` on readable `lldpLocManAddrLen` and derives subtype/address from the index.
- A3 preserves LLDP local management address bytes with `format: hex`; IP-compatible bytes are normalized by topology runtime, while non-IP subtype payloads are not dropped at collection time.
- Engine support is implemented: `format: mac_address` works for index-derived values, accepts normal 6-octet suffixes and defensive 7-component length-prefixed suffixes, validates octets, and emits lowercase colon-separated MACs. Column-side MAC formatting is also lowercase for parity.
- Runtime behavior is implemented: FDB rows with empty MAC increment a per-cycle drop counter; FDB rows with unmapped bridge ports increment a per-cycle diagnostic counter; VLAN attribution falls back to `fdbID` at observation-output time only after the mapping table has had a chance to populate.
- Guardrails are implemented: `.agents/skills/project-snmp-profiles-authoring/SKILL.md`, the AGENTS.md Project Skills Index, and `src/go/plugin/go.d/collector/snmp/profile-format.md` now document the MAX-ACCESS rule and audit recipe.

Tests or equivalent validation:

- PASS: `cd src/go && go test -count=1 ./plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition`
- PASS: `cd src/go && go test -count=1 ./plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector`
- PASS: `cd src/go && go test -count=1 ./plugin/go.d/collector/snmp_topology`
- PASS: `cd src/go && go test -count=1 ./pkg/topology/engine`
- PASS: `cd src/go && go test -count=1 ./plugin/go.d/collector/snmp`
- PASS: `git diff --check`
- Added refresh-lifecycle regression coverage: `TestCollector_RefreshKeepsPublishedSnapshotWhileCollectionRuns` proves a registered cache is not cleared while a new collection is still running.
- Added actual-profile collector tests loading `_std-topology-q-bridge-mib`, `_std-topology-fdb-arp-mib`, and `_std-topology-lldp-mib` through `ddsnmp.LoadProfileByName` and mocked strict-spec table walks. These verify the shipped YAML emits the index-derived Q-BRIDGE MAC, modern ARP IPv4/IPv6 fields, and LLDP local management address bytes, including a non-IP-length LLDP address payload.
- Added focused unit tests for variable-tail `index_transform`, index-derived MAC formatting, length-prefixed MAC suffixes, invalid MAC octets, column-vs-index MAC parity, VLAN fallback, and FDB diagnostics.

Real-use evidence:

- Not run in this session. The originally reported affected Netgear GS110TP v3 live poll still needs access to that device and a fresh SNMP walk to compare `topology:snmp` FDB endpoint count against `dot1qTpFdbPort` row count. The SOW is completed by user request with this live-device check recorded as residual PR/local-testing risk, not as claimed validation evidence.

Reviewer findings:

- Pre-implementation review findings from the three multi-agent rounds are recorded in the execution log and were folded into the implementation plan before code changes.
- No new external assistant review was run after implementation in this turn because the active repository instruction allows running external AI assistants only when the user asks for that explicitly.
- PR review thread `PRRT_kwDOAKPxd85_EfxH` was valid and fixed: the IPv6 checklist now scans global-unicast-like `2000::/3` addresses case-insensitively and no longer includes ULA `fc00::/7`.
- SonarCloud Quality Gate reported new-code duplication centered on `topology_profile_index_test.go`; the duplicated Q-BRIDGE test setup was made table-driven and revalidated with the focused `ddsnmpcollector` test package.
- PR review thread `PRRT_kwDOAKPxd85_Eovf` was valid and fixed: LLDP local management address extraction now uses index `format: hex`, preserving non-IP subtype payloads for runtime normalization instead of dropping them during profile collection.
- PR review thread `PRRT_kwDOAKPxd85_Etpy` was valid and fixed: full-history sensitive-data scanning now includes the same code/config/documentation file classes as changed-file scanning.

Same-failure scan:

- Command:
  `rg -n 'name:[[:space:]]*(dot1qTpFdbAddress|ipNetToPhysicalIfIndex|ipNetToPhysicalNetAddressType|ipNetToPhysicalNetAddress|lldpLocManAddrSubtype|lldpLocManAddr)\b|\.1\.3\.6\.1\.2\.1\.(17\.7\.1\.2\.2\.1\.1|4\.35\.1\.4\.1\.(1|3|4)|8802\.1\.1\.2\.1\.3\.8\.1\.(1|2))' src/go/plugin/go.d/config/go.d/snmp.profiles`
- Result: four expected name-only hits remain, all in the corrected profiles and all without a `symbol.OID` for the not-accessible object:
  - `_std-topology-q-bridge-mib.yaml:23` — `dot1qTpFdbAddress`, index-derived MAC.
  - `_std-topology-fdb-arp-mib.yaml:206` — `ipNetToPhysicalNetAddressType`, `index: 2`.
  - `_std-topology-fdb-arp-mib.yaml:214` — `ipNetToPhysicalNetAddress`, index-derived IP.
  - `_std-topology-lldp-mib.yaml:97` — `lldpLocManAddr`, index-derived management address.

Sensitive data gate:

- `.agents/sow/audit.sh` sensitive-data guardrail reports: scanned durable artifact files, including completed SOWs under `done/`; no sensitive-data patterns found.
- Targeted diff scan for credential assignments returned zero hits.
- Targeted device-string scan returned only synthetic test constants (`00:11:22:33:44:55`) and profile tag names, not real device identities.
- Targeted dotted-quad scan returned only TEST-NET documentation context and MAC-index numeric suffixes in tests; no raw customer IPs, SNMP communities, bearer tokens, SNMPv3 secrets, customer names, personal data, private endpoints, or proprietary incident details were added.

Artifact maintenance gate:

- AGENTS.md: updated Project Skills Index to include `.agents/skills/project-snmp-profiles-authoring/`.
- Runtime project skills: added `.agents/skills/project-snmp-profiles-authoring/SKILL.md`.
- Specs: no `.agents/sow/specs/` update. This change is an SNMP profile authoring/runtime rule and is recorded in the project skill plus `profile-format.md`; no separate durable product contract was changed.
- End-user/operator docs: updated `src/go/plugin/go.d/collector/snmp/profile-format.md` with a Field Accessibility section and audit recipe.
- End-user/operator skills: none affected. `docs/netdata-ai/skills/` and `src/ai-skills/` are not involved in SNMP profile authoring.
- SOW lifecycle: moved from `pending/` to `current/` during implementation; moved from `current/` to `done/` with status `completed` after PR creation, regression fix, review-thread fix, and explicit user request to close. Live-device validation against the originally reported affected device was not independently runnable in this workspace; this is recorded as residual runtime confirmation, not hidden as completed evidence.

Specs update:

- No spec update was needed. The durable behavior rule for future work is procedural/authoring guidance, covered by the new runtime project skill and profile-format documentation.

Project skills update:

- Added `.agents/skills/project-snmp-profiles-authoring/SKILL.md`.

End-user/operator docs update:

- Updated `src/go/plugin/go.d/collector/snmp/profile-format.md`.

End-user/operator skills update:

- No output/reference skills were affected by this SNMP collector/profile change.

Lessons:

- `index_transform` needed an explicit variable-tail semantic; otherwise standards-compliant INDEX-derived fields cannot be represented cleanly without brittle duplicate tags.
- While adding actual-profile tests, existing behavior was confirmed: mappings nested under `symbol:` are not applied to same-table column tags. This SOW does not change that broader contract; the required ARP address-type mapping is top-level, and changing the generic mapping behavior would alter existing tag outputs outside this fix.
- Topology refresh must be double-buffered. The global registry is a read surface for function calls, so registered caches must not be used as mutable write buffers during SNMP collection.

Follow-up mapping:

- Implemented in this SOW: A1, A2, A3, D1, D2, E4, F10, G2, H1, H4.
- Rejected for this SOW: changing generic same-table `symbol.mapping` behavior, because it is broader than the root-cause fix and can alter existing status tag outputs.
- Out of scope for SOW-0001 and requiring separate user-approved SOWs if prioritized later: static FDB tables, IEEE8021-Q-BRIDGE-MIB FDB, modern `ipAddressTable`, vendor proprietary FDB MIBs, D3-D6 attribution work, F2/F3/F5 vendor quirks, G3/G4 diagnostics, and H3 snmprec fixture authoring docs.

## Outcome

Completed. Implementation, focused validation, refresh-regression fix, PR creation, first review-thread fix, Sonar duplication cleanup, and SOW lifecycle close are done. The originally reported device was not directly available from this workspace for an independent live walk/count comparison, so that specific live confirmation remains a residual PR/local-testing risk rather than a claimed validation result.

## Lessons Extracted

- Future SNMP profile work must verify source MIB `MAX-ACCESS` before adding `symbol.OID` entries.
- Actual-profile collector tests are necessary here; formatter-only tests would not prove the shipped YAML emits the topology tags.

## Followup

Child SOWs to open after this one closes (one per item):

- **Static FDB tables** (B1, B2, C1, C2): coordinated SOW covering both BRIDGE-MIB and Q-BRIDGE-MIB static unicast/multicast tables. Different semantics from learned FDB (filtering policy, multi-egress, allowed-port lists). Needs design.
- **IEEE8021-Q-BRIDGE-MIB FDB** (B3, C3): modern alternative, increasingly relevant. INDEX includes `ComponentId`. Profile + selector design.
- **Modern `ipAddressTable`** (B4, C4): for L3 devices using only the new IP-MIB. Same not-accessible column pattern.
- **Vendor proprietary FDB MIBs** (C5): one SOW per vendor as real demand surfaces — Aruba/HPE (HP-ICF-BRIDGE), Huawei (HUAWEI-L2MAM-MIB / VRP `hwDynFdbPort`), Extreme (EXTREME-FDB-MIB), Nokia/Alcatel (ALCATEL-IND1-MAC-ADDRESS-MIB / TiMOS), Juniper (JUNIPER-VLAN/L2ALD-MIB), AlaxalA (AX-FDB-MIB), Ubiquiti EdgeSwitch, Fortinet FortiSwitch, Alcatel-Lucent OmniSwitch (AOS6/AOS7). LibreNMS handlers serve as references.
- **D3-D6 attribution work**: FDB truncation detection, LLDP-vs-FDB deduplication, LACP/LAG rollup, cross-protocol freshness reconciliation.
- **F2/F3/F5 vendor quirks**: Zyxel malformed-index reshape, TP-Link JetStream offset, Aruba IAP truncation.
- **G3 fail-loud, G4 per-poll stats**: engine-level diagnostic improvements beyond E4's FDB-specific coverage.
- **H3 snmprec fixture authoring docs**: how to derive sanitized fixtures from real walks.
- (Resolved in this PR — was previously listed as a follow-up.) Engine `mac_address` output format parity: column-side (`utils.go:54`) and index-side `format: mac_address` are converged to lowercase `aa:bb:cc:dd:ee:ff` in commit 1, with an explicit column-vs-index parity unit test.

## Regression - 2026-05-02

### Derived endpoints disappear during refresh windows

Observed symptom:

- Local PR testing showed that SNMP devices and LLDP links could remain visible while all derived FDB/ARP endpoints disappeared for a few seconds, then reappeared.

Root cause:

- `refreshDeviceTopology()` called `getOrCreateDeviceCache()` at the start of a refresh.
- `getOrCreateDeviceCache()` reset the registered topology cache in place: it cleared FDB, ARP, bridge-port, LLDP/CDP, interface, VLAN, and STP maps and set `lastUpdate` to zero before the SNMP walk and topology ingest had completed.
- The topology function reads from the global registry concurrently with refreshes. During that window, readers could observe the registered cache after it had been cleared but before the replacement data was ready.

Fix:

- Refresh now builds the next device snapshot in an unregistered scratch cache.
- The previously published cache remains visible to function readers during SNMP collection and ingest.
- After the scratch cache is finalized, `replaceWith()` publishes it into the registered cache under the registered cache lock.

Validation:

- Added `TestCollector_RefreshKeepsPublishedSnapshotWhileCollectionRuns`, which blocks a refresh mid-collection and verifies the published snapshot still contains the previous FDB and ARP-derived endpoint evidence.
- Re-ran the focused SOW 1 validation suite after the fix; results are recorded under `## Validation`.
