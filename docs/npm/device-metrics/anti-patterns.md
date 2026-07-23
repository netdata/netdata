<!-- markdownlint-disable-file -->

# Anti-patterns and pitfalls

SNMP polling is simple to start and easy to get subtly wrong. The mistakes below cause the most wasted time and the most wrong conclusions in real deployments. Each explains how the mistake happens, what it costs, and how to avoid it.

## 1. Polling every device every 10 seconds

**The mistake.** You leave the default interval on everything, including big core devices with large tables.

**Why it's wrong.** SNMP is served by a device's modest control-plane CPU. Walking a full MAC, interface, or BGP table every 10 seconds can spike that CPU and starve real work — and it inflates the hub's poll cycle for no benefit, since infrastructure rarely changes meaningfully sub-minute.

**What it costs.** Device CPU alarms, timeouts, gaps, and a hub that falls behind — self-inflicted.

**How to avoid it.** Match the interval to the device. Keep 10 seconds for simple, important interfaces; use 30–60 seconds for heavy core devices. Tune `update_every` (and `max_repetitions`) per job — see [Sizing and Scaling](/docs/npm/device-metrics/sizing-and-scaling.md).

## 2. Building a central SNMP poller for every site

**The mistake.** You point one big collector at every device across every site, the way classic central-NMS products work.

**Why it's wrong.** It creates a single tier that must be sized for the whole estate, ships management traffic over the WAN, and becomes a fleet-wide blast radius when it saturates or fails.

**What it costs.** WAN bandwidth, central capacity planning, and one outage that blinds everything.

**How to avoid it.** Run **one Netdata Agent per site as its SNMP hub**, polling only local devices, and stream to Parents with Cloud federating the global view. You scale by adding hubs, not by growing a central tier.

## 3. Polling across the WAN

**The mistake.** The collector sits in a central data center and polls remote-site devices over the WAN or a VPN.

**Why it's wrong.** Latency lengthens every poll, large walk responses can exceed the path MTU and get silently dropped (it looks like a timeout but isn't), and you pay WAN bandwidth for management traffic.

**What it costs.** Intermittent gaps that are maddening to diagnose, and slow cycles.

**How to avoid it.** Poll locally. The hub belongs on the device LAN. If a site is too small for its own Agent, keep its walks small (`max_repetitions`) and its interval long.

## 4. Treating "SNMP responds" as "the device is healthy"

**The mistake.** The device answers SNMP, so you mark it healthy.

**Why it's wrong.** SNMP reaches the management plane. A device can answer SNMP while its data plane drops traffic, and a hard fault can stop SNMP entirely with no warning. Polling confirms current state on an interval; it is not a guarantee of forwarding health.

**What it costs.** A "green" device that is silently dropping customer traffic.

**How to avoid it.** Read device metrics together with [SNMP Traps](/docs/npm/snmp-traps/README.md) for the transitions a device reports and [Network Flows](/docs/npm/network-flows/README.md) for whether traffic is actually moving. The hub co-locates all three for exactly this reason.

## 5. Trusting 32-bit counters on high-speed links

**The mistake.** A profile collects the legacy `ifInOctets`/`ifOutOctets` counters on 1G+ interfaces.

**Why it's wrong.** A 32-bit octet counter wraps in roughly 3.4 seconds at 10G line rate. Naive differencing turns each wrap into a fake terabit spike — so teams either chase phantoms or learn to ignore spikes and then miss a real one.

**What it costs.** Alert fatigue, or a missed real event dismissed as "just a rollover."

**How to avoid it.** Use the 64-bit high-capacity counters (`ifHCInOctets`). Netdata reads HC counters where the device exposes them; make sure your profile collects them on every link at 1G and above.

## 6. Polling only, with no traps

**The mistake.** You rely on the polling interval to catch everything.

**Why it's wrong.** Polling samples state every few seconds; a link that bounces between two polls, or a brief auth failure, can leave no trace in the sampled data. Devices push those transitions the instant they happen — but only if you're listening.

**What it costs.** Missed flaps and transient events that polling never sampled.

**How to avoid it.** Enable [SNMP Traps](/docs/npm/snmp-traps/README.md) on the same hub. Traps catch the transitions between polls; polling confirms the steady state. Neither replaces the other.

## 7. SNMPv2c on an exposed management network

**The mistake.** You use SNMPv2c community strings on a management network that isn't isolated.

**Why it's wrong.** SNMPv2c community strings are sent in cleartext. Anyone who can sniff the management path can capture them — and a default community like `public` is the first thing a scan tries.

**What it costs.** Read access to your device fleet, handed to anyone on the wire.

**How to avoid it.** Use SNMPv3 with `authPriv` where devices support it, and isolate the management VLAN. Never leave default community strings in place.

## Summary

| Mistake | One-line fix |
|---|---|
| Polling everything at 10s | Match `update_every` to the device; 30–60s for heavy core gear |
| Central poller per site | One Agent per site as the SNMP hub; stream to Parents |
| Polling across the WAN | Poll locally; keep walks under the path MTU |
| SNMP-up = healthy | Pair polling with traps (transitions) and flows (traffic) |
| 32-bit counters on fast links | Collect 64-bit HC counters (`ifHCInOctets`) |
| Polling only, no traps | Run traps on the same hub for between-poll transitions |
| SNMPv2c on an exposed network | SNMPv3 `authPriv`; isolate the management VLAN |

## What's next

- [Validation](/docs/npm/device-metrics/validation.md) — confirm your data is trustworthy.
- [Sizing and Scaling](/docs/npm/device-metrics/sizing-and-scaling.md) — the hub model these pitfalls point back to.
- [SNMP Traps](/docs/npm/snmp-traps/README.md) and [Network Flows](/docs/npm/network-flows/README.md) — the other legs of device monitoring.
