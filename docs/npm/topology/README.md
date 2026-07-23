<!-- markdownlint-disable-file -->

# Network Topologies

Netdata shows you how your network is connected — which device links to which, and where each part of your infrastructure sits — built automatically from the devices you already monitor.

![Network topology overview](https://www.netdata.cloud/img/network/snmp-topology-overview.png)

The topology view, assembled automatically from the devices you already monitor — Layer 2 and Layer 3 links, kept current as devices come and go.

## Built from your devices

When Netdata monitors your SNMP devices, it reads what they already know about their neighbors and assembles the topology, with no extra setup:

- **LLDP and CDP** — the neighbor each device advertises (device, port, platform).
- **Forwarding (FDB) and ARP tables** — which MAC and IP addresses are seen on which switch ports.
- **Spanning Tree (STP)** — which Layer 2 links are forwarding and which are blocked.
- **BGP and OSPF** — the Layer 3 routing relationships between routers.

The result is a live Layer 2 and Layer 3 map, kept current as devices come and go. Each link carries the evidence behind it — how it was discovered and how confident Netdata is in it — so a confirmed link is distinguishable from an inferred one.

## What you can do with it

- **See the real wiring** — device-to-device links across your network.
- **Trace a path** — follow how two points connect, hop by hop.
- **Find what's affected** — what sits downstream of a link or device that's in trouble.
- **Locate an endpoint** — which switch port an address is on, cross-referenced from forwarding and neighbor data.

## Beyond the device fabric

The same topology view brings in other infrastructure, each in the same form so it sits alongside your device topology:

- **Live network connections** — what your hosts and services are talking to right now (`topology:network-connections`).
- **Netdata streaming** — how your Agents connect to each other (`topology:streaming`).
- **VMware vSphere** — datacenters, clusters, resource pools, hosts, VMs, datastores, and networks (`topology:vsphere`).
- **Cato Networks** — Cato SASE sites, devices, POPs, and BGP peers (`topology:cato_networks`).

The device fabric (`topology:snmp`), live connections, and streaming come up automatically; vSphere and Cato come from their own collectors, configured separately.

![Live network connections function](https://www.netdata.cloud/img/dashboard-screens/functions-network-connections.png)

Live host and service connections (`topology:network-connections`) appear in the same topology view, alongside your device fabric.

## Where to start

- Topology comes up on its own once you're [monitoring devices](/docs/npm/device-metrics/README.md) — open the topology view in Netdata.
- The entries in this section list each discovery method and source, and what each one contributes to the map.
