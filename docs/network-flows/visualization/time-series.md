<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/network-flows/visualization/time-series.md"
sidebar_label: "Time-Series"
learn_status: "Published"
learn_rel_path: "Network Flows/Visualization"
keywords: ['time series', 'top-n over time', 'trends', 'visualization']
endmeta-->

<!-- markdownlint-disable-file -->

# Time-Series

The Time-Series view plots traffic over time. Same top-N selection as the Sankey + Table, but rendered as a stacked chart across the time range you've selected.

Use it for: anomaly detection, trending, comparing now to last week, capacity planning. Use the Sankey/Table view for: "what's the breakdown right now".

![Time-Series top 25 with table](https://github.com/user-attachments/assets/0bb2637d-632b-4e97-900f-b14155ab0771)

Stacked chart on top, table at the bottom. Each colored band is one of the 25 top groups, summed into time buckets. The table holds the same 25 rows with totals across the whole window.

## How it works

The view runs the same aggregation as the Sankey + Table:

1. The plugin scans the journal across your time range, aggregating by your group-by fields, summing bytes and packets.
2. It picks the top-N groups by your sort metric (bytes or packets) over the **whole** window.
3. It re-scans the journal and accumulates those top-N groups into time buckets.
4. The result is a stacked chart with one dimension per top-N group.

The top-N is computed once over the entire window — not per bucket. A flow that's huge for 5 minutes and absent the rest of the time may not make the top-N if a steady mid-volume flow accumulates more total bytes over the same window. If you want to see those bursts, narrow the time range or filter to the conversation you're investigating.

## Bucket size

The view auto-picks a bucket size based on the time range:

| Time range | Tier used | Bucket size |
|---|---|---|
| ≤ ~100 minutes | 1-minute | 60 seconds (the floor) |
| 100 minutes to ~8h20m | 5-minute | 300 seconds |
| ≥ ~8h20m | 1-hour | 3600 seconds |

The rule: pick the coarsest tier where the time window contains at least 100 buckets, with bucket size `max(tier_bucket, 60)`. For longer ranges, the bucket grows in proportion to keep the chart readable (capped at 500 buckets total).

The window is **rounded outward** to align with bucket boundaries — your "11:23:00 to 11:48:00" request may render as "11:23:00 to 11:48:30" if the bucket size doesn't divide your range evenly. This is intentional; it ensures every record in your window is reachable.

### Sub-minute zoom

The minimum bucket size is **60 seconds**. Zoom in past one minute and the chart silently widens to 60-second buckets. There's no warning — sub-minute jitter just smooths out. For sub-second analysis, flow data is the wrong tool ([microbursts are invisible](/docs/network-flows/anti-patterns.md)).

## What forces raw tier

Some queries can't use the rollup tiers. They drop to raw tier and inherit raw-tier retention:

- Filtering or grouping by `SRC_ADDR`, `DST_ADDR`, `SRC_PORT`, `DST_PORT`, or any geo city / latitude / longitude field
- Any non-empty full-text search

In those cases the 100-bucket rule still applies, but the source tier is raw tier. Time depth is bounded by raw-tier retention (default: the raw tier has its own 10GB / 7d retention limits, and busy collectors often hit the size cap before 7 days).

If you've been working at a higher tier and add an IP filter, the time depth on your chart may suddenly shrink — that's the tier switch.

## "No data" buckets

Buckets that received no contributing records render as zero. There's no special "missing data" indicator on the chart — the plot is flat at zero in those regions.

That includes the case where the time range crosses the retention boundary of a tier. Raw-tier (raw) holds the most recent data; older fragments fall back to coarser tiers when available, and emptiness when no tier has the span.

The dashboard's diagnostic side-panels surface tier coverage in the response stats (`query_tier`, `query_files`, etc.), but the chart itself doesn't visually distinguish "no data" from "zero".

## Group overflow

Same overflow semantics as the Sankey + Table view. If your aggregation produces more than `query_max_groups` (default 50 000) distinct group tuples, the surplus is folded into a synthetic `__overflow__` group, which appears as one of the chart's dimensions. Look for the warning in the response stats; narrow your filter or reduce the group-by depth to avoid it.

## What controls are available

Same controls as the other views:

- **Time range** — Netdata's global time picker
- **Filters** — facet selections + autocomplete + full-text search (in the filter ribbon)
- **Top-N** — 25 / 50 / 100 / 200 / 500
- **Sort by** — bytes or packets (determines what "top" means and what units the chart uses)
- **Group-by fields** — same as Sankey, 1-10 fields. The chart shows one stacked dimension per surviving top-N group

The default group-by is `Source AS Name → Protocol → Destination AS Name`, same as Sankey + Table.

## Things that go wrong

- **Bursty flow not in top-N.** Top-N is over the whole window. Narrow the time range or filter to that conversation.
- **Sub-minute zoom doesn't render finer.** The 60-second floor is hard. For finer detail, use packet capture.
- **Wide range plus IP filter shows less than expected.** IP filter forced raw tier; raw retention is your bound.
- **Window appears to extend slightly beyond what you asked.** Bucket alignment rounds outward.
- **`__overflow__` shows up as the biggest dimension.** Your group-by is producing more distinct tuples than `query_max_groups` (50 000). Narrow the filter or drop a high-cardinality group-by field.

## What's next

- [Sankey and Table](/docs/network-flows/visualization/summary-sankey.md) — The default view; same aggregation, point-in-time.
- [Retention and Querying](/docs/network-flows/retention-querying.md) — How tiers map to time ranges.
- [Filters and Facets](/docs/network-flows/visualization/filters-facets.md) — Narrowing the data.
- [Anti-patterns](/docs/network-flows/anti-patterns.md) — Common misuses to avoid when reading time-series volume.
