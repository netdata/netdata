<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/network-flows/enrichment/network-identity.md"
sidebar_label: "Network Identity"
learn_status: "Published"
learn_rel_path: "Network Flows/Enrichment Concepts"
keywords: ['network identity', 'network sources', 'ipam', 'cmdb', 'cloud ip ranges', 'enrichment', 'concept']
endmeta-->

# Network Identity

Network-identity enrichment labels your own network prefixes with names, roles, sites, regions, tenants, and country / city overrides. Where [IP intelligence](/docs/network-flows/enrichment/ip-intelligence.md) tells you "this IP is in Germany" from a public database, network-identity tells you "this prefix is our staging environment in Frankfurt" from your authoritative source.

The data comes from external feeds — cloud-provider published prefix lists (AWS, GCP, Azure), IPAM systems (NetBox, Infoblox, BlueCat, phpIPAM), and custom CMDBs. Each source is configured as a separate integration card. This page covers the **cross-cutting concept**: how the lookups combine, what fields can be set, the operational rules.

## What it populates

| Field | Notes |
|---|---|
| `SRC_NET_NAME` / `DST_NET_NAME` | Friendly name |
| `SRC_NET_ROLE` / `DST_NET_ROLE` | Role tag (e.g., `dmz`, `office`, `iot`) |
| `SRC_NET_SITE` / `DST_NET_SITE` | Physical site |
| `SRC_NET_REGION` / `DST_NET_REGION` | Region |
| `SRC_NET_TENANT` / `DST_NET_TENANT` | Tenant |
| `SRC_COUNTRY` / `DST_COUNTRY` | Country override (when set explicitly) |
| `SRC_GEO_STATE` / `DST_GEO_STATE` | State / province override |
| `SRC_GEO_CITY` / `DST_GEO_CITY` | City override |

What network-identity sources cannot set: `SRC_GEO_LATITUDE` / `DST_GEO_LATITUDE`, `SRC_GEO_LONGITUDE` / `DST_GEO_LONGITUDE`. Coordinates are static-only — use the [`networks` block in static metadata](/docs/network-flows/enrichment/static-metadata.md) for those.

The per-row `asn` field can also override the AS *number* via the resolution chain. The AS *name* still comes from the [ASN MMDB](/docs/network-flows/enrichment/ip-intelligence.md) — there is no `asn_name` override in network-identity sources.

## Lookup priority

In the network-attributes resolution merge order:

1. **GeoIP** seeds the base layer.
2. **Network-identity sources** (cloud IP ranges, IPAM, generic IPAM) merge on top — at each prefix length (least-specific to most-specific).
3. **Static `networks` config** merges last — at each prefix length, **after** network-identity sources.

So when a prefix is defined in both a remote source and the static config, the static config wins on any non-empty field. This is intentional: explicit operator configuration overrides imported data.

## How a fetch works

For each configured source:

