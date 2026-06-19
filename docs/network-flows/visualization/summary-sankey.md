<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/network-flows/visualization/summary-sankey.md"
sidebar_label: "Sankey and Table"
learn_status: "Published"
learn_rel_path: "Network Flows/Visualization"
keywords: ['sankey', 'table', 'top-n', 'aggregation', 'visualization']
endmeta-->

<!-- markdownlint-disable-file -->

# Sankey and Table

The Network Flows view (open the **Live** tab in the top navigation, then select **Network Flows**) opens with two panels stacked: a Sankey diagram on top, a sortable table beneath. Both render the same data, the same top-N aggregation, the same field selection. Selecting fields, filtering, and sorting affects both at once.

This is your default view. Most investigative workflows start here.

![Sankey top 25 with table, filtered by source IP](https://github.com/user-attachments/assets/314e7195-3fff-4bc7-a01a-0d59ded513d7)

Sankey + table side by side, with a `SRC_ADDR` filter applied. The table at the bottom shows the same 25 rows as the diagram above; clicking a column header re-sorts both. The filter ribbon at the top is how you narrow to one source.

## What you see by default

When you first open the tab:

- **Time range**: last 15 minutes (Netdata's global time picker default — it applies to metrics, logs, flows, and topology together)
- **View**: Sankey + Table
- **Top-N**: 25 (selectable: 25 / 50 / 100 / 200 / 500)
- **Sort by**: bytes (alternative: packets)
- **Aggregation fields**: `Source AS Name → Protocol → Destination AS Name`
- **No filters applied** — the dashboard remembers your last selections, so on subsequent visits you'll land on whatever you had open

The Sankey shows the top 25 conversations between Source AS Name, Protocol, and Destination AS Name, weighted by bytes. The table below shows the same 25 rows, with bytes and packets columns appended. The dashboard groups by AS Name strings (the human-readable form) by default; the bare ASN-number field is also available if you prefer.

## How to read the Sankey

A Sankey diagram has columns of nodes and weighted bands flowing between them.

- Each **column** corresponds to one of your selected aggregation fields, in the order you specified.
- Each **node** is one distinct value in that column (e.g., one ASN, or one country).
- Each **band** is one row in the underlying top-N — its width is proportional to the bytes (or packets) for that combination.

With the default 3-column setup (Source AS Name → Protocol → Destination AS Name), you see the top 25 (Source AS Name, Protocol, Destination AS Name) tuples by traffic volume. A wide band from `AS65000 ACME-CORP` to `tcp` to `AS15169 GOOGLE` says "ACME-CORP sent a lot of TCP to GOOGLE in this time window".

You can pick **1 to 10 fields** as columns. Order matters — the Sankey draws bands left-to-right in the order you list. There are roughly 84-85 fields available for aggregation; metric fields (`BYTES`, `PACKETS`, sampling rate, timestamps) and the geo coordinates (latitude/longitude) are not selectable here.

![Sankey top 25, table collapsed](https://github.com/user-attachments/assets/c846e80f-b1d1-4330-bb46-162d021604fa)

Same view with the table folded away — useful when you want the Sankey's full vertical real estate. Click the table header to expand it again.

## Top-N is "top-N grouped tuples"

When you set top-N to 25, the response contains the **25 top group-by tuples**, ranked by your sort metric. The 26th-largest and beyond are folded into a synthetic `__other__` row that represents "everything else, summed".

If your aggregation produces enormously many distinct tuples (more than `query_max_groups`, default 50 000), an additional `__overflow__` row appears, summing everything that didn't fit in the in-memory accumulator. Both `__other__` and `__overflow__` are real rows in the response and may show up in the Sankey and the table — they aren't bugs, they're "everything off the bottom of the list".

To narrow further: filter, or change the aggregation columns. Bumping top-N higher (200, 500) helps for shallow searches; for serious investigation, filter.

## How to read the Table

The same data, sortable and column-customisable.

- One row per top-N tuple
- One column per aggregation field
- Plus `bytes` and `packets` columns, both sortable

`SRC_AS_NAME` and `DST_AS_NAME` columns get extra width because AS names are long. Latitude / longitude columns are present in the underlying data but **hidden by default** — they're carried through so the city map and globe views can use them, but they aren't useful in tabular form.

Click any column header to re-sort. Click a value to add a filter on that field. The same filter applies to the Sankey.

## The filter ribbon

A filter strip sits between the Sankey and the table. Three things you can do here:

- **Select facet values** — click a field, pick one or more values. The query updates.
- **Autocomplete** — type into a facet field; the dashboard suggests existing values from the live data. Useful for high-cardinality fields like AS names.
- **Free-text search** — anything you type in the search box runs as a regex against the raw journal data.

Filter logic is "AND across fields, OR within a field". Selecting `PROTOCOL = TCP` and `PROTOCOL = UDP` shows TCP-or-UDP. Selecting `PROTOCOL = TCP` plus `SRC_COUNTRY = US` shows only US-source TCP.

There is no negative match. To exclude a value, select all values and remove the unwanted one — works for low-cardinality fields, becomes impractical for high-cardinality ones (the autocomplete cap is 100).

See [Filters and Facets](/docs/network-flows/visualization/filters-facets.md) for the full mechanics.

## Choosing aggregation fields

Some shapes that work well:

- **Default**: `Source AS Name → Protocol → Destination AS Name`. The "who, on what, to whom" overview. Good first look.
- **Country flow**: `Source Country → Destination Country`. Cleanest geographic view. Combine with `protocol` for service-level detail.
- **Per-router slice**: `Exporter Name → Ingress Interface Name → Destination AS Name`. Use when you have per-router questions.
- **Service drill-down**: `Destination Port → Source AS Name`. Who's hitting your services.
- **Internal/external split**: `IN_IF_BOUNDARY → DST_COUNTRY → Destination ASN`. After labelling your boundaries via static metadata.

The order of fields determines the visual flow. Reorder to change which dimension is "left" and "right" in the Sankey.

## Things to know

### Doubling

Without filtering, aggregate volume on a router that exports both ingress and egress (a common configuration; vendor best practice is ingress-only) is roughly 2× the actual traffic — every packet generates two flow records, one ingress and one egress. To see real volume on a specific link, filter to one exporter and one interface (`Ingress Interface Name` OR `Egress Interface Name`, pick one). Each packet then appears in exactly one record on that interface. See [Anti-patterns](/docs/network-flows/anti-patterns.md) for the full framing.

### Sharing your view

The dashboard URL preserves all state — time range, filters, aggregation fields, top-N, sort. Copy the URL and share with anyone who has access to the same Netdata Cloud space.

### Limits

- `top_n` clamps to one of {25, 50, 100, 200, 500}.
- Maximum 10 group-by fields. More are silently truncated.
- Maximum 50 000 distinct group tuples per query (`query_max_groups`); over that, surplus folds into `__overflow__`.
- The query itself has a 30-second hard timeout.

## What's next

- [Filters and Facets](/docs/network-flows/visualization/filters-facets.md) — Filtering mechanics in detail.
- [Time-Series](/docs/network-flows/visualization/time-series.md) — How traffic evolves over the time window.
- [Maps and Globe](/docs/network-flows/visualization/maps-globe.md) — Geographic views.
- [Field Reference](/docs/network-flows/field-reference.md) — Which fields are available for aggregation.
- [Anti-patterns](/docs/network-flows/anti-patterns.md) — How to read the numbers correctly.
