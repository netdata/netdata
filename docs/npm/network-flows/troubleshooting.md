<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/npm/network-flows/troubleshooting.md"
sidebar_label: "Troubleshooting"
learn_status: "Published"
learn_rel_path: "Network Flows"
keywords: ['troubleshooting', 'debugging', 'plugin health', 'failures']
endmeta-->

<!-- markdownlint-disable-file -->

# Troubleshooting

Concrete recipes for the most common failures, organised by symptom. Most issues are diagnosable from the [plugin health charts](/docs/npm/network-flows/visualization/dashboard-cards.md), the Netdata journal logs, and a couple of OS-level commands.

## The plugin doesn't start

The plugin won't come up at all, or starts and immediately exits.

**Symptoms:**
- Netdata reports `netflow-plugin` as not running, or restart-looping.
- Nothing in the Network Flows view.
- An error in `journalctl --namespace netdata`.

**Likely causes:**

| Cause | What to check |
|---|---|
| YAML typo or unknown key | `journalctl --namespace netdata --since "5 minutes ago" \| grep -E 'failed to load configuration\|netflow'`. The plugin uses strict YAML — any unknown key fails parsing. |
| Required GeoIP DB missing (`optional: false`) | Same log search. Look for `failed to load database`. Either fix the path or set `optional: true`. |
| Listen address conflict | Look for `failed to bind`. Another process is on one of the configured ports (stock defaults: `2055` and `6343`). |
| Validation error | Look for `must be greater than 0` and similar. The plugin validates the full config at startup. |
| `enabled: false` was set | Look for `netflow plugin disabled by config`. The plugin honours this and shuts down cleanly — looks like "not running" if you don't read the log. |

**Recovery:**

```bash
# Read the failure
sudo journalctl --namespace netdata --since "5 minutes ago" | grep -E 'netflow|failed to|error'

# Validate the YAML (use an online linter or `yamllint`)
yamllint /etc/netdata/netflow.yaml

# After fixing, restart
sudo systemctl restart netdata
```

## The plugin starts, but no flows appear

The plugin is running, but the Network Flows view is empty.

**First check:** is anything reaching the plugin?

```bash
sudo tcpdump -i any -nn -c 50 'udp port 2055 or udp port 6343'
```

- **No packets in 30 seconds** — exporter not sending, or firewall blocking. Check the exporter's status (`show flow exporter` on Cisco, equivalents elsewhere) and the network path. The plugin can't help here; the data isn't reaching it.
- **Packets arriving** — keep going.

**Second check:** is the listener bound?

```bash
sudo ss -unlp | grep -E ':(2055|6343)([[:space:]]|$)|netflow'
```

If nothing matches, the plugin isn't listening. See "doesn't start" above.

**Third check:** what do the plugin's own counters say?

Open `netflow.input_packets` on the standard Netdata charts page. The dimensions tell the story:

- `udp_received > 0`, `parsed_packets == 0` — datagrams arriving, none decoding successfully. Wrong protocol on the listener, or all datagrams malformed.
- `udp_received > 0`, `parsed_packets > 0`, but no per-protocol counter (`netflow_v9`, `ipfix`, etc.) is moving — the protocol you're sending may be disabled in the plugin config. Check `protocols.v9`, `protocols.ipfix`, etc. in `netflow.yaml`.
- `parse_errors` rising in lockstep with `udp_received` — datagrams aren't valid for the protocols the plugin supports. Capture a small UDP sample with `tcpdump -w` and inspect it with Wireshark.

### sFlow packets arrive, but no sFlow rows appear

sFlow datagrams can carry flow samples, counter samples, or both. Netdata's Network Flows view creates flow rows from sFlow flow samples only. Counter-only sFlow traffic is valid sFlow traffic, and `netflow.input_packets` can show `sflow` increasing, but there are no source/destination flow rows to display.

Capture a small sample and check the sFlow sample types:

```bash
sudo tcpdump -i any -s 0 -w /tmp/sflow-check.cap -c 50 'udp port 6343 or udp port 2055'
tshark -r /tmp/sflow-check.cap \
  -d udp.port==6343,sflow -d udp.port==2055,sflow \
  -T fields -e sflow_245.sampletype -e sflow_245.flow_record_format | sort | uniq -c
```

