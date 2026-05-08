<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/network-flows/enrichment.md"
sidebar_label: "Enrichment"
learn_status: "Published"
learn_rel_path: "Network Flows"
keywords: [enrichment, geoip, asn, bgp, classifiers, network labels, mmdb, ipam]
endmeta-->

<!-- markdownlint-disable-file -->

# Enrichment

Raw flow records carry IP addresses, ports, ASNs the exporter happens to know, ifIndex numbers, and not much else. Enrichment is the post-decode pipeline that turns those into the operational labels you actually want on the dashboard: country and city, AS numbers and AS names, BGP next-hop and AS path, "this is our DMZ in Frankfurt", "this interface is the Lumen transit", "this exporter is a leaf in dc-fra1".

This page is the cross-cutting reference for that pipeline. It documents the order of evaluation, the provider chains that decide where each field comes from, the shared mechanisms that span all enrichment methods, and the operational properties you need to know about. **Per-method specifics — installation steps, refresh cadence, expected upstream schemas, vendor-specific gotchas — live on the integration cards** linked at the bottom of this page.

## Order of evaluation per flow record

Every flow record passes through the same pipeline before it is written to the journal. For one record:

1. **Decode** — the raw NetFlow / IPFIX / sFlow message is parsed into typed fields (`SRC_ADDR`, `DST_ADDR`, `SRC_AS`, ifIndexes, etc.).
2. **Decapsulation** — when `protocols.decapsulation_mode` is `srv6` or `vxlan` and the exporter ships inner-packet bytes, the inner 5-tuple replaces the outer one. Everything below operates on the inner addresses.
3. **GeoIP MMDB lookups** — the resolver runs the source and destination IPs against every configured ASN MMDB and every configured geo MMDB. Country, state, city, coordinates, AS number candidate, AS name, and the `ip_class` flag are seeded from the result.
4. **Static metadata** — `enrichment.metadata_static.exporters` is matched against the exporter's IP (UDP source) and the ifIndex; `enrichment.networks` is matched per-IP, longest-prefix wins. Static `networks` entries can override country, state, city, latitude, longitude, AS number, and the `*_NET_*` labels.
5. **Dynamic network sources** — every `enrichment.network_sources.<name>` source contributes CIDR records to the same network-attribute resolution path. Lookups for the source/destination IP merge matching records with static `enrichment.networks` entries.
6. **Classifiers** — `exporter_classifiers` runs once per exporter; `interface_classifiers` runs twice per record (once for the input interface, once for the output). They write `EXPORTER_*` and `IN_IF_*` / `OUT_IF_*` fields. **Classifiers are skipped entirely when static metadata already set any classification field on the same target.**
7. **Routing overlay** — the BGP-fed routing trie (BMP and BioRIS contribute to it) is consulted. The `asn_providers` and `net_providers` chains decide whether the AS number and the network mask come from the flow record, from BGP, or from the GeoIP MMDB. BGP-only fields (`NEXT_HOP`, `DST_AS_PATH`, `DST_COMMUNITIES`, `DST_LARGE_COMMUNITIES`) are written from BGP data.
8. **Journal write** — the resulting record is written to the raw tier; rollup tiers (1 minute, 5 minutes, 1 hour) are computed from it asynchronously.

The enricher runs when any enrichment feature is configured, including provider-chain settings. A deployment with all enrichment settings disabled writes raw decoded flow fields only.

## The two provider chains

A "provider chain" is an ordered list of where to look for a given field. The plugin walks the chain left-to-right and takes the first non-zero answer. Two chains exist:

```yaml
enrichment:
  asn_providers: [flow, routing, geoip]    # default
  net_providers: [flow, routing]            # default
```

These are the default provider chains.

### `asn_providers` — where the AS number comes from

The chain decides where `SRC_AS` and `DST_AS` come from. The AS *name* (`SRC_AS_NAME`, `DST_AS_NAME`) is **always** rendered separately from the ASN MMDB lookup, regardless of the chain — see "AS numbers vs AS names" below.

| Provider | What it returns |
|---|---|
| `flow` | The `SRC_AS` / `DST_AS` the exporter put on the record |
| `flow_except_private` | Same, but treats private/reserved AS numbers as zero |
| `flow_except_default_route` | Same, but treats `AS 0 with mask 0` as zero |
| `routing` | The AS from the BGP-fed routing trie (BMP, BioRIS, or static routing) |
| `routing_except_private` | Same as `routing` with the private filter |
| `geoip` | **Terminal "use 0" shortcut** — see below |

