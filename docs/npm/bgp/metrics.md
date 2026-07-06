<!-- markdownlint-disable-file -->

# Metrics and the BGP Peers Function

Netdata collects BGP state per peer from your routers over SNMP, and presents it as live charts plus an interactive table.

## Charts

BGP metrics appear in three groups:

- **`snmp.bgp.peers.*`** — per peer: connection state and availability, established uptime, session transitions and flaps, message traffic, negotiated and configured timers, and the time since the peer's last update.
- **`snmp.bgp.peer_families.*`** — per address family (AFI/SAFI), where the device reports it: connection state, availability, established uptime, traffic, established transitions, update-recency, and timers, plus the **prefix and route metrics** — counts (received, accepted, active, advertised, rejected, suppressed, withdrawn), route totals, and configured route limits. Prefix counts live at this scope, not at the per-peer level, and only on devices that expose per-family route tables.
- **`snmp.bgp.devices.*`** — device-level totals: peer counts and peer-state counts.

The standard BGP4-MIB covers the core per peer — peer state, uptime, transitions, the updates and messages counters, and last-update age. Vendor BGP MIBs add more where the device exposes it: some carry a fuller per-message traffic breakdown (notifications, route-refresh, open, keepalive), and the entire per-address-family scope (`snmp.bgp.peer_families.*`), including the route counts and limits, comes from vendor MIBs. Diagnostic detail — the last error, down reason, and graceful-restart state — is shown in the `snmp:bgp-peers` function rather than as charts.

## The `snmp:bgp-peers` function

Open **`snmp:bgp-peers`** on a router for a sortable, filterable table of every peer — served from already-collected data, with no extra requests to the device. Each row shows the **neighbor and remote AS**, **admin and connection state**, **established uptime**, **last-update age**, **updates sent and received**, **prefixes accepted and advertised**, and the **last error**, **down reason**, and **graceful-restart state**.

Use the **`view`** parameter to choose what each row represents:

- **`peers`** (default) — one row per BGP peer.
- **`peer_families`** — one row per peer per address family (AFI/SAFI), carrying the per-family route counts and limits.
- **`all`** — peers and families together in one table.

Every column sorts and filters, so you can sort by last-update age to surface up-but-stale peers, or filter to a single remote AS.

## Spotting "Established but stale"

The most useful combination is **connection state + last-update age**: a peer that is Established while its last-update age keeps climbing has quietly stopped receiving routes. Sort the table by last-update age to surface these before they bite.

## Alerts

Netdata ships seven stock BGP alerts that raise on their own — route them to your channels as you would any Netdata alert:

- **Peer down** and **peer-family down** — a session that should be up has left **Established**.
- **Transition anomaly**, for peers and for peer families — an unusual rate of established-state flapping, caught by Netdata's ML anomaly detection.
- **Update anomaly**, for peers and for peer families — abnormal update-message churn, a sign of route instability.
- **Accepted-prefix anomaly** — an unexpected change in a peer family's accepted-prefix count, surfacing route loss or a sudden flood.

## What's next

- [Overview](/docs/npm/bgp/README.md) — what BGP monitoring covers and which devices.
