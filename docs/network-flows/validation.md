<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/network-flows/validation.md"
sidebar_label: "Validation and Data Quality"
learn_status: "Published"
learn_rel_path: "Network Flows"
keywords: ['validation', 'snmp cross-check', 'data quality', 'silent failures', 'sanity check']
endmeta-->

# Validation and Data Quality

Flow data is statistical. It can be wrong in subtle ways that the dashboard cannot detect — silent UDP drops, undocumented sampling rate changes, exporters that stopped sending. This page is the routine you should run when you set up the plugin, when something looks suspicious, and periodically thereafter.

The goal: distinguish "the data is correct" from "the data looks plausible but isn't".

## The biggest risks are silent failures

The most dangerous failures don't generate alerts. They look like data is flowing — just less of it, or skewed, or scaled wrong. Six common silent failures:

1. **UDP datagram drops** — kernel drops happen when the receive buffer fills. Plugin sees fewer datagrams than the network sent. Counters are smaller; nothing logs the drop.
2. **Sampling rate misinterpretation** — exporter samples 1-in-1000, no one documented it. Bytes look 1000× smaller than reality.
3. **Sampling rate change** — someone reconfigures a router. Trends show a phantom 10× spike. No alert fires.
4. **Wrong interfaces being exported** — flow export was enabled on three of five interfaces. Some traffic is invisible.
5. **Template loss after collector restart** — v9 / IPFIX records arrive but cannot be decoded until the next template arrives. Counts dip silently.
6. **Stale GeoIP / ASN database** — country and AS-name fields drift away from reality over weeks.

For each, the system appears to be working. The only way to detect them is cross-validation against an independent source.

## The minimum viable validation routine

Run this once after deployment, then quarterly, plus whenever something looks off.

### 1. SNMP cross-check (every 5 minutes if you have an SNMP collector handy)

Compare flow-derived bandwidth on a specific interface to the SNMP `ifInOctets` / `ifOutOctets` counter for that same interface. They should be close.

The flow-derived bandwidth: filter the dashboard to one exporter, one input interface (or one output interface — pick a direction), and read the bytes/s rate.

