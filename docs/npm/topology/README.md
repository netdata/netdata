<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/npm/topology/README.md"
sidebar_label: "Overview"
learn_status: "Published"
learn_rel_path: "Network Performance Monitoring/Topologies"
keywords: ['topology', 'network topology', 'lldp', 'cdp', 'fdb', 'arp', 'stp', 'bgp', 'ospf', 'overview']
endmeta-->

<!-- markdownlint-disable-file -->

# Network Topologies

Netdata shows you **how your network is actually connected** — which device links to which, and where each part of your infrastructure sits — and it builds that picture automatically from the devices you're already monitoring.

This section is for the teams who need the real shape of the network: to trace a path, find what's downstream of a failing link, or see how a site is wired — without drawing it by hand or keeping a diagram up to date.

## Built automatically from your devices

When Netdata monitors your SNMP devices, it also reads what they already know about their neighbors and assembles the topology from it — with no extra setup:

- **LLDP and CDP** — the neighbor each device advertises (device, port, platform).
- **Forwarding (FDB) and ARP tables** — which MAC and IP addresses are seen on which switch ports.
- **Spanning Tree (STP)** — which Layer 2 links are forwarding and which are blocked.
- **BGP and OSPF** — the Layer 3 routing relationships between your routers.

The result is a live Layer 2 and Layer 3 map of your network, kept current as devices come and go. Because it's built from the same devices you're polling, it stays in step with your metrics and traps — when a trap or an alert points at a device, you can already see where that device sits.

## What you can do with it

- **See the real wiring** — device-to-device links across your network, instead of a stale diagram.
- **Trace a path** — follow how two points connect, hop by hop.
- **Find what's affected** — what sits downstream of a link or device that's in trouble.
- **Locate an endpoint** — which switch port an address is on, cross-referenced from forwarding and neighbor data.

Each link carries the evidence behind it — how it was discovered and how confident Netdata is in it — so you can tell a freshly-confirmed link from one that's inferred.

## More than the device fabric

The same topology view brings in other parts of your infrastructure, each in the same form so they sit alongside your device topology rather than in a separate tool:

- **Live network connections** — what your hosts and services are actually talking to, right now.
- **Netdata streaming** — how your Netdata Agents connect to each other.
- **vSphere** — your VMware datacenters, clusters, resource pools, hosts, VMs, datastores, datastore clusters, and networks.
- **Cato Networks** — your Cato SASE sites, devices, POPs, and BGP peers.

The device fabric, live connections, and Agent streaming come up automatically; **vSphere** and **Cato Networks** come from their own collectors, which you configure separately. Each is its own view — `topology:snmp` for the device fabric, `topology:network-connections` for live connections, `topology:streaming` for the Agent mesh, plus `topology:vsphere` and `topology:cato_networks` — and Netdata Cloud overlays them into one picture.

## One topology across every site

Each hub builds the topology for its own part of the network, and **Netdata Cloud aggregates them into one** — a single topology of your entire network, however many Agents and sites you run. Scaling out adds detail to the same picture; it never fragments it.

## Where to start

- Topology comes up on its own once you're [monitoring devices](/docs/npm/device-metrics/README.md) — open the topology view in Netdata to see it.
- The entries in this section list each discovery method and source, and what each one contributes to the map.
