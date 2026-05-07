<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/network-flows/troubleshooting.md"
sidebar_label: "Troubleshooting"
learn_status: "Published"
learn_rel_path: "Network Flows"
keywords: ['troubleshooting', 'debugging', 'plugin health', 'failures']
endmeta-->

# Troubleshooting

Concrete recipes for the most common failures, organised by symptom. Most issues are diagnosable from the [plugin health charts](/docs/network-flows/visualization/dashboard-cards.md), the Netdata journal logs, and a couple of OS-level commands.

## The plugin doesn't start

The plugin won't come up at all, or starts and immediately exits.

**Symptoms:**
- Netdata reports `netflow-plugin` as not running, or restart-looping.
- Nothing in the Network Flows tab.
- An error in `journalctl -u netdata`.

**Likely causes:**

| Cause | What to check |
|---|---|
| YAML typo or unknown key | `journalctl -u netdata --since "5 minutes ago" \| grep -E 'failed to load configuration\|netflow'`. The plugin uses strict YAML — any unknown key fails parsing. |
| Required GeoIP DB missing (`optional: false`) | Same log search. Look for `failed to load database`. Either fix the path or set `optional: true`. |
| Listen address conflict | Look for `failed to bind`. Another process is on the configured port (default 2055). |
| Validation error | Look for `must be greater than 0` and similar. The plugin validates the full config at startup. |
| `enabled: false` was set | Look for `netflow plugin disabled by config`. The plugin honours this and shuts down cleanly — looks like "not running" if you don't read the log. |

**Recovery:**

```bash
# Read the failure
sudo journalctl -u netdata --since "5 minutes ago" | grep -E 'netflow|failed to|error'

# Validate the YAML (use an online linter or `yamllint`)
yamllint /etc/netdata/netflow.yaml

# After fixing, restart
sudo systemctl restart netdata
```

## The plugin starts, but no flows appear

The plugin is running, but the Network Flows tab is empty.

**First check:** is anything reaching the plugin?

```bash
sudo tcpdump -i any -nn -c 50 'udp port 2055'
```

- **No packets in 30 seconds** — exporter not sending, or firewall blocking. Check the exporter's status (`show flow exporter` on Cisco, equivalents elsewhere) and the network path. The plugin can't help here; the data isn't reaching it.
- **Packets arriving** — keep going.

**Second check:** is the listener bound?

```bash
sudo ss -unlp | grep -E ':2055|netflow'
```

If nothing matches, the plugin isn't listening. See "doesn't start" above.

**Third check:** what do the plugin's own counters say?

Open `netflow.input_packets` on the standard Netdata charts page. The dimensions tell the story:

- `udp_received > 0`, `parsed_packets == 0` — datagrams arriving, none decoding successfully. Wrong protocol on the listener, or all datagrams malformed.
- `udp_received > 0`, `parsed_packets > 0`, but no per-protocol counter (`netflow_v9`, `ipfix`, etc.) is moving — the protocol you're sending may be disabled in the plugin config. Check `protocols.v9`, `protocols.ipfix`, etc. in `netflow.yaml`.
- `parse_errors` rising in lockstep with `udp_received` — datagrams aren't valid for the protocols the plugin supports. Capture a sample (`tcpdump -w sample.pcap`) and inspect with Wireshark.

## Partial data — some flows are dropped

Counters show received traffic but you suspect data loss.

**Template errors (NetFlow v9, IPFIX):**

```bash
# Watch the template_errors dimension
# In the dashboard: netflow.input_packets > template_errors
```

If it's climbing, the exporter is sending data records before their templates. Either:

