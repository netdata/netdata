<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/network-flows/enrichment/bgp-routing.md"
sidebar_label: "BGP Routing"
learn_status: "Published"
learn_rel_path: "Network Flows/Enrichment Concepts"
keywords: ['bgp', 'routing', 'bmp', 'bioris', 'enrichment', 'concept']
endmeta-->

# BGP Routing

BGP-routing enrichment fills `SRC_AS`, `DST_AS`, `SRC_MASK`, `DST_MASK`, `NEXT_HOP`, `DST_AS_PATH`, `DST_COMMUNITIES`, and `DST_LARGE_COMMUNITIES` from a live BGP feed. Two transports are supported, both of which feed the same in-memory routing trie:

- **BMP** (BGP Monitoring Protocol, RFC 7854) — routers push their BGP updates to Netdata over TCP
- **BioRIS** — Netdata pulls BGP data from a [bio-rd](https://github.com/bio-routing/bio-rd) `cmd/ris/` daemon over gRPC

This page covers the **cross-cutting concept**: how the trie works, how the two sources combine, what survives a restart, and what to expect operationally. For per-protocol setup, follow the integration cards on Learn (BMP and bio-rd / RIPE RIS).

## What gets enriched

BMP and BioRIS populate the same fields. When a flow's source or destination IP matches a learned BGP route:

| Field | Side | Notes |
|---|---|---|
| `SRC_AS` / `DST_AS` | both | When the `routing` provider in `asn_providers` chain reaches BGP data |
| `SRC_MASK` / `DST_MASK` | both | When the `routing` provider in `net_providers` chain reaches BGP data |
| `NEXT_HOP` | dest only | BGP next-hop from the destination route |
| `DST_AS_PATH` | dest only | Full BGP AS path (CSV of ASNs) |
| `DST_COMMUNITIES` | dest only | Standard BGP communities (CSV of u32) |
| `DST_LARGE_COMMUNITIES` | dest only | RFC 8092 large communities |

Two notes:

- AS *names* (`*_AS_NAME`) come from the [GeoIP/ASN MMDB](/docs/network-flows/enrichment/ip-intelligence.md), not BGP. BGP gives you accurate AS *numbers* and path/communities; the names come from the ASN database.
- Source-side AS path and communities are **not** surfaced. BGP path attributes are most meaningful for the destination of the traffic.

## Shared trie

Both BMP and BioRIS populate a single in-memory routing trie keyed by IP prefix. Each prefix entry holds a list of routes (one per `(peer, route_key)` tuple), so multipath BGP and multiple BGP peers contributing the same prefix coexist cleanly.

When both BMP and BioRIS are enabled, they contribute to the same trie. Lookups pick the best-matching route across both sources, preferring routes whose exporter or next-hop matches the flow being enriched, falling back to longest-prefix-match.

This is intentional: a deployment that runs BMP from internal routers and BioRIS for external (RIPE RIS) views gets unified enrichment without duplicate trie entries.

## Memory growth

The trie has **no time-based eviction**. Routes are only removed via:

- Explicit BGP withdrawal (`MP_UNREACH`, `withdraw_routes`)
- Peer Down notification (BMP) — clears all routes for the affected peer
- TCP disconnect (BMP) followed by the `keep` interval expiring (default 5 minutes) — clears all routes for that session
- bio-rd refresh cycle — explicit removal of routes for routers that have disappeared

A full IPv4+IPv6 BGP table is roughly 1.2M prefixes per peer (2026 figures). Each entry stores the AS-path `Vec<u32>`, communities `Vec<u32>`, large communities `Vec<(u32,u32,u32)>`, plus a `route_key` `String` per path. Expect several hundred MB of resident memory per peer for a full feed.

Plan capacity accordingly. If you run many peers with full feeds, watch the agent's RSS.

## Restart behaviour

The trie is **not persisted**. Restarting the netflow plugin wipes BGP-derived data. Routes are re-learned as routers re-send Initiation + Update messages (for BMP) or as bio-rd's next refresh cycle dumps the RIB (BioRIS).

Convergence times after restart:

| Source | Typical convergence |
|---|---|
| FRR over BMP | seconds (FRR re-emits everything immediately) |
| Cisco IOS-XR over BMP | minutes (IOS-XR's initial-refresh has a configurable spread) |
| Juniper JunOS over BMP | seconds to minutes (depends on station options) |
| BioRIS over RIPE RIS | minutes (full DumpRIB takes a while for large feeds) |

Until convergence, BGP-derived enrichment is incomplete. Plan restarts during low-traffic windows if BGP attribution matters for your workflow.

## Provider chain integration

BGP-derived routes contribute to flow enrichment via the `routing` entry in the [ASN resolution](/docs/network-flows/enrichment/asn-resolution.md) provider chain:

```yaml
enrichment:
  asn_providers: [flow, routing, geoip]    # default
  net_providers: [flow, routing]            # default
```

With the defaults, an exporter-supplied AS number wins over BGP. To prefer BGP over the exporter (useful when your BMP/BioRIS feed is more accurate than the exporter's view), reorder:

```yaml
enrichment:
  asn_providers: [routing, flow, geoip]
```

`bmp` is accepted as an alias for `routing` in the provider list, for backward compatibility.

## Integration test gap

The runtime path of both BMP and BioRIS — TCP listener / gRPC client, framed decode loop, trie apply, per-router cleanup — is **not** integration-tested in this repository. The parsing layers (BMP message parsing, gRPC proto conversion) are well-unit-tested, but end-to-end against real router firmware or real bio-rd daemons is not exercised.

Implications:

- The features ship because the parsing is solid and the runtime is built on standard tokio + netgauze + tonic primitives.
- Vendor compatibility (Cisco IOS-XR / IOS-XE, Juniper JunOS, Arista EOS, FRR) is not validated by tests in this repository.
- Treat configuration changes as production-impacting. Validate against your specific gear before relying on BGP-derived data for capacity or security decisions.

## What can go wrong

- **No connections forming (BMP).** Routers initiate BMP sessions to the plugin. Check the router side (`show bmp` / `show bmp connections` / `show bmp targets`). The plugin doesn't proactively retry; it waits.
- **gRPC deadline exceeded (BioRIS).** Default timeout 200 ms is aggressive over the public internet. Raise to 2-5 s.
- **Memory growth without bound.** A full BGP feed is permanent (no eviction). Plan capacity.
- **Plugin restart wipes the trie.** Re-converge takes seconds (FRR) to minutes (IOS-XR). Schedule restarts off-peak.
- **AS path inconsistent with the exporter's view.** Different vantage points see different paths. This is normal in BGP. If your exporter and your BMP-feeding router are different boxes with different routing tables, expect divergence.
- **Empty BGP data after enabling.** Check the per-provider integration card for the specific protocol's setup gotchas — e.g., FRR requires `-M bmp` in `/etc/frr/daemons` (otherwise every BMP command silently fails).

## What's next

- **BMP** integration card — how to enable the listener, configure routers (Cisco, Juniper, Arista, FRR).
- **bio-rd / RIPE RIS** integration card — how to set up bio-rd, configure the gRPC client.
- [ASN resolution](/docs/network-flows/enrichment/asn-resolution.md) — How BGP plugs into the provider chain.
- [Static metadata](/docs/network-flows/enrichment/static-metadata.md) — Per-prefix overrides that win over BGP.
