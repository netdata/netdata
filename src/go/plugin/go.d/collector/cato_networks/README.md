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
- `url`: Cato GraphQL API endpoint.
- `timeout`: HTTP request timeout.

The site selector accepts glob-style terms and `!` exclusions. For example, `!lab-* *` collects everything except site IDs or names matching `lab-*`.

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
- `cato_networks.collector_selected_entities`: selected Cato sites after `site_selector`.
- `cato_networks.collector_skipped_entities`: Cato sites skipped by `site_selector`.
- `cato_networks.collector_bgp_scan_window`: estimated seconds needed to refresh BGP status for all discovered sites.
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
- narrow `site_selector`.

BGP is refreshed as a rolling scan. For large accounts, use `cato_networks.collector_bgp_scan_window` to estimate how long a complete BGP refresh takes.
