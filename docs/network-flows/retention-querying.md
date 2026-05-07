<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/network-flows/retention-querying.md"
sidebar_label: "Retention and Querying"
learn_status: "Published"
learn_rel_path: "Network Flows"
keywords: ['retention', 'tiers', 'querying', 'tier selection', 'rollup']
endmeta-->

# Retention and Querying

Netdata stores flow data in four tiers. The tier model is transparent — you do not pick a tier when you query, the dashboard picks for you. Understanding how it picks helps you interpret what you're seeing and avoid surprises when older data isn't there.

## The four tiers

| Tier | Bucket | On-disk dir | YAML key |
|---|---|---|---|
| Raw | per-flow | `flows/raw/` | `raw` |
| 1-minute | 60 s | `flows/1m/` | `minute_1` |
| 5-minute | 300 s | `flows/5m/` | `minute_5` |
| 1-hour | 3600 s | `flows/1h/` | `hour_1` |

The raw tier stores every flow record as it arrived. The other three are rollup tiers — they aggregate raw flows into time-bucketed groups by identity (exporter, ASN, country, ports — see below).

## What survives the rollup

Rollup tiers (1m, 5m, 1h) deliberately drop a few fields to keep cardinality manageable. **The dropped fields are: `SRC_ADDR`, `DST_ADDR`, `SRC_PORT`, `DST_PORT`, `SRC_GEO_CITY`, `DST_GEO_CITY`, `SRC_GEO_LATITUDE`, `DST_GEO_LATITUDE`, `SRC_GEO_LONGITUDE`, `DST_GEO_LONGITUDE`.**

Everything else survives — country, state, ASN, AS path, BGP communities, exporter and interface labels, protocol, TCP flags, ToS/DSCP, ICMP type/code, MPLS labels, VLANs, MACs, next-hop, post-NAT addresses, and the bytes/packets sums. So rollups are perfectly fine for most country / ASN / interface / protocol questions, but useless if you need to ask "which IP".

This is why filtering or grouping by IP/port/city/lat/lon forces the query to the raw tier — there is no other tier that has those fields.

## How the dashboard picks a tier

For every query the dashboard sends to the plugin, the planner makes a single decision: which tier (or tiers) can satisfy this?

**Rules:**

1. **Any IP/port/city/lat/lon filter or group-by → raw tier.** No exception. The rollup tiers don't have those fields.
2. **A non-empty full-text search → raw tier.** Full-text search runs as a regex against the raw journal payload, which only the raw tier carries.
3. **Otherwise, pick the coarsest tier that satisfies the time range and bucket-count requirement.**
   - Time-Series view needs at least 100 buckets in the window. So:
     - under 100 minutes → 1-minute tier
     - 100 minutes to 8h20m → 5-minute tier
     - 8h20m and longer → 1-hour tier
   - Table / Sankey / Maps don't have a bucket-count constraint, but the configured query-window guardrails (`query_1m_max_window` default 6h, `query_5m_max_window` default 24h) skip a tier when the window is too wide.

When the planner picks a tier and the time range crosses tier-aligned boundaries, the query is **stitched** — head fragment in a finer tier, aligned middle in the chosen tier, tail fragment in a finer tier. You don't see this; the results merge cleanly. It exists so wide windows that don't quite align to one-hour boundaries still work.

The plugin reports the chosen tier in the response stats (`query_tier` = `0`, `1`, `5`, or `60`). The dashboard uses this for diagnostic banners.

## What "no data" actually means

If you ask for a 30-day window with an IP filter and tier-0 retention is 24 hours, you get an empty response. No error, no banner reading "data has expired" — just an empty result set. The dashboard renders this as "No data".

The reason is a layered fallback in the planner: if a span asks for tier 0 and the files for that span have been rotated out, the planner tries the smaller tiers (1m, 5m, 1h), but those don't have IP fields, so they cannot satisfy a query that filters on IP. Result: the span returns no flows.

Other spans within the same query that don't need raw data may still return flows. So it's also possible to see partial coverage — half the time range filled, half empty.

For Time-Series, "no data" appears as zero values in the affected buckets, not as a special "missing" indicator. The chart still draws; the empty regions are flat lines at zero.

## What forces tier 0 in practice

Quick reference for "why is my query slow / showing less time?":

- Adding `SRC_ADDR`, `DST_ADDR`, `SRC_PORT`, or `DST_PORT` as a filter
- Adding any of those fields to the group-by
- Switching to the city map (it uses `SRC_GEO_CITY`/`DST_GEO_CITY` plus latitudes/longitudes)
- Typing anything into the global search ribbon

If you see the time depth in your dashboard suddenly shrink after you applied a filter, you've hit the raw-tier limit.

## Default retention and the most common misconfiguration

The default `size_of_journal_files: 10GB` and `duration_of_journal_files: 7d` apply to **every tier independently**. With defaults, all four tiers (raw, 1m, 5m, 1h) are capped at 10GB / 7d.

