<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/network-flows/anti-patterns.md"
sidebar_label: "Anti-patterns"
learn_status: "Published"
learn_rel_path: "Network Flows"
keywords: ['anti-patterns', 'mistakes', 'pitfalls', 'gotchas', 'misuse']
endmeta-->

<!-- markdownlint-disable-file -->

# Anti-patterns and pitfalls

Flow data is powerful but easy to misuse. The mistakes below are the ones that cause the most lost analyst time and the most wrong conclusions in real deployments. Each entry explains how the mistake happens, what it costs, and how to avoid it.

## 1. Reading aggregate volume without filtering

**The mistake.** You open Network Flows, see a total bandwidth number, and assume it represents your real traffic.

**Why it's wrong.** When a router is configured to export both ingress and egress flow records on every monitored interface — a common configuration, though vendor best practice is ingress-only — a single packet entering interface A and leaving interface B produces two records: one tagged ingress on A, one tagged egress on B. Summing every flow record without filtering then gives you roughly **2× the actual traffic** on a single such router. Add a second router on the same path with the same configuration and you see 4×.

**What it costs.** You think your link carries 2 Gbps when it really carries 1 Gbps. Capacity decisions based on these numbers are wrong by a factor of 2 or more.

**How to avoid it.** Always filter by one exporter and one interface (`Ingress Interface Name` OR `Egress Interface Name`, pick one) when reading absolute volume numbers. Each packet then appears in exactly one record on that interface. To validate: compare to SNMP interface counters on the same interface — values should be close.

## 2. Collecting flows but never looking at them

**The mistake.** Flow export is enabled on every router. Storage fills up. Nobody opens the dashboard between incidents.

**Why it's wrong.** Flow data is only useful when someone actively interprets it. Without baselines, watchlists, and routine review, you have data without insight.

**What it costs.** When an incident happens, you don't know what "normal" looks like, so you can't recognise abnormal. Storage and CPU are spent without operational value.

**How to avoid it.** Schedule a weekly 15-minute review. Document what "normal" looks like — top 10 talkers, traffic curve shape, protocol distribution, geographic distribution. Add anything new that appears in the top-10 to a watchlist for investigation. Use [Investigation Playbooks](/docs/network-flows/investigation-playbooks.md) for the recurring questions.

## 3. Confusing flows with sessions

**The mistake.** You see 50 000 flow records in an hour and report it as "we had 50 000 user sessions".

**Why it's wrong.** A flow record is a network-level artifact, not an application session. A single page load generates dozens of flows: DNS lookups, the TCP handshake, the TLS handshake, HTTP requests for embedded resources, telemetry pings. A long file transfer may be one flow or many, depending on timeout configuration.

**What it costs.** Wildly inflated user activity numbers. Misinterpretation of usage patterns.

**How to avoid it.** Aggregate by source IP and time window for a session-like view. Use ports and protocols to classify, not to count transactions. If you need real session data, use application logs or APM, not flow records.

## 4. NAT blindness

**The mistake.** You place the collector outside a NAT gateway because mirroring traffic there is easier.

**Why it's wrong.** Every internal host appears as the same public IP after NAT. You can't identify the actual source of the traffic.

**What it costs.** Your top talker is "the firewall". Security can't find the infected host, capacity can't identify the bandwidth hog.

**How to avoid it.** Collect inside each NAT boundary, or correlate flow data with NAT translation logs (`iptables NFLOG`, vendor NAT logging) to map external 5-tuples back to internal hosts.

## 5. Geographic firewall of shame

**The mistake.** You configure an alert: "page security if traffic goes to any country except the home country."

**Why it's wrong.** CDNs, cloud providers, and SaaS endpoints serve from edge nodes worldwide. Traffic to the same SaaS provider may resolve to Singapore one day and Frankfurt the next. None of this is suspicious.

**What it costs.** Constant false positives. Trust in the alerting system collapses. Real anomalies get ignored among the noise.

