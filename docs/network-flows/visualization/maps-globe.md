<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/network-flows/visualization/maps-globe.md"
sidebar_label: "Maps and Globe"
learn_status: "Published"
learn_rel_path: "Network Flows/Visualization"
keywords: ['country map', 'state map', 'city map', 'globe', 'visualization']
endmeta-->

# Maps and Globe

Four geographic views, all driven by the same aggregation engine as the Sankey and Time-Series:

- **Country map** — countries connected by edges weighted by traffic
- **State map** — same, at state/province granularity
- **City map** — same, at city level (down to street-level granularity, depending on your GeoIP database)
- **Globe** — a 3D view of city-level connections rendered as arcs over the globe

Use these to spot geographic patterns at a glance — unexpected destinations, asymmetric traffic, CDN routing.

![Country map, top 500](https://github.com/user-attachments/assets/f9f09cf2-40c5-4bda-bf56-19b04b6cddf1)

Country map with top-N pushed to 500, so practically every country with traffic shows up. Edge thickness is bandwidth aggregated per country pair.

## How they work

For each map view, the dashboard:

1. Forces a specific aggregation. You don't pick `group_by` for these views — the view picks for you.
2. Runs the aggregation across your time range and filters.
3. Renders the top-N (25/50/100/200/500) results as edges on the map.
4. Same aggregation drives the side-panel list of countries / cities. The list and the map are two views of the same data.

The forced aggregations are:

| View | Forced group-by |
|---|---|
| Country map | `SRC_COUNTRY`, `DST_COUNTRY` |
| State map | `SRC_COUNTRY`, `SRC_GEO_STATE`, `DST_COUNTRY`, `DST_GEO_STATE` |
| City map | `SRC_COUNTRY`, `SRC_GEO_STATE`, `SRC_GEO_CITY`, latitude, longitude (source + destination) |
| Globe | Same as city map |

Edge width is proportional to your sort metric (bytes or packets). The geographic coordinates needed to draw cities and arcs come from the response itself — they're already enriched into each flow record by the time the dashboard renders. You don't need a separate city-coordinates database in the dashboard.

## Country and state vs city / globe

The country map and state map can use the rollup tiers. They're cheap over long time windows.

The city map and the globe **need raw-tier data**. City, latitude, and longitude are dropped from the rollup tiers (1m / 5m / 1h) to keep cardinality manageable. So:

- Country / state map over the last 30 days — fine, uses the 1-hour tier.
- City map over the last 30 days — likely empty. Tier 0 retention defaults to 7 days (shared budget across all tiers); often less in practice.

If your city map looks empty over a long window, try the country map first to confirm data is arriving, then narrow the time range until the city map fills in.

## Tooltips

Hover over a country, state, city, or arc to see a tooltip. The tooltip shows the same fields as the underlying row — endpoints, byte and packet counts. Click does **not** drill down to a different view; the maps are read-only with respect to navigation. To change perspective (e.g., "show me traffic for this country only"), use the filter ribbon to add a `SRC_COUNTRY` or `DST_COUNTRY` selection.

![State map zoomed over the US, hovering an Attica↔California link](https://github.com/user-attachments/assets/6f124a7c-e12f-453f-8599-59e48bc839e8)

State map with top-N at 500, zoomed over the US. The tooltip on the link between Attica (Greece) and California shows bidirectional traffic — bytes and packets in each direction.

![City map zoomed over Europe](https://github.com/user-attachments/assets/e752e1e3-4f6a-4366-b2e2-6af04d4bc2fe)

City map with top-N at 500, zoomed over Europe. Dozens of European cities appear connected by edges weighted by bandwidth.

![Globe view over the Atlantic, US ↔ EU links](https://github.com/user-attachments/assets/c83a963d-797f-44f8-9ae1-e9aba7e16eec)

Globe view, top-N at 500, rotated over the Atlantic. The 3D projection shows US cities and EU cities at the curvy edges, with arcs (bandwidth-thickness) bridging them.

## Things to know

### GeoIP is required

Without a GeoIP database, country / state / city / coordinate fields are empty and the maps are blank. The default install includes a stock DB-IP database — see [GeoIP enrichment](/docs/network-flows/enrichment/ip-intelligence.md). Source builds need the operator to run the downloader once.

### Internal IPs in random countries

If you see "traffic from China" or "traffic to Russia" coming from your own network, that's almost always GeoIP misidentifying internal IPs. The fix is to declare your internal CIDRs explicitly under `enrichment.networks` with a country override. See [Static metadata](/docs/network-flows/enrichment/static-metadata.md). Don't trust GeoIP for RFC 1918 / RFC 6598 / link-local addresses.

### CDN traffic shifts

Your traffic to a SaaS provider may resolve to one country today and another tomorrow because the CDN's routing changed. This is normal CDN behaviour, not a security incident. ASN-based aggregation is more stable for cloud / CDN traffic than country-based — see the [Anti-patterns page](/docs/network-flows/anti-patterns.md) "Geographic firewall of shame".

### Mirroring

Bidirectional conversations show up as two arcs (A→B and B→A). With the default 25 top-N, that means about 12 actual conversations get rendered, not 25. To see one direction only, filter on a specific source or destination.

### Globe vs City Map

The globe and city map use the same data. The globe is purely a different rendering of the same response — useful for visual presentation, less useful for analysis (the 3D projection makes precise reading harder than a 2D map).

## What controls are available

- **Time range** — Netdata's global time picker
- **Filters** — facet selections + autocomplete + full-text search
- **Top-N** — 25 / 50 / 100 / 200 / 500
- **Sort by** — bytes or packets (determines edge weight and the side-list ranking)
- **Group-by** — locked to the view-specific aggregation; not user-configurable for maps

## Things that go wrong

- **City map empty.** Time range exceeds tier-0 retention. Narrow the range, or use country/state map for a wider view.
- **Random countries appearing for internal traffic.** Declare your internal CIDRs in `enrichment.networks`.
- **Ireland or Singapore showing up unexpectedly.** Probably AWS/GCP/Azure shifting CDN routing. ASN-based aggregation is more stable.
- **A whole country disappears.** Your filter excluded it. Check the filter ribbon.
- **No data on globe but city map works.** Both should fail or succeed identically — they consume the same response. If they diverge, that's a dashboard bug worth reporting.

## What's next

- [GeoIP enrichment](/docs/network-flows/enrichment/ip-intelligence.md) — Required for any geographic visualization.
- [Static metadata](/docs/network-flows/enrichment/static-metadata.md) — Declare your internal networks to override GeoIP for RFC 1918.
- [Filters and Facets](/docs/network-flows/visualization/filters-facets.md) — Narrowing geographic views.
- [Anti-patterns](/docs/network-flows/anti-patterns.md) — Why "alert on traffic to country X" is fragile.
