<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/npm/README.md"
sidebar_label: "Overview"
learn_status: "Published"
learn_rel_path: "Network Performance Monitoring"
keywords: ['network performance monitoring', 'npm', 'snmp', 'network', 'topology', 'flows', 'traps', 'overview']
endmeta-->

<!-- markdownlint-disable-file -->

# Network Performance Monitoring

Network Performance Monitoring (NPM) in Netdata covers your network devices and the traffic between them — metrics, topology, flows, events, and logs — collected by the Netdata Agent and visualized per second in Netdata dashboards and Netdata Cloud.

## What you can monitor

- **Device metrics (SNMP)** — poll routers, switches, firewalls, access points, UPSs, and PDUs. Netdata matches each device to a vendor profile by its `sysObjectID` and collects interfaces (traffic, errors, discards, operational state), system and host resources, and vendor-specific hardware, environmental, and protocol metrics. Every metric is a chart you can alert on, with interactive per-interface and per-device tables.

- **Topology** — see how your network connects. Netdata builds Layer 2 maps (LLDP, CDP, MAC/FDB, STP) and Layer 3 maps (BGP, OSPF, ARP), plus live host connections, the Netdata streaming hierarchy, and virtual infrastructure (VMware vSphere, Cato).

- **BGP monitoring** — peer state, advertised and received prefixes, and session health on your routers, with an interactive peer table and alerts on session changes.

- **Licensing** — license state, entitlements, and expiry on devices that report them (Cisco Smart Licensing, Fortinet, Check Point, and others), so a feature never silently stops.

- **Network flows** — collect NetFlow, sFlow, and IPFIX. See who is talking to whom, how much, and over which ports and protocols, enriched with geolocation, ASN, and IPAM data, in Sankey, time-series, and map views.

- **SNMP traps** — receive and decode the events your devices report (thousands of trap definitions across hundreds of vendors) into structured, searchable journal entries with severity and category, queryable in Netdata Logs and forwardable to your SIEM.

- **Syslog from network devices** — ingest device syslog through the OpenTelemetry Collector into Netdata Logs.

## Supported devices

Netdata ships profiles for hundreds of device and trap vendors. Each capability lists exactly what it covers under its **Integrations** submenu: if your device is there, Netdata collects it out of the box; if it isn't, you can add a custom profile. Start from [Device Metrics](/docs/npm/device-metrics/README.md) to check coverage and add devices.

## Where to start

- **Monitor your devices** → [Device Metrics](/docs/npm/device-metrics/README.md)
- **See how they connect** → [Topologies](/docs/npm/topology/README.md)
- **Watch routing and licensing** → [BGP Monitoring](/docs/npm/bgp/README.md) and [Licensing Monitoring](/docs/npm/licensing/README.md)
- **Analyze their traffic** → [Network Flows](/docs/npm/network-flows/README.md)
- **Catch their events and logs** → [SNMP Traps](/docs/npm/snmp-traps/README.md) and [Syslog from Network Devices](/docs/npm/syslog/README.md)