`bmp` and `bmp-except-private` are accepted as backward-compatible aliases for `routing` and `routing_except_private`.

The `geoip` slot is a **terminal shortcut**, not a normal provider. When the chain reaches `geoip`, the AS number is forced to `0` and the chain ends. The MMDB-derived AS number is then applied separately, outside the chain. This is intentional compatibility behaviour, not a bug.

The implication is operational: **putting `geoip` anywhere except last truncates the chain.** With `[geoip, flow, routing]`, every AS resolves to `0` from the chain — only the GeoIP-derived AS makes it through, and `flow` and `routing` never run. This is rarely what you want.

Common chain orderings:

| Configuration | Behaviour |
|---|---|
| `[flow, routing, geoip]` (default) | Trust the exporter, fall back to BGP, then to GeoIP |
| `[flow, routing]` | No GeoIP at all |
| `[routing, flow, geoip]` | Trust the BGP feed first — useful when exporters have stale AS |
| `[flow_except_private, routing, geoip]` | Drop private AS from flow data, let routing fill in |

`is_private_as` returns true for `0`, `23456` (RFC 4893 transition reserved), `64496..=65551` (documentation / private), and `>= 4_200_000_000` (32-bit private and high-reserved).

### `net_providers` — where the network mask comes from

Same idea, smaller menu: the chain produces `SRC_MASK` and `DST_MASK` (and indirectly `NEXT_HOP` via the routing entry). Only `flow` and `routing` are valid here — there is no `geoip` slot, because the MMDB does not carry network masks.

### AS numbers vs AS names

The two are resolved by different mechanisms and never share the chain.

- **AS numbers** come from the chain above.
- **AS names** are always rendered from the resolved AS number plus the ASN MMDB. The format is `AS{n}` for non-zero ASNs (with `{organisation}` appended when the MMDB has it), `AS0 Unknown ASN` when the resolved AS is zero, and `AS0 Private IP Address Space` when the ASN MMDB tagged the IP as private. There is no static configuration option for AS names.

A static `enrichment.networks.<cidr>.asn` override is applied during the per-IP network-attribute merge **before** the chain runs. The AS name still goes through the MMDB lookup.

## How attributes compose: GeoIP, network sources, static `networks`

For any source or destination IP, the plugin produces network attributes by merging multiple inputs. The merge rule is "non-empty overlay overwrites; empty overlay leaves the field alone" — so the **last input with a non-empty value wins, per field.**

The merge order, per IP, is:

1. **GeoIP base layer** — country, state, city, coordinates, AS number candidate, AS name, `ip_class`.
2. **All matching prefixes in ascending prefix-length order** (least-specific first, most-specific last). At each prefix length:
   - **Dynamic `network_sources` records** are merged first (priority 0).
   - **Static `enrichment.networks` records** are merged second (priority 1).

Two consequences worth stating explicitly:

- **Specificity dominates.** A more-specific prefix (longer mask) always wins on its non-empty fields, regardless of whether it came from a static source or a dynamic feed. A `/24` static entry overwrites a `/16` dynamic entry; a `/24` dynamic entry overwrites a `/16` static entry.
- **At the same prefix length, static `networks` wins.** Because static is merged after dynamic at each level, and the merge primitive is "non-empty wins", a non-empty field in a static entry overwrites the dynamic entry's value at the same prefix length. This is intentional — explicit operator config beats imported data — but it surprises operators who expect the remote feed to be authoritative.

A practical consequence of "merge ascending, non-empty wins": **a more-specific entry that leaves a field blank inherits the supernet's value for that field.** To clear a field on a `/24`, you must set it explicitly to a value, not just leave it out.

Static `networks` can also override `country`, `state`, `city`, `latitude`, `longitude`, and `asn` per CIDR. These compose with the same merge rules — they are simply additional non-empty fields that overwrite the GeoIP base layer when matched. Coordinates (`latitude`, `longitude`) are static-only; network-identity sources cannot set them.

## The MMDB shared mechanism

