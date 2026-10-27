# Windows network collector: parity and correctness review

- **Status:** Approved implementation is committed locally on `win_net` in `c6dd6fddbe` and `eee1fd26e3`. Windows CI found a header-guard compile failure on 2026-10-27; the fix is local and a CI rerun is pending. Windows runtime validation remains unavailable.
- **Tracking:** `.agents/sow/q/current/SOW-20261026-windows-network-parity.md` records the approved design, review, and validation. The local SOW is not committed.
- **Comparison source:** `prometheus-community/windows_exporter @ 7b55600d6eac6cf56367afe0af92c652feaaa71c`, `internal/collector/net/{net.go,types.go,AGENTS.md}`.
- **Evidence limit:** Code and static checks establish the implementation. Physical and virtual provider availability, Perflib-to-adapter matching, operational-state transitions, and collection cost still need observation on Windows.

## Outcome

The collector now charts `Packets Received Unknown` per interface as `net.unknown_protocol_packets` (`inbound`, packets/s). It also registers a Windows `network-interfaces` Function for charted physical and virtual interfaces. The Function shows available traffic, packet, drop, speed, MTU, operational state, and friendly-name values. It requires signed-in, same-space access because a friendly name can contain user-entered text.

The Function does not create a historical operational-state chart. MAC and IP address inventory are intentionally omitted. NIC include/exclude filtering is separate work; no new filter option was added. Existing chart IDs, contexts, dimensions, and units remain unchanged.

## Windows Exporter comparison

| Exporter metric or feature | Current Netdata presentation | Evidence |
| --- | --- | --- |
| Received/sent bytes and packets | Existing `net.net` and `net.packets` rate charts; Function also shows latest valid rates | `perflib-network.c` interface counter paths and `network_function_add_rows()` |
| Discards and errors | Existing `net.drops` and `net.errors` rate charts | `perflib-network.c` interface counter paths |
| Output queue length | Existing `net.queue_length` chart and `network_interface_output_queue_length` alert | `perflib-network.c`; `src/health/health.d/net.conf` |
| Current bandwidth | Existing `net.speed` chart; Function shows latest valid speed | `perflib-network.c` speed path |
| Received/sent totals | Derivable from directional values; no redundant chart added | Exporter sample in its `AGENTS.md` |
| `packets_received_unknown_total` | New `net.unknown_protocol_packets` chart | `perflib-network.c:602,858-883`; `metadata.yaml:1071` |
| `nic_operation_status` | Live Function `OpState`; no state chart or state history | `windows_api.c:62-80`; `perflib-network.c:1186,1263-1295` |
| `nic_info.friendly_name` | Live Function `FriendlyName` when adapter matching is unique | `windows_api.c:124-135`; `perflib-network.c:1130-1142,1198` |
| `nic_info.mac`, `nic_address_info` | Omitted by approved scope; no MAC/IP inventory | SOW decisions 1A and 3C |
| NIC include/exclude filtering | No Windows option; independent follow-up | SOW decision 6A |

Netdata also retains its existing RSC and Chimney charts and protocol charts. Those are not gaps in this Exporter comparison. Unicast and non-unicast packet splits, `net.mtu`, and `net.carrier` charts remain separate opportunities; they were not part of the approved target.

## Implemented collector behavior

