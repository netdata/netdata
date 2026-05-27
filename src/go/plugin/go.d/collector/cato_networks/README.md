# Cato Networks collector

This collector monitors Cato Networks accounts through the Cato GraphQL API.

It collects:

- site discovery and account snapshot data;
- site and interface traffic, latency, jitter, packet loss, and discard metrics;
- per-site BGP peer status and route counts;
- Cato site, PoP, tunnel, and BGP topology data for Netdata topology views.
- collector health and API failure diagnostics.

## Configuration

Edit `go.d/cato_networks.conf` and set at least:

```yaml
jobs:
  - name: account
    account_id: "12345"
    api_key: "replace-with-cato-api-key"
```

The default endpoint is `https://api.catonetworks.com/api/v1/graphql2`. Production endpoints must use HTTPS because the collector sends the API key in request headers. HTTP is accepted only for loopback test endpoints.

Important options:

- `update_every`: collection interval in seconds. Minimum is `60`.
- `site_selector`: space-separated simple patterns matched against Cato site ID or site name.
- `interface_selector`: space-separated simple patterns matched against Cato interface ID or interface name.
- `limits.max_sites`: maximum sites collected after `site_selector` filtering. `0` disables this cap.
- `limits.max_interfaces_per_site`: maximum interfaces collected per selected site after `interface_selector` filtering. `0` disables this cap.
- `discovery.refresh_every`: site rediscovery interval in seconds.
- `metrics.max_sites_per_query`: maximum site IDs sent in one `accountMetrics` query.
- `metrics.time_frame`: Cato TimeFrame value, such as `last.PT5M` or `utc.2020-02-11/{04:50:15--16:50:15}`.
- `metrics.group_interfaces`: controls the Cato `groupInterfaces` argument. `auto` leaves the argument unset so Cato applies its default; `enabled` and `disabled` send explicit `true` or `false`.
- `bgp.max_sites_per_collection`: maximum sites queried for BGP status during one BGP refresh.
- `bgp.peer_selector`: space-separated simple patterns matched against BGP peer remote IP or remote ASN.
- `bgp.max_peers_per_site`: maximum BGP peers collected per selected site after `bgp.peer_selector` filtering. `0` disables this cap.
- `bgp.refresh_every`: BGP refresh interval in seconds.

Selectors accept glob-style terms and `!` exclusions. For example, `!lab-* *` collects everything except IDs or names matching `lab-*`; exclusions win across all identity fields for the selected entity type.

## Permissions

The Cato API key must be allowed to read:

- `entityLookup`
- `accountSnapshot`
- `accountMetrics`
- `siteBgpStatus`

## Troubleshooting

Use the collector health charts first:

- `cato_networks.collector_collection_status`: last collection success.
- `cato_networks.collector_discovered_sites`: number of discovered Cato sites.
- `cato_networks.collector_selected_entities`: selected sites, interfaces, and BGP peers after selectors and caps.
- `cato_networks.collector_skipped_entities`: entities skipped by selector or cap.
- `cato_networks.collector_cardinality_limit_status`: whether a site, interface, or BGP peer cap was hit.
- `cato_networks.collector_bgp_scan_window`: estimated seconds needed to refresh BGP status for all discovered sites with the current `bgp.max_sites_per_collection` and `bgp.refresh_every`.
- `cato_networks.collector_bgp_scan_progress`: BGP sites queried per collection and sites with cached BGP state.
- `cato_networks.collector_operation_status`: last status by operation (`entityLookup`, `accountSnapshot`, `accountMetrics`, `siteBgpStatus`).
- `cato_networks.collector_operation_failures`: operation failures by normalized class (`auth`, `rate_limit`, `timeout`, `network`, `tls`, `proxy`, `decode`, `graphql`, `empty`, `pagination`, `canceled`, `error`).
- `cato_networks.collector_operation_affected_sites`: number of sites affected by partial `accountMetrics` or `siteBgpStatus` failures.
- `cato_networks.collector_collection_failures`: full collection failures by normalized class.
- `cato_networks.collector_normalization_issues`: schema or normalization issues by surface and normalized issue class. These indicate payloads that were accepted but had unexpected status values, unknown timeseries labels, empty BGP peers, or parse issues.

If the failure class is `auth`, verify the API key, account ID, endpoint region, and Cato API permissions.

If the failure class is `network`, `tls`, or `proxy`, verify DNS, firewall egress, TLS inspection, and proxy configuration before changing collector options.

If the failure class is `decode`, save debug logs and open an issue with the Cato API operation name. This usually means the live API payload differs from the SDK schema or the tested fixtures.

If `entityLookup` fails after an earlier successful discovery, the collector continues with the cached site list and records the operation failure. A first-run `entityLookup` failure reports collection failure in the collector health charts because no cached site list exists yet.

If the API reports rate limits:

- increase `update_every`;
- narrow `site_selector`, `interface_selector`, or `bgp.peer_selector`;
- reduce `metrics.max_sites_per_query`;
- reduce `bgp.max_sites_per_collection`;
- increase `bgp.refresh_every`.

If `collector_cardinality_limit_status` is `1`, the collector is intentionally skipping some site, interface, or BGP peer entities to protect the Agent and Cloud pipeline from unbounded series growth. Check `collector_skipped_entities` to identify whether the skip reason is `selector` or `limit`, then either narrow selectors or raise the relevant cap if the account has enough API and metric-cardinality headroom.

BGP is refreshed as a rolling scan. For large accounts, use `cato_networks.collector_bgp_scan_window` to estimate how long a complete BGP refresh takes. Reduce the window by increasing `bgp.max_sites_per_collection` only when the Cato account has enough API rate-limit headroom.
