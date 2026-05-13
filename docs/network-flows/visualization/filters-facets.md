<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/network-flows/visualization/filters-facets.md"
sidebar_label: "Filters and Facets"
learn_status: "Published"
learn_rel_path: "Network Flows/Visualization"
keywords: ['filters', 'facets', 'autocomplete', 'search', 'fts', 'negative match', 'visualization']
endmeta-->

<!-- markdownlint-disable-file -->

# Filters and Facets

The filter ribbon (between the visualisation and the table) is how you narrow flow data to the subset you want. Filters apply to every view — Sankey, table, time-series, country/state/city maps, globe — at once.

## What you can filter on

Around 80 fields are available as facets. They're a subset of the full 91-field schema:

- **Excluded** — metric fields (`BYTES`, `PACKETS`, `FLOWS`, `RAW_BYTES`, `RAW_PACKETS`, `SAMPLING_RATE`), timestamp fields (`FLOW_START_USEC`, `FLOW_END_USEC`, `OBSERVATION_TIME_MILLIS`), and the four geo coordinate fields (latitude/longitude). These don't make sense as categorical filters.
- **Two virtual facets**: `ICMPV4` and `ICMPV6` — synthesised on the fly from `PROTOCOL` plus the type and code fields. Filtering on `ICMPV4 = "Echo Request"` gives you that ICMP message type without writing two separate filters.

Everything else — IPs, ports, protocol, AS numbers and names, country, state, city, exporter labels, interfaces, MACs, VLANs, NAT addresses, TCP flags, ToS, etc. — is filterable.

## Filter logic

Within a single field: **OR**. Selecting `PROTOCOL = TCP` and `PROTOCOL = UDP` shows TCP-or-UDP.

Across different fields: **AND**. Adding `SRC_COUNTRY = US` to the above shows TCP-or-UDP from the US.

## No negative match

You cannot directly say "everything except X". The workaround is to select all values and remove the unwanted one — works for low-cardinality fields like `PROTOCOL` (a handful of values). For high-cardinality fields like `SRC_AS_NAME`, the autocomplete only surfaces the top 100 values, so there's no practical way to "select all and remove".

Negative matching is not supported.

## Autocomplete

Type into a facet field and the dashboard suggests existing values from your live data. The list:

- Shows up to **100 matching values**, sorted alphabetically.
- Matching policy is per-field. Free-form text fields (`SRC_AS_NAME`, `EXPORTER_NAME`, `IN_IF_DESCRIPTION`, MAC addresses, AS paths, BGP communities, country/city/state names) match by **substring**, so typing `Akamai` finds `AS20940 Akamai International`. IPs and short numeric fields (ports, protocols, ASN numbers, interface speeds) match by **prefix**, so typing `10.0.` narrows to that range.
- Runs against an **in-memory snapshot of the live journal** plus on-disk FST sidecars for promoted high-cardinality fields. Autocomplete never reads the raw flow tiers, and is fast even on busy collectors.
- The autocomplete `term` is hard-capped at 256 bytes; longer requests are rejected.

For high-cardinality fields, autocomplete is the only practical way to discover values. You can't scroll a list of millions of IP addresses, but you can find one by typing what you remember.

**Autocomplete and regular filtering are different paths.** When you select a value from the dropdown, the resulting filter is **exact equality**, not substring. The dropdown only helps you discover values; the filter that gets applied is `key = value` (or `key in [values]`) and uses indexes — never a substring scan over flow data.

## Full-text search

The search box at the top of the filter ribbon performs a regex match against the raw journal payload bytes. Notes:

- The search is **regex**, not literal. `8.8.8.8` is a regex where `.` matches any byte — so it can match `8a8b8c8`, `888x888`, etc. To match the literal string, escape with backslashes: `8\.8\.8\.8`.
- The match is **byte-level** against the journal payload, so it can find substrings inside enriched fields (AS names, exporter names, country codes).
- Any non-empty search **forces raw tier**. The full-text search only works against the raw journal — it doesn't apply to the rollup tiers. Time depth is therefore bounded by raw-tier retention.
- The plugin's "fast aggregation" path is also disabled when full-text search is active, because aggregation needs to scan every record. Expect somewhat slower responses than tier-based aggregation queries.

For "find anything containing this string in any field", the search is the right tool. For "filter by an exact value of a specific field", use the facet on that field — it's faster and doesn't trigger raw-tier mode.

## URL preservation

Every filter and selection is preserved in the dashboard URL. Copy the URL and share it; the recipient sees the exact same view, provided they have access to the same Netdata Cloud space. The dashboard also remembers your last selections per session, so you'll land on the same configuration when you return.

A practical note: filters use a structured representation (per-field IN-list) that's easy to encode as a JSON payload but awkward to URL-encode. The dashboard handles this transparently for sharing — but anyone scripting their own queries against the function should use JSON-payload requests to the function, not GET-style args.

## Things that go wrong

- **Search for `192.168.1.1` matches unrelated rows.** Regex semantics: each `.` is "any byte". Escape: `192\.168\.1\.1`.
- **Time depth shrinks unexpectedly after typing in search.** Full-text search forces raw tier. Clear the search to use rollup tiers and longer time ranges.
- **Negative match is unsupported.** Workaround: select-all-minus-one for low-cardinality fields. For high-cardinality fields, use a positive filter that narrows the result set instead.
- **Filter on an ICMP virtual facet seems slower than expected.** `ICMPV4` / `ICMPV6` virtual facets aren't optimised by the journal index — they're evaluated per-record. The query still returns; the cost shows up as longer wall time on busy collectors.
- **`query_max_groups` exceeded.** Result rows after the limit fold into `__overflow__`. Narrow the filter or reduce group-by depth.
- **GET-style args don't carry selections.** When integrating the function call yourself, send a JSON payload — the dashboard does this automatically.

## What's next

- [Sankey and Table](/docs/network-flows/visualization/summary-sankey.md) — The view that filters drive most often.
- [Retention and Querying](/docs/network-flows/retention-querying.md) — Why filters can shift the tier the query uses.
- [Field Reference](/docs/network-flows/field-reference.md) — Which fields are available as facets.
- [Investigation Playbooks](/docs/network-flows/investigation-playbooks.md) — Practical filter-driven workflows.
