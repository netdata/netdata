<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/network-flows/validation.md"
sidebar_label: "Validation and Data Quality"
learn_status: "Published"
learn_rel_path: "Network Flows"
keywords: ['validation', 'snmp cross-check', 'data quality', 'sanity check']
endmeta-->

<!-- markdownlint-disable-file -->

# Validation and Data Quality

The plugin handles per-flow sampling-rate multiplication, template persistence, and database refresh internally — these are not concerns you have to monitor. What's left to validate is whether the data you see corresponds to the traffic that actually crossed your network. This page is the routine to confirm that.

The goal: distinguish "the data is correct" from "the data looks plausible but isn't".

## What you actually need to watch

A small number of failure modes need active monitoring. Most are signalled by Netdata's existing alerts; the rest you check periodically.

| Failure mode | Detection | Notes |
|---|---|---|
| **Kernel-level UDP receive-buffer drops** | The system alert `1m_ipv4_udp_receive_buffer_errors` fires when the `RcvbufErrors` rate averages more than **10 errors/second** over a 1-minute window. The dimension is incremental in Netdata, so the numeric threshold is per-second, not per-minute. | OS-wide signal. Ships as `to: silent` by default -- change the `to:` field in your alert config to receive notifications. Tune `net.core.rmem_max` (see [Troubleshooting](/docs/network-flows/troubleshooting.md)). |
| **An exporter stopped sending** | Filter the dashboard to that exporter; rate dropped to zero. Per-exporter ingest counters are not published. | Manual periodic spot-check or external monitoring. |
| **Wrong set of interfaces being exported** | Cross-check `show flow exporter` (or vendor equivalent) on each router against the interfaces you intended. | Configuration drift over time; audit quarterly. |
| **An exporter is sampling but not communicating the rate** | The plugin treats those records as unsampled, so volumes are undercounted by the sampling factor. Cross-check against SNMP. | NetFlow v7 has no rate field; v5 sometimes sends `rate=0`; v9 / IPFIX without a Sampling Options Template lose the per-flow rate. See "Sampling rate verification" below. |
| **Stale GeoIP / ASN database** | No in-process signal. Check file mtimes; refresh on the schedule the provider recommends. | DB-IP and GeoLite2 ship monthly. |

The plugin handles sampling-rate changes per flow record, and it persists decoder templates under `journal_dir/decoder-state.d/` so they reload automatically across restarts. You still need external validation for packet drops, exporter drift, stale enrichment data, and exporters that do not report their sampling rate.

## The minimum viable validation routine

Run this once after deployment, then quarterly, plus whenever something looks off.

### 1. SNMP cross-check

Compare flow-derived bandwidth on a specific interface to the SNMP `ifInOctets` / `ifOutOctets` counter for that same interface. They should be close.

The flow-derived bandwidth: filter the dashboard to one exporter and one interface (Ingress Interface Name OR Egress Interface Name — pick one), and read the bytes/s rate.

