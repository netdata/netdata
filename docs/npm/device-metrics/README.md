<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/npm/device-metrics/README.md"
sidebar_label: "Overview"
learn_status: "Published"
learn_rel_path: "Network Performance Monitoring/Device Metrics"
keywords: ['snmp', 'network devices', 'router', 'switch', 'firewall', 'discovery', 'interface', 'overview']
endmeta-->

<!-- markdownlint-disable-file -->

# Device Metrics

Netdata monitors your network devices over SNMP — routers, switches, firewalls, access points, load balancers, UPS and PDU units. Point it at your network and it **discovers your devices, recognizes each model automatically, and starts monitoring them** — interface traffic, errors, device health, BGP, licenses — with no OID lists to build and no per-device dashboards to design.

SNMP devices answer from a small management processor, so Netdata polls them at a cadence the device can sustain: a safe **10 seconds by default**, tunable per device — tighter where the data is worth it and the device can take it, gentler on heavy or fragile gear.

This section is for the network and SRE teams who run the fabric: you want to know which links are saturated, which devices are unhealthy, and you want it for every device in every site without hand-building a monitoring system.

## What you get, with almost no configuration

- **Discovery.** Give Netdata your network ranges and SNMP credentials and it finds the devices on them — or point it at a single device directly. Either way it reads the device, recognizes the model, and starts collecting.
- **Full coverage per device, automatically.** Interface traffic, errors, discards, and operational status; device health (CPU and memory, plus environment sensors — temperature, fans, power supplies — where the device's profile exposes them); and — where the device exposes them — BGP peers and license state. Each device becomes **its own node** with live charts and alerts.
- **Topology and traps, together, automatically.** On the same Agent, each device's **topology** (how it connects to its neighbors) and its **SNMP traps** come up alongside its metrics — and every trap arrives **already labeled with the device that sent it and its place in the topology**. There are no correlation rules to write.

That last point is Netdata's zero-configuration promise applied to the network: discovery creates the jobs, topology follows the devices, and traps enrich themselves.

## One Agent per site is the hub

Run the Agent on the device LAN. That Agent is the site's hub — it polls the local devices, maps their topology, and receives their traps, all in one place, close to the devices. Scale the way that fits your network:

- **Up** — a larger Agent covering more devices.
- **Out** — more Agents, each owning a part of the network.

Keep a segment's SNMP, traps, and topology on the same Agent and the enrichment stays automatic. As you grow, you add Agents — and [Netdata Cloud](https://app.netdata.cloud) gives you **one unified view**, dashboards and a single topology, across every hub. See [Sizing and Scaling](/docs/npm/device-metrics/sizing-and-scaling.md).

## What you can answer

- Which interfaces are saturated, erroring, or discarding — on which device, right now and over time?
- Is this device healthy — control-plane CPU, memory, temperature, fans, power supplies?
- Which interfaces flapped, or went down?
- What model and vendor is each device? (Netdata recognizes it for you.)
- What is the state of a router's BGP peers, and are they still receiving updates?
- Which firewalls have licenses about to expire?

## What to reach for instead

The same hub runs these too — use them for the questions SNMP polling isn't built for:

- **Who is talking to whom on the wire** — [Network Flows](/docs/npm/network-flows/README.md).
- **An event the moment a device reports it**, between polls — [SNMP Traps](/docs/npm/snmp-traps/README.md).
- **Sub-second microbursts** — flow data or device-side counters; polling samples on an interval.

## What ships with the collector

- **SNMP v1, v2c, and v3 polling**, with optional ICMP latency and loss alongside every device.
- **Automatic model recognition** from a large built-in profile library — interfaces, system health, environment sensors, and vendor-specific metrics — with custom profiles for anything special, and standard interface and system metrics for any device that reports a `sysObjectID`, even before a vendor-specific profile matches.
- **A node per device**, with live charts, labels, and alerts.
- **Live tables** you can open on any device: `snmp:interfaces` (traffic, status, errors, discards), `snmp:bgp-peers` (per-peer state and recency), and `snmp:licenses`.
- **Stock alerts** for license expiry and BGP session health.

## Where to start

- **Your first device** — [Quick Start](/docs/npm/device-metrics/quick-start.md). To onboard a whole subnet at once, enable [discovery](/docs/npm/device-metrics/configuration.md).
- **Add credentials, SNMPv3, many devices** — [Configuration](/docs/npm/device-metrics/configuration.md).
- **Understand or extend model recognition** — [SNMP Profile Format](/src/go/plugin/go.d/collector/snmp/profile-format.md).
- **Many devices or many sites** — [Sizing and Scaling](/docs/npm/device-metrics/sizing-and-scaling.md).
- **Trust the data** — [Validation](/docs/npm/device-metrics/validation.md); avoid the common mistakes in [Anti-patterns](/docs/npm/device-metrics/anti-patterns.md).
- **Something's off** — [Troubleshooting](/docs/npm/device-metrics/troubleshooting.md).
- **Topology, BGP, licensing, traps, flows, syslog** — the same hub does all of it; see the other sections of Network Performance Monitoring.