**How to avoid it.** Whitelist known cloud and CDN ASNs. Use ASN as the primary signal and country as secondary corroboration. If you must alert on country, alert only on countries you have no business relationship with — and review the whitelist quarterly.

## 6. Treating flow duration as latency

**The mistake.** You divide flow bytes by flow duration and present that as "speed", or use duration as a proxy for round-trip time.

**Why it's wrong.** Flow duration is dominated by the active timeout setting and application think time. A flow with a 60-second active timeout is exported every 60 seconds whether the network is fast or slow. There's no relationship between flow duration and latency.

**What it costs.** False conclusions about network performance. Misdirected troubleshooting.

**How to avoid it.** Use SNMP for interface utilisation, ICMP probes for round-trip time, APM tools for application performance. Flow data answers "how much" and "between whom", never "how fast".

## 7. Trying to detect microbursts

**The mistake.** Users complain about momentary slowness. You look in flow data for the burst.

**Why it's wrong.** NetFlow active timeout aggregates traffic into windows of 60 seconds or more. sFlow random sampling misses bursts that occur between sampled packets. Neither protocol can resolve sub-second events. The Netdata time-series view also clamps to 60-second buckets.

**What it costs.** You spend time looking for something flow data physically cannot show.

**How to avoid it.** For microburst detection use packet capture, switch microburst counters, or hardware-assisted telemetry. Flow data is for sustained patterns, not millisecond events.

## 8. Reasoning from raw byte counts when sampling is on

**The mistake.** You see `RAW_BYTES = 5000` for a flow and assume 5000 bytes was the actual traffic.

**Why it's wrong.** `RAW_BYTES` is the unscaled byte count from the exporter. With sampling at 1-in-1000, the actual traffic was approximately 5 000 000 bytes. The scaled value is in `BYTES`.

**What it costs.** A bandwidth estimate that is wrong by the sampling factor — typically 100× to 2000× too low — and capacity or anomaly conclusions drawn from those wrong numbers.

**How to avoid it.** Use `BYTES` (auto-scaled per-flow) for normal analysis. Use `RAW_BYTES` only when you specifically need the exporter's literal pre-scaling counts — for example, when comparing the per-flow record to the exporter's own logs.

## 9. Comparing flow counts across protocols

**The mistake.** You report "Arista switches see far more flows than Cisco routers" based on flow counts.

**Why it's wrong.** NetFlow aggregates many packets into one flow record. sFlow exports individual packet samples — each becomes its own "flow" record. Flow record counts mean different things in the two protocols and are not directly comparable.

**What it costs.** False conclusions about which devices "produce more traffic", capacity decisions made on artefact numbers rather than real traffic volume.

**How to avoid it.** Compare bytes (`BYTES`, auto-scaled per-flow at ingest), not flow record counts. Document which protocol each exporter speaks.

## Summary

| Mistake | One-line fix |
|---|---|
| Doubled aggregate (when ingress + egress are both exported) | Filter by one exporter and one interface (`Ingress Interface Name` or `Egress Interface Name`, pick one) |
| Collect-and-ignore | Weekly 15-minute review with documented baselines |
| Flows ≠ sessions | Aggregate by IP and time window |
| NAT blindness | Collect inside the NAT boundary |
| Geographic firewall of shame | Use ASN, whitelist cloud and CDN providers |
| Duration as latency | Use SNMP/ICMP/APM for latency |
| Microburst hunting | Use packet capture or hardware telemetry |
| Raw bytes confusion | Use `BYTES` (auto-scaled per-flow); use `RAW_BYTES` only when you need the exporter's literal pre-scaling counts |
| Cross-protocol flow counts | Use bytes (scaled), not flow counts |

## What's next

- [Validation and Data Quality](/docs/network-flows/validation.md) — How to confirm your data is trustworthy.
- [Investigation Playbooks](/docs/network-flows/investigation-playbooks.md) — Step-by-step recipes for common questions.
- [Flow Protocols](/src/crates/netflow-plugin/integrations/netflow.md) — Per-protocol behaviour that drives many of these gotchas.
