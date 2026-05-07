<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/network-flows/enrichment/asn-resolution.md"
sidebar_label: "ASN Resolution"
learn_status: "Published"
learn_rel_path: "Network Flows/Enrichment"
keywords: ['asn', 'as resolution', 'bgp', 'enrichment']
endmeta-->

# ASN resolution

ASN resolution is how Netdata fills in the `SRC_AS`, `DST_AS`, `SRC_AS_NAME`, and `DST_AS_NAME` fields on every flow record. It uses a configurable provider chain — your flow data first, then dynamic routing if you have BMP/BioRIS/static prefixes set up, then the GeoIP database as a fallback. The chain runs independently for source and destination IPs.

## Numbers vs names

The two are resolved by completely different code paths.

**AS numbers** (`SRC_AS`, `DST_AS`) come from the **provider chain** (`asn_providers`). The chain walks providers in order; the first one that returns a non-zero AS wins.

**AS names** (`SRC_AS_NAME`, `DST_AS_NAME`) always come from the **ASN database** lookup, regardless of the chain. Whichever AS number ends up resolved, the name is rendered as `AS{n} {organisation}` from the ASN MMDB. If the MMDB doesn't know the name, the rendering is `AS{n}` with no trailing label. If the resolved AS is `0`, the rendering is `AS0 Unknown ASN` (or `AS0 Private IP Address Space` if the MMDB tagged the IP as private).

There is **no static configuration option** for AS names. You can set `enrichment.networks.<cidr>.asn` to override the AS *number*, but the name is always looked up.

## The provider chain

```yaml
enrichment:
  asn_providers: [flow, routing, geoip]
```

This is the default. The plugin walks it left-to-right; the first provider returning a non-zero value wins.

| Provider | What it reads | Notes |
|---|---|---|
| `flow` | `SRC_AS` / `DST_AS` from the flow record itself | What the exporter sent |
| `flow_except_private` | Same, but treats private/reserved AS numbers as zero | Use when your exporters announce private AS that you don't want to surface |
| `flow_except_default_route` | Same, but treats AS 0 with mask 0 as zero | Use when default-route flows pollute your top-N |
| `routing` | `lookup_routing()` — BMP runtime, BioRIS, static prefixes | Requires routing enrichment to be configured |
| `routing_except_private` | Same as `routing`, with the private filter | |
| `geoip` | **Always returns 0** — terminal short-circuit | See below |

`geoip` is **terminal**: putting it in the chain stops the chain at that position. Once reached, the chain returns 0, and the GeoIP MMDB's AS data is then re-applied separately (without going through the chain). Because of that, `geoip` is meaningful only as the last entry in the chain — it acts as "let GeoIP fill in if nothing else did".

If you put `[geoip, flow, routing]`, you effectively set every AS to 0, and only the GeoIP-derived AS makes it through. That is rarely what you want.

### Common chain configurations

| Configuration | Behaviour |
|---|---|
| `[flow, routing, geoip]` (default) | Trust the exporter, fall back to routing, then to GeoIP. |
| `[flow, routing]` | No GeoIP at all. Use when you don't trust GeoIP for your traffic mix. |
| `[routing, flow, geoip]` | Trust your BMP/BGP feed first. Use when your routers report stale AS. |
| `[flow_except_private, routing, geoip]` | Drop AS 64512-65534 from flow data; let routing fill in. |

### What counts as private/reserved

`is_private_as` returns true for:

- `0` (unknown / default route)
- `23456` (RFC 4893 transition-period reserved)
- `64496..=65551` (documentation, RFC 6996/RFC 5398/RFC 6793 private/reserved range)
- `>= 4_200_000_000` (32-bit private range and reserved high values)

These are filtered out by the `*_except_private` variants.

## The network-prefix chain

A second chain controls how `SRC_MASK`, `DST_MASK`, and `NEXT_HOP` get resolved:

```yaml
enrichment:
  net_providers: [flow, routing]
```

This is the default. Same logic: first non-empty value wins. Only `flow` and `routing` are valid here — there is no `geoip` provider for network attributes.

## AS overrides via static configuration

If a flow's source or destination IP falls inside a CIDR you've declared under `enrichment.networks`, and that entry includes an `asn` field, the configured value **overrides whatever the chain produced**:

```yaml
enrichment:
  networks:
    198.51.100.0/24:
      name: customer-acme
      asn: 64500          # forces SRC_AS / DST_AS = 64500 for traffic in this prefix
```

This override is applied after the chain. It only sets the AS number — the name is still resolved from the ASN database (so it'll render as `AS64500` if your MMDB doesn't have a name, or `AS64500 Acme Corp` if it does).

## What you get out of the box

With the default `[flow, routing, geoip]` chain, no routing enrichment configured, and the stock ASN MMDB shipped with native packages:

- `SRC_AS` / `DST_AS` populated whenever the exporter sends them (most NetFlow v9, IPFIX, sFlow exporters do for public IPs)
- `SRC_AS_NAME` / `DST_AS_NAME` populated whenever the IP is in the ASN MMDB
- For internal RFC 1918 addresses: `*_AS = 0`, `*_AS_NAME = AS0 Private IP Address Space` (because the stock MMDB tags private ranges with that flag)
- For unknown public addresses: `*_AS = 0`, `*_AS_NAME = AS0 Unknown ASN`

If you don't have an ASN MMDB at all, names render as `AS{n}` for non-zero ASNs and `AS0 Unknown ASN` for zero — the dashboard never shows blank cells.

## Failure modes

- **ASN MMDB missing.** With `optional: true` (the default for auto-detected files), the plugin starts and AS names render as `AS{n}` or `AS0 Unknown ASN`. With `optional: false` and a configured path, the plugin fails to start.
- **AS not in any provider.** `*_AS = 0`, `*_AS_NAME = AS0 Unknown ASN`.
- **Wrong order of providers.** Putting `geoip` mid-chain truncates everything after it. Putting `routing` before `flow` makes routing data win over what the exporter sent — fine if your BGP feed is more accurate than your exporter's view.
- **Empty `asn_providers`.** No validation rejects this. The plugin starts but every AS number resolves to 0; only `enrichment.networks.<cidr>.asn` overrides can produce non-zero AS.

## What can go wrong

- **AS numbers all zero.** Check the chain. If `[geoip, ...]` is the order, `geoip` short-circuits to 0. Reorder to put `geoip` last.
- **Wrong AS for a known prefix.** Likely the exporter's view differs from the BGP table. Override per-prefix via `enrichment.networks.<cidr>.asn`, or reorder the chain to `[routing, flow, geoip]`.
- **Names show `AS{n}` without an organisation.** The MMDB doesn't have a name for that AS. Either accept it or use a richer MMDB.
- **Names show wrong organisation.** ASN ownership data is best-effort and lags real-world transfers by weeks. Refresh the MMDB. If that doesn't help, file an issue with the database vendor — Netdata is a passive consumer.

## What's next

- [GeoIP](/docs/network-flows/enrichment/ip-intelligence.md) — How the ASN MMDB gets installed and refreshed.
- [Static metadata](/docs/network-flows/enrichment/static-metadata.md) — Per-prefix AS overrides and network labels.
- [BMP routing](/docs/network-flows/enrichment/bgp-routing.md) — Live BGP feed as an AS source for the `routing` provider.
- [BioRIS](/docs/network-flows/enrichment/bgp-routing.md) — RIPE RIS as an AS source for the `routing` provider.
- [Network sources](/docs/network-flows/enrichment/network-identity.md) — HTTP-fetched prefix metadata.
