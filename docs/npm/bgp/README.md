<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/npm/bgp/README.md"
sidebar_label: "Overview"
learn_status: "Published"
learn_rel_path: "Network Performance Monitoring/BGP Monitoring"
keywords: ['bgp', 'bgp4-mib', 'peering', 'routing', 'snmp', 'overview']
endmeta-->

<!-- markdownlint-disable-file -->

# BGP Monitoring

Netdata monitors the BGP sessions on your routers — peer state, the prefixes they exchange, and the health of each session — so you can see when a peer goes down, flaps, or quietly stops sending updates.

This section is for the teams running BGP at their edge or between sites who want clear operational visibility into their peers: is the session up, is it stable, is it still carrying routes.

## What Netdata watches

For every BGP peer on a monitored router:

- **Session state** — Established, or where in the lifecycle it sits.
- **Uptime and stability** — how long the session has held, and how often it has flapped.
- **Prefixes** — per address family, the routes exchanged (received, accepted, active, advertised, rejected, suppressed, withdrawn), where the device reports them.
- **Update recency** — how long since the peer last sent an update.
- **Why it last went down** — the last error, plus the down reason and graceful-restart state where the vendor MIB reports them.

The session and traffic metrics appear as live charts per peer; the full per-peer detail — including the last error, down reason, and graceful-restart state — is in the **`snmp:bgp-peers`** function, a sortable table of every peer.

## The check that catches silent trouble

A BGP session can show **Established** while it has quietly stopped receiving routes — the session looks fine, but the routing behind it is stale. Netdata tracks the **time since each peer's last update**, so you can spot a peer that's up-but-stale: state Established, last-update age climbing. Watch the two together and you catch the failure that peer-state alone hides.

## Which devices

BGP monitoring works on any router that exposes its peers over SNMP through the standard BGP4-MIB, with vendor BGP MIB profiles for Alcatel-Lucent, Arista, Cisco, Dell, Huawei, Juniper, and Nokia adding per-vendor detail. It comes up automatically with the device's profile.

## Alerts

Netdata ships stock alerts for BGP health — a peer or address family dropping out of Established, plus ML-based anomaly alerts on session-transition rate, update churn, and accepted-prefix drift — raised automatically without you writing one.

## Where to start

- BGP comes up automatically when you [monitor a router](/docs/npm/device-metrics/README.md) that runs it — open its charts or the `snmp:bgp-peers` function.
- Any router exposing the BGP4-MIB is covered; the per-vendor entries add platform-specific detail.