- Sample type `1` (`flow_sample`) and `3` (`expanded_flow_sample`) can produce Network Flow rows.
- Sample type `2` (`counters_sample`) and `4` (`expanded_counters_sample`) do not contain endpoint-level flow records.
- If you only see sample types `2` or `4`, configure the exporter or generator to send flow samples, or both flow and counter samples. For MIMIC, select `Flow` or `All`/`Both` samples and make sure the loaded SFLOW configuration file actually contains flow sample blocks.

## Partial data — some flows are dropped

Counters show received traffic but you suspect data loss.

**Template errors (NetFlow v9, IPFIX):**

```bash
# Watch the template_errors dimension
# In the dashboard: netflow.input_packets > template_errors
```

If it's climbing, the exporter is sending data records before their templates. Either:

- The exporter restarted and the plugin's template cache is stale. Wait for the exporter to send the next template (the cadence depends on the exporter's `template-refresh` configuration — vendor defaults vary widely), or restart the exporter to force an immediate template refresh.
- Templates are sent rarely. Cisco IOS / IOS-XE Flexible NetFlow ships a default `template data timeout` of **600 seconds (10 minutes)**; Juniper and others have their own defaults, often longer. After a plugin restart, you'll see template errors until the next template re-send. **Fix on the router side**: lower the template refresh interval to 60 seconds (the [Quick Start](/docs/npm/network-flows/quick-start.md) configurations show this).
- The exporter is using template IDs that collide with another exporter's templates. Most common cause: two exporters NATted behind the same public IP. Place the plugin inside the NAT boundary or give each exporter a distinct address.

### Cisco ASA NSEL packets arrive, but fewer traffic rows appear

Cisco ASA Network Secure Event Logging is detected automatically from its NetFlow v9 templates; there is no NSEL configuration switch. Netdata requires a validated event field (233, or legacy 40005) together with Cisco extended-event field 33002 before applying NSEL semantics.

NSEL exporter records and stored traffic rows are intentionally not one-to-one:

- Event 5 updates contain interval traffic and are the only events stored in the flow database.
- Event 1 create, event 2 teardown, and event 3 deny records are counted but not stored as traffic. Teardown contains lifetime totals that repeat earlier updates; storing it would double-count and put old traffic in the close-time bucket.
- One update can create two rows: initiator traffic in the reported direction and nonzero responder traffic with endpoints swapped.
- An all-zero initiator direction remains visible. An all-zero responder direction is suppressed and diagnosed. A direction with only bytes or only packets is stored with zero for the missing member and diagnosed as partial.

The Network Flows function response exposes cumulative `decoded_nsel_*` statistics for received event types, malformed/counterless/partial records, suppressed zero responders, and emitted forward/reverse rows. These statistics are not yet dimensions of `netflow.input_packets`; the health-chart layout is tracked separately.

If `template_errors` rises, check the v9 template refresh cadence. On first startup, Netdata cannot decode data received before the first template. Learned templates are persisted for later restarts, but v9 streams are separated by exporter IP, UDP source port, and Source ID; a new source port needs its own template.

**UDP kernel drops:**

The plugin doesn't count these. Check at the OS level:

```bash
sudo ss -uamn sport = :2055        # inspect the d<N> field inside skmem:(...)
sudo ss -uamn sport = :6343        # repeat for the stock sFlow listener
grep ^Udp: /proc/net/snmp          # RcvbufErrors counter (system-wide)
```

