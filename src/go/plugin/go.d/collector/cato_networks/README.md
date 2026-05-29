# Cato Networks collector

This collector monitors Cato Networks accounts through the Cato GraphQL API.

It collects:

- site discovery and account snapshot data;
- site and interface traffic, latency, jitter, packet loss, and discard metrics;
- per-site BGP peer status and route counts;
- Cato site, PoP, tunnel, and BGP topology data for Netdata topology views.

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

Use the collector logs first. Collection failures include a normalized `error_class` in the log message.

If the failure class is `auth`, verify the API key, account ID, endpoint region, and Cato API permissions.

If the failure class is `network`, `tls`, or `proxy`, verify DNS, firewall egress, TLS inspection, and proxy configuration before changing collector options.

If the failure class is `decode`, save debug logs and open an issue with the Cato API operation name. This usually means the live API payload differs from the SDK schema or the tested fixtures.

If `entityLookup` fails after an earlier successful discovery, the collector continues with the cached site list and logs a rate-limited warning. A first-run `entityLookup` failure fails the collection because no cached site list exists yet.

If the API reports rate limits:

- increase `update_every`;
- narrow `site_selector`.

BGP is refreshed as a rolling scan. Large accounts may need multiple collection intervals before every site's BGP state is refreshed.
