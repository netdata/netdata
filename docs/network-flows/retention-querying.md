<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/network-flows/retention-querying.md"
sidebar_label: "Retention and Tiers"
learn_status: "Published"
learn_rel_path: "Network Flows"
keywords: ['retention', 'tiers', 'rollup', 'tier selection']
endmeta-->

<!-- markdownlint-disable-file -->

# Retention and Tiers

Netdata stores flow data in four tiers. The tier model is transparent — you do not pick a tier when you query, the dashboard picks for you. Understanding how it picks helps you interpret what you're seeing and avoid surprises when older data isn't there.

For the configuration surface (per-tier `size_of_journal_files` and `duration_of_journal_files`), see [Configuration → Per-tier retention](/docs/network-flows/configuration.md#per-tier-retention). For the query semantics (group-by limits, full-text search, URL sharing, dashboard query parameters), see [Visualization → Overview](/docs/network-flows/visualization/overview.md).

## The four tiers

| Tier | Bucket | On-disk dir | YAML key |
|---|---|---|---|
| Raw | per-flow | `flows/raw/` | `raw` |
| 1-minute | 60 s | `flows/1m/` | `minute_1` |
| 5-minute | 300 s | `flows/5m/` | `minute_5` |
| 1-hour | 3600 s | `flows/1h/` | `hour_1` |

The raw tier stores every flow record as it arrived. The other three are rollup tiers — they aggregate raw flows into time-bucketed groups by identity (exporter, interface, ASN, country/state, network labels, VLAN, next-hop — see below for the full preserved set).

## What survives the rollup

Rollup tiers (1m, 5m, 1h) deliberately drop the high-cardinality and protocol-specific fields and keep an aggregate-friendly subset.

**Forced to the raw tier** (any query that filters on, groups by, or runs full-text search against these fields is rerouted to the raw tier — see [Field Reference](/docs/network-flows/field-reference.md) for the per-field matrix):

- `SRC_ADDR`, `DST_ADDR`, `SRC_PORT`, `DST_PORT`
- `SRC_GEO_CITY`, `DST_GEO_CITY`, `SRC_GEO_LATITUDE`, `DST_GEO_LATITUDE`, `SRC_GEO_LONGITUDE`, `DST_GEO_LONGITUDE`
- All `V9_*` and `IPFIX_*` raw-protocol fields.

**Dropped from rollup output but do not switch tier** (the field comes back as null on rollup tiers; the planner does not reroute the query to raw):

- AS path, BGP community fields (`SRC_COMMUNITIES`, `DST_COMMUNITIES`, etc.), MPLS labels, MAC addresses, NAT / post-NAT addresses, and any other field not in the preserved set below.

If you need any of these fields populated in the result, force the raw tier explicitly (open a city map, add a port filter, type something into the search ribbon, or pick a window that fits inside raw-tier retention).

**Preserved in rollup tiers** (these queries can use coarser tiers):

- Core: `PROTOCOL`, `DIRECTION`, `ETYPE`, `FORWARDING_STATUS`, `FLOW_VERSION`, `IPTOS`, `TCP_FLAGS`, `ICMPV4_TYPE/CODE`, `ICMPV6_TYPE/CODE`, `SRC_AS` / `DST_AS` (ASN number), `SRC_AS_NAME` / `DST_AS_NAME`.
- Exporter: `EXPORTER_IP`, `EXPORTER_PORT`, `EXPORTER_NAME`, `EXPORTER_GROUP/ROLE/SITE/REGION/TENANT`.
- Interface: `IN_IF`, `OUT_IF`, `IN_IF_NAME`/`OUT_IF_NAME`, plus their description / speed / provider / connectivity / boundary variants.
- Network: `SRC_NET_*` / `DST_NET_*` (name / role / site / region / tenant), `SRC_COUNTRY` / `DST_COUNTRY`, `SRC_GEO_STATE` / `DST_GEO_STATE`, `NEXT_HOP`, `SRC_VLAN` / `DST_VLAN`.
- Aggregates: bytes / packets / flow-count sums per bucket.

So rollups are fine for most country / state / ASN / interface / VLAN / protocol questions, but useless if you need to ask "which IP", "which port", "which AS path", "which MPLS label", or "where in the city".

This is why filtering or grouping by IP/port/city/lat/lon forces the query to the raw tier — there is no other tier that has those fields.

For the per-field tier-preservation matrix, see [Field Reference](/docs/network-flows/field-reference.md).

## How the dashboard picks a tier

For every query the dashboard sends to the plugin, the planner makes a single decision: which tier (or tiers) can satisfy this?

**Rules:**

1. **Any raw-only field used as a filter or group-by → raw tier.** No exception. See the "Forced to the raw tier" list above. Selecting the city map or filtering on any IP / port / city / lat / lon field (plus the `V9_*` / `IPFIX_*` raw protocol fields) falls in this category.
2. **A non-empty full-text search → raw tier.** Full-text search runs as a regex against the raw journal payload, which only the raw tier carries.
3. **Otherwise, pick the coarsest tier that satisfies the time range alignment.**
   - **Time-Series view** additionally needs at least 100 buckets in the window. The planner walks the tiers from coarsest to finest and picks the first that delivers ≥100 buckets, falling back to 1-minute when no tier qualifies:
     - ≥ 100 hours of window → 1-hour tier (3600 s buckets)
     - 8h20m to less than 100 hours → 5-minute tier (300 s buckets)
     - 100 minutes to less than 8h20m → 1-minute tier (60 s buckets)
     - Less than 100 minutes → 1-minute tier (Time-Series buckets are still floored at 60 seconds, so very short windows render fewer than 100 buckets)
   - **Table / Sankey / Map** views have no bucket-count constraint; the planner walks 1-hour → 5-minute → 1-minute by alignment alone, so they can land on a coarser tier than Time-Series for the same window.

When the planner picks a tier and the time range crosses tier-aligned boundaries, the query is **stitched** — head fragment in a finer tier, aligned middle in the chosen tier, tail fragment in a finer tier. You don't see this; the results merge cleanly. It exists so wide windows that don't quite align to one-hour boundaries still work.

The plugin reports the chosen tier in the response stats (`query_tier` = `0`, `1`, `5`, or `60`). The dashboard uses this for diagnostic banners.

## What "no data" actually means

If you ask for a 30-day window with an IP filter and raw-tier retention is 24 hours, you get an empty response. No error, no banner reading "data has expired" — just an empty result set. The dashboard renders this as "No data".

The planner does not fall back to a coarser tier for raw-only queries. When a span requires the raw tier (because the query filters or groups on an IP / port / city / lat / lon / V9_* / IPFIX_* field, or runs a full-text search) and that span's raw-tier files have been rotated out, the planner returns no flows for that span. Rollups never carry raw-only fields, so they cannot satisfy the query anyway. Conversely, when a span only needs preserved fields (country, ASN, exporter, interface, protocol…), the planner can fall back from a coarser tier to a finer one if the coarser files have rotated out — finer tiers are supersets of coarser tiers for the preserved fields.

Other spans within the same query that don't need raw data may still return flows. So it's also possible to see partial coverage — half the time range filled, half empty.

For Time-Series, "no data" appears as zero values in the affected buckets, not as a special "missing" indicator. The chart still draws; the empty regions are flat lines at zero.

## What forces the raw tier in practice

Quick reference for "why is my query slow / showing less time?":

- Adding `SRC_ADDR`, `DST_ADDR`, `SRC_PORT`, or `DST_PORT` as a filter
- Adding any of those fields to the group-by
- Switching to the city map (it uses `SRC_GEO_CITY`/`DST_GEO_CITY` plus latitudes/longitudes)
- Typing anything into the global search ribbon

If you see the time depth in your dashboard suddenly shrink after you applied a filter, you've hit the raw-tier limit.

## Default retention and the most common misconfiguration

Each tier has its own `size_of_journal_files` and `duration_of_journal_files`. The built-in defaults are uniform — `10GB` and `7d` on every tier. That is rarely what you want; the whole point of having rollup tiers is to keep them around longer than raw.

A more useful production profile:

```yaml
journal:
  tiers:
    raw:
      size_of_journal_files: 200GB
      duration_of_journal_files: 24h
    minute_1:
      size_of_journal_files: 20GB
      duration_of_journal_files: 14d
    minute_5:
      size_of_journal_files: 20GB
      duration_of_journal_files: 30d
    hour_1:
      size_of_journal_files: 20GB
      duration_of_journal_files: 365d
```

This gives you 24 hours of full-detail forensics, 14 days of 1-minute trends, 30 days of 5-minute snapshots, and a year of hourly aggregates.

See [Configuration → Per-tier retention](/docs/network-flows/configuration.md#per-tier-retention) for the full schema and [Sizing and Capacity Planning](/docs/network-flows/sizing-capacity.md) for how to estimate the actual disk footprint per tier from your flow rate.

## Things that surprise people

- **An IP filter shrinks the time depth.** This is correct behaviour, but the dashboard doesn't always make it obvious. If your time range is wider than raw-tier retention, drop the IP filter to see the broader rollup data.
- **The city map can't go back as far as the country map.** The city map needs the city/lat/lon fields (raw-only); the country map only needs `SRC_COUNTRY`/`DST_COUNTRY` (preserved in rollups).
- **Tier files use short names** (`1m`, `5m`, `1h` on disk) but YAML uses the explicit names (`minute_1`, `minute_5`, `hour_1`). Mind the difference.

## What's next

- [Configuration → Per-tier retention](/docs/network-flows/configuration.md#per-tier-retention) — `netflow.yaml` schema for per-tier retention.
- [Sizing and Capacity Planning](/docs/network-flows/sizing-capacity.md) — Disk and CPU estimates from your flow rate.
- [Field Reference](/docs/network-flows/field-reference.md) — Which fields exist and which survive into rollups.
- [Visualization → Overview](/docs/network-flows/visualization/overview.md) — How the dashboard sends queries; group-by limits; full-text search; URL sharing.
