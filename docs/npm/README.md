<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/npm/README.md"
sidebar_label: "Overview"
learn_status: "Published"
learn_rel_path: "Network Performance Monitoring"
keywords: ['network performance monitoring', 'npm', 'snmp', 'network', 'topology', 'flows', 'traps', 'overview']
endmeta-->

<!-- markdownlint-disable-file -->

# Network Performance Monitoring

You run a network — devices spread across sites, the traffic moving between them, and the events and logs they emit. Netdata gives you **one way to see all of it**, without building a monitoring system to do it.

This section is for the network and SRE teams who keep the fabric running: you want to find the saturated link, the unhealthy device, the flapping peer, the expiring license — across your whole network, in one place.

## One Agent per site, and it does the rest

Put a Netdata Agent at each site and point it at your network. It becomes the **hub** for that part of the network:

- it **discovers your devices** and monitors them — interfaces, health, errors;
- it **maps how they connect** — the Layer 2 and Layer 3 topology;
- it **catches the events and logs** they send — traps and syslog.

These come up **together, automatically, already correlated**: a trap arrives knowing which device sent it and where it sits in the topology. There are no correlation rules to write. Add **network flows** and you also see who is talking to whom, how much, and where.

## Scale by adding Agents — the view stays the same

Grow the way your network is shaped: a **bigger Agent** for more devices, or **more Agents**, each owning a site, a building, or a subnet. Keep a slice's SNMP, traps, and topology on the same Agent and the correlation stays automatic; flows can run wherever you like.

However you split it, **Netdata Cloud gives you one unified view** — dashboards and a single topology of the whole network. There's no central server to size or babysit: you add a site by adding an Agent.

## What's in this section

- **[Device Metrics](/docs/npm/device-metrics/README.md)** — discover and monitor your devices over SNMP: interfaces, health, errors, and more.
- **[Topologies](/docs/npm/topology/README.md)** — see how your network is connected: device topology, live connections, and virtual infrastructure.
- **[BGP Monitoring](/docs/npm/bgp/README.md)** — peer state, prefixes, and session health on your routers.
- **[Licensing Monitoring](/docs/npm/licensing/README.md)** — track license state and expiry before a feature silently stops.
- **[Network Flows](/docs/npm/network-flows/README.md)** — who is talking to whom, how much, over what, and to where (NetFlow / IPFIX / sFlow).
- **[SNMP Traps](/docs/npm/snmp-traps/README.md)** — the asynchronous events your devices report.
- **[Syslog from Network Devices](/docs/npm/syslog/README.md)** — device logs, ingested through the OpenTelemetry Collector.

## Where to start

- **Monitor your devices** — [Device Metrics](/docs/npm/device-metrics/README.md).
- **See how they connect** — [Topologies](/docs/npm/topology/README.md).
- **Catch their events** — [SNMP Traps](/docs/npm/snmp-traps/README.md).
- **Analyze their traffic** — [Network Flows](/docs/npm/network-flows/README.md).
- **Read their logs** — [Syslog from Network Devices](/docs/npm/syslog/README.md).
- **Watch routing and licensing** — [BGP Monitoring](/docs/npm/bgp/README.md) and [Licensing Monitoring](/docs/npm/licensing/README.md).

Each one runs on the same hub. Start with whichever question is in front of you.
