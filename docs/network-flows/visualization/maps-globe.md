<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/network-flows/visualization/maps-globe.md"
sidebar_label: "Maps and Globe"
learn_status: "Published"
learn_rel_path: "Network Flows/Visualization"
keywords: ['country map', 'state map', 'city map', 'globe', 'visualization']
endmeta-->

<!-- markdownlint-disable-file -->

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
- City map over the last 30 days — likely empty. Raw-tier retention defaults to its own 10GB / 7d limits; busy collectors often hit the raw-tier size cap before 7 days.

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

Without a GeoIP database, country / state / city / coordinate fields are empty and the maps are blank. Native packages include a stock DB-IP database — see the [DB-IP integration card](/src/crates/netflow-plugin/integrations/db-ip_ip_intelligence.md) and the [Enrichment Intel Downloader](/docs/network-flows/intel-downloader.md). Source builds need the operator to run the downloader once.

### CDN traffic shifts

Your traffic to a SaaS provider may resolve to one country today and another tomorrow because the CDN's routing changed. This is normal CDN behaviour, not a security incident. ASN-based aggregation is more stable for cloud / CDN traffic than country-based — see the [Anti-patterns page](/docs/network-flows/anti-patterns.md) "Geographic firewall of shame".

### Bidirectional traffic on the map

Bidirectional traffic between two endpoints produces two separate flow records (one per direction) and renders as two distinct edges (A→B and B→A). The two directions are usually asymmetric in volume — for example, a download is large in one direction and small in the other. To see only one direction, filter on a specific source or destination.

### Globe vs City Map

The globe and city map render the same data with the same table beneath. The 2D city map is best for precise comparisons within a continent. The 3D globe is best when distance and great-circle paths matter — transcontinental traffic, undersea cable corridors, intercontinental CDN routing. Pick the one that fits the question.

## What controls are available

- **Time range** — Netdata's global time picker
- **Filters** — facet selections + autocomplete + full-text search
- **Top-N** — 25 / 50 / 100 / 200 / 500
- **Sort by** — bytes or packets (determines edge weight and the side-list ranking)
- **Group-by** — locked to the view-specific aggregation; not user-configurable for maps

## Things that go wrong

- **City map empty.** Time range exceeds raw-tier retention. Narrow the range, or use country/state map for a wider view.
- **Ireland or Singapore showing up unexpectedly.** Probably AWS/GCP/Azure shifting CDN routing. ASN-based aggregation is more stable.
- **A whole country disappears.** Your filter excluded it. Check the filter ribbon.
- **No data on globe but city map works.** Both should fail or succeed identically — they consume the same response. If they diverge, that's a dashboard bug worth reporting.

## What's next

- [Enrichment](/docs/network-flows/enrichment.md) — Order of evaluation and the MMDB shared mechanism that drives geographic visualisation.
- [DB-IP integration card](/src/crates/netflow-plugin/integrations/db-ip_ip_intelligence.md) — The default GeoIP source that ships with Netdata.
- [Static Metadata integration card](/src/crates/netflow-plugin/integrations/static_metadata.md) — Declare your internal networks to override GeoIP for RFC 1918.
- [Filters and Facets](/docs/network-flows/visualization/filters-facets.md) — Narrowing geographic views.
- [Anti-patterns](/docs/network-flows/anti-patterns.md) — Why "alert on traffic to country X" is fragile.
