<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/npm/topology/discovery-methods.md"
sidebar_label: "Discovery Methods"
learn_status: "Published"
learn_rel_path: "Network Performance Monitoring/Topologies"
keywords: ['topology', 'lldp', 'cdp', 'fdb', 'arp', 'stp', 'bgp', 'ospf', 'discovery']
endmeta-->

<!-- markdownlint-disable-file -->

# Discovery Methods

Netdata builds your topology by reading, over SNMP, what your devices already know about their neighbors. Each method below contributes part of the map; together they give the full Layer 2 and Layer 3 picture. They all come up automatically — there's nothing to enable per method.

## Layer 2 — how switches and their links connect

- **LLDP** (Link Layer Discovery Protocol) — the vendor-neutral neighbor each device advertises: chassis, port, system name, management address. The backbone of device-to-device links.
- **CDP** (Cisco Discovery Protocol) — the same neighbor discovery on Cisco and Cisco-compatible gear, with extra detail like platform, native VLAN, and duplex.
- **FDB** (forwarding database) — the MAC address tables of your switches: which MAC is seen on which port. This is how Netdata knows where endpoints attach.
- **ARP / IP neighbors** — the IP-to-MAC bindings from routers and switches, so an endpoint can be located by its IP, not only its MAC.
- **STP** (Spanning Tree) — which Layer 2 links are forwarding and which are blocked, so the map reflects the paths traffic actually takes.

## Layer 3 — how routers connect

- **BGP** — the BGP peering relationships between your routers (neighbor and remote AS), drawn as router-to-router links.
- **OSPF** — the OSPF adjacencies between your routers.
- **Connected subnets** — point-to-point router links inferred from interface IP addresses that share a `/30` or `/31` subnet, catching the router-to-router links that aren't carried by BGP or OSPF (directly-connected or statically-routed links).

## How they come together

Netdata fuses these per device into one live graph: LLDP and CDP give the links, FDB and ARP place the endpoints, STP marks which links are active, and BGP and OSPF add the routing layer. Each link records how it was discovered and how confident Netdata is in it. Because it's all built from the devices you're already polling, the topology stays current as the network changes.

## Open and shape the map

The device topology is served by the **`topology:snmp`** function — open it from the topology view to see the fabric. A few controls shape what you see, so you can move between a high-trust overview and the complete picture:

- **Nodes identity** — show actors by **IP** (the default, which collapses duplicates and drops non-IP inferred nodes) or by **MAC**.
- **Map** — choose **LLDP/CDP/Managed Devices Map** (the default — the links your monitored devices advertise directly over LLDP and CDP; "managed" devices are the ones Netdata polls over SNMP), **High Confidence Inferred Map** (adds links Netdata infers with strong evidence), or **All Devices (Low Confidence)** (everything seen, including weakly-inferred links).
- **Infer strategy** — how Netdata reconstructs links the devices don't advertise directly, from **FDB Minimum-Knowledge (Baseline)** (the default) through **STP Parent Tree**, **FDB Pairwise Minimum-Knowledge**, **STP + FDB Correlated**, and **CDP + FDB Hybrid**. Different strategies suit different fabrics.
- **Focus on** and **Focus depth** — pick one or more devices as roots and limit the map to a number of hops out from them, to zoom into one part of a large network.

Start with the defaults; reach for the other map types and strategies when a link you expect isn't showing, or when you want the complete picture rather than the high-confidence one.

## What's next

- [Overview](/docs/npm/topology/README.md) — what topology gives you and the other sources it brings in.
- [Device Metrics](/docs/npm/device-metrics/README.md) — monitoring the devices the topology is built from.