This is rarely what you want. The whole point of having rollup tiers is to keep them around longer than raw. A more useful production profile:

```yaml
journal:
  size_of_journal_files: 100GB     # top-level inherited by tiers without an override
  duration_of_journal_files: 7d
  tiers:
    raw:
      size_of_journal_files: 200GB
      duration_of_journal_files: 24h
    minute_1:
      duration_of_journal_files: 14d
    minute_5:
      duration_of_journal_files: 30d
    hour_1:
      duration_of_journal_files: 365d
      size_of_journal_files: null   # time-only, no size cap on the long tail
```

This gives you 24 hours of full-detail forensics, 14 days of 1-minute trends, 30 days of 5-minute snapshots, and a year of hourly aggregates.

See [Sizing and Capacity Planning](/docs/network-flows/sizing-capacity.md) for how to estimate the actual disk footprint per tier from your flow rate.

## How queries work, briefly

The dashboard sends one of two query modes to the plugin:

- **`flows`** — the normal aggregation request. Returns top-N groups, sums of bytes and packets, optional facet counts.
- **`autocomplete`** — for the filter ribbon. Returns up to 100 facet values matching the user's term. Matching policy is per-field: text fields use substring matching, IP and numeric fields use prefix. Term is capped at 256 bytes. Runs against in-memory facet snapshots and on-disk FST sidecars; never scans tier files. Resulting filters apply as exact equality, not substring.

A `flows` query carries:

- A time range (`after` / `before`, or `last`).
- A list of `group_by` fields (up to 10).
- A list of `selections` — per-field IN-lists for filtering.
- Optional `facets` to enrich the response with per-facet value counts.
- A `top_n` (one of 25, 50, 100, 200, 500).
- A `sort_by` (`bytes` or `packets`).
- An optional regex `query` (full-text search; forces tier 0).
- A `view` (`table-sankey`, `timeseries`, `country-map`, `state-map`, `city-map`).

Defaults if you don't specify: time range = last 15 minutes, `group_by = ["SRC_AS_NAME", "PROTOCOL", "DST_AS_NAME"]`, `top_n = 25`, `sort_by = bytes`, `view = table-sankey`.

The plugin enforces a hard timeout of **30 seconds** per query. If your query is too wide, narrow the time range, add a filter that lets a higher tier serve it, or reduce the group-by depth.

## Group-by limits and overflow

Two configuration limits guard against pathological queries:

- `query_max_groups` (default `50000`) — total distinct groups in an aggregation. Past this, results overflow into a single `__overflow__` bucket and the response carries a warning.
- `query_facet_max_values_per_field` (default `5000`) — distinct values returned per facet field.

If you see `__overflow__` rows, your query is too wide for the current limit. Either narrow the filter, drop a high-cardinality `group_by` field, or raise the limit (carefully — the limit exists for memory reasons).

## Full-text search

The global search ribbon supports full-text search. It runs as a **regex** match against the raw journal payload. A search of `8.8.8.8` is the regex `8.8.8.8`, where each `.` matches any byte — so it can match unrelated text. To match the literal string, escape with backslashes: `8\.8\.8\.8`.

Any non-empty full-text search forces the query to tier 0. Time depth is therefore limited by raw-tier retention.

## URL sharing

The dashboard URL preserves all of: time range, view, top-N, sort, group-by, selections, full-text search. Copy the URL and share it — the recipient sees exactly what you see, provided they have access to the same Netdata Cloud space.

## Things that surprise people

- **An IP filter shrinks the time depth.** This is correct behaviour, but the dashboard doesn't always make it obvious. If your time range is wider than tier-0 retention, drop the IP filter to see the broader rollup data.
- **The city map can't go back as far as the country map.** The city map needs the city/lat/lon fields (raw-only); the country map only needs `SRC_COUNTRY`/`DST_COUNTRY` (preserved in rollups).
- **`__overflow__` is a real value.** It will show up in result tables, sankey diagrams, and group-by listings. It means "everything that didn't fit in the top groups for this query" — narrow the filter or raise the limit.
- **30-second timeout is hard.** A query that runs to the timeout returns whatever it has so far with a warning. Don't expect more than 30s of work per query.
- **Tier files use short names** (`1m`, `5m`, `1h` on disk) but YAML uses the explicit names (`minute_1`, `minute_5`, `hour_1`). Mind the difference.

## What's next

- [Configuration](/docs/network-flows/configuration.md) — `netflow.yaml` reference, including per-tier retention overrides.
- [Sizing and Capacity Planning](/docs/network-flows/sizing-capacity.md) — Disk and CPU estimates from your flow rate.
- [Field Reference](/docs/network-flows/field-reference.md) — Which fields exist and which survive into rollups.
- [Visualisation](/docs/network-flows/visualization/summary-sankey.md) — How the dashboard uses the tier model to render views.
