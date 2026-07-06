<!-- markdownlint-disable-file -->

# BGP Monitoring

Netdata monitors the BGP sessions on your routers — peer state, the prefixes they exchange, and the health of each session — so you see when a peer goes down, flaps, or quietly stops sending updates.

## What Netdata watches

For every BGP peer on a monitored router:

- **Session state** — Established, or where in the lifecycle it sits.
- **Uptime and stability** — how long the session has held, and how often it has flapped.
- **Prefixes** — per address family, the routes exchanged (received, accepted, active, advertised, rejected, suppressed, withdrawn), where the device reports them.
- **Update recency** — how long since the peer last sent an update.
- **Last-down detail** — the last error, down reason, and graceful-restart state, where the vendor MIB reports them.

Session and traffic metrics appear as live charts per peer; the full per-peer detail is in the **`snmp:bgp-peers`** function, a sortable table of every peer.

## Up-but-stale detection

A BGP session can show **Established** while it has quietly stopped receiving routes — the session looks fine, but the routing behind it is stale. Netdata tracks the time since each peer's last update, so an up-but-stale peer (state Established, last-update age climbing) is visible.

## Which devices

Any router that exposes its peers over the standard BGP4-MIB, with vendor BGP profiles for Alcatel-Lucent, Arista, Cisco, Dell, Huawei, Juniper, and Nokia adding per-vendor detail. It comes up automatically with the device's profile.

## Alerts

Stock alerts for BGP health — a peer or address family dropping out of Established, plus ML-based anomaly alerts on session-transition rate, update churn, and accepted-prefix drift.

## Where to start

- BGP comes up automatically when you [monitor a router](/docs/npm/device-metrics/README.md) that runs it — open its charts or the `snmp:bgp-peers` function.