The SNMP-derived bandwidth: from your SNMP monitoring (Netdata's `snmp.d`, your separate SNMP system, or your network team).

**Acceptable difference: roughly 5-15%.** SNMP includes layer-2 traffic (ARP, STP, LLDP, routing protocols, interface-level multicast) that flow data filters out. Expect SNMP slightly higher.

**Not acceptable: more than 30% gap.** That indicates one of:

- UDP drops at the kernel. Check the alert mentioned above, or run `sudo ss -uamn sport = :2055` and inspect the `d<N>` value inside the `skmem:(...)` line (the `sock_drop` counter increments on every dropped datagram). The `-n` flag keeps the port numeric in the output.
- Sampling rate not honoured (the exporter is sampling but not communicating the rate). See "Sampling rate verification" below.
- Wrong interfaces being exported.

**Plugin reporting wildly more than SNMP** indicates the doubling effect — see below.

### 2. Doubling sanity check

If your dashboard's total bandwidth exceeds the **physical link capacity**, you're double-counting. When both ingress and egress flow exporters are enabled on the same router — a common but not universal configuration; vendor best practice is ingress-only — each packet is recorded twice. With multiple monitored routers on the same path, even more.

Verify by: filter to one exporter and one interface (Ingress Interface Name OR Egress Interface Name, pick one). Each packet then appears in exactly one record on that interface. Compare to SNMP for that same interface; they should agree within 5-15%.

### 3. Sampling rate verification

The plugin multiplies bytes and packets by the sampling rate each flow carries — **per flow, at ingestion**. You don't have to keep rates uniform across exporters; mixed rates are scaled correctly.

What you DO need to verify, once per exporter, is that the plugin is actually seeing the rate. The dashboard does not surface the per-flow `SAMPLING_RATE` field as a filter, group-by, or facet, so the verification is by **magnitude**, not by reading the rate directly:

- After the plugin has data, compare the dashboard's bytes/s on a known interface to that interface's SNMP `ifInOctets` / `ifOutOctets` rate. If the dashboard reading is roughly the SNMP figure divided by the configured sampling rate (e.g. ~1/1000 of SNMP at 1-in-1000), the plugin saw the rate as `1` and is *not* multiplying. If the dashboard agrees with SNMP within 5-15%, the plugin is honouring the rate.
- The "sampling but not telling" case happens with NetFlow v7 (no rate field), v5 with rate=0, and v9 / IPFIX exporters that don't send a Sampling Options Template. Fix on the exporter side, or apply `enrichment.override_sampling_rate` per source prefix to substitute a known rate (see [Configuration → enrichment](/docs/network-flows/configuration.md#enrichment) and the [Static Metadata integration card](/src/crates/netflow-plugin/integrations/static_metadata.md)).

### 4. Per-exporter health check

Per-exporter ingest counters are not published. To verify each exporter is sending:

- Filter the dashboard to one exporter at a time. A healthy edge router during business hours should show non-zero traffic.
- An exporter that abruptly drops to zero is offline. The plugin won't tell you — your monitoring practice has to.

### 5. GeoIP / ASN database freshness

The plugin doesn't publish a "MMDB last loaded" signal. To verify your databases aren't stale:

```bash
ls -la /var/cache/netdata/topology-ip-intel/ /usr/share/netdata/topology-ip-intel/
```

Files older than ~60 days are likely stale. Refresh:

```bash
sudo /usr/sbin/topology-ip-intel-downloader
```

Packaged 32-bit installs do not include `topology-ip-intel-downloader`; use the packaged stock MMDB payload there, or refresh the cache from a system that includes the downloader.

The plugin polls the files every 30 seconds — a successful refresh picks up automatically without restart.

To cross-check the on-disk size of each tier:

```bash
sudo du -sh /var/cache/netdata/flows/{raw,1m,5m,1h}
```

The raw tier dominates. If raw-tier disk usage is approaching the `size_of_journal_files` you set, plan retention vs. capacity (see [Sizing and Capacity Planning](/docs/network-flows/sizing-capacity.md)).

## Plugin-side signals worth alerting on

These are charts the plugin already exposes. Tune the alert thresholds to your environment.

| Signal | Where | Suggested alert |
|---|---|---|
| `udp_received` rate dropped | `netflow.input_packets` | Sustained 0 during business hours indicates the listener is up but no exporter is sending. |
| `parse_errors` rising | `netflow.input_packets` | Sustained > 5% of `udp_received` indicates the wire is producing malformed datagrams. |
| `template_errors` rising | `netflow.input_packets` | Sustained > 1% of `udp_received` after the warm-up period indicates an exporter sends data records before templates. |
| Memory growing (`unaccounted`) | `netflow.memory_accounted_bytes` | RSS climbs linearly without ingest growth -- possible leak. |
| `decoder_scopes` unbounded growth | `netflow.decoder_scopes` | An exporter is rotating template IDs; investigate per-router behaviour. |
| Disk write errors | `netflow.raw_journal_ops` `write_errors` | Any non-zero indicates filesystem trouble. |
| SNMP-flow gap | external | More than 30% on a steady-state link triggers the validation routine above. |

## When to file a "data is wrong" investigation

Start an investigation when **two independent signals disagree**:

- SNMP says 500 Mbps; flow data says 50 Mbps. Investigate sampling, kernel drops, exporter coverage.
- Flow data shows a destination ASN you don't expect; threat-intelligence or DNS resolution disagrees. Investigate ASN MMDB staleness or anycast.
- Last week's top talker disappeared this week. Investigate exporter health, routing changes, business-side changes.

For each, read the [Anti-patterns](/docs/network-flows/anti-patterns.md) page first — most "data is wrong" reports are expected behaviour misread.

## What's next

- [Plugin Health Charts](/docs/network-flows/visualization/dashboard-cards.md) — The charts referenced above.
- [Anti-patterns](/docs/network-flows/anti-patterns.md) — Misreadings to rule out before declaring a bug.
- [Investigation Playbooks](/docs/network-flows/investigation-playbooks.md) — Concrete recipes for common questions.
- [Troubleshooting](/docs/network-flows/troubleshooting.md) — Recovery for the symptoms above.