The SNMP-derived bandwidth: from your SNMP monitoring (Netdata's snmp.d, your separate SNMP system, or your network team).

**Acceptable difference: roughly 5-15%.** SNMP includes layer-2 traffic (ARP, STP, LLDP, routing protocols, interface-level multicast) that flow data filters out. Expect SNMP slightly higher.

**Not acceptable: more than 30% gap.** That indicates one of:

- UDP drops (kernel-level). Run `sudo ss -uam sport = :2055` and check the `dRcv` column.
- Sampling rate not honoured. The exporter is sampling but not communicating the rate to the plugin (NetFlow v7, NetFlow v5 with rate=0, v9 / IPFIX without the Sampling Options Template).
- Wrong interfaces being exported. Cross-check `show flow exporter` (or vendor equivalent) against your expectations.
- Template loss. Watch `netflow.input_packets > template_errors` on the plugin health charts.

**Plugin reporting wildly more than SNMP** indicates the doubling effect (see below).

### 2. Doubling sanity check

If your dashboard's total bandwidth exceeds the **physical link capacity**, you're double-counting. Standard NetFlow / IPFIX configuration produces two flow records per packet (one ingress, one egress). With multiple monitored routers on the same path, even more.

Verify by: filter to one exporter and one interface in one direction (input OR output, not both). Compare to SNMP for that same interface. They should agree within 5-15%. The difference between "all flows summed" and "filtered to one direction" is exactly the doubling factor.

### 3. Sampling rate sanity check

For each exporter, document:

- Does it sample? At what rate?
- Does it carry the rate in flow records (NetFlow v9 / IPFIX) or in the header (v5)?
- For NetFlow v9 / IPFIX, does the exporter send a Sampling Options Template? At what frequency?

If the exporter samples and the plugin doesn't see the rate, bytes are undercounted.

To verify the plugin sees the rate: query a known flow on the dashboard and look at `RAW_BYTES` and `BYTES`. If they differ, the plugin is multiplying — sampling rate is being honoured. If they're identical, the plugin sees rate 1 (no scaling).

### 4. Per-exporter health check

The plugin doesn't publish per-exporter ingest counters today. To verify each exporter is sending:

- Filter the dashboard to one exporter at a time. Check the byte rate. A healthy edge router during business hours should show non-zero traffic.
- An exporter that abruptly drops to zero is offline (silently). The plugin won't tell you — your monitoring practice has to.

### 5. Template cache health (NetFlow v9 / IPFIX)

On the plugin health chart `netflow.input_packets`, watch `template_errors`. In steady state, it should be near zero. A sustained non-zero rate means data records are arriving before their templates — usually because the exporter sends templates rarely (every 30 minutes is common Cisco default) and the plugin's template cache was wiped (restart with no persistence, or first-time setup).

The plugin persists template state across restarts to `decoder_state_dir`, so a routine restart shouldn't cause this. If it does, check the cache directory permissions.

### 6. GeoIP / ASN database freshness

The plugin doesn't publish a "MMDB last loaded" signal. To verify your databases aren't stale:

```bash
ls -la /var/cache/netdata/topology-ip-intel/ /usr/share/netdata/topology-ip-intel/
```

Files older than ~60 days are likely stale. Refresh:

```bash
sudo /usr/sbin/topology-ip-intel-downloader
```

The plugin polls the files every 30 seconds — a successful refresh picks up automatically without restart.

### 7. Internal IP enrichment validation

Before relying on geographic analysis, spot-check that internal IPs are properly handled. Filter to an internal source IP you know and look at the `SRC_COUNTRY` and `SRC_AS_NAME` fields:

- Empty / "AS0 Private IP Address Space" — correct.
- Some random country — your GeoIP database is returning data for private space. Declare the range under `enrichment.networks` (see [Static metadata](/docs/network-flows/enrichment/static-metadata.md)).

## Quick reference: what to monitor and what alerts to consider

| Signal | Where | What to alert on |
|---|---|---|
| `udp_received` rate dropped | `netflow.input_packets` chart | Sustained 0 during business hours |
| `template_errors` rising | `netflow.input_packets` chart | Sustained > 1% of `udp_received` |
| `parse_errors` rising | `netflow.input_packets` chart | Sustained > 5% of `udp_received` |
| Memory growing (`unaccounted`) | `netflow.memory_accounted_bytes` | RSS grows linearly without ingest growth |
| `decoder_scopes` unbounded growth | `netflow.decoder_scopes` chart | Monotonic growth over hours |
| Disk full warnings | `netflow.raw_journal_ops` `write_errors` | Any non-zero |
| SNMP-flow gap | external | More than 30% on a steady-state link |
| Sampling rate change | router config diff (yours) | Any change to active timeout or sampling |

## When to file a "data is wrong" investigation

Start an investigation when **two independent signals disagree**:

- SNMP says 500 Mbps; flow data says 50 Mbps. Investigate sampling, drops, exporter coverage.
- Flow data shows traffic to a country; threat intelligence says that country's ASN doesn't host known infrastructure. Investigate GeoIP or anycast.
- Last week's top talker disappeared this week. Investigate exporter health, routing changes, business-side changes.

For each, read the [Anti-patterns](/docs/network-flows/anti-patterns.md) page first — most "data is wrong" reports are actually expected behaviour misread.

## What's next

- [Plugin Health Charts](/docs/network-flows/visualization/dashboard-cards.md) — The charts referenced above.
- [Anti-patterns](/docs/network-flows/anti-patterns.md) — Misreadings to rule out before declaring a bug.
- [Investigation Playbooks](/docs/network-flows/investigation-playbooks.md) — Concrete recipes for common questions.
- [Troubleshooting](/docs/network-flows/troubleshooting.md) — Recovery for the symptoms above.