- `Network Interface` and `Network Adapter` are requested as separate Perflib objects. The second query follows every consumer of the first block because the Perflib helper reuses its thread-local buffer (`perflib-network.c:1425-1498`; `windows-perflib/perflib.c:958-973`). This makes virtual interface collection independent of incidental object co-delivery.
- Zero cumulative traffic no longer prevents packet, speed, error, drop, queue, or offload collection. An incomplete instance walk no longer prunes valid interface charts; unreadable names are skipped instead of merging into `[unknown]` (`perflib-network.c:673-694,1073-1086`).
- The new unknown-protocol counter is an independent incremental chart, because it is semantically distinct from discards and errors (`perflib-network.c:858-883`).
- The speed chart priority no longer collides with the Chimney chart, and the metadata `device` label describes the Perflib instance name (`perflib-network.c:779`; `metadata.yaml`).
- The Function reads an on-demand `GetAdaptersAddresses` snapshot. The shared Windows API helper retries up to three times, bounds the new snapshot at 16 MiB and 4096 adapters, converts description and friendly name from UTF-16, and maps the seven `IF_OPER_STATUS` values (`windows_api.c:14-140`). The existing default-network probe shares the retry helper.
- Description normalization follows Windows Exporter: `( ) # / \\` become `[ ] _ _ _`. A non-unique normalized description yields unavailable state and friendly name, rather than attributing an adapter to the wrong chart (`perflib-network.c:1106-1142`).
- The Function suppresses stale rates when a Perflib poll fails, an instance is absent, a counter is missing, or a snapshot ages out. It omits severity when drop values are unavailable (`perflib-network.c:1153-1203,1283-1295,1427-1443`). Missing physical carrier state, duplex, and per-interface multicast values remain empty.

## Approved design decisions

1. **Scope — 1A, clarified:** Deliver the selected network improvements without MAC/IP inventory.
2. **Operational state — 2B:** Expose current `OpState` in the Function only. There is no `net.operstate` chart, history, or chart-based state alert.
3. **Identity and addresses — 3C:** Use one per-interface Function; omit MAC and per-address rows. A later access decision selected FriendlyName in that Function.
4. **Coverage — 4A:** Include charted physical and virtual Perflib instances. Adapters without a charted Perflib instance are not rows.
5. **Unknown-protocol packets — 5A:** Add `net.unknown_protocol_packets` with the `inbound` dimension, in packets/s.
6. **NIC filtering — 6A:** Keep filtering separate from this implementation; it still needs separate tracking.
7. **Function access — D7A:** Include FriendlyName and require `HTTP_ACCESS_SIGNED_ID | HTTP_ACCESS_SAME_SPACE`. This differs from the anonymous Linux interface Function by approved design.

The Linux `network-interfaces` Function provided the table and chart-value precedent. The Linux `net.operstate` chart was evaluated but deliberately not added on Windows. The earlier concern that a missing traffic chart would crash the speed variable update was rejected: `rrdvar_chart_variable_set()` null-guards both arguments (`src/health/rrdvar.c:104-105`).

## Validation and remaining limits

- The independent review found stale Function rates after failed polls, false `normal` severity with missing drops, and reliance on incidental `Network Adapter` co-delivery. The follow-up commit fixes all three; a second review found no remaining definite blocker.
- `python3 -m unittest integrations.tests.test_collector_metadata integrations.tests.test_health_alerts integrations.tests.test_descriptions`: **77 passed**.
- `python3 integrations/gen_docs_integrations.py --check`: **passed**, 1639 integration pages.
- `clang-format --dry-run --Werror` on the changed C/header files and `git diff --check`: **passed**.
- Windows CI compilation failed in `windows_api.c` because its header tested `OS_WINDOWS` before loading the project platform definitions, leaving the new interface struct undefined. A focused C compile check reproduced the failure; adding `libnetdata-platform-fwd.h` to the header makes that check pass. This checkout still has no Windows SDK/MinGW headers or Windows host, so the full Windows build and executable regression tests await CI. Source-level checks cannot establish runtime adapter identity matching, provider availability, state transitions, or the extra per-cycle `Network Adapter` query cost.

On Windows, verify active, idle, disconnected, disabled, physical, and Hyper-V virtual adapters. Compare the two Perflib instance lists with `GetAdaptersAddresses` and `Get-NetAdapter`; test duplicate descriptions, counter gaps, adapter removal/re-add, and Function access. Measure the additional Perflib query at the one-second default collection interval.

## Separate work

- **NIC filtering:** No open Windows network filtering issue or PR was found in the available GitHub read-only search. A GitHub issue link or explicit approval of private local tracking is still needed under the repository follow-up rule.
- **Other opportunities:** MAC/IP inventory, a historical state chart, carrier/MTU charts, and unicast/non-unicast packet splits require their own scope and design choices if pursued.