1. The plugin issues an HTTP request (default GET, or POST if configured) at the `interval` cadence.
2. Headers configured under `headers:` are added (typically for authentication).
3. The response body is parsed as JSON.
4. The configured `transform` (a [jaq](https://github.com/01mf02/jaq) jq-equivalent expression) runs over the parsed JSON.
5. The transform must produce a stream of objects, each with a `prefix` field (a CIDR string) and any of the optional attribute fields.
6. The records are merged into the network-attributes trie.

Each source runs in its own task. Multiple sources fetch in parallel; within a source, only one fetch is in flight at a time.

On any failure (HTTP error, JSON parse error, jq runtime error, empty result), the source backs off exponentially (starting at `interval / 10`, doubling up to `interval`) and retries. On success it resets to the configured `interval`.

## The expected jq output shape

The `transform` is a jq expression compiled by jaq. It receives the entire parsed JSON body and must produce a **stream of objects**.

Each output object should look like:

```json
{
  "prefix": "10.0.0.0/8",
  "name": "internal",
  "role": "lan",
  "site": "fra1",
  "region": "eu-central",
  "country": "DE",
  "state": "HE",
  "city": "Frankfurt",
  "tenant": "tenant-a",
  "asn": 64500,
  "asn_name": "Internal AS"
}
```

Required: `prefix`. All other fields are optional and default to empty / 0.

The `asn` field accepts an integer (`64500`), a string (`"64500"`), or the AS notation (`"AS64500"`).

If the transform produces nothing (empty result), the cycle is treated as a failure and triggers backoff. The same applies to non-object rows — every output element must be an object.

## TLS verification cannot be disabled

The configuration accepts the legacy keys `tls.verify` and `tls.skip_verify` for compatibility, but the validation layer **rejects** any attempt to disable verification (`tls.verify: false` or `tls.skip_verify: true`). Self-signed or internal CAs must be supplied via `tls.ca_file`. There is no override.

This is deliberate. Network-identity data flows directly into enrichment that affects security investigations and capacity decisions — silently accepting MITM-able responses would corrupt every downstream analysis.

## Single page only

The fetch is one-shot per cycle. There is no pagination, no cursor handling, no `Link: rel=next` following. If your IPAM exposes paginated endpoints, either:

- Expose a separate "all prefixes" bulk endpoint (most IPAMs have one).
- Wrap with a server-side script that aggregates all pages and serves the result at one URL.

## Authentication

The plugin has no built-in OAuth flow, basic-auth helpers, or token refresh. Set whatever the API needs explicitly:

```yaml
headers:
  Authorization: "Token abc123"
```

If your endpoint needs short-lived tokens, refresh them outside Netdata and put the current valid token in the headers config (and reload).

## Available sources

Each is configured as a separate integration card. See the per-source card for setup details:

- **AWS IP Ranges** — public AWS prefix list with per-region and per-service tagging
- **GCP IP Ranges** — public GCP prefix list with per-scope and per-service tagging
- **Azure IP Ranges** — published per Azure Service Tags (requires an internal mirror because Azure's URL rotates weekly)
- **NetBox** — open-source IPAM / DCIM, REST API with bearer-token auth
- **Generic JSON-over-HTTP IPAM** — catch-all for Infoblox, BlueCat, phpIPAM, custom CMDBs

## What can go wrong

- **Endpoint is paginated.** Only the first page is fetched. Use a bulk endpoint or wrap with a server-side script.
- **Default interval is 60s.** Fast for an IPAM, slow for AWS/GCP ranges. Tune per source — daily is fine for cloud IP ranges, 5-15 minutes for IPAMs that change often.
- **TLS verify cannot be disabled.** Use `tls.ca_file` for internal CAs.
- **Empty result from the transform** is treated as failure. If your endpoint returns no prefixes (legitimate state for a quiet IPAM), the source backs off as if it errored. Workaround: have the upstream return at least one synthetic prefix.
- **Authorization header must be in `headers:`**, not in the URL. URLs with embedded credentials (`https://user:pass@host`) are not specially handled.
- **JSON parse errors are silent in the dashboard.** Watch the Netdata journal (`journalctl -u netdata | grep network_sources`) for warnings.
- **Static config silently wins ties.** When a prefix is defined in both a remote source and `networks:`, the static config's values overwrite the remote ones. This is by design but can surprise operators expecting the remote feed to be authoritative.

## What's next

- **AWS IP Ranges, GCP IP Ranges, Azure IP Ranges, NetBox, Generic JSON-over-HTTP IPAM** — per-source integration cards with concrete setup instructions and example jq transforms.
- [Static metadata](/docs/network-flows/enrichment/static-metadata.md) — Static `networks` block (overrides network-identity at the same prefix length).
- [IP Intelligence](/docs/network-flows/enrichment/ip-intelligence.md) — The base layer that network-identity merges on top of.
- [ASN resolution](/docs/network-flows/enrichment/asn-resolution.md) — How the per-row `asn` field plugs in.