- The exporter restarted and the plugin's template cache is stale. Wait for the exporter to send the next template (typically every 30-60 seconds, depending on its config), or restart the exporter to force an immediate template refresh.
- Templates are sent rarely (Cisco's default template refresh is 30 minutes). After a plugin restart, you'll see template errors for that long. **Fix on the router side**: lower the template refresh interval to 60 seconds.
- The exporter is using template IDs that collide with another exporter's templates. Most common cause: two exporters NATted behind the same public IP. Place the plugin inside the NAT boundary or give each exporter a distinct address.

**UDP kernel drops:**

The plugin doesn't count these. Check at the OS level:

```bash
sudo ss -uam sport = :2055         # check 'd' columns for drops
cat /proc/net/udp | head -20       # RcvbufErrors column
```

If drops are occurring, the kernel UDP receive buffer is too small for the burst rate. Tune:

```bash
sudo sysctl -w net.core.rmem_max=33554432
sudo sysctl -w net.core.rmem_default=8388608
sudo sysctl -w net.core.netdev_max_backlog=250000
```

Persist in `/etc/sysctl.d/99-netflow.conf`.

**Per-protocol switch off:**

```yaml
protocols:
  v5: false      # are you accidentally rejecting v5 datagrams?
```

## Data is wrong — numbers don't match expectations

**Volume looks doubled:**

This is the most common report. With one router, traffic appears 2× because every packet generates an ingress record AND an egress record. With two routers on the same path, 4×. Filter to one exporter + one direction (input interface OR output interface) to see real volume. See [Anti-patterns](/docs/network-flows/anti-patterns.md).

**Bandwidth doesn't match SNMP:**

Several legitimate causes:

- **Doubling**, as above. Filter properly before comparing.
- **Sampling rate not honoured.** The plugin auto-multiplies bytes by the sampling rate, but if the exporter doesn't carry the rate (NetFlow v7 has no field for it; v5 sometimes sends 0 instead of the actual rate; v9 may not send the Sampling Options Template), the result is undercounted.
- **Mixed sampling rates across exporters.** If your dashboard aggregates exporters with different rates, the result blends estimates and isn't comparable to any single SNMP measurement.
- **SNMP includes layer-2 traffic** (ARP, STP, LLDP, routing protocols) that flow data filters out. Expect SNMP to be 5-15% higher than flow on a healthy collector. More than that, investigate.

See [Validation and Data Quality](/docs/network-flows/validation.md).

**Internal IPs in random countries:**

GeoIP databases don't have entries for RFC 1918 / private space. The plugin doesn't skip private IPs — it just hands the IP to the database and uses what comes back. For the stock DB-IP build, private ranges are tagged so they render as "AS0 Private IP Address Space" with empty country. For other MMDBs, private ranges may resolve to weird countries.

**Fix:** declare your internal CIDRs under `enrichment.networks` with country / role / name labels. See [Static metadata](/docs/network-flows/enrichment/static-metadata.md).

**AS resolution chain misbehaving:**

If `SRC_AS` / `DST_AS` are zero everywhere despite the exporter sending them, check the `asn_providers` chain:

- `[geoip, ...]` — `geoip` is a terminal short-circuit. The chain stops at `geoip` (it returns 0). Reorder: `[flow, routing, geoip]`.
- `[]` (empty) — no validation rejects this. Every AS is forced to 0.

See [ASN resolution](/docs/network-flows/enrichment/asn-resolution.md).

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

See [Sizing and Capacity Planning](/docs/network-flows/sizing-capacity.md) for measured throughput limits on this hardware class.

**Memory growth:**

```bash
# Watch the resident memory chart over time
# netflow.memory_resident_bytes - rss dimension
```

- If `rss` climbs and `netflow.memory_accounted_bytes` shows `unaccounted` growing, that's an unattributed allocation — could be allocator fragmentation, possibly a leak.
- If `tier_indexes` or `open_tiers` is the climbing dimension, ingest is outpacing tier flushes. Check `netflow.materialized_tier_ops` for `flushes` rate and `*_errors`.
- If `netflow.decoder_scopes` is growing without bound, your exporter is rotating template IDs. Investigate per-router behaviour.

**Disk fill:**

```bash
sudo du -sh /var/cache/netdata/flows/*
```

Default retention is `10GB / 7d` per tier — the same budget applies to all four tiers, so total can reach roughly 40 GB plus some. If your config left this default and your collector is busy, expect to hit it. See [Configuration](/docs/network-flows/configuration.md) for per-tier overrides — most production deployments need them.

## Things that look like bugs but aren't

- **Traffic appears 2×.** Standard ingress + egress monitoring. Filter to one direction.
- **Bidirectional conversations show twice.** A→B and B→A are real, distinct flows. Filter to one direction or one ASN to see one side.
- **Internal IPs in odd countries.** GeoIP doesn't know about your private space. Declare it explicitly.
- **City map empty over long windows.** City + lat/lon are tier-0-only. Default tier-0 retention is short. Use the country map for long ranges.
- **`__overflow__` row in results.** Your aggregation produced more groups than `query_max_groups`. Narrow the filter or reduce group-by depth.
- **30-second query timeout.** Hard limit. Narrow time range, add filters, or reduce group-by depth.
- **Sampled byte counts not exact.** sFlow is statistical by design; even NetFlow with sampling is an estimate. Cross-check against SNMP for sanity, accept some divergence.
- **`enabled: false` makes the plugin look crashed.** It's intentional — the plugin tells the parent to stop respawning it. Look for the "disabled by config" line in the journal.

## Diagnostic command quick reference

```bash
# What's happening
sudo journalctl -u netdata --since "10 minutes ago" | grep -iE 'netflow|geoip|bmp|bioris|network-sources'

# What's arriving on the wire
sudo tcpdump -i any -nn -c 50 'udp port 2055'

# Is the listener bound
sudo ss -unlp | grep 2055

# UDP kernel drops
sudo ss -uam sport = :2055
cat /proc/net/udp

# Disk usage by tier
sudo du -sh /var/cache/netdata/flows/*

# Process resources
top -p $(pgrep -f netflow-plugin)

# Capture a sample for offline analysis
sudo tcpdump -w /tmp/netflow-sample.pcap -c 200 'udp port 2055'
```

## When to file an issue

Collect this before opening a bug report:

- Plugin version (`netdata --version` from the running daemon).
- A sample of `netflow.input_packets` chart for the failure window — all dimensions visible.
- A sample of `netflow.memory_resident_bytes` if performance-related.
- A captured pcap (`tcpdump -w` from the agent's interface) reproducing the issue.
- Sanitised `netflow.yaml` (redact internal IPs, customer names, secrets).
- Relevant log lines from `journalctl -u netdata`.

Open issues against [github.com/netdata/netdata](https://github.com/netdata/netdata) with `area/collectors/netflow` in the title.

## What's next

- [Plugin Health Charts](/docs/network-flows/visualization/dashboard-cards.md) — The charts referenced above.
- [Validation and Data Quality](/docs/network-flows/validation.md) — How to spot silent data corruption.
- [Anti-patterns](/docs/network-flows/anti-patterns.md) — Why some "weird" results are actually normal.
- [Configuration](/docs/network-flows/configuration.md) — Tuning that affects most of the symptoms above.
