<!-- markdownlint-disable-file -->

# Sizing and Scaling

Netdata monitors your network from Agents you place across it — every device polled in parallel, at a cadence you set per device — and scales to thousands of devices and hundreds of sites by **adding Agents, not by growing a central server**. However you split the network, the dashboards stay unified.

## One Agent per site is your hub

Run the SNMP collector on the Agent on the device LAN. That Agent is the **hub for its part of the network**: it polls the local devices, maps their topology, and receives their traps — together, near the devices. Each device appears as its own node with full charts, alerts, and retention. This is the unit you deploy and size.

## Scale up and out — the dashboards don't change

You can grow a hub two ways, and mix them freely:

- **Up** — a larger Agent that covers more devices.
- **Out** — more Agents, each owning a slice of the network (a site, a building, a subnet, a region — split it however suits you).

One rule makes the difference: **keep a slice's SNMP, traps, and topology on the same Agent.** That co-location is what lets traps arrive already enriched with device and topology context, automatically, with no correlation rules to maintain. (NetFlow has no such tie — you can run it on the same Agent or a separate one, as you prefer.)

No matter how you split the network — up, out, or both — **[Netdata Cloud](https://app.netdata.cloud) presents one unified view**: dashboards and a single topology across every hub. The deployment shape is your operational choice; it never changes what operators see.

For centralized retention and high availability, hubs stream to [Parents](/docs/observability-centralization-points/metrics-centralization-points/clustering-and-high-availability-of-netdata-parents.md) (a Parent handles on the order of 500 Agents); cluster Parents for HA.

## What sets a hub's capacity

SNMP polling is **CPU work** — and the CPU that matters most is usually the *device's*, not the Agent's. The Agent polls every device in parallel; a device, by contrast, answers SNMP from a modest control-plane processor it shares with real work, and walking a large table (a full MAC, interface, or BGP table) costs it real CPU.

So you pace polling to the devices:

- **Match the interval to the device.** The default is a safe 10 seconds. Tighten it for the few interfaces where finer resolution is worth it and the device can take it (busy uplinks, core links); relax it to 30 or 60 seconds for slower or busier-CPU gear. You set the interval per device, so light and heavy devices coexist on one hub.
- **Ease the heavy walks** with a smaller `max_repetitions` on any device that struggles with large responses.

When a slice of the network outgrows comfortable pacing on one Agent, you don't push harder — you **add an Agent** and hand it part of the network.

## Keep a hub healthy

Two signals, both visible in Netdata, tell you a hub is comfortable:

- **Each poll cycle finishes inside its interval** (the collector reports poll timings per device).
- **Per-device SNMP timeout/retry rates stay near zero.**

If a heavy device pushes either, give it a longer interval or a smaller `max_repetitions` — see [Troubleshooting](/docs/npm/device-metrics/troubleshooting.md).

## Checklist

1. **Put the hub on the device LAN** and let it poll locally.
2. **Set the interval per device** — tight where you need resolution, longer for slow or busy gear.
3. **Keep SNMP + traps + topology together** on each Agent so enrichment stays automatic.
4. **Add Agents** to grow — split the network however suits you; the dashboards stay unified.
5. **Stream to Parents** for centralized retention and HA.

## What's next

- [Configuration](/docs/npm/device-metrics/configuration.md) — intervals, `max_repetitions`, SNMPv3, discovery.
- [Validation](/docs/npm/device-metrics/validation.md) — confirm clean polling.
- [Troubleshooting](/docs/npm/device-metrics/troubleshooting.md) — pacing a device that struggles to answer.
