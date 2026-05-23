<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/network-flows/README.md"
sidebar_label: "Overview"
learn_status: "Published"
learn_rel_path: "Network Flows"
keywords: ['netflow', 'sflow', 'ipfix', 'network flows', 'traffic analysis', 'overview']
endmeta-->

<!-- markdownlint-disable-file -->

# Network Flows

Netdata can collect, store, and visualise network flow data from your routers and switches. You see who is talking to whom on your network, how much data they exchanged, over what protocols, and to which countries — without inspecting packet contents and without an external database.

This section is for network engineers, security analysts, and IT managers who want to understand what's happening on the wire. The same dashboard answers all three audiences from the same data.

## What flow data is

A network flow is a *summary of a conversation*. Routers and switches watch packets as they pass through, group them by source IP, destination IP, source port, destination port, and protocol, and produce one record per flow when the conversation ends or after a timeout. That record contains:

- The endpoints (IPs, ports, protocol)
- How much data moved (bytes, packets)
- When the flow started and ended
- Optional metadata: AS numbers, interface indexes, TCP flags, ToS, MAC addresses, VLAN IDs

Think of flow data like an itemised phone bill. You can see who called whom, when, and for how long. You **cannot** read the conversation. That trade-off is the entire value proposition: low storage, no privacy intrusion, complete coverage of all traffic — but no payload visibility.

## What you can answer

- Who is using the most bandwidth right now? Last week? Last month?
- Where does our traffic go — by country, ASN, port, or protocol?
- A specific IP appeared in a security alert — what did it talk to, when, and how much?
- Is this a normal pattern for this time of day, or has something changed?
- Should we upgrade this Internet link, and when?
- Which interfaces of which routers are saturated?

## What you cannot answer

- **Is the application slow?** Flow data has no payload, no response times, no error messages. Use APM or application logs.
- **What's the latency?** Flow records show duration, not round-trip time. Duration is dominated by timeout configuration, not network performance. Use ICMP probes or hardware telemetry.
- **What did the user actually do?** Flow data sees ports and IPs, not user actions or URLs.
- **Did this packet arrive late?** Flow data is aggregated; sub-second jitter and microbursts are invisible.

If those are your questions, flow data is the wrong tool. You probably need application performance monitoring (APM), logging, or packet capture.

## Two things to know on day one

These two facts are not Netdata-specific. They're how flow data works on every collector. Understanding them up-front saves a lot of head-scratching when you first see the dashboard.

### Traffic can appear doubled

When a router exports flow records for both ingress and egress on every monitored interface — a common configuration — a single packet entering interface A and leaving interface B produces two records: one tagged ingress on A, one tagged egress on B. With a second router on the same path doing the same thing, **4×**. Vendor best practice is to enable ingress-only exports to avoid this entirely; if you can't change exporter configuration, the dashboard view still has to compensate.

To see real numbers, filter by one exporter and one interface. Each packet then appears in exactly one record on that interface. See the [Anti-patterns page](/docs/network-flows/anti-patterns.md) for the full framing.

### Bidirectional traffic shows both directions

A conversation between host A and host B has packets going both ways: A→B for requests or uploads, B→A for responses or downloads. These are real, separate packets, exported as separate flow records. The Sankey diagram, country map, and sorted top-N tables show both directions when you don't filter by direction.

Volumes in the two directions are usually asymmetric — for example, a video download produces large B→A flows and small A→B ACKs. So a "host A to host B" entry and a "host B to host A" entry refer to the same conversation but typically have very different byte counts.

This is not duplication; it's correct per-direction accounting.

## What ships with the plugin

The Netdata netflow plugin decodes:

- **NetFlow v5** (legacy, IPv4-only)
- **NetFlow v7** (rare, Cisco Catalyst 5000)
- **NetFlow v9** (the modern Cisco / Juniper / FortiGate / Arista format)
- **IPFIX** (RFC 7011, the IETF-standardised successor to NetFlow v9)
- **sFlow v5** (the packet-sampling protocol most switches use)

A single UDP listener (default `0.0.0.0:2055`) accepts all five. The plugin auto-detects each datagram's protocol from its header.

Each flow record is enriched at ingestion with:

- **Country, state, city, coordinates, ASN, AS name** — from a stock GeoIP database (DB-IP-based; refreshable)
- **Exporter name and labels** — from your static-metadata configuration
- **Interface name, description, speed, provider, connectivity, boundary** — from your static-metadata configuration
- **Network labels** for your own CIDRs (name, role, site, region, tenant)
- **Classifier-derived attributes** for rule-based tagging (Akvorado-compatible subset of the expression language)
- **Live BGP attributes** (AS path, communities, next-hop) — from BMP, BioRIS, or static prefix configuration
- **Decapsulated inner-packet fields** for SRv6 / VXLAN traffic

Flow records land in a four-tier journal: raw + 1-minute + 5-minute + 1-hour rollups, with independent retention per tier. Rollup tiers drop a few high-cardinality fields (IPs, ports, city/coordinates) to stay compact, so any query that filters or groups by those fields is served from the raw tier; everything else can use a coarser tier. The dashboard auto-picks the best tier for each query.

## What sampling does to your numbers

Many routers sample. They export one packet in N — typically 1-in-100 to 1-in-2000. Each flow record carries its own sampling rate (in the record itself, in the protocol header, in a Sampling Options Template, or — for sFlow — in the flow sample). At ingestion, Netdata multiplies that record's bytes and packets by **its own rate**, so dashboard numbers are estimates of actual traffic.

The multiplication is per-flow, so different exporters and different interfaces can sample at different rates and aggregates remain accurate. The dashboard does not surface the sampling rate as a UI element — with mixed rates a single displayed value would be meaningless, and with uniform rates the operator already knows it.

Sampling has a real statistical limit, separate from the multiplication. At 1-in-1000, a single-packet flow has roughly a 99.9% chance of not being sampled at all. If you need to catch small, rare events (security beaconing, scanning), pick a lower sampling rate on the exporters that watch that traffic, or run unsampled there.

For the exact pre-multiplication counts the exporter literally reported, see `RAW_BYTES` and `RAW_PACKETS` in the [field reference](/docs/network-flows/field-reference.md).

## What the dashboard looks like

Six visualisations, all driven by the same query engine:

- **Sankey + Table** — the default. Top-N flows aggregated by 1-10 fields you pick. Best for "who's responsible".
- **Time-Series** — the same top-N over time. Best for "how does this change".
- **Country map / state map / city map** — geographic views. Best for "where".
- **Globe** — a 3D rendering of the city-level data. Same data and table as the city map; useful when distance and great-circle paths matter (e.g. transcontinental traffic).

A filter ribbon between the visualisation and the table lets you narrow data by any combination of fields. Selections persist in the URL — copy and share to give a colleague exactly your view.

Default settings on first open: last 15 minutes, top-25 flows by bytes, grouped as `Source AS Name → Protocol → Destination AS Name`.

Default fields are tuned to surface meaningful traffic at a glance. From there, you adjust the time range, change the aggregation, add filters, and dig in.

## Where to start

Pick the page that matches your situation:

- **You're setting up the plugin for the first time** — [Installation](/docs/network-flows/installation.md), then [Quick Start](/docs/network-flows/quick-start.md).
- **You have data, you want to find a bandwidth hog or trace an IP** — [Investigation Playbooks](/docs/network-flows/investigation-playbooks.md).
- **You want to make sure your data is trustworthy** — [Validation and Data Quality](/docs/network-flows/validation.md).
- **You want to avoid the most common mistakes** — [Anti-patterns](/docs/network-flows/anti-patterns.md).