`/proc/net/udp` lists open sockets and includes per-socket `drops`; the kernel-wide UDP `RcvbufErrors` total lives under the `Udp:` line of `/proc/net/snmp` (this is what Netdata's own `ipv4.udperrors` chart and the `1m_ipv4_udp_receive_buffer_errors` alert read).

If drops are occurring, the kernel UDP receive buffer is too small for the burst rate. The plugin requests a 64 MiB receive buffer at startup, but the kernel silently caps unprivileged requests at `net.core.rmem_max` — and distribution defaults are tiny (~208 KiB, a few tens of datagrams). Raise the cap:

```bash
sudo sysctl -w net.core.rmem_max=67108864
sudo sysctl -w net.core.netdev_max_backlog=250000
```

Persist in `/etc/sysctl.d/99-netflow.conf` and restart the plugin (it logs the effective buffer size at startup). `net.core.rmem_default` does not matter for the plugin's socket — only `rmem_max` does.

**Per-protocol switch off:**

```yaml
protocols:
  v5: false      # are you accidentally rejecting v5 datagrams?
```

## Data is wrong — numbers don't match expectations

**Volume looks doubled:**

This is the most common report. When a router is configured to export both ingress and egress on each monitored interface — a common configuration; vendor best practice is ingress-only — every packet generates an ingress record AND an egress record, so traffic appears 2× on a single such router. With two routers on the same path doing the same thing, 4×. Filter to one exporter and one interface (`Ingress Interface Name` OR `Egress Interface Name`, pick one) to see real volume. See [Anti-patterns](/docs/npm/network-flows/anti-patterns.md).

**Bandwidth doesn't match SNMP:**

Several legitimate causes:

- **Doubling**, as above. Filter to one exporter and one interface before comparing.
- **Comparing aggregates to a single interface counter.** SNMP `ifInOctets` / `ifOutOctets` is per-interface; an unfiltered flow aggregate sums many interfaces. Compare like-with-like by filtering the dashboard to the same exporter and the same interface (Input or Output, pick one).
- **Sampling rate not honoured by the exporter.** The plugin multiplies each flow's bytes/packets by that flow's own sampling rate. If the exporter doesn't carry the rate (NetFlow v7 has no field for it; v5 sometimes sends 0 instead of the actual rate; v9 / IPFIX without the Sampling Options Template), the plugin treats those records as unsampled and undercounts.
- **SNMP includes layer-2 traffic** (ARP, STP, LLDP, routing protocols) that flow data filters out. Expect SNMP to be 5-15% higher than flow on a healthy collector. More than that, investigate.

See [Validation and Data Quality](/docs/npm/network-flows/validation.md).

**AS resolution chain misbehaving:**

If `SRC_AS` / `DST_AS` are zero everywhere despite the exporter sending them, check the `asn_providers` chain:

- `[geoip, ...]` — `geoip` is a terminal short-circuit. The chain stops at `geoip` (it returns 0). Reorder: `[flow, routing, geoip]`.
- `[]` (empty) — no validation rejects this. Every AS is forced to 0.

See [Enrichment](/docs/npm/network-flows/enrichment.md) (the asn_providers chain section).

**Decapsulation eating non-tunnel traffic:**

If you've enabled `decapsulation_mode: vxlan` and traffic that isn't VXLAN suddenly disappears from the L2-section path, that's by design — the decap is destructive on non-matching traffic. Standard NetFlow / IPFIX records (no IE 104 / IE 315) are unaffected.

## Performance issues

**High CPU:**

```bash
top -p $(pgrep -f netflow-plugin)
```

If `netflow-plugin` is using a lot of CPU:

- Check `netflow.input_packets` — high `udp_received` rate? You're at the limit of what one core can do for the post-decode hot path. Each instance is single-process; you can't scale horizontally on one host.
- If `udp_received` is moderate but CPU is high, classifier rules with complex regex might be the cause. Check `enrichment.classifier_cache_duration` — if too short, classifiers re-evaluate too often.
- Investigate with `perf top` or similar to find the hot function.

See [Sizing and Capacity Planning](/docs/npm/network-flows/sizing-capacity.md) for measured throughput limits on this hardware class.

**Memory growth:**

```bash
# Watch default lightweight state-cardinality charts over time
# netflow.facet_values
# netflow.tier_index_entries
# netflow.open_tiers
```

- If `netflow.facet_values` climbs, facet vocabulary is growing.
- If `netflow.tier_index_entries` or `netflow.open_tiers` climbs, ingest is outpacing tier flushes. Check `netflow.materialized_tier_ops` for `flushes` rate and `*_errors`.
- If you need byte-level attribution, enable `charts.memory_diagnostics` in `netflow.yaml` and inspect `netflow.memory_resident_bytes` and `netflow.memory_accounted_bytes`.
- If `netflow.decoder_scopes` is growing without bound, your exporter is rotating template IDs. Investigate per-router behaviour.

**Tier commit worker health:**

Rollup tiers (1m / 5m / 1h) are committed to disk by dedicated worker threads so the receive path never blocks on tier disk I/O. Four charts expose their health:

- `netflow.tier_commit_age` — seconds since each tier's worker last completed a claim cycle. This tracks worker liveness, not tier traffic: it stays low even on an idle tier. A steadily climbing age means the worker is stuck (most likely blocked on disk I/O) — check disk latency on the journal volume.
- `netflow.tier_commit_duration` — how long the last commit batch took, fsync included. Sustained growth means the disk is falling behind the rollup volume.
- `netflow.tier_commit_batches` — commit batches per second; each tier normally commits once per its bucket interval.
- `netflow.tier_commit_stretched` — commit windows that carried more than one closed bucket because the worker missed an anniversary. Occasional events after a restart (catch-up) are normal; a steady rate means the disk cannot keep up with the rollup cadence. Data is not lost — windows stretch (a 1m rollup may become a 70-80s rollup) — but investigate disk throughput.

Note: the live (open) tier rows shown by queries refresh on a 1-second cadence, so the current minute's in-progress rollup can lag the raw tier by up to one second.

**Disk fill:**

```bash
sudo du -sh /var/cache/netdata/flows/*
```

Default retention is `10GB` per tier with no time-based age limit. The default is applied separately to raw, 1m, 5m, and 1h tiers, so total can reach roughly 40 GB plus some. If your config left this default and your collector is busy, expect to hit the size cap quickly; quiet collectors may keep data for longer than seven days. See [Configuration](/docs/npm/network-flows/configuration.md) for per-tier overrides — most production deployments need them.

## Things that look like bugs but aren't

- **Traffic appears 2×.** When the router is configured to export both ingress + egress (common, but not universal — vendor best practice is ingress-only), the same packet is recorded once on entry and once on exit on a single router. Filter to one exporter and one interface (`Ingress Interface Name` or `Egress Interface Name`, pick one).
- **Bidirectional conversations show twice.** A→B and B→A are real, distinct flows representing different packets going each way. Their volumes are usually asymmetric. Filter by `Source AS Name` (your network) for outbound or `Destination AS Name` (your network) for inbound to see one side.
- **City map empty over long windows.** City + lat/lon are raw-tier-only. Raw-tier retention is bounded by its size budget, so busy collectors can exhaust raw history quickly. Use the country or state map for long ranges.
- **`__overflow__` row in results.** Your aggregation produced more groups than `query_max_groups`. Narrow the filter or reduce group-by depth.
- **30-second query timeout.** Hard limit. Narrow time range, add filters, or reduce group-by depth.
- **Sampled byte counts not exact.** sFlow is statistical by design; even NetFlow with sampling is an estimate. Cross-check against SNMP for sanity, accept some divergence.
- **`enabled: false` makes the plugin look crashed.** It's intentional — the plugin tells the parent to stop respawning it. Look for the "disabled by config" line in the journal.

## Diagnostic command quick reference

```bash
# What's happening
sudo journalctl --namespace netdata --since "10 minutes ago" | grep -iE 'netflow|geoip|bmp|bioris|network-sources'

# What's arriving on the wire
sudo tcpdump -i any -nn -c 50 'udp port 2055 or udp port 6343'

# Is the listener bound
sudo ss -unlp | grep -E ':(2055|6343)([[:space:]]|$)'

# UDP kernel drops
sudo ss -uamn sport = :2055
sudo ss -uamn sport = :6343
grep ^Udp: /proc/net/snmp

# Disk usage by tier
sudo du -sh /var/cache/netdata/flows/*

# Process resources
top -p $(pgrep -f netflow-plugin)

# Capture a sample for offline analysis
sudo tcpdump -w /tmp/flow-sample.cap -c 200 'udp port 2055 or udp port 6343'
```

## When to file an issue

Collect this before opening a bug report:

- Plugin version (`netdata --version` from the running daemon).
- A sample of `netflow.input_packets` chart for the failure window — all dimensions visible.
- A sample of `netflow.facet_values`, `netflow.tier_index_entries`, and `netflow.open_tiers` if performance-related. Include `netflow.memory_resident_bytes` only if memory diagnostics were enabled.
- A small packet-capture file (`tcpdump -w` from the agent's interface) reproducing the issue.
- Sanitised `netflow.yaml` (redact internal IPs, customer names, secrets).
- Relevant log lines from `journalctl --namespace netdata`.

Open issues against [github.com/netdata/netdata](https://github.com/netdata/netdata) with `area/collectors/netflow` in the title.

## What's next

- [Plugin Health Charts](/docs/npm/network-flows/visualization/dashboard-cards.md) — The charts referenced above.
- [Validation and Data Quality](/docs/npm/network-flows/validation.md) — How to spot silent data corruption.
- [Anti-patterns](/docs/npm/network-flows/anti-patterns.md) — Why some "weird" results are actually normal.
- [Configuration](/docs/npm/network-flows/configuration.md) — Tuning that affects most of the symptoms above.