DB-IP, MaxMind GeoIP / GeoLite2, IPtoASN-converted-to-MMDB, and any custom MMDB build all use the same MMDB resolver behavior. The configuration:

```yaml
enrichment:
  geoip:
    asn_database:
      - /var/cache/netdata/topology-ip-intel/topology-ip-asn.mmdb
    geo_database:
      - /var/cache/netdata/topology-ip-intel/topology-ip-geo.mmdb
    optional: true
```

| Key | Type | Notes |
|---|---|---|
| `asn_database` | list of paths | One or more ASN MMDB files. Aliases: `asn-database`. |
| `geo_database` | list of paths | One or more geographic MMDB files. Aliases: `geo-database`, `country-database`. |
| `optional` | bool, default `false` | When `false`, a missing file at startup aborts the plugin. When `true`, missing or unreadable files are tolerated. Auto-detected files are always treated as `optional: true`. |

### Auto-detect path order

When neither `asn_database` nor `geo_database` is configured, the plugin searches in this order at startup:

1. **`<cache_dir>/topology-ip-intel/`** — where `cache_dir` is the parent of `journal.journal_dir` if that path is absolute, otherwise `NETDATA_CACHE_DIR`. Typically `/var/cache/netdata/topology-ip-intel/`.
2. **`<stock_data_dir>/topology-ip-intel/`** — typically `/usr/share/netdata/topology-ip-intel/`.

The canonical filenames are `topology-ip-asn.mmdb` and `topology-ip-geo.mmdb`. Native packages ship stock DB-IP files under the stock data directory. The [Intel Downloader](/docs/network-flows/intel-downloader.md) writes fresher copies to the cache directory, which takes precedence over stock files. Netdata does not install a downloader timer; schedule one if freshness matters.

### Composition: last non-empty wins, per field

`asn_database` and `geo_database` are lists. For each lookup, every database in the list runs in order; per output field, the **last** database that produces a **non-empty** value wins. Empty / zero values returned by a later database do not overwrite an earlier match.

This is the same merge rule as the network-attributes merge, applied to the MMDB scan.

The practical use is stacking: list a stronger source first for one field and a backup for the rest. For example, layering MaxMind after IPtoASN in `asn_database` recovers AS *names* (which IPtoASN-converted MMDBs often lack) without losing IPtoASN's AS *number* coverage.

### Signature-watch reload

The resolver checks each configured database's signature (size + mtime) every 30 seconds and reloads the readers in place when the signature changes.

Only successful reloads swap the active readers — a transient read error keeps the previous readers serving lookups.

This is why MMDB refresh scripts only need to atomically replace the file on disk: the plugin picks up the new file within 30 seconds, no restart needed.

### IPv4 / IPv6 dual-stack handling

The resolver inspects each MMDB's metadata. An IPv6 lookup against an IPv4-only database is **silently skipped** (no warning, no error). Most current providers (DB-IP, MaxMind GeoLite2, MaxMind GeoIP2, IPtoASN combined) ship a single dual-stack MMDB that covers both families; legacy single-family builds are the only case where this matters in practice.

## Network sources: shared operational properties

`enrichment.network_sources.<name>` is the dynamic counterpart to static `networks`. AWS IP Ranges, GCP IP Ranges, Azure IP Ranges, NetBox, and custom JSON-over-HTTP IPAM sources share the same operational contract. The only difference between cards is the upstream URL, the JSON shape, and the jq transform.

### Fetch loop and back-off

Each source runs in its own task. Multiple sources fetch in parallel; within a source, only one fetch is in flight at a time.

Per cycle:

1. HTTP GET (or POST) at `interval` cadence, with `headers:` applied.
2. Response parsed as JSON.
3. The configured `transform` (a [jaq](https://github.com/01mf02/jaq) expression) runs over the parsed JSON.
4. Output is decoded as a stream of `RemoteRecord` objects.
5. Records are merged into the shared network-attributes trie.

On any failure (HTTP error, JSON parse error, jq runtime error, **empty result**), the source backs off exponentially starting at `interval / 10` (floor 1 second), doubling on each retry, capped at `interval`. On success it resets to the configured `interval`.

The scheduler floors dynamic-source intervals at 60 seconds. So `interval: 5s` is effectively `60s`. There is no signature/etag change detection — the plugin re-parses and re-publishes the entire record set on every successful fetch.

### Expected jq output schema

The `transform` must emit a stream of objects with this shape:

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

- **Required**: `prefix` (CIDR string).
- **Optional**: every other field. Defaults to empty / 0.
- `asn` accepts an integer (`64500`), a string (`"64500"`), or AS notation (`"AS64500"`).
- **Coordinates are not settable** — the deserializer has no field for `latitude` / `longitude`. Use static `networks` for coordinates.

The default `transform` is `"."`, which returns the raw JSON object — never a per-prefix stream. **Every real source needs a custom `transform`**; the per-source integration cards each ship a working example.

If the transform produces zero objects on a successful HTTP fetch, the cycle is treated as a failure and triggers backoff. This catches silently broken jq but punishes legitimately empty IPAMs.

### Strict YAML validation

Configuration schemas use `deny_unknown_fields` at every level. A typo in `network_sources.<name>.<key>` fails at startup with a `unknown field 'X', expected one of ...` error. This applies to the entire enrichment config, not just network sources.

### TLS verification cannot be disabled

Configuration validation rejects any attempt to disable TLS verification. Legacy keys are accepted for forward compatibility, but no override exists.

Self-signed or internal CAs must be supplied via `tls.ca_file`. The rationale is deliberate: enrichment data flows directly into security investigations and capacity decisions — silently accepting MITM-able responses would corrupt every downstream analysis.

### Single page only — no pagination

The fetch is one-shot per cycle. There is no pagination, no cursor handling, no `Link: rel=next` following. Operators with paginated IPAM endpoints must either expose a separate "all prefixes" bulk endpoint (most IPAMs have one) or wrap with a server-side aggregator script.

This is irrelevant to AWS / GCP / Azure (single-file feeds) and bites NetBox / generic IPAM. The NetBox card documents `?limit=0` (NetBox 4.x) as the bypass.

### Authentication — bring your own headers

The plugin has no built-in OAuth flow, basic-auth helper, or token refresh. Whatever the upstream API needs goes into `headers:`:

```yaml
headers:
  Authorization: "Token abc123"
```

`headers` is a free-form map, so any single-shot scheme works. URL-embedded credentials (`https://user:pass@host/...`) are converted to HTTP Basic authentication by the HTTP client, but explicit `Authorization` headers are clearer and avoid storing credentials in URLs. Short-lived tokens must be refreshed outside Netdata and reloaded into the config.

POST is supported but **sent with no body**. POST endpoints that require a request body are not supported.

### Field-level merge across sources sharing prefixes

When two sources (say `aws` and a custom internal IPAM) both define overlapping prefixes, all records are merged into the same trie. Within one source, last-write-wins for an exact prefix length; across sources, the merge is field-level — non-empty fields from a later record overwrite earlier ones. This is the same primitive described in "How attributes compose" above.

### Diagnostic journal output

Failures (transform compile, HTTP, JSON parse, jq runtime, empty result) are logged in the Netdata journal namespace:

```bash
journalctl --namespace netdata | grep network-sources
```

JSON parse errors are silent in the dashboard — the journal is the place to look.

## Static metadata short-circuits classifiers

When `metadata_static.exporters` set **any** classification field (`group`, `role`, `site`, `region`, `tenant`) for an exporter, the entire `exporter_classifiers` rule chain is skipped for that exporter.

The same rule applies to interfaces — any of `provider` / `connectivity` / `boundary` set by static metadata short-circuits `interface_classifiers` for that interface.

This is "Akvorado parity" by design. The implication: **don't try to mix static and rule-based classification on the same target.** If you set even one field statically, all classifier rules for that target are bypassed. Decide per-target: enumerate it in static metadata, or let the classifier rules find it.

## Classifier evaluation surfaces and ordering

- **Exporter classifiers** run once per exporter, with the result cached.
- **Interface classifiers** run once per `(exporter, interface)` pair, **twice per flow record** (once for the input interface, once for the output).
- **First write wins** per output slot. Once a rule sets `EXPORTER_GROUP`, no later rule can change it. Order rules from most-specific to least-specific.
- **Cache TTL** is `enrichment.classifier_cache_duration` (default 5 minutes, last-access). Validation rejects values shorter than 1 second.
- **Runtime errors stop the rule list** but keep whatever was set so far. Use string-safe operators (`matches`, `startsWith`, `contains`) rather than `>` / `<` on string fields.

The classifier syntax is intentionally Akvorado-compatible for the documented operators and actions, so existing Akvorado rule lists usually paste in unchanged. Akvorado's full `expr-lang` features (arithmetic, ternaries, lambdas) are not implemented here.

## Decapsulation: the inner-packet override

When `protocols.decapsulation_mode` is `srv6` or `vxlan` and the exporter ships inner-packet bytes (NetFlow v9 IE 104, IPFIX IE 315, sFlow `SampledHeader`), the inner 5-tuple replaces the outer one **before any enrichment runs**:

- `SRC_ADDR`, `DST_ADDR`, `SRC_PORT`, `DST_PORT`, `PROTOCOL`, `ETYPE`, `IPTOS`, `IPTTL`, `IPV6_FLOW_LABEL`, `TCP_FLAGS`, MPLS labels, ICMP type/code, `BYTES` (inner L3 length).
- For VXLAN: inner `SRC_MAC` / `DST_MAC` / `SRC_VLAN` / `DST_VLAN`. **Outer MACs and VLANs are lost.**

This means **all downstream enrichment operates on the inner addresses**: GeoIP runs against the inner IPs, network attributes match the inner prefixes, and the routing trie is consulted with the inner addresses.

Two important rules:

- **Decap is destructive on non-tunnel traffic.** Records arriving via the L2-section path that are not the configured tunnel are **dropped**, not falled back to outer view. Plain NetFlow / IPFIX flow records that don't go through the L2-section path are unaffected.
- **VXLAN VNI is parsed but not surfaced.** Bytes 4-6 of the VXLAN header are skipped. Pure VNI-based segmentation is not visible as a filter or group-by field.

GRE, IP-in-IP, GENEVE, MPLS-over-UDP, and NVGRE are not decoded.

See the [Decapsulation integration card](/src/crates/netflow-plugin/integrations/decapsulation.md) for exporter configuration recipes.

## Routing overlay (BMP and BioRIS share the trie)

BMP and BioRIS are separate transports that feed the **same** in-memory routing trie. A deployment running BMP from internal routers and BioRIS from a separate bio-rd-compatible RIS endpoint gets unified enrichment without duplicate trie entries.

BioRIS means Netdata consumes the `bio.ris.RoutingInformationService` gRPC API. Netdata does not connect directly to RIPE RIS Live, RIPEstat, RIS MRT dumps, or RIPE route collector sessions. If you need a RIPE-derived external view, put a converter or bio-rd-compatible service in front of that data and point Netdata at the resulting gRPC endpoint.

Each prefix entry holds a list of routes keyed by `(peer, route_key)` so multipath BGP and multiple peers contributing the same prefix coexist. Lookups walk the trie longest-prefix-first, then refine within candidates by:

1. Exporter IP matches the flow's exporter AND next-hop matches the flow's next-hop;
2. otherwise exporter IP matches;
3. otherwise next-hop matches;
4. otherwise the first route at the matched prefix.

The fields populated are `SRC_AS` / `DST_AS` (via the `routing` provider in the chain), `SRC_MASK` / `DST_MASK` (via `net_providers`), and `NEXT_HOP`, `DST_AS_PATH`, `DST_COMMUNITIES`, `DST_LARGE_COMMUNITIES` (always written from BGP data when the lookup hits, no chain involvement).

AS *names* still come from the ASN MMDB, not from BGP. BGP gives accurate AS numbers, paths, and communities; the names come from the database.

## Operational properties that span all methods

### Refresh windows

| Mechanism | Window | Notes |
|---|---|---|
| GeoIP MMDB | **30 seconds** | Signature watch (size + mtime) |
| Dynamic network sources | per-source `interval`, **floor 60 s** | Independent per source |
| BMP routing | live | Routers push updates over TCP |
| BioRIS routing | live | gRPC stream from a bio-rd daemon |
| Static metadata | **plugin restart** | No hot-reload |
| Classifier rules | **plugin restart** | No hot-reload |

### Restart behaviour

- **GeoIP databases** are reloaded automatically — no special handling needed.
- **The routing trie is not persisted.** A restart wipes BGP-derived data; it is re-learned as routers re-send Initiation + Update messages (BMP) or as bio-rd's next refresh cycle dumps the RIB (BioRIS). Convergence ranges from seconds (FRR) to minutes for full-table BioRIS feeds. Schedule restarts off-peak when BGP attribution matters.
- **Network-source records** are re-fetched on the next interval tick. There is no persistence between restarts.
- **Classifier caches** are wiped — they refill on first hit per target.

### No in-process freshness signal

There is no metric or alert for "your MMDB is too old", "your IPAM hasn't refreshed in 6 hours", or "your BMP session is stuck". Operators must watch:

- The MMDB file's `mtime` on disk.
- The Netdata journal (`journalctl --namespace netdata | grep -E 'network-sources|geoip|bmp|bioris'`) for fetch failures, parse errors, and session events.
- Per-source last-success timestamps (where the integration card documents them).

### Empty enrichment trees disable the enricher

When every static and dynamic input is empty, no enrichment runs and the journal carries no `*_NET_*`, `*_AS_NAME`, `*_COUNTRY`, etc. Useful for verifying that adding config actually opted you in — if fields stay empty after a config change, suspect the enricher never started because the config did not enable any enrichment source.

### Field survival into rollup tiers

The journal stores raw records and three rollup tiers (1 minute, 5 minutes, 1 hour). High-cardinality fields are dropped from rollups to keep them tractable.

| Field | Raw | 1 m | 5 m | 1 h | Notes |
|---|---|---|---|---|---|
| `SRC_COUNTRY`, `DST_COUNTRY` | yes | yes | yes | yes | ~250 values |
| `SRC_GEO_STATE`, `DST_GEO_STATE` | yes | yes | yes | yes | Low thousands |
| `SRC_AS`, `DST_AS`, `SRC_AS_NAME`, `DST_AS_NAME` | yes | yes | yes | yes | Bounded by global routing table size |
| `SRC_NET_*`, `DST_NET_*` (NAME, ROLE, SITE, REGION, TENANT) | yes | yes | yes | yes | Operator-defined cardinality |
| `NEXT_HOP` | yes | yes | yes | yes | |
| `SRC_GEO_CITY`, `DST_GEO_CITY` | yes | — | — | — | **Raw tier only** |
| `SRC_GEO_LATITUDE`, `DST_GEO_LATITUDE` | yes | — | — | — | **Raw tier only**, hidden by default |
| `SRC_GEO_LONGITUDE`, `DST_GEO_LONGITUDE` | yes | — | — | — | **Raw tier only**, hidden by default |
| `DST_AS_PATH`, `DST_COMMUNITIES`, `DST_LARGE_COMMUNITIES` | yes | — | — | — | **Raw tier only** |
| `MPLS_LABELS`, `SRC_MAC`, `DST_MAC`, NAT fields | yes | — | — | — | **Raw tier only** |

City / coordinate / AS-path / community queries that span longer windows than the raw retention will silently fall back to a rollup tier and return empty. Narrow the window or use country / state.

See [Retention and querying](/docs/network-flows/retention-querying.md) and [Field reference](/docs/network-flows/field-reference.md) for the full picture.

### Geographic accuracy is best-effort

City-level GeoIP is accurate for many public IPs but wrong for VPNs, mobile carriers, and cloud-provider egress IPs (which often resolve to the cloud region's central city, not the actual user). Use country and state for trends; use city only after validating it for the prefix you care about. ASN ownership data also drifts — companies merge, prefixes get reassigned. A database older than a quarter or two starts labelling reassigned prefixes with the previous owner.

### Sampling-rate knobs

Two related, easily confused knobs (set under `enrichment` in `netflow.yaml`):

- `default_sampling_rate` — consulted **only** when the flow record did not carry a rate.
- `override_sampling_rate` — **always** wins when its prefix matches the exporter IP (the source IP of the UDP datagram), regardless of what the flow carried.

Both accept either an integer (uniform rate) or a CIDR-keyed map. The exporter match key is the same one used by `metadata_static.exporters` lookup — the source IP of the UDP datagram, not a router-internal management ID.

### Validate BGP-derived enrichment before relying on it

BMP and BioRIS depend on router configuration, export policy, peer identity, and route-refresh behaviour. Treat configuration changes as production-impacting and validate against your specific routers before relying on BGP-derived data for capacity or security decisions.

## What can go wrong

- **AS numbers all zero.** `geoip` is mid-chain. Reorder to put `geoip` last.
- **Map renders empty over a long window.** City / lat / lon are raw-tier only; the query auto-fell-back to a rollup tier. Narrow the window or use the country / state map.
- **Static-defined exporter doesn't get classifier labels.** Static metadata set at least one classification field; classifiers are skipped for that target. Decide one or the other per target.
- **Remote feed value not appearing — static config silently won.** Static `networks` wins ties at the same prefix length. Either use a more-specific dynamic entry or remove the conflicting static field.
- **Network source backs off on an empty IPAM.** Empty result is treated as failure. Workaround: have the upstream return at least one synthetic prefix.
- **Decap drops legitimate traffic.** When `decapsulation_mode` is set, non-tunnel traffic on the L2-section path is dropped, not fallen back. Either use a uniform encapsulation per exporter or disable decap.
- **`speed: 1000` means 1 kbps.** The static-metadata `speed` field is in **bits per second**. A 1 Gbps interface is `1000000000`.
- **MMDB updates not picked up.** Verify the file actually changed on disk (size or mtime). The 30-second poll only triggers on signature change.
- **Network-source TLS cannot be disabled.** Use `tls.ca_file` for internal CAs.

## What's next

### Per-method integration cards

These cards carry the per-method specifics — installation steps, refresh cadence, expected upstream schemas, vendor-specific gotchas — that this page deliberately does not duplicate.

**IP intelligence (MMDB)**
- [DB-IP IP Intelligence](/src/crates/netflow-plugin/integrations/db-ip_ip_intelligence.md) — the default that ships with Netdata
- [MaxMind GeoIP / GeoLite2](/src/crates/netflow-plugin/integrations/maxmind_geoip_-_geolite2.md) — commercial GeoIP2 or free GeoLite2 with attribution
- [IPtoASN](/src/crates/netflow-plugin/integrations/iptoasn.md) — public-domain ASN + country, hourly cadence
- [Custom MMDB Database](/src/crates/netflow-plugin/integrations/custom_mmdb_database.md) — your own MMDB build

**BGP routing**
- [BMP (BGP Monitoring Protocol)](/src/crates/netflow-plugin/integrations/bmp_bgp_monitoring_protocol.md) — routers push BGP updates over TCP
- [bio-rd RIS](/src/crates/netflow-plugin/integrations/bio-rd_-_ripe_ris.md) — pull BGP data from a bio-rd-compatible RIS gRPC daemon

**Network identity (cloud IP ranges, IPAM)**
- [AWS IP Ranges](/src/crates/netflow-plugin/integrations/aws_ip_ranges.md) — public AWS prefix list
- [GCP IP Ranges](/src/crates/netflow-plugin/integrations/gcp_ip_ranges.md) — public GCP prefix list
- [Azure IP Ranges](/src/crates/netflow-plugin/integrations/azure_ip_ranges.md) — Azure Service Tags (requires an internal mirror)
- [NetBox](/src/crates/netflow-plugin/integrations/netbox.md) — open-source IPAM / DCIM
- [Generic JSON-over-HTTP IPAM](/src/crates/netflow-plugin/integrations/generic_json-over-http_ipam.md) — Infoblox, BlueCat, phpIPAM, custom CMDBs

**Static and rule-based**
- [Static Metadata](/src/crates/netflow-plugin/integrations/static_metadata.md) — per-exporter, per-interface, per-CIDR labels
- [Classifiers](/src/crates/netflow-plugin/integrations/classifiers.md) — Akvorado-compatible expression rules

**Decapsulation**
- [Decapsulation](/src/crates/netflow-plugin/integrations/decapsulation.md) — SRv6 and VXLAN inner-packet extraction

### Related concepts

- [Intel Downloader](/docs/network-flows/intel-downloader.md) — the bundled tool that fetches and converts MMDB files for the auto-detect path.
- [Configuration](/docs/network-flows/configuration.md) — the full `enrichment` section reference.
- [Field reference](/docs/network-flows/field-reference.md) — every flow record field, what writes it, what reads it.
- [Retention and querying](/docs/network-flows/retention-querying.md) — which fields survive into rollups, how queries pick a tier.
