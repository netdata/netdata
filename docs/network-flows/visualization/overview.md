<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/network-flows/visualization/overview.md"
sidebar_label: "Overview"
learn_status: "Published"
learn_rel_path: "Network Flows/Visualization"
keywords: ['visualization', 'overview', 'queries', 'fts', 'url sharing', 'group-by']
endmeta-->

<!-- markdownlint-disable-file -->

# Visualization

The Network Flows view exposes the same query engine through five panel types: Sankey, Table, Time-Series, maps (country / state / city), and the 3D globe. The panels share their inputs (filters, group-by, time range, top-N) and their constraints (limits, timeout, FTS rules). This page documents what's common across them; each panel has its own page for the panel-specific reading.

## How queries work

The dashboard sends one of two query modes to the plugin:

- **`flows`** — the normal aggregation request. Returns top-N groups, sums of bytes and packets, optional facet counts.
- **`autocomplete`** — for the filter ribbon. Returns up to 100 facet values matching the user's term. Matching policy is per-field: text fields use substring matching, IP and numeric fields use prefix. Term is capped at 256 bytes. Runs against in-memory facet snapshots and on-disk FST sidecars; never scans tier files. Resulting filters apply as exact equality, not substring.

A `flows` query carries:

- A time range (`after` / `before`). If you omit both, the plugin uses the last 15 minutes.
- A list of `group_by` fields (up to 10).
- A list of `selections` — per-field IN-lists for filtering.
- Optional `facets` to enrich the response with per-facet value counts.
- A `top_n` (one of 25, 50, 100, 200, 500).
- A `sort_by` (`bytes` or `packets`).
- An optional regex `query` (full-text search; forces the raw tier).
- A `view` (`table-sankey`, `timeseries`, `country-map`, `state-map`, `city-map`).

Defaults if you don't specify: time range = last 15 minutes, `group_by = ["SRC_AS_NAME", "PROTOCOL", "DST_AS_NAME"]`, `top_n = 25`, `sort_by = bytes`, `view = table-sankey`.

The plugin enforces a hard timeout of **30 seconds** per query. If your query is too wide, narrow the time range, add a filter that lets a coarser tier serve it, or reduce the group-by depth.

## Group-by limit and overflow

`query_max_groups` (default `50000`) caps the total number of distinct group keys an aggregation can build. Past this, additional groups are folded into a synthetic `__overflow__` bucket and the response carries a warning. The limit exists to protect the query worker from accidentally wide group-by combinations exhausting memory.

If you see `__overflow__` rows, the query is too wide for the current limit. Narrow the filter, drop a high-cardinality `group_by` field, or raise the limit (carefully).

## Full-text search

The search box at the top of the filter ribbon performs a regex match against the raw journal payload bytes. Three things to know:

- The search is **regex**, not literal. `8.8.8.8` is a regex where each `.` matches any byte — so it can match `8a8b8c8`, `888x888`, etc. To match the literal string, escape with backslashes: `8\.8\.8\.8`.
- The match is **byte-level** against the journal payload, so it can find substrings inside enriched fields (AS names, exporter names, country codes).
- Any non-empty search forces the query to the **raw tier**. Time depth is therefore limited by raw-tier retention.

Use full-text search for the cases where you don't have an indexed handle for what you're looking for. For everything else, the filter ribbon (which uses indexed fields and is much faster) is the right tool.

## URL sharing

The dashboard URL preserves all of: time range, view, top-N, sort, group-by, selections, full-text search. Copy the URL and share it — the recipient sees exactly what you see, provided they have access to the same Netdata Cloud space.

The dashboard also remembers your last selections per session, so subsequent visits land on whatever you had open last time.

## Filtering

Filtering uses a structured representation (per-field IN-lists) that's easy to encode as a JSON payload but awkward to URL-encode by hand. The dashboard handles this transparently for sharing — if you script your own queries against the function, use JSON-payload requests, not GET-style args. See [Filters and Facets](/docs/network-flows/visualization/filters-facets.md) for the details.

## Picking the right view

Each panel suits a different question:

- **[Sankey + Table](/docs/network-flows/visualization/summary-sankey.md)** — "Who's responsible for this traffic right now". Default landing view.
- **[Time-Series](/docs/network-flows/visualization/time-series.md)** — "How does this change over time". Same top-N as the Sankey, plotted across the window.
- **[Maps and Globe](/docs/network-flows/visualization/maps-globe.md)** — "Where is this traffic going". Country / state / city / 3D globe variants.
- **[Filters and Facets](/docs/network-flows/visualization/filters-facets.md)** — Narrowing the data; the same controls all panels share.
- **[Plugin Health Charts](/docs/network-flows/visualization/dashboard-cards.md)** — Operational metrics for the plugin itself, not the flow data. Open these first when something looks wrong.

## What's next

- [Sankey and Table](/docs/network-flows/visualization/summary-sankey.md) — Default landing view.
- [Filters and Facets](/docs/network-flows/visualization/filters-facets.md) — Narrowing controls.
- [Retention and Tiers](/docs/network-flows/retention-querying.md) — How the four-tier model determines which queries can satisfy which time ranges.
- [Configuration](/docs/network-flows/configuration.md) — Tuning the query guardrails (`query_max_groups`).
